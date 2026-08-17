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
	SuggestedName       string       `json:"suggestedName"`
	MarkerPresent       bool         `json:"markerPresent,omitempty"`
	MarkerSchemaVersion int          `json:"markerSchemaVersion,omitempty"`
	MarkerProjectID     string       `json:"markerProjectId,omitempty"`
	MarkerName          string       `json:"markerName,omitempty"`
	MarkerValid         bool         `json:"markerValid,omitempty"`
	MarkerReason        string       `json:"markerReason,omitempty"`
	Repositories        []Repository `json:"repositories,omitempty"`
}

type markerFile struct {
	SchemaVersion int    `json:"schemaVersion"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
}

type markerObservation struct {
	Present bool
	Marker  markerFile
	Valid   bool
	Reason  string
}

// StableProjectID is the portable local project identity used before Cloud
// alias resolution. A validated marker wins; otherwise the sorted repository
// set produces the same identity across workspaces that observe the same repos.
func StableProjectID(snapshot Snapshot) string {
	if snapshot.MarkerValid && snapshot.MarkerSchemaVersion == 1 {
		if marker := strings.TrimSpace(snapshot.MarkerProjectID); marker != "" {
			return marker
		}
	}
	ids := make([]string, 0, len(snapshot.Repositories))
	for _, repository := range snapshot.Repositories {
		if id := strings.TrimSpace(repository.ID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(ids, "\x00")))
	return "reposet_" + hex.EncodeToString(sum[:12])
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
	rel, err := filepath.Rel(workspaceRoot, repoRoot)
	if err != nil {
		rel = "."
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		rel = "."
	}
	id := ""
	source := ""
	if remote != "" {
		id = digest("remote", remote)
		source = "remote"
	} else if lineage != "" {
		id = digest("lineage", lineage)
		source = "lineage"
	} else {
		// A freshly initialized repository may have neither a remote nor a first
		// commit yet. Keep it routable locally without leaking the absolute path;
		// once remote/lineage evidence appears, the stronger identity takes over.
		id = digest("local-checkout", filepath.Clean(workspaceRoot), rel)
		source = "local_checkout"
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

func validMarkerProjectID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 4 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func readMarker(root string) markerObservation {
	data, err := os.ReadFile(filepath.Join(root, ".codelocal", "project.json"))
	if os.IsNotExist(err) {
		return markerObservation{}
	}
	if err != nil {
		return markerObservation{Present: true, Reason: "read_error"}
	}
	if len(data) > 16<<10 {
		return markerObservation{Present: true, Reason: "too_large"}
	}
	var marker markerFile
	if json.Unmarshal(data, &marker) != nil {
		return markerObservation{Present: true, Reason: "invalid_json"}
	}
	marker.ProjectID = strings.TrimSpace(marker.ProjectID)
	marker.Name = truncateRunes(strings.TrimSpace(marker.Name), 120)
	if marker.SchemaVersion != 1 {
		return markerObservation{Present: true, Marker: marker, Reason: "unsupported_schema_version"}
	}
	if !validMarkerProjectID(marker.ProjectID) {
		return markerObservation{Present: true, Marker: marker, Reason: "invalid_project_id"}
	}
	if marker.Name == "" {
		return markerObservation{Present: true, Marker: marker, Reason: "missing_name"}
	}
	return markerObservation{Present: true, Marker: marker, Valid: true}
}

func Discover(root, workspaceName string) Snapshot {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		root = filepath.Clean(root)
	}
	marker := readMarker(root)
	name := ""
	if marker.Valid {
		name = strings.TrimSpace(marker.Marker.Name)
	}
	if name == "" {
		name = strings.TrimSpace(workspaceName)
	}
	if name == "" {
		name = filepath.Base(root)
	}
	name = truncateRunes(name, 120)
	out := Snapshot{
		SuggestedName:       name,
		MarkerPresent:       marker.Present,
		MarkerSchemaVersion: marker.Marker.SchemaVersion,
		MarkerValid:         marker.Valid,
		MarkerReason:        marker.Reason,
	}
	if marker.Valid {
		out.MarkerProjectID = marker.Marker.ProjectID
		out.MarkerName = marker.Marker.Name
	}
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
