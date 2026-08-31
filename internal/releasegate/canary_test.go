package releasegate

import "testing"

func TestDecideCanaryUsesBoundedPromotionLadder(t *testing.T){
	passGate:=Gate{Passed:true};passFlagship:=FlagshipResult{Passed:true}
	for _,tc:=range []struct{from,to int}{{0,1},{1,10},{10,50},{50,100},{100,100}}{
		decision:=DecideCanary(tc.from,passGate,passFlagship);if decision.ToPercent!=tc.to{t.Fatalf("%d -> %d, want %d",tc.from,decision.ToPercent,tc.to)}
	}
	decision:=DecideCanary(50,Gate{Passed:false,Blockers:[]string{"lost_update_detected"}},passFlagship)
	if decision.Action!=RolloutRollback || decision.ToPercent!=0{t.Fatalf("unsafe canary did not roll back: %+v",decision)}
}
