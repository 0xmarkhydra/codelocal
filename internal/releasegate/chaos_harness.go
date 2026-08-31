package releasegate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrChaosHandlerMissing=errors.New("chaos handler missing")

type ChaosHandler func(context.Context)(evidence string,err error)

type ChaosHarness struct{handlers map[ChaosScenario]ChaosHandler}

func NewChaosHarness(handlers map[ChaosScenario]ChaosHandler)*ChaosHarness{
	copyHandlers:=map[ChaosScenario]ChaosHandler{}
	for scenario,handler:=range handlers{if handler!=nil{copyHandlers[scenario]=handler}}
	return &ChaosHarness{handlers:copyHandlers}
}

// RunRequired executes every mandatory recovery scenario in deterministic order.
// Raw errors are never persisted in gate evidence; failures are represented by a
// short stable signature so secrets/provider payloads cannot leak into reports.
func (h *ChaosHarness) RunRequired(parent context.Context,perScenarioTimeout time.Duration)[]ChaosResult{
	if perScenarioTimeout<=0{perScenarioTimeout=30*time.Second};if perScenarioTimeout>5*time.Minute{perScenarioTimeout=5*time.Minute}
	results:=make([]ChaosResult,0,len(RequiredChaosScenarios()))
	for _,scenario:=range RequiredChaosScenarios(){
		handler:=ChaosHandler(nil);if h!=nil{handler=h.handlers[scenario]}
		if handler==nil{results=append(results,ChaosResult{ID:string(scenario),Passed:false,Evidence:"handler_missing"});continue}
		ctx,cancel:=context.WithTimeout(parent,perScenarioTimeout)
		evidence,err:=handler(ctx);cancel()
		if err!=nil{results=append(results,ChaosResult{ID:string(scenario),Passed:false,Evidence:"error:"+chaosErrorSignature(err)});continue}
		evidence=strings.Join(strings.Fields(evidence)," ");if len(evidence)>256{evidence=evidence[:256]}
		if evidence==""{evidence="verified"}
		results=append(results,ChaosResult{ID:string(scenario),Passed:true,Evidence:evidence})
	}
	return results
}

func chaosErrorSignature(err error)string{if err==nil{return ""};sum:=sha256.Sum256([]byte(err.Error()));return hex.EncodeToString(sum[:8])}
