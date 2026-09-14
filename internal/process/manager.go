package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusExited    Status = "exited"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

type streamBuffer struct {
	Data       []byte
	BaseOffset int64
	TotalBytes int64
}

type Record struct {
	ProcessID      string
	WorkspaceKey   string
	OwnerSessionID string
	RequestID      string
	PID            int
	Command        string
	CWD            string
	DisplayCWD     string
	StartedAt      int64
	LastActivityAt int64
	Status         Status
	ExitCode       *int
	Signal         string
	Stdout         streamBuffer
	Stderr         streamBuffer
	TimeoutAt      int64
	PTY            bool
	ExecutionMode  string
	Sandboxed      bool
	SandboxUnavailable string
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	pty            ptyHandle
	cancel         context.CancelFunc
	redactValues   []string
}

type Snapshot struct {
	ProcessID      string         `json:"processId"`
	WorkspaceKey   string         `json:"workspaceKey"`
	OwnerSessionID string         `json:"ownerSessionId,omitempty"`
	PID            int            `json:"pid,omitempty"`
	Command        string         `json:"command"`
	CWD            string         `json:"cwd"`
	StartedAt      int64          `json:"startedAt"`
	LastActivityAt int64          `json:"lastActivityAt"`
	Status         Status         `json:"status"`
	Running        bool           `json:"running"`
	ExitCode       *int           `json:"exitCode"`
	Signal         string         `json:"signal,omitempty"`
	TimeoutAt      int64          `json:"timeoutAt,omitempty"`
	PTY            bool           `json:"pty"`
	ExecutionMode  string         `json:"executionMode"`
	// Sandboxed reports whether the command ran behind a kernel sandbox.
	// SandboxUnavailable is set instead when sandboxing could not be applied,
	// so a caller never has to infer the boundary from the mode string alone.
	Sandboxed          bool   `json:"sandboxed"`
	SandboxUnavailable string `json:"sandboxUnavailable,omitempty"`
	Stdout         map[string]any `json:"stdout"`
	Stderr         map[string]any `json:"stderr"`
}

type StartOptions struct {
	CWD            string
	DisplayCWD     string
	Timeout        time.Duration
	OwnerSessionID string
	RequestID      string
	UsePTY         bool
	Cols           int
	Rows           int
	Env            map[string]string
	RedactValues   []string
	// SandboxProfile is an SBPL (seatbelt) profile applied to the shell that
	// runs the command. Command text rules cannot stop an interpreter from
	// building a credential path at runtime, so the boundary is enforced in
	// the kernel where the path is finally resolved.
	SandboxProfile string
	// SandboxHelper is the sandbox-exec binary path. Empty means the command
	// runs unsandboxed and SandboxUnavailable says why.
	SandboxHelper string
	// SandboxUnavailable records the reason kernel sandboxing is off, so the
	// process snapshot can surface it instead of silently losing the boundary.
	SandboxUnavailable string
}

type Manager struct {
	mu               sync.Mutex
	workspaceRoot    string
	workspaceKey     string
	records          map[string]*Record
	requestToProcess map[string]string
	maxBufferBytes   int
	maxProcesses     int
	onOutput         func(*Record, string, string)
	onSettled        func(*Record)
}

func NewManager(workspaceRoot, workspaceKey string, onOutput func(*Record, string, string), onSettled func(*Record)) *Manager {
	maxBuffer := envInt("CODELOCAL_MAX_PROCESS_BUFFER_BYTES", 2*1024*1024)
	maxProcesses := envInt("CODELOCAL_MAX_PROCESSES", 64)
	return &Manager{workspaceRoot: workspaceRoot, workspaceKey: workspaceKey, records: map[string]*Record{}, requestToProcess: map[string]string{}, maxBufferBytes: maxBuffer, maxProcesses: maxProcesses, onOutput: onOutput, onSettled: onSettled}
}

func envInt(name string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func id() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

func (m *Manager) append(record *Record, stream, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record.LastActivityAt = time.Now().UnixMilli()
	buffer := &record.Stdout
	if stream == "stderr" {
		buffer = &record.Stderr
	}
	chunk := []byte(value)
	buffer.TotalBytes += int64(len(chunk))
	buffer.Data = append(buffer.Data, chunk...)
	if len(buffer.Data) > m.maxBufferBytes {
		start := len(buffer.Data) - m.maxBufferBytes
		for start < len(buffer.Data) && (buffer.Data[start]&0xc0) == 0x80 {
			start++
		}
		buffer.BaseOffset += int64(start)
		buffer.Data = append([]byte(nil), buffer.Data[start:]...)
	}
	if m.onOutput != nil {
		copyRecord := *record
		go m.onOutput(&copyRecord, stream, redactProcessSecrets(record, value))
	}
}

func readBuffer(buffer streamBuffer, cursor *int64) map[string]any {
	requested := buffer.BaseOffset
	if cursor != nil && *cursor > requested {
		requested = *cursor
	}
	relative := requested - buffer.BaseOffset
	if relative < 0 {
		relative = 0
	}
	if relative > int64(len(buffer.Data)) {
		relative = int64(len(buffer.Data))
	}
	for relative < int64(len(buffer.Data)) && (buffer.Data[relative]&0xc0) == 0x80 {
		relative++
	}
	return map[string]any{"text": string(buffer.Data[relative:]), "cursor": buffer.TotalBytes, "truncatedBeforeCursor": cursor != nil && *cursor < buffer.BaseOffset}
}

func (m *Manager) pruneLocked() error {
	if len(m.records) < m.maxProcesses {
		return nil
	}
	var oldest *Record
	for _, record := range m.records {
		if record.Status == StatusRunning {
			continue
		}
		if oldest == nil || record.StartedAt < oldest.StartedAt {
			oldest = record
		}
	}
	if oldest == nil {
		return fmt.Errorf("too many active CodeLocal processes (%d)", m.maxProcesses)
	}
	delete(m.records, oldest.ProcessID)
	return nil
}

func hostShell(command, cwd string) *exec.Cmd {
	return hostShellSandboxed(command, cwd, "", "")
}

// hostShellSandboxed builds the shell invocation, wrapping it in seatbelt when
// a profile is supplied. The profile is passed to sandbox-exec, which applies
// it to the shell and every descendant, so interpreters that construct a
// credential path at runtime are constrained identically to `cat`.
func hostShellSandboxed(command, cwd, profile, helper string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		shell := os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd.exe"
		}
		cmd := exec.Command(shell, "/d", "/s", "/c", command)
		cmd.Dir = cwd
		return cmd
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	if profile == "" || helper == "" {
		cmd := exec.Command(shell, "-c", command)
		cmd.Dir = cwd
		return cmd
	}
	cmd := exec.Command(helper, "-p", profile, shell, "-c", command)
	cmd.Dir = cwd
	return cmd
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, r := range key {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		if letter || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func explicitEnvAllowed(key string) bool {
	if !validEnvKey(key) {
		return false
	}
	upper := strings.ToUpper(key)
	if strings.HasPrefix(upper, "CODELOCAL_") {
		return false
	}
	switch upper {
	case "PATH", "HOME", "SHELL", "COMSPEC", "CI", "PAGER", "GIT_PAGER", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "NODE_OPTIONS", "PYTHONPATH", "BASH_ENV", "ENV", "PROMPT_COMMAND", "GIT_SSH_COMMAND", "SSH_AUTH_SOCK":
		return false
	default:
		return true
	}
}

// CanInjectEnvKey reports whether an explicitly requested runtime value may be
// injected into a child process without overriding CodeLocal's own execution
// boundary or high-risk process-loader variables.
func CanInjectEnvKey(key string) bool { return explicitEnvAllowed(key) }

func withEnv(cmd *exec.Cmd, explicit map[string]string) {
	values := map[string]string{}
	for _, entry := range security.SanitizeEnvironment(os.Environ()) {
		key, value, ok := strings.Cut(entry, "=")
		if ok && validEnvKey(key) {
			values[key] = value
		}
	}
	values["PAGER"], values["GIT_PAGER"], values["CODELOCAL_EXECUTION_MODE"] = "cat", "cat", "host-policy"
	if os.Getenv("CI") == "" {
		values["CI"] = "1"
	}
	for key, value := range explicit {
		if explicitEnvAllowed(key) {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	cmd.Env = make([]string, 0, len(keys))
	for _, key := range keys {
		cmd.Env = append(cmd.Env, key+"="+values[key])
	}
}

func (m *Manager) Start(command string, options StartOptions) (Snapshot, error) {
	m.mu.Lock()
	if err := m.pruneLocked(); err != nil {
		m.mu.Unlock()
		return Snapshot{}, err
	}
	now := time.Now().UnixMilli()
	executionMode := "host-policy"
	if options.SandboxProfile != "" && options.SandboxHelper != "" {
		executionMode = "host-policy+seatbelt"
	}
	record := &Record{ProcessID: id(), WorkspaceKey: m.workspaceKey, OwnerSessionID: options.OwnerSessionID, RequestID: options.RequestID, Command: command, CWD: options.CWD, DisplayCWD: options.DisplayCWD, StartedAt: now, LastActivityAt: now, Status: StatusRunning, ExecutionMode: executionMode, Sandboxed: options.SandboxProfile != "" && options.SandboxHelper != "", SandboxUnavailable: options.SandboxUnavailable, redactValues: append([]string(nil), options.RedactValues...)}
	m.records[record.ProcessID] = record
	if record.RequestID != "" {
		m.requestToProcess[record.RequestID] = record.ProcessID
	}
	m.mu.Unlock()

	ctx := context.Background()
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		record.cancel = cancel
		record.TimeoutAt = time.Now().Add(options.Timeout).UnixMilli()
	}
	cmd := hostShellSandboxed(command, options.CWD, options.SandboxProfile, options.SandboxHelper)
	withEnv(cmd, options.Env)
	if options.UsePTY {
		if handle, err := startPTY(cmd, options.Cols, options.Rows); err == nil && handle != nil {
			record.PTY = true
			record.pty = handle
			record.PID = cmd.Process.Pid
			go m.copyPTY(record, handle)
			go m.wait(record, ctx, cmd)
			return m.Snapshot(record.ProcessID, nil, nil)
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Snapshot{}, m.failStart(record, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Snapshot{}, m.failStart(record, err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Snapshot{}, m.failStart(record, err)
	}
	if err := cmd.Start(); err != nil {
		return Snapshot{}, m.failStart(record, err)
	}
	record.cmd, record.stdin, record.PID = cmd, stdin, cmd.Process.Pid
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		m.copyStream(record, "stdout", stdout)
	}()
	go func() {
		defer close(stderrDone)
		m.copyStream(record, "stderr", stderr)
	}()
	go m.wait(record, ctx, cmd, stdoutDone, stderrDone)
	return m.Snapshot(record.ProcessID, nil, nil)
}

func (m *Manager) failStart(record *Record, err error) error {
	m.mu.Lock()
	record.Status = StatusFailed
	code := -1
	record.ExitCode = &code
	m.mu.Unlock()
	m.append(record, "stderr", "\n[process error] "+err.Error()+"\n")
	m.settled(record)
	return err
}

func (m *Manager) copyStream(record *Record, stream string, source io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := source.Read(buf)
		if n > 0 {
			m.append(record, stream, string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) copyPTY(record *Record, handle ptyHandle) {
	buf := make([]byte, 32*1024)
	for {
		n, err := handle.Read(buf)
		if n > 0 {
			m.append(record, "stdout", string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) wait(record *Record, ctx context.Context, cmd *exec.Cmd, outputDone ...<-chan struct{}) {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		m.mu.Lock()
		if record.Status == StatusRunning {
			record.Status = StatusCancelled
		}
		m.mu.Unlock()
		_ = terminateProcess(cmd)
		err = <-done
	}
	// StdoutPipe and StderrPipe are drained by our own goroutines. cmd.Wait may
	// return before those goroutines have appended their final bytes, especially
	// on slower CI runners. Do not expose a settled snapshot until captured
	// output is complete; callers treat Running=false as the completion barrier.
	for _, drained := range outputDone {
		if drained != nil {
			<-drained
		}
	}
	m.mu.Lock()
	if record.Status == StatusRunning {
		if err != nil && !isExitError(err) {
			record.Status = StatusFailed
		} else {
			record.Status = StatusExited
		}
	}
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		record.ExitCode = &code
	}
	record.LastActivityAt = time.Now().UnixMilli()
	m.mu.Unlock()
	m.settled(record)
}

func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func (m *Manager) settled(record *Record) {
	m.mu.Lock()
	if record.RequestID != "" && m.requestToProcess[record.RequestID] == record.ProcessID {
		delete(m.requestToProcess, record.RequestID)
	}
	if record.cancel != nil {
		record.cancel()
		record.cancel = nil
	}
	copyRecord := *record
	m.mu.Unlock()
	if m.onSettled != nil {
		go m.onSettled(&copyRecord)
	}
}

func (m *Manager) displayCWD(record *Record) string {
	if record == nil {
		return "."
	}
	if display := strings.TrimSpace(record.DisplayCWD); display != "" {
		display = filepath.ToSlash(filepath.Clean(display))
		if display == "" || display == "./" {
			return "."
		}
		return display
	}
	rel, err := filepath.Rel(m.workspaceRoot, record.CWD)
	if err != nil || rel == "" || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func processPathAliases(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	seen := map[string]struct{}{}
	aliases := make([]string, 0, 5)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == "." {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		aliases = append(aliases, candidate)
	}
	clean := filepath.Clean(value)
	add(clean)
	add(filepath.ToSlash(clean))
	windowsSlash := strings.ReplaceAll(value, "\\", "/")
	add(windowsSlash)
	if len(windowsSlash) >= 3 && windowsSlash[1] == ':' && windowsSlash[2] == '/' {
		drive := windowsSlash[:1]
		rest := windowsSlash[2:]
		add("/" + strings.ToLower(drive) + rest)
		add("/" + strings.ToUpper(drive) + rest)
		lower := strings.ToLower(windowsSlash)
		if index := strings.Index(lower, "/appdata/local/temp/"); index >= 0 {
			relativeTempPath := strings.TrimPrefix(windowsSlash[index+len("/appdata/local/temp/"):], "/")
			if relativeTempPath != "" {
				add("/tmp/" + relativeTempPath)
			}
		}
	}
	return aliases
}

func redactProcessSecrets(record *Record, text string) string {
	if record == nil || text == "" {
		return text
	}
	for _, value := range record.redactValues {
		if value = strings.TrimSpace(value); len(value) >= 4 {
			text = strings.ReplaceAll(text, value, "[REDACTED]")
		}
	}
	return text
}

func sanitizeProcessOutput(record *Record, read map[string]any) map[string]any {
	if record == nil || read == nil {
		return read
	}
	text, _ := read["text"].(string)
	if text == "" {
		return read
	}
	if strings.TrimSpace(record.DisplayCWD) != "" && strings.TrimSpace(record.CWD) != "" {
		replacement := filepath.ToSlash(filepath.Clean(record.DisplayCWD))
		if replacement == "" || replacement == "./" {
			replacement = "."
		}
		for _, privatePath := range processPathAliases(record.CWD) {
			text = strings.ReplaceAll(text, privatePath, replacement)
		}
	}
	read["text"] = redactProcessSecrets(record, text)
	return read
}

func (m *Manager) Snapshot(processID string, stdoutCursor, stderrCursor *int64) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.records[processID]
	if record == nil {
		return Snapshot{}, errors.New("unknown processId")
	}
	return Snapshot{ProcessID: record.ProcessID, WorkspaceKey: record.WorkspaceKey, OwnerSessionID: record.OwnerSessionID, PID: record.PID, Command: security.RedactCommand(record.Command), CWD: m.displayCWD(record), StartedAt: record.StartedAt, LastActivityAt: record.LastActivityAt, Status: record.Status, Running: record.Status == StatusRunning, ExitCode: record.ExitCode, Signal: record.Signal, TimeoutAt: record.TimeoutAt, PTY: record.PTY, ExecutionMode: record.ExecutionMode, Sandboxed: record.Sandboxed, SandboxUnavailable: record.SandboxUnavailable, Stdout: sanitizeProcessOutput(record, readBuffer(record.Stdout, stdoutCursor)), Stderr: sanitizeProcessOutput(record, readBuffer(record.Stderr, stderrCursor))}, nil
}

func (m *Manager) List() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]any, 0, len(m.records))
	for _, record := range m.records {
		out = append(out, map[string]any{"processId": record.ProcessID, "pid": record.PID, "command": security.RedactCommand(record.Command), "cwd": m.displayCWD(record), "status": record.Status, "exitCode": record.ExitCode, "signal": record.Signal, "startedAt": record.StartedAt, "lastActivityAt": record.LastActivityAt, "pty": record.PTY, "executionMode": record.ExecutionMode, "sandboxed": record.Sandboxed, "sandboxUnavailable": record.SandboxUnavailable})
	}
	return out
}

func (m *Manager) Write(processID, input string) (map[string]any, error) {
	m.mu.Lock()
	record := m.records[processID]
	m.mu.Unlock()
	if record == nil || record.Status != StatusRunning {
		return nil, errors.New("process not running")
	}
	var err error
	if record.pty != nil {
		_, err = record.pty.Write([]byte(input))
	} else if record.stdin != nil {
		_, err = io.WriteString(record.stdin, input)
	} else {
		err = errors.New("process stdin unavailable")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"written": len([]byte(input))}, nil
}

func (m *Manager) Resize(processID string, cols, rows int) (map[string]any, error) {
	m.mu.Lock()
	record := m.records[processID]
	m.mu.Unlock()
	if record == nil || record.pty == nil {
		return nil, errors.New("PTY resize is not available for this process")
	}
	if err := record.pty.Resize(cols, rows); err != nil {
		return nil, err
	}
	return map[string]any{"resized": true, "cols": cols, "rows": rows}, nil
}

func (m *Manager) Signal(processID, signal string) (map[string]any, error) {
	m.mu.Lock()
	record := m.records[processID]
	m.mu.Unlock()
	if record == nil {
		return nil, errors.New("unknown processId")
	}
	if record.Status != StatusRunning {
		return map[string]any{"signalled": false, "status": record.Status}, nil
	}
	if signal == "" {
		signal = "SIGTERM"
	}
	if err := signalProcess(record.cmd, record.pty, signal); err != nil {
		return nil, err
	}
	m.mu.Lock()
	record.Signal = signal
	record.LastActivityAt = time.Now().UnixMilli()
	m.mu.Unlock()
	return map[string]any{"signalled": true, "signal": signal}, nil
}

func (m *Manager) Cancel(processID, reason string) (map[string]any, error) {
	m.mu.Lock()
	record := m.records[processID]
	if record == nil {
		m.mu.Unlock()
		return nil, errors.New("unknown processId")
	}
	if record.Status == StatusRunning {
		record.Status = StatusCancelled
	}
	m.mu.Unlock()
	if record.Status == StatusCancelled {
		m.append(record, "stderr", "\n[CodeLocal] "+reason+"\n")
		_ = signalProcess(record.cmd, record.pty, "SIGTERM")
	}
	return map[string]any{"cancelled": true, "processId": processID}, nil
}

func (m *Manager) CancelRequest(requestID, reason string) map[string]any {
	m.mu.Lock()
	processID := m.requestToProcess[requestID]
	m.mu.Unlock()
	if processID == "" {
		return map[string]any{"cancelled": false, "reason": "no process associated with request"}
	}
	result, err := m.Cancel(processID, reason)
	if err != nil {
		return map[string]any{"cancelled": false, "reason": err.Error()}
	}
	return result
}

func (m *Manager) StopAll(reason string) map[string]any {
	m.mu.Lock()
	ids := []string{}
	for id, record := range m.records {
		if record.Status == StatusRunning {
			ids = append(ids, id)
		}
	}
	m.requestToProcess = map[string]string{}
	m.mu.Unlock()
	for _, id := range ids {
		_, _ = m.Cancel(id, reason)
	}
	return map[string]any{"cancelled": len(ids)}
}

func Tail(data []byte, max int) string {
	if len(data) <= max {
		return string(data)
	}
	return string(bytes.Clone(data[len(data)-max:]))
}

func Redacted(command string) string { return security.RedactCommand(command) }
