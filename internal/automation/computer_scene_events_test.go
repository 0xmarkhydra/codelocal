package automation

import "testing"

func TestApplySceneEventsInvalidatesAffectedScene(t *testing.T) {
	controller := &ComputerController{
		Capabilities: map[string]any{"eventDrivenScene": true},
		scene:        map[string]sceneCacheEntry{"ax:7:0": {Value: map[string]any{"nodes": []any{"cached"}}}},
		sceneState:   map[string]computerSceneState{},
	}
	controller.MarkSceneClean("ax:7:0")
	if err := controller.applySceneEvents(map[string]any{"events": []any{
		map[string]any{"kind": "AXValueChanged", "windowId": "ax:7:0", "elementId": "7:0.1"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := controller.scene["ax:7:0"]; ok {
		t.Fatal("scene cache must be invalidated by native event")
	}
	meta := controller.SceneMetadata("ax:7:0")
	if dirty, _ := meta["dirty"].(bool); !dirty {
		t.Fatalf("scene metadata should be dirty: %#v", meta)
	}
	if got := meta["elementId"]; got != "7:0.1" {
		t.Fatalf("unexpected event element: %#v", got)
	}
}

func TestApplyGlobalSceneEventInvalidatesAllCachedScenes(t *testing.T) {
	controller := &ComputerController{
		Capabilities: map[string]any{"eventDrivenScene": true},
		scene: map[string]sceneCacheEntry{
			"ax:1:0": {Value: "one"},
			"ax:2:0": {Value: "two"},
		},
		sceneState: map[string]computerSceneState{},
	}
	if err := controller.applySceneEvents(map[string]any{"events": []any{
		map[string]any{"kind": "AXWindowCreated", "pid": 1},
	}}); err != nil {
		t.Fatal(err)
	}
	if len(controller.scene) != 0 {
		t.Fatalf("global scene event must clear all cached scenes: %#v", controller.scene)
	}
	meta := controller.SceneMetadata("")
	if dirty, _ := meta["dirty"].(bool); !dirty {
		t.Fatalf("global scene should be dirty: %#v", meta)
	}
}

func TestApplySceneEventsFailsClosedOnInvalidPayload(t *testing.T) {
	controller := &ComputerController{scene: map[string]sceneCacheEntry{"ax:1:0": {Value: "cached"}}, sceneState: map[string]computerSceneState{}}
	if err := controller.applySceneEvents([]any{"invalid"}); err == nil {
		t.Fatal("invalid event payload must fail")
	}
	if len(controller.scene) != 0 {
		t.Fatal("invalid event transport must invalidate caches")
	}
}
