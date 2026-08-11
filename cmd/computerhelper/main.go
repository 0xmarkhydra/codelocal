package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

type request struct {
	Version       int            `json:"version"`
	Operation     string         `json:"operation"`
	WorkspaceID   string         `json:"workspaceId,omitempty"`
	WorkspaceRoot string         `json:"workspaceRoot,omitempty"`
	Arguments     map[string]any `json:"arguments,omitempty"`
}

type response struct {
	OK     bool `json:"ok"`
	Result any  `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: codelocal-computer --capabilities|--json")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "--capabilities":
		writeJSON(platformCapabilities())
	case "--json":
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
		if err != nil {
			writeJSON(response{OK: false, Error: err.Error()})
			return
		}
		var input request
		if err := json.Unmarshal(raw, &input); err != nil {
			writeJSON(response{OK: false, Error: "invalid request: " + err.Error()})
			return
		}
		if input.Version != 1 || input.Operation == "" {
			writeJSON(response{OK: false, Error: "unsupported Computer Use request"})
			return
		}
		if input.Arguments == nil {
			input.Arguments = map[string]any{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := platformHandle(ctx, input)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				writeJSON(response{OK: false, Error: "Computer Use operation timed out"})
				return
			}
			writeJSON(response{OK: false, Error: err.Error()})
			return
		}
		writeJSON(response{OK: true, Result: result})
	default:
		fmt.Fprintln(os.Stderr, "unknown option")
		os.Exit(2)
	}
}

func stringValue(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func numberValue(args map[string]any, key string) (float64, bool) {
	value, ok := args[key].(float64)
	return value, ok
}
