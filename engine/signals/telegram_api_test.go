package signals

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	TEST_MT_HANDLE         = "FixtureCoin"
	TEST_MT_CHANNEL        = 123
	TEST_MT_SINCE          = 1749481200
	TEST_MT_MESSAGE        = "Long BTC"
	TEST_MT_RPC_FLOOD      = 420
	TEST_MT_FLOOD_RESPONSE = "FLOOD_WAIT_0"
	TEST_MT_FLOOD_TIMEOUT  = 20 * time.Millisecond
)

type telegramFixture struct {
	resolves int
	pages    []tg.MessagesGetHistoryRequest
}

func (f *telegramFixture) ContactsResolveUsername(_ context.Context, request *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	f.resolves++
	if request.Username != "fixturecoin" {
		return nil, errors.New("unexpected fixture username")
	}
	return &tg.ContactsResolvedPeer{
		Peer:  &tg.PeerChannel{ChannelID: TEST_MT_CHANNEL},
		Chats: []tg.ChatClass{&tg.Channel{ID: TEST_MT_CHANNEL, AccessHash: 1}},
	}, nil
}

func (f *telegramFixture) MessagesGetHistory(_ context.Context, request *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	f.pages = append(f.pages, *request)
	switch {
	case request.MinID == 9:
		return &tg.MessagesChannelMessages{Messages: []tg.MessageClass{
			&tg.Message{ID: 10, Date: TEST_MT_SINCE + 10, Message: TEST_MT_MESSAGE},
		}}, nil
	case request.OffsetID == 0:
		return &tg.MessagesChannelMessages{Messages: []tg.MessageClass{
			&tg.Message{ID: 9, Date: TEST_MT_SINCE + 9, Message: TEST_MT_MESSAGE, PostAuthor: "Fixture desk"},
			&tg.Message{ID: 8, Date: TEST_MT_SINCE + 8, Message: "Pair updated"},
			&tg.MessageEmpty{ID: 7},
			&tg.Message{ID: 9, Date: TEST_MT_SINCE + 9, Message: TEST_MT_MESSAGE},
		}}, nil
	case request.OffsetID == 7:
		return &tg.MessagesChannelMessages{Messages: []tg.MessageClass{
			&tg.Message{ID: 8, Date: TEST_MT_SINCE + 8, Message: "Pair updated"},
			&tg.Message{ID: 6, Date: TEST_MT_SINCE + 6, Message: "Previous call"},
			&tg.Message{ID: 5, Date: TEST_MT_SINCE - 5, Message: "Outside the requested window"},
		}}, nil
	default:
		return &tg.MessagesChannelMessages{}, nil
	}
}

func TestTelegramAPIPagingAndCachedUsername(t *testing.T) {
	api := &telegramFixture{}
	reader := &telegramReader{resolving: make(chan struct{}, 1), peers: make(map[string]*tg.InputPeerChannel)}
	source := Source{Type: TYPE_TELEGRAM, Handle: TEST_MT_HANDLE}
	ctx := context.Background()
	posts, high, err := reader.fetchAPI(ctx, api, source, 0, 10, time.Unix(TEST_MT_SINCE, 0), 3)
	if err != nil || len(posts) != 3 || high != 9 || len(api.pages) != 2 ||
		api.pages[1].OffsetID != 7 || posts[2].PostID != "fixturecoin/6" {
		t.Fatalf("unexpected Telegram history: %+v, cursor %d, requests %d, error %v", posts, high, len(api.pages), err)
	}
	if posts[0].SourceID != source.ID() || posts[0].Text != TEST_MT_MESSAGE || posts[0].Author != "Fixture desk" ||
		posts[0].URL != "https://t.me/fixturecoin/9" || posts[0].PostedAt.Unix() != TEST_MT_SINCE+9 {
		t.Fatalf("unexpected Telegram post: %+v", posts[0])
	}
	live, high, err := reader.fetchAPI(ctx, api, source, high, 1, time.Time{}, 1)
	if err != nil || len(live) != 1 || high != 10 || live[0].PostID != "fixturecoin/10" ||
		api.resolves != 1 || api.pages[2].MinID != 9 {
		t.Fatalf("unexpected Telegram live poll: %+v, cursor %d, resolutions %d, error %v", live, high, api.resolves, err)
	}
}

func TestTelegramFloodWaitRetriesAndHonorsCancellation(t *testing.T) {
	gate := &telegramFloodGate{busy: make(chan struct{}, 1)}
	calls := 0
	invoke := gate.middleware(telegram.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
		calls++
		if calls == 1 {
			return tgerr.New(TEST_MT_RPC_FLOOD, TEST_MT_FLOOD_RESPONSE)
		}
		return nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), TEST_MT_FLOOD_TIMEOUT)
	defer cancel()
	if err := invoke(ctx, nil, nil); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("Telegram retried before the cooldown: %d calls, %v", calls, err)
	}
	retryAt := gate.until
	ctx, cancelRetry := context.WithTimeout(context.Background(), LIVE_READER_TIMEOUT)
	defer cancelRetry()
	if err := invoke(ctx, nil, nil); err != nil || calls != 2 || time.Now().Before(retryAt) {
		t.Fatalf("Telegram did not retry after the cooldown: %d calls, %v", calls, err)
	}
}

func TestTelegramHistoryAndLiveShareRecorderDedupe(t *testing.T) {
	rec, err := OpenRecorder(filepath.Join(t.TempDir(), TEST_JSON_POSTS))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	now := time.Now().UTC()
	started := now.Add(-time.Minute)
	source := Source{Type: TYPE_TELEGRAM, Handle: TEST_MT_HANDLE}
	archived := RawPost{SourceID: source.ID(), PostID: "FixtureCoin/1", PostedAt: started.Add(-time.Second), Text: TEST_MT_MESSAGE}
	if added, err := recordHistoryPosts(context.Background(), rec, []RawPost{archived}); err != nil || added != 1 {
		t.Fatalf("history recorded %d posts, %v", added, err)
	}
	archived.PostID = "fixturecoin/1"
	// The old journal's spelling and a live poll's normalized spelling are the same post.
	if !rec.Has(source.ID(), archived.PostID) {
		t.Fatal("public preview post was not found by its MTProto ID")
	}
	boundary := RawPost{SourceID: source.ID(), PostID: "fixturecoin/2", PostedAt: started, Text: TEST_MT_MESSAGE}
	fresh := RawPost{SourceID: source.ID(), PostID: "fixturecoin/3", PostedAt: now, Text: TEST_MT_MESSAGE}
	out := make(chan Signal, 3)
	for range 2 {
		if err := recordLivePosts(context.Background(), &Catalog{}, rec, out, source, started, []RawPost{fresh, boundary, archived}); err != nil {
			t.Fatal(err)
		}
	}
	if len(out) != 1 || (<-out).PostID != fresh.PostID || len(rec.Signals()[source.ID()]) != 3 {
		t.Fatal("live polling did not record history separately or queue exactly one fresh post")
	}
}

func TestTelegramSessionReplacesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), TELEGRAM_SESSION)
	storage := telegramSession{path: path}
	ctx := context.Background()
	for _, text := range []string{"fixture", "replacement"} {
		if err := storage.StoreSession(ctx, []byte(text)); err != nil {
			t.Fatal(err)
		}
		contents, err := storage.LoadSession(ctx)
		if err != nil || string(contents) != text {
			t.Fatal("session storage did not persist the new session file")
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("session file is missing")
	}
}
