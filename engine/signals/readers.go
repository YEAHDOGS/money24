package signals

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gotd/td/tg"
)

// SignalReaders shares authenticated reader state between history and live polling.
type SignalReaders struct {
	public   *http.Client
	telegram *telegramReader
	discord  *discordReader
	failures followFailures // each source whose last read failed, for GET /api/follow
}

func NewSignalReaders(client *http.Client, data string) *SignalReaders {
	readers := &SignalReaders{public: client, discord: newDiscordReader(client)}
	config := telegramConfigFromEnv(data)
	if config.id <= 0 || config.hash == "" {
		return readers
	}
	if info, err := os.Stat(config.path); err == nil && info.Mode().IsRegular() {
		readers.telegram = &telegramReader{config: config, ready: make(chan struct{}),
			resolving: make(chan struct{}, 1), peers: make(map[string]*tg.InputPeerChannel)}
	}
	return readers
}

func (r *SignalReaders) history(ctx context.Context, source Source) ([]RawPost, error) {
	since := time.Now().Add(-PUBLIC_HISTORY_DAYS)
	switch {
	case source.Type == TYPE_TELEGRAM && r.telegram != nil:
		posts, _, err := r.telegram.fetch(ctx, source, 0, PUBLIC_HISTORY_POSTS, since, PUBLIC_HISTORY_PAGES)
		return posts, err
	case source.Type == TYPE_DISCORD:
		posts, _, err := r.discord.fetch(ctx, source, 0, 0, PUBLIC_HISTORY_POSTS, since, PUBLIC_HISTORY_PAGES)
		return posts, err
	default:
		return fetchPublicPosts(ctx, r.public, source, PUBLIC_HISTORY_POSTS, since, PUBLIC_HISTORY_PAGES)
	}
}

// Follow records each reader's snapshot and polls until ctx ends. History loads
// use a separate entry point and cannot enqueue signals.
func (r *SignalReaders) Follow(ctx context.Context, catalog *Catalog, rec *Recorder, out chan<- Signal, started time.Time, followPublic bool) {
	var workers sync.WaitGroup
	if r.telegram != nil {
		workers.Go(func() { r.telegram.run(ctx) })
	}
	for _, source := range catalog.Sources {
		switch {
		case source.Type == TYPE_DISCORD, source.Type == TYPE_TELEGRAM && r.telegram != nil:
		case followPublic && (source.Type == TYPE_TELEGRAM || source.Type == TYPE_REDDIT):
		default:
			continue
		}
		workers.Go(func() { r.followSource(ctx, catalog, rec, out, source, started) })
	}
	workers.Wait()
}

func (r *SignalReaders) followSource(ctx context.Context, catalog *Catalog, rec *Recorder, out chan<- Signal, source Source, started time.Time) {
	interval := PUBLIC_POLL_INTERVAL
	if source.Type == TYPE_DISCORD {
		interval = DISCORD_INTERVAL
	}
	var telegramID int
	discordID := discordCursor(started)
	initialDiscord := source.Type == TYPE_DISCORD
	for ctx.Err() == nil {
		var posts []RawPost
		var err error
		nextTelegramID, nextDiscordID := telegramID, discordID
		switch {
		case initialDiscord:
			posts, _, err = r.discord.fetch(ctx, source, discordID, 0, DISCORD_PAGE_SIZE, time.Time{}, PUBLIC_LIVE_PAGES)
			// Use the startup snowflake to start live even if the snapshot fails.
			initialDiscord = false
			if err == nil {
				_, err = recordHistoryPosts(ctx, rec, posts)
			}
			posts = nil
		case source.Type == TYPE_DISCORD:
			posts, nextDiscordID, err = r.discord.fetch(ctx, source, 0, discordID, PUBLIC_HISTORY_POSTS, time.Time{}, PUBLIC_HISTORY_PAGES)
		case source.Type == TYPE_TELEGRAM && r.telegram != nil:
			posts, nextTelegramID, err = r.telegram.fetch(ctx, source, telegramID, PUBLIC_HISTORY_POSTS, time.Time{}, PUBLIC_HISTORY_PAGES)
		default:
			err = pollPublicSource(ctx, r.public, catalog, rec, out, source, started)
		}
		if err == nil {
			err = recordLivePosts(ctx, catalog, rec, out, source, started, posts)
		}
		if err == nil || errors.Is(err, errDiscordEmpty) {
			telegramID, discordID = nextTelegramID, nextDiscordID
		}
		if err != nil && ctx.Err() == nil {
			log.Printf(PUBLIC_FOLLOW_ERROR, source.ID(), err)
		}
		if ctx.Err() == nil && !errors.Is(err, errDiscordEmpty) {
			r.failures.note(source.ID(), err, time.Now())
		}
		if waitForReader(ctx, interval) != nil {
			return
		}
	}
}

func recordHistoryPosts(ctx context.Context, rec *Recorder, posts []RawPost) (int, error) {
	added, now := 0, time.Now().UTC()
	for _, post := range posts {
		if err := ctx.Err(); err != nil {
			return added, err
		}
		post.SeenAt = now
		fresh, err := rec.recordNew(post)
		if err != nil {
			return added, err
		}
		if fresh {
			added++
		}
	}
	return added, nil
}

func waitForReader(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
