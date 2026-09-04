package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type skillManagementItemDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Publisher     string   `json:"publisher"`
	Scope         string   `json:"scope"`
	Kind          string   `json:"kind"`
	State         string   `json:"state"`
	Tags          []string `json:"tags,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Quality       float64  `json:"quality"`
	Verified      bool     `json:"verified"`
	License       string   `json:"license,omitempty"`
	Mode          string   `json:"mode"`
	PinnedVersion string   `json:"pinnedVersion,omitempty"`
	UserRating    int      `json:"userRating,omitempty"`
	RatingAverage float64  `json:"ratingAverage,omitempty"`
	RatingCount   int64    `json:"ratingCount,omitempty"`
}

type skillManagementResponseDTO struct {
	AutoUse           bool                     `json:"autoUse"`
	StorageConfigured bool                     `json:"storageConfigured"`
	Items             []skillManagementItemDTO `json:"items"`
}

type adminSkillVersionDTO struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Scope       string  `json:"scope"`
	Kind        string  `json:"kind"`
	Publisher   string  `json:"publisher"`
	State       string  `json:"state"`
	Quality     float64 `json:"quality"`
	Verified    bool    `json:"verified"`
	PackageHash string  `json:"packageHash"`
	CreatedAt   int64   `json:"createdAt"`
	UpdatedAt   int64   `json:"updatedAt"`
	PromotedAt  int64   `json:"promotedAt,omitempty"`
}

func (s *Server) skillMutationIdentity(w http.ResponseWriter, r *http.Request, requireFresh bool, requireAdmin bool) (*webauth.Identity, bool) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return nil, false
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return nil, false
	}
	if requireFresh && identity.RequiresReauthentication() {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "reauthentication_required"})
		return nil, false
	}
	if requireAdmin && !cloud.IsAdminEmail(identity.User.Email) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "admin_required"})
		return nil, false
	}
	return identity, true
}

func (s *Server) skillsResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	items, err := s.skillManagementItems(r, identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skills_unavailable"})
		return
	}
	services := skillServicesForServer(s)
	webutil.JSON(w, http.StatusOK, skillManagementResponseDTO{AutoUse: true, StorageConfigured: services.Configured && services.Err == nil, Items: items})
}

func (s *Server) skillManagementItems(r *http.Request, userID string) ([]skillManagementItemDTO, error) {
	records, err := s.Store.StableSkillVersionRecords(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	states, err := s.Store.ListSkillUserStates(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	stateByID := map[string]cloud.SkillUserState{}
	for _, state := range states {
		stateByID[state.SkillID] = state
	}

	items := []skillManagementItemDTO{}
	seenSystem := map[string]bool{}
	for _, record := range records {
		if record.Manifest.Scope == skillintel.ScopeSystem {
			seenSystem[record.Manifest.ID] = true
		}
		items = append(items, skillManagementItemFromManifest(record.Manifest, string(record.State), stateByID[record.Manifest.ID]))
	}
	for _, manifest := range skillintel.BuiltinManifests() {
		if seenSystem[manifest.ID] {
			continue
		}
		items = append(items, skillManagementItemFromManifest(manifest, "active", stateByID[manifest.ID]))
	}

	communityRefs := []cloud.SkillVersionRef{}
	for _, item := range items {
		if item.Scope == string(skillintel.ScopeCommunity) {
			communityRefs = append(communityRefs, cloud.SkillVersionRef{SkillID: item.ID, Version: item.Version})
		}
	}
	quality, err := s.Store.SkillQualitySignals(r.Context(), communityRefs)
	if err != nil {
		return nil, err
	}
	for index := range items {
		item := &items[index]
		if signal, ok := quality[item.ID+"@"+item.Version]; ok {
			item.Quality = signal.Quality
			item.RatingAverage = signal.RatingAverage
			item.RatingCount = signal.RatingCount
		}
		if rating, ok, ratingErr := s.Store.GetSkillRating(r.Context(), userID, item.ID); ratingErr != nil {
			return nil, ratingErr
		} else if ok {
			item.UserRating = rating
		}
	}
	return items, nil
}

func skillManagementItemFromManifest(manifest skillintel.Manifest, state string, userState cloud.SkillUserState) skillManagementItemDTO {
	mode := strings.TrimSpace(userState.Mode)
	if mode == "" {
		mode = "auto"
	}
	capabilities := make([]string, 0, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		capabilities = append(capabilities, string(capability))
	}
	return skillManagementItemDTO{
		ID: manifest.ID, Name: manifest.Name, Version: manifest.Version, Publisher: manifest.Publisher,
		Scope: string(manifest.Scope), Kind: string(manifest.Kind), State: state,
		Tags: append([]string(nil), manifest.Tags...), Capabilities: capabilities,
		Quality: manifest.Quality, Verified: manifest.Verified, License: manifest.License,
		Mode: mode, PinnedVersion: userState.PinnedVersion,
	}
}

func (s *Server) skillImportPersonalAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, false)
	if !ok {
		return
	}
	services := skillServicesForServer(s)
	if services.Imports == nil || services.Err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill_storage_unavailable"})
		return
	}
	var pkg skillintel.Package
	if err := webutil.DecodeJSON(r, int64(skillintel.MaxStoredSkillPackageBytes)+1, &pkg); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_package"})
		return
	}
	result, err := services.Imports.ImportPersonal(r.Context(), identity.User.ID, pkg)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_import_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.personal_imported", Detail: map[string]any{"skillId": pkg.Manifest.ID, "version": pkg.Manifest.Version, "packageHash": pkg.PackageHash}})
	webutil.JSON(w, http.StatusCreated, result)
}

func (s *Server) skillPublishCommunityAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, false)
	if !ok {
		return
	}
	services := skillServicesForServer(s)
	if services.Imports == nil || services.Err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill_storage_unavailable"})
		return
	}
	var pkg skillintel.Package
	if err := webutil.DecodeJSON(r, int64(skillintel.MaxStoredSkillPackageBytes)+1, &pkg); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_package"})
		return
	}
	result, err := services.Imports.PublishCommunity(r.Context(), identity.User.ID, pkg)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_publish_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.community_submitted", Detail: map[string]any{"skillId": pkg.Manifest.ID, "version": pkg.Manifest.Version, "packageHash": pkg.PackageHash}})
	webutil.JSON(w, http.StatusAccepted, result)
}

func (s *Server) skillUserStateAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, false, false)
	if !ok {
		return
	}
	skillID := strings.TrimSpace(r.PathValue("skillID"))
	var input struct {
		Mode          string `json:"mode"`
		PinnedVersion string `json:"pinnedVersion"`
	}
	if skillID == "" || webutil.DecodeJSON(r, 32<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_state"})
		return
	}
	state := cloud.SkillUserState{UserID: identity.User.ID, SkillID: skillID, Mode: strings.TrimSpace(input.Mode), PinnedVersion: strings.TrimSpace(input.PinnedVersion)}
	if err := s.Store.SetSkillUserState(r.Context(), state); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_state_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.user_state_updated", Detail: map[string]any{"skillId": skillID, "mode": state.Mode, "pinnedVersion": state.PinnedVersion}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) skillRatingAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, false, false)
	if !ok {
		return
	}
	skillID := strings.TrimSpace(r.PathValue("skillID"))
	var input struct {
		Rating int `json:"rating"`
	}
	if skillID == "" || webutil.DecodeJSON(r, 16<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_rating"})
		return
	}
	if !s.communitySkillStableForUser(r, identity.User.ID, skillID) {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "community_skill_required"})
		return
	}
	if err := s.Store.SetSkillRating(r.Context(), identity.User.ID, skillID, input.Rating); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_rating_rejected", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) communitySkillStableForUser(r *http.Request, userID, skillID string) bool {
	records, err := s.Store.StableSkillVersionRecords(r.Context(), userID)
	if err != nil {
		return false
	}
	for _, record := range records {
		if record.Manifest.ID == skillID && record.Manifest.Scope == skillintel.ScopeCommunity && record.State == cloud.SkillVersionPromoted {
			return true
		}
	}
	return false
}

func (s *Server) adminSkillsResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !cloud.IsAdminEmail(identity.User.Email) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "admin_required"})
		return
	}
	records, err := s.Store.ListSharedSkillVersions(r.Context())
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin_skills_unavailable"})
		return
	}
	items := make([]adminSkillVersionDTO, 0, len(records))
	for _, record := range records {
		items = append(items, adminSkillVersionDTO{
			ID: record.Manifest.ID, Name: record.Manifest.Name, Version: record.Manifest.Version,
			Scope: string(record.Manifest.Scope), Kind: string(record.Manifest.Kind), Publisher: record.Publisher,
			State: string(record.State), Quality: record.Manifest.Quality, Verified: record.Manifest.Verified,
			PackageHash: record.PackageHash, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, PromotedAt: record.PromotedAt,
		})
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) adminSkillImportAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, true)
	if !ok {
		return
	}
	services := skillServicesForServer(s)
	if services.Imports == nil || services.Err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill_storage_unavailable"})
		return
	}
	var pkg skillintel.Package
	if err := webutil.DecodeJSON(r, int64(skillintel.MaxStoredSkillPackageBytes)+1, &pkg); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_package"})
		return
	}
	result, err := services.Imports.ImportAdminCandidate(r.Context(), identity.User.ID, pkg)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "admin_skill_import_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.admin_candidate_imported", Detail: map[string]any{"skillId": pkg.Manifest.ID, "version": pkg.Manifest.Version, "packageHash": pkg.PackageHash}})
	webutil.JSON(w, http.StatusCreated, result)
}

func (s *Server) verifyAdminSkillPackage(w http.ResponseWriter, r *http.Request, skillID, version string) (cloud.SkillVersionRecord, bool) {
	services := skillServicesForServer(s)
	if services.Packages == nil || services.Err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill_storage_unavailable"})
		return cloud.SkillVersionRecord{}, false
	}
	record, exists, err := s.Store.SkillVersionRecordByIdentity(r.Context(), "", skillID, version)
	if err != nil || !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "skill_version_not_found"})
		return cloud.SkillVersionRecord{}, false
	}
	pkg, exists, err := services.Packages.Get(r.Context(), record.PackageHash)
	if err != nil || !exists || pkg.Artifact.Manifest.ContentHash != record.ArtifactHash {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "skill_package_integrity_unavailable"})
		return cloud.SkillVersionRecord{}, false
	}
	return record, true
}

func (s *Server) adminSkillEvaluationStartAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, true)
	if !ok {
		return
	}
	skillID, version := strings.TrimSpace(r.PathValue("skillID")), strings.TrimSpace(r.PathValue("version"))
	if _, ok := s.verifyAdminSkillPackage(w, r, skillID, version); !ok {
		return
	}
	record, err := s.Store.StartSkillEvaluation(r.Context(), skillID, version)
	if err != nil {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "skill_evaluation_start_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.evaluation_started", Detail: map[string]any{"skillId": skillID, "version": version}})
	webutil.JSON(w, http.StatusOK, record)
}

func (s *Server) adminSkillEvaluationCompleteAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, true)
	if !ok {
		return
	}
	skillID, version := strings.TrimSpace(r.PathValue("skillID")), strings.TrimSpace(r.PathValue("version"))
	if _, ok := s.verifyAdminSkillPackage(w, r, skillID, version); !ok {
		return
	}
	var input struct {
		Passed bool           `json:"passed"`
		Score  float64        `json:"score"`
		Checks map[string]any `json:"checks"`
	}
	if webutil.DecodeJSON(r, 128<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_evaluation"})
		return
	}
	record, evaluation, err := s.Store.CompleteSkillEvaluation(r.Context(), identity.User.ID, skillID, version, input.Passed, input.Score, input.Checks)
	if err != nil {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "skill_evaluation_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.evaluation_completed", Detail: map[string]any{"skillId": skillID, "version": version, "decision": evaluation.Decision, "score": evaluation.Score}})
	webutil.JSON(w, http.StatusOK, map[string]any{"record": record, "evaluation": evaluation})
}

func (s *Server) adminSkillPromoteAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, true)
	if !ok {
		return
	}
	skillID, version := strings.TrimSpace(r.PathValue("skillID")), strings.TrimSpace(r.PathValue("version"))
	if _, ok := s.verifyAdminSkillPackage(w, r, skillID, version); !ok {
		return
	}
	record, err := s.Store.PromoteSkillVersion(r.Context(), skillID, version)
	if err != nil {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "skill_promotion_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.promoted", Detail: map[string]any{"skillId": skillID, "version": version}})
	webutil.JSON(w, http.StatusOK, record)
}

func (s *Server) adminSkillRollbackAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, true)
	if !ok {
		return
	}
	skillID, version := strings.TrimSpace(r.PathValue("skillID")), strings.TrimSpace(r.PathValue("version"))
	record, ok := s.verifyAdminSkillPackage(w, r, skillID, version)
	if !ok {
		return
	}
	if record.State != cloud.SkillVersionRolledBack {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "rollback_target_must_be_previous_promoted_version"})
		return
	}
	restored, err := s.Store.PromoteSkillVersion(r.Context(), skillID, version)
	if err != nil {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "skill_rollback_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.rolled_back", Detail: map[string]any{"skillId": skillID, "version": version}})
	webutil.JSON(w, http.StatusOK, restored)
}
