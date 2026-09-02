package aipool

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	router     *Router
	registry   *SourceRegistry
	store      *SourceStore
	apiKey     string
	adminToken string
	http       *http.Server
}

type Config struct {
	Addr       string
	APIKey     string
	AdminToken string
	Registry   *SourceRegistry
	Store      *SourceStore
}

func NewServer(config Config) (*Server, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("POOL_API_KEY is required")
	}
	if config.Registry == nil {
		return nil, errors.New("Pool source registry is required")
	}
	addr := strings.TrimSpace(config.Addr)
	if addr == "" {
		addr = ":8080"
	}
	s := &Server{
		router:     NewRouter(config.Registry),
		registry:   config.Registry,
		store:      config.Store,
		apiKey:     strings.TrimSpace(config.APIKey),
		adminToken: strings.TrimSpace(config.AdminToken),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/models", s.requireClient(s.models))
	mux.HandleFunc("POST /v1/chat/completions", s.requireClient(s.routeOpenAI))
	mux.HandleFunc("POST /v1/responses", s.requireClient(s.routeOpenAI))
	mux.HandleFunc("GET /api/models", s.requireAdmin(s.adminModels))
	mux.HandleFunc("GET /api/sources", s.requireAdmin(s.adminSources))
	mux.HandleFunc("POST /api/sources", s.requireAdmin(s.adminCreateSource))
	mux.HandleFunc("PATCH /api/sources/{id}", s.requireAdmin(s.adminUpdateSource))
	mux.HandleFunc("DELETE /api/sources/{id}", s.requireAdmin(s.adminDeleteSource))
	s.http = &http.Server{
		Addr:              addr,
		Handler:           requestLog(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	return s, nil
}

func NewServerFromEnv(ctx context.Context) (*Server, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	var store *SourceStore
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	encryptionKey := strings.TrimSpace(os.Getenv("POOL_ENCRYPTION_KEY"))
	if databaseURL != "" || encryptionKey != "" {
		if databaseURL == "" || encryptionKey == "" {
			return nil, errors.New("DATABASE_URL and POOL_ENCRYPTION_KEY must be configured together")
		}
		created, err := NewSourceStore(ctx, databaseURL, encryptionKey)
		if err != nil {
			return nil, err
		}
		store = created
	}

	static := []Source{}
	nineRouterURL := firstEnv("POOL_9ROUTER_BASE_URL", "CODELOCAL_AI_POOL_BASE_URL")
	nineRouterKey := firstEnv("POOL_9ROUTER_API_KEY", "CODELOCAL_AI_POOL_API_KEY")
	if nineRouterURL != "" || nineRouterKey != "" {
		if nineRouterURL == "" || nineRouterKey == "" {
			if store != nil {
				store.Close()
			}
			return nil, errors.New("POOL_9ROUTER_BASE_URL and POOL_9ROUTER_API_KEY must be configured together")
		}
		priority := envInt("POOL_9ROUTER_PRIORITY", 100)
		nineRouter, err := NewOpenAISource(OpenAISourceOptions{
			ID:            "9router",
			Name:          "9Router",
			Kind:          "9router",
			BaseURL:       nineRouterURL,
			APIKey:        nineRouterKey,
			Priority:      priority,
			StripPrefixes: true,
		})
		if err != nil {
			if store != nil {
				store.Close()
			}
			return nil, err
		}
		static = append(static, nineRouter)
	}

	registry := NewSourceRegistry(store, static...)
	if _, err := registry.Sources(ctx); err != nil {
		if store != nil {
			store.Close()
		}
		return nil, err
	}
	server, err := NewServer(Config{
		Addr:       ":" + port,
		APIKey:     strings.TrimSpace(os.Getenv("POOL_API_KEY")),
		AdminToken: strings.TrimSpace(os.Getenv("POOL_ADMIN_TOKEN")),
		Registry:   registry,
		Store:      store,
	})
	if err != nil && store != nil {
		store.Close()
	}
	return server, err
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) Handler() http.Handler { return s.http.Handler }

func (s *Server) ListenAndServe() error { return s.http.ListenAndServe() }

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if s.store != nil {
		s.store.Close()
	}
	return err
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 8 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func secretEqual(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) requireClient(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !secretEqual(bearerToken(r), s.apiKey) {
			writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid Pool API key")
			return
		}
		next(w, r)
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The client gateway key is intentionally never accepted for control-plane
		// operations. Production must configure a distinct POOL_ADMIN_TOKEN.
		if !secretEqual(bearerToken(r), s.adminToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	models, modelErr := s.router.Models(ctx, true, false)
	sources, sourceErr := s.registry.Summaries(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           modelErr == nil && sourceErr == nil,
		"service":      "codelocal-pool",
		"activeModels": len(models),
		"sources":      len(sources),
	})
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	models, err := s.router.Models(ctx, true, false)
	if err != nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "pool_unavailable", "Pool model catalog is temporarily unavailable")
		return
	}
	data := make([]map[string]any, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]any{
			"id":       model.ID,
			"object":   "model",
			"owned_by": "codelocal-pool",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) adminModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	models, err := s.router.Models(ctx, false, true)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pool_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) routeOpenAI(w http.ResponseWriter, r *http.Request) {
	if contentType := strings.ToLower(r.Header.Get("Content-Type")); !strings.Contains(contentType, "application/json") {
		writeOpenAIError(w, http.StatusUnsupportedMediaType, "invalid_request_error", "Content-Type must be application/json")
		return
	}
	if err := s.router.Route(w, r); err != nil {
		var unavailable *ModelUnavailableError
		if errors.As(err, &unavailable) {
			writeOpenAIError(w, http.StatusNotFound, "model_not_found", "Model is not active in the Pool")
			return
		}
		var upstreams *UpstreamsUnavailableError
		if errors.As(err, &upstreams) {
			writeOpenAIError(w, http.StatusServiceUnavailable, "model_exhausted", "All sources for this model are currently unavailable")
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
}

func (s *Server) adminSources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	sources, err := s.registry.Summaries(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sources_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "storageConfigured": s.store != nil})
}

type createSourceRequest struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	Priority int    `json:"priority"`
}

func (s *Server) adminCreateSource(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "source_storage_not_configured"})
		return
	}
	var input createSourceRequest
	if err := decodeJSON(r, 128<<10, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_source"})
		return
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind == "" {
		kind = "openai-compatible"
	}
	source := StoredSource{
		Name: strings.TrimSpace(input.Name), Kind: kind, BaseURL: strings.TrimSpace(input.BaseURL),
		APIKey: strings.TrimSpace(input.APIKey), Priority: input.Priority, Enabled: true,
	}
	created, err := s.registry.Create(r.Context(), source)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_rejected", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) adminUpdateSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if id == "" || decodeJSON(r, 16<<10, &input) != nil || input.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_source_update"})
		return
	}
	if err := s.registry.SetEnabled(r.Context(), id, *input.Enabled); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "source_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": *input.Enabled})
}

func (s *Server) adminDeleteSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_source"})
		return
	}
	if err := s.registry.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "source_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeJSON(r *http.Request, limit int64, target any) error {
	if r.Body == nil {
		return errors.New("request body required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit {
		return errors.New("request body too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"message": message, "type": code, "code": code},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("pool request", "method", r.Method, "path", r.URL.Path, "durationMs", time.Since(started).Milliseconds())
	})
}
