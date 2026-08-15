package runtime

import (
	"context"
	"sync"

	"github.com/0xmarkhydra/codelocal/internal/automation"
	"github.com/0xmarkhydra/codelocal/internal/localclient"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
)

var workspaceAutomation sync.Map // map[*WorkspaceWorker]*automation.Controller

func automationController(w *WorkspaceWorker) *automation.Controller {
	if w == nil {
		return nil
	}
	if existing, ok := workspaceAutomation.Load(w); ok {
		return existing.(*automation.Controller)
	}
	key := w.Runtime.Options.Credential.DeviceID + "::" + w.Workspace.WorkspaceID
	controller := automation.NewController(w.Workspace.WorkspaceID, key, w.Workspace.LocalPath)
	actual, loaded := workspaceAutomation.LoadOrStore(w, controller)
	if loaded {
		controller.Close()
		return actual.(*automation.Controller)
	}
	return controller
}

func workspaceAutomationCapabilities(w *WorkspaceWorker) protocol.AutomationCapabilities {
	controller := automationController(w)
	browserEnabled, browserPrepared := automation.BrowserConfigured()
	browserAvailable := controller != nil && controller.Browser != nil && browserEnabled && browserPrepared
	computerMap := automation.ComputerCapabilities()
	boolValue := func(key string) bool {
		value, _ := computerMap[key].(bool)
		return value
	}
	stringValue := func(key string) string {
		value, _ := computerMap[key].(string)
		return value
	}
	uiTree := boolValue("uiTree")
	return protocol.AutomationCapabilities{
		Browser: protocol.BrowserCapabilities{
			Available:       browserAvailable,
			IsolatedProfile: browserAvailable,
			Screenshots:     browserAvailable,
			Devtools:        browserAvailable,
			AttachExisting:  false,
		},
		Computer: protocol.ComputerCapabilities{
			Available:              boolValue("available"),
			Backend:                stringValue("backend"),
			Engine:                 stringValue("engine"),
			PersistentEngine:       boolValue("persistentEngine"),
			SceneCache:             boolValue("sceneCache"),
			BatchActions:           boolValue("batchActions"),
			WindowList:             boolValue("windowList"),
			ScreenCapture:          boolValue("screenCapture"),
			ScreenCaptureStreaming: boolValue("screenCaptureStreaming"),
			UITree:                 uiTree,
			SemanticActions:        boolValue("semanticActions") || uiTree,
			PhysicalInputFallback:  boolValue("physicalInputFallback"),
			Pointer:                boolValue("pointer"),
			Keyboard:               boolValue("keyboard"),
			Clipboard:              boolValue("clipboard"),
			BackgroundControl:      boolValue("backgroundControl"),
			SecureDesktop:          false,
		},
	}
}

func (w *WorkspaceWorker) handleTool(ctx context.Context, tool string, args map[string]any, opts localclient.HandleOptions) (any, error) {
	if automation.IsTool(tool) {
		return automationController(w).HandleScoped(ctx, tool, args, opts.SessionID)
	}
	return w.Engine.Handle(ctx, tool, args, opts)
}

func closeWorkspaceAutomation(w *WorkspaceWorker) {
	if w == nil {
		return
	}
	if controller, ok := workspaceAutomation.LoadAndDelete(w); ok {
		controller.(*automation.Controller).Close()
	}
}
