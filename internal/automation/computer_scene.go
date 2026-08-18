package automation

import (
	"strings"
	"time"
)

const globalSceneStateKey = "*"

type computerSceneState struct {
	Generation uint64
	Dirty      bool
	Reason     string
	ElementID  string
	UpdatedAt  time.Time
}

// ComputerSceneEvent is the internal event boundary between a native desktop
// backend and the controller scene cache. A future AXObserver/UIA event source
// can call NotifySceneEvent without changing the public MCP schema.
type ComputerSceneEvent struct {
	WindowID  string
	Kind      string
	ElementID string
}

func normalizeSceneKey(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return globalSceneStateKey
	}
	return windowID
}

func nextSceneState(previous computerSceneState, dirty bool, reason, elementID string) computerSceneState {
	previous.Generation++
	previous.Dirty = dirty
	previous.Reason = strings.TrimSpace(reason)
	previous.ElementID = strings.TrimSpace(elementID)
	previous.UpdatedAt = time.Now()
	return previous
}

func (c *ComputerController) MarkSceneDirty(windowID, reason string) {
	if c == nil {
		return
	}
	key := normalizeSceneKey(windowID)
	c.sceneMu.Lock()
	if key == globalSceneStateKey {
		c.scene = map[string]sceneCacheEntry{}
		c.windows = sceneCacheEntry{}
	} else {
		delete(c.scene, key)
	}
	if c.sceneState == nil {
		c.sceneState = map[string]computerSceneState{}
	}
	c.sceneState[key] = nextSceneState(c.sceneState[key], true, reason, "")
	if key == globalSceneStateKey {
		for sceneKey, state := range c.sceneState {
			if sceneKey == globalSceneStateKey {
				continue
			}
			c.sceneState[sceneKey] = nextSceneState(state, true, reason, "")
		}
	}
	c.sceneMu.Unlock()
}

func (c *ComputerController) MarkSceneClean(windowID string) {
	if c == nil {
		return
	}
	key := normalizeSceneKey(windowID)
	c.sceneMu.Lock()
	if c.sceneState == nil {
		c.sceneState = map[string]computerSceneState{}
	}
	current := c.sceneState[key]
	current.Dirty = false
	current.Reason = "scene-read"
	current.ElementID = ""
	current.UpdatedAt = time.Now()
	c.sceneState[key] = current
	c.sceneMu.Unlock()
}

func (c *ComputerController) NotifySceneEvent(event ComputerSceneEvent) {
	if c == nil {
		return
	}
	key := normalizeSceneKey(event.WindowID)
	reason := strings.TrimSpace(event.Kind)
	if reason == "" {
		reason = "native-scene-event"
	}
	c.sceneMu.Lock()
	if key == globalSceneStateKey {
		c.scene = map[string]sceneCacheEntry{}
		c.windows = sceneCacheEntry{}
	} else {
		delete(c.scene, key)
	}
	if c.sceneState == nil {
		c.sceneState = map[string]computerSceneState{}
	}
	c.sceneState[key] = nextSceneState(c.sceneState[key], true, reason, event.ElementID)
	if key == globalSceneStateKey {
		for sceneKey, state := range c.sceneState {
			if sceneKey == globalSceneStateKey {
				continue
			}
			c.sceneState[sceneKey] = nextSceneState(state, true, reason, event.ElementID)
		}
	}
	c.sceneMu.Unlock()
}

func (c *ComputerController) MarkWindowRegistryClean() {
	if c == nil {
		return
	}
	c.sceneMu.Lock()
	if c.sceneState == nil {
		c.sceneState = map[string]computerSceneState{}
	}
	current := c.sceneState[globalSceneStateKey]
	current.Dirty = false
	current.Reason = "window-registry-read"
	current.ElementID = ""
	current.UpdatedAt = time.Now()
	c.sceneState[globalSceneStateKey] = current
	c.sceneMu.Unlock()
}

func (c *ComputerController) SceneMetadata(windowID string) map[string]any {
	if c == nil {
		return nil
	}
	key := normalizeSceneKey(windowID)
	c.sceneMu.RLock()
	state := c.sceneState[key]
	global := c.sceneState[globalSceneStateKey]
	c.sceneMu.RUnlock()
	result := map[string]any{
		"generation":          state.Generation,
		"globalGeneration":    global.Generation,
		"dirty":               state.Dirty,
		"windowRegistryDirty": global.Dirty,
	}
	if state.Reason != "" {
		result["reason"] = state.Reason
	} else if global.Reason != "" {
		result["reason"] = global.Reason
	}
	if state.ElementID != "" {
		result["elementId"] = state.ElementID
	}
	if !state.UpdatedAt.IsZero() {
		result["updatedAtUnixMs"] = state.UpdatedAt.UnixMilli()
	} else if !global.UpdatedAt.IsZero() {
		result["updatedAtUnixMs"] = global.UpdatedAt.UnixMilli()
	}
	return result
}
