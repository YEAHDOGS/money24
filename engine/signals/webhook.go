package signals

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	MAX_WEBHOOK_BODY = 8 << 10
	BEARER           = "Bearer "
	DEFAULT_SOURCE   = "webhook"
	MANUAL_SOURCE    = "manual"
	X_SOURCE_PREFIX  = "x:"
)

// WebhookPost is what callers send: callout text, an x.com status link, or both.
// {"text": "BTO $SPY 600C 11/21 @ 1.20"}  ·  {"url": "https://x.com/someone/status/123"}
// Manual marks a post the user shared by hand to paper-trade it right now.
type WebhookPost struct {
	SourceID string `json:"source_id"`
	PostID   string `json:"post_id"`
	ThreadID string `json:"thread_id"`
	Text     string `json:"text"`
	URL      string `json:"url"`
	Manual   bool   `json:"manual"`
}

// Resolver fetches a post's text from its status link.
type Resolver func(ctx context.Context, statusURL string) (Tweet, error)

// Webhook accepts POST /signals. Callers need "Authorization: Bearer <secret>" unless trusted(r) says
// the request comes from the local desk UI. An x.com link is resolved to its author, time and text.
// It answers with the parsed signal so callers see how the post was read.
func Webhook(secret string, trusted func(*http.Request) bool, resolve Resolver, out chan<- Signal) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), BEARER)
		authed := secret != "" && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
		if !authed && !trusted(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var post WebhookPost
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MAX_WEBHOOK_BODY)).Decode(&post); err != nil {
			http.Error(w, "body must be JSON with a text or url field", http.StatusBadRequest)
			return
		}
		post.Text = strings.TrimSpace(post.Text)
		if post.URL == "" {
			post.URL = FindXStatusURL(post.Text)
		}
		if post.Text == "" && post.URL == "" {
			http.Error(w, "nothing to read: send text or an x.com status url", http.StatusBadRequest)
			return
		}

		now := time.Now()
		text := post.Text
		var tweet Tweet
		if post.URL != "" {
			t, err := ParseXStatusURL(post.URL)
			if err == nil {
				tweet = t
				// The shared text is often just the link; the post itself has the callout.
				if resolved, err := resolve(r.Context(), post.URL); err == nil {
					tweet, text = resolved, resolved.Text
				} else if strings.TrimSpace(strings.Replace(text, post.URL, "", 1)) == "" {
					http.Error(w, "could not read that post: "+err.Error()+". Paste its text instead.", http.StatusBadGateway)
					return
				}
			}
		}

		// Expiries without a year ("11/21") resolve against the post's own date, not today's.
		ref := now
		if !tweet.At.IsZero() {
			ref = tweet.At
		}
		sig := Parse(text, ref)
		sig.SeenAt = now
		sig.SourceID, sig.PostID, sig.ThreadID, sig.Manual = post.SourceID, post.PostID, post.ThreadID, post.Manual
		if tweet.ID != "" {
			sig.PostID, sig.PostedAt, sig.URL = tweet.ID, tweet.At, tweet.URL
			if sig.SourceID == "" {
				sig.SourceID = X_SOURCE_PREFIX + tweet.Handle
			}
		}
		if sig.SourceID == "" {
			sig.SourceID = DEFAULT_SOURCE
			if post.Manual {
				sig.SourceID = MANUAL_SOURCE
			}
		}
		select {
		case out <- sig:
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sig)
	}
}
