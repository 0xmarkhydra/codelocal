package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
	"github.com/0xmarkhydra/codelocal/internal/identity"
	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	"github.com/0xmarkhydra/codelocal/internal/localclient"
	"github.com/0xmarkhydra/codelocal/internal/mediatransport"
	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	"github.com/0xmarkhydra/codelocal/internal/security"
	usagecalc "github.com/0xmarkhydra/codelocal/internal/usage"
	"github.com/0xmarkhydra/codelocal/internal/version"
	"github.com/0xmarkhydra/codelocal/internal/workspace"
	"github.com/coder/websocket"
)

type Options struct {
	BaseURL       string
	Credential    identity.Credential
	LongPoll      time.Duration
	IdleWorkspace time.Duration
	OnReady       func()
}

type Runtime struct {
	Options                 Options
	Registry                *workspace.Registry
	client                  *http.Client
	mu                      sync.Mutex
	workers                 map[string]*WorkspaceWorker
	projectIdentities       map[string]projectidentity.Snapshot
	knowledgeManifests      map[string]projectbrain.Manifest
	knowledgeBaseRevisions  map[string]map[string]string
	syncedKnowledgeRoots    map[string]string
	brainSync               *projectbrain.SyncStateStore
	knowledgeMu             sync.Mutex
	brainCloudSyncEnabled   bool
	lastControlPlaneSync    int64
	stopped                 bool
	pollCancel              context.CancelFunc
	syncedRegistrySignature string
	syncedSignature         string
	mediaOnce               sync.Once
	mediaPublisher          *mediatransport.Publisher
	runtimeSettings         map[string]cloud.RuntimeMaterializedConfig
}

type WorkspaceWorker struct {
	Runtime   *Runtime
	Workspace workspace.Workspace
	Engine    *localclient.Engine
	conn      *websocket.Conn
	writeMu   sync.Mutex
	sideMu    sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	stopOnce  sync.Once
	lastUsed  atomic.Int64
	callsMu   sync.Mutex
	calls     map[string]context.CancelFunc
}

type pollResponse struct {
	Activation *cloud.WorkspaceActivation `json:"activation"`
	Revocation *cloud.WorkspaceRevocation `json:"revocation"`
	Now        int64                      `json:"now"`
}

var ErrDeviceAuthorizationRevoked = errors.New("CodeLocal runtime device authorization was revoked; run `codelocal login` to sign in again")

func New(options Options) *Runtime {
	if options.LongPoll <= 0 {
		options.LongPoll = 25 * time.Second
	}
	if options.LongPoll > 30*time.Second {
		options.LongPoll = 30 * time.Second
	}
	if options.IdleWorkspace <= 0 {
		options.IdleWorkspace = 20 * time.Minute
	}
	return &Runtime{Options: options, Registry: workspace.New(), client: &http.Client{Timeout: 45 * time.Second}, workers: map[string]*WorkspaceWorker{}, projectIdentities: map[string]projectidentity.Snapshot{}, knowledgeManifests: map[string]projectbrain.Manifest{}, knowledgeBaseRevisions: map[string]map[string]string{}, syncedKnowledgeRoots: map[string]string{}, brainSync: projectbrain.NewSyncStateStore(), brainCloudSyncEnabled: false, runtimeSettings: map[string]cloud.RuntimeMaterializedConfig{}}
}
func normalizeBase(value string) string { return strings.TrimRight(value, "/") }
func wsURL(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/client"
	u.RawQuery = ""
	return u.String()
}
func (r *Runtime) headers(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CodeLocal-Credential-Id", r.Options.Credential.CredentialID)
	req.Header.Set("Authorization", "Device "+r.Options.Credential.CredentialSecret)
}
func (r *Runtime) post(ctx context.Context, path string, input any, output any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeBase(r.Options.BaseURL)+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	r.headers(req)
	if err := deviceauth.SignRequest(req, raw, r.Options.Credential.DevicePrivateKey, time.Now()); err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return ErrDeviceAuthorizationRevoked
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CodeLocal Cloud %s failed (%d)", path, resp.StatusCode)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}

func registrySignature(items []workspace.Workspace) string {
	parts := make([]string, 0, len(items))
	for _, w := range items {
		parts = append(parts, w.WorkspaceID+"\x00"+w.WorkspaceName+"\x00"+w.LocalPath)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func learnedSkillMetadataSnapshot(store *learnedskills.Store, deviceID, workspaceID string) ([]cloud.LearnedSkillMetadata, string) {
	items := []cloud.LearnedSkillMetadata{}
	signature := []string{}
	if store == nil {
		return items, ""
	}
	recipes, err := store.List(deviceID+"::"+workspaceID, 128)
	if err != nil {
		return items, ""
	}
	for _, recipe := range recipes {
		metadata := cloud.LearnedSkillMetadata{
			ID: recipe.ID, Intent: recipe.Intent, TaskKind: recipe.TaskKind, Status: string(recipe.Status), Confidence: recipe.Confidence,
			SuccessCount: recipe.SuccessCount, FailureCount: recipe.FailureCount, StepCount: len(recipe.Steps), UpdatedAt: recipe.UpdatedAt, LastUsedAt: recipe.LastUsedAt,
		}
		portableID := ""
		if recipe.Context != nil {
			if portable, ok, _ := learnedskills.PortableRecipeFor(recipe, recipe.Context.ProjectID); ok {
				metadata.Portable = &portable
				portableID = portable.ID
			}
		}
		items = append(items, metadata)
		signature = append(signature, fmt.Sprintf("%s:%s:%s:%d:%d:%d:%d:%d", recipe.ID, portableID, recipe.Status, recipe.SuccessCount, recipe.FailureCount, len(recipe.Steps), recipe.UpdatedAt, recipe.LastUsedAt))
	}
	sort.Strings(signature)
	return items, strings.Join(signature, "|")
}

type workspaceSyncResponse struct {
	Knowledge       map[string]cloud.KnowledgeManifestSyncResult `json:"knowledge"`
	PortableSkills  map[string][]learnedskills.PortableRecipe    `json:"portableSkills"`
	RuntimeSettings map[string]cloud.RuntimeMaterializedConfig   `json:"runtimeSettings"`
	ProjectBrain    *struct {
		CloudSyncEnabled bool `json:"cloudSyncEnabled"`
	} `json:"projectBrain,omitempty"`
}

func portableSkillImportAllowed(recipe learnedskills.PortableRecipe) bool {
	if recipe.Evidence == nil {
		return true
	}
	switch recipe.Evidence.Health {
	case "stale", "degraded":
		return false
	default:
		return true
	}
}

const controlPlaneRefreshInterval = 5 * time.Minute

func controlPlaneRefreshDue(lastSync, now int64) bool {
	return lastSync <= 0 || now-lastSync >= controlPlaneRefreshInterval.Milliseconds()
}

func (r *Runtime) projectBrainCloudEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.brainCloudSyncEnabled
}

func (r *Runtime) setProjectBrainCloudEnabled(value bool) {
	r.mu.Lock()
	r.brainCloudSyncEnabled = value
	r.mu.Unlock()
}

func copyRevisionMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (r *Runtime) SyncRegistry(ctx context.Context, force bool) ([]workspace.Workspace, error) {
	items, err := r.Registry.List()
	if err != nil {
		return nil, err
	}
	registrySig := registrySignature(items)
	skillStore := learnedskills.New()
	skillVersions := make([]string, 0, len(items))
	for _, item := range items {
		workspaceKey := r.Options.Credential.DeviceID + "::" + item.WorkspaceID
		skillVersions = append(skillVersions, item.WorkspaceID+"="+skillStore.MetadataVersion(workspaceKey))
	}
	sort.Strings(skillVersions)

	authorized := make(map[string]struct{}, len(items))
	authorizedIDs := make([]string, 0, len(items))
	for _, item := range items {
		authorized[item.WorkspaceID] = struct{}{}
		authorizedIDs = append(authorizedIDs, item.WorkspaceID)
	}
	staleWorkers := make([]*WorkspaceWorker, 0)
	r.mu.Lock()
	for id, worker := range r.workers {
		if _, ok := authorized[id]; !ok {
			staleWorkers = append(staleWorkers, worker)
		}
	}
	cachedIdentities := make(map[string]projectidentity.Snapshot, len(r.projectIdentities))
	for id, snapshot := range r.projectIdentities {
		cachedIdentities[id] = snapshot
	}
	r.mu.Unlock()
	for _, worker := range staleWorkers {
		go worker.Stop("workspace authorization removed")
	}

	// Project identity is durable local metadata. Do not rediscover every
	// authorized workspace on a timer: sleeping workspaces may be large and a
	// repository walk on the runtime heartbeat is unnecessary. Reuse memory,
	// then the private Project Brain state, and discover only a workspace that
	// has no durable identity yet. Active workspaces refresh identity separately.
	nextIdentities := make(map[string]projectidentity.Snapshot, len(items))
	for _, w := range items {
		identity, cached := cachedIdentities[w.WorkspaceID]
		var syncState projectbrain.WorkspaceSyncState
		var haveSyncState bool
		if !cached && r.brainSync != nil {
			if stored, ok, loadErr := r.brainSync.Workspace(w.WorkspaceID); loadErr != nil {
				slog.Debug("project identity cache unreadable; rediscovering workspace", "workspaceId", w.WorkspaceID, "error", loadErr)
			} else if ok {
				syncState, haveSyncState = stored, true
				if projectIdentityPresent(stored.ProjectIdentity) {
					identity, cached = stored.ProjectIdentity, true
				}
			}
		}
		if !cached {
			identity = projectidentity.Discover(w.LocalPath, w.WorkspaceName)
			if r.brainSync != nil {
				if !haveSyncState {
					syncState = projectbrain.WorkspaceSyncState{WorkspaceID: w.WorkspaceID, BaseRevisions: map[string]string{}}
				}
				syncState.WorkspaceID = w.WorkspaceID
				syncState.ProjectIdentity = identity
				if persistErr := r.brainSync.Put(syncState); persistErr != nil {
					slog.Debug("project identity cache persistence failed; runtime remains usable", "workspaceId", w.WorkspaceID, "error", persistErr)
				}
			}
		}
		nextIdentities[w.WorkspaceID] = identity
	}
	signature := registrySig + "\nlearned-skills\n" + strings.Join(skillVersions, "\n")
	now := time.Now().UnixMilli()
	r.mu.Lock()
	unchanged := signature == r.syncedSignature
	controlRefreshDue := controlPlaneRefreshDue(r.lastControlPlaneSync, now)
	r.mu.Unlock()
	if !force && unchanged && !controlRefreshDue {
		return items, nil
	}

	workspaces := make([]map[string]any, 0, len(items))
	for _, w := range items {
		skillMetadata, _ := learnedSkillMetadataSnapshot(skillStore, r.Options.Credential.DeviceID, w.WorkspaceID)
		workspaces = append(workspaces, map[string]any{
			"workspaceId": w.WorkspaceID, "workspaceName": w.WorkspaceName,
			"projectIdentity": nextIdentities[w.WorkspaceID], "learnedSkills": skillMetadata,
		})
	}
	payload := map[string]any{"clientVersion": version.Version, "workspaces": workspaces}
	var response workspaceSyncResponse
	if err := r.post(ctx, "/api/client/workspaces/sync", payload, &response); err != nil {
		return nil, err
	}
	r.applyRuntimeSettings(response.RuntimeSettings)

	// Pull portable project recipes only into already-active workspaces. Sleeping
	// projects stay asleep, and imported recipes remain non-replayable until a
	// successful local execution replaces them with a verified candidate.
	r.mu.Lock()
	activeWorkers := make(map[string]*WorkspaceWorker, len(r.workers))
	for id, worker := range r.workers {
		activeWorkers[id] = worker
	}
	r.mu.Unlock()
	for workspaceID, recipes := range response.PortableSkills {
		worker := activeWorkers[workspaceID]
		if worker == nil || worker.Engine == nil || worker.Engine.Skills == nil {
			continue
		}
		workspaceKey := r.Options.Credential.DeviceID + "::" + workspaceID
		contexts := map[string]*learnedskills.ContextFingerprint{}
		imported := 0
		for _, portable := range recipes {
			if !portableSkillImportAllowed(portable) {
				continue
			}
			taskKind := strings.TrimSpace(portable.TaskKind)
			localContext, cached := contexts[taskKind]
			if !cached {
				localContext = worker.Engine.LearnedSkillContext(taskKind)
				contexts[taskKind] = localContext
			}
			if localContext == nil {
				continue
			}
			_, changed, importErr := worker.Engine.Skills.ImportPortable(workspaceKey, portable.ProjectID, portable, localContext)
			if importErr != nil {
				slog.Debug("portable learned skill import skipped", "workspaceId", workspaceID, "error", importErr)
				continue
			}
			if changed {
				imported++
			}
		}
		if imported > 0 {
			slog.Info("portable learned skills imported as untrusted local suggestions", "workspaceId", workspaceID, "count", imported)
		}
	}

	r.mu.Lock()
	r.syncedRegistrySignature = registrySig
	r.syncedSignature = signature
	r.projectIdentities = nextIdentities
	r.lastControlPlaneSync = now
	if response.ProjectBrain != nil {
		r.brainCloudSyncEnabled = response.ProjectBrain.CloudSyncEnabled
	}
	r.mu.Unlock()
	if r.brainSync != nil {
		if err := r.brainSync.Prune(authorizedIDs); err != nil {
			slog.Debug("project brain sync-state prune failed; runtime remains usable", "error", err)
		}
	}
	return items, nil
}

func (r *Runtime) Status() map[string]any {
	items, _ := r.Registry.List()
	r.mu.Lock()
	active := []map[string]any{}
	for _, worker := range r.workers {
		active = append(active, map[string]any{"workspaceId": worker.Workspace.WorkspaceID, "workspaceName": worker.Workspace.WorkspaceName, "lastUsedAt": worker.lastUsed.Load()})
	}
	r.mu.Unlock()
	return map[string]any{"authorizedWorkspaces": items, "activeWorkspaces": active}
}

func (r *Runtime) Activate(ctx context.Context, workspaceID string) (*WorkspaceWorker, error) {
	r.mu.Lock()
	if existing := r.workers[workspaceID]; existing != nil {
		existing.lastUsed.Store(time.Now().UnixMilli())
		r.mu.Unlock()
		return existing, nil
	}
	r.mu.Unlock()
	entry, err := r.Registry.Get(workspaceID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("workspace is not authorized on this machine: %s", workspaceID)
	}
	worker, err := newWorkspaceWorker(r, *entry)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if existing := r.workers[workspaceID]; existing != nil {
		r.mu.Unlock()
		worker.Stop("duplicate activation")
		return existing, nil
	}
	r.workers[workspaceID] = worker
	r.mu.Unlock()
	if err := r.Registry.MarkActivated(workspaceID); err != nil {
		slog.Warn("failed to persist activation time", "workspaceId", workspaceID, "error", err)
	}
	if err := worker.Start(ctx); err != nil {
		r.mu.Lock()
		if r.workers[workspaceID] == worker {
			delete(r.workers, workspaceID)
		}
		r.mu.Unlock()
		worker.Stop("activation failed")
		return nil, err
	}
	// Project Brain is intentionally outside the activation critical path. The
	// workspace is already usable once the worker has registered; knowledge
	// discovery/sync may retry independently if Cloud or the local cache fails.
	go func(active workspace.Workspace) {
		syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := r.syncKnowledgeWorkspace(syncCtx, active, true); err != nil {
			slog.Debug("project brain activation sync delayed; workspace remains usable", "workspaceId", active.WorkspaceID, "error", err)
		}
	}(worker.Workspace)
	return worker, nil
}

func newWorkspaceWorker(r *Runtime, w workspace.Workspace) (*WorkspaceWorker, error) {
	key := r.Options.Credential.DeviceID + "::" + w.WorkspaceID
	engine, err := localclient.New(w.LocalPath, w.WorkspaceID, w.WorkspaceName, key, r.Options.Credential.DeviceID)
	if err != nil {
		return nil, err
	}
	setting := r.runtimeSetting(w.WorkspaceID)
	engine.SetRuntimeEnvironment(runtimeConfigEnvironment(setting.Snapshot, setting.Secrets), runtimeSecretRedactValues(setting.Secrets))
	worker := &WorkspaceWorker{Runtime: r, Workspace: w, Engine: engine, done: make(chan struct{}), calls: map[string]context.CancelFunc{}}
	worker.lastUsed.Store(time.Now().UnixMilli())
	return worker, nil
}

func (w *WorkspaceWorker) Start(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	target := wsURL(w.Runtime.Options.BaseURL)
	headers := http.Header{}
	if w.Runtime.Options.Credential.DevicePrivateKey != "" {
		signed, err := deviceauth.SignatureHeaders(http.MethodGet, target, nil, w.Runtime.Options.Credential.DevicePrivateKey, time.Now())
		if err != nil {
			return err
		}
		headers = signed
	}
	conn, _, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPHeader: headers, CompressionMode: websocket.CompressionContextTakeover})
	if err != nil {
		return err
	}
	conn.SetReadLimit(32 << 20)
	w.conn = conn
	register := protocol.RegisterMessage{Type: "register", ProtocolVersion: protocol.Version, ClientVersion: version.Version, CredentialID: w.Runtime.Options.Credential.CredentialID, CredentialSecret: w.Runtime.Options.Credential.CredentialSecret, DeviceID: w.Runtime.Options.Credential.DeviceID, DeviceName: w.Runtime.Options.Credential.DeviceName, WorkspaceID: w.Workspace.WorkspaceID, WorkspaceName: w.Workspace.WorkspaceName, ProjectRoot: w.Workspace.LocalPath, Capabilities: protocol.Capabilities{Filesystem: true, Git: true, Shell: w.Engine.ShellEnabled, PTY: w.Engine.ShellEnabled, Sandbox: "policy-only", SemanticProviders: w.Engine.SemanticProviders(), Idempotency: true, Cancellation: true, Approvals: true, ApprovalMemory: true, HostPolicyExecution: true, MCPHub: true, TerminalChatApproval: true, TerminalHistory: true, LearnedSkills: true, Automation: workspaceAutomationCapabilities(w)}}
	if err := w.send(ctx, register); err != nil {
		conn.Close(websocket.StatusInternalError, "register failed")
		return err
	}
	registeredCtx, cancelRegister := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := conn.Read(registeredCtx)
	cancelRegister()
	if err != nil {
		return err
	}
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil || response["type"] != "registered" {
		return errors.New("CodeLocal Cloud rejected workspace registration")
	}
	go w.loop(ctx)
	go w.idleLoop(ctx)
	return nil
}

func (w *WorkspaceWorker) send(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.Write(ctx, websocket.MessageText, raw)
}
func (w *WorkspaceWorker) loop(ctx context.Context) {
	defer w.Stop("workspace connection ended")
	for {
		_, data, err := w.conn.Read(ctx)
		if err != nil {
			return
		}
		var envelope struct {
			Type           string         `json:"type"`
			RequestID      string         `json:"requestId"`
			ID             string         `json:"id"`
			SessionID      string         `json:"sessionId"`
			WorkspaceKey   string         `json:"workspaceKey"`
			Tool           string         `json:"tool"`
			Args           map[string]any `json:"args"`
			IdempotencyKey string         `json:"idempotencyKey"`
			Reason         string         `json:"reason"`
			Deadline       int64          `json:"deadline"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			continue
		}
		switch envelope.Type {
		case "ping":
			_ = w.send(ctx, map[string]any{"type": "pong", "protocolVersion": protocol.Version, "ts": time.Now().UnixMilli()})
		case "tool_cancel":
			requestID := first(envelope.RequestID, envelope.ID)
			w.callsMu.Lock()
			cancel := w.calls[requestID]
			w.callsMu.Unlock()
			if cancel != nil {
				cancel()
			}
		case "tool_call":
			go w.handleCall(ctx, envelope)
		}
	}
}
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

const (
	terminalANSIReset   = "\x1b[0m"
	terminalANSIBold    = "\x1b[1m"
	terminalANSIGray    = "\x1b[90m"
	terminalANSICyan    = "\x1b[36m"
	terminalANSIGreen   = "\x1b[32m"
	terminalANSIRed     = "\x1b[31m"
	terminalANSIMagenta = "\x1b[35m"
)

func terminalTraceColor(code, value string) string {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return value
	}
	return code + value + terminalANSIReset
}

func terminalTraceDim(value string) string { return terminalTraceColor(terminalANSIGray, value) }

func compactTerminalText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if maxRunes > 1 && len(runes) > maxRunes {
		return string(runes[:maxRunes-1]) + "…"
	}
	return value
}

func terminalToolContext(w workspace.Workspace) string {
	name := strings.TrimSpace(w.WorkspaceName)
	id := strings.TrimSpace(w.WorkspaceID)
	if name != "" && id != "" && name != id {
		return compactTerminalText(name+" · "+id, 48)
	}
	return compactTerminalText(first(name, id, "workspace"), 48)
}

func terminalToolDetail(args map[string]any) string {
	if value, ok := args["command"].(string); ok && strings.TrimSpace(value) != "" {
		return "  $ " + compactTerminalText(security.RedactCommand(value), 64)
	}
	if value, ok := args["path"].(string); ok && strings.TrimSpace(value) != "" {
		return "  " + compactTerminalText(value, 58)
	}
	if value, ok := args["query"].(string); ok && strings.TrimSpace(value) != "" {
		return "  “" + compactTerminalText(value, 58) + "”"
	}
	if value, ok := args["name"].(string); ok && strings.TrimSpace(value) != "" {
		return "  " + compactTerminalText(value, 58)
	}
	if value, ok := args["processId"].(string); ok && strings.TrimSpace(value) != "" {
		return "  process " + compactTerminalText(value, 8)
	}
	return ""
}

func terminalTimestamp(value time.Time) string {
	return value.Format("01/02/2006 - 15:04")
}

func (w *WorkspaceWorker) handleCall(parent context.Context, msg struct {
	Type           string         `json:"type"`
	RequestID      string         `json:"requestId"`
	ID             string         `json:"id"`
	SessionID      string         `json:"sessionId"`
	WorkspaceKey   string         `json:"workspaceKey"`
	Tool           string         `json:"tool"`
	Args           map[string]any `json:"args"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Reason         string         `json:"reason"`
	Deadline       int64          `json:"deadline"`
}) {
	requestID := first(msg.RequestID, msg.ID)
	if requestID == "" {
		return
	}
	ctx := parent
	var cancel context.CancelFunc
	if msg.Deadline > 0 {
		ctx, cancel = context.WithDeadline(parent, time.UnixMilli(msg.Deadline))
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	w.callsMu.Lock()
	if old := w.calls[requestID]; old != nil {
		old()
	}
	w.calls[requestID] = cancel
	w.callsMu.Unlock()
	defer func() { cancel(); w.callsMu.Lock(); delete(w.calls, requestID); w.callsMu.Unlock() }()
	w.lastUsed.Store(time.Now().UnixMilli())
	side := protocol.SideEffecting(msg.Tool)
	if side {
		w.sideMu.Lock()
		defer w.sideMu.Unlock()
	}
	inputBytes, inputTokens := usagecalc.EstimateTokens(msg.Args)
	contextLabel := terminalToolContext(w.Workspace)
	detail := terminalToolDetail(msg.Args)
	if detail != "" {
		detail = terminalTraceDim(detail)
	}
	fmt.Printf("  %s %s %s %s%s\n",
		terminalTraceColor(terminalANSIMagenta, "◆"),
		terminalTraceDim(contextLabel),
		terminalTraceDim("›"),
		terminalTraceColor(terminalANSICyan+terminalANSIBold, msg.Tool),
		detail,
	)
	slog.Debug("MCP tool received", "requestId", requestID, "sessionId", msg.SessionID, "workspace", w.Workspace.WorkspaceID, "tool", msg.Tool)
	startedAt := time.Now()
	result, err := w.handleTool(ctx, msg.Tool, msg.Args, localclient.HandleOptions{RequestID: requestID, SessionID: msg.SessionID, IdempotencyKey: msg.IdempotencyKey})
	if err == nil {
		var mediaErr error
		result, mediaErr = w.Runtime.prepareOutboundMedia(ctx, result)
		if mediaErr != nil {
			err = mediaErr
		}
	}
	response := protocol.ToolResult{Type: "tool_result", ProtocolVersion: protocol.Version, RequestID: requestID, OK: err == nil, Result: result}
	if err != nil {
		response.ErrorCode = normalizeErrorCode(err)
		response.ErrorMessage = err.Error()
	}
	durationMs := time.Since(startedAt).Milliseconds()
	response.Metadata = map[string]any{"runtimeDurationMs": durationMs}
	outputBytes, outputTokens := usagecalc.EstimateTokens(response)
	totalTokens := inputTokens + outputTokens
	stamp := terminalTimestamp(time.Now())
	usage := fmt.Sprintf(" - %d token", totalTokens)
	if err == nil {
		fmt.Printf("  %s %s %s %s  %s - %s%s\n",
			terminalTraceColor(terminalANSIGreen, "✓"),
			terminalTraceDim(contextLabel),
			terminalTraceDim("›"),
			terminalTraceColor(terminalANSICyan+terminalANSIBold, msg.Tool),
			terminalTraceDim(fmt.Sprintf("%dms", durationMs)),
			terminalTraceDim(stamp),
			terminalTraceDim(usage),
		)
	} else {
		fmt.Printf("  %s %s %s %s  %s - %s%s  %s\n",
			terminalTraceColor(terminalANSIRed, "✕"),
			terminalTraceDim(contextLabel),
			terminalTraceDim("›"),
			terminalTraceColor(terminalANSICyan+terminalANSIBold, msg.Tool),
			terminalTraceDim(fmt.Sprintf("%dms", durationMs)),
			terminalTraceDim(stamp),
			terminalTraceDim(usage),
			terminalTraceColor(terminalANSIRed, compactTerminalText(err.Error(), 72)),
		)
	}
	slog.Debug("MCP tool completed", "requestId", requestID, "sessionId", msg.SessionID, "workspace", w.Workspace.WorkspaceID, "tool", msg.Tool, "inputBytes", inputBytes, "outputBytes", outputBytes, "inputTokensEstimated", inputTokens, "outputTokensEstimated", outputTokens, "totalTokensEstimated", totalTokens, "durationMs", durationMs)
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = w.send(sendCtx, response)
	sendCancel()
}
func normalizeErrorCode(err error) string {
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "sensitive"):
		return "SENSITIVE_PATH"
	case strings.Contains(lower, "escape") || strings.Contains(lower, "unsafe patch path"):
		return "PATH_ESCAPE"
	case strings.Contains(lower, "changed since read") || strings.Contains(lower, "hash mismatch") || strings.Contains(lower, "conflict"):
		return "CONFLICT"
	case strings.Contains(lower, "duplicate") && strings.Contains(lower, "request"):
		return "DUPLICATE_REQUEST"
	case strings.Contains(lower, "approval") && (strings.Contains(lower, "denied") || strings.Contains(lower, "rejected")):
		return "APPROVAL_DENIED"
	case strings.Contains(lower, "blocked") || strings.Contains(lower, "policy"):
		return "POLICY_BLOCKED"
	case strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return "TOOL_TIMEOUT"
	case strings.Contains(lower, "cancel"):
		return "TOOL_CANCELLED"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "enoent") || strings.Contains(lower, "unknown process"):
		return "NOT_FOUND"
	case strings.Contains(lower, "unsupported") || strings.Contains(lower, "not available"):
		return "UNSUPPORTED"
	default:
		return "TOOL_FAILED"
	}
}
func (w *WorkspaceWorker) idleLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(time.UnixMilli(w.lastUsed.Load())) >= w.Runtime.Options.IdleWorkspace {
				w.Stop("workspace idle")
				return
			}
		}
	}
}
func (w *WorkspaceWorker) Stop(reason string) {
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		w.callsMu.Lock()
		for _, cancel := range w.calls {
			cancel()
		}
		w.calls = map[string]context.CancelFunc{}
		w.callsMu.Unlock()
		if w.conn != nil {
			_ = w.conn.Close(websocket.StatusNormalClosure, reason)
		}
		closeWorkspaceAutomation(w)
		if w.Engine != nil {
			w.Engine.Close()
		}
		close(w.done)
		w.Runtime.mu.Lock()
		if w.Runtime.workers[w.Workspace.WorkspaceID] == w {
			delete(w.Runtime.workers, w.Workspace.WorkspaceID)
		}
		w.Runtime.mu.Unlock()
	})
}

func (r *Runtime) poll(ctx context.Context) (pollResponse, error) {
	waitMs := r.Options.LongPoll.Milliseconds()
	var response pollResponse
	pollCtx, cancel := context.WithTimeout(ctx, r.Options.LongPoll+10*time.Second)
	r.mu.Lock()
	r.pollCancel = cancel
	r.mu.Unlock()
	defer func() { cancel(); r.mu.Lock(); r.pollCancel = nil; r.mu.Unlock() }()
	items, _ := r.Registry.List()
	ids := make([]string, 0, len(items))
	for _, w := range items {
		ids = append(ids, w.WorkspaceID)
	}
	err := r.post(pollCtx, "/api/client/runtime/poll", map[string]any{"workspaceIds": ids, "waitMs": waitMs}, &response)
	return response, err
}
func (r *Runtime) ackRevocation(ctx context.Context, requestID, workspaceID string) {
	_ = r.post(ctx, "/api/client/runtime/revocation-ack", map[string]any{"requestId": requestID, "workspaceId": workspaceID}, &map[string]any{})
}

func (r *Runtime) Run(ctx context.Context) error {
	brainCtx, cancelBrain := context.WithCancel(ctx)
	defer cancelBrain()
	go r.runKnowledgeSyncLoop(brainCtx)
	return r.runRealtime(ctx)
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	if r.pollCancel != nil {
		r.pollCancel()
	}
	workers := make([]*WorkspaceWorker, 0, len(r.workers))
	for _, worker := range r.workers {
		workers = append(workers, worker)
	}
	r.mu.Unlock()
	for _, worker := range workers {
		worker.Stop("runtime stopped")
	}
}
