package skills

import (
	"context"
	"errors"
	"testing"
)

type staticPolicyResolver struct {
	result PolicyResult
	err    error
}

func (r staticPolicyResolver) ResolveSkillPolicy(context.Context, PolicyRequest) (PolicyResult, error) {
	return r.result, r.err
}

func TestKnowledgeSkillAutoAuthorizesWithoutRuntime(t *testing.T) {
	manifest := BuiltinManifests()[0]
	result, err := AuthorizeExecution(context.Background(), manifest, nil)
	if err != nil || result.Decision != PolicyAllow {
		t.Fatalf("knowledge skill should not require runtime approval: result=%#v err=%v", result, err)
	}
}

func TestRuntimeSkillFailsClosedWithoutAuthorizationAdapter(t *testing.T) {
	manifest := Manifest{ID: "deploy", Name: "Deploy", Version: "1.0.0", Publisher: "test", Scope: ScopeCommunity, Kind: KindRuntime, Quality: 1, Capabilities: []Capability{CapabilityShell, CapabilityNetwork}}
	result, err := AuthorizeExecution(context.Background(), manifest, nil)
	if !errors.Is(err, ErrSkillAuthorizationUnavailable) || result.Decision != PolicyDeny {
		t.Fatalf("runtime skill must fail closed: result=%#v err=%v", result, err)
	}
}

func TestRuntimeSkillUsesAuthoritativeResolver(t *testing.T) {
	manifest := Manifest{ID: "deploy", Name: "Deploy", Version: "1.0.0", Publisher: "test", Scope: ScopeCommunity, Kind: KindRuntime, Quality: 1, Capabilities: []Capability{CapabilityShell}}
	result, err := AuthorizeExecution(context.Background(), manifest, staticPolicyResolver{result: PolicyResult{Decision: PolicyRequireApproval, Reason: "workspace policy"}})
	if err != nil || result.Decision != PolicyRequireApproval {
		t.Fatalf("expected workspace policy decision to pass through: result=%#v err=%v", result, err)
	}
}

func TestRuntimeSkillResolverFailureDeniesExecution(t *testing.T) {
	manifest := Manifest{ID: "deploy", Name: "Deploy", Version: "1.0.0", Publisher: "test", Scope: ScopeCommunity, Kind: KindRuntime, Quality: 1, Capabilities: []Capability{CapabilityShell}}
	resolverErr := errors.New("runtime unavailable")
	result, err := AuthorizeExecution(context.Background(), manifest, staticPolicyResolver{err: resolverErr})
	if !errors.Is(err, resolverErr) || result.Decision != PolicyDeny {
		t.Fatalf("resolver error must deny execution: result=%#v err=%v", result, err)
	}
}
