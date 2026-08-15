package runtimecontrol

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/osutil"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

type LeaseRecord struct {
	PID        int    `json:"pid"`
	InstanceID string `json:"instanceId"`
	StartedAt  int64  `json:"startedAt"`
	Version    string `json:"version"`
	Endpoint   string `json:"endpoint"`
}

type Lease struct {
	Acquired bool
	Record   LeaseRecord
	release  func() error
}

func (l Lease) Release() error {
	if l.release == nil {
		return nil
	}
	return l.release()
}

type Command struct {
	Type string `json:"type"`
}

type Handler func(context.Context, Command) (any, error)

func lockPath(dir string) string { return filepath.Join(dir, "runtime.lock") }

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func readLease(dir string) (*LeaseRecord, error) {
	var record LeaseRecord
	if err := state.ReadJSON(lockPath(dir), &record); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !osutil.ProcessAlive(record.PID) {
		return nil, nil
	}
	return &record, nil
}

func sendToRecord(ctx context.Context, record LeaseRecord, command Command) (any, error) {
	conn, err := dialControl(ctx, record.Endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
	if err := json.NewEncoder(conn).Encode(command); err != nil {
		return nil, err
	}
	var envelope struct {
		OK         bool   `json:"ok"`
		InstanceID string `json:"instanceId"`
		Result     any    `json:"result"`
		Error      string `json:"error"`
	}
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&envelope); err != nil {
		return nil, err
	}
	if envelope.InstanceID != record.InstanceID {
		return nil, errors.New("runtime control identity mismatch")
	}
	if !envelope.OK {
		if envelope.Error == "" {
			envelope.Error = "runtime control command failed"
		}
		return nil, errors.New(envelope.Error)
	}
	return envelope.Result, nil
}

func Acquire(version, dir string) (Lease, error) {
	if dir == "" {
		dir = state.Dir()
	}
	if err := state.EnsurePrivateDir(dir); err != nil {
		return Lease{}, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record := LeaseRecord{PID: os.Getpid(), InstanceID: randomID(), StartedAt: time.Now().UnixMilli(), Version: version, Endpoint: Endpoint(dir)}
		file, err := os.OpenFile(lockPath(dir), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if encodeErr := json.NewEncoder(file).Encode(record); encodeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath(dir))
				return Lease{}, encodeErr
			}
			_ = file.Sync()
			_ = file.Close()
			return Lease{Acquired: true, Record: record, release: func() error {
				var current LeaseRecord
				if state.ReadJSON(lockPath(dir), &current) == nil && current.InstanceID == record.InstanceID {
					_ = os.Remove(lockPath(dir))
				}
				return nil
			}}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return Lease{}, err
		}
		existing, readErr := readLease(dir)
		if readErr != nil {
			return Lease{}, readErr
		}
		if existing == nil {
			_ = os.Remove(lockPath(dir))
			_ = removeEndpoint(Endpoint(dir))
			continue
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
		_, liveErr := sendToRecord(probeCtx, *existing, Command{Type: "status"})
		cancel()
		if liveErr == nil {
			return Lease{Acquired: false, Record: *existing}, nil
		}
		if time.Since(time.UnixMilli(existing.StartedAt)) < 1500*time.Millisecond {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if !endpointExists(existing.Endpoint) {
			_ = os.Remove(lockPath(dir))
			_ = removeEndpoint(existing.Endpoint)
			continue
		}
		return Lease{}, fmt.Errorf("CodeLocal runtime PID %d is alive but its control channel is unresponsive; refusing to start a duplicate runtime", existing.PID)
	}
	return Lease{}, errors.New("unable to acquire the CodeLocal runtime lock")
}

type Server struct {
	listener net.Listener
	endpoint string
}

func Start(ctx context.Context, dir, instanceID string, handler Handler) (*Server, error) {
	if dir == "" {
		dir = state.Dir()
	}
	if err := state.EnsurePrivateDir(dir); err != nil {
		return nil, err
	}
	endpoint := Endpoint(dir)
	_ = removeEndpoint(endpoint)
	ln, err := listenControl(endpoint)
	if err != nil {
		return nil, err
	}
	_ = secureEndpoint(endpoint)
	s := &Server{listener: ln, endpoint: endpoint}
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleConn(ctx, conn, instanceID, handler)
		}
	}()
	return s, nil
}

func handleConn(ctx context.Context, conn net.Conn, instanceID string, handler Handler) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	var cmd Command
	if err := json.NewDecoder(io.LimitReader(conn, 64<<10)).Decode(&cmd); err != nil {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "invalid_runtime_control_command", "instanceId": instanceID})
		return
	}
	if cmd.Type != "status" && cmd.Type != "reload" && cmd.Type != "shutdown" {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "invalid_runtime_control_command", "instanceId": instanceID})
		return
	}
	result, err := handler(ctx, cmd)
	if err != nil {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": err.Error(), "instanceId": instanceID})
		return
	}
	_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "result": result, "instanceId": instanceID})
}

func (s *Server) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	err := s.listener.Close()
	_ = removeEndpoint(s.endpoint)
	return err
}

func Send(ctx context.Context, dir, command string) (any, error) {
	if dir == "" {
		dir = state.Dir()
	}
	record, err := readLease(dir)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, os.ErrNotExist
	}
	return sendToRecord(ctx, *record, Command{Type: command})
}

func Summary(ctx context.Context, dir string) map[string]any {
	if dir == "" {
		dir = state.Dir()
	}
	record, err := readLease(dir)
	if err != nil || record == nil {
		return map[string]any{"running": false}
	}
	detail, liveErr := sendToRecord(ctx, *record, Command{Type: "status"})
	result := map[string]any{
		"running":    true,
		"pid":        record.PID,
		"version":    record.Version,
		"startedAt":  record.StartedAt,
		"responsive": liveErr == nil,
	}
	if detail != nil {
		result["detail"] = detail
	}
	return result
}
