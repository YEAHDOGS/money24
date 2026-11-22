package signals

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	TELEGRAM_API_ID_ENV       = "TELEGRAM_API_ID"
	TELEGRAM_API_HASH_ENV     = "TELEGRAM_API_HASH"
	TELEGRAM_PHONE_ENV        = "TELEGRAM_PHONE"
	TELEGRAM_SESSION          = "telegram.session"
	TELEGRAM_PAGE_SIZE        = 100
	TELEGRAM_API_UNAVAILABLE  = "Telegram user API is connecting or unavailable"
	TELEGRAM_API_DISCONNECTED = "Telegram user API disconnected, will retry"
	TELEGRAM_API_RELOGIN      = "Telegram session is no longer authorized, run m24d --telegram-login again"
	TELEGRAM_API_FAILURE      = "Telegram user API request failed"
	TELEGRAM_API_RPC          = "Telegram user API request failed (RPC %d)"
	TELEGRAM_API_NO_CHANNEL   = "Telegram username does not resolve to an accessible channel"
	TELEGRAM_API_BAD_HISTORY  = "Telegram returned an unexpected history response"
)

type telegramRPC interface {
	ContactsResolveUsername(context.Context, *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error)
	MessagesGetHistory(context.Context, *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error)
}

type telegramConfig struct {
	id   int
	hash string
	path string
}

func telegramConfigFromEnv(data string) telegramConfig {
	id, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(TELEGRAM_API_ID_ENV)))
	return telegramConfig{id: id, hash: strings.TrimSpace(os.Getenv(TELEGRAM_API_HASH_ENV)), path: filepath.Join(data, TELEGRAM_SESSION)}
}

func (c telegramConfig) client(storage session.Storage, limiter *telegramFloodGate) *telegram.Client {
	return telegram.NewClient(c.id, c.hash, telegram.Options{
		NoUpdates: true, SessionStorage: storage,
		Middlewares: []telegram.Middleware{telegram.MiddlewareFunc(limiter.middleware)},
	})
}

type telegramFloodGate struct {
	busy  chan struct{}
	until time.Time
}

func (g *telegramFloodGate) middleware(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case g.busy <- struct{}{}:
			defer func() { <-g.busy }()
		}
		for {
			if delay := time.Until(g.until); delay > 0 {
				if err := waitForReader(ctx, delay); err != nil {
					return err
				}
			}
			err := next.Invoke(ctx, input, output)
			delay, flooded := tgerr.AsFloodWait(err)
			if !flooded {
				return err
			}
			g.until = time.Now().Add(delay + time.Second)
		}
	}
}

type telegramReader struct {
	config    telegramConfig
	api       atomic.Pointer[tg.Client]
	ready     chan struct{}
	resolving chan struct{}
	peers     map[string]*tg.InputPeerChannel
}

func (r *telegramReader) run(ctx context.Context) {
	limiter := &telegramFloodGate{busy: make(chan struct{}, 1)}
	ready := r.ready
	for ctx.Err() == nil {
		client := r.config.client(telegramSession{path: r.config.path}, limiter)
		err := client.Run(ctx, func(connected context.Context) error {
			r.api.Store(client.API())
			defer r.api.Store(nil)
			if ready != nil {
				close(ready)
				ready = nil
			}
			<-connected.Done()
			return connected.Err()
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Print(TELEGRAM_API_DISCONNECTED)
		}
		if waitForReader(ctx, PUBLIC_POLL_INTERVAL) != nil {
			return
		}
	}
}

func (r *telegramReader) fetch(ctx context.Context, source Source, cursor, limit int, since time.Time, pages int) ([]RawPost, int, error) {
	readyCtx, cancel := context.WithTimeout(ctx, PUBLIC_REQUEST_TIMEOUT)
	defer cancel()
	select {
	case <-readyCtx.Done():
		return nil, cursor, errors.New(TELEGRAM_API_UNAVAILABLE)
	case <-r.ready:
	}
	api := r.api.Load()
	if api == nil {
		return nil, cursor, errors.New(TELEGRAM_API_UNAVAILABLE)
	}
	return r.fetchAPI(ctx, api, source, cursor, limit, since, pages)
}

func (r *telegramReader) resolve(ctx context.Context, api telegramRPC, handle string) (*tg.InputPeerChannel, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r.resolving <- struct{}{}:
		defer func() { <-r.resolving }()
	}
	if peer := r.peers[handle]; peer != nil {
		return peer, nil
	}
	result, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: handle})
	if err != nil {
		return nil, telegramAPIError(err)
	}
	channelPeer, ok := result.Peer.(*tg.PeerChannel)
	if !ok {
		return nil, errors.New(TELEGRAM_API_NO_CHANNEL)
	}
	for _, chat := range result.Chats {
		channel, ok := chat.(*tg.Channel)
		if !ok || channel.ID != channelPeer.ChannelID || channel.AccessHash == 0 {
			continue
		}
		peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
		r.peers[handle] = peer
		return peer, nil
	}
	return nil, errors.New(TELEGRAM_API_NO_CHANNEL)
}

func (r *telegramReader) fetchAPI(ctx context.Context, api telegramRPC, source Source, cursor, limit int, since time.Time, pages int) ([]RawPost, int, error) {
	handle := strings.ToLower(strings.TrimPrefix(source.Handle, TELEGRAM_AT))
	if !TELEGRAM_HANDLE.MatchString(handle) {
		return nil, cursor, errors.New(TELEGRAM_INVALID)
	}
	peer, err := r.resolve(ctx, api, handle)
	if err != nil {
		return nil, cursor, err
	}
	var posts []RawPost
	seen := make(map[int]bool)
	offset, newest := 0, cursor
	for page := 0; page < pages && len(posts) < limit; page++ {
		size := min(TELEGRAM_PAGE_SIZE, limit-len(posts))
		history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer: peer, OffsetID: offset, MinID: cursor, Limit: size,
		})
		if err != nil {
			return nil, cursor, telegramAPIError(err)
		}
		modified, ok := history.(tg.ModifiedMessagesMessages)
		if !ok {
			return nil, cursor, errors.New(TELEGRAM_API_BAD_HISTORY)
		}
		items := modified.GetMessages()
		oldest, reachedStart := 0, false
		for _, item := range items {
			id := item.GetID()
			if id <= cursor || (offset > 0 && id >= offset) {
				continue
			}
			newest = max(newest, id)
			if oldest == 0 || id < oldest {
				oldest = id
			}
			message, ok := item.(*tg.Message)
			if !ok || message.Date <= 0 {
				continue
			}
			at := time.Unix(int64(message.Date), 0).UTC()
			if !since.IsZero() && at.Before(since) {
				reachedStart = true
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			postID := handle + TELEGRAM_ID_SEPARATOR + strconv.Itoa(id)
			posts = append(posts, RawPost{SourceID: source.ID(), PostID: postID, PostedAt: at,
				Author: message.PostAuthor, URL: TELEGRAM_PERMALINK + postID, Text: message.Message})
		}
		if oldest == 0 || reachedStart {
			break
		}
		offset = oldest
	}
	slices.SortStableFunc(posts, func(a, b RawPost) int {
		return b.PostedAt.Compare(a.PostedAt)
	})
	return posts[:min(limit, len(posts))], newest, nil
}

func telegramAPIError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if rpc, ok := tgerr.As(err); ok {
		if rpc.Code == 401 {
			return errors.New(TELEGRAM_API_RELOGIN)
		}
		return fmt.Errorf(TELEGRAM_API_RPC, rpc.Code)
	}
	return errors.New(TELEGRAM_API_FAILURE)
}
