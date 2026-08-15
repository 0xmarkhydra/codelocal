package workspace

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/osutil"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

type Workspace struct {
	WorkspaceID     string `json:"workspaceId"`
	WorkspaceName   string `json:"workspaceName"`
	LocalPath       string `json:"localPath"`
	GrantedAt       int64  `json:"grantedAt"`
	LastActivatedAt int64  `json:"lastActivatedAt,omitempty"`
}

type fileState struct {
	Version    int         `json:"version"`
	Workspaces []Workspace `json:"workspaces"`
}

type lockState struct {
	PID       int    `json:"pid"`
	Token     string `json:"token"`
	CreatedAt int64  `json:"createdAt"`
}

type Registry struct{ File string }

var slugRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func New() *Registry { return &Registry{File: filepath.Join(state.Dir(), "workspaces.json")} }

func IDForPath(project string) string {
	abs, _ := filepath.Abs(project)
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	base := strings.Trim(slugRE.ReplaceAllString(filepath.Base(abs), "-"), "-")
	if base == "" {
		base = "workspace"
	}
	if len(base) > 48 {
		base = base[:48]
	}
	digest := sha256.Sum256([]byte(abs))
	return base + "-" + hex.EncodeToString(digest[:])[:10]
}

func (r *Registry) read() (fileState, error) {
	var data fileState
	if err := state.ReadJSON(r.File, &data); err != nil {
		if os.IsNotExist(err) {
			return fileState{Version: 1}, nil
		}
		return fileState{}, err
	}
	data.Version = 1
	return data, nil
}

func randomToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func (r *Registry) withLock(fn func() error) error {
	if err := state.EnsurePrivateDir(filepath.Dir(r.File)); err != nil {
		return err
	}
	lockPath := r.File + ".lock"
	deadline := time.Now().Add(5 * time.Second)
	token := randomToken()
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			payload, _ := json.Marshal(lockState{PID: os.Getpid(), Token: token, CreatedAt: time.Now().UnixMilli()})
			_, _ = f.Write(append(payload, '\n'))
			_ = f.Sync()
			_ = f.Close()
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		var existing lockState
		readErr := state.ReadJSON(lockPath, &existing)
		stale := false
		if readErr != nil {
			// Another goroutine/process may have created the lock file but not
			// finished writing its JSON yet. Never delete a fresh, partially
			// written lock; only recover it after the same stale window used for
			// a parsed lock record.
			if info, statErr := os.Stat(lockPath); statErr == nil {
				stale = time.Since(info.ModTime()) > 30*time.Second
			} else if os.IsNotExist(statErr) {
				continue
			}
		} else {
			stale = !osutil.ProcessAlive(existing.PID) || time.Since(time.UnixMilli(existing.CreatedAt)) > 30*time.Second
		}
		if stale {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for the CodeLocal workspace registry lock")
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer func() {
		var current lockState
		if state.ReadJSON(lockPath, &current) == nil && current.Token == token {
			_ = os.Remove(lockPath)
		}
	}()
	return fn()
}

func unsafeBroadGrant(path string) bool {
	home, _ := os.UserHomeDir()
	volume := filepath.VolumeName(path)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	return path == root || path == home
}

func credentialDirectory(path string) bool {
	lower := "/" + strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/") + "/"
	for _, marker := range []string{"/.ssh/", "/.aws/", "/.gnupg/", "/.gcloud/", "/.azure/"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (r *Registry) List() ([]Workspace, error) {
	data, err := r.read()
	if err != nil {
		return nil, err
	}
	out := make([]Workspace, 0, len(data.Workspaces))
	for _, ws := range data.Workspaces {
		info, err := os.Stat(ws.LocalPath)
		if err == nil && info.IsDir() {
			out = append(out, ws)
		}
	}
	return out, nil
}

func (r *Registry) Grant(path, name string) (Workspace, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Workspace{}, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Workspace{}, err
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return Workspace{}, errors.New("only directories can be granted as CodeLocal workspaces")
	}
	if unsafeBroadGrant(real) {
		return Workspace{}, errors.New("refusing to grant the filesystem root or entire home directory")
	}
	if credentialDirectory(real) {
		return Workspace{}, errors.New("credential/config directories cannot be granted")
	}
	var out Workspace
	err = r.withLock(func() error {
		data, err := r.read()
		if err != nil {
			return err
		}
		id := IDForPath(real)
		grantedAt := time.Now().UnixMilli()
		var activated int64
		for _, ws := range data.Workspaces {
			if ws.WorkspaceID == id {
				grantedAt = ws.GrantedAt
				activated = ws.LastActivatedAt
			}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = filepath.Base(real)
		}
		if len(name) > 120 {
			name = name[:120]
		}
		out = Workspace{WorkspaceID: id, WorkspaceName: name, LocalPath: real, GrantedAt: grantedAt, LastActivatedAt: activated}
		next := make([]Workspace, 0, len(data.Workspaces)+1)
		for _, ws := range data.Workspaces {
			if ws.WorkspaceID != id {
				next = append(next, ws)
			}
		}
		next = append(next, out)
		sort.Slice(next, func(i, j int) bool { return next[i].WorkspaceName < next[j].WorkspaceName })
		return state.WriteJSONAtomic(r.File, fileState{Version: 1, Workspaces: next})
	})
	return out, err
}

func (r *Registry) Revoke(identifier string) (bool, error) {
	resolved := ""
	if abs, err := filepath.Abs(identifier); err == nil {
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			resolved = real
		}
	}
	removed := false
	err := r.withLock(func() error {
		data, err := r.read()
		if err != nil {
			return err
		}
		next := make([]Workspace, 0, len(data.Workspaces))
		for _, ws := range data.Workspaces {
			if ws.WorkspaceID == identifier || (resolved != "" && ws.LocalPath == resolved) {
				removed = true
				continue
			}
			next = append(next, ws)
		}
		if !removed {
			return nil
		}
		return state.WriteJSONAtomic(r.File, fileState{Version: 1, Workspaces: next})
	})
	return removed, err
}

func (r *Registry) Get(id string) (*Workspace, error) {
	items, err := r.List()
	if err != nil {
		return nil, err
	}
	for _, ws := range items {
		if ws.WorkspaceID == id {
			copy := ws
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *Registry) MarkActivated(id string) error {
	return r.withLock(func() error {
		data, err := r.read()
		if err != nil {
			return err
		}
		for i := range data.Workspaces {
			if data.Workspaces[i].WorkspaceID == id {
				data.Workspaces[i].LastActivatedAt = time.Now().UnixMilli()
				return state.WriteJSONAtomic(r.File, data)
			}
		}
		return nil
	})
}
