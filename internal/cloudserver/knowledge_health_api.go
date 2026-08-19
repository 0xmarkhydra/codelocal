package cloudserver

import (
	"net/http"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type knowledgePipelineHealthDTO struct {
	Available         bool   `json:"available"`
	Status            string `json:"status"`
	PendingCount      int    `json:"pendingCount"`
	ProcessingCount   int    `json:"processingCount"`
	RetryingCount     int    `json:"retryingCount"`
	DeadCount         int    `json:"deadCount"`
	ProcessedLastHour int    `json:"processedLastHour"`
	OldestActiveAgeMS int64  `json:"oldestActiveAgeMs"`
}

type knowledgeIndexHealthDTO struct {
	Available       bool   `json:"available"`
	Status          string `json:"status"`
	ProjectCount    int    `json:"projectCount"`
	CurrentProjects int    `json:"currentProjects"`
	EmptyProjects   int    `json:"emptyProjects"`
	StaleProjects   int    `json:"staleProjects"`
	MissingProjects int    `json:"missingProjects"`
	MaxLagMS        int64  `json:"maxLagMs"`
}

type knowledgeCanaryDTO struct {
	Available                  bool  `json:"available"`
	AttemptsTotal              int64 `json:"attemptsTotal"`
	AppliedCount               int64 `json:"appliedCount"`
	AppliedClaimsTotal         int64 `json:"appliedClaimsTotal"`
	DeterministicFallbackCount int64 `json:"deterministicFallbackCount"`
	ReadinessBlockedCount      int64 `json:"readinessBlockedCount"`
	ErrorCount                 int64 `json:"errorCount"`
	TimeoutCount               int64 `json:"timeoutCount"`
	SlowCount                  int64 `json:"slowCount"`
}

type collectiveRecommendationDTO struct {
	TaskKind            string   `json:"taskKind"`
	CheckProfile        []string `json:"checkProfile"`
	FileCountBucket     string   `json:"fileCountBucket"`
	SymbolCountBucket   string   `json:"symbolCountBucket"`
	ExecutionTool       string   `json:"executionTool"`
	QualityBucket       string   `json:"qualityBucket"`
	SkillUsed           bool     `json:"skillUsed"`
	DiffObserved        bool     `json:"diffObserved"`
	ContributorCount    int      `json:"contributorCount"`
	SampleCount         int64    `json:"sampleCount"`
	MeanUserSuccessRate float64  `json:"meanUserSuccessRate"`
	LastSeenAt          int64    `json:"lastSeenAt"`
}

type collectiveHealthDTO struct {
	Available             bool                          `json:"available"`
	ContributionEnabled   bool                          `json:"contributionEnabled"`
	SuggestionsEnabled    bool                          `json:"suggestionsEnabled"`
	ContributionAvailable bool                          `json:"contributionAvailable"`
	SuggestionsAvailable  bool                          `json:"suggestionsAvailable"`
	MinimumContributors   int                           `json:"minimumContributors"`
	Recommendations       []collectiveRecommendationDTO `json:"recommendations"`
}

type knowledgeHealthResourceDTO struct {
	CSRF          string                     `json:"csrf"`
	Pipeline      knowledgePipelineHealthDTO `json:"pipeline"`
	GraphIndex    knowledgeIndexHealthDTO    `json:"graphIndex"`
	SemanticIndex knowledgeIndexHealthDTO    `json:"semanticIndex"`
	Canary        knowledgeCanaryDTO         `json:"canary"`
	Collective    collectiveHealthDTO        `json:"collective"`
	Privacy       map[string]bool            `json:"privacy"`
}

func knowledgePipelineHealth(value cloud.DurableOutboxHealth, available bool) knowledgePipelineHealthDTO {
	return knowledgePipelineHealthDTO{
		Available: available, Status: value.Status, PendingCount: value.PendingCount,
		ProcessingCount: value.ProcessingCount, RetryingCount: value.RetryingCount, DeadCount: value.DeadCount,
		ProcessedLastHour: value.ProcessedLastHour, OldestActiveAgeMS: value.OldestActiveAgeMS,
	}
}

func knowledgeGraphIndexHealth(value cloud.CanonicalGraphFreshnessSummary, available bool) knowledgeIndexHealthDTO {
	return knowledgeIndexHealthDTO{
		Available: available, Status: value.Status, ProjectCount: value.ProjectCount, CurrentProjects: value.CurrentProjects,
		EmptyProjects: value.EmptyProjects, StaleProjects: value.StaleProjects, MissingProjects: value.MissingProjects, MaxLagMS: value.MaxLagMS,
	}
}

func knowledgeSemanticIndexHealth(value cloud.CanonicalEmbeddingFreshnessSummary, available bool) knowledgeIndexHealthDTO {
	return knowledgeIndexHealthDTO{
		Available: available, Status: value.Status, ProjectCount: value.ProjectCount, CurrentProjects: value.CurrentProjects,
		EmptyProjects: value.EmptyProjects, StaleProjects: value.StaleProjects, MissingProjects: value.MissingProjects, MaxLagMS: value.MaxLagMS,
	}
}

func knowledgeCanaryHealth(value cloud.CanonicalSemanticCanaryMetrics, available bool) knowledgeCanaryDTO {
	return knowledgeCanaryDTO{
		Available: available, AttemptsTotal: value.AttemptsTotal, AppliedCount: value.AppliedCount, AppliedClaimsTotal: value.AppliedClaimsTotal,
		DeterministicFallbackCount: value.DeterministicFallbackCount, ReadinessBlockedCount: value.ReadinessBlockedCount,
		ErrorCount: value.ReadinessErrorCount + value.RecallErrorCount, TimeoutCount: value.TimeoutCount, SlowCount: value.SlowCount,
	}
}

func collectiveRecommendationResources(values []cloud.CollectiveRecommendation) []collectiveRecommendationDTO {
	out := make([]collectiveRecommendationDTO, 0, len(values))
	for _, value := range values {
		out = append(out, collectiveRecommendationDTO{
			TaskKind: value.Fingerprint.TaskKind, CheckProfile: append([]string(nil), value.Fingerprint.CheckProfile...),
			FileCountBucket: value.Fingerprint.FileCountBucket, SymbolCountBucket: value.Fingerprint.SymbolBucket,
			ExecutionTool: value.Fingerprint.ExecutionTool, QualityBucket: value.Fingerprint.QualityBucket,
			SkillUsed: value.Fingerprint.SkillUsed, DiffObserved: value.Fingerprint.DiffObserved,
			ContributorCount: value.ContributorCount, SampleCount: value.SampleCount,
			MeanUserSuccessRate: graphUnit(value.MeanUserSuccessRate), LastSeenAt: value.LastSeenAt,
		})
	}
	return out
}

func (s *Server) knowledgeHealthResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}

	pipeline, pipelineErr := s.Store.DurableOutboxHealth(r.Context(), identity.User.ID)
	_, graphIndex, graphErr := s.Store.CanonicalGraphFreshness(r.Context(), identity.User.ID)
	_, semanticIndex, semanticErr := s.Store.CanonicalEmbeddingFreshness(r.Context(), identity.User.ID)
	canary, canaryErr := s.Store.CanonicalSemanticCanaryMetrics(r.Context(), identity.User.ID)
	preference, preferenceErr := s.Store.CollectivePreference(r.Context(), identity.User.ID)

	recommendations := []cloud.CollectiveRecommendation{}
	if preferenceErr == nil && preference.SuggestionsEnabled {
		values, err := s.Store.CollectiveRecommendations(r.Context(), identity.User.ID, 6)
		if err == nil {
			recommendations = values
		}
	}

	webutil.JSON(w, http.StatusOK, knowledgeHealthResourceDTO{
		CSRF:          identity.CSRF,
		Pipeline:      knowledgePipelineHealth(pipeline, pipelineErr == nil),
		GraphIndex:    knowledgeGraphIndexHealth(graphIndex, graphErr == nil),
		SemanticIndex: knowledgeSemanticIndexHealth(semanticIndex, semanticErr == nil),
		Canary:        knowledgeCanaryHealth(canary, canaryErr == nil),
		Collective: collectiveHealthDTO{
			Available: preferenceErr == nil, ContributionEnabled: preference.ContributionEnabled, SuggestionsEnabled: preference.SuggestionsEnabled,
			ContributionAvailable: cloud.CollectiveContributionAvailable(), SuggestionsAvailable: cloud.CollectiveSuggestionsAvailable(),
			MinimumContributors: cloud.CollectiveMinimumContributors(), Recommendations: collectiveRecommendationResources(recommendations),
		},
		Privacy: map[string]bool{
			"rawCodeShared": false, "conversationShared": false, "projectIdentityShared": false, "localReplayTrustShared": false,
		},
	})
}
