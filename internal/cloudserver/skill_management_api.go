package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

// skillsManagementResourceAPI is the user-facing management catalog. It is
// intentionally broader than the router catalog: disabled skills and a
// creator's pending Community submissions remain visible so they can be
// managed without becoming routable.
func (s *Server) skillsManagementResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	items, err := s.skillManagementItemsForUser(r, identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skills_unavailable"})
		return
	}
	services := skillServicesForServer(s)
	webutil.JSON(w, http.StatusOK, skillManagementResponseDTO{
		AutoUse: true,
		StorageConfigured: services.Configured && services.Err == nil,
		Items: items,
	})
}

func (s *Server) skillManagementItemsForUser(r *http.Request, userID string) ([]skillManagementItemDTO, error) {
	stable, err := s.Store.StableSkillVersionRecords(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	personal, err := s.Store.ListPersonalSkillVersions(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	shared, err := s.Store.ListSharedSkillVersions(r.Context())
	if err != nil {
		return nil, err
	}
	states, err := s.Store.ListSkillUserStates(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	stateByID := make(map[string]cloud.SkillUserState, len(states))
	for _, state := range states {
		stateByID[state.SkillID] = state
	}

	stableKey := map[string]bool{}
	seenSystem := map[string]bool{}
	items := []skillManagementItemDTO{}
	seenItem := map[string]bool{}
	appendRecord := func(record cloud.SkillVersionRecord) {
		key := string(record.Manifest.Scope) + "\x00" + record.Manifest.ID + "\x00" + record.Manifest.Version
		if seenItem[key] {
			return
		}
		seenItem[key] = true
		item := skillManagementItemFromManifest(record.Manifest, string(record.State), stateByID[record.Manifest.ID])
		// Publisher authority comes from the authenticated Cloud registry record,
		// never from the creator-controlled manifest claim inside the package.
		item.Publisher = record.Publisher
		items = append(items, item)
		if record.Manifest.Scope == skillintel.ScopeSystem {
			seenSystem[record.Manifest.ID] = true
		}
	}
	for _, record := range stable {
		stableKey[record.Manifest.ID+"@"+record.Manifest.Version] = true
		appendRecord(record)
	}

	// Show one current Personal version per ID. Stable/pinned versions were
	// already added above; ListPersonalSkillVersions is newest-first per skill.
	seenPersonal := map[string]bool{}
	for _, record := range personal {
		if seenPersonal[record.Manifest.ID] {
			continue
		}
		seenPersonal[record.Manifest.ID] = true
		appendRecord(record)
	}

	// Pending Community submissions are visible only to their creator. Promoted
	// shared versions are already present through the stable catalog.
	for _, record := range shared {
		if record.Manifest.Scope != skillintel.ScopeCommunity || record.CreatorUserID != userID {
			continue
		}
		if stableKey[record.Manifest.ID+"@"+record.Manifest.Version] {
			continue
		}
		appendRecord(record)
	}
	for _, manifest := range skillintel.BuiltinManifests() {
		if seenSystem[manifest.ID] {
			continue
		}
		item := skillManagementItemFromManifest(manifest, "active", stateByID[manifest.ID])
		items = append(items, item)
	}

	communityRefs := []cloud.SkillVersionRef{}
	skillIDs := make([]string, 0, len(items))
	for _, item := range items {
		skillIDs = append(skillIDs, item.ID)
		if item.Scope == string(skillintel.ScopeCommunity) {
			communityRefs = append(communityRefs, cloud.SkillVersionRef{SkillID: item.ID, Version: item.Version})
		}
	}
	quality, err := s.Store.SkillQualitySignals(r.Context(), communityRefs)
	if err != nil {
		return nil, err
	}
	ratings, err := s.Store.SkillRatingsForUser(r.Context(), userID, skillIDs)
	if err != nil {
		return nil, err
	}
	for index := range items {
		item := &items[index]
		if signal, exists := quality[item.ID+"@"+item.Version]; exists {
			item.Quality = signal.Quality
			item.RatingAverage = signal.RatingAverage
			item.RatingCount = signal.RatingCount
		}
		item.UserRating = ratings[item.ID]
		item.Publisher = strings.TrimSpace(item.Publisher)
	}
	return items, nil
}
