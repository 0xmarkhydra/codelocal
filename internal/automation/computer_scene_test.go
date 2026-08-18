package automation

import (
	"testing"
	"time"
)

func newSceneTestController() *ComputerController {
	return &ComputerController{
		scene:      map[string]sceneCacheEntry{},
		sceneState: map[string]computerSceneState{},
	}
}

func TestSceneEventInvalidatesOnlyTargetWindowAndAdvancesGeneration(t *testing.T) {
	controller := newSceneTestController()
	controller.scene["ax:1:0"] = sceneCacheEntry{Value: map[string]any{"cached": true}, ExpiresAt: time.Now().Add(time.Minute)}
	controller.scene["ax:2:0"] = sceneCacheEntry{Value: map[string]any{"cached": true}, ExpiresAt: time.Now().Add(time.Minute)}

	controller.NotifySceneEvent(ComputerSceneEvent{WindowID: "ax:1:0", Kind: "value-changed", ElementID: "1:0.2"})

	if _, ok := controller.scene["ax:1:0"]; ok {
		t.Fatal("dirty target window cache should be invalidated")
	}
	if _, ok := controller.scene["ax:2:0"]; !ok {
		t.Fatal("unrelated window cache should remain available")
	}
	meta := controller.SceneMetadata("ax:1:0")
	if generation, _ := meta["generation"].(uint64); generation != 1 {
		t.Fatalf("generation = %d, want 1", generation)
	}
	if dirty, _ := meta["dirty"].(bool); !dirty {
		t.Fatalf("scene should be dirty after event: %#v", meta)
	}
	if got, _ := meta["elementId"].(string); got != "1:0.2" {
		t.Fatalf("event element = %q", got)
	}
}

func TestSceneReadMarksTargetWindowCleanWithoutResettingGeneration(t *testing.T) {
	controller := newSceneTestController()
	controller.NotifySceneEvent(ComputerSceneEvent{WindowID: "ax:1:0", Kind: "children-changed"})
	controller.rememberScene("ax:1:0", map[string]any{"nodes": []any{}})

	meta := controller.SceneMetadata("ax:1:0")
	if generation, _ := meta["generation"].(uint64); generation != 1 {
		t.Fatalf("generation changed after scene read: %#v", meta)
	}
	if dirty, _ := meta["dirty"].(bool); dirty {
		t.Fatalf("scene should be clean after fresh tree read: %#v", meta)
	}
}

func TestGlobalSceneEventPropagatesToKnownWindows(t *testing.T) {
	controller := newSceneTestController()
	controller.MarkSceneClean("ax:1:0")
	controller.MarkSceneClean("ax:2:0")
	controller.NotifySceneEvent(ComputerSceneEvent{Kind: "window-registry-changed"})

	for _, windowID := range []string{"ax:1:0", "ax:2:0"} {
		meta := controller.SceneMetadata(windowID)
		if dirty, _ := meta["dirty"].(bool); !dirty {
			t.Fatalf("%s should be dirty after global event: %#v", windowID, meta)
		}
	}
	global := controller.SceneMetadata("")
	if dirty, _ := global["windowRegistryDirty"].(bool); !dirty {
		t.Fatalf("window registry should be dirty after global event: %#v", global)
	}
}
