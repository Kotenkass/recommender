package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestAggregatesDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aggregates" {
			t.Fatalf("path = %s, want /aggregates", r.URL.Path)
		}
		if got := r.URL.Query().Get("chat_id"); got != "42" {
			t.Fatalf("chat_id = %q", got)
		}
		if got := r.URL.Query().Get("since"); got != "2026-08-18T00:00:00Z" {
			t.Fatalf("since = %q", got)
		}
		if got := r.URL.Query().Get("until"); got != "2026-08-25T00:00:00Z" {
			t.Fatalf("until = %q", got)
		}
		_, _ = w.Write([]byte(`{"active_days":2,"total_messages":5}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	resp, err := client.Aggregates(context.Background(), "42", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Aggregates: %v", err)
	}
	if resp.ActiveDays != 2 || resp.TotalMessages != 5 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestAnalyticsSummaryText(t *testing.T) {
	resp := AnalyticsResponse{
		ActiveDays:       3,
		TotalMessages:    12,
		SentMessages:     7,
		ReceivedMessages: 5,
		MinutesActive:    45,
		TopChannels:      []RankedItem{{Name: "general", Count: 10}},
		TopTopics:        []RankedItem{{Name: "health", Count: 4}},
		Tags:             []string{"sleep", "steps"},
	}
	got := resp.SummaryText()
	if !reflect.DeepEqual(got, "active 3 days; 12 total messages; 7 sent; 5 received; 45 active minutes; top channel: general; top topic: health; tags: sleep, steps") {
		t.Fatalf("summary = %q", got)
	}
}

func TestAnalyticsUnmarshalNestedSummary(t *testing.T) {
	var resp AnalyticsResponse
	data := []byte(`{"period_summary":{"active_days":1,"total_messages":3,"sent_messages":1,"received_messages":2,"minutes_active":7,"top_channels":[{"name":"a","count":2}],"top_topics":[{"name":"b","count":1}],"tags":["t"]},"messages":{"total":4,"sent":1,"received":3},"activity":{"active_days":2,"minutes_active":8,"top_channels":[{"name":"c","count":5}],"top_topics":[{"name":"d","count":6}]}}`)
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ActiveDays != 1 || resp.TotalMessages != 3 || resp.SentMessages != 1 || resp.ReceivedMessages != 2 || resp.MinutesActive != 7 {
		t.Fatalf("summary fields not applied: %#v", resp)
	}
	if resp.TopChannels[0].Name != "a" || resp.TopTopics[0].Name != "b" || resp.Tags[0] != "t" {
		t.Fatalf("nested lists not applied: %#v", resp)
	}
}
