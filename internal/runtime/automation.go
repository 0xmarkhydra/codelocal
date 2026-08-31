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
	desktop := automation.ComputerCapabilities()
	mobile := automation.MobileCapabilities()
	boolValue := func(values map[string]any, key string) bool {
		value, _ := values[key].(bool)
		return value
	}
	stringValue := func(values map[string]any, key string) string {
		value, _ := values[key].(string)
		return value
	}
	desktopAvailable := boolValue(desktop, "available")
	mobileAvailable := boolValue(mobile, "available")
	uiTree := boolValue(desktop, "uiTree")
	return protocol.AutomationCapabilities{
		Browser: protocol.BrowserCapabilities{
			Available:       browserAvailable,
			IsolatedProfile: browserAvailable,
			Screenshots:     browserAvailable,
			Devtools:        browserAvailable,
			AttachExisting:  false,
		},
		Computer: protocol.ComputerCapabilities{
			Available:              desktopAvailable || mobileAvailable,
			DesktopAvailable:       desktopAvailable,
			Backend:                stringValue(desktop, "backend"),
			Engine:                 stringValue(desktop, "engine"),
			PersistentEngine:       boolValue(desktop, "persistentEngine"),
			SceneCache:             boolValue(desktop, "sceneCache"),
			BatchActions:           boolValue(desktop, "batchActions"),
			WindowList:             boolValue(desktop, "windowList"),
			ScreenCapture:          boolValue(desktop, "screenCapture"),
			ScreenCaptureStreaming: boolValue(desktop, "screenCaptureStreaming"),
			UITree:                 uiTree,
			SemanticActions:        boolValue(desktop, "semanticActions") || uiTree,
			PhysicalInputFallback:  boolValue(desktop, "physicalInputFallback"),
			Pointer:                boolValue(desktop, "pointer"),
			Keyboard:               boolValue(desktop, "keyboard"),
			Clipboard:              boolValue(desktop, "clipboard"),
			BackgroundControl:      boolValue(desktop, "backgroundControl"),
			SecureDesktop:          false,
			Mobile: &protocol.MobileCapabilities{
				Available: mobileAvailable, Backend: stringValue(mobile, "backend"), Version: stringValue(mobile, "version"), Managed: boolValue(mobile, "managed"),
				IOS: boolValue(mobile, "ios"), Android: boolValue(mobile, "android"), DeviceList: boolValue(mobile, "deviceList"), ScreenCapture: boolValue(mobile, "screenCapture"),
				UITree: boolValue(mobile, "uiTree"), Pointer: boolValue(mobile, "pointer"), Keyboard: boolValue(mobile, "keyboard"), AppLifecycle: boolValue(mobile, "appLifecycle"),
				OpenURL: boolValue(mobile, "openURL"), Orientation: boolValue(mobile, "orientation"), Recording: boolValue(mobile, "recording"), CrashReports: boolValue(mobile, "crashReports"),
			},
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
