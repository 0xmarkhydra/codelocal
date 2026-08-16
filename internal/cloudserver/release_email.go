package cloudserver

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/mailer"
)

var releaseVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type releaseNotificationRequest struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
}

type releaseNotificationResponse struct {
	Version     string `json:"version"`
	Recipients  int    `json:"recipients"`
	BatchesSent int    `json:"batchesSent"`
	AlreadySent bool   `json:"alreadySent"`
}

func (s *Server) RegisterReleaseEmail() {
	s.Mux.HandleFunc("POST /internal/releases/notify", s.releaseNotify)
}

func releaseNotifyAuthorized(r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_NOTIFY_SECRET"))
	if expected == "" {
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	actual := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	a, b := []byte(actual), []byte(expected)
	return len(a) == len(b) && len(a) >= 32 && subtle.ConstantTimeCompare(a, b) == 1
}

func releaseEmailContent(version string) (string, string) {
	text := fmt.Sprintf("CodeLocal %s is available.\n\nUpdate with:\n  npm install -g codelocal@latest\n\nThen restart CodeLocal.\n", version)
	html := `<div style="font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;max-width:560px;margin:auto;padding:32px">` +
		`<h2 style="margin:0 0 12px">CodeLocal ` + version + ` is available 🚀</h2>` +
		`<p style="color:#555">A new CodeLocal npm release is ready.</p>` +
		`<div style="background:#111;color:#f5f5f5;border-radius:10px;padding:14px 16px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;margin:22px 0">npm install -g codelocal@latest</div>` +
		`<p style="color:#777;font-size:14px">Restart CodeLocal after updating so your AI clients use the newest runtime and tool surface.</p></div>`
	return text, html
}

func writeReleaseJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) releaseNotify(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_NOTIFY_SECRET")) == "" {
		http.Error(w, "Release notifications are not configured", http.StatusServiceUnavailable)
		return
	}
	if !releaseNotifyAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	var input releaseNotificationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "Invalid release notification payload", http.StatusBadRequest)
		return
	}
	input.Version = strings.TrimSpace(input.Version)
	if !releaseVersionRE.MatchString(input.Version) {
		http.Error(w, "Invalid release version", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	doneKey := "codelocal:release-email:sent:" + input.Version
	if sent, err := s.Store.Redis.Exists(ctx, doneKey).Result(); err != nil {
		http.Error(w, "Unable to check release notification state", http.StatusInternalServerError)
		return
	} else if sent > 0 {
		writeReleaseJSON(w, releaseNotificationResponse{Version: input.Version, AlreadySent: true})
		return
	}
	lockKey := "codelocal:release-email:lock:" + input.Version
	locked, err := s.Store.Redis.SetNX(ctx, lockKey, "1", 15*time.Minute).Result()
	if err != nil {
		http.Error(w, "Unable to lock release notification", http.StatusInternalServerError)
		return
	}
	if !locked {
		http.Error(w, "Release notification is already running", http.StatusConflict)
		return
	}
	defer s.Store.Redis.Del(ctx, lockKey)

	client, err := mailer.FromEnv()
	if err != nil {
		http.Error(w, "Email delivery is not configured", http.StatusServiceUnavailable)
		return
	}
	rows, err := s.Store.DB.Query(ctx, `SELECT email FROM codelocal_users ORDER BY created_at,id`)
	if err != nil {
		http.Error(w, "Unable to load release recipients", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	emails := make([]string, 0, 128)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			http.Error(w, "Unable to read release recipients", http.StatusInternalServerError)
			return
		}
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			emails = append(emails, email)
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Unable to read release recipients", http.StatusInternalServerError)
		return
	}

	text, html := releaseEmailContent(input.Version)
	batchesSent := 0
	for start, batchIndex := 0, 0; start < len(emails); start, batchIndex = start+100, batchIndex+1 {
		end := start + 100
		if end > len(emails) {
			end = len(emails)
		}
		batchKey := fmt.Sprintf("codelocal:release-email:batch:%s:%d", input.Version, batchIndex)
		if done, redisErr := s.Store.Redis.Exists(ctx, batchKey).Result(); redisErr != nil {
			http.Error(w, "Unable to check release batch state", http.StatusInternalServerError)
			return
		} else if done > 0 {
			continue
		}
		messages := make([]mailer.Message, 0, end-start)
		for _, email := range emails[start:end] {
			messages = append(messages, mailer.Message{To: email, Subject: "CodeLocal " + input.Version + " is available", HTML: html, Text: text})
		}
		if err := client.SendBatch(ctx, messages, fmt.Sprintf("codelocal-release-%s-%d", input.Version, batchIndex)); err != nil {
			http.Error(w, "Unable to send release email batch", http.StatusBadGateway)
			return
		}
		if err := s.Store.Redis.Set(ctx, batchKey, "1", 0).Err(); err != nil {
			http.Error(w, "Unable to persist release batch state", http.StatusInternalServerError)
			return
		}
		batchesSent++
	}
	if err := s.Store.Redis.Set(ctx, doneKey, time.Now().UnixMilli(), 0).Err(); err != nil {
		http.Error(w, "Unable to persist release notification state", http.StatusInternalServerError)
		return
	}
	writeReleaseJSON(w, releaseNotificationResponse{Version: input.Version, Recipients: len(emails), BatchesSent: batchesSent})
}
