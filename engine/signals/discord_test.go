package signals

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

const (
	TEST_DISCORD_FIXTURE    = "discord_messages.json"
	TEST_DISCORD_TOKEN_ENV  = "M24D_FIXTURE_BOT"
	TEST_DISCORD_AUTH       = "Bot fixture"
	TEST_DISCORD_CHANNEL    = "123"
	TEST_DISCORD_EMBED      = "Signal desk\nLong BTC\nEntry 101\nStop: 99\nPaper"
	TEST_DISCORD_LINK       = "https://discord.com/channels/321/123/105"
	TEST_DISCORD_DELAY      = 20 * time.Millisecond
	TEST_DISCORD_BODY_DELAY = 40 * time.Millisecond
)

func TestDiscordEmbedsAndLiveCursor(t *testing.T) {
	t.Setenv(TEST_DISCORD_TOKEN_ENV, "fixture")
	body := readerFixture(t, TEST_DISCORD_FIXTURE)
	source := Source{Type: TYPE_DISCORD, Handle: TEST_DISCORD_CHANNEL, TokenEnv: TEST_DISCORD_TOKEN_ENV}
	var queries []string
	channelLookups := 0
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get(DISCORD_AUTH) != TEST_DISCORD_AUTH {
			t.Fatal("request did not use the configured bot")
		}
		switch req.URL.Path {
		case "/api/v10/channels/123":
			channelLookups++
			return readerResponse(req, `{"guild_id":"321"}`, http.StatusOK), nil
		case "/api/v10/channels/123/messages":
			queries = append(queries, req.URL.RawQuery)
			return readerResponse(req, body, http.StatusOK), nil
		default:
			t.Fatalf("unexpected Discord path %s", req.URL.Path)
			return nil, nil
		}
	})}
	reader := newDiscordReader(client)
	reader.interval = 0
	ctx := context.Background()
	history, _, err := reader.fetch(ctx, source, 0, 0, 3, time.Time{}, 2)
	if err != nil || len(history) != 2 || history[0].Text != TEST_DISCORD_EMBED ||
		history[0].URL != TEST_DISCORD_LINK || history[0].Author != "fixture-bot" || history[0].PostedAt.Second() != 5 {
		t.Fatalf("unexpected Discord history: %+v, %v", history, err)
	}
	queries = nil
	live, cursor, err := reader.fetch(ctx, source, 0, 103, 3, time.Time{}, 2)
	if err != nil || len(live) != 2 || cursor != 105 || len(queries) != 1 ||
		queries[0] != "after=103&limit=3" || channelLookups != 1 {
		t.Fatalf("unexpected Discord live page: %d posts, cursor %d, queries %v, error %v", len(live), cursor, queries, err)
	}
}

func TestDiscordHistoryPaginatesPastRepeatedIDs(t *testing.T) {
	t.Setenv(DISCORD_TOKEN_ENV, "fixture")
	var queries []string
	reader := newDiscordReader(&http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		queries = append(queries, req.URL.RawQuery)
		var body string
		switch req.URL.Query().Get(DISCORD_BEFORE) {
		case "":
			body = `[{"id":"105","guild_id":"321","timestamp":"2025-06-09T15:00:05Z","content":"latest"},{"id":"104","timestamp":"2025-06-09T15:00:04Z","content":""},{"id":"103","timestamp":"2025-06-09T15:00:03Z","content":"prior"}]`
		case "103":
			body = `[{"id":"103","timestamp":"2025-06-09T15:00:03Z","content":"prior"},{"id":"102","timestamp":"2025-06-09T15:00:02Z","content":"older"}]`
		default:
			t.Fatalf("unexpected Discord query %s", req.URL.RawQuery)
		}
		return readerResponse(req, body, http.StatusOK), nil
	})})
	reader.interval = 0
	source := Source{Type: TYPE_DISCORD, Handle: TEST_DISCORD_CHANNEL}
	posts, _, err := reader.fetch(context.Background(), source, 0, 0, 3, time.Time{}, 2)
	if err != nil || len(posts) != 3 || posts[2].PostID != "102" || len(queries) != 2 || queries[1] != "before=103&limit=1" {
		t.Fatalf("unexpected Discord history: %+v, queries %v, error %v", posts, queries, err)
	}
}

func TestDiscordRateHeadersAndGlobalRetry(t *testing.T) {
	t.Setenv(DISCORD_TOKEN_ENV, "fixture")
	var sent []time.Time
	reader := newDiscordReader(&http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		sent = append(sent, time.Now())
		if len(sent) == 1 {
			res := readerResponse(req, `{"retry_after":0.04,"global":true}`, http.StatusTooManyRequests)
			res.Header = http.Header{}
			res.Header.Set(PUBLIC_RETRY_AFTER_HEADER, "0.02")
			return res, nil
		}
		res := readerResponse(req, "[]", http.StatusOK)
		res.Header = http.Header{}
		res.Header.Set(DISCORD_REMAINING, "0")
		res.Header.Set(DISCORD_RESET_AFTER, "0.02")
		return res, nil
	})})
	reader.interval = time.Millisecond
	ctx := context.Background()
	source := Source{Type: TYPE_DISCORD, Handle: TEST_DISCORD_CHANNEL}
	if _, _, err := reader.page(ctx, source, DISCORD_AFTER, 100, 1); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[1].Sub(sent[0]) < TEST_DISCORD_BODY_DELAY ||
		reader.global["fixture"].Sub(sent[0]) < TEST_DISCORD_BODY_DELAY {
		t.Fatalf("Discord did not honor the global retry: %v", sent)
	}
	if _, _, err := reader.page(ctx, source, DISCORD_AFTER, 100, 1); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 3 || sent[2].Sub(sent[1]) < TEST_DISCORD_DELAY {
		t.Fatalf("Discord did not honor the bucket reset: %v", sent)
	}
}

func TestDiscordReportsEmptyContent(t *testing.T) {
	t.Setenv(DISCORD_TOKEN_ENV, "fixture")
	reader := newDiscordReader(&http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		return readerResponse(req, `[{"id":"105","guild_id":"321","timestamp":"2025-06-09T15:00:05Z","content":"","embeds":[]}]`, http.StatusOK), nil
	})})
	source := Source{Type: TYPE_DISCORD, Handle: TEST_DISCORD_CHANNEL}
	_, cursor, err := reader.fetch(context.Background(), source, 0, 104, 1, time.Time{}, 1)
	if !errors.Is(err, errDiscordEmpty) || cursor != 105 {
		t.Fatalf("expected Message Content Intent diagnostic, got cursor %d and %v", cursor, err)
	}
}
