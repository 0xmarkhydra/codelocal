package mcphub

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
)

const (
	managedPenpotName       = "penpot"
	managedPenpotVersion    = "2.17.0"
	managedPenpotNPXPackage = "@penpot/mcp@" + managedPenpotVersion
	managedPenpotURL        = plugindomain.ManagedPenpotMCPURL
	managedPenpotAddr       = "127.0.0.1:4401"
)

type penpotRuntimeState struct {
	sync.Mutex
	cmd  *exec.Cmd
	refs int
}

var managedPenpotRuntime penpotRuntimeState

func managedPenpotConfig() (ServerConfig, bool) {
	if raw := strings.TrimSpace(os.Getenv("CODELOCAL_PENPOT_MCP_URL")); raw != "" {
		endpoint, err := validURL(raw)
		if err != nil {
			return ServerConfig{}, false
		}
		return ServerConfig{Name: managedPenpotName, Enabled: true, Managed: true, Scope: "global", Transport: "http", URL: endpoint}, true
	}
	// Penpot is a default-installed System Plugin backed by CodeLocal's hosted
	// Penpot deployment. The server remains visible before a user connects their
	// per-account MCP key; the key is materialized only when the HTTP transport
	// is opened and is never included in this public config.
	return ServerConfig{Name: managedPenpotName, Enabled: true, Managed: true, Scope: "global", Transport: "http", URL: managedPenpotURL}, true
}

func managedPenpotCommand() (string, []string, bool) {
	if configured := strings.TrimSpace(os.Getenv("CODELOCAL_PENPOT_MCP_CLI")); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil, true
		}
	}
	if root := strings.TrimSpace(os.Getenv("CODELOCAL_PACKAGE_ROOT")); root != "" {
		entries := []string{
			filepath.Join(root, "node_modules", "@penpot", "mcp", "bin", "mcp-local.js"),
			filepath.Join(filepath.Dir(root), "@penpot", "mcp", "bin", "mcp-local.js"),
		}
		for _, entry := range entries {
			if info, err := os.Stat(entry); err == nil && !info.IsDir() {
				node, err := exec.LookPath("node")
				if err != nil {
					return "", nil, false
				}
				return node, []string{entry}, true
			}
		}
	}
	npx, err := exec.LookPath("npx")
	if err != nil {
		return "", nil, false
	}
	return npx, []string{"-y", managedPenpotNPXPackage}, true
}

func managedPenpotEnv() []string {
	env := os.Environ()
	filtered := env[:0]
	nodeOptions := strings.TrimSpace(os.Getenv("NODE_OPTIONS"))
	for _, entry := range env {
		if strings.HasPrefix(entry, "PENPOT_MCP_") || strings.HasPrefix(entry, "NODE_OPTIONS=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	if !strings.Contains(nodeOptions, "--experimental-sqlite") {
		nodeOptions = strings.TrimSpace(nodeOptions + " --experimental-sqlite")
	}
	return append(filtered,
		"NODE_OPTIONS="+nodeOptions,
		"PENPOT_MCP_SERVER_HOST=127.0.0.1",
		"PENPOT_MCP_SERVER_PORT=4401",
		"PENPOT_MCP_WEBSOCKET_PORT=4402",
		"PENPOT_MCP_REMOTE_MODE=false",
		"PENPOT_MCP_DEVENV=false",
	)
}

func managedPenpotUsesLocalRuntime(endpoint string) bool {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	host := strings.Trim(strings.ToLower(u.Hostname()), "[]")
	return isLoopback(host) && (u.Port() == "" || u.Port() == "4401")
}

func managedPenpotEndpointReady() bool {
	conn, err := net.DialTimeout("tcp", managedPenpotAddr, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func acquireManagedPenpot(ctx context.Context, root string) error {
	managedPenpotRuntime.Lock()
	defer managedPenpotRuntime.Unlock()

	if managedPenpotEndpointReady() {
		managedPenpotRuntime.refs++
		return nil
	}
	command, args, ok := managedPenpotCommand()
	if !ok {
		return errors.New("managed Penpot MCP backend is unavailable; update CodeLocal to a package that includes @penpot/mcp")
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = root
	cmd.Env = managedPenpotEnv()
	if runtime.GOOS != "windows" {
		cmd.Env = append(cmd.Env, "NO_COLOR=1")
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start managed Penpot MCP backend: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			<-done
			return ctx.Err()
		case <-deadline.C:
			_ = cmd.Process.Kill()
			<-done
			return errors.New("managed Penpot MCP backend did not become ready on 127.0.0.1:4401")
		case err := <-done:
			if err == nil {
				return errors.New("managed Penpot MCP backend exited before becoming ready")
			}
			return fmt.Errorf("managed Penpot MCP backend exited before becoming ready: %w", err)
		case <-ticker.C:
			if managedPenpotEndpointReady() {
				managedPenpotRuntime.cmd = cmd
				managedPenpotRuntime.refs++
				go watchManagedPenpot(cmd, done)
				return nil
			}
		}
	}
}

func watchManagedPenpot(cmd *exec.Cmd, done <-chan error) {
	<-done
	managedPenpotRuntime.Lock()
	defer managedPenpotRuntime.Unlock()
	if managedPenpotRuntime.cmd == cmd {
		managedPenpotRuntime.cmd = nil
		managedPenpotRuntime.refs = 0
	}
}

func releaseManagedPenpotRuntime() {
	managedPenpotRuntime.Lock()
	defer managedPenpotRuntime.Unlock()
	if managedPenpotRuntime.refs > 0 {
		managedPenpotRuntime.refs--
	}
	if managedPenpotRuntime.refs == 0 && managedPenpotRuntime.cmd != nil && managedPenpotRuntime.cmd.Process != nil {
		_ = managedPenpotRuntime.cmd.Process.Kill()
		managedPenpotRuntime.cmd = nil
	}
}

func (h *Hub) ensureManagedPenpot(ctx context.Context, config ServerConfig) error {
	if !managedPenpotUsesLocalRuntime(config.URL) {
		return nil
	}
	h.mu.Lock()
	acquired := h.penpot
	h.mu.Unlock()
	if acquired && managedPenpotEndpointReady() {
		return nil
	}
	if acquired {
		releaseManagedPenpotRuntime()
		h.mu.Lock()
		h.penpot = false
		h.mu.Unlock()
	}
	if err := acquireManagedPenpot(ctx, h.Root); err != nil {
		return err
	}
	h.mu.Lock()
	h.penpot = true
	h.mu.Unlock()
	return nil
}

func (h *Hub) releaseManagedPenpot() {
	h.mu.Lock()
	acquired := h.penpot
	h.penpot = false
	h.mu.Unlock()
	if acquired {
		releaseManagedPenpotRuntime()
	}
}
