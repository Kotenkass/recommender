package recommender

import (
	"testing"
	"time"

	"recommender/internal/analytics"
)

func TestPromptGeneration(t *testing.T) {
	builder := NewPromptBuilder()
	since := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	got := builder.Build(since, until, "active 3 days; 12 total messages")
	if builder.SystemPrompt() == "" {
		t.Fatal("system prompt is empty")
	}
	if !contains(got, "Period: 2026-08-18T00:00:00Z to 2026-08-25T00:00:00Z") {
		t.Fatalf("prompt missing date range: %q", got)
	}
	if !contains(got, "Activity summary: active 3 days; 12 total messages") {
		t.Fatalf("prompt missing summary: %q", got)
	}
	if contains(got, "raw") || contains(got, "{") {
		t.Fatalf("prompt appears to include raw data: %q", got)
	}
}

func TestBuildAnalyticsSummaryEmpty(t *testing.T) {
	got := BuildAnalyticsSummary(analytics.AnalyticsResponse{})
	if got != "no activity data available" {
		t.Fatalf("summary = %q", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
