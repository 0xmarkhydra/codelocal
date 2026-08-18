//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const macNativeWorkerLockPoll = 10 * time.Millisecond

func macNativeDaemonName(goarch string) string {
	if strings.TrimSpace(goarch) == "" {
		goarch = runtime.GOARCH
	}
	return "computer-native-darwin-" + goarch
}

func macNativeDaemonCandidates(configured, packageRoot, executablePath, goarch string) []string {
	name := macNativeDaemonName(goarch)
	candidates := make([]string, 0, 4)
	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, configured)
	}
	if packageRoot = strings.TrimSpace(packageRoot); packageRoot != "" {
		candidates = append(candidates,
			filepath.Join(packageRoot, "bin", "helpers", name),
			filepath.Join(packageRoot, "helpers", name),
		)
	}
	if executablePath = strings.TrimSpace(executablePath); executablePath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(executablePath), name))
	}
	return candidates
}

func macNativeDaemonPath() string {
	executable, _ := os.Executable()
	for _, candidate := range macNativeDaemonCandidates(
		os.Getenv("CODELOCAL_COMPUTER_NATIVE_DAEMON"),
		os.Getenv("CODELOCAL_PACKAGE_ROOT"),
		executable,
		runtime.GOARCH,
	) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

type macNativeWorker struct {
	mu     sync.Mutex
	path   string
	ready  bool
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
}

var sharedMacNativeWorker macNativeWorker

func (w *macNativeWorker) lock(ctx context.Context) error {
	ticker := time.NewTicker(macNativeWorkerLockPoll)
	defer ticker.Stop()
	for {
		if w.mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *macNativeWorker) resetLocked() {
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_, _ = w.cmd.Process.Wait()
	}
	w.path = ""
	w.ready = false
	w.cmd = nil
	w.stdin = nil
	w.stdout = nil
	w.stderr.Reset()
}

func (w *macNativeWorker) startLocked(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("native macOS AX daemon is not installed")
	}
	if w.cmd != nil && w.cmd.Process != nil && w.stdin != nil && w.stdout != nil && w.path == path {
		return nil
	}
	w.resetLocked()
	cmd := exec.Command(path, "--serve")
	cmd.Env = append(os.Environ(), "CI=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	w.stderr.Reset()
	cmd.Stderr = &w.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}
	w.path = path
	w.cmd = cmd
	w.stdin = stdin
	w.stdout = bufio.NewReaderSize(stdoutPipe, 64<<10)
	return nil
}

func (w *macNativeWorker) callLocked(ctx context.Context, request map[string]any) (any, error) {
	raw, _ := json.Marshal(request)
	if _, err := w.stdin.Write(append(raw, '\n')); err != nil {
		w.resetLocked()
		return nil, err
	}
	type readResult struct {
		line []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func(reader *bufio.Reader) {
		line, err := reader.ReadBytes('\n')
		readCh <- readResult{line: line, err: err}
	}(w.stdout)
	select {
	case <-ctx.Done():
		w.resetLocked()
		return nil, ctx.Err()
	case read := <-readCh:
		if read.err != nil {
			message := strings.TrimSpace(w.stderr.String())
			w.resetLocked()
			if message == "" {
				message = read.err.Error()
			}
			return nil, errors.New(message)
		}
		var response macWorkerResponse
		if err := json.Unmarshal(read.line, &response); err != nil {
			w.resetLocked()
			return nil, fmt.Errorf("invalid native macOS AX daemon response: %w", err)
		}
		if !response.OK {
			if response.Error == "" {
				response.Error = "native macOS AX operation failed"
			}
			return nil, errors.New(response.Error)
		}
		return response.Result, nil
	}
}

func (w *macNativeWorker) ensureReady(ctx context.Context) bool {
	path := macNativeDaemonPath()
	if path == "" {
		return false
	}
	if err := w.lock(ctx); err != nil {
		return false
	}
	defer w.mu.Unlock()
	if err := w.startLocked(path); err != nil {
		return false
	}
	if w.ready {
		return true
	}
	value, err := w.callLocked(ctx, map[string]any{"version": 1, "op": "ping"})
	if err != nil {
		return false
	}
	root, _ := value.(map[string]any)
	ready, _ := root["ready"].(bool)
	w.ready = ready
	return ready
}

func (w *macNativeWorker) call(ctx context.Context, request map[string]any) (any, error) {
	path := macNativeDaemonPath()
	if path == "" {
		return nil, errors.New("native macOS AX daemon is not installed")
	}
	if err := w.lock(ctx); err != nil {
		return nil, err
	}
	defer w.mu.Unlock()
	if err := w.startLocked(path); err != nil {
		return nil, err
	}
	request["version"] = 1
	return w.callLocked(ctx, request)
}

func macNativeDaemonProbe(ctx context.Context) (map[string]any, bool) {
	path := macNativeDaemonPath()
	if path == "" {
		return nil, false
	}
	cmd := exec.CommandContext(ctx, path, "--capabilities")
	cmd.Env = append(os.Environ(), "CI=1")
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var capabilities map[string]any
	if json.Unmarshal(output, &capabilities) != nil {
		return nil, false
	}
	ready, _ := capabilities["ready"].(bool)
	return capabilities, ready
}

func macNativeWindows(ctx context.Context) ([]any, error) {
	value, err := sharedMacNativeWorker.call(ctx, map[string]any{"op": "windows"})
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("native macOS windows returned invalid payload")
	}
	return items, nil
}

func macNativeTree(ctx context.Context, pid, windowIndex, max int) (map[string]any, error) {
	value, err := sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "tree", "pid": pid, "windowIndex": windowIndex, "max": max,
	})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("native macOS tree returned invalid payload")
	}
	return root, nil
}

func macNativeCapture(ctx context.Context, pid, windowIndex, maxWidth int) (map[string]any, error) {
	value, err := sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "capture", "pid": pid, "windowIndex": windowIndex, "maxWidth": maxWidth,
	})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("native macOS capture returned invalid payload")
	}
	return root, nil
}

func macNativeVision(ctx context.Context, pid, windowIndex, maxWidth int) ([]any, error) {
	value, err := sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "vision", "pid": pid, "windowIndex": windowIndex, "maxWidth": maxWidth,
	})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("native macOS Vision returned invalid payload")
	}
	nodes, ok := root["nodes"].([]any)
	if !ok {
		return nil, errors.New("native macOS Vision nodes are invalid")
	}
	return nodes, nil
}

func macNativeElementRead(ctx context.Context, pid int, elementID string) (any, error) {
	return sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "element_read", "pid": pid, "elementId": elementID,
	})
}

func macNativeElementAction(ctx context.Context, pid int, elementID, operation, text string) (any, error) {
	return sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "element_action", "pid": pid, "elementId": elementID, "operation": operation, "text": text,
	})
}

func macNativeSemanticAction(ctx context.Context, pid, windowIndex int, operation, target, text string) (any, error) {
	return sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "semantic", "pid": pid, "windowIndex": windowIndex, "operation": operation, "target": target, "text": text, "max": 500,
	})
}

func macNativeSemanticBatch(ctx context.Context, pid, windowIndex int, steps any) (any, error) {
	return sharedMacNativeWorker.call(ctx, map[string]any{
		"op": "semantic_batch", "pid": pid, "windowIndex": windowIndex, "steps": steps,
	})
}

func macNativeSceneEvents(ctx context.Context) ([]any, error) {
	value, err := sharedMacNativeWorker.call(ctx, map[string]any{"op": "events"})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("native macOS scene event payload is invalid")
	}
	events, ok := root["events"].([]any)
	if !ok {
		return nil, errors.New("native macOS scene events are invalid")
	}
	return events, nil
}
