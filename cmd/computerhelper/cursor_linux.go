//go:build linux

package main

import "context"

func platformCursor(_ context.Context, _ request) (any, error) {
	// X11 and Wayland require different overlay mechanisms and Wayland may
	// intentionally prevent global overlays. Do not move the user's real cursor
	// as a substitute for an independent visual agent cursor.
	return map[string]any{
		"visible":     false,
		"independent": false,
		"reason":      "independent agent cursor overlay is not available in this Linux session",
	}, nil
}
