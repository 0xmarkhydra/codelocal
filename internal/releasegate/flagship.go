package releasegate

import "sort"

type FlagshipEvidence struct {
	DirtyCheckoutPreserved       bool `json:"dirtyCheckoutPreserved"`
	UntrackedUserFilesPreserved  bool `json:"untrackedUserFilesPreserved"`
	AgentWorktreeIsolated        bool `json:"agentWorktreeIsolated"`
	CrashResumeSucceeded         bool `json:"crashResumeSucceeded"`
	NoDuplicateSideEffect        bool `json:"noDuplicateSideEffect"`
	StalePatchBlocked            bool `json:"stalePatchBlocked"`
	ThreeWayReconciled           bool `json:"threeWayReconciled"`
	RequiredVerificationPassed   bool `json:"requiredVerificationPassed"`
	ProductVerificationPassed    bool `json:"productVerificationPassed"`
	PolicyPassed                 bool `json:"policyPassed"`
	NoSecretExposure             bool `json:"noSecretExposure"`
	ReviewerPassed               bool `json:"reviewerPassed"`
	LearningUsedVerifiedEvidence bool `json:"learningUsedVerifiedEvidence"`
	TokenMetricRecorded          bool `json:"tokenMetricRecorded"`
}

type FlagshipResult struct {
	Passed  bool     `json:"passed"`
	Missing []string `json:"missing,omitempty"`
}

// EvaluateFlagship encodes the acceptance demo as machine-readable gates. A
// successful coding result alone is insufficient if user changes, recovery,
// policy, product verification or verified learning were not also proven.
func EvaluateFlagship(e FlagshipEvidence) FlagshipResult {
	missing := []string{}
	checks := []struct {
		name string
		ok   bool
	}{
		{"dirty_checkout_preserved", e.DirtyCheckoutPreserved},
		{"untracked_user_files_preserved", e.UntrackedUserFilesPreserved},
		{"agent_worktree_isolated", e.AgentWorktreeIsolated},
		{"crash_resume_succeeded", e.CrashResumeSucceeded},
		{"no_duplicate_side_effect", e.NoDuplicateSideEffect},
		{"stale_patch_blocked", e.StalePatchBlocked},
		{"three_way_reconciled", e.ThreeWayReconciled},
		{"required_verification_passed", e.RequiredVerificationPassed},
		{"product_verification_passed", e.ProductVerificationPassed},
		{"policy_passed", e.PolicyPassed},
		{"no_secret_exposure", e.NoSecretExposure},
		{"reviewer_passed", e.ReviewerPassed},
		{"learning_verified", e.LearningUsedVerifiedEvidence},
		{"token_metric_recorded", e.TokenMetricRecorded},
	}
	for _, check := range checks {
		if !check.ok {
			missing = append(missing, check.name)
		}
	}
	sort.Strings(missing)
	return FlagshipResult{Passed: len(missing) == 0, Missing: missing}
}

type ChaosScenario string

const (
	ChaosRuntimeCrash      ChaosScenario = "runtime_crash"
	ChaosProviderLoss      ChaosScenario = "provider_loss"
	ChaosNetworkLoss       ChaosScenario = "network_loss"
	ChaosStalePatch        ChaosScenario = "stale_patch"
	ChaosDirtyCheckout     ChaosScenario = "dirty_checkout"
	ChaosActivationLoss    ChaosScenario = "activation_loss"
	ChaosDuplicateDelivery ChaosScenario = "duplicate_delivery"
	ChaosCorruptTail       ChaosScenario = "corrupt_event_tail"
	ChaosLeaseDoubleSpend  ChaosScenario = "lease_double_spend"
)

func RequiredChaosScenarios() []ChaosScenario {
	return []ChaosScenario{ChaosRuntimeCrash, ChaosProviderLoss, ChaosNetworkLoss, ChaosStalePatch, ChaosDirtyCheckout, ChaosActivationLoss, ChaosDuplicateDelivery, ChaosCorruptTail, ChaosLeaseDoubleSpend}
}
func ChaosCoverage(results []ChaosResult) []string {
	seen := map[string]bool{}
	for _, r := range results {
		if r.Passed {
			seen[r.ID] = true
		}
	}
	missing := []string{}
	for _, id := range RequiredChaosScenarios() {
		if !seen[string(id)] {
			missing = append(missing, string(id))
		}
	}
	sort.Strings(missing)
	return missing
}
