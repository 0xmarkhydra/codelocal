package releasegate

import (
	"context"
	"errors"
	"testing"
)

func TestChaosHarnessRequiresFullCoverageAndRedactsErrors(t *testing.T){
	handlers:=map[ChaosScenario]ChaosHandler{}
	for _,scenario:=range RequiredChaosScenarios(){s:=scenario;handlers[s]=func(context.Context)(string,error){return string(s)+":ok",nil}}
	handlers[ChaosNetworkLoss]=func(context.Context)(string,error){return "",errors.New("Bearer super-secret-token")}
	results:=NewChaosHarness(handlers).RunRequired(context.Background(),0)
	if len(results)!=len(RequiredChaosScenarios()){t.Fatalf("coverage=%d",len(results))}
	for _,result:=range results{if result.ID==string(ChaosNetworkLoss){if result.Passed{t.Fatal("expected network loss failure")};if result.Evidence=="" || result.Evidence=="Bearer super-secret-token"{t.Fatalf("raw error leaked: %q",result.Evidence)}}}
}
