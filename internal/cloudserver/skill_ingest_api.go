package cloudserver

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

const (
	maxSkillIngestRequestBytes = 14 << 20
	maxSkillArchiveBytes       = 8 << 20
	maxSkillSourceBytes        = 2 << 20
	maxSkillRedirects          = 5
)

type skillIngestRequest struct {
	Name          string                      `json:"name,omitempty"`
	Text          string                      `json:"text,omitempty"`
	Documents     []skillintel.SourceDocument `json:"documents,omitempty"`
	ArchiveBase64 string                      `json:"archiveBase64,omitempty"`
	SourceURL     string                      `json:"sourceUrl,omitempty"`
	Package       *skillintel.Package          `json:"package,omitempty"`
}

type skillIngestResponse struct {
	SkillID     string                       `json:"skillId"`
	Name        string                       `json:"name"`
	Version     string                       `json:"version"`
	SourceType  string                       `json:"sourceType"`
	Disposition string                       `json:"disposition"`
	Import      cloud.CloudSkillImportResult `json:"import"`
}

func (s *Server) skillIngestAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, false)
	if !ok {
		return
	}
	services := skillServicesForServer(s)
	if services.Imports == nil || services.Err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill_storage_unavailable"})
		return
	}

	var input skillIngestRequest
	if err := webutil.DecodeJSON(r, maxSkillIngestRequestBytes, &input); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_ingest_request"})
		return
	}

	pkg, sourceType, err := buildPersonalPackageFromIngest(r.Context(), input)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_ingest_rejected", "detail": err.Error()})
		return
	}
	result, err := services.Imports.ImportPersonal(r.Context(), identity.User.ID, pkg)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_import_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{
		UserID: identity.User.ID,
		Event:  "skill.personal_ingested",
		Detail: map[string]any{
			"skillId": pkg.Manifest.ID, "version": pkg.Manifest.Version,
			"packageHash": pkg.PackageHash, "sourceType": sourceType,
		},
	})
	webutil.JSON(w, http.StatusCreated, skillIngestResponse{
		SkillID: pkg.Manifest.ID, Name: pkg.Manifest.Name, Version: pkg.Manifest.Version,
		SourceType: sourceType, Disposition: result.Disposition, Import: result,
	})
}

func buildPersonalPackageFromIngest(ctx context.Context, input skillIngestRequest) (skillintel.Package, string, error) {
	if input.Package != nil {
		pkg := *input.Package
		if pkg.Manifest.Scope != skillintel.ScopePersonal {
			return skillintel.Package{}, "package", fmt.Errorf("Add Skill accepts Personal packages only; publish Community Skills separately")
		}
		if err := skillintel.ValidatePackageForImport(pkg, skillintel.UserImportPolicy()); err != nil {
			return skillintel.Package{}, "package", err
		}
		return pkg, "package", nil
	}

	documents := append([]skillintel.SourceDocument(nil), input.Documents...)
	sourceType := "documents"
	sourceURL := strings.TrimSpace(input.SourceURL)
	if text := strings.TrimSpace(input.Text); text != "" {
		documents = append(documents, skillintel.SourceDocument{Path: "SKILL.md", Content: text})
		sourceType = "text"
	}
	if archive := strings.TrimSpace(input.ArchiveBase64); archive != "" {
		archiveDocuments, err := sourceDocumentsFromZIPBase64(archive)
		if err != nil {
			return skillintel.Package{}, "archive", err
		}
		documents = append(documents, archiveDocuments...)
		sourceType = "archive"
	}
	if sourceURL != "" {
		remoteDocuments, resolvedURL, err := sourceDocumentsFromURL(ctx, sourceURL)
		if err != nil {
			return skillintel.Package{}, "url", err
		}
		documents = append(documents, remoteDocuments...)
		sourceURL = resolvedURL
		sourceType = "url"
	}
	if len(documents) == 0 {
		return skillintel.Package{}, sourceType, fmt.Errorf("paste text or a URL, upload files/folder/ZIP, or provide a .skill.json package")
	}

	name := deriveSkillName(input.Name, sourceURL, documents)
	manifest := skillintel.Manifest{
		ID:        personalSkillID(name),
		Name:      name,
		Version:   fmt.Sprintf("0.1.%d", time.Now().UnixMilli()),
		Publisher: "CodeLocal user",
		Scope:     skillintel.ScopePersonal,
		Kind:      skillintel.KindKnowledge,
		Intents:   deriveSkillSignals(name, documents),
		Tags:      deriveSkillSignals(name, documents),
		Quality:   0.72,
		Verified:  false,
		SourceURL: sourceURL,
	}
	artifact, err := skillintel.BuildArtifactFromDocuments(manifest, documents, skillintel.DefaultKnowledgeIngestPolicy())
	if err != nil {
		return skillintel.Package{}, sourceType, err
	}
	pkg, err := skillintel.BuildPackage(manifest, artifact)
	if err != nil {
		return skillintel.Package{}, sourceType, err
	}
	return pkg, sourceType, nil
}

func sourceDocumentsFromZIPBase64(encoded string) ([]skillintel.SourceDocument, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP payload")
	}
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
	documents := make([]skillintel.SourceDocument, 0, len(reader.File))
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimSpace(strings.ReplaceAll(entry.Name, "\\", "/"))
		clean := path.Clean(name)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return nil, fmt.Errorf("ZIP entry escapes archive root")
		}
		if entry.UncompressedSize64 > uint64(policy.MaxDocumentBytes) {
			return nil, fmt.Errorf("ZIP document %q exceeds %d bytes", clean, policy.MaxDocumentBytes)
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open ZIP document %q: %w", clean, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, int64(policy.MaxDocumentBytes)+1))
		closeErr := stream.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read ZIP document %q: %w", clean, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close ZIP document %q: %w", clean, closeErr)
		}
		if len(content) > policy.MaxDocumentBytes {
			return nil, fmt.Errorf("ZIP document %q exceeds %d bytes", clean, policy.MaxDocumentBytes)
		}
		total += int64(len(content))
		if total > maxSkillArchiveBytes {
			return nil, fmt.Errorf("ZIP expanded content exceeds %d bytes", maxSkillArchiveBytes)
		}
		documents = append(documents, skillintel.SourceDocument{Path: clean, Content: string(content)})
	}
	return documents, nil
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
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return nil, "", fmt.Errorf("HTML pages are not ingested as raw knowledge yet; paste the text or use a raw/document URL")
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		name = "source.md"
	}
	if path.Ext(name) == "" {
		name += ".md"
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
		return []skillintel.SourceDocument{{Path: filePath, Content: string(payload)}}, source.String(), true, nil
	}
	if len(segments) > 2 && segments[2] != "tree" {
		return nil, "", false, nil
	}

	ref := ""
	subpath := ""
	if len(segments) >= 4 && segments[2] == "tree" {
		ref = segments[3]
		if len(segments) > 4 {
			subpath = strings.Join(segments[4:], "/")
		}
	}
	if ref == "" {
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
		metadata, _, err := fetchSkillSource(ctx, apiURL, 256<<10)
		if err != nil {
			return nil, source.String(), true, fmt.Errorf("resolve GitHub repository: %w", err)
		}
		var repoMetadata struct{ DefaultBranch string `json:"default_branch"` }
		if json.Unmarshal(metadata, &repoMetadata) != nil || strings.TrimSpace(repoMetadata.DefaultBranch) == "" {
			return nil, source.String(), true, fmt.Errorf("resolve GitHub default branch")
		}
		ref = repoMetadata.DefaultBranch
	}
	archiveURL := fmt.Sprintf("https://codeload.github.com/%s/%s/zip/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))
	archive, _, err := fetchSkillSource(ctx, archiveURL, maxSkillArchiveBytes)
	if err != nil {
		return nil, source.String(), true, fmt.Errorf("download GitHub source snapshot: %w", err)
	}
	documents, err := sourceDocumentsFromZIPBytes(archive, subpath)
	if err != nil {
		return nil, source.String(), true, err
	}
	return documents, source.String(), true, nil
}

func sourceDocumentsFromZIPBytes(payload []byte, subpath string) ([]skillintel.SourceDocument, error) {
	if len(payload) == 0 || len(payload) > maxSkillArchiveBytes {
		return nil, fmt.Errorf("source archive exceeds %d bytes", maxSkillArchiveBytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("invalid source archive")
	}
	policy := skillintel.DefaultKnowledgeIngestPolicy()
	documents := []skillintel.SourceDocument{}
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimSpace(strings.ReplaceAll(entry.Name, "\\", "/"))
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			continue
		}
		relative := path.Clean(parts[1])
		if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || strings.HasPrefix(relative, "/") {
			return nil, fmt.Errorf("source archive entry escapes root")
		}
		if subpath != "" {
			cleanSubpath := strings.Trim(path.Clean(subpath), "/")
			if relative != cleanSubpath && !strings.HasPrefix(relative, cleanSubpath+"/") {
				continue
			}
			relative = strings.TrimPrefix(strings.TrimPrefix(relative, cleanSubpath), "/")
			if relative == "" {
				continue
			}
		}
		if entry.UncompressedSize64 > uint64(policy.MaxDocumentBytes) {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, int64(policy.MaxDocumentBytes)+1))
		_ = stream.Close()
		if readErr != nil || len(content) > policy.MaxDocumentBytes {
			continue
		}
		total += int64(len(content))
		if total > maxSkillArchiveBytes {
			return nil, fmt.Errorf("source archive expanded content exceeds %d bytes", maxSkillArchiveBytes)
		}
		documents = append(documents, skillintel.SourceDocument{Path: relative, Content: string(content)})
		if len(documents) > policy.MaxDocuments {
			return nil, fmt.Errorf("source archive contains too many documents")
		}
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("source archive contained no supported knowledge documents")
	}
	return documents, nil
}

func fetchSkillSource(ctx context.Context, rawURL string, limit int64) ([]byte, string, error) {
	client := safeSkillHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "CodeLocal-Skill-Ingest/1.0")
	req.Header.Set("Accept", "text/plain,text/markdown,application/json,application/yaml,application/zip,application/octet-stream;q=0.8,*/*;q=0.1")
	response, err := client.Do(req)
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
		Proxy: http.ProxyFromEnvironment,
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

func deriveSkillName(preferred, sourceURL string, documents []skillintel.SourceDocument) string {
	if name := strings.TrimSpace(preferred); name != "" {
		return truncateSkillName(name)
	}
	if sourceURL != "" {
		if parsed, err := url.Parse(sourceURL); err == nil {
			segments := splitURLPath(parsed.Path)
			if strings.EqualFold(parsed.Hostname(), "github.com") && len(segments) >= 2 {
				return truncateSkillName(strings.TrimSuffix(segments[1], ".git"))
			}
			if base := strings.TrimSuffix(path.Base(parsed.Path), path.Ext(parsed.Path)); base != "" && base != "." {
				return truncateSkillName(base)
			}
		}
	}
	if len(documents) > 0 {
		base := strings.TrimSuffix(path.Base(documents[0].Path), path.Ext(documents[0].Path))
		if base != "" && base != "." {
			return truncateSkillName(base)
		}
	}
	return "Personal Skill"
}

func truncateSkillName(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 80 {
		return value
	}
	return strings.TrimSpace(value[:80])
}

var nonSkillID = regexp.MustCompile(`[^a-z0-9]+`)

func personalSkillID(name string) string {
	slug := strings.Trim(nonSkillID.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		slug = "personal-skill"
	}
	if len(slug) > 72 {
		slug = strings.Trim(slug[:72], "-")
	}
	return "personal." + slug
}

func deriveSkillSignals(name string, documents []skillintel.SourceDocument) []string {
	words := []string{name}
	for _, document := range documents {
		words = append(words, strings.TrimSuffix(path.Base(document.Path), path.Ext(document.Path)))
		if len(words) >= 16 {
			break
		}
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range words {
		for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 0x80
		}) {
			token = strings.TrimSpace(token)
			if len(token) < 2 {
				continue
			}
			if _, exists := seen[token]; exists {
				continue
			}
			seen[token] = struct{}{}
			out = append(out, token)
			if len(out) >= 16 {
				sort.Strings(out)
				return out
			}
		}
	}
	sort.Strings(out)
	return out
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

func sourceFingerprint(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:12])
}
