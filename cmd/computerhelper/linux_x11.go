//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"strconv"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

var x11State struct {
	sync.Mutex
	conn  *xgb.Conn
	setup *xproto.SetupInfo
	input bool
}

func x11Connection() (*xgb.Conn, *xproto.SetupInfo, bool, error) {
	x11State.Lock()
	defer x11State.Unlock()
	if x11State.conn != nil {
		return x11State.conn, x11State.setup, x11State.input, nil
	}
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, nil, false, err
	}
	setup := xproto.Setup(conn)
	input := xtest.Init(conn) == nil
	x11State.conn = conn
	x11State.setup = setup
	x11State.input = input
	return conn, setup, input, nil
}

func x11Capabilities() map[string]any {
	_, _, input, err := x11Connection()
	if err != nil {
		return map[string]any{
			"available":         false,
			"backend":           "linux-x11+xgb+xtest",
			"screenCapture":     false,
			"uiTree":            false,
			"pointer":           false,
			"keyboard":          false,
			"clipboard":         false,
			"backgroundControl": false,
			"secureDesktop":     false,
			"notes":             []string{"Unable to connect to the active X11 display: " + err.Error()},
		}
	}
	return map[string]any{
		"available":         true,
		"backend":           "linux-x11+xgb+xtest",
		"screenCapture":     true,
		"uiTree":            false,
		"pointer":           input,
		"keyboard":          input,
		"clipboard":         false,
		"backgroundControl": input,
		"secureDesktop":     false,
		"notes":             []string{"X11 window metadata, screen capture and XTEST input are available. Structured AT-SPI UI tree is not advertised until its accessibility bus is verified."},
	}
}

func x11Atom(conn *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	return reply.Atom, nil
}

func x11Property(conn *xgb.Conn, window xproto.Window, name string) (*xproto.GetPropertyReply, error) {
	propertyAtom, err := x11Atom(conn, name)
	if err != nil {
		return nil, err
	}
	return xproto.GetProperty(conn, false, window, propertyAtom, xproto.AtomAny, 0, 1<<20).Reply()
}

func x11Uint32Values(data []byte) []uint32 {
	out := make([]uint32, 0, len(data)/4)
	for len(data) >= 4 {
		out = append(out, xgb.Get32(data[:4]))
		data = data[4:]
	}
	return out
}

func x11WindowID(raw string) (xproto.Window, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || value == 0 {
		return 0, errors.New("invalid X11 windowId")
	}
	return xproto.Window(value), nil
}

func x11ListWindows(ctx context.Context) (any, error) {
	conn, setup, _, err := x11Connection()
	if err != nil {
		return nil, err
	}
	screen := setup.DefaultScreen(conn)
	windows := []xproto.Window{}
	if reply, propErr := x11Property(conn, screen.Root, "_NET_CLIENT_LIST"); propErr == nil && reply != nil && reply.Format == 32 {
		for _, value := range x11Uint32Values(reply.Value) {
			windows = append(windows, xproto.Window(value))
		}
	}
	if len(windows) == 0 {
		tree, treeErr := xproto.QueryTree(conn, screen.Root).Reply()
		if treeErr != nil {
			return nil, treeErr
		}
		windows = tree.Children
	}
	items := make([]map[string]any, 0, len(windows))
	for _, window := range windows {
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
		x, y := geometry.X, geometry.Y
		if translated != nil {
			x, y = translated.DstX, translated.DstY
		}
		title := ""
		if reply, propErr := x11Property(conn, window, "_NET_WM_NAME"); propErr == nil && reply != nil {
			title = strings.TrimRight(string(reply.Value), "\x00")
		}
		if title == "" {
			if reply, propErr := x11Property(conn, window, "WM_NAME"); propErr == nil && reply != nil {
				title = strings.TrimRight(string(reply.Value), "\x00")
			}
		}
		pid := uint32(0)
		if reply, propErr := x11Property(conn, window, "_NET_WM_PID"); propErr == nil && reply != nil && len(reply.Value) >= 4 {
			pid = xgb.Get32(reply.Value[:4])
		}
		items = append(items, map[string]any{
			"windowId": strconv.FormatUint(uint64(window), 10),
			"pid":      pid,
			"title":    title,
			"bounds":   map[string]any{"x": x, "y": y, "width": geometry.Width, "height": geometry.Height},
		})
	}
	return items, nil
}

func x11VisualMasks(setup *xproto.SetupInfo, visual xproto.Visualid) (uint32, uint32, uint32) {
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

func x11PixelComponent(pixel, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := uint(bits.TrailingZeros32(mask))
	max := mask >> shift
	value := (pixel & mask) >> shift
	return uint8((uint64(value)*255 + uint64(max)/2) / uint64(max))
}

func x11ReadPixel(data []byte, order byte) uint32 {
	var value uint32
	if order == xproto.ImageOrderMSBFirst {
		for _, item := range data {
			value = value<<8 | uint32(item)
		}
		return value
	}
	for index := len(data) - 1; index >= 0; index-- {
		value = value<<8 | uint32(data[index])
	}
	return value
}

func x11EncodePNG(setup *xproto.SetupInfo, reply *xproto.GetImageReply, width, height uint16) ([]byte, error) {
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
		return nil, fmt.Errorf("unsupported X11 screenshot depth=%d bpp=%d", reply.Depth, bitsPerPixel)
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
	redMask, greenMask, blueMask := x11VisualMasks(setup, reply.Visual)
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := 0; y < int(height); y++ {
		row := reply.Data[y*stride:]
		for x := 0; x < int(width); x++ {
			start := x * bytesPerPixel
			pixel := x11ReadPixel(row[start:start+bytesPerPixel], setup.ImageByteOrder)
			img.SetRGBA(x, y, color.RGBA{
				R: x11PixelComponent(pixel, redMask),
				G: x11PixelComponent(pixel, greenMask),
				B: x11PixelComponent(pixel, blueMask),
				A: 255,
			})
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
	window, err := x11WindowID(windowID)
	if err != nil {
		return nil, err
	}
	if window == 0 {
		window = setup.DefaultScreen(conn).Root
	}
	geometry, err := xproto.GetGeometry(conn, xproto.Drawable(window)).Reply()
	if err != nil || geometry == nil {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("X11 window geometry unavailable")
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
	data, err := x11EncodePNG(setup, reply, geometry.Width, geometry.Height)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"windowId":   windowID,
		"__mcpImage": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(data)},
	}, nil
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
	return xtest.FakeInputChecked(conn, xproto.MotionNotify, 0, uint32(xproto.TimeCurrentTime), setup.DefaultScreen(conn).Root, int16(x), int16(y), 0).Check()
}

func x11Click(ctx context.Context, x, y float64) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	if err := x11Move(conn, setup, x, y); err != nil {
		return nil, err
	}
	root := setup.DefaultScreen(conn).Root
	if err := xtest.FakeInputChecked(conn, xproto.ButtonPress, 1, 0, root, 0, 0, 0).Check(); err != nil {
		return nil, err
	}
	if err := xtest.FakeInputChecked(conn, xproto.ButtonRelease, 1, 0, root, 0, 0, 0).Check(); err != nil {
		return nil, err
	}
	return map[string]any{"clicked": true, "x": x, "y": y}, nil
}

func x11Focus(ctx context.Context, windowID string) (any, error) {
	conn, _, err := x11Input()
	if err != nil {
		return nil, err
	}
	window, err := x11WindowID(windowID)
	if err != nil || window == 0 {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("windowId is required")
	}
	if err := xproto.SetInputFocusChecked(conn, xproto.InputFocusPointerRoot, window, xproto.Timestamp(xproto.TimeCurrentTime)).Check(); err != nil {
		return nil, err
	}
	return map[string]any{"focused": true, "windowId": windowID}, nil
}

type x11Key struct {
	keycode byte
	shift   bool
}

func x11KeyboardMap(conn *xgb.Conn, setup *xproto.SetupInfo) (map[uint32]x11Key, error) {
	count := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1
	if count <= 0 || count > 255 {
		return nil, errors.New("invalid X11 keyboard range")
	}
	reply, err := xproto.GetKeyboardMapping(conn, setup.MinKeycode, byte(count)).Reply()
	if err != nil {
		return nil, err
	}
	per := int(reply.KeysymsPerKeycode)
	mapping := map[uint32]x11Key{}
	for index := 0; index < count; index++ {
		for slot := 0; slot < per && slot < 2; slot++ {
			position := index*per + slot
			if position >= len(reply.Keysyms) {
				break
			}
			keysym := uint32(reply.Keysyms[position])
			if keysym != 0 {
				if _, exists := mapping[keysym]; !exists {
					mapping[keysym] = x11Key{keycode: byte(int(setup.MinKeycode) + index), shift: slot == 1}
				}
			}
		}
	}
	return mapping, nil
}

func linuxRuneKeysym(value rune) uint32 {
	if value >= 0x20 && value <= 0x7e {
		return uint32(value)
	}
	if value == '\n' || value == '\r' {
		return 0xff0d
	}
	if value == '\t' {
		return 0xff09
	}
	if value <= 0xff {
		return uint32(value)
	}
	return 0x01000000 | uint32(value)
}

func linuxNamedKeysym(key string) (uint32, bool) {
	values := map[string]uint32{
		"enter": 0xff0d, "return": 0xff0d, "tab": 0xff09, "escape": 0xff1b,
		"space": 0x20, "backspace": 0xff08, "delete": 0xffff,
		"left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
		"pageup": 0xff55, "pagedown": 0xff56, "home": 0xff50, "end": 0xff57,
	}
	value, ok := values[strings.ToLower(strings.TrimSpace(key))]
	return value, ok
}

func x11PressKeysym(conn *xgb.Conn, setup *xproto.SetupInfo, mapping map[uint32]x11Key, keysym uint32) error {
	item, ok := mapping[keysym]
	if !ok {
		return fmt.Errorf("X11 keyboard layout has no key for keysym 0x%x", keysym)
	}
	root := setup.DefaultScreen(conn).Root
	shift, hasShift := mapping[0xffe1]
	if item.shift && hasShift {
		if err := xtest.FakeInputChecked(conn, xproto.KeyPress, shift.keycode, 0, root, 0, 0, 0).Check(); err != nil {
			return err
		}
		defer func() { _ = xtest.FakeInputChecked(conn, xproto.KeyRelease, shift.keycode, 0, root, 0, 0, 0).Check() }()
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
	for _, value := range text {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if err := x11PressKeysym(conn, setup, mapping, linuxRuneKeysym(value)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"typed": true, "characters": len([]rune(text))}, nil
}

func x11KeyPress(ctx context.Context, key string) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	mapping, err := x11KeyboardMap(conn, setup)
	if err != nil {
		return nil, err
	}
	keysym, ok := linuxNamedKeysym(key)
	if !ok {
		runes := []rune(key)
		if len(runes) != 1 {
			return nil, errors.New("unsupported X11 key")
		}
		keysym = linuxRuneKeysym(runes[0])
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
	clickButton := func(button byte, count int) error {
		if count > 20 {
			count = 20
		}
		for index := 0; index < count; index++ {
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
		if dy < 0 {
			button = 4
		}
		if err := clickButton(button, int(absFloat(dy)/80)+1); err != nil {
			return nil, err
		}
	}
	if dx != 0 {
		button := byte(7)
		if dx < 0 {
			button = 6
		}
		if err := clickButton(button, int(absFloat(dx)/80)+1); err != nil {
			return nil, err
		}
	}
	return map[string]any{"scrolled": true, "deltaX": dx, "deltaY": dy}, nil
}

func x11Drag(ctx context.Context, x1, y1, x2, y2 float64) (any, error) {
	conn, setup, err := x11Input()
	if err != nil {
		return nil, err
	}
	root := setup.DefaultScreen(conn).Root
	if err := x11Move(conn, setup, x1, y1); err != nil {
		return nil, err
	}
	if err := xtest.FakeInputChecked(conn, xproto.ButtonPress, 1, 0, root, 0, 0, 0).Check(); err != nil {
		return nil, err
	}
	if err := waitContext(ctx, 40_000_000); err != nil {
		return nil, err
	}
	if err := x11Move(conn, setup, x2, y2); err != nil {
		return nil, err
	}
	if err := waitContext(ctx, 40_000_000); err != nil {
		return nil, err
	}
	if err := xtest.FakeInputChecked(conn, xproto.ButtonRelease, 1, 0, root, 0, 0, 0).Check(); err != nil {
		return nil, err
	}
	return map[string]any{"dragged": true, "from": map[string]any{"x": x1, "y": y1}, "to": map[string]any{"x": x2, "y": y2}}, nil
}

func x11Handle(ctx context.Context, input request) (any, error) {
	switch input.Operation {
	case "list_windows":
		return x11ListWindows(ctx)
	case "ui_tree":
		return nil, errors.New("AT-SPI UI tree is not advertised by the X11 helper")
	case "screenshot":
		return x11Screenshot(ctx, stringValue(input.Arguments, "windowId"))
	case "focus":
		return x11Focus(ctx, stringValue(input.Arguments, "windowId"))
	case "click":
		x, okX := numberValue(input.Arguments, "x")
		y, okY := numberValue(input.Arguments, "y")
		if !okX || !okY {
			return nil, errors.New("X11 computer_click requires x/y coordinates")
		}
		return x11Click(ctx, x, y)
	case "type":
		return x11Type(ctx, stringValue(input.Arguments, "text"))
	case "key":
		return x11KeyPress(ctx, stringValue(input.Arguments, "key"))
	case "scroll":
		dx, _ := numberValue(input.Arguments, "deltaX")
		dy, _ := numberValue(input.Arguments, "deltaY")
		return x11Scroll(ctx, dx, dy)
	case "drag":
		x1, ok1 := numberValue(input.Arguments, "fromX")
		y1, ok2 := numberValue(input.Arguments, "fromY")
		x2, ok3 := numberValue(input.Arguments, "toX")
		y2, ok4 := numberValue(input.Arguments, "toY")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, errors.New("X11 computer_drag requires fromX/fromY/toX/toY")
		}
		return x11Drag(ctx, x1, y1, x2, y2)
	default:
		return nil, errors.New("unsupported X11 Computer Use operation")
	}
}
