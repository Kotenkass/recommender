package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestParseOpenRouterResponse(t *testing.T) {
	got, err := ParseCompletionResponse([]byte(`{"choices":[{"message":{"role":"assistant","content":"Рекомендация."}}]}`))
	if err != nil {
		t.Fatalf("ParseCompletionResponse: %v", err)
	}
	if got != "Рекомендация." {
		t.Fatalf("content = %q", got)
	}
}

func TestParseOpenRouterResponseErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want error
	}{
		{name: "malformed", data: `{`, want: ErrMalformedJSON},
		{name: "no choices", data: `{}`, want: ErrNoChoices},
		{name: "empty content", data: `{"choices":[{"message":{"role":"assistant","content":" "}}]}`, want: ErrEmptyContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCompletionResponse([]byte(tt.data))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRetryLogic(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch call {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			_ = json.NewEncoder(w).Encode(ChatCompletionResponse{Choices: []Choice{{Message: ChatMessage{Content: "Рекомендация."}}}})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "test-model", time.Second, rate.NewLimiter(rate.Inf, 1))
	got, err := client.GenerateRecommendation(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GenerateRecommendation: %v", err)
	}
	if got != "Рекомендация." {
		t.Fatalf("content = %q", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestNoRetryForMalformedResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":" "}}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "test-model", time.Second, rate.NewLimiter(rate.Inf, 1))
	if _, err := client.GenerateRecommendation(context.Background(), "prompt"); !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("error = %v, want %v", err, ErrEmptyContent)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestNoRetryForNormal4xx(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "test-model", time.Second, rate.NewLimiter(rate.Inf, 1))
	if _, err := client.GenerateRecommendation(context.Background(), "prompt"); err == nil {
		t.Fatal("error is nil")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestRateLimitingBehavior(t *testing.T) {
	limiter := rate.NewLimiter(rate.Limit(10), 1)
	client := NewClient("", "token", "test-model", time.Second, limiter)
	if client.limiter != limiter {
		t.Fatal("client did not use supplied limiter")
	}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := client.limiter.Wait(context.Background()); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("rate limiter returned too quickly: %v", elapsed)
	}
}

func TestGenerateRecommendationUsesOpenAICompatibleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("content type = %q", got)
		}
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Fatalf("model = %q", req.Model)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Fatalf("messages = %#v", req.Messages)
		}
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{Choices: []Choice{{Message: ChatMessage{Content: "Рекомендация."}}}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", "test-model", time.Second, rate.NewLimiter(rate.Inf, 1))
	got, err := client.GenerateRecommendation(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GenerateRecommendation: %v", err)
	}
	if got != "Рекомендация." {
		t.Fatalf("content = %q", got)
	}
}
