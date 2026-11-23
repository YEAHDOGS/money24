package aicli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ASK_TIMEOUT    = 60 * time.Second // past this a news answer is worth nothing
	MAX_OUTPUT     = 1 << 20
	WORK_PREFIX    = "ask-"
	DIR_PERMS      = 0o700
	FILE_PERMS     = 0o600
	ERR_TAIL_CHARS = 300
	STATE_OK       = "ok"
	STATE_SIGN_IN  = "sign in"
	STATE_LIMIT    = "out of usage"
	STATE_MISSING  = "not installed"
	STATE_FAILED   = "failed"
	STATE_SLOW     = "too slow"
)

// Answer is one app's reply to one question.
type Answer struct {
	App     string          `json:"app"`
	Model   string          `json:"model"`
	Started time.Time       `json:"started_at"`
	Millis  int64           `json:"ms"`
	State   string          `json:"state"`
	JSON    json.RawMessage `json:"answer,omitempty"`
	Error   string          `json:"error,omitempty"`
	Web     bool            `json:"web,omitempty"` // the app was allowed to search the web for this answer
}

// Question is one fixed-shape ask: a prompt and the JSON Schema the answer must match.
type Question struct {
	Prompt string
	Schema map[string]any
	// Deep uses each app's DeepModel at DEEP_EFFORT: for a person's question, where speed matters less.
	Deep bool
	// Web lets the apps that can search the web and read pages. Nothing else is allowed.
	Web bool
	// Timeout overrides ASK_TIMEOUT.
	Timeout time.Duration
}

// Runner asks one app one question. Tests replace it.
type Runner func(ctx context.Context, app App, q Question, workRoot string) Answer

// Asker sends questions to a set of apps, each in its own empty folder under WorkRoot.
type Asker struct {
	Apps     []App
	WorkRoot string
	Run      Runner
}

// Run asks one app with the real command-line tool.
func Run(ctx context.Context, app App, q Question, workRoot string) Answer {
	model := app.Model
	if q.Deep {
		model = app.DeepModel
	}
	ans := Answer{App: app.ID, Model: model, Started: time.Now(), Web: q.Web && app.Web}
	finish := func(state string, err error) Answer {
		ans.Millis = time.Since(ans.Started).Milliseconds()
		ans.State = state
		if err != nil {
			ans.Error = err.Error()
		}
		return ans
	}
	path, err := app.Path()
	if err != nil {
		return finish(STATE_MISSING, err)
	}
	schemaJSON, err := json.Marshal(q.Schema)
	if err != nil {
		return finish(STATE_FAILED, err)
	}
	if err := os.MkdirAll(workRoot, DIR_PERMS); err != nil {
		return finish(STATE_FAILED, err)
	}
	// An empty folder per ask: no project instructions to read and no files to touch.
	dir, err := os.MkdirTemp(workRoot, WORK_PREFIX+app.ID+"-")
	if err != nil {
		return finish(STATE_FAILED, err)
	}
	defer os.RemoveAll(dir)
	schemaFile, answerFile := filepath.Join(dir, SCHEMA_FILE), filepath.Join(dir, ANSWER_FILE)
	if err := os.WriteFile(schemaFile, schemaJSON, FILE_PERMS); err != nil {
		return finish(STATE_FAILED, err)
	}
	prompt := q.Prompt
	if app.schemaInPrompt {
		prompt += SCHEMA_PROMPT + string(schemaJSON)
	}
	limit := cmp.Or(q.Timeout, ASK_TIMEOUT)
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	c := call{Model: model, SchemaJSON: string(schemaJSON), SchemaFile: schemaFile, AnswerFile: answerFile, Prompt: prompt,
		Deep: q.Deep, Web: q.Web && app.Web}
	cmd := exec.CommandContext(ctx, path, app.args(c)...)
	cmd.Dir = dir
	hideWindow(cmd)
	if app.stdin {
		cmd.Stdin = strings.NewReader(prompt)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limited{&stdout}, &limited{&stderr}
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return finish(STATE_SLOW, fmt.Errorf("no answer in %s", limit))
	}
	if ctx.Err() != nil {
		return finish(STATE_FAILED, ctx.Err())
	}
	answer, parseErr := app.parse(stdout.Bytes(), answerFile)
	if parseErr == nil && json.Valid(answer) {
		ans.JSON = answer
		return finish(STATE_OK, nil)
	}
	detail := tail(strings.TrimSpace(stderr.String() + "\n" + stdout.String()))
	if parseErr != nil && runErr == nil {
		detail = parseErr.Error()
	}
	return finish(classify(detail), errors.New(cmp.Or(detail, fmt.Sprint(runErr))))
}

// AskAll asks every app at once and sends each answer to onAnswer as it lands, from one goroutine at a time.
func (a *Asker) AskAll(ctx context.Context, q Question, onAnswer func(Answer)) []Answer {
	results := a.launch(ctx, q)
	out := make([]Answer, 0, len(a.Apps))
	for range a.Apps {
		ans := <-results
		out = append(out, ans)
		if onAnswer != nil {
			onAnswer(ans)
		}
	}
	return out
}

// Quorum asks every app at once and returns as soon as `need` answers agree, without waiting for the
// slowest. agree says whether two answers say the same thing. Answers still running when the quorum
// is reached keep going and reach onLate, so every app's record stays complete.
func (a *Asker) Quorum(ctx context.Context, q Question, need int, agree func(x, y json.RawMessage) bool, onLate func(Answer)) (agreed []Answer, all []Answer) {
	results := a.launch(ctx, q)
	for i := range a.Apps {
		ans := <-results
		all = append(all, ans)
		if group := agreeing(all, need, agree); group != nil {
			if onLate != nil {
				go func(left int) {
					for range left {
						onLate(<-results)
					}
				}(len(a.Apps) - i - 1)
			}
			return group, all
		}
	}
	return nil, all
}

// launch starts every app on the question and returns where their answers land.
func (a *Asker) launch(ctx context.Context, q Question) chan Answer {
	run := a.Run
	if run == nil {
		run = Run
	}
	results := make(chan Answer, len(a.Apps))
	for _, app := range a.Apps {
		go func() { results <- run(ctx, app, q, a.WorkRoot) }()
	}
	return results
}

// agreeing finds `need` ok answers that each agree with the same answer, or nil.
func agreeing(answers []Answer, need int, agree func(x, y json.RawMessage) bool) []Answer {
	for _, seed := range answers {
		if seed.State != STATE_OK {
			continue
		}
		group := []Answer{seed}
		for _, other := range answers {
			if other.App != seed.App && other.State == STATE_OK && agree(seed.JSON, other.JSON) {
				group = append(group, other)
			}
		}
		if len(group) >= need {
			return group[:need]
		}
	}
	return nil
}

func classify(detail string) string {
	low := strings.ToLower(detail)
	for _, h := range LIMIT_HINTS {
		if strings.Contains(low, h) {
			return STATE_LIMIT
		}
	}
	for _, h := range SIGN_IN_HINTS {
		if strings.Contains(low, h) {
			return STATE_SIGN_IN
		}
	}
	return STATE_FAILED
}

func tail(s string) string {
	if len(s) <= ERR_TAIL_CHARS {
		return s
	}
	return "…" + s[len(s)-ERR_TAIL_CHARS:]
}

// limited stops collecting output past MAX_OUTPUT, so a runaway app can't fill memory.
type limited struct{ buf *bytes.Buffer }

func (l *limited) Write(p []byte) (int, error) {
	if room := MAX_OUTPUT - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
