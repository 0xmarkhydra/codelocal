//go:build linux

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	atspiAccessibleInterface = "org.a11y.atspi.Accessible"
	atspiActionInterface     = "org.a11y.atspi.Action"
	atspiRegistryBus         = "org.a11y.atspi.Registry"
	atspiRootPath            = dbus.ObjectPath("/org/a11y/atspi/accessible/root")
)

type atspiRef struct {
	Bus  string
	Path dbus.ObjectPath
}

type atspiElementID struct {
	Bus  string `json:"b"`
	Path string `json:"p"`
}

var atspiState struct {
	sync.Mutex
	conn *dbus.Conn
}

func atspiConnection(ctx context.Context) (*dbus.Conn, error) {
	atspiState.Lock()
	defer atspiState.Unlock()
	if atspiState.conn != nil {
		return atspiState.conn, nil
	}
	session, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	var address string
	call := session.Object("org.a11y.Bus", dbus.ObjectPath("/org/a11y/bus")).CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0)
	if call.Err != nil {
		return nil, call.Err
	}
	if err := call.Store(&address); err != nil {
		return nil, err
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("AT-SPI accessibility bus returned an empty address")
	}
	conn, err := dbus.Connect(address)
	if err != nil {
		return nil, err
	}
	atspiState.conn = conn
	return conn, nil
}

func atspiAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	conn, err := atspiConnection(ctx)
	if err != nil {
		return false
	}
	var children []atspiRef
	return conn.Object(atspiRegistryBus, atspiRootPath).
		CallWithContext(ctx, atspiAccessibleInterface+".GetChildren", 0).
		Store(&children) == nil
}

func encodeATSPID(element atspiRef) string {
	raw, _ := json.Marshal(atspiElementID{Bus: element.Bus, Path: string(element.Path)})
	return "atspi:" + base64.RawURLEncoding.EncodeToString(raw)
}

func decodeATSPID(value string) (atspiRef, error) {
	if !strings.HasPrefix(value, "atspi:") {
		return atspiRef{}, errors.New("invalid AT-SPI elementId")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "atspi:"))
	if err != nil {
		return atspiRef{}, errors.New("invalid AT-SPI elementId")
	}
	var payload atspiElementID
	if json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.Bus) == "" || !dbus.ObjectPath(payload.Path).IsValid() {
		return atspiRef{}, errors.New("invalid AT-SPI elementId")
	}
	return atspiRef{Bus: payload.Bus, Path: dbus.ObjectPath(payload.Path)}, nil
}

func atspiPropertyString(object dbus.BusObject, property string) string {
	variant, err := object.GetProperty(atspiAccessibleInterface + "." + property)
	if err != nil {
		return ""
	}
	value, _ := variant.Value().(string)
	return value
}

func atspiStringMethod(ctx context.Context, object dbus.BusObject, method string) string {
	var value string
	if object.CallWithContext(ctx, atspiAccessibleInterface+"."+method, 0).Store(&value) != nil {
		return ""
	}
	return value
}

func atspiInterfaces(ctx context.Context, object dbus.BusObject) []string {
	var values []string
	if object.CallWithContext(ctx, atspiAccessibleInterface+".GetInterfaces", 0).Store(&values) != nil {
		return nil
	}
	return values
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func atspiWalk(ctx context.Context, conn *dbus.Conn, ref atspiRef, count *int, limit, depth int, seen map[string]bool) map[string]any {
	if *count >= limit || depth > 9 || strings.TrimSpace(ref.Bus) == "" || !ref.Path.IsValid() || ref.Path == dbus.ObjectPath("/org/a11y/atspi/null") {
		return nil
	}
	key := ref.Bus + "\x00" + string(ref.Path)
	if seen[key] {
		return nil
	}
	seen[key] = true
	*count++
	object := conn.Object(ref.Bus, ref.Path)
	interfaces := atspiInterfaces(ctx, object)
	node := map[string]any{
		"elementId":   encodeATSPID(ref),
		"name":        atspiPropertyString(object, "Name"),
		"description": atspiPropertyString(object, "Description"),
		"role":        atspiStringMethod(ctx, object, "GetRoleName"),
		"actionable":  containsString(interfaces, atspiActionInterface),
		"interfaces":  interfaces,
		"children":    []map[string]any{},
	}
	if accessibleID := atspiPropertyString(object, "AccessibleId"); accessibleID != "" {
		node["accessibleId"] = accessibleID
	}
	var children []atspiRef
	if object.CallWithContext(ctx, atspiAccessibleInterface+".GetChildren", 0).Store(&children) == nil {
		childNodes := make([]map[string]any, 0, len(children))
		for _, child := range children {
			if *count >= limit || ctx.Err() != nil {
				break
			}
			if childNode := atspiWalk(ctx, conn, child, count, limit, depth+1, seen); childNode != nil {
				childNodes = append(childNodes, childNode)
			}
		}
		node["children"] = childNodes
	}
	return node
}

func atspiTree(ctx context.Context, limit int) (any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	conn, err := atspiConnection(ctx)
	if err != nil {
		return nil, err
	}
	root := atspiRef{Bus: atspiRegistryBus, Path: atspiRootPath}
	count := 0
	node := atspiWalk(ctx, conn, root, &count, limit, 0, map[string]bool{})
	if node == nil {
		return nil, errors.New("AT-SPI root accessible is unavailable")
	}
	return map[string]any{"backend": "at-spi", "node": node, "count": count, "truncated": count >= limit}, nil
}

func atspiDoAction(ctx context.Context, elementID string) (any, error) {
	ref, err := decodeATSPID(elementID)
	if err != nil {
		return nil, err
	}
	conn, err := atspiConnection(ctx)
	if err != nil {
		return nil, err
	}
	object := conn.Object(ref.Bus, ref.Path)
	interfaces := atspiInterfaces(ctx, object)
	if !containsString(interfaces, atspiActionInterface) {
		return nil, errors.New("AT-SPI element has no Action interface")
	}
	var count int32
	if variant, propErr := object.GetProperty(atspiActionInterface + ".NActions"); propErr == nil {
		count, _ = variant.Value().(int32)
	}
	if count == 0 {
		return nil, errors.New("AT-SPI element exposes no actions")
	}
	var ok bool
	if err := object.CallWithContext(ctx, atspiActionInterface+".DoAction", 0, int32(0)).Store(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("AT-SPI default action was rejected by the application")
	}
	return map[string]any{"invoked": true, "elementId": elementID, "actionIndex": 0}, nil
}

func atspiSummary() string {
	if atspiAvailable() {
		return "available"
	}
	return "unavailable"
}

func atspiError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("AT-SPI %s failed: %w", operation, err)
}
