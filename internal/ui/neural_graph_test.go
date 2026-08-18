package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeuralGraphIncludesLivingSignalsAndReducedMotionGuard(t *testing.T) {
	html := NeuralGraph(NeuralGraphOptions{
		Mode: "code",
		Data: map[string]any{
			"nodes": []map[string]any{{"id": "a", "kind": "symbol", "name": "A"}, {"id": "b", "kind": "symbol", "name": "B"}},
			"edges": []map[string]any{{"id": "e", "from": "a", "to": "b", "relation": "CALLS", "confidence": .9}},
		},
		RemoteSearch: true,
		Filters:      []NeuralGraphFilter{{Value: "all", Label: "All nodes"}},
	})
	for _, want := range []string{
		`id="neural-canvas"`,
		`prefers-reduced-motion: reduce`,
		`const reduced=window.matchMedia('(prefers-reduced-motion: reduce)').matches`,
		`function pulse(`,
		`document.addEventListener('visibilitychange'`,
		`id="neural-inspect"`,
		`data-neural-mode="code"`,
		`['symbol','method','function','callsite'].includes(n.kind)`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("neural graph must include %q", want)
		}
	}
}

func TestNeuralGraphJavaScriptSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	path := filepath.Join(t.TempDir(), "neural_graph.js")
	if err := os.WriteFile(path, []byte(neuralGraphScript), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "--check", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("neural graph JavaScript syntax invalid: %v\n%s", err, output)
	}
}

func TestNeuralGraphEscapesClosingScriptInData(t *testing.T) {
	html := NeuralGraph(NeuralGraphOptions{Mode: "knowledge", Data: map[string]any{"nodes": []map[string]any{{"id": "x", "name": "</script><script>alert(1)</script>"}}}})
	if strings.Contains(html, `</script><script>alert(1)</script>`) {
		t.Fatal("graph JSON must not allow a payload value to close the data script element")
	}
	if !strings.Contains(html, `\u003c/script\u003e`) && !strings.Contains(html, `<\/script>`) {
		t.Fatal("graph JSON should encode closing script sequences without emitting a literal closing tag")
	}
}

func TestDashboardPageIncludesCodeGraphNavigation(t *testing.T) {
	html := DashboardPage(DashboardOptions{Title: "Code Graph", Active: "codegraph", Email: "user@example.com", CSRF: "csrf"})
	if !strings.Contains(html, `href="/dashboard/code-graph"`) || !strings.Contains(html, `>Code Graph</span>`) {
		t.Fatal("dashboard must expose Code Graph beside Knowledge Graph")
	}
	if !strings.Contains(html, `href="/dashboard/code-graph" aria-label="Code Graph" title="Code Graph" class="active"`) {
		t.Fatal("Code Graph navigation must render active on the graph page")
	}
}
