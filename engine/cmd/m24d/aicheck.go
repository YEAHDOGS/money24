package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/YEAHDOGS/money24/engine/aicli"
)

const (
	AI_CHECK_FLAG = "ai-check"
	// The README's example: a story the AIs must agree on before anything trades.
	AI_CHECK_NEWS = "The FAA ordered a full ground stop of all commercial flights in the US for four hours after a system outage at a contractor, L3Harris. At a press conference the FAA asked Congress for $50 billion in emergency funding."
	AI_ROW        = "  %-9s %-12s %-14s %6s  %s\n"
)

// runAICheck shows which AI apps on this machine answer and how fast, then puts the README's story to
// them all. Run it on the desk machine after signing each app in.
func runAICheck(dataDir string, out io.Writer) error {
	desk := aicli.NewDesk(dataDir)
	ctx := context.Background()
	fmt.Fprintln(out, "AI apps on this machine (a tiny question to each, all at once):")
	fmt.Fprintf(out, AI_ROW, "AI", "via", "state", "time", "next step")
	ready := 0
	for _, ans := range desk.Probe(ctx) {
		app := appByID(desk, ans.App)
		next := ""
		switch ans.State {
		case aicli.STATE_OK:
			ready++
		case aicli.STATE_MISSING:
			next = "install " + app.Via
		case aicli.STATE_SIGN_IN:
			next = app.SignIn
		default:
			next = firstLine(ans.Error)
		}
		fmt.Fprintf(out, AI_ROW, app.Name, app.Via, ans.State, fmt.Sprintf("%.1fs", float64(ans.Millis)/1000), next)
	}
	if ready < aicli.IMPACT_NEED {
		return fmt.Errorf("%d AI apps answered; at least %d must, so they can check each other", ready, aicli.IMPACT_NEED)
	}
	fmt.Fprintf(out, "\nThe README's FAA story, decided when %d agree:\n", aicli.IMPACT_NEED)
	run := desk.Impact(ctx, AI_CHECK_NEWS)
	for _, ans := range run.Answers {
		fmt.Fprintf(out, "  %-9s %5.1fs  %s %s\n", appByID(desk, ans.App).Name, float64(ans.Millis)/1000, ans.State, strings.TrimSpace(string(ans.JSON)))
	}
	if !run.Agreed {
		return fmt.Errorf("no %d AIs agreed after %.1fs", aicli.IMPACT_NEED, float64(run.DecidedMs)/1000)
	}
	fmt.Fprintf(out, "Agreed in %.1fs: %s %s (confidence %.2f)\n", float64(run.DecidedMs)/1000, run.Consensus.Direction, strings.Join(run.Consensus.Tickers, " "), run.Consensus.Confidence)
	return nil
}

func appByID(desk *aicli.Desk, id string) aicli.App {
	for _, app := range desk.Asker.Apps {
		if app.ID == id {
			return app
		}
	}
	return aicli.App{ID: id, Name: id}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
