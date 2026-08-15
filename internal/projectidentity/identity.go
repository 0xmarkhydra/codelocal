package projectidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const maxRepositoryDepth = 6

type Repository struct {
	ID             string `json:"id"`
	Remote         string `json:"remote,omitempty"`
	Lineage        string `json:"lineage,omitempty"`
	RelativePath   string `json:"relativePath,omitempty"`
	IdentitySource string `json:"identitySource"`
}

type Snapshot struct {
	SuggestedName   string       `json:"suggestedName"`
	MarkerProjectID string       `json:"markerProjectId,omitempty"`
	Repositories    []Repository `json:"repositories,omitempty"`
}

type markerFile struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])[:32]
}

func NormalizeRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		if at := strings.LastIndex(raw, "@"); at >= 0 {
			raw = raw[at+1:]
		}
		if colon := strings.Index(raw, ":"); colon > 0 {
			host := strings.ToLower(strings.TrimSpace(raw[:colon]))
			path := strings.Trim(strings.TrimSpace(raw[colon+1:]), "/")
			path = strings.TrimSuffix(path, ".git")
			if host != "" && path != "" && !strings.Contains(host, "/") {
				return host + "/" + path
			}
		}
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" {
		host += ":" + port
	}
	path := strings.Trim(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	if path == "" {
		return ""
	}
	return host + "/" + path
}

func gitOutput(root string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func repositoryAt(workspaceRoot, repoRoot string) (Repository, bool) {
	remote := NormalizeRemote(gitOutput(repoRoot, "config", "--get", "remote.origin.url"))
	roots := strings.Fields(gitOutput(repoRoot, "rev-list", "--max-parents=0", "HEAD"))
	sort.Strings(roots)
	lineage := ""
	if len(roots) > 0 {
		lineage = strings.ToLower(strings.TrimSpace(roots[0]))
	}
	id := ""
	source := ""
	if remote != "" {
		id = digest("remote", remote)
		source = "remote"
	} else if lineage != "" {
		id = digest("lineage", lineage)
		source = "lineage"
	}
	if id == "" {
		return Repository{}, false
	}
	rel, err := filepath.Rel(workspaceRoot, repoRoot)
	if err != nil {
		rel = "."
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		rel = "."
	}
	return Repository{ID: id, Remote: remote, Lineage: lineage, RelativePath: rel, IdentitySource: source}, true
}

func ignoredDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "vendor", ".venv", "venv", "dist", "build", ".next", ".dart_tool", "target", "coverage":
		return true
	default:
		return false
	}
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

func discoverRepositoryRoots(root string) []string {
	seen := map[string]struct{}{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		if entry.IsDir() && depth(root, path) > maxRepositoryDepth {
			return filepath.SkipDir
		}
		name := entry.Name()
		if entry.IsDir() && ignoredDirectory(name) {
			if name == ".git" {
				seen[filepath.Dir(path)] = struct{}{}
			}
			return filepath.SkipDir
		}
		if !entry.IsDir() && name == ".git" {
			seen[filepath.Dir(path)] = struct{}{}
		}
		return nil
	})
	roots := make([]string, 0, len(seen))
	for path := range seen {
		roots = append(roots, path)
	}
	sort.Strings(roots)
	return roots
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func readMarker(root string) markerFile {
	data, err := os.ReadFile(filepath.Join(root, ".codelocal", "project.json"))
	if err != nil || len(data) > 16<<10 {
		return markerFile{}
	}
	var marker markerFile
	if json.Unmarshal(data, &marker) != nil {
		return markerFile{}
	}
	marker.ProjectID = strings.TrimSpace(marker.ProjectID)
	marker.Name = strings.TrimSpace(marker.Name)
	if len(marker.ProjectID) > 128 {
		marker.ProjectID = ""
	}
	marker.Name = truncateRunes(marker.Name, 120)
	return marker
}

func Discover(root, workspaceName string) Snapshot {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		root = filepath.Clean(root)
	}
	marker := readMarker(root)
	name := strings.TrimSpace(marker.Name)
	if name == "" {
		name = strings.TrimSpace(workspaceName)
	}
	if name == "" {
		name = filepath.Base(root)
	}
	name = truncateRunes(name, 120)
	out := Snapshot{SuggestedName: name, MarkerProjectID: marker.ProjectID}
	seen := map[string]struct{}{}
	for _, repoRoot := range discoverRepositoryRoots(root) {
		repo, ok := repositoryAt(root, repoRoot)
		if !ok {
			continue
		}
		if _, exists := seen[repo.ID]; exists {
			continue
		}
		seen[repo.ID] = struct{}{}
		out.Repositories = append(out.Repositories, repo)
	}
	sort.Slice(out.Repositories, func(i, j int) bool {
		if out.Repositories[i].RelativePath != out.Repositories[j].RelativePath {
			return out.Repositories[i].RelativePath < out.Repositories[j].RelativePath
		}
		return out.Repositories[i].ID < out.Repositories[j].ID
	})
	return out
}
