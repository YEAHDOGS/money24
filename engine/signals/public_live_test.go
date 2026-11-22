package signals

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	LIVE_READER_ENV      = "M24D_LIVE"
	LIVE_READER_ENABLED  = "1"
	LIVE_READER_TIMEOUT  = 45 * time.Second
	LIVE_TELEGRAM_HANDLE = "cointelegraph"
	LIVE_TELEGRAM_LIMIT  = 25
	LIVE_REDDIT_HANDLE   = "r/Bitcoin"
	LIVE_REDDIT_LIMIT    = 5
)

func TestLiveTelegramPublicReader(t *testing.T) {
	if os.Getenv(LIVE_READER_ENV) != LIVE_READER_ENABLED {
		t.Skip("set M24D_LIVE=1 to check the public preview")
	}
	ctx, cancel := context.WithTimeout(context.Background(), LIVE_READER_TIMEOUT)
	defer cancel()
	source := Source{Type: TYPE_TELEGRAM, Handle: LIVE_TELEGRAM_HANDLE}
	posts, err := fetchTelegram(ctx, http.DefaultClient, source, LIVE_TELEGRAM_LIMIT, time.Now().Add(-PUBLIC_HISTORY_DAYS), PUBLIC_HISTORY_PAGES)
	if err != nil || len(posts) == 0 {
		t.Fatalf("%s: %d posts, %v", source.ID(), len(posts), err)
	}
	t.Logf("%s: %d posts, newest %s (%s), oldest %s (%s)", source.ID(), len(posts), posts[0].PostID,
		posts[0].PostedAt.Format(time.RFC3339), posts[len(posts)-1].PostID, posts[len(posts)-1].PostedAt.Format(time.RFC3339))
}

func TestLiveRedditPublicReader(t *testing.T) {
	if os.Getenv(LIVE_READER_ENV) != LIVE_READER_ENABLED {
		t.Skip("set M24D_LIVE=1 to check the public JSON listing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), LIVE_READER_TIMEOUT)
	defer cancel()
	source := Source{Type: TYPE_REDDIT, Handle: LIVE_REDDIT_HANDLE}
	posts, err := fetchReddit(ctx, http.DefaultClient, source, LIVE_REDDIT_LIMIT, time.Time{}, PUBLIC_LIVE_PAGES)
	if err != nil || len(posts) == 0 {
		t.Fatalf("%s: %d posts, %v", source.ID(), len(posts), err)
	}
	t.Logf("%s: %d posts, newest %s (%s)", source.ID(), len(posts), posts[0].PostID, posts[0].PostedAt.Format(time.RFC3339))
}
