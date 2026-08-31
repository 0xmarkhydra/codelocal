package taskexecution

import "testing"

func TestRuntimeGenerationDefaultsToV1(t *testing.T) {
	if (Bundle{}).EffectiveRuntimeGeneration() != RuntimeV1 { t.Fatal("legacy bundle must stay on V1") }
	generation, err := SelectRuntimeGeneration("task", RuntimeRollout{Enabled:false, Percent:100})
	if err != nil || generation != RuntimeV1 { t.Fatalf("disabled rollout moved task: %s %v", generation, err) }
}

func TestRuntimeGenerationDeterministicAndPinnable(t *testing.T) {
	config := RuntimeRollout{Enabled:true, Percent:50}
	first, err := SelectRuntimeGeneration("task-123", config)
	if err != nil { t.Fatal(err) }
	for i:=0; i<10; i++ { next, _ := SelectRuntimeGeneration("task-123", config); if next != first { t.Fatalf("rollout changed bucket: %s != %s", next, first) } }
	bundle, err := PinRuntimeGeneration(Bundle{TaskID:"task-123"}, first)
	if err != nil || bundle.EffectiveRuntimeGeneration() != first { t.Fatalf("pin failed: %+v err=%v", bundle, err) }
	other := RuntimeV1; if first == RuntimeV1 { other = RuntimeV2 }
	if _, err := PinRuntimeGeneration(bundle, other); err == nil { t.Fatal("running task generation changed after pin") }
}

func TestRuntimeGenerationForceAndValidation(t *testing.T) {
	if generation, err := SelectRuntimeGeneration("task", RuntimeRollout{Enabled:true, ForceV2:true}); err != nil || generation != RuntimeV2 { t.Fatalf("force V2 failed: %s %v", generation, err) }
	if _, err := SelectRuntimeGeneration("task", RuntimeRollout{Enabled:true, ForceV1:true, ForceV2:true}); err == nil { t.Fatal("conflicting force flags accepted") }
	if _, err := SelectRuntimeGeneration("task", RuntimeRollout{Enabled:true, Percent:101}); err == nil { t.Fatal("invalid percentage accepted") }
}
