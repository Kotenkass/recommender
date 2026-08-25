package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecorderUpdatesMetrics(t *testing.T) {
	recorder := NewRecorder()
	recorder.SentRecommendation()
	recorder.FailedRecommendation()
	recorder.ObserveLLM("test-model", 250*time.Millisecond)

	if got := testutil.ToFloat64(recorder.Sent); got != 1 {
		t.Fatalf("sent counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.Failed); got != 1 {
		t.Fatalf("failed counter = %v, want 1", got)
	}

	gathered, err := recorder.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	found := false
	for _, mf := range gathered {
		if mf.GetName() == "llm_latency_seconds" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("llm_latency_seconds metric was not registered")
	}
}

func TestNewDefaultRecorder(t *testing.T) {
	recorder := NewDefaultRecorder()
	if recorder == nil || recorder.Sent == nil || recorder.Failed == nil || recorder.LLMLatency == nil {
		t.Fatal("default recorder is incomplete")
	}
}
