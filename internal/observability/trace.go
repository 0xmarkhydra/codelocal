package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SpanKind string
type SpanStatus string

const (
	SpanTask         SpanKind   = "task"
	SpanAgent        SpanKind   = "agent"
	SpanStep         SpanKind   = "step"
	SpanModel        SpanKind   = "model"
	SpanTool         SpanKind   = "tool"
	SpanProcess      SpanKind   = "process"
	SpanVerification SpanKind   = "verification"
	SpanRunning      SpanStatus = "running"
	SpanOK           SpanStatus = "ok"
	SpanError        SpanStatus = "error"
	SpanCancelled    SpanStatus = "cancelled"
)

type Span struct {
	TraceID      string            `json:"traceId"`
	SpanID       string            `json:"spanId"`
	ParentSpanID string            `json:"parentSpanId,omitempty"`
	Kind         SpanKind          `json:"kind"`
	Name         string            `json:"name"`
	Status       SpanStatus        `json:"status"`
	StartedAt    time.Time         `json:"startedAt"`
	EndedAt      time.Time         `json:"endedAt,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type Recorder struct {
	mu       sync.Mutex
	traceID  string
	sequence uint64
	spans    map[string]Span
	order    []string
}

func NewRecorder(traceID string) *Recorder {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		sum := sha256.Sum256([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
		traceID = "trace_" + hex.EncodeToString(sum[:8])
	}
	return &Recorder{traceID: traceID, spans: map[string]Span{}}
}

func cloneAttributes(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
func (r *Recorder) nextSpanID(name string) string {
	r.sequence++
	sum := sha256.Sum256([]byte(r.traceID + "\x00" + name + "\x00" + time.Now().UTC().Format(time.RFC3339Nano) + "\x00" + strconv.FormatUint(r.sequence, 10)))
	return "span_" + hex.EncodeToString(sum[:8])
}

func (r *Recorder) Start(parentSpanID string, kind SpanKind, name string, attributes map[string]string) Span {
	if r == nil {
		return Span{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	span := Span{TraceID: r.traceID, SpanID: r.nextSpanID(name), ParentSpanID: strings.TrimSpace(parentSpanID), Kind: kind, Name: strings.TrimSpace(name), Status: SpanRunning, StartedAt: time.Now().UTC(), Attributes: cloneAttributes(attributes)}
	r.spans[span.SpanID] = span
	r.order = append(r.order, span.SpanID)
	return span
}

func (r *Recorder) End(spanID string, status SpanStatus, attributes map[string]string) (Span, bool) {
	if r == nil {
		return Span{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	span, ok := r.spans[strings.TrimSpace(spanID)]
	if !ok {
		return Span{}, false
	}
	if status == "" || status == SpanRunning {
		status = SpanOK
	}
	span.Status = status
	span.EndedAt = time.Now().UTC()
	if span.Attributes == nil && len(attributes) > 0 {
		span.Attributes = map[string]string{}
	}
	for key, value := range attributes {
		span.Attributes[key] = value
	}
	r.spans[span.SpanID] = span
	return span, true
}

func (r *Recorder) Snapshot() []Span {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Span, 0, len(r.order))
	for _, id := range r.order {
		span := r.spans[id]
		span.Attributes = cloneAttributes(span.Attributes)
		out = append(out, span)
	}
	return out
}
