package toolprogram

import (
	"context"
	"errors"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestExecuteGuardedRetriesOnlyExplicitlySafeCapability(t *testing.T){
	calls:=0
	capability:=security.ProjectedCapability{CapabilityDescriptor:security.CapabilityDescriptor{ID:"read",Action:"read",Tool:"fs"},Visibility:security.CapabilityAvailable}
	binding:=Binding{Capability:capability,Handler:func(context.Context,map[string]any)(HandlerResult,error){calls++;if calls==1{return HandlerResult{},errors.New("transient")};return HandlerResult{Text:"ok"},nil}}
	runtime,err:=New([]Binding{binding},Limits{MaxOperations:2},nil);if err!=nil{t.Fatal(err)}
	if _,err:=runtime.ExecuteGuarded(context.Background(),Program{ID:"p",Steps:[]Step{{ID:"s",CapabilityID:"read"}}},Guardrails{MaxRetries:1,Retryable:func(security.ProjectedCapability,error)bool{return true}});err!=nil{t.Fatal(err)}
	if calls!=2{t.Fatalf("expected one retry, calls=%d",calls)}
}

func TestExecuteGuardedDoesNotRetryByDefault(t *testing.T){
	calls:=0
	binding:=Binding{Capability:security.ProjectedCapability{CapabilityDescriptor:security.CapabilityDescriptor{ID:"mutate",Action:"write"},Visibility:security.CapabilityAvailable},Handler:func(context.Context,map[string]any)(HandlerResult,error){calls++;return HandlerResult{},errors.New("fail")}}
	runtime,_:=New([]Binding{binding},Limits{},nil)
	_,err:=runtime.ExecuteGuarded(context.Background(),Program{ID:"p",Steps:[]Step{{ID:"s",CapabilityID:"mutate"}}},Guardrails{MaxRetries:3})
	if err==nil || calls!=1{t.Fatalf("unsafe implicit retry: err=%v calls=%d",err,calls)}
}
