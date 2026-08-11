//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	portalBus  = "org.freedesktop.portal.Desktop"
	portalPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")
)

type waylandRemoteSession struct {
	session dbus.ObjectPath
	devices uint32
	stream  uint32
}

var waylandState struct {
	sync.Mutex
	conn    *dbus.Conn
	signals chan *dbus.Signal
	remote  *waylandRemoteSession
}

func portalConnection() (*dbus.Conn, chan *dbus.Signal, error) {
	waylandState.Lock()
	defer waylandState.Unlock()
	if waylandState.conn != nil {
		return waylandState.conn, waylandState.signals, nil
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, nil, err
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	waylandState.conn = conn
	waylandState.signals = signals
	return conn, signals, nil
}

func portalProperty(ctx context.Context, iface, property string) (dbus.Variant, error) {
	conn, _, err := portalConnection()
	if err != nil {
		return dbus.Variant{}, err
	}
	var value dbus.Variant
	err = conn.Object(portalBus, portalPath).
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, iface, property).
		Store(&value)
	return value, err
}

func waylandCapabilities() map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	devicesValue, remoteErr := portalProperty(ctx, "org.freedesktop.portal.RemoteDesktop", "AvailableDeviceTypes")
	_, screenshotErr := portalProperty(ctx, "org.freedesktop.portal.Screenshot", "version")
	devices := uint32(0)
	if remoteErr == nil {
		devices, _ = devicesValue.Value().(uint32)
	}
	available := remoteErr == nil || screenshotErr == nil
	return map[string]any{
		"available":         available,
		"backend":           "linux-wayland+xdg-desktop-portal",
		"screenCapture":     screenshotErr == nil,
		"uiTree":            false,
		"pointer":           remoteErr == nil && devices&2 != 0,
		"keyboard":          remoteErr == nil && devices&1 != 0,
		"clipboard":         false,
		"backgroundControl": false,
		"secureDesktop":     false,
		"notes":             []string{"Wayland input uses one persistent xdg-desktop-portal RemoteDesktop session. The compositor owns the consent prompt and secure surfaces stay outside CodeLocal control."},
	}
}

func portalToken(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return prefix + hex.EncodeToString(buf)
}

func portalRequest(ctx context.Context, method string, args ...any) (map[string]dbus.Variant, error) {
	conn, signals, err := portalConnection()
	if err != nil {
		return nil, err
	}
	var handle dbus.ObjectPath
	if err := conn.Object(portalBus, portalPath).CallWithContext(ctx, method, 0, args...).Store(&handle); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case signal := <-signals:
			if signal == nil || signal.Path != handle || signal.Name != "org.freedesktop.portal.Request.Response" || len(signal.Body) < 2 {
				continue
			}
			code, _ := signal.Body[0].(uint32)
			results, _ := signal.Body[1].(map[string]dbus.Variant)
			if code != 0 {
				return nil, fmt.Errorf("desktop portal request was not approved (response %d)", code)
			}
			return results, nil
		}
	}
}

func portalSessionPath(value dbus.Variant) dbus.ObjectPath {
	switch typed := value.Value().(type) {
	case dbus.ObjectPath:
		return typed
	case string:
		return dbus.ObjectPath(typed)
	default:
		return ""
	}
}

func firstPortalUint32(value any) uint32 {
	var walk func(reflect.Value) uint32
	walk = func(current reflect.Value) uint32 {
		if !current.IsValid() {
			return 0
		}
		if current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer {
			if current.IsNil() {
				return 0
			}
			return walk(current.Elem())
		}
		switch current.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return uint32(current.Uint())
		case reflect.Array, reflect.Slice:
			for index := 0; index < current.Len(); index++ {
				if result := walk(current.Index(index)); result != 0 {
					return result
				}
			}
		case reflect.Struct:
			for index := 0; index < current.NumField(); index++ {
				if result := walk(current.Field(index)); result != 0 {
					return result
				}
			}
		}
		return 0
	}
	return walk(reflect.ValueOf(value))
}

func ensureWaylandRemote(ctx context.Context) (*waylandRemoteSession, error) {
	waylandState.Lock()
	if waylandState.remote != nil {
		remote := waylandState.remote
		waylandState.Unlock()
		return remote, nil
	}
	waylandState.Unlock()

	created, err := portalRequest(ctx, "org.freedesktop.portal.RemoteDesktop.CreateSession", map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(portalToken("clreq")),
		"session_handle_token": dbus.MakeVariant(portalToken("clses")),
	})
	if err != nil {
		return nil, err
	}
	session := portalSessionPath(created["session_handle"])
	if session == "" {
		return nil, errors.New("Wayland RemoteDesktop portal did not return a session handle")
	}

	if _, err := portalRequest(ctx, "org.freedesktop.portal.RemoteDesktop.SelectDevices", session, map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(portalToken("cldev")),
		"types":        dbus.MakeVariant(uint32(3)),
		"persist_mode": dbus.MakeVariant(uint32(1)),
	}); err != nil {
		return nil, err
	}

	// Select one monitor in the same RemoteDesktop session. The returned stream
	// gives the compositor-approved logical coordinate space for absolute input.
	if _, err := portalRequest(ctx, "org.freedesktop.portal.ScreenCast.SelectSources", session, map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(portalToken("clsrc")),
		"types":        dbus.MakeVariant(uint32(1)),
		"multiple":     dbus.MakeVariant(false),
		"cursor_mode":  dbus.MakeVariant(uint32(2)),
	}); err != nil {
		return nil, err
	}

	started, err := portalRequest(ctx, "org.freedesktop.portal.RemoteDesktop.Start", session, "", map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(portalToken("clstart")),
	})
	if err != nil {
		return nil, err
	}
	devices := uint32(0)
	if value, ok := started["devices"]; ok {
		devices, _ = value.Value().(uint32)
	}
	stream := uint32(0)
	if value, ok := started["streams"]; ok {
		stream = firstPortalUint32(value.Value())
	}
	remote := &waylandRemoteSession{session: session, devices: devices, stream: stream}
	waylandState.Lock()
	if waylandState.remote == nil {
		waylandState.remote = remote
	} else {
		remote = waylandState.remote
	}
	waylandState.Unlock()
	return remote, nil
}

func waylandCall(ctx context.Context, method string, args ...any) error {
	conn, _, err := portalConnection()
	if err != nil {
		return err
	}
	return conn.Object(portalBus, portalPath).CallWithContext(ctx, method, 0, args...).Err
}

func waylandScreenshot(ctx context.Context) (any, error) {
	results, err := portalRequest(ctx, "org.freedesktop.portal.Screenshot.Screenshot", "", map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(portalToken("clshot")),
		"interactive":  dbus.MakeVariant(false),
		"target":       dbus.MakeVariant(uint32(1)),
	})
	if err != nil {
		return nil, err
	}
	uri := ""
	if value, ok := results["uri"]; ok {
		uri, _ = value.Value().(string)
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return nil, errors.New("Wayland screenshot portal returned an unsupported URI")
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	return map[string]any{"__mcpImage": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(data)}}, nil
}

func waylandMove(ctx context.Context, remote *waylandRemoteSession, x, y float64) error {
	if remote.devices&2 == 0 {
		return errors.New("Wayland pointer permission was not granted")
	}
	options := map[string]dbus.Variant{}
	if remote.stream != 0 {
		return waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerMotionAbsolute", remote.session, options, remote.stream, x, y)
	}
	// Fallback for a portal that grants pointer input without returning a
	// screencast stream: clamp to top-left with a large relative move first.
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerMotion", remote.session, options, -100000.0, -100000.0); err != nil {
		return err
	}
	return waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerMotion", remote.session, options, x, y)
}

func waylandClick(ctx context.Context, x, y float64) (any, error) {
	remote, err := ensureWaylandRemote(ctx)
	if err != nil {
		return nil, err
	}
	if err := waylandMove(ctx, remote, x, y); err != nil {
		return nil, err
	}
	options := map[string]dbus.Variant{}
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerButton", remote.session, options, int32(0x110), uint32(1)); err != nil {
		return nil, err
	}
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerButton", remote.session, options, int32(0x110), uint32(0)); err != nil {
		return nil, err
	}
	return map[string]any{"clicked": true, "x": x, "y": y}, nil
}

func waylandKeysym(ctx context.Context, remote *waylandRemoteSession, keysym uint32) error {
	if remote.devices&1 == 0 {
		return errors.New("Wayland keyboard permission was not granted")
	}
	options := map[string]dbus.Variant{}
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyKeyboardKeysym", remote.session, options, int32(keysym), uint32(1)); err != nil {
		return err
	}
	return waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyKeyboardKeysym", remote.session, options, int32(keysym), uint32(0))
}

func waylandType(ctx context.Context, text string) (any, error) {
	remote, err := ensureWaylandRemote(ctx)
	if err != nil {
		return nil, err
	}
	for _, value := range text {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if err := waylandKeysym(ctx, remote, linuxRuneKeysym(value)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"typed": true, "characters": len([]rune(text))}, nil
}

func waylandKeyPress(ctx context.Context, key string) (any, error) {
	remote, err := ensureWaylandRemote(ctx)
	if err != nil {
		return nil, err
	}
	keysym, ok := linuxNamedKeysym(key)
	if !ok {
		runes := []rune(key)
		if len(runes) != 1 {
			return nil, errors.New("unsupported Wayland key")
		}
		keysym = linuxRuneKeysym(runes[0])
	}
	if err := waylandKeysym(ctx, remote, keysym); err != nil {
		return nil, err
	}
	return map[string]any{"pressed": key}, nil
}

func waylandScroll(ctx context.Context, dx, dy float64) (any, error) {
	remote, err := ensureWaylandRemote(ctx)
	if err != nil {
		return nil, err
	}
	if remote.devices&2 == 0 {
		return nil, errors.New("Wayland pointer permission was not granted")
	}
	options := map[string]dbus.Variant{"finish": dbus.MakeVariant(true)}
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerAxis", remote.session, options, dx, dy); err != nil {
		return nil, err
	}
	return map[string]any{"scrolled": true, "deltaX": dx, "deltaY": dy}, nil
}

func waylandDrag(ctx context.Context, x1, y1, x2, y2 float64) (any, error) {
	remote, err := ensureWaylandRemote(ctx)
	if err != nil {
		return nil, err
	}
	if err := waylandMove(ctx, remote, x1, y1); err != nil {
		return nil, err
	}
	options := map[string]dbus.Variant{}
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerButton", remote.session, options, int32(0x110), uint32(1)); err != nil {
		return nil, err
	}
	if err := waitContext(ctx, 40*time.Millisecond); err != nil {
		return nil, err
	}
	if err := waylandMove(ctx, remote, x2, y2); err != nil {
		return nil, err
	}
	if err := waylandCall(ctx, "org.freedesktop.portal.RemoteDesktop.NotifyPointerButton", remote.session, options, int32(0x110), uint32(0)); err != nil {
		return nil, err
	}
	return map[string]any{"dragged": true, "from": map[string]any{"x": x1, "y": y1}, "to": map[string]any{"x": x2, "y": y2}}, nil
}

func waylandHandle(ctx context.Context, input request) (any, error) {
	switch input.Operation {
	case "screenshot":
		return waylandScreenshot(ctx)
	case "click":
		x, okX := numberValue(input.Arguments, "x")
		y, okY := numberValue(input.Arguments, "y")
		if !okX || !okY {
			return nil, errors.New("Wayland computer_click requires x/y coordinates")
		}
		return waylandClick(ctx, x, y)
	case "type":
		return waylandType(ctx, stringValue(input.Arguments, "text"))
	case "key":
		return waylandKeyPress(ctx, stringValue(input.Arguments, "key"))
	case "scroll":
		dx, _ := numberValue(input.Arguments, "deltaX")
		dy, _ := numberValue(input.Arguments, "deltaY")
		return waylandScroll(ctx, dx, dy)
	case "drag":
		x1, ok1 := numberValue(input.Arguments, "fromX")
		y1, ok2 := numberValue(input.Arguments, "fromY")
		x2, ok3 := numberValue(input.Arguments, "toX")
		y2, ok4 := numberValue(input.Arguments, "toY")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, errors.New("Wayland computer_drag requires fromX/fromY/toX/toY")
		}
		return waylandDrag(ctx, x1, y1, x2, y2)
	case "list_windows", "ui_tree", "focus":
		return nil, errors.New("Wayland does not advertise window enumeration, accessibility tree or forced focus on this helper")
	default:
		return nil, errors.New("unsupported Wayland Computer Use operation")
	}
}
