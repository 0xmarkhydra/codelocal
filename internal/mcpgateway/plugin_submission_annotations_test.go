package mcpgateway

import "testing"

type expectedPluginAnnotation struct {
	readOnly    bool
	destructive bool
	openWorld   bool
}

func TestPluginSubmissionToolAnnotationsMatchCapabilities(t *testing.T) {
	expected := map[string]expectedPluginAnnotation{
		"device":     {false, true, false},
		"workspace":  {false, false, false},
		"project":    {true, false, false},
		"context":    {true, false, false},
		"agent":      {false, true, true},
		"read":       {true, false, false},
		"search":     {true, false, false},
		"dependency": {true, false, false},
		"lsp":        {true, false, false},
		"edit":       {false, true, false},
		"verify":     {true, false, false},
		"git":        {false, false, true},
		"terminal":   {false, true, true},
		"process":    {false, true, false},
		"approvals":  {false, true, false},
		"security":   {true, false, false},
		"mcp":        {false, true, true},
		"social":     {true, false, true},
		"browser":    {false, true, true},
		"computer":   {false, true, true},
	}

	definitions := compactToolDefinitions()
	if len(definitions) != len(expected) {
		t.Fatalf("public plugin tool count=%d want=%d", len(definitions), len(expected))
	}
	for _, definition := range definitions {
		want, ok := expected[definition.Name]
		if !ok {
			t.Fatalf("unexpected public plugin tool %q", definition.Name)
		}
		if definition.Annotations == nil {
			t.Fatalf("tool %q has no plugin annotations", definition.Name)
		}
		got := expectedPluginAnnotation{
			readOnly:    definition.Annotations.ReadOnlyHint,
			destructive: annotationFlag(definition.Annotations.DestructiveHint),
			openWorld:   annotationFlag(definition.Annotations.OpenWorldHint),
		}
		if got != want {
			t.Fatalf("tool %q annotations=%+v want=%+v", definition.Name, got, want)
		}
	}
}
