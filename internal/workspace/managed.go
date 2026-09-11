package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

var managedWorkspaceIDRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)

// GrantManaged authorizes a workspace using an explicit durable workspace ID.
// Cloud runtimes use this instead of IDForPath because every sandbox mounts the
// user workspace at the same path (normally /workspace). The ID is supplied by
// the control plane and must remain stable across ephemeral compute sessions.
func (r *Registry) GrantManaged(id, path, name string) (Workspace, error) {
	id = strings.TrimSpace(id)
	if !managedWorkspaceIDRE.MatchString(id) {
		return Workspace{}, errors.New("invalid managed workspace ID")
	}
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

	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(real)
	}
	if len(name) > 120 {
		name = name[:120]
	}

	var out Workspace
	err = r.withLock(func() error {
		data, err := r.read()
		if err != nil {
			return err
		}
		grantedAt := time.Now().UnixMilli()
		var activated int64
		for _, ws := range data.Workspaces {
			if ws.WorkspaceID == id {
				grantedAt = ws.GrantedAt
				activated = ws.LastActivatedAt
			}
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
