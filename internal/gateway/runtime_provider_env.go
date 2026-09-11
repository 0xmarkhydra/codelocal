package gateway

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway/opensandbox"
)

func (s *WorkspaceService) ensureRuntimeRouter() (*RuntimeRouter, error) {
	if s == nil {
		return nil, errors.New("workspace service unavailable")
	}
	s.runtimeRouterMu.Lock()
	defer s.runtimeRouterMu.Unlock()
	if s.RuntimeRouter != nil {
		return s.RuntimeRouter, nil
	}
	if s.runtimeRouterInitialized {
		return nil, s.runtimeRouterErr
	}
	s.runtimeRouterInitialized = true

	providers := []RuntimeProvider{NewLocalRuntimeProvider(s, s.Hub)}
	if cloudRuntimeEnabled() {
		provider, err := newOpenSandboxRuntimeProviderFromEnv(s)
		if err != nil {
			s.runtimeRouterErr = err
			return nil, err
		}
		providers = append(providers, provider)
	}
	s.RuntimeRouter = NewRuntimeRouter(providers...)
	return s.RuntimeRouter, nil
}

func cloudRuntimeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_CLOUD_RUNTIME_ENABLED"))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func runtimeDurationEnv(name string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func newOpenSandboxRuntimeProviderFromEnv(workspaces *WorkspaceService) (*OpenSandboxRuntimeProvider, error) {
	baseURL := firstRuntimeEnv("CODELOCAL_OPENSANDBOX_URL", "OPEN_SANDBOX_BASE_URL", "OPENSANDBOX_BASE_URL")
	apiKey := firstRuntimeEnv("CODELOCAL_OPENSANDBOX_API_KEY", "OPEN_SANDBOX_API_KEY", "OPENSANDBOX_API_KEY")
	image := strings.TrimSpace(os.Getenv("CODELOCAL_CLOUD_RUNTIME_IMAGE"))
	serverURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	if baseURL == "" {
		return nil, errors.New("CODELOCAL_OPENSANDBOX_URL is required when cloud runtime is enabled")
	}
	if image == "" {
		return nil, errors.New("CODELOCAL_CLOUD_RUNTIME_IMAGE is required when cloud runtime is enabled")
	}
	if serverURL == "" {
		return nil, errors.New("PUBLIC_BASE_URL is required when cloud runtime is enabled")
	}
	client, err := opensandbox.NewClient(opensandbox.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	profile := strings.TrimSpace(os.Getenv("CODELOCAL_CLOUD_RUNTIME_PROFILE"))
	if profile == "" {
		profile = "general-small"
	}
	cpu := strings.TrimSpace(os.Getenv("CODELOCAL_CLOUD_RUNTIME_CPU"))
	if cpu == "" {
		cpu = "2"
	}
	memory := strings.TrimSpace(os.Getenv("CODELOCAL_CLOUD_RUNTIME_MEMORY"))
	if memory == "" {
		memory = "4Gi"
	}
	if workspaces == nil || workspaces.Store == nil || workspaces.Store.Redis == nil || workspaces.Coordinator == nil || workspaces.Hub == nil {
		return nil, errors.New("cloud runtime requires shared store, coordinator and gateway hub")
	}
	idleTimeout := runtimeDurationEnv("CODELOCAL_CLOUD_RUNTIME_IDLE_TIMEOUT", 20*time.Minute)
	reapInterval := runtimeDurationEnv("CODELOCAL_CLOUD_RUNTIME_REAP_INTERVAL", time.Minute)
	provider := &OpenSandboxRuntimeProvider{
		Provisioner:    opensandbox.NewManager(client),
		Workspaces:     workspaces,
		Hub:            workspaces.Hub,
		Coordinator:    workspaces.Coordinator,
		Leases:         NewRuntimeLeaseCoordinator(NewRedisRuntimeLeaseBackend(workspaces.Store.Redis), 3*time.Minute),
		Bootstrap:      NewRuntimeBootstrapStore(NewRedisRuntimeBootstrapBackend(workspaces.Store.Redis), 2*time.Minute),
		Sessions:       NewCloudRuntimeSessionStore(workspaces.Store.Redis, 30*24*time.Hour),
		ServerURL:      serverURL,
		Image:          image,
		Profile:        profile,
		ResourceLimits: map[string]string{"cpu": cpu, "memory": memory},
		SandboxTTL:     runtimeDurationEnv("CODELOCAL_CLOUD_RUNTIME_SANDBOX_TTL", 30*time.Minute),
		ReadyTimeout:   runtimeDurationEnv("CODELOCAL_CLOUD_RUNTIME_READY_TIMEOUT", 75*time.Second),
		AliasTTL:       runtimeDurationEnv("CODELOCAL_CLOUD_RUNTIME_ALIAS_TTL", 35*time.Minute),
		IdleTimeout:    idleTimeout,
		ReaperOwner:    workspaces.Coordinator.InstanceID,
	}
	// Product callers already route through Coordinator.Call after the cloud
	// alias is resolved. Track activity there so every MCP/Dashboard call blocks
	// idle checkpoint/delete for its full lifetime, regardless of caller shape.
	workspaces.Coordinator.SetRuntimeCallTracking(provider.Sessions, provider.Profile)
	// The reaper is safe to start on every Railway replica: each due session is
	// guarded by a Redis ownership token before any snapshot/delete operation.
	provider.StartIdleReaper(context.Background(), reapInterval)
	return provider, nil
}

func firstRuntimeEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}
