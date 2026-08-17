package repository

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

// Checkout is one concrete repository checkout inside an authorized workspace.
// Repository ID is logical; Root is local-only execution state.
type Checkout struct {
	ID             string `json:"id"`
	RelativePath   string `json:"relativePath"`
	Root           string `json:"-"`
	Remote         string `json:"remote,omitempty"`
	Lineage        string `json:"lineage,omitempty"`
	IdentitySource string `json:"identitySource"`
}

type Registry struct {
	Root      string
	checkouts []Checkout
}

func New(root, workspaceName string) *Registry {
	return FromSnapshot(root, projectidentity.Discover(root, workspaceName))
}

func FromSnapshot(root string, snapshot projectidentity.Snapshot) *Registry {
	root = filepath.Clean(root)
	checkouts := make([]Checkout, 0, len(snapshot.Repositories))
	for _, repo := range snapshot.Repositories {
		rel := filepath.ToSlash(filepath.Clean(repo.RelativePath))
		if rel == "" || rel == "./" {
			rel = "."
		}
		checkoutRoot := root
		if rel != "." {
			checkoutRoot = filepath.Join(root, filepath.FromSlash(rel))
		}
		checkouts = append(checkouts, Checkout{
			ID: repo.ID, RelativePath: rel, Root: checkoutRoot,
			Remote: repo.Remote, Lineage: repo.Lineage, IdentitySource: repo.IdentitySource,
		})
	}
	sort.Slice(checkouts, func(i, j int) bool {
		if checkouts[i].RelativePath != checkouts[j].RelativePath {
			return checkouts[i].RelativePath < checkouts[j].RelativePath
		}
		return checkouts[i].ID < checkouts[j].ID
	})
	return &Registry{Root: root, checkouts: checkouts}
}

func (r *Registry) All() []Checkout {
	return append([]Checkout(nil), r.checkouts...)
}

func (r *Registry) IsMulti() bool { return len(r.checkouts) > 1 }

func (r *Registry) ResolveSelector(selector string) (Checkout, error) {
	selector = strings.TrimSpace(filepath.ToSlash(selector))
	if selector == "" {
		if len(r.checkouts) == 1 {
			return r.checkouts[0], nil
		}
		if len(r.checkouts) == 0 {
			return Checkout{}, errors.New("workspace contains no discovered Git repository")
		}
		return Checkout{}, errors.New("workspace contains multiple Git repositories; specify repository or path")
	}
	selector = strings.TrimSuffix(selector, "/")
	for _, repo := range r.checkouts {
		if selector == repo.ID || selector == repo.RelativePath {
			return repo, nil
		}
	}
	return Checkout{}, errors.New("repository not found in authorized workspace")
}

func cleanWorkspacePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ".", nil
	}
	if filepath.IsAbs(value) {
		return "", errors.New("absolute repository paths are not allowed")
	}
	clean := filepath.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("repository path escapes the authorized workspace")
	}
	return filepath.ToSlash(clean), nil
}

// ResolvePath returns the deepest repository checkout owning a workspace-relative
// path and the same path rewritten relative to that repository root.
func (r *Registry) ResolvePath(workspacePath string) (Checkout, string, error) {
	clean, err := cleanWorkspacePath(workspacePath)
	if err != nil {
		return Checkout{}, "", err
	}
	best := Checkout{}
	bestDepth := -1
	for _, repo := range r.checkouts {
		rel := repo.RelativePath
		matches := rel == "." || clean == rel || strings.HasPrefix(clean, rel+"/")
		if !matches {
			continue
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, "/") + 1
		}
		if depth > bestDepth {
			best = repo
			bestDepth = depth
		}
	}
	if bestDepth < 0 {
		return Checkout{}, "", errors.New("path does not belong to a discovered Git repository")
	}
	repoPath := clean
	if best.RelativePath != "." {
		if clean == best.RelativePath {
			repoPath = "."
		} else {
			repoPath = strings.TrimPrefix(clean, best.RelativePath+"/")
		}
	}
	return best, repoPath, nil
}
