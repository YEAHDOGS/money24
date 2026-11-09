package signals

import (
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
)

// WebhookPost is what callers send: free text, optionally tagged with a source and post id.
// {"text": "BTO $SPY 600C 11/21 @ 1.20"}
type WebhookPost struct {
	SourceID string `json:"source_id"`
	PostID   string `json:"post_id"`
	ThreadID string `json:"thread_id"`
	Text     string `json:"text"`
}

// Webhook accepts POST /signals with "Authorization: Bearer <secret>", parses the text and hands
// the signal to out. It answers with the parsed signal so callers can see how it was read.
func Webhook(secret string, out chan<- Signal) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), BEARER)
		if secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var post WebhookPost
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MAX_WEBHOOK_BODY)).Decode(&post); err != nil || strings.TrimSpace(post.Text) == "" {
			http.Error(w, "body must be JSON with a text field", http.StatusBadRequest)
			return
		}

		sig := Parse(post.Text, time.Now())
		sig.SourceID, sig.PostID, sig.ThreadID = post.SourceID, post.PostID, post.ThreadID
		if sig.SourceID == "" {
			sig.SourceID = DEFAULT_SOURCE
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
