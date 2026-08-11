//go:build linux

package main

import "testing"

func TestLinuxNamedKeysym(t *testing.T) {
	tests := map[string]uint32{
		"enter": 0xff0d,
		"escape": 0xff1b,
		"left": 0xff51,
		"down": 0xff54,
	}
	for key, want := range tests {
		got, ok := linuxNamedKeysym(key)
		if !ok || got != want {
			t.Fatalf("linuxNamedKeysym(%q) = 0x%x, %v; want 0x%x", key, got, ok, want)
		}
	}
}

func TestLinuxRuneKeysym(t *testing.T) {
	if got := linuxRuneKeysym('A'); got != uint32('A') {
		t.Fatalf("ASCII keysym = 0x%x", got)
	}
	if got := linuxRuneKeysym('\n'); got != 0xff0d {
		t.Fatalf("newline keysym = 0x%x", got)
	}
	if got := linuxRuneKeysym('€'); got != 0x010020ac {
		t.Fatalf("Unicode keysym = 0x%x", got)
	}
}

func TestX11PixelComponent(t *testing.T) {
	pixel := uint32(0x00123456)
	if got := x11PixelComponent(pixel, 0x00ff0000); got != 0x12 {
		t.Fatalf("red = 0x%x", got)
	}
	if got := x11PixelComponent(pixel, 0x0000ff00); got != 0x34 {
		t.Fatalf("green = 0x%x", got)
	}
	if got := x11PixelComponent(pixel, 0x000000ff); got != 0x56 {
		t.Fatalf("blue = 0x%x", got)
	}
}

func TestFirstPortalUint32FindsStreamNode(t *testing.T) {
	value := []struct {
		Node uint32
		Meta map[string]any
	}{{Node: 42, Meta: map[string]any{"position": "primary"}}}
	if got := firstPortalUint32(value); got != 42 {
		t.Fatalf("stream node = %d, want 42", got)
	}
}
