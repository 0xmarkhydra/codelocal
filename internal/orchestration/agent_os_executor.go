package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/usage"
)

var (
	ErrAgentOSExecutionBlocked = errors.New("agent os execution blocked")
	ErrAgentOSExecutionFailed  = errors.New("agent os execution failed")
	ErrAgentOSVerification     = errors.New("agent os verification incomplete")
)

type VerificationExecutor func(context.Context, *PreparedAgentOS) ([]VerificationEvidence, error)
type UsageExtractor func(agentruntime.Result) usage.TaskUsage

type AgentOSExecutionRequest struct {
	ProjectID       string
	WorkspaceKey    string
	TaskID          string
	Objective       string
	Context         agentruntime.ContextPacket
	RuntimeProvider string
	MaxParallel     int
	Verify          VerificationExecutor
	UsageFromResult UsageExtractor
}

type AgentExecutionResult struct {
	AgentID  string              `json:"agentId"`
	NodeID   string              `json:"nodeId"`
	EngineID string              `json:"engineId"`
	Result   agentruntime.Result `json:"result"`
}

type AgentOSExecutionReport struct {
	Completed    bool                   `json:"completed"`
	Agents       []AgentExecutionResult `json:"agents"`
	Verification VerificationSnapshot   `json:"verification"`
	Usage        usage.LedgerSnapshot   `json:"usage"`
}

// ExecutePreparedAgentOS is the production dispatch seam for a previously
// admitted V3 plan. Admission/routing/budget happen in PrepareAgentOS; this
// function only executes runnable DAG waves, persists lifecycle transitions,
// settles budget reservations, records verification evidence, and finalizes the
// lead when every required invariant is satisfied.
func ExecutePreparedAgentOS(ctx context.Context, prepared *PreparedAgentOS, engines *agentruntime.Runtime, req AgentOSExecutionRequest) (AgentOSExecutionReport, error) {
	if prepared == nil || prepared.Runtime == nil || engines == nil || engines.Registry == nil { return AgentOSExecutionReport{}, ErrAgentOSExecutionBlocked }
	req.WorkspaceKey = strings.TrimSpace(req.WorkspaceKey)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.Objective = strings.Join(strings.Fields(req.Objective), " ")
	if req.WorkspaceKey == "" || req.TaskID == "" || req.Objective == "" { return AgentOSExecutionReport{}, ErrAgentOSExecutionBlocked }
	if req.RuntimeProvider == "" { req.RuntimeProvider = "engine" }
	parallel := req.MaxParallel
	if parallel <= 0 { parallel = prepared.Team.Parallelism }
	if parallel <= 0 { parallel = 1 }
	if prepared.Team.Parallelism > 0 && parallel > prepared.Team.Parallelism { parallel = prepared.Team.Parallelism }

	report := AgentOSExecutionReport{Agents: []AgentExecutionResult{}}
	byNode := map[string]PlannedAgent{}
	for _, planned := range prepared.Agents { byNode[planned.NodeID] = planned }

	for {
		runnable := prepared.Runtime.DAG().Runnable()
		if len(runnable) == 0 {
			if allTaskNodesCompleted(prepared.Runtime.DAG().Nodes()) { break }
			return finalizeExecutionReport(prepared, report), ErrAgentOSExecutionBlocked
		}
		if len(runnable) > parallel { runnable = runnable[:parallel] }
		type outcome struct { result AgentExecutionResult; err error }
		outcomes := make(chan outcome, len(runnable))
		var wg sync.WaitGroup
		for _, node := range runnable {
			planned, ok := byNode[node.ID]
			if !ok { return finalizeExecutionReport(prepared, report), ErrAgentOSExecutionBlocked }
			wg.Add(1)
			go func(node TaskNode, planned PlannedAgent) {
				defer wg.Done()
				result, err := executePreparedNode(ctx, prepared, engines, req, node, planned)
				outcomes <- outcome{result: result, err: err}
			}(node, planned)
		}
		wg.Wait()
		close(outcomes)
		var firstErr error
		for item := range outcomes {
			if item.result.AgentID != "" { report.Agents = append(report.Agents, item.result) }
			if item.err != nil && firstErr == nil { firstErr = item.err }
		}
		if firstErr != nil { return finalizeExecutionReport(prepared, report), firstErr }
	}

	if req.Verify != nil {
		evidence, err := req.Verify(ctx, prepared)
		if err != nil { return finalizeExecutionReport(prepared, report), err }
		for _, item := range evidence {
			snapshot := prepared.Runtime.Verification().Snapshot()
			if _, err := prepared.Runtime.Verification().Record(snapshot.Revision, item); err != nil { return finalizeExecutionReport(prepared, report), err }
		}
	}
	if !prepared.Runtime.Verification().CanComplete() { return finalizeExecutionReport(prepared, report), ErrAgentOSVerification }
	lead, ok := prepared.Runtime.Graph().Agent(prepared.LeadAgentID)
	if !ok { return finalizeExecutionReport(prepared, report), ErrAgentOSExecutionBlocked }
	if _, err := prepared.Runtime.FinalizeLead(lead.ID, lead.Revision, time.Now().UTC()); err != nil { return finalizeExecutionReport(prepared, report), err }
	report = finalizeExecutionReport(prepared, report)
	report.Completed = true
	return report, nil
}

func executePreparedNode(ctx context.Context, prepared *PreparedAgentOS, engines *agentruntime.Runtime, req AgentOSExecutionRequest, node TaskNode, planned PlannedAgent) (AgentExecutionResult, error) {
	identity, ok := prepared.Runtime.Graph().Agent(planned.AgentID)
	if !ok { return AgentExecutionResult{}, ErrAgentOSExecutionBlocked }
	if identity.Status == agentruntime.AgentCreated || identity.Status == agentruntime.AgentIdle {
		updated, err := prepared.Runtime.Graph().TransitionCAS(identity.ID, identity.Revision, agentruntime.AgentActive, time.Now().UTC())
		if err != nil { return AgentExecutionResult{}, err }
		identity = updated
	}
	if identity.Status != agentruntime.AgentActive { return AgentExecutionResult{}, ErrAgentOSExecutionBlocked }
	activation, err := prepared.Runtime.Activations().Start(agentruntime.ActivationStart{AgentID:identity.ID, RuntimeProvider:req.RuntimeProvider+":"+planned.Route.EngineID})
	if err != nil { return AgentExecutionResult{}, err }
	activation, err = prepared.Runtime.Activations().TransitionCAS(activation.ID, activation.Revision, agentruntime.ActivationRunning, time.Now().UTC())
	if err != nil { return AgentExecutionResult{}, err }
	if _, err := prepared.Runtime.StartNode(node.ID, node.Revision, identity.ID); err != nil { return AgentExecutionResult{}, err }

	mode := agentruntime.ModeMutate
	if planned.Member.ReadOnly || planned.Member.VerificationOnly { mode = agentruntime.ModeReview }
	session, startErr := engines.Start(ctx, planned.Route.EngineID, agentruntime.Request{TaskID:req.TaskID, ProjectID:req.ProjectID, WorkspaceKey:req.WorkspaceKey, Objective:req.Objective, TaskKind:string(planned.Member.Role), Mode:mode, Context:req.Context})
	if startErr != nil {
		markPreparedFailure(prepared, node.ID, identity.ID, activation.ID)
		releaseReservation(prepared, planned.Member.ID)
		return AgentExecutionResult{AgentID:identity.ID,NodeID:node.ID,EngineID:planned.Route.EngineID}, fmt.Errorf("%w: %v",ErrAgentOSExecutionFailed,startErr)
	}
	go func(){ for range session.Events(){} }()
	result, waitErr := session.Wait(ctx)
	if waitErr != nil {
		_ = session.Cancel(context.Background())
		markPreparedFailure(prepared, node.ID, identity.ID, activation.ID)
		settleReservation(prepared, planned.Member.ID, req.UsageFromResult, result)
		return AgentExecutionResult{AgentID:identity.ID,NodeID:node.ID,EngineID:planned.Route.EngineID,Result:result}, fmt.Errorf("%w: %v",ErrAgentOSExecutionFailed,waitErr)
	}
	if current, ok := prepared.Runtime.Activations().Activation(activation.ID); ok { _, _ = prepared.Runtime.Activations().TransitionCAS(current.ID,current.Revision,agentruntime.ActivationCompleted,time.Now().UTC()) }
	currentAgent, ok := prepared.Runtime.Graph().Agent(identity.ID)
	if !ok { return AgentExecutionResult{}, ErrAgentOSExecutionBlocked }
	if currentAgent.Status != agentruntime.AgentCompleted {
		if _, err := prepared.Runtime.Graph().TransitionCAS(currentAgent.ID,currentAgent.Revision,agentruntime.AgentCompleted,time.Now().UTC()); err != nil { return AgentExecutionResult{}, err }
	}
	freshNode, ok := prepared.Runtime.DAG().Node(node.ID)
	if !ok { return AgentExecutionResult{}, ErrAgentOSExecutionBlocked }
	if _, err := prepared.Runtime.CompleteNode(freshNode.ID,freshNode.Revision,identity.ID); err != nil { return AgentExecutionResult{}, err }
	settleReservation(prepared, planned.Member.ID, req.UsageFromResult, result)
	return AgentExecutionResult{AgentID:identity.ID,NodeID:node.ID,EngineID:planned.Route.EngineID,Result:result}, nil
}

func markPreparedFailure(prepared *PreparedAgentOS, nodeID, agentID, activationID string) {
	if activation, ok := prepared.Runtime.Activations().Activation(activationID); ok { _, _ = prepared.Runtime.Activations().TransitionCAS(activation.ID,activation.Revision,agentruntime.ActivationFailed,time.Now().UTC()) }
	if agent, ok := prepared.Runtime.Graph().Agent(agentID); ok && agent.Status != agentruntime.AgentFailed { _, _ = prepared.Runtime.Graph().TransitionCAS(agent.ID,agent.Revision,agentruntime.AgentFailed,time.Now().UTC()) }
	if node, ok := prepared.Runtime.DAG().Node(nodeID); ok && node.Status != TaskNodeFailed { _, _ = prepared.Runtime.DAG().SetStatus(node.ID,node.Revision,TaskNodeFailed) }
}

func reservationFor(prepared *PreparedAgentOS, memberID string) (usage.Reservation, bool) {
	needle := ":" + strings.TrimSpace(memberID)
	for _, reservation := range prepared.Reservations { if strings.HasSuffix(reservation.ID, needle) { return reservation, true } }
	return usage.Reservation{}, false
}
func releaseReservation(prepared *PreparedAgentOS, memberID string) { if reservation,ok:=reservationFor(prepared,memberID); ok { _,_ = prepared.Usage.Release(reservation.ID) } }
func settleReservation(prepared *PreparedAgentOS, memberID string, extractor UsageExtractor, result agentruntime.Result) {
	reservation,ok:=reservationFor(prepared,memberID); if !ok{return}
	actual:=reservation.Usage
	if extractor!=nil { actual=extractor(result); if strings.TrimSpace(actual.TaskID)==""{actual.TaskID=reservation.Usage.TaskID}; if strings.TrimSpace(actual.AgentID)==""{actual.AgentID=reservation.Usage.AgentID} }
	_,_,_ = prepared.Usage.Settle(reservation.ID,actual)
}
func allTaskNodesCompleted(nodes []TaskNode) bool { if len(nodes)==0{return false}; for _,node:=range nodes{if node.Status!=TaskNodeCompleted{return false}}; return true }
func finalizeExecutionReport(prepared *PreparedAgentOS, report AgentOSExecutionReport) AgentOSExecutionReport { if prepared==nil{return report}; report.Verification=prepared.Runtime.Verification().Snapshot(); report.Usage=prepared.Usage.Snapshot(); return report }
