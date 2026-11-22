package signals

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

const (
	CRYPTO_CALL_FIXTURES = "testdata/crypto_calls.json"
	CRYPTO_LIVE_ENV      = "M24D_LIVE"
	CRYPTO_FIXTURE_FEE   = 0.25
)

type cryptoCallFixture struct {
	Text   string
	Want   *CryptoCall
	Reject string
	LiveAt time.Time `json:"live_at"`
}

func readCryptoFixtures[T any](t *testing.T, path string) T {
	t.Helper()
	var data T
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseCryptoCallFixtures(t *testing.T) {
	seen := time.Date(2025, time.January, 2, 15, 8, 0, 0, time.UTC)
	post := RawPost{PostID: "fixture", SeenAt: seen, PostedAt: seen.Add(-time.Hour)}
	for i, fixture := range readCryptoFixtures[[]cryptoCallFixture](t, CRYPTO_CALL_FIXTURES) {
		post.Text = fixture.Text
		got, ok := ParseCryptoCall(post)
		if fixture.Want == nil {
			if ok {
				t.Errorf("fixture %d (%s): unexpectedly parsed %+v", i, fixture.Reject, got)
			}
			continue
		}
		if !ok {
			t.Errorf("fixture %d: rejected %q", i, fixture.Text)
			continue
		}
		want := *fixture.Want
		want.PostID, want.SeenAt, want.PostedAt = post.PostID, post.SeenAt, post.PostedAt
		switch {
		case got.Confidence < CRYPTO_CONF_BARE || got.Confidence > CRYPTO_CONF_EXPLICIT:
			t.Errorf("fixture %d: confidence %g", i, got.Confidence)
		case want.Quote != CRYPTO_USD && got.Confidence != CRYPTO_CONF_EXPLICIT:
			t.Errorf("fixture %d: explicit pair confidence %g", i, got.Confidence)
		}
		want.Confidence = got.Confidence
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fixture %d:\ngot  %+v\nwant %+v", i, got, want)
		}
	}
}
