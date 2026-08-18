package automation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const computerUserActivityThreshold = 2 * time.Second

func (c *ComputerController) UserActivityGuardSupported() bool {
	if c == nil {
		return false
	}
	enabled, _ := c.Capabilities["userActivityGuard"].(bool)
	return enabled
}

func (c *ComputerController) UserActivity(ctx context.Context) (map[string]any, error) {
	if c == nil {
		return nil, errors.New("Computer Use helper unavailable")
	}
	value, err := c.Call(ctx, "user_activity", map[string]any{})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("native user activity probe returned an invalid payload")
	}
	return root, nil
}

func computerUserRecentlyActive(activity map[string]any, threshold time.Duration) (bool, error) {
	if threshold <= 0 {
		return false, errors.New("user activity threshold must be positive")
	}
	raw, ok := activity["idleMs"]
	if !ok {
		return false, errors.New("native user activity probe did not report idleMs")
	}
	var idleMs float64
	switch typed := raw.(type) {
	case float64:
		idleMs = typed
	case float32:
		idleMs = float64(typed)
	case int:
		idleMs = float64(typed)
	case int64:
		idleMs = float64(typed)
	default:
		return false, fmt.Errorf("native user activity idleMs has unexpected type %T", raw)
	}
	if idleMs < 0 {
		return false, errors.New("native user activity idleMs is negative")
	}
	return idleMs < float64(threshold.Milliseconds()), nil
}

func (c *Controller) guardDisruptiveComputerAction(ctx context.Context, operation string, physical bool) error {
	if c == nil || c.Computer == nil || (!physical && operation != "focus") {
		return nil
	}
	if !c.Computer.UserActivityGuardSupported() {
		return nil
	}
	activity, err := c.Computer.UserActivity(ctx)
	if err != nil {
		return fmt.Errorf("cannot safely check whether the user is active before disruptive desktop control: %w", err)
	}
	active, err := computerUserRecentlyActive(activity, computerUserActivityThreshold)
	if err != nil {
		return fmt.Errorf("cannot safely interpret user activity before disruptive desktop control: %w", err)
	}
	if !active {
		return nil
	}
	idleMs := activity["idleMs"]
	return fmt.Errorf("foreground/physical desktop control refused because the user is currently active (idleMs=%v); use background semantic control or an isolated execution lane", idleMs)
}
