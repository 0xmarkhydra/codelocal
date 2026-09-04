package cloudserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/jackc/pgx/v5"
)

type dashboardChatThreadContextKey struct{}

type dashboardChatThreadCreateRequest struct {
	Title        string `json:"title"`
	Model        string `json:"model"`
	WorkspaceKey string `json:"workspaceKey"`
}

type dashboardChatThreadUpdateRequest struct {
	Title string `json:"title"`
}

func dashboardWithChatThread(r *http.Request, threadID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), dashboardChatThreadContextKey{}, strings.TrimSpace(threadID)))
}

func dashboardChatThreadID(r *http.Request) string {
	threadID, _ := r.Context().Value(dashboardChatThreadContextKey{}).(string)
	return strings.TrimSpace(threadID)
}

func dashboardChatMessageID(r *http.Request, userID, role string) string {
	state := dashboardExecutionStateFromRequest(r)
	if state == nil || strings.TrimSpace(state.requestID) == "" {
		return cloud.RandomHex(16)
	}
	digest := sha256.Sum256([]byte(userID + "\n" + state.requestID + "\n" + role))
	return "chat_" + hex.EncodeToString(digest[:16])
}

func (s *Server) saveDashboardChatMessage(r *http.Request, msg cloud.DashboardChatMessage) error {
	if s == nil || s.Store == nil {
		return errors.New("dashboard chat store unavailable")
	}
	if msg.ThreadID == "" {
		msg.ThreadID = dashboardChatThreadID(r)
	}
	if err := s.Store.SaveDashboardChatMessage(r.Context(), msg); err != nil {
		return err
	}
	if msg.ThreadID != "" {
		if err := s.Store.TouchDashboardChatThread(r.Context(), msg.UserID, msg.ThreadID, msg.CreatedAt); err != nil {
			slog.Warn("dashboard chat thread touch after save failed", "error", err, "user", msg.UserID, "thread", msg.ThreadID)
		}
	}
	return nil
}

func writeDashboardChatThreadError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "chat_thread_not_found"})
		return
	}
	webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat_threads_unavailable"})
}

func (s *Server) dashboardChatThreadsAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		threads, err := s.Store.ListDashboardChatThreads(r.Context(), identity.User.ID)
		if err != nil {
			writeDashboardChatThreadError(w, err)
			return
		}
		if threads == nil {
			threads = []cloud.DashboardChatThread{}
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"threads": threads})
		return
	}
	if r.Method != http.MethodPost {
		webutil.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var input dashboardChatThreadCreateRequest
	if webutil.DecodeJSON(r, 32<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	thread, err := s.Store.CreateDashboardChatThread(r.Context(), identity.User.ID, input.Title, input.Model, input.WorkspaceKey, time.Now().UnixMilli())
	if err != nil {
		writeDashboardChatThreadError(w, err)
		return
	}
	webutil.JSON(w, http.StatusCreated, map[string]any{"thread": thread})
}

func (s *Server) dashboardChatThreadAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	threadID := strings.TrimSpace(r.PathValue("id"))
	if threadID == "" || len(threadID) > 96 || strings.ContainsAny(threadID, "\x00\r\n") {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_chat_thread"})
		return
	}
	thread, err := s.Store.GetDashboardChatThread(r.Context(), identity.User.ID, threadID)
	if err != nil {
		writeDashboardChatThreadError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		webutil.JSON(w, http.StatusOK, map[string]any{"thread": thread})
	case http.MethodPatch:
		var input dashboardChatThreadUpdateRequest
		if webutil.DecodeJSON(r, 16<<10, &input) != nil || strings.TrimSpace(input.Title) == "" {
			webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		now := time.Now().UnixMilli()
		if err := s.Store.UpdateDashboardChatThreadTitle(r.Context(), identity.User.ID, threadID, input.Title, now); err != nil {
			writeDashboardChatThreadError(w, err)
			return
		}
		thread, err = s.Store.GetDashboardChatThread(r.Context(), identity.User.ID, threadID)
		if err != nil {
			writeDashboardChatThreadError(w, err)
			return
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"thread": thread})
	case http.MethodDelete:
		if err := s.Store.DeleteDashboardChatThread(r.Context(), identity.User.ID, threadID); err != nil {
			writeDashboardChatThreadError(w, err)
			return
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "threadId": threadID})
	default:
		webutil.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
