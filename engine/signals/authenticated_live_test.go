package signals

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	LIVE_MT_CHANNEL_ENV      = "M24D_LIVE_TELEGRAM_CHANNEL"
	LIVE_MT_DATA_ENV         = "M24D_LIVE_TELEGRAM_DATA"
	LIVE_DISCORD_CHANNEL_ENV = "M24D_LIVE_DISCORD_CHANNEL"
	LIVE_AUTHENTICATED_POSTS = 5
)

func TestLiveTelegramUserReader(t *testing.T) {
	data, handle := os.Getenv(LIVE_MT_DATA_ENV), os.Getenv(LIVE_MT_CHANNEL_ENV)
	if os.Getenv(LIVE_READER_ENV) != LIVE_READER_ENABLED || data == "" || handle == "" {
		t.Skip("requires M24D_LIVE=1, M24D_LIVE_TELEGRAM_CHANNEL and M24D_LIVE_TELEGRAM_DATA")
	}
	readers := NewSignalReaders(http.DefaultClient, data)
	if readers.telegram == nil {
		t.Skip("requires Telegram API credentials and a saved session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), LIVE_READER_TIMEOUT)
	done := make(chan struct{})
	go func() {
		defer close(done)
		readers.telegram.run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	source := Source{Type: TYPE_TELEGRAM, Handle: handle}
	posts, _, err := readers.telegram.fetch(ctx, source, 0, LIVE_AUTHENTICATED_POSTS, time.Time{}, PUBLIC_LIVE_PAGES)
	if err != nil || len(posts) == 0 {
		t.Fatalf("Telegram returned %d posts: %v", len(posts), err)
	}
}

func TestLiveDiscordBotReader(t *testing.T) {
	handle := os.Getenv(LIVE_DISCORD_CHANNEL_ENV)
	if os.Getenv(LIVE_READER_ENV) != LIVE_READER_ENABLED || handle == "" || os.Getenv(DISCORD_TOKEN_ENV) == "" {
		t.Skip("requires M24D_LIVE=1, M24D_LIVE_DISCORD_CHANNEL and DISCORD_BOT_TOKEN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), LIVE_READER_TIMEOUT)
	defer cancel()
	source := Source{Type: TYPE_DISCORD, Handle: handle}
	posts, _, err := newDiscordReader(http.DefaultClient).fetch(ctx, source, 0, 0, LIVE_AUTHENTICATED_POSTS, time.Time{}, PUBLIC_LIVE_PAGES)
	if err != nil || len(posts) == 0 {
		t.Fatalf("Discord returned %d posts: %v", len(posts), err)
	}
}
