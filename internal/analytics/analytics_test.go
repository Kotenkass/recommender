package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAggregatesDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aggregates" {
			t.Fatalf("path = %s, want /aggregates", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]DailyCount{
			{Date: "2026-08-18", Count: 4},
			{Date: "2026-08-19", Count: 8},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	resp, err := client.Aggregates(
		context.Background(),
		"42",
		time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("Aggregates: %v", err)
	}
	if resp.ActiveDays != 2 {
		t.Fatalf("ActiveDays = %d, want 2", resp.ActiveDays)
	}
	if resp.TotalMessages != 12 {
		t.Fatalf("TotalMessages = %d, want 12", resp.TotalMessages)
	}
}

func TestAggregatesEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "[]")
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	resp, err := client.Aggregates(
		context.Background(),
		"42",
		time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("Aggregates: %v", err)
	}
	if !resp.Empty() {
		t.Fatalf("expected empty response")
	}
}

func TestAggregatesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.Aggregates(
		context.Background(),
		"42",
		time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("expected error from server error")
	}
}
