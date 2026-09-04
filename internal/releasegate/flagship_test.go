package releasegate

import "testing"

func TestFlagshipRequiresEverySafetyAndQualityProof(t *testing.T) {
	evidence := FlagshipEvidence{DirtyCheckoutPreserved:true,UntrackedUserFilesPreserved:true,AgentWorktreeIsolated:true,CrashResumeSucceeded:true,NoDuplicateSideEffect:true,StalePatchBlocked:true,ThreeWayReconciled:true,RequiredVerificationPassed:true,ProductVerificationPassed:true,PolicyPassed:true,NoSecretExposure:true,ReviewerPassed:true,LearningUsedVerifiedEvidence:true,TokenMetricRecorded:true}
	result := EvaluateFlagship(evidence)
	if !result.Passed || len(result.Missing)!=0 { t.Fatalf("complete flagship failed: %+v",result) }
	evidence.NoSecretExposure=false
	result=EvaluateFlagship(evidence)
	if result.Passed || len(result.Missing)!=1 || result.Missing[0]!="no_secret_exposure" { t.Fatalf("missing proof not detected: %+v",result) }
}

func TestChaosCoverageListsRequiredMissingScenarios(t *testing.T) {
	results:=[]ChaosResult{}
	for _,id:=range RequiredChaosScenarios(){ results=append(results,ChaosResult{ID:string(id),Passed:true}) }
	if missing:=ChaosCoverage(results); len(missing)!=0 { t.Fatalf("complete chaos coverage reported missing: %+v",missing) }
	results[0].Passed=false
	if missing:=ChaosCoverage(results); len(missing)!=1 || missing[0]!=string(ChaosRuntimeCrash) { t.Fatalf("failed chaos scenario hidden: %+v",missing) }
}
