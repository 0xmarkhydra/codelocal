package releasegate

import "strings"

type RolloutAction string
const(
	RolloutHold RolloutAction="hold"
	RolloutPromote RolloutAction="promote"
	RolloutRollback RolloutAction="rollback"
)

type CanaryDecision struct{
	Action RolloutAction `json:"action"`
	FromPercent int `json:"fromPercent"`
	ToPercent int `json:"toPercent"`
	Reason string `json:"reason"`
}

// DecideCanary advances only through the approved 0→1→10→50→100 ladder.
// Any release or flagship regression immediately selects V1-only rollout (0%).
// Task-level generation remains pinned by taskexecution.SelectRuntimeGeneration.
func DecideCanary(currentPercent int,gate Gate,flagship FlagshipResult)CanaryDecision{
	if currentPercent<0{currentPercent=0};if currentPercent>100{currentPercent=100}
	if !gate.Passed || !flagship.Passed{return CanaryDecision{Action:RolloutRollback,FromPercent:currentPercent,ToPercent:0,Reason:rolloutReason(gate,flagship)}}
	next:=currentPercent
	switch{case currentPercent<1:next=1;case currentPercent<10:next=10;case currentPercent<50:next=50;case currentPercent<100:next=100}
	if next==currentPercent{return CanaryDecision{Action:RolloutHold,FromPercent:currentPercent,ToPercent:currentPercent,Reason:"release_gates_stable"}}
	return CanaryDecision{Action:RolloutPromote,FromPercent:currentPercent,ToPercent:next,Reason:"release_gates_passed"}
}
func rolloutReason(gate Gate,flagship FlagshipResult)string{if !gate.Passed&&len(gate.Blockers)>0{return "gate:"+strings.Join(gate.Blockers,",")};if !flagship.Passed&&len(flagship.Missing)>0{return "flagship:"+strings.Join(flagship.Missing,",")};return "release_gate_failed"}
