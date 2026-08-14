//go:build windows

package main

import "context"

func platformCursor(_ context.Context, _ request) (any, error) {
	// Keep this explicit instead of moving the user's physical pointer and
	// pretending it is an independent agent cursor. A DPI-safe layered-window
	// implementation can replace this without changing the helper protocol.
	return map[string]any{
		"visible":     false,
		"independent": false,
		"reason":      "independent agent cursor overlay is not yet available on Windows",
	}, nil
}
