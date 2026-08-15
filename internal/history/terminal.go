package history

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	processmgr "github.com/0xmarkhydra/codelocal/internal/process"
	"github.com/0xmarkhydra/codelocal/internal/security"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

type Record struct {
	TS            string   `json:"ts"`
	Event         string   `json:"event"`
	WorkspaceKey  string   `json:"workspaceKey"`
	ProcessID     string   `json:"processId"`
	RequestID     string   `json:"requestId,omitempty"`
	SessionID     string   `json:"sessionId,omitempty"`
	CWD           string   `json:"cwd"`
	Command       string   `json:"command"`
	RiskLevel     string   `json:"riskLevel"`
	MatchedRules  []string `json:"matchedRules"`
	Approval      string   `json:"approval"`
	StartedAt     int64    `json:"startedAt"`
	FinishedAt    int64    `json:"finishedAt,omitempty"`
	DurationMs    int64    `json:"durationMs,omitempty"`
	ExitCode      *int     `json:"exitCode,omitempty"`
	Status        string   `json:"status,omitempty"`
	ExecutionMode string   `json:"executionMode,omitempty"`
}

type StartInput struct {
	WorkspaceKey  string
	ProcessID     string
	RequestID     string
	SessionID     string
	CWD           string
	Command       string
	RiskLevel     string
	MatchedRules  []string
	Approval      string
	StartedAt     int64
	ExecutionMode string
}

type Terminal struct {
	mu     sync.Mutex
	file   string
	starts map[string]Record
}

func New() *Terminal {
	file := os.Getenv("CODELOCAL_TERMINAL_HISTORY_PATH")
	if file == "" {
		file = filepath.Join(state.Dir(), "terminal-history.jsonl")
	}
	return &Terminal{file: file, starts: map[string]Record{}}
}

func (t *Terminal) Started(input StartInput) (Record, error) {
	command := security.RedactCommand(input.Command)
	if len(command) > 4000 {
		command = command[:4000]
	}
	record := Record{TS: time.Now().UTC().Format(time.RFC3339Nano), Event: "started", WorkspaceKey: input.WorkspaceKey, ProcessID: input.ProcessID, RequestID: input.RequestID, SessionID: input.SessionID, CWD: input.CWD, Command: command, RiskLevel: input.RiskLevel, MatchedRules: append([]string(nil), input.MatchedRules...), Approval: input.Approval, StartedAt: input.StartedAt, ExecutionMode: input.ExecutionMode}
	t.mu.Lock()
	t.starts[record.ProcessID] = record
	t.mu.Unlock()
	return record, state.AppendJSONL(t.file, record)
}

func (t *Terminal) Finished(record *processmgr.Record) (Record, error) {
	t.mu.Lock()
	start, ok := t.starts[record.ProcessID]
	delete(t.starts, record.ProcessID)
	t.mu.Unlock()
	finishedAt := time.Now().UnixMilli()
	if !ok {
		command := security.RedactCommand(record.Command)
		if len(command) > 4000 {
			command = command[:4000]
		}
		start = Record{WorkspaceKey: record.WorkspaceKey, ProcessID: record.ProcessID, CWD: record.CWD, Command: command, RiskLevel: "UNKNOWN", MatchedRules: []string{}, Approval: "automatic", StartedAt: record.StartedAt}
	}
	out := Record{TS: time.Now().UTC().Format(time.RFC3339Nano), Event: "finished", WorkspaceKey: record.WorkspaceKey, ProcessID: record.ProcessID, RequestID: start.RequestID, SessionID: start.SessionID, CWD: start.CWD, Command: start.Command, RiskLevel: start.RiskLevel, MatchedRules: start.MatchedRules, Approval: start.Approval, StartedAt: record.StartedAt, FinishedAt: finishedAt, DurationMs: max64(0, finishedAt-record.StartedAt), ExitCode: record.ExitCode, Status: string(record.Status), ExecutionMode: record.ExecutionMode}
	return out, state.AppendJSONL(t.file, out)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func tail(path string, maxBytes int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	length := stat.Size()
	truncated := false
	start := int64(0)
	if length > maxBytes {
		start = length - maxBytes
		length = maxBytes
		truncated = true
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, length))
	if err != nil {
		return nil, false, err
	}
	if truncated {
		if idx := strings.IndexByte(string(data), '\n'); idx >= 0 {
			data = data[idx+1:]
		}
	}
	return data, truncated, nil
}

func (t *Terminal) Query(workspaceKey, query, event string, limit int) (map[string]any, error) {
	if event == "" {
		event = "started"
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	data, truncated, err := tail(t.file, 4*1024*1024)
	if err != nil {
		return nil, err
	}
	lines := []string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	records := []Record{}
	for i := len(lines) - 1; i >= 0 && len(records) < limit; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		var record Record
		if json.Unmarshal([]byte(lines[i]), &record) != nil {
			continue
		}
		if workspaceKey != "" && record.WorkspaceKey != workspaceKey {
			continue
		}
		if event != "all" && record.Event != event {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(record.Command + " " + record.CWD + " " + record.RiskLevel + " " + record.Status)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		records = append(records, record)
	}
	return map[string]any{"records": records, "truncatedByReadWindow": truncated}, nil
}
