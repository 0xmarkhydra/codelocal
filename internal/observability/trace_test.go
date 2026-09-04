package observability

import "testing"

func TestRecorderPreservesParentChildTrace(t *testing.T) {
	recorder := NewRecorder("trace-x")
	task := recorder.Start("", SpanTask, "fix auth", nil)
	agent := recorder.Start(task.SpanID, SpanAgent, "backend", map[string]string{"engine": "codex"})
	if _, ok := recorder.End(agent.SpanID, SpanOK, map[string]string{"tokens": "120"}); !ok {
		t.Fatal("missing agent span")
	}
	spans := recorder.Snapshot()
	if len(spans) != 2 || spans[1].ParentSpanID != task.SpanID || spans[1].TraceID != "trace-x" || spans[1].Status != SpanOK {
		t.Fatalf("unexpected trace: %+v", spans)
	}
}
