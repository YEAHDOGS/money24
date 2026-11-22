package signals

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	PUBLIC_POLL_INTERVAL = time.Minute
	PUBLIC_LIVE_MAX_AGE  = 2 * PUBLIC_POLL_INTERVAL
	PUBLIC_LIVE_PAGES    = 1
	PUBLIC_FOLLOW_ERROR  = "signal reader %s: %v"
	PUBLIC_QUEUE_FULL    = "signal reader %s: queue full, post %s recorded"
)

// FollowPublic polls the configured Telegram and Reddit sources until ctx ends. The initial
// snapshot records existing posts, and only posts published after startup can reach the queue.
// Discord is the next adapter; it needs a bot token for a server we run.
func FollowPublic(ctx context.Context, client *http.Client, catalog *Catalog, rec *Recorder, out chan<- Signal) {
	var sources []Source
	for _, source := range catalog.Sources {
		if source.Type == TYPE_TELEGRAM || source.Type == TYPE_REDDIT {
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return
	}
	started := time.Now()
	for ctx.Err() == nil {
		for _, source := range sources {
			if ctx.Err() != nil {
				return
			}
			if err := pollPublicSource(ctx, client, catalog, rec, out, source, started); err != nil && ctx.Err() == nil {
				log.Printf(PUBLIC_FOLLOW_ERROR, source.ID(), err)
			}
		}
		// Wait after the entire pass, so even a slow request cannot compress two polls.
		timer := time.NewTimer(PUBLIC_POLL_INTERVAL)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func pollPublicSource(ctx context.Context, client *http.Client, catalog *Catalog, rec *Recorder, out chan<- Signal, source Source, started time.Time) error {
	posts, err := fetchPublicPosts(ctx, client, source, PUBLIC_HISTORY_POSTS, time.Time{}, PUBLIC_LIVE_PAGES)
	if err != nil {
		return err
	}
	seen := time.Now().UTC()
	// Readers return newest first. Apply older callouts before later exits or adds.
	for i := len(posts) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := posts[i]
		p.SeenAt = seen
		newPost, err := rec.recordNew(p)
		if err != nil {
			return err
		}
		if !newPost || p.PostedAt.Before(started) || !freshPublicPost(p.PostedAt, catalog.SettingsFor(p.SourceID), time.Now()) {
			continue
		}
		sig := p.parsed()
		select {
		case out <- sig:
		default:
			log.Printf(PUBLIC_QUEUE_FULL, source.ID(), p.PostID)
		}
	}
	return nil
}

func freshPublicPost(posted time.Time, settings Settings, now time.Time) bool {
	maxAge := PUBLIC_LIVE_MAX_AGE
	if settings.MaxSignalAgeSec > 0 {
		maxAge = time.Duration(settings.MaxSignalAgeSec) * time.Second
	}
	return !posted.IsZero() && !posted.After(now) && now.Sub(posted) <= maxAge
}

// stalePublicSignal also checks age on the runner side, where a queue backlog can age any call.
func stalePublicSignal(sig Signal, settings Settings, now time.Time) bool {
	if sig.Manual || sig.PostedAt.IsZero() {
		return false
	}
	kind, _, _ := strings.Cut(sig.SourceID, ":")
	if kind != TYPE_TELEGRAM && kind != TYPE_REDDIT {
		return false
	}
	return !freshPublicPost(sig.PostedAt, settings, now)
}
