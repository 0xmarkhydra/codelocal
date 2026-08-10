package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadFrame(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)
	wire := append([]byte("Content-Length: "+itoa(len(body))+"\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n"), body...)
	got, err := readFrame(bufio.NewReader(bytes.NewReader(wire)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q want %q", got, body)
	}
}

func TestNormalizeLocationSupportsLocationLink(t *testing.T) {
	value := map[string]any{
		"targetUri": fileURI(filepath.Join(t.TempDir(), "main.go")),
		"targetSelectionRange": map[string]any{
			"start": map[string]any{"line": float64(4), "character": float64(2)},
			"end":   map[string]any{"line": float64(4), "character": float64(6)},
		},
	}
	items := normalizeLocation(value, "gopls")
	if len(items) != 1 {
		t.Fatalf("items=%#v", items)
	}
	if items[0]["provider"] != "gopls" || items[0]["line"] != 5 || items[0]["column"] != 3 {
		t.Fatalf("normalized=%#v", items[0])
	}
}

func TestFileURIRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "folder with spaces", "main.go")
	uri := fileURI(path)
	got := uriPath(uri)
	want, _ := filepath.Abs(path)
	if runtime.GOOS == "windows" {
		got = filepath.Clean(got)
		want = filepath.Clean(want)
	}
	if got != want {
		t.Fatalf("round trip: got=%q want=%q uri=%q", got, want, uri)
	}
}

func TestDecodeAny(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{{"name": "Symbol"}})
	value, ok := decodeAny(raw).([]any)
	if !ok || len(value) != 1 {
		t.Fatalf("decoded=%#v", decodeAny(raw))
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	buf := [32]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
