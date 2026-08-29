package cloudserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type dashboardChatHistoryItem struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

type dashboardChatWorkspace struct {
	DeviceID      string `json:"deviceId"`
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
}

type dashboardChatImageMeta struct {
	ImageRef    string `json:"imageRef"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type dashboardChatRequest struct {
	Message   string                     `json:"message"`
	History   []dashboardChatHistoryItem `json:"history"`
	Image     string                     `json:"image,omitempty"`
	ImageMeta *dashboardChatImageMeta    `json:"imageMeta,omitempty"`
	Model     string                     `json:"model,omitempty"`
	Workspace *dashboardChatWorkspace    `json:"workspace,omitempty"`
}

func dashboardChatStoredImage(req dashboardChatRequest) string {
	if req.ImageMeta == nil || strings.TrimSpace(req.ImageMeta.SHA256) == "" || strings.TrimSpace(req.ImageMeta.ContentType) == "" || req.ImageMeta.Size <= 0 {
		return strings.TrimSpace(req.Image)
	}
	payload, err := json.Marshal(req.ImageMeta)
	if err != nil {
		return strings.TrimSpace(req.Image)
	}
	return string(payload)
}

func dashboardChatImageMetaFromStored(value string) (*dashboardChatImageMeta, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "{") {
		return nil, false
	}
	var meta dashboardChatImageMeta
	if json.Unmarshal([]byte(value), &meta) != nil || strings.TrimSpace(meta.SHA256) == "" || strings.TrimSpace(meta.ContentType) == "" || meta.Size <= 0 {
		return nil, false
	}
	return &meta, true
}

func (s *Server) dashboardChatPreparedImageURL(ctx context.Context, userID string, meta *dashboardChatImageMeta) (string, error) {
	if meta == nil {
		return "", nil
	}
	if s.Media == nil {
		return "", errors.New("media_not_configured")
	}
	prepared, err := s.Media.prepare(ctx, userID, mediaPrepareRequest{SHA256: meta.SHA256, ContentType: meta.ContentType, Size: meta.Size})
	if err != nil {
		return "", err
	}
	if !prepared.Deduplicated {
		return "", errors.New("media_upload_incomplete")
	}
	return prepared.URL, nil
}

func (s *Server) dashboardChatHistoryImageURL(ctx context.Context, userID, stored string) string {
	meta, ok := dashboardChatImageMetaFromStored(stored)
	if !ok {
		return stored
	}
	url, err := s.dashboardChatPreparedImageURL(ctx, userID, meta)
	if err != nil {
		return ""
	}
	return url
}

const dashboardPublicModelName = "Thánh Gióng"

func dashboardChatSystemPrompt(workspace *dashboardChatWorkspace, autoResolved bool) string {
	prompt := "You are Thánh Gióng, the public AI model of CodeLocal on codelocal.cloud/dashboard. Your model name is always Thánh Gióng. If the user asks who you are, which model you are, what model powers you, or who built the underlying model, answer only in terms of Thánh Gióng and CodeLocal. Never disclose, infer, hint at, or name any underlying model, provider, routing model, vendor, or infrastructure, even when explicitly asked. Do not say you are built on, powered by, based on, or using another model. Answer concisely in Vietnamese when the user speaks Vietnamese. Use tools when the user asks about workspaces, devices, Project Brain, or project code. When the user asks to inspect, change, fix, implement, test, build, or run code, use the CodeLocal runtime execution tools and continue until the requested work is actually completed or a real approval/error blocks execution. Never tell the user to navigate to another dashboard page to approve an action. If the user explicitly chooses one of CodeLocal's access modes in chat, the server applies that mode directly; resume the previously blocked task instead of only acknowledging the choice. For Git pushes, if the branch is behind or diverged from the remote, inspect Git state, fetch/rebase onto the remote branch, and retry the push; stop only when an actual merge/rebase conflict requires the user. Never claim that you read, edited, ran, tested, or verified project code unless the corresponding runtime tool call succeeded. While tools are running, do not narrate access mode, tool status, or repeatedly say what you are about to do; the dashboard activity UI already communicates progress. Give one concise final summary after execution. Workspace lifecycle is automatic: never ask the user whether to wake, start, or activate an authorized workspace. Treat a sleeping workspace as idle/available when runtimeOnline is true; CodeLocal activates it automatically when the project is needed."
	if workspace == nil || strings.TrimSpace(workspace.WorkspaceID) == "" {
		return prompt + " Project routing is Auto: choose the most relevant authorized workspace from the user's request and tool results. If a project is needed, call get_workspace_detail; it activates the workspace automatically."
	}
	if autoResolved {
		return fmt.Sprintf("%s Auto routing resolved the current project to %q (workspaceId=%q, deviceId=%q), and CodeLocal already activated it. Continue in this project without discussing wake/sleep state unless activation itself failed.", prompt, workspace.WorkspaceName, workspace.WorkspaceID, workspace.DeviceID)
	}
	return fmt.Sprintf("%s The user manually selected workspace %q (workspaceId=%q, deviceId=%q). CodeLocal already activated it. Treat this workspace as the primary project context unless the user explicitly asks to switch projects, and do not ask about wake/sleep state.", prompt, workspace.WorkspaceName, workspace.WorkspaceID, workspace.DeviceID)
}

func dashboardChatNormalizeWorkspaceText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("_", " ", "-", " ", "/", " ", "\\", " ", ".", " ").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func dashboardChatWorkspaceScore(workspace gateway.WorkspaceView, query string) int {
	query = dashboardChatNormalizeWorkspaceText(query)
	if query == "" {
		return 0
	}
	aliases := []string{workspace.WorkspaceName, workspace.ProjectName, workspace.WorkspaceID, workspace.ProjectID}
	best := 0
	for _, alias := range aliases {
		normalized := dashboardChatNormalizeWorkspaceText(alias)
		if len([]rune(normalized)) < 3 {
			continue
		}
		score := 0
		switch {
		case query == normalized:
			score = 10000 + len(normalized)
		case strings.Contains(query, normalized):
			score = 1000 + len(normalized)
		}
		if score > best {
			best = score
		}
	}
	return best
}

func dashboardChatFindWorkspace(catalog []gateway.WorkspaceView, query string) *gateway.WorkspaceView {
	bestScore := 0
	var best *gateway.WorkspaceView
	for i := range catalog {
		score := dashboardChatWorkspaceScore(catalog[i], query)
		if score <= bestScore {
			continue
		}
		copy := catalog[i]
		best = &copy
		bestScore = score
	}
	return best
}

func dashboardChatWantsCurrentWorkspace(message string) bool {
	message = dashboardChatNormalizeWorkspaceText(message)
	for _, phrase := range []string{"dự án đang active", "project đang active", "dự án đang hoạt động", "dự án hiện tại", "project hiện tại", "current project", "active project"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func dashboardChatContinuationMessage(message string) bool {
	message = dashboardChatNormalizeWorkspaceText(message)
	if len([]rune(message)) <= 48 {
		return true
	}
	for _, phrase := range []string{"dự án đó", "project đó", "ở đó", "tiếp tục", "xong chưa", "làm tiếp"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func dashboardChatCurrentWorkspace(catalog []gateway.WorkspaceView) *gateway.WorkspaceView {
	var best *gateway.WorkspaceView
	for i := range catalog {
		if catalog[i].Status != "active" {
			continue
		}
		if best == nil || catalog[i].LastSeenAt > best.LastSeenAt {
			copy := catalog[i]
			best = &copy
		}
	}
	return best
}

func dashboardChatResolveWorkspace(ctx context.Context, s *Server, userID string, req dashboardChatRequest) (*gateway.WorkspaceView, bool, error) {
	if s.Workspaces == nil {
		return nil, false, nil
	}
	catalog, err := s.Workspaces.Catalog(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if req.Workspace != nil && strings.TrimSpace(req.Workspace.WorkspaceID) != "" {
		for i := range catalog {
			if catalog[i].WorkspaceID != req.Workspace.WorkspaceID {
				continue
			}
			if req.Workspace.DeviceID != "" && catalog[i].DeviceID != req.Workspace.DeviceID {
				continue
			}
			copy := catalog[i]
			return &copy, false, nil
		}
		return nil, false, fmt.Errorf("selected workspace is unavailable: %s", req.Workspace.WorkspaceName)
	}
	if matched := dashboardChatFindWorkspace(catalog, req.Message); matched != nil {
		return matched, true, nil
	}
	if dashboardChatWantsCurrentWorkspace(req.Message) {
		if current := dashboardChatCurrentWorkspace(catalog); current != nil {
			return current, true, nil
		}
	}
	if dashboardChatContinuationMessage(req.Message) {
		for i := len(req.History) - 1; i >= 0; i-- {
			if req.History[i].Role != "user" {
				continue
			}
			if matched := dashboardChatFindWorkspace(catalog, req.History[i].Content); matched != nil {
				return matched, true, nil
			}
		}
	}
	return nil, false, nil
}

func dashboardChatActivateWorkspace(ctx context.Context, s *Server, userID string, workspace *gateway.WorkspaceView) (*gateway.WorkspaceView, error) {
	if workspace == nil || workspace.Status == "active" {
		return workspace, nil
	}
	if !workspace.RuntimeOnline {
		return nil, fmt.Errorf("device offline for workspace %s", workspace.WorkspaceName)
	}
	if workspace.Authorized != true {
		return nil, fmt.Errorf("workspace is not authorized: %s", workspace.WorkspaceName)
	}
	return s.Workspaces.Activate(ctx, userID, workspace.Key)
}

func dashboardChatWorkspaceError(workspace *gateway.WorkspaceView, err error) string {
	name := "dự án"
	if workspace != nil && strings.TrimSpace(workspace.WorkspaceName) != "" {
		name = workspace.WorkspaceName
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "offline"):
		return fmt.Sprintf("Máy chứa %s đang offline. Mở CodeLocal trên máy đó rồi thử lại.", name)
	case strings.Contains(lower, "not authorized"), strings.Contains(lower, "no longer authorized"):
		return fmt.Sprintf("%s chưa được CodeLocal cấp quyền trên máy này.", name)
	case strings.Contains(lower, "timed out"):
		return fmt.Sprintf("%s chưa mở kịp. CodeLocal runtime vẫn online, hãy thử lại sau vài giây.", name)
	default:
		return fmt.Sprintf("Không thể mở %s lúc này.", name)
	}
}

func dashboardWorkspaceToolView(workspace gateway.WorkspaceView) map[string]any {
	status := workspace.Status
	if status == "sleeping" {
		status = "idle"
	}
	if status == "device_offline" {
		status = "offline"
	}
	return map[string]any{
		"deviceId": workspace.DeviceID, "deviceName": workspace.DeviceName,
		"workspaceId": workspace.WorkspaceID, "workspaceName": workspace.WorkspaceName,
		"projectId": workspace.ProjectID, "projectName": workspace.ProjectName,
		"status": status, "runtimeOnline": workspace.RuntimeOnline,
		"authorized": workspace.Authorized == true, "lastSeenAt": workspace.LastSeenAt,
	}
}

type dashboardToolCall struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Result     string `json:"result,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Status     string `json:"status"`
}

var dashboardChatTools = append([]map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "list_workspaces",
			"description": "List authorized CodeLocal workspaces",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"all", "active", "idle", "offline"}},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "list_devices",
			"description": "List paired devices and online status",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search_project_brain",
			"description": "Search Project Brain memory/graph",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "get_workspace_detail",
			"description": "Get detail of a workspace by name or id. If the workspace is idle and its runtime is online, CodeLocal activates it automatically before returning details. Never ask the user to wake it manually.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"workspace": map[string]any{"type": "string"}},
				"required":   []string{"workspace"},
			},
		},
	},
}, dashboardRuntimeChatTools...)

func dashboardLLMConfig() (apiKey, baseURL, model string) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_LLM_PROVIDER")))
	apiKey = strings.TrimSpace(os.Getenv("CODELOCAL_LLM_API_KEY"))
	if apiKey == "" && provider == "zen" {
		apiKey = strings.TrimSpace(os.Getenv("OPENCODE_ZEN_API_KEY"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	baseURL = strings.TrimSpace(os.Getenv("CODELOCAL_LLM_BASE_URL"))
	if baseURL == "" {
		if provider == "zen" {
			baseURL = "https://opencode.ai/zen/v1"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	model = strings.TrimSpace(os.Getenv("CODELOCAL_LLM_MODEL"))
	if model == "" {
		if provider == "zen" {
			model = "muse-spark-1.2-contributor-free"
		} else {
			model = "gpt-4o-mini"
		}
	}
	return apiKey, baseURL, model
}

type dashboardLLMProtocol int

const (
	dashboardProtocolUnsupported dashboardLLMProtocol = iota
	dashboardProtocolChatCompletions
	dashboardProtocolResponses
)

func dashboardUsesZen(baseURL string) bool {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_LLM_PROVIDER")))
	return provider == "zen" || strings.Contains(strings.ToLower(baseURL), "opencode.ai/zen")
}

func dashboardProtocolForModel(baseURL, model string) dashboardLLMProtocol {
	if !dashboardUsesZen(baseURL) {
		return dashboardProtocolChatCompletions
	}
	name := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(name, "gpt-"), strings.HasPrefix(name, "grok-"), strings.HasPrefix(name, "muse-"):
		return dashboardProtocolResponses
	case strings.HasPrefix(name, "claude-"), strings.HasPrefix(name, "qwen"), strings.HasPrefix(name, "gemini-"):
		return dashboardProtocolUnsupported
	case strings.HasPrefix(name, "deepseek-"), strings.HasPrefix(name, "minimax-"), strings.HasPrefix(name, "glm-"), strings.HasPrefix(name, "kimi-"), strings.HasPrefix(name, "nemotron-"), strings.HasPrefix(name, "mimo-"), strings.HasPrefix(name, "hy3-"), strings.HasPrefix(name, "x-preview-"), name == "big-pickle":
		return dashboardProtocolChatCompletions
	default:
		return dashboardProtocolUnsupported
	}
}

func writeDashboardSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	payload, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	if flusher != nil {
		flusher.Flush()
	}
}

func decodeDashboardChatRequest(w http.ResponseWriter, r *http.Request) (dashboardChatRequest, bool, error) {
	var req dashboardChatRequest
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return req, false, webutil.DecodeJSON(r, 12<<20, &req)
	}

	maxImageBytes := mediaMaxBytes()
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(2<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return req, false, err
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	payload := r.FormValue("payload")
	if payload == "" || len(payload) > 1<<20 {
		return req, false, errors.New("invalid multipart payload")
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return req, false, err
	}

	file, header, err := r.FormFile("image")
	if errors.Is(err, http.ErrMissingFile) {
		return req, false, nil
	}
	if err != nil {
		return req, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		return req, false, err
	}
	if len(data) == 0 || int64(len(data)) > maxImageBytes {
		return req, false, errors.New("invalid image size")
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if mediaExtension(contentType) == "" {
		contentType = strings.ToLower(http.DetectContentType(data))
	}
	if mediaExtension(contentType) == "" {
		return req, false, errors.New("unsupported image content type")
	}
	req.Image = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
	return req, true, nil
}

func (s *Server) dashboardModelsAPI(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedAPIIdentity(w, r); !ok {
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"models":        dashboardSelectableModels(),
		"default_model": dashboardModelAuto,
	})
}

func (s *Server) dashboardChatAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	// 7. Rate limit 20 req/min per user to prevent LLM abuse
	if allowed, _, retry, _ := s.Store.RateLimit(r.Context(), "dashboard-chat", identity.User.ID, 20, 60); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retry))
		webutil.JSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retry_after": retry})
		return
	}
	if r.Method != http.MethodPost {
		webutil.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	req, ephemeralImage, err := decodeDashboardChatRequest(w, r)
	if err != nil {
		errorCode := "invalid_request"
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
			errorCode = "invalid_json"
		}
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": errorCode})
		return
	}
	msg := strings.TrimSpace(req.Message)
	storedImage := dashboardChatStoredImage(req)
	if ephemeralImage {
		storedImage = ""
	}
	if req.ImageMeta != nil {
		imageURL, imageErr := s.dashboardChatPreparedImageURL(r.Context(), identity.User.ID, req.ImageMeta)
		if imageErr != nil {
			message := "Không thể đọc ảnh đã tải lên. Hãy dán ảnh lại rồi thử gửi."
			if strings.Contains(imageErr.Error(), "media_not_configured") {
				message = "Hệ thống chưa bật upload ảnh."
			}
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": message})
			return
		}
		req.Image = imageURL
	}
	if msg == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing_message"})
		return
	}
	if len(req.History) > 12 {
		req.History = req.History[len(req.History)-12:]
	}

	promptWorkspace := req.Workspace
	autoResolved := false
	var executionWorkspace *gateway.WorkspaceView
	resolvedWorkspace, resolvedByAuto, resolveErr := dashboardChatResolveWorkspace(r.Context(), s, identity.User.ID, req)
	if resolveErr != nil {
		if req.Workspace != nil {
			name := strings.TrimSpace(req.Workspace.WorkspaceName)
			if name == "" {
				name = "Dự án đã chọn"
			}
			webutil.JSON(w, http.StatusConflict, map[string]string{"error": name + " không còn khả dụng."})
			return
		}
		slog.Warn("dashboard chat auto workspace resolve failed", "error", resolveErr, "user", identity.User.ID)
	} else if resolvedWorkspace != nil {
		activeWorkspace, activateErr := dashboardChatActivateWorkspace(r.Context(), s, identity.User.ID, resolvedWorkspace)
		if activateErr != nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": dashboardChatWorkspaceError(resolvedWorkspace, activateErr)})
			return
		}
		if activeWorkspace != nil {
			executionWorkspace = activeWorkspace
			promptWorkspace = &dashboardChatWorkspace{DeviceID: activeWorkspace.DeviceID, WorkspaceID: activeWorkspace.WorkspaceID, WorkspaceName: activeWorkspace.WorkspaceName}
			autoResolved = resolvedByAuto
		}
	}
	r = dashboardWithExecutionState(r, identity.User.ID, executionWorkspace)

	accessChoice, accessRequested := dashboardRequestedAccessChoice(msg)
	accessLabel := ""
	if accessRequested {
		accessWorkspace, accessErr := dashboardAccessWorkspace(r.Context(), s, identity.User.ID, executionWorkspace)
		if accessErr != nil {
			webutil.JSON(w, http.StatusConflict, map[string]string{"error": "Không xác định được dự án để đổi quyền truy cập. Hãy chọn dự án rồi thử lại."})
			return
		}
		executionWorkspace = accessWorkspace
		dashboardSetExecutionWorkspace(r, executionWorkspace)
		promptWorkspace = &dashboardChatWorkspace{DeviceID: executionWorkspace.DeviceID, WorkspaceID: executionWorkspace.WorkspaceID, WorkspaceName: executionWorkspace.WorkspaceName}
		label, accessErr := dashboardApplyAccessChoice(r.Context(), s, identity.User.ID, executionWorkspace, accessChoice)
		if accessErr != nil {
			slog.Warn("dashboard chat access mode update failed", "error", accessErr, "user", identity.User.ID, "workspace", executionWorkspace.WorkspaceID, "mode", accessChoice.Mode)
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Không thể đổi quyền truy cập cho dự án lúc này."})
			return
		}
		accessLabel = label
	}

	selection := dashboardNormalizeModelSelection(req.Model)
	allowCommunity := dashboardCommunityEligible(req) && promptWorkspace == nil
	route := dashboardLLMRoute(selection, allowCommunity)
	model := selection
	isStream := r.URL.Query().Get("stream") == "1" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(event string, data any) {
			writeDashboardSSE(w, flusher, event, data)
		}
		if accessRequested {
			writeSSE("access_mode", dashboardAccessEvent(accessChoice, accessLabel, executionWorkspace))
		}
		// Mock stream when no configured route is available.
		if len(route) == 0 {
			lower2 := strings.ToLower(msg)
			var tcs []dashboardToolCall
			var reply string
			if req.Image != "" {
				tcs = []dashboardToolCall{{ID: "mock_img", Name: "image_upload", Arguments: `{"size":` + fmt.Sprintf("%d", len(req.Image)) + `}`, Result: `{"received":true}`, DurationMs: 30, Status: "done"}}
				reply = "Đã nhận ảnh stream (" + fmt.Sprintf("%d", len(req.Image)) + " bytes) — đã lưu backend."
			} else if strings.Contains(lower2, "workspace") {
				tcs = []dashboardToolCall{{ID: "mock_1", Name: "list_workspaces", Arguments: `{"status":"all"}`, Result: `{"total":26,"sample":["codex-mcp","X.com","BIDDI"]}`, DurationMs: 42, Status: "done"}}
				reply = "Đây là workspaces của bạn (Go mock stream):"
			} else if strings.Contains(lower2, "device") || strings.Contains(lower2, "máy") {
				tcs = []dashboardToolCall{{ID: "mock_2", Name: "list_devices", Arguments: `{}`, Result: `{"paired":1,"online":1}`, DurationMs: 18, Status: "done"}}
				reply = "Thiết bị đã pair (Go mock stream):"
			} else {
				reply = "CodeLocal Go (mock stream - chưa gắn CODELOCAL_LLM_API_KEY): đã nhận \"" + msg + "\"."
			}
			if len(tcs) > 0 {
				writeSSE("tool_calls", map[string]any{"tool_calls": tcs})
				time.Sleep(120 * time.Millisecond)
			}
			// stream reply by words like opencode text-delta
			for _, wrd := range strings.Split(reply, " ") {
				writeSSE("delta", map[string]any{"delta": wrd + " "})
				time.Sleep(35 * time.Millisecond)
			}
			writeSSE("done", map[string]any{"reply": reply, "tool_calls": tcs, "mock": true, "model": dashboardPublicModelName})
			now2 := time.Now().UnixMilli()
			tcsJSON2, _ := json.Marshal(tcs)
			if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), Image: storedImage, CreatedAt: now2}); err != nil {
				slog.Warn("dashboard chat stream mock save user failed", "error", err)
			}
			if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: reply, ToolCalls: json.RawMessage(tcsJSON2), CreatedAt: now2 + 1}); err != nil {
				slog.Warn("dashboard chat stream mock save assistant failed", "error", err)
			}
			return
		}
		// Real LLM stream: proxy OpenAI SSE, handle tool_calls and second call if needed.
		system2 := dashboardChatSystemPrompt(promptWorkspace, autoResolved)
		msgs := []map[string]any{{"role": "system", "content": system2}}
		for _, h := range req.History {
			m := map[string]any{"role": h.Role, "content": h.Content}
			if h.ToolCallID != "" {
				m["tool_call_id"] = h.ToolCallID
			}
			if h.Name != "" {
				m["name"] = h.Name
			}
			msgs = append(msgs, m)
		}
		if accessRequested {
			msgs = append(msgs, dashboardAccessResumeInstruction(accessChoice, accessLabel, executionWorkspace))
		}
		if req.Image != "" {
			msgs = append(msgs, map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": msg}, {"type": "image_url", "image_url": map[string]any{"url": req.Image}}}})
		} else {
			msgs = append(msgs, map[string]any{"role": "user", "content": msg})
		}
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), Image: storedImage, CreatedAt: time.Now().UnixMilli()}); err != nil {
			slog.Warn("dashboard chat stream save user failed", "error", err)
		}
		if _, err := proxyDashboardLLMRouteStream(w, flusher, selection, allowCommunity, msgs, dashboardChatTools, r, s, identity.User.ID); err != nil {
			slog.Warn("dashboard chat stream failed after retry", "error", err, "user", identity.User.ID)
			writeSSE("error", map[string]string{"error": dashboardFriendlyStreamError(err)})
		}
		return
	}
	lower := strings.ToLower(msg)
	if len(route) == 0 {
		var tcs []dashboardToolCall
		var reply string
		if req.Image != "" {
			tcs = []dashboardToolCall{{ID: "mock_img", Name: "image_upload", Arguments: `{"size":` + fmt.Sprintf("%d", len(req.Image)) + `}`, Result: `{"received":true}`, DurationMs: 30, Status: "done"}}
			reply = "Đã nhận ảnh (" + fmt.Sprintf("%d", len(req.Image)) + " bytes) — CodeLocal sẽ phân tích khi model vision được gắn. Hiện mock đã lưu ảnh vào history backend."
		} else if strings.Contains(lower, "workspace") {
			tcs = []dashboardToolCall{{ID: "mock_1", Name: "list_workspaces", Arguments: `{"status":"all"}`, Result: `{"total":26,"sample":["codex-mcp","X.com","BIDDI"]}`, DurationMs: 42, Status: "done"}}
			reply = "Đây là workspaces của bạn (Go mock tool call):"
		} else if strings.Contains(lower, "device") || strings.Contains(lower, "máy") {
			tcs = []dashboardToolCall{{ID: "mock_2", Name: "list_devices", Arguments: `{}`, Result: `{"paired":1,"online":1}`, DurationMs: 18, Status: "done"}}
			reply = "Thiết bị đã pair (Go mock):"
		} else if strings.Contains(lower, "brain") || strings.Contains(lower, "memory") {
			tcs = []dashboardToolCall{{ID: "mock_3", Name: "search_project_brain", Arguments: `{"query":` + jsonQuote(msg) + `}`, Result: `{"hits":3}`, DurationMs: 55, Status: "done"}}
			reply = "Kết quả Project Brain (Go mock):"
		} else {
			reply = "CodeLocal Go (mock - chưa gắn CODELOCAL_LLM_API_KEY): đã nhận \"" + msg + "\". Gắn key vào Go env (railway.json) với MODEL=" + model + " để dùng model free qua codelocal."
		}
		// persist to backend (not FE localStorage)
		now := time.Now().UnixMilli()
		tcsJSON, _ := json.Marshal(tcs)
		// image handled: save with image field for backend history
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), Image: storedImage, CreatedAt: now}); err != nil {
			slog.Warn("dashboard chat save user failed", "error", err, "user", identity.User.ID)
		}
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: reply, ToolCalls: json.RawMessage(tcsJSON), CreatedAt: now + 1}); err != nil {
			slog.Warn("dashboard chat save assistant failed", "error", err)
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"reply": reply, "tool_calls": tcs, "mock": true, "model": dashboardPublicModelName})
		return
	}
	system := dashboardChatSystemPrompt(promptWorkspace, autoResolved)
	messages := []map[string]any{{"role": "system", "content": system}}
	for _, h := range req.History {
		m := map[string]any{"role": h.Role, "content": h.Content}
		if h.ToolCallID != "" {
			m["tool_call_id"] = h.ToolCallID
		}
		if h.Name != "" {
			m["name"] = h.Name
		}
		messages = append(messages, m)
	}
	if accessRequested {
		messages = append(messages, dashboardAccessResumeInstruction(accessChoice, accessLabel, executionWorkspace))
	}
	if req.Image != "" {
		messages = append(messages, map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": msg}, {"type": "image_url", "image_url": map[string]any{"url": req.Image}}}})
	} else {
		messages = append(messages, map[string]any{"role": "user", "content": msg})
	}
	target, toolCalls, content, err := callDashboardLLMWithTools(selection, allowCommunity, messages, dashboardChatTools)
	if err != nil {
		webutil.JSON(w, http.StatusBadGateway, map[string]string{"error": "upstream: " + err.Error()})
		return
	}
	if len(toolCalls) == 0 {
		now3 := time.Now().UnixMilli()
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), Image: storedImage, CreatedAt: now3}); err != nil {
			slog.Warn("dashboard chat save no-tool user failed", "error", err)
		}
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: content, ToolCalls: json.RawMessage(`[]`), CreatedAt: now3 + 1}); err != nil {
			slog.Warn("dashboard chat save no-tool assistant failed", "error", err)
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"reply": content, "model": target.Model, "tool_calls": []dashboardToolCall{}})
		return
	}
	var results []dashboardToolCall
	for _, tc := range toolCalls {
		t0 := time.Now()
		argsMap := map[string]any{}
		_ = json.Unmarshal([]byte(tc.Arguments), &argsMap)
		resStr := execDashboardTool(r, s, identity.User.ID, tc.Name, argsMap)
		results = append(results, dashboardToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, Result: resStr, DurationMs: time.Since(t0).Milliseconds(), Status: "done"})
	}
	follow := append([]map[string]any{}, messages...)
	toolCallsAny := []map[string]any{}
	for _, tc := range toolCalls {
		toolCallsAny = append(toolCallsAny, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
	}
	follow = append(follow, map[string]any{"role": "assistant", "content": content, "tool_calls": toolCallsAny})
	for _, tr := range results {
		follow = append(follow, map[string]any{"role": "tool", "content": tr.Result, "tool_call_id": tr.ID, "name": tr.Name})
	}
	finalTarget, _, finalContent, err2 := callDashboardLLMWithTools(target.Model, false, follow, nil)
	if err2 != nil {
		webutil.JSON(w, http.StatusBadGateway, map[string]any{"error": "upstream2: " + err2.Error(), "tool_calls": results})
		return
	}
	now4 := time.Now().UnixMilli()
	tcsJSON4, _ := json.Marshal(results)
	if len(tcsJSON4) > 5000 {
		tcsJSON4 = tcsJSON4[:5000]
	}
	if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), Image: storedImage, CreatedAt: now4}); err != nil {
		slog.Warn("dashboard chat save final user failed", "error", err)
	}
	if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: finalContent, ToolCalls: json.RawMessage(tcsJSON4), CreatedAt: now4 + 1}); err != nil {
		slog.Warn("dashboard chat save final assistant failed", "error", err)
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"reply": finalContent, "model": finalTarget.Model, "tool_calls": results})
}

func (s *Server) dashboardChatHistoryAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodDelete {
		_ = s.Store.ClearDashboardChatHistory(r.Context(), identity.User.ID)
		webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	msgs, err := s.Store.ListDashboardChatHistory(r.Context(), identity.User.ID, 50)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history_unavailable"})
		return
	}
	for i := range msgs {
		if msgs[i].Image != "" {
			msgs[i].Image = s.dashboardChatHistoryImageURL(r.Context(), identity.User.ID, msgs[i].Image)
		}
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func execDashboardTool(r *http.Request, s *Server, userID, name string, args map[string]any) string {
	const maxToolResult = 5000
	trunc := func(b []byte) string {
		if len(b) > maxToolResult {
			return string(b[:maxToolResult]) + `,"truncated":true}`
		}
		return string(b)
	}
	switch name {
	case "list_workspaces":
		ws, err := s.Workspaces.Catalog(r.Context(), userID)
		if err != nil {
			return `{"error":"workspaces_unavailable"}`
		}
		wanted, _ := args["status"].(string)
		views := make([]map[string]any, 0, len(ws))
		for _, workspace := range ws {
			view := dashboardWorkspaceToolView(workspace)
			status, _ := view["status"].(string)
			if wanted != "" && wanted != "all" && wanted != status {
				continue
			}
			views = append(views, view)
		}
		b, _ := json.Marshal(map[string]any{"total": len(views), "workspaces": views})
		return trunc(b)
	case "list_devices":
		devs, err := s.Store.ListDevices(r.Context(), userID)
		if err != nil {
			return `{"error":"devices_unavailable"}`
		}
		for i := range devs {
			devs[i].SecretHash = ""
		}
		b, _ := json.Marshal(map[string]any{"devices": devs})
		return trunc(b)
	case "search_project_brain":
		q, _ := args["query"].(string)
		return `{"query":` + jsonQuote(q) + `,"hits":[{"path":"web/src/app/dashboard","score":0.92}],"note":"Go mock - will query Project Brain index"}`
	case "get_workspace_detail":
		wk, _ := args["workspace"].(string)
		catalog, err := s.Workspaces.Catalog(r.Context(), userID)
		if err != nil {
			return `{"error":"workspaces_unavailable"}`
		}
		workspace := dashboardChatFindWorkspace(catalog, wk)
		if workspace == nil {
			return `{"error":"workspace_not_found","workspace":` + jsonQuote(wk) + `}`
		}
		active, err := dashboardChatActivateWorkspace(r.Context(), s, userID, workspace)
		if err != nil {
			b, _ := json.Marshal(map[string]any{"error": "workspace_activation_failed", "message": dashboardChatWorkspaceError(workspace, err), "workspace": dashboardWorkspaceToolView(*workspace)})
			return trunc(b)
		}
		dashboardSetExecutionWorkspace(r, active)
		b, _ := json.Marshal(map[string]any{"workspace": dashboardWorkspaceToolView(*active), "ready": true})
		return trunc(b)
	default:
		if result, ok := execDashboardRuntimeTool(r, s, userID, name, args); ok {
			return result
		}
		return `{"error":"unknown tool ` + name + `"}`
	}
}

type llmToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func responsesInput(messages []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		role, _ := message["role"].(string)
		if role == "tool" {
			out = append(out, map[string]any{"type": "function_call_output", "call_id": message["tool_call_id"], "output": message["content"]})
			continue
		}
		if role == "assistant" {
			if calls, ok := message["tool_calls"].([]map[string]any); ok && len(calls) > 0 {
				if content, ok := message["content"].(string); ok && strings.TrimSpace(content) != "" {
					out = append(out, map[string]any{"role": role, "content": content})
				}
				for _, call := range calls {
					fn, _ := call["function"].(map[string]any)
					out = append(out, map[string]any{"type": "function_call", "call_id": call["id"], "name": fn["name"], "arguments": fn["arguments"]})
				}
				continue
			}
		}
		content := message["content"]
		if parts, ok := content.([]map[string]any); ok {
			converted := make([]map[string]any, 0, len(parts))
			for _, part := range parts {
				switch part["type"] {
				case "text":
					converted = append(converted, map[string]any{"type": "input_text", "text": part["text"]})
				case "image_url":
					image, _ := part["image_url"].(map[string]any)
					converted = append(converted, map[string]any{"type": "input_image", "image_url": image["url"]})
				default:
					converted = append(converted, part)
				}
			}
			content = converted
		}
		out = append(out, map[string]any{"role": role, "content": content})
	}
	return out
}

func responsesTools(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		fn, _ := tool["function"].(map[string]any)
		if len(fn) == 0 {
			continue
		}
		out = append(out, map[string]any{"type": "function", "name": fn["name"], "description": fn["description"], "parameters": fn["parameters"]})
	}
	return out
}

func callResponsesWithTools(baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any) ([]llmToolCall, string, error) {
	body := map[string]any{"model": model, "input": responsesInput(messages)}
	if converted := responsesTools(tools); len(converted) > 0 {
		body["tools"] = converted
		body["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/responses", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	var data struct {
		Output []struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", err
	}
	var content strings.Builder
	var calls []llmToolCall
	for _, item := range data.Output {
		switch item.Type {
		case "function_call":
			id := item.CallID
			if id == "" {
				id = item.ID
			}
			calls = append(calls, llmToolCall{ID: id, Name: item.Name, Arguments: item.Arguments})
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" && part.Text != "" {
					content.WriteString(part.Text)
				}
			}
		}
	}
	return calls, content.String(), nil
}

func callChatCompletionsWithTools(baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any) ([]llmToolCall, string, error) {
	body := map[string]any{"model": model, "messages": messages, "temperature": 0.7}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", err
	}
	if len(data.Choices) == 0 {
		return nil, "", &httpError{Status: 502, Body: "empty choices"}
	}
	m := data.Choices[0].Message
	var tcs []llmToolCall
	for _, tc := range m.ToolCalls {
		tcs = append(tcs, llmToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return tcs, m.Content, nil
}

func callLLMWithTools(baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any) ([]llmToolCall, string, error) {
	switch dashboardProtocolForModel(baseURL, model) {
	case dashboardProtocolResponses:
		return callResponsesWithTools(baseURL, apiKey, model, messages, tools)
	case dashboardProtocolChatCompletions:
		return callChatCompletionsWithTools(baseURL, apiKey, model, messages, tools)
	default:
		return nil, "", &httpError{Status: http.StatusBadRequest, Body: "unsupported model protocol"}
	}
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

func writeDashboardTextDeltas(w http.ResponseWriter, flusher http.Flusher, content string) {
	runes := []rune(content)
	const chunkSize = 28
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		writeDashboardSSE(w, flusher, "delta", map[string]any{"delta": string(runes[start:end])})
	}
}

func proxyResponsesStream(w http.ResponseWriter, flusher http.Flusher, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, r *http.Request, s *Server, userID string) error {
	follow := append([]map[string]any{}, messages...)
	allResults := make([]dashboardToolCall, 0, 12)
	seenProgress := map[string]int{}
	stopReason := ""
	var visibleContent strings.Builder

	for round := 0; round < dashboardMaxToolRounds && len(allResults) < dashboardMaxToolCalls; round++ {
		visibleBeforeRound := visibleContent.String()
		roundTextVisible := false
		roundUsesTool := false
		rollbackRoundText := func() {
			if !roundTextVisible {
				return
			}
			visibleContent.Reset()
			visibleContent.WriteString(visibleBeforeRound)
			writeDashboardSSE(w, flusher, "replace", map[string]any{"content": visibleBeforeRound})
			roundTextVisible = false
		}
		callbacks := dashboardResponsesStreamCallbacks{
			OnText: func(delta string) {
				if delta == "" || roundUsesTool {
					return
				}
				roundTextVisible = true
				visibleContent.WriteString(delta)
				writeDashboardSSE(w, flusher, "delta", map[string]any{"delta": delta})
			},
			OnToolDelta: func(index int, id, name, arguments string) {
				if !roundUsesTool {
					roundUsesTool = true
					rollbackRoundText()
				}
				writeDashboardSSE(w, flusher, "tool_delta", map[string]any{"tool_calls": []map[string]any{{
					"index": index, "id": id, "name": name, "arguments": arguments,
				}}})
			},
		}

		roundResult, err := dashboardStreamResponsesRoundWithRetry(r.Context(), baseURL, apiKey, model, follow, tools, callbacks)
		if err != nil {
			if len(allResults) == 0 {
				return err
			}
			rollbackRoundText()
			stopReason = "model connection interrupted after completed tool work"
			break
		}
		toolCalls := roundResult.ToolCalls
		content := roundResult.Content
		if len(toolCalls) > 0 && !roundUsesTool {
			roundUsesTool = true
			rollbackRoundText()
		}
		if len(toolCalls) == 0 {
			finalContent := visibleContent.String()
			if finalContent == "" {
				finalContent = content
			}
			tcsJSON, _ := json.Marshal(allResults)
			_ = s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(12), UserID: userID, Role: "assistant", Content: finalContent, ToolCalls: json.RawMessage(tcsJSON), CreatedAt: time.Now().UnixMilli()})
			writeDashboardSSE(w, flusher, "done", map[string]any{"done": true, "reply": finalContent, "tool_calls": allResults, "model": dashboardPublicModelName})
			return nil
		}

		results := make([]dashboardToolCall, 0, len(toolCalls))
		toolCallsAny := make([]map[string]any, 0, len(toolCalls))
		noProgress := false
		for _, tc := range toolCalls {
			if len(allResults) >= dashboardMaxToolCalls {
				stopReason = "tool budget reached"
				break
			}
			t0 := time.Now()
			argsMap := map[string]any{}
			_ = json.Unmarshal([]byte(tc.Arguments), &argsMap)
			resStr := execDashboardTool(r, s, userID, tc.Name, argsMap)
			status := dashboardToolResultStatus(resStr)
			result := dashboardToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, Result: resStr, DurationMs: time.Since(t0).Milliseconds(), Status: status}
			results = append(results, result)
			allResults = append(allResults, result)
			toolCallsAny = append(toolCallsAny, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})

			fingerprint := dashboardToolProgressFingerprint(tc, resStr)
			seenProgress[fingerprint]++
			if seenProgress[fingerprint] >= dashboardDuplicateResultLimit {
				noProgress = true
			}
		}
		if len(results) > 0 {
			writeDashboardSSE(w, flusher, "tool_calls", map[string]any{"tool_calls": results})
		}
		follow = append(follow, map[string]any{"role": "assistant", "content": "", "tool_calls": toolCallsAny})
		for _, result := range results {
			follow = append(follow, map[string]any{"role": "tool", "content": result.Result, "tool_call_id": result.ID, "name": result.Name})
		}
		if noProgress {
			stopReason = "repeated tool calls produced no new result"
			break
		}
	}

	if stopReason == "" {
		stopReason = "tool execution budget reached"
	}
	if visibleContent.Len() > 0 {
		visibleContent.WriteString("\n\n")
		writeDashboardSSE(w, flusher, "delta", map[string]any{"delta": "\n\n"})
	}
	finalMessages := dashboardFinalSynthesisMessages(follow, stopReason)
	finalCallbacks := dashboardResponsesStreamCallbacks{OnText: func(delta string) {
		if delta == "" {
			return
		}
		visibleContent.WriteString(delta)
		writeDashboardSSE(w, flusher, "delta", map[string]any{"delta": delta})
	}}
	finalRound, err := dashboardStreamResponsesRoundWithRetry(r.Context(), baseURL, apiKey, model, finalMessages, nil, finalCallbacks)
	if err != nil {
		if strings.TrimSpace(finalRound.Content) == "" {
			fallback := dashboardFallbackReply(allResults)
			visibleContent.WriteString(fallback)
			writeDashboardTextDeltas(w, flusher, fallback)
		} else {
			suffix := "\n\nKết nối phần tổng hợp vừa gián đoạn; các thay đổi đã thực hiện vẫn được giữ nguyên."
			visibleContent.WriteString(suffix)
			writeDashboardSSE(w, flusher, "delta", map[string]any{"delta": suffix})
		}
	}
	finalContent := visibleContent.String()
	if strings.TrimSpace(finalContent) == "" {
		finalContent = dashboardFallbackReply(allResults)
		writeDashboardTextDeltas(w, flusher, finalContent)
	}
	tcsJSON, _ := json.Marshal(allResults)
	_ = s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(12), UserID: userID, Role: "assistant", Content: finalContent, ToolCalls: json.RawMessage(tcsJSON), CreatedAt: time.Now().UnixMilli()})
	writeDashboardSSE(w, flusher, "done", map[string]any{"done": true, "reply": finalContent, "tool_calls": allResults, "model": dashboardPublicModelName, "recovered": true})
	return nil
}

func proxyLLMStream(w http.ResponseWriter, flusher http.Flusher, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, r *http.Request, s *Server, userID string) error {
	switch dashboardProtocolForModel(baseURL, model) {
	case dashboardProtocolResponses:
		return proxyResponsesStream(w, flusher, baseURL, apiKey, model, messages, tools, r, s, userID)
	case dashboardProtocolUnsupported:
		return &httpError{Status: http.StatusBadRequest, Body: "unsupported model protocol"}
	}

	body := map[string]any{"model": model, "messages": messages, "temperature": 0.7, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	writeRaw := func(data string) {
		_, _ = fmt.Fprintf(w, "event: delta\ndata: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	toolCallsByIndex := map[int]*llmToolCall{}
	var fullContent strings.Builder
	finishedWithToolCalls := false
	rolledBackToolPreamble := false
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   *string `json:"content"`
					ToolCalls []struct {
						Index    int     `json:"index"`
						ID       *string `json:"id"`
						Function *struct {
							Name      *string `json:"name"`
							Arguments *string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != nil && *delta.Content != "" {
			fullContent.WriteString(*delta.Content)
			b2, _ := json.Marshal(map[string]any{"delta": *delta.Content})
			writeRaw(string(b2))
		}
		if len(delta.ToolCalls) > 0 && !rolledBackToolPreamble && fullContent.Len() > 0 {
			rolledBackToolPreamble = true
			writeDashboardSSE(w, flusher, "replace", map[string]any{"content": ""})
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if _, ok := toolCallsByIndex[idx]; !ok {
				toolCallsByIndex[idx] = &llmToolCall{}
			}
			if tc.ID != nil {
				toolCallsByIndex[idx].ID = *tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != nil {
					toolCallsByIndex[idx].Name = *tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					toolCallsByIndex[idx].Arguments += *tc.Function.Arguments
				}
			}
			// stream tool call delta as event
			b2, _ := json.Marshal(map[string]any{"tool_calls": []map[string]any{{"index": idx, "id": toolCallsByIndex[idx].ID, "name": toolCallsByIndex[idx].Name, "arguments": toolCallsByIndex[idx].Arguments}}})
			_, _ = fmt.Fprintf(w, "event: tool_delta\ndata: %s\n\n", string(b2))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "tool_calls" {
			finishedWithToolCalls = true
		}
	}
	if len(toolCallsByIndex) > 0 && finishedWithToolCalls {
		var tcs []llmToolCall
		for i := 0; i < len(toolCallsByIndex); i++ {
			if tc, ok := toolCallsByIndex[i]; ok {
				tcs = append(tcs, *tc)
			}
		}
		if err := dashboardValidateToolCalls(tcs); err != nil {
			return err
		}
		// execute tools and stream final answer like opencode second call
		var results []dashboardToolCall
		for _, tc := range tcs {
			t0 := time.Now()
			argsMap := map[string]any{}
			_ = json.Unmarshal([]byte(tc.Arguments), &argsMap)
			resStr := execDashboardTool(r, s, userID, tc.Name, argsMap)
			results = append(results, dashboardToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, Result: resStr, DurationMs: time.Since(t0).Milliseconds(), Status: "done"})
			b2, _ := json.Marshal(map[string]any{"tool_calls": results})
			_, _ = fmt.Fprintf(w, "event: tool_calls\ndata: %s\n\n", string(b2))
			if flusher != nil {
				flusher.Flush()
			}
		}
		// second LLM call streamed
		follow := append([]map[string]any{}, messages...)
		toolCallsAny := []map[string]any{}
		for _, tc := range tcs {
			toolCallsAny = append(toolCallsAny, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
		}
		follow = append(follow, map[string]any{"role": "assistant", "content": fullContent.String(), "tool_calls": toolCallsAny})
		for _, tr := range results {
			follow = append(follow, map[string]any{"role": "tool", "content": tr.Result, "tool_call_id": tr.ID, "name": tr.Name})
		}
		_, _, secondContent, err := callDashboardLLMWithTools(model, false, follow, nil)
		if err != nil {
			return err
		}
		writeDashboardTextDeltas(w, flusher, secondContent)
		tcsJSON, _ := json.Marshal(results)
		_ = s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(12), UserID: userID, Role: "assistant", Content: secondContent, ToolCalls: json.RawMessage(tcsJSON), CreatedAt: time.Now().UnixMilli()})
	}
	// persist assistant for non-tool stream
	if len(toolCallsByIndex) == 0 {
		_ = s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(12), UserID: userID, Role: "assistant", Content: fullContent.String(), ToolCalls: json.RawMessage(`[]`), CreatedAt: time.Now().UnixMilli()})
	}
	_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", `{"done":true}`)
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
