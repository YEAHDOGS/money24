// Package aicli asks the AI apps the user already pays for, through their own command-line tools,
// signed in with the user's subscriptions or their free tiers. Money 24 pays for no AI.
//
// Each app gets the question with no tools, no web, no saved session and an empty working folder,
// and must answer in a fixed JSON shape. Answers are untrusted input: nothing here places a trade.
// The apps run on the machine that runs the desk, as the signed-in Windows user, because that is
// where they are signed in.
package aicli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	APP_CLAUDE   = "claude"
	APP_CODEX    = "codex"
	APP_GEMINI   = "gemini"
	APP_OPENCODE = "opencode"

	// Fast models at low effort: news decays in seconds. Each can be changed in .env without a rebuild.
	CLAUDE_MODEL   = "haiku"
	CLAUDE_EFFORT  = "low"
	CODEX_EFFORT   = "low"
	GEMINI_MODEL   = "gemini-3.8-flash-low"
	OPENCODE_MODEL = "opencode/deepseek-v4.1-flash-free"

	ENV_CLAUDE_MODEL   = "M24D_AI_CLAUDE_MODEL"
	ENV_CODEX_MODEL    = "M24D_AI_CODEX_MODEL"
	ENV_GEMINI_MODEL   = "M24D_AI_GEMINI_MODEL"
	ENV_OPENCODE_MODEL = "M24D_AI_OPENCODE_MODEL"
	ENV_APPS           = "M24D_AI_APPS" // comma-separated app ids to use; empty uses every one that is installed

	SCHEMA_FILE   = "schema.json"
	ANSWER_FILE   = "answer.json"
	EXE           = ".exe"
	STDIN_PROMPT  = "-"
	PRINT_TIMEOUT = "90s"
	SCHEMA_PROMPT = "\n\nAnswer with one JSON object and nothing else, matching this JSON Schema:\n"
)

// SIGN_IN_HINTS are what each app prints when it is not signed in or is out of usage, lower case.
var SIGN_IN_HINTS = []string{"/login", "not logged in", "log in", "login required", "unauthorized", "sign in", "authenticate"}
var LIMIT_HINTS = []string{"usage limit", "rate limit", "quota", "limit reached", "too many requests"}

// jsonObject finds the last top-level JSON object in free text, for apps that print decoration around it.
var jsonObject = regexp.MustCompile(`(?s)\{.*\}`)
var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// App is one AI command-line tool and how to ask it a single question.
type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`    // what the user knows it as
	Via    string `json:"via"`     // the tool that carries the question
	Plan   string `json:"plan"`    // whose plan pays: the user's subscription, or a free tier
	SignIn string `json:"sign_in"` // the one-time step on the desk machine
	Model  string `json:"model"`
	bins   []string
	stdin  bool // the prompt goes on stdin, so long articles never hit the command-line limit
	// The tool cannot enforce a JSON Schema, so the schema is written into the prompt instead.
	schemaInPrompt bool
	args           func(model, schemaJSON, schemaFile, answerFile, prompt string) []string
	parse          func(stdout []byte, answerFile string) (json.RawMessage, error)
	envName        string
}

// Apps lists every app in the order they are usually fastest, with models from the environment.
func Apps() []App {
	home, _ := os.UserHomeDir()
	appData := os.Getenv("APPDATA")
	localAppData := os.Getenv("LOCALAPPDATA")
	apps := []App{
		{ID: APP_CLAUDE, Name: "Claude", Via: "Claude Code", Plan: "your Claude plan", SignIn: "run `claude` once and sign in",
			Model: CLAUDE_MODEL, envName: ENV_CLAUDE_MODEL, stdin: true,
			bins: []string{"claude", filepath.Join(home, ".local", "bin", "claude"+EXE)},
			args: func(model, schemaJSON, _, _, _ string) []string {
				return []string{"-p", "--output-format", "json", "--json-schema", schemaJSON, "--tools", "",
					"--no-session-persistence", "--strict-mcp-config", "--setting-sources", "", "--effort", CLAUDE_EFFORT, "--model", model}
			},
			parse: parseClaude},
		{ID: APP_CODEX, Name: "ChatGPT", Via: "Codex", Plan: "your ChatGPT plan", SignIn: "run `codex login` and choose Sign in with ChatGPT",
			envName: ENV_CODEX_MODEL, stdin: true,
			bins: []string{"codex", filepath.Join(localAppData, "Programs", "OpenAI", "Codex", "bin", "codex"+EXE)},
			args: func(model, _, schemaFile, answerFile, _ string) []string {
				a := []string{"exec", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "-s", "read-only",
					"-c", `web_search="disabled"`, "-c", `model_reasoning_effort="` + CODEX_EFFORT + `"`,
					"--output-schema", schemaFile, "-o", answerFile}
				if model != "" {
					a = append(a, "-m", model)
				}
				return append(a, STDIN_PROMPT)
			},
			parse: parseAnswerFile},
		{ID: APP_GEMINI, Name: "Gemini", Via: "Antigravity", Plan: "your Google account", SignIn: "open Antigravity once and sign in with Google",
			Model: GEMINI_MODEL, envName: ENV_GEMINI_MODEL,
			bins: []string{"agy", filepath.Join(localAppData, "agy", "bin", "agy"+EXE)},
			args: func(model, schemaJSON, _, _, prompt string) []string {
				return []string{"-p", prompt, "--output-format", "json", "--json-schema", schemaJSON, "--model", model,
					"--disable-slash-commands", "--sandbox", "--print-timeout", PRINT_TIMEOUT}
			},
			parse: parseAntigravity},
		{ID: APP_OPENCODE, Name: "OpenCode", Via: "OpenCode", Plan: "OpenCode's free models", SignIn: "nothing: its free models need no account",
			Model: OPENCODE_MODEL, envName: ENV_OPENCODE_MODEL, schemaInPrompt: true,
			bins: []string{filepath.Join(appData, "npm", "node_modules", "opencode-ai", "bin", "opencode"+EXE), "opencode"},
			args: func(model, _, _, _, prompt string) []string {
				return []string{"run", "--pure", "--agent", "plan", "-m", model, prompt}
			},
			parse: parseLastObject},
	}
	for i := range apps {
		apps[i].Model = cmp.Or(os.Getenv(apps[i].envName), apps[i].Model)
	}
	return apps
}

// Selected keeps the apps named in M24D_AI_APPS, or all of them when it is empty.
func Selected(apps []App) []App {
	want := strings.TrimSpace(os.Getenv(ENV_APPS))
	if want == "" {
		return apps
	}
	keep := map[string]bool{}
	for _, id := range strings.Split(want, ",") {
		keep[strings.TrimSpace(id)] = true
	}
	out := []App{}
	for _, a := range apps {
		if keep[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

// Path finds the app's executable: a real .exe first, since npm's .cmd shims mangle arguments.
func (a App) Path() (string, error) {
	for _, bin := range a.bins {
		if filepath.IsAbs(bin) {
			if info, err := os.Stat(bin); err == nil && !info.IsDir() {
				return bin, nil
			}
			continue
		}
		path, err := exec.LookPath(bin)
		if err != nil || strings.EqualFold(filepath.Ext(path), ".cmd") || strings.EqualFold(filepath.Ext(path), ".bat") {
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("%s is not installed on this machine", a.Via)
}

// claudeResult is the part of Claude Code's --output-format json that matters here.
type claudeResult struct {
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func parseClaude(stdout []byte, _ string) (json.RawMessage, error) {
	var r claudeResult
	if err := json.Unmarshal(stdout, &r); err != nil {
		return nil, fmt.Errorf("unreadable answer: %w", err)
	}
	if r.IsError {
		return nil, errors.New(firstLine(r.Result))
	}
	if len(r.StructuredOutput) == 0 || string(r.StructuredOutput) == "null" {
		return parseLastObject([]byte(r.Result), "")
	}
	return r.StructuredOutput, nil
}

func parseAnswerFile(_ []byte, answerFile string) (json.RawMessage, error) {
	b, err := os.ReadFile(answerFile)
	if err != nil {
		return nil, errors.New("no answer written")
	}
	return parseLastObject(b, "")
}

// antigravityResult is the part of agy's --output-format json that matters here.
type antigravityResult struct {
	Status           string          `json:"status"`
	Response         string          `json:"response"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func parseAntigravity(stdout []byte, _ string) (json.RawMessage, error) {
	var r antigravityResult
	obj, err := parseLastObject(stdout, "")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(obj, &r); err != nil {
		return nil, fmt.Errorf("unreadable answer: %w", err)
	}
	if len(r.StructuredOutput) > 0 && string(r.StructuredOutput) != "null" {
		return r.StructuredOutput, nil
	}
	if r.Response == "" {
		return nil, fmt.Errorf("no answer (%s)", strings.ToLower(r.Status))
	}
	return parseLastObject([]byte(r.Response), "")
}

// parseLastObject takes the last top-level JSON object in the text, after any decoration the app printed.
func parseLastObject(stdout []byte, _ string) (json.RawMessage, error) {
	text := string(ansi.ReplaceAll(stdout, nil))
	var last json.RawMessage
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		var obj json.RawMessage
		if dec.Decode(&obj) != nil {
			continue
		}
		last = obj
		i += int(dec.InputOffset()) - 1
	}
	if last == nil {
		return nil, errors.New("no JSON in the answer")
	}
	return last, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
