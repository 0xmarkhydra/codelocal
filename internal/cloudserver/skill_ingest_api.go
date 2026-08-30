package cloudserver

import (
	"context"
	"fmt"
	"net/http"
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

const maxSkillIngestRequestBytes = 14 << 20

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
		return validatePersonalPackagePassthrough(*input.Package)
	}
	documents, sourceURL, sourceType, err := ingestDocuments(ctx, input)
	if err != nil {
		return skillintel.Package{}, sourceType, err
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

func validatePersonalPackagePassthrough(pkg skillintel.Package) (skillintel.Package, string, error) {
	if pkg.Manifest.Scope != skillintel.ScopePersonal {
		return skillintel.Package{}, "package", fmt.Errorf("Add Skill accepts Personal packages only; publish Community Skills separately")
	}
	if err := skillintel.ValidatePackageForImport(pkg, skillintel.UserImportPolicy()); err != nil {
		return skillintel.Package{}, "package", err
	}
	return pkg, "package", nil
}

func ingestDocuments(ctx context.Context, input skillIngestRequest) ([]skillintel.SourceDocument, string, string, error) {
	documents := append([]skillintel.SourceDocument(nil), input.Documents...)
	sourceURL := strings.TrimSpace(input.SourceURL)
	sourceType := "documents"
	if text := strings.TrimSpace(input.Text); text != "" {
		documents = append(documents, skillintel.SourceDocument{Path: "SKILL.md", Content: text})
		sourceType = "text"
	}
	if archive := strings.TrimSpace(input.ArchiveBase64); archive != "" {
		archiveDocuments, err := sourceDocumentsFromZIPBase64(archive)
		if err != nil {
			return nil, sourceURL, "archive", err
		}
		documents = append(documents, archiveDocuments...)
		sourceType = "archive"
	}
	if sourceURL != "" {
		remoteDocuments, resolvedURL, err := sourceDocumentsFromURL(ctx, sourceURL)
		if err != nil {
			return nil, sourceURL, "url", err
		}
		documents = append(documents, remoteDocuments...)
		sourceURL = resolvedURL
		sourceType = "url"
	}
	if len(documents) == 0 {
		return nil, sourceURL, sourceType, fmt.Errorf("paste text or a URL, upload files/folder/ZIP, or provide a .skill.json package")
	}
	return documents, sourceURL, sourceType, nil
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
		for _, token := range strings.FieldsFunc(strings.ToLower(value), splitSkillSignal) {
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

func splitSkillSignal(r rune) bool {
	return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 0x80
}
