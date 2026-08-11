//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

const (
	portalBus  = "org.freedesktop.portal.Desktop"
	portalPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")
)

var linuxState struct {
	sync.Mutex
	xconn        *xgb.Conn
	xsetup       *xproto.SetupInfo
	xinput       bool
	portal       *dbus.Conn
	portalSignal chan *dbus.Signal
	remote       *waylandRemote
}

type waylandRemote struct {
	session dbus.ObjectPath
	devices uint32
	stream  uint32
}

func linuxMode() string {
	if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		return "wayland"
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) != "" {
		return "x11"
	}
	return "headless"
}

func x11Connection() (*xgb.Conn, *xproto.SetupInfo, bool, error) {
	linuxState.Lock()
	defer linuxState.Unlock()
	if linuxState.xconn != nil {
		return linuxState.xconn, linuxState.xsetup, linuxState.xinput, nil
	}
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, nil, false, err
	}
	setup := xproto.Setup(conn)
	input := xtest.Init(conn) == nil
	linuxState.xconn = conn
	linuxState.xsetup = setup
	linuxState.xinput = input
	return conn, setup, input, nil
}

func portalConnection() (*dbus.Conn, chan *dbus.Signal, error) {
	linuxState.Lock()
	defer linuxState.Unlock()
	if linuxState.portal != nil {
		return linuxState.portal, linuxState.portalSignal, nil
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, nil, err
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		conn.Close()
		return nil, nil, err
	}
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	linuxState.portal = conn
	linuxState.portalSignal = ch
	return conn, ch, nil
}

func portalProperty(ctx context.Context, iface, property string) (dbus.Variant, error) {
	conn, _, err := portalConnection()
	if err != nil {
		return dbus.Variant{}, err
	}
	var value dbus.Variant
	err = conn.Object(portalBus, portalPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, iface, property).Store(&value)
	return value, err
}

func platformCapabilities() map[string]any {
	mode := linuxMode()
	base := map[string]any{
		"available": false, "backend": "linux-headless", "screenCapture": false,
		"uiTree": false, "pointer": false, "keyboard": false, "clipboard": false,
		"backgroundControl": false, "secureDesktop": false,
	}
	switch mode {
	case "x11":
		_, _, input, err := x11Connection()
		if err != nil {
			base["notes"] = []string{"Unable to connect to the active X11 display: " + err.Error()}
			return base
		}
		base["available"] = true
		base["backend"] = "linux-x11+xgb+xtest"
		base["screenCapture"] = true
		base["pointer"] = input
		base["keyboard"] = input
		base["backgroundControl"] = input
		base["notes"] = []string{"Structured AT-SPI UI tree is not advertised on this build; X11 window metadata, capture and XTEST input remain available."}
	case "wayland":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		devicesVar, remoteErr := portalProperty(ctx, "org.freedesktop.portal.RemoteDesktop", "AvailableDeviceTypes")
		_, screenshotErr := portalProperty(ctx, "org.freedesktop.portal.Screenshot", "version")
		var devices uint32
		if remoteErr == nil {
			devices, _ = devicesVar.Value().(uint32)
		}
		available := remoteErr == nil || screenshotErr == nil
		base["available"] = available
		base["backend"] = "linux-wayland+xdg-desktop-portal"
		base["screenCapture"] = screenshotErr == nil
		base["pointer"] = devices&2 != 0
		base["keyboard"] = devices&1 != 0
		base["backgroundControl"] = false
		base["notes"] = []string{"Wayland input is granted by xdg-desktop-portal for the lifetime of the CodeLocal helper session; compositor secure surfaces remain outside CodeLocal control."}
	default:
		base["notes"] = []string{"No graphical Linux session was detected."}
	}
	return base
}

func atom(conn *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	return reply.Atom, nil
}

func property(conn *xgb.Conn, window xproto.Window, name string) (*xproto.GetPropertyReply, error) {
	propertyAtom, err := atom(conn, name)
	if err != nil {
		return nil, err
	}
	return xproto.GetProperty(conn, false, window, propertyAtom, xproto.AtomAny, 0, 1<<20).Reply()
}

func uint32Values(data []byte) []uint32 {
	values := make([]uint32, 0, len(data)/4)
	for len(data) >= 4 {
		values = append(values, xgb.Get32(data[:4]))
		data = data[4:]
	}
	return values
}

func x11Windows(ctx context.Context) (any, error) {
	conn, setup, _, err := x11Connection()
	if err != nil {
		return nil, err
	}
	screen := setup.DefaultScreen(conn)
	windowIDs := []xproto.Window{}
	if reply, err := property(conn, screen.Root, "_NET_CLIENT_LIST"); err == nil && reply != nil && reply.Format == 32 {
		for _, id := range uint32Values(reply.Value) {
			windowIDs = append(windowIDs, xproto.Window(id))
		}
	}
	if len(windowIDs) == 0 {
		tree, treeErr := xproto.QueryTree(conn, screen.Root).Reply()
		if treeErr != nil {
			return nil, treeErr
		}
		windowIDs = tree.Children
	}
	out := make([]map[string]any, 0, len(windowIDs))
	for _, window := range windowIDs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		geometry, geoErr := xproto.GetGeometry(conn, xproto.Drawable(window)).Reply()
		if geoErr != nil || geometry == nil || geometry.Width == 0 || geometry.Height == 0 {
			continue
		}
		translated, _ := xproto.TranslateCoordinates(conn, window, screen.Root, 0, 0).Reply()
		x, y := int16(geometry.X), int16(geometry.Y)
		if translated != nil {
			x, y = translated.DstX, translated.DstY
		}
		name := ""
		if reply, e := property(conn, window, "_NET_WM_NAME"); e == nil && reply != nil {
			name = strings.TrimRight(string(reply.Value), "\x00")
		}
		if name == "" {
			if reply, e := property(conn, window, "WM_NAME"); e == nil && reply != nil {
				name = strings.TrimRight(string(reply.Value), "\x00")
			}
		}
		pid := uint32(0)
		if reply, e := property(conn, window, "_NET_WM_PID"); e == nil && reply != nil && len(reply.Value) >= 4 {
			pid = xgb.Get32(reply.Value[:4])
		}
		out = append(out, map[string]any{
			"windowId": strconv.FormatUint(uint64(window), 10), "pid": pid, "title": name,
			"bounds": map[string]any{"x": x, "y": y, "width": geometry.Width, "height": geometry.Height},
		})
	}
	return out, nil
}

func x11Window(windowID string) (xproto.Window, error) {
	if strings.TrimSpace(windowID) == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(windowID, 10, 32)
	if err != nil || value == 0 {
		return 0, errors.New("invalid X11 windowId")
	}
	return xproto.Window(value), nil
}

func visualMasks(setup *xproto.SetupInfo, visual xproto.Visualid) (uint32, uint32, uint32) {
	for _, screen := range setup.Roots {
		for _, depth := range screen.AllowedDepths {
			for _, item := range depth.Visuals {
				if item.VisualId == visual {
					return item.RedMask, item.GreenMask, item.BlueMask
				}
		}
		}
	}
	return 0x00ff0000, 0x0000ff00, 0x000000ff
}

func pixelComponent(pixel, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := uint(bits.TrailingZeros32(mask))
	max := mask >> shift
	value := (pixel & mask) >> shift
	return uint8((uint64(value)*255 + uint64(max)/2) / uint64(max))
}

func readPixel(data []byte, order byte) uint32 {
	var value uint32
	if order == xproto.ImageOrderMSBFirst {
		for _, b := range data {
			value = (value << 8) | uint32(b)
		}
		return value
	}
	for i := len(data) - 1; i >= 0; i-- {
		value = (value << 8) | uint32(data[i])
	}
	return value
}

func x11PNG(setup *xproto.SetupInfo, reply *xproto.GetImageReply, width, height uint16) ([]byte, error) {
	bitsPerPixel := byte(0)
	padBits := byte(32)
	for _, format := range setup.PixmapFormats {
		if format.Depth == reply.Depth {
			bitsPerPixel = format.BitsPerPixel
			padBits = format.ScanlinePad
			break
		}
	}
	if bitsPerPixel == 0 || bitsPerPixel%8 != 0 {
		return nil, fmt.Errorf("unsupported X11 pixel format depth=%d bpp=%d", reply.Depth, bitsPerPixel)
	}
	bytesPerPixel := int(bitsPerPixel / 8)
	padBytes := int(padBits / 8)
	if padBytes <= 0 {
		padBytes = 4
	}
	rowBytes := int(width) * bytesPerPixel
	stride := ((rowBytes + padBytes - 1) / padBytes) * padBytes
	if len(reply.Data) < stride*int(height) {
		return nil, errors.New("X11 screenshot buffer is shorter than expected")
	}
	redMask, greenMask, blueMask := visualMasks(setup, reply.Visual)
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := 0; y < int(height); y++ {
		row := reply.Data[y*stride:]
		for x := 0; x < int(width); x++ {
			start := x * bytesPerPixel
			pixel := readPixel(row[start:start+bytesPerPixel], setup.ImageByteOrder)
			img.SetRGBA(x, y, color.RGBA{R: pixelComponent(pixel, redMask), G: pixelComponent(pixel, greenMask), B: pixelComponent(pixel, blueMask), A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func x11Screenshot(ctx context.Context, windowID string) (any, error) {
	conn, setup, _, err := x11Connection()
	if err != nil {
		return nil, err
	}
	window, err := x11Window(windowID)
	if err != nil {
		return nil, err
	}
	if window == 0 {
		window = setup.DefaultScreen(conn).Root
	}
	geometry, err := xproto.GetGeometry(conn, xproto.Drawable(window)).Reply()
	if err != nil || geometry == nil {
		return nil, firstError(err, errors.New("X11 window geometry unavailable"))
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	reply, err := xproto.GetImage(conn, xproto.ImageFormatZPixmap, xproto.Drawable(window), 0, 0, geometry.Width, geometry.Height, 0xffffffff).Reply()
	if err != nil {
		return nil, err
	}
	data, err := x11PNG(setup, reply, geometry.Width, geometry.Height)
	if err != nil {
		return nil, err
	}
	return map[string]any{"windowId": windowID, "__mcpImage": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(data)}}, nil
}

func firstError(actual, fallback error) error {
	if actual != nil {
		return actual
	}
	return fallback
}

func x11Input() (*xgb.Conn, *xproto.SetupInfo, error) {
	conn, setup, input, err := x11Connection()
	if err != nil {
		return nil, nil, err
	}
	if !input {
		return nil, nil, errors.New("XTEST input extension is unavailable")
	}
	return conn, setup, nil
}

func x11Move(conn *xgb.Conn, setup *xproto.SetupInfo, x, y float64) error {
	root := setup.DefaultScreen(conn).Root
	return xtest.FakeInputChecked(conn, xproto.MotionNotify, 0, uint32(xproto.TimeCurrentTime), root, int16(x), int16(y), 0).Check()
}

func x11Click(ctx context.Context, x, y float64) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	if err := x11Move(conn, setup, x, y); err != nil {
		return nil, err
	}
	if err := xtest.FakeInputChecked(conn, xproto.ButtonPress, 1, uint32(xproto.TimeCurrentTime), setup.DefaultScreen(conn).Root, 0, 0, 0).Check(); err != nil {
		return nil, err
	}
	if err := xtest.FakeInputChecked(conn, xproto.ButtonRelease, 1, uint32(xproto.TimeCurrentTime), setup.DefaultScreen(conn).Root, 0, 0, 0).Check(); err != nil {
		return nil, err
	}
	return map[string]any{"clicked": true, "x": x, "y": y}, nil
}

func x11Focus(ctx context.Context, windowID string) (any, error) {
	conn, _, err := x11Input()
	if err != nil {
		return nil, err
	}
	window, err := x11Window(windowID)
	if err != nil || window == 0 {
		return nil, firstError(err, errors.New("windowId is required"))
	}
	if err := xproto.SetInputFocusChecked(conn, xproto.InputFocusPointerRoot, window, xproto.Timestamp(xproto.TimeCurrentTime)).Check(); err != nil {
		return nil, err
	}
	return map[string]any{"focused": true, "windowId": windowID}, nil
}

func x11KeyboardMap(conn *xgb.Conn, setup *xproto.SetupInfo) (map[uint32]struct{ keycode byte; shift bool }, error) {
	count := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1
	if count <= 0 || count > 255 {
		return nil, errors.New("invalid X11 keyboard range")
	}
	reply, err := xproto.GetKeyboardMapping(conn, setup.MinKeycode, byte(count)).Reply()
	if err != nil {
		return nil, err
	}
	per := int(reply.KeysymsPerKeycode)
	out := map[uint32]struct{ keycode byte; shift bool }{}
	for index := 0; index < count; index++ {
		for slot := 0; slot < per && slot < 2; slot++ {
			position := index*per + slot
			if position >= len(reply.Keysyms) {
				break
			}
			keysym := uint32(reply.Keysyms[position])
			if keysym != 0 {
				if _, exists := out[keysym]; !exists {
					out[keysym] = struct{ keycode byte; shift bool }{keycode: byte(int(setup.MinKeycode) + index), shift: slot == 1}
				}
		}
	}
	return out, nil
}

func runeKeysym(r rune) uint32 {
	if r >= 0x20 && r <= 0x7e {
		return uint32(r)
	}
	if r == '\n' || r == '\r' {
		return 0xff0d
	}
	if r == '\t' {
		return 0xff09
	}
	if r <= 0xff {
		return uint32(r)
	}
	return 0x01000000 | uint32(r)
}

func namedKeysym(key string) (uint32, bool) {
	values := map[string]uint32{"enter": 0xff0d, "return": 0xff0d, "tab": 0xff09, "escape": 0xff1b, "space": 0x20, "backspace": 0xff08, "delete": 0xffff, "left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54, "pageup": 0xff55, "pagedown": 0xff56, "home": 0xff50, "end": 0xff57}
	value, ok := values[strings.ToLower(strings.TrimSpace(key))]
	return value, ok
}

func x11PressKeysym(conn *xgb.Conn, setup *xproto.SetupInfo, mapping map[uint32]struct{ keycode byte; shift bool }, keysym uint32) error {
	item, ok := mapping[keysym]
	if !ok {
		return fmt.Errorf("X11 keyboard layout has no key for keysym 0x%x", keysym)
	}
	root := setup.DefaultScreen(conn).Root
	shiftItem, hasShift := mapping[0xffe1]
	if item.shift && hasShift {
		if err := xtest.FakeInputChecked(conn, xproto.KeyPress, shiftItem.keycode, 0, root, 0, 0, 0).Check(); err != nil {
			return err
		}
		defer xtest.FakeInputChecked(conn, xproto.KeyRelease, shiftItem.keycode, 0, root, 0, 0, 0).Check()
	}
	if err := xtest.FakeInputChecked(conn, xproto.KeyPress, item.keycode, 0, root, 0, 0, 0).Check(); err != nil {
		return err
	}
	return xtest.FakeInputChecked(conn, xproto.KeyRelease, item.keycode, 0, root, 0, 0, 0).Check()
}

func x11Type(ctx context.Context, text string) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	mapping, err := x11KeyboardMap(conn, setup)
	if err != nil {
		return nil, err
	}
	for _, r := range text {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if err := x11PressKeysym(conn, setup, mapping, runeKeysym(r)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"typed": true, "characters": len([]rune(text))}, nil
}

func x11Key(ctx context.Context, key string) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	mapping, err := x11KeyboardMap(conn, setup)
	if err != nil {
		return nil, err
	}
	keysym, ok := namedKeysym(key)
	if !ok {
		runes := []rune(key)
		if len(runes) != 1 {
			return nil, errors.New("unsupported X11 key")
		}
		keysym = runeKeysym(runes[0])
	}
	if err := x11PressKeysym(conn, setup, mapping, keysym); err != nil {
		return nil, err
	}
	return map[string]any{"pressed": key}, nil
}

func x11Scroll(ctx context.Context, dx, dy float64) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	root := setup.DefaultScreen(conn).Root
	step := func(button byte, count int) error {
		if count < 0 {
			count = -count
		}
		if count > 20 {
			count = 20
		}
		for i := 0; i < count; i++ {
			if err := xtest.FakeInputChecked(conn, xproto.ButtonPress, button, 0, root, 0, 0, 0).Check(); err != nil {
				return err
			}
			if err := xtest.FakeInputChecked(conn, xproto.ButtonRelease, button, 0, root, 0, 0, 0).Check(); err != nil {
				return err
			}
		}
		return nil
	}
	if dy != 0 {
		button := byte(5)
		if dy < 0 { button = 4 }
		if err := step(button, int(mathAbs(dy)/80)+1); err != nil { return nil, err }
	}
	if dx != 0 {
		button := byte(7)
		if dx < 0 { button = 6 }
		if err := step(button, int(mathAbs(dx)/80)+1); err != nil { return nil, err }
	}
	return map[string]any{"scrolled": true, "deltaX": dx, "deltaY": dy}, nil
}

func mathAbs(value float64) float64 { if value < 0 { return -value }; return value }

func x11Drag(ctx context.Context, x1, y1, x2, y2 float64) (any, error) {
	conn, setup, err := x11Input()
	if err != nil { return nil, err }
	root := setup.DefaultScreen(conn).Root
	if err := x11Move(conn, setup, x1, y1); err != nil { return nil, err }
	if err := xtest.FakeInputChecked(conn, xproto.ButtonPress, 1, 0, root, 0, 0, 0).Check(); err != nil { return nil, err }
	time.Sleep(40*time.Millisecond)
	if err := x11Move(conn, setup, x2, y2); err != nil { return nil, err }
	time.Sleep(40*time.Millisecond)
	if err := xtest.FakeInputChecked(conn, xproto.ButtonRelease, 1, 0, root, 0, 0, 0).Check(); err != nil { return nil, err }
	return map[string]any{"dragged": true, "from": map[string]any{"x":x1,"y":y1}, "to":map[string]any{"x":x2,"y":y2}}, nil
}

func randomToken(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return prefix + hex.EncodeToString(buf)
}

func portalRequest(ctx context.Context, method string, args ...any) (map[string]dbus.Variant, error) {
	conn, signals, err := portalConnection()
	if err != nil { return nil, err }
	var handle dbus.ObjectPath
	if err := conn.Object(portalBus, portalPath).CallWithContext(ctx, method, 0, args...).Store(&handle); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done(): return nil, ctx.Err()
		case signal := <-signals:
			if signal == nil || signal.Path != handle || signal.Name != "org.freedesktop.portal.Request.Response" || len(signal.Body) < 2 { continue }
			code, _ := signal.Body[0].(uint32)
			results, _ := signal.Body[1].(map[string]dbus.Variant)
			if code != 0 { return nil, fmt.Errorf("portal request was not approved (response %d)", code) }
			return results, nil
		}
	}
}

func variantObjectPath(value dbus.Variant) dbus.ObjectPath {
	switch typed := value.Value().(type {
	case dbus.ObjectPath:
		return typed
	case string:
		return dbus.ObjectPath(typed)
	default:
		return ""
	}
}

func firstUint32(value any) uint32 {
	var walk func(reflect.Value) uint32
	walk = func(v reflect.Value) uint32 {
		if !v.IsValid() { return 0 }
		if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer { if v.IsNil(){return 0}; return walk(v.Elem()) }
		switch v.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return uint32(v.Uint())
		case reflect.Array, reflect.Slice:
			for i:=0;i<v.Len();i++ { if result:=walk(v.Index(i)); result!=0 { return result } }
		case reflect.Struct:
			for i:=0;i<v.NumField();i++ { if result:=walk(v.Field(i)); result!=0 { return result } }
		}
		return 0
	}
	return walk(reflect.ValueOf(value))
}

func ensureWaylandRemote(ctx context.Context) (*waylandRemote, error) {
	linuxState.Lock()
	if linuxState.remote != nil {
		remote := linuxState.remote
		linuxState.Unlock()
		return remote, nil
	}
	linuxState.Unlock()
	createOptions := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(randomToken("clreq")), "session_handle_token": dbus.MakeVariant(randomToken("clses"))}
	created, err := portalRequest(ctx, "org.freedesktop.portal.RemoteDesktop.CreateSession", createOptions)
	if err != nil { return nil, err }
	session := variantObjectPath(created["session_handle"])
	if session == "" { return nil, errors.New("Wayland portal did not return a session handle") }
	selectOptions := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(randomToken("cldev")), "types": dbus.MakeVariant(uint32(3)), "persist_mode": dbus.MakeVariant(uint32(1))}
	if _, err := portalRequest(ctx, "org.freedesktop.portal.RemoteDesktop.SelectDevices", session, selectOptions); err != nil { return nil, err }
	// Select one monitor as part of the same RemoteDesktop session. This gives
	// absolute pointer coordinates without requiring CodeLocal to scrape the compositor.
	sourceOptions := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(randomToken("clsrc")), "types": dbus.MakeVariant(uint32(1)), "multiple": dbus.MakeVariant(false), "cursor_mode": dbus.MakeVariant(uint32(2))}
	if _, err := portalRequest(ctx, "org.freedesktop.portal.ScreenCast.SelectSources", session, sourceOptions); err != nil { return nil, err }
	started, err := portalRequest(ctx, "org.freedesktop.portal.RemoteDesktop.Start", session, "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(randomToken("clstart"))})
	if err != nil { return nil, err }
	devices, _ := started["devices"].Value().(uint32)
	stream := uint32(0)
	if streams, ok := started["streams"]; ok { stream = firstUint32(streams.Value()) }
	remote := &waylandRemote{session: session, devices: devices, stream: stream}
	linuxState.Lock()
	if linuxState.remote == nil { linuxState.remote = remote } else { remote = linuxState.remote }
	linuxState.Unlock()
	return remote, nil
}

func waylandCall(ctx context.Context, method string, args ...any) error {
	conn, _, err := portalConnection()
	if err != nil { return err }
	return conn.Object(portalBus, portalPath).CallWithContext(ctx, method, 0, args...).Err
}

func waylandScreenshot(ctx context.Context) (any, error) {
	results, err := portalRequest(ctx, "org.freedesktop.portal.Screenshot.Screenshot", "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(randomToken("clshot")), "interactive": dbus.MakeVariant(false), "target": dbus.MakeVariant(uint32(1))})
	if err != nil { return nil, err }
	uri, _ := results["uri"].Value().(string)
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" { return nil, errors.New("Wayland screenshot portal returned an unsupported URI") }
	path, err := url.PathUnescape(parsed.Path)
	if err != nil { return nil, err }
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil { return nil, err }
	return map[string]any{"__mcpImage":map[string]any{"mimeType":"image/png","data":base64.StdEncoding.EncodeToString(data)}}, nil
}

func waylandMove(ctx context.Context, remote *waylandRemote, x,y float64) error {
	if remote.devices&2 == 0 { return errors.New("Wayland pointer permission was not granted") }
	options:=map[string]dbus.Variant{}
	if remote.stream != 0 {
		return waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerMotionAbsolute",remote.session,options,remote.stream,x,y)
	}
	// Relative fallback is bounded by the compositor; moving far negative first
	// establishes the top-left edge before applying requested coordinates.
	if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerMotion",remote.session,options,-100000.0,-100000.0);err!=nil{return err}
	return waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerMotion",remote.session,options,x,y)
}

func waylandClick(ctx context.Context,x,y float64)(any,error){
	remote,err:=ensureWaylandRemote(ctx);if err!=nil{return nil,err};if err:=waylandMove(ctx,remote,x,y);err!=nil{return nil,err};opts:=map[string]dbus.Variant{}
	if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerButton",remote.session,opts,int32(0x110),uint32(1));err!=nil{return nil,err}
	if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerButton",remote.session,opts,int32(0x110),uint32(0));err!=nil{return nil,err}
	return map[string]any{"clicked":true,"x":x,"y":y},nil
}

func waylandKeysym(ctx context.Context, remote *waylandRemote, keysym uint32) error {
	if remote.devices&1==0{return errors.New("Wayland keyboard permission was not granted")};opts:=map[string]dbus.Variant{}
	if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyKeyboardKeysym",remote.session,opts,int32(keysym),uint32(1));err!=nil{return err}
	return waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyKeyboardKeysym",remote.session,opts,int32(keysym),uint32(0))
}

func waylandType(ctx context.Context,text string)(any,error){remote,err:=ensureWaylandRemote(ctx);if err!=nil{return nil,err};for _,r:=range text{select{case<-ctx.Done():return nil,ctx.Err();default:};if err:=waylandKeysym(ctx,remote,runeKeysym(r));err!=nil{return nil,err}};return map[string]any{"typed":true,"characters":len([]rune(text))},nil}
func waylandKey(ctx context.Context,key string)(any,error){remote,err:=ensureWaylandRemote(ctx);if err!=nil{return nil,err};keysym,ok:=namedKeysym(key);if !ok{r:=[]rune(key);if len(r)!=1{return nil,errors.New("unsupported Wayland key")};keysym=runeKeysym(r[0])};if err:=waylandKeysym(ctx,remote,keysym);err!=nil{return nil,err};return map[string]any{"pressed":key},nil}
func waylandScroll(ctx context.Context,dx,dy float64)(any,error){remote,err:=ensureWaylandRemote(ctx);if err!=nil{return nil,err};if remote.devices&2==0{return nil,errors.New("Wayland pointer permission was not granted")};if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerAxis",remote.session,map[string]dbus.Variant{"finish":dbus.MakeVariant(true)},dx,dy);err!=nil{return nil,err};return map[string]any{"scrolled":true,"deltaX":dx,"deltaY":dy},nil}
func waylandDrag(ctx context.Context,x1,y1,x2,y2 float64)(any,error){remote,err:=ensureWaylandRemote(ctx);if err!=nil{return nil,err};if err:=waylandMove(ctx,remote,x1,y1);err!=nil{return nil,err};opts:=map[string]dbus.Variant{};if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerButton",remote.session,opts,int32(0x110),uint32(1));err!=nil{return nil,err};if err:=waylandMove(ctx,remote,x2,y2);err!=nil{return nil,err};if err:=waylandCall(ctx,"org.freedesktop.portal.RemoteDesktop.NotifyPointerButton",remote.session,opts,int32(0x110),uint32(0));err!=nil{return nil,err};return map[string]any{"dragged":true,"from":map[string]any{"x":x1,"y":y1},"to":map[string]any{"x":x2,"y":y2}},nil}

func platformHandle(ctx context.Context,input request)(any,error){
	mode:=linuxMode()
	if input.Operation=="status"{return platformCapabilities(),nil}
	if mode=="headless"{return nil,errors.New("no graphical Linux session is available")}
	if mode=="wayland"{
		switch input.Operation{
		case "screenshot":return waylandScreenshot(ctx)
		case "click":x,ox:=numberValue(input.Arguments,"x");y,oy:=numberValue(input.Arguments,"y");if !ox||!oy{return nil,errors.New("Wayland computer_click requires x/y coordinates")};return waylandClick(ctx,x,y)
		case "type":return waylandType(ctx,stringValue(input.Arguments,"text"))
		case "key":return waylandKey(ctx,stringValue(input.Arguments,"key"))
		case "scroll":dx,_:=numberValue(input.Arguments,"deltaX");dy,_:=numberValue(input.Arguments,"deltaY");return waylandScroll(ctx,dx,dy)
		case "drag":x1,o1:=numberValue(input.Arguments,"fromX");y1,o2:=numberValue(input.Arguments,"fromY");x2,o3:=numberValue(input.Arguments,"toX");y2,o4:=numberValue(input.Arguments,"toY");if !o1||!o2||!o3||!o4{return nil,errors.New("Wayland computer_drag requires fromX/fromY/toX/toY")};return waylandDrag(ctx,x1,y1,x2,y2)
		default:return nil,errors.New("this Wayland backend does not advertise window enumeration, UI tree or forced focus")
		}
	}
	switch input.Operation{
	case "list_windows":return x11Windows(ctx)
	case "ui_tree":return nil,errors.New("AT-SPI UI tree is not advertised by the X11 helper yet")
	case "screenshot":return x11Screenshot(ctx,stringValue(input.Arguments,"windowId"))
	case "focus":return x11Focus(ctx,stringValue(input.Arguments,"windowId"))
	case "click":x,ox:=numberValue(input.Arguments,"x");y,oy:=numberValue(input.Arguments,"y");if !ox||!oy{return nil,errors.New("X11 computer_click requires x/y coordinates")};return x11Click(ctx,x,y)
	case "type":return x11Type(ctx,stringValue(input.Arguments,"text"))
	case "key":return x11Key(ctx,stringValue(input.Arguments,"key"))
	case "scroll":dx,_:=numberValue(input.Arguments,"deltaX");dy,_:=numberValue(input.Arguments,"deltaY");return x11Scroll(ctx,dx,dy)
	case "drag":x1,o1:=numberValue(input.Arguments,"fromX");y1,o2:=numberValue(input.Arguments,"fromY");x2,o3:=numberValue(input.Arguments,"toX");y2,o4:=numberValue(input.Arguments,"toY");if !o1||!o2||!o3||!o4{return nil,errors.New("X11 computer_drag requires fromX/fromY/toX/toY")};return x11Drag(ctx,x1,y1,x2,y2)
	default:return nil,errors.New("unsupported Linux Computer Use operation")
	}
}
