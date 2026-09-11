package cloud

import (
	"context"
	"net/url"
	"path"
	"strings"
)

type WorkspaceRepositorySource struct {
	RepositoryID string `json:"repositoryId"`
	Remote       string `json:"remote"`
	RelativePath string `json:"relativePath"`
}

// WorkspaceRepositorySources returns repository-backed durable sources already
// learned from the local runtime. It deliberately rejects credential-bearing or
// non-HTTP remotes: managed Git credentials must be injected separately rather
// than persisted inside a remote URL or sandbox environment.
func (s *Store) WorkspaceRepositorySources(ctx context.Context, userID, deviceID, workspaceID string) ([]WorkspaceRepositorySource, error) {
	rows, err := s.DB.Query(ctx, `
SELECT wr.repository_id,COALESCE(r.remote,''),wr.relative_path
FROM codelocal_workspace_repositories wr
JOIN codelocal_repositories r ON r.user_id=wr.user_id AND r.repository_id=wr.repository_id
WHERE wr.user_id=$1 AND wr.device_id=$2 AND wr.workspace_id=$3
ORDER BY wr.relative_path ASC,wr.repository_id ASC`, strings.TrimSpace(userID), strings.TrimSpace(deviceID), strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkspaceRepositorySource{}
	for rows.Next() {
		var source WorkspaceRepositorySource
		if err := rows.Scan(&source.RepositoryID, &source.Remote, &source.RelativePath); err != nil {
			return nil, err
		}
		source.Remote = safeCloudRepositoryRemote(source.Remote)
		source.RelativePath = safeCloudRepositoryPath(source.RelativePath)
		if source.Remote == "" || source.RelativePath == "" {
			continue
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func safeCloudRepositoryRemote(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func safeCloudRepositoryPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || value == "." {
		return "."
	}
	clean := path.Clean(value)
	if clean == "." {
		return "."
	}
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || len(clean) > 500 {
		return ""
	}
	return clean
}
