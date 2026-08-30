package cloudserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
)

const (
	maxSkillArchiveBytes = 8 << 20
	maxSkillSourceBytes  = 2 << 20
	maxSkillRedirects    = 5
)

var (
	htmlScriptStylePattern = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</\1\s*>`)
	htmlTagPattern         = regexp.MustCompile(`(?s)<[^>]+>`)
)

func sourceDocumentsFromZIPBase64(encoded string) ([]skillintel.SourceDocument, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP payload")
	}
	return sourceDocumentsFromZIP(payload, "", false)
}

func sourceDocumentsFromURL(ctx context.Context, rawURL string) ([]skillintel.SourceDocument, string, error) {
	parsed, err := validateSkillSourceURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	if strings.EqualFold(parsed.Hostname(), "github.com") {
		if documents, resolved, handled, githubErr := sourceDocumentsFromGitHub(ctx, parsed); handled {
			return documents, resolved, githubErr
		}
	}
	payload, contentType, err := fetchSkillSource(ctx, parsed.String(), maxSkillSourceBytes)
	if err != nil {
		return nil, "", err
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		name = "source.md"
	}
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		content := htmlKnowledgeText(payload)
		if content == "" {
			return nil, "", fmt.Errorf("HTML source produced no readable knowledge")
		}
		return []skillintel.SourceDocument{{Path: strings.TrimSuffix(name, path.Ext(name)) + ".md", Content: content}}, parsed.String(), nil
	}
	if path.Ext(name) == "" {
		name += ".md"
	}
	if !utf8.Valid(payload) {
		return nil, "", fmt.Errorf("Skill source is not valid UTF-8 text")
	}
	return []skillintel.SourceDocument{{Path: name, Content: string(payload)}}, parsed.String(), nil
}

func sourceDocumentsFromGitHub(ctx context.Context, source *url.URL) ([]skillintel.SourceDocument, string, bool, error) {
	segments := splitURLPath(source.Path)
	if len(segments) < 2 {
		return nil, "", false, nil
	}
	owner, repo := segments[0], strings.TrimSuffix(segments[1], ".git")
	if owner == "" || repo == "" {
		return nil, "", false, nil
	}
	if len(segments) >= 5 && segments[2] == "blob" {
		ref := segments[3]
		filePath := strings.Join(segments[4:], "/")
		raw := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref), escapeURLPath(filePath))
		payload, _, err := fetchSkillSource(ctx, raw, maxSkillSourceBytes)
		if err != nil {
			return nil, source.String(), true, err
		}
		if !utf8.Valid(payload) {
			return nil, source.String(), true, fmt.Errorf("GitHub document is not valid UTF-8 text")
		}
		return []skillintel.SourceDocument{{Path: filePath, Content: string(payload)}}, source.String(), true, nil
	}
	if len(segments) > 2 && segments[2] != "tree" {
		return nil, "", false, nil
	}

	ref, subpath, err := resolveGitHubRef(ctx, owner, repo, segments)
	if err != nil {
		return nil, source.String(), true, err
	}
	archiveURL := fmt.Sprintf("https://codeload.github.com/%s/%s/zip/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))
	archive, _, err := fetchSkillSource(ctx, archiveURL, maxSkillArchiveBytes)
	if err != nil {
		return nil, source.String(), true, fmt.Errorf("download GitHub source snapshot: %w", err)
	}
	documents, err := sourceDocumentsFromZIP(archive, subpath, true)
	if err != nil {
		return nil, source.String(), true, err
	}
	return documents, source.String(), true, nil
}

func resolveGitHubRef(ctx context.Context, owner, repo string, segments []string) (string, string, error) {
	if len(segments) >= 4 && segments[2] == "tree" {
		ref := strings.TrimSpace(segments[3])
		subpath := ""
		if len(segments) > 4 {
			subpath = strings.Join(segments[4:], "/")
		}
		return ref, subpath, nil
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	metadata, _, err := fetchSkillSource(ctx, apiURL, 256<<10)
	if err != nil {
		return "", "", fmt.Errorf("resolve GitHub repository: %w", err)
	}
	var repository struct{ DefaultBranch string `json:"default_branch"` }
	if json.Unmarshal(metadata, &repository) != nil || strings.TrimSpace(repository.DefaultBranch) == "" {
		return "", "", fmt.Errorf("resolve GitHub default branch")
	}
	return repository.DefaultBranch, "", nil
}

func sourceDocumentsFromZIP(payload []byte, subpath string, stripRoot bool) ([]skillintel.SourceDocument, error) {
	if len(payload) == 0 || len(payload) > maxSkillArchiveBytes {
		return nil, fmt.Errorf("ZIP archive must be between 1 byte and %d bytes", maxSkillArchiveBytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP archive")
	}
	policy := skillintel.DefaultKnowledgeIngestPolicy()
	if len(reader.File) > policy.MaxDocuments*2 {
		return nil, fmt.Errorf("ZIP archive contains too many entries")
	}
	cleanSubpath := strings.Trim(path.Clean(subpath), "/")
	if subpath == "" {
		cleanSubpath = ""
	}
	documents := make([]skillintel.SourceDocument, 0, len(reader.File))
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		relative, ok, err := archiveRelativePath(entry.Name, cleanSubpath, stripRoot)
		if err != nil {
			return nil, err
		}
		if !ok || !supportedKnowledgePath(relative, policy) {
			continue
		}
		if entry.UncompressedSize64 > uint64(policy.MaxDocumentBytes) {
			return nil, fmt.Errorf("ZIP document %q exceeds %d bytes", relative, policy.MaxDocumentBytes)
		}
		content, err := readZIPDocument(entry, policy.MaxDocumentBytes)
		if err != nil {
			return nil, err
		}
		total += int64(len(content))
		if total > maxSkillArchiveBytes {
			return nil, fmt.Errorf("ZIP expanded content exceeds %d bytes", maxSkillArchiveBytes)
		}
		documents = append(documents, skillintel.SourceDocument{Path: relative, Content: content})
		if len(documents) > policy.MaxDocuments {
			return nil, fmt.Errorf("ZIP archive contains too many knowledge documents")
		}
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("ZIP archive contained no supported knowledge documents")
	}
	return documents, nil
}

func archiveRelativePath(raw, subpath string, stripRoot bool) (string, bool, error) {
	name := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false, fmt.Errorf("ZIP entry escapes archive root")
	}
	if stripRoot {
		parts := strings.SplitN(clean, "/", 2)
		if len(parts) != 2 {
			return "", false, nil
		}
		clean = parts[1]
	}
	if subpath != "" {
		if clean != subpath && !strings.HasPrefix(clean, subpath+"/") {
			return "", false, nil
		}
		clean = strings.TrimPrefix(strings.TrimPrefix(clean, subpath), "/")
		if clean == "" {
			return "", false, nil
		}
	}
	return clean, true, nil
}

func supportedKnowledgePath(documentPath string, policy skillintel.IngestPolicy) bool {
	extension := strings.ToLower(path.Ext(documentPath))
	allowed := false
	for _, candidate := range policy.AllowedExtensions {
		if strings.EqualFold(extension, candidate) {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	blocked := map[string]struct{}{}
	for _, segment := range policy.ExcludedPathSegments {
		blocked[strings.ToLower(strings.TrimSpace(segment))] = struct{}{}
	}
	for _, segment := range strings.Split(documentPath, "/") {
		if _, exists := blocked[strings.ToLower(segment)]; exists {
			return false
		}
	}
	return true
}

func readZIPDocument(entry *zip.File, limit int) (string, error) {
	stream, err := entry.Open()
	if err != nil {
		return "", fmt.Errorf("open ZIP document %q: %w", entry.Name, err)
	}
	defer stream.Close()
	payload, err := io.ReadAll(io.LimitReader(stream, int64(limit)+1))
	if err != nil {
		return "", fmt.Errorf("read ZIP document %q: %w", entry.Name, err)
	}
	if len(payload) > limit {
		return "", fmt.Errorf("ZIP document %q exceeds %d bytes", entry.Name, limit)
	}
	if !utf8.Valid(payload) {
		return "", fmt.Errorf("ZIP document %q is not valid UTF-8 text", entry.Name)
	}
	return string(payload), nil
}

func fetchSkillSource(ctx context.Context, rawURL string, limit int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "CodeLocal-Skill-Ingest/1.0")
	req.Header.Set("Accept", "text/plain,text/markdown,text/html,application/json,application/yaml,application/zip,application/octet-stream;q=0.8,*/*;q=0.1")
	response, err := safeSkillHTTPClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch Skill source: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("Skill source returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, "", fmt.Errorf("Skill source exceeds %d bytes", limit)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(payload)) > limit {
		return nil, "", fmt.Errorf("Skill source exceeds %d bytes", limit)
	}
	return payload, response.Header.Get("Content-Type"), nil
}

func safeSkillHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, fmt.Errorf("resolve Skill source host")
			}
			for _, candidate := range addresses {
				if !publicSkillSourceIP(candidate) {
					continue
				}
				dialer := net.Dialer{Timeout: 10 * time.Second}
				return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			}
			return nil, fmt.Errorf("Skill source host resolves to a private or unsafe network")
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxSkillRedirects {
				return fmt.Errorf("too many Skill source redirects")
			}
			_, err := validateSkillSourceURL(req.URL.String())
			return err
		},
	}
}

func validateSkillSourceURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return nil, fmt.Errorf("invalid Skill source URL")
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("Skill source URL must use https")
	}
	if parsed.User != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("Skill source URL must not contain credentials")
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return nil, fmt.Errorf("Skill source URL must use the default HTTPS port")
	}
	return parsed, nil
}

func publicSkillSourceIP(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsValid() && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() &&
		!address.IsLinkLocalMulticast() && !address.IsMulticast() && !address.IsUnspecified()
}

func htmlKnowledgeText(payload []byte) string {
	if !utf8.Valid(payload) {
		return ""
	}
	value := htmlScriptStylePattern.ReplaceAllString(string(payload), "\n")
	value = htmlTagPattern.ReplaceAllString(value, "\n")
	value = html.UnescapeString(value)
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func splitURLPath(value string) []string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func escapeURLPath(value string) string {
	parts := strings.Split(value, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}
