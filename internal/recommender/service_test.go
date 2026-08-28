package recommender

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"recommender/internal/analytics"
	"recommender/internal/metrics"
	"recommender/internal/redispubsub"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
)

func TestServicePublishesAndCachesRecommendation(t *testing.T) {
	svc, users, analyticsClient, redisStore, recorder := newTestService(t, nil)
	users.ids = []string{"42"}
	analyticsClient.resp = analytics.AnalyticsResponse{ActiveDays: 2, TotalMessages: 5}

	svc.processChat(context.Background(), "42", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))

	if analyticsClient.calls["42"] != 1 {
		t.Fatalf("analytics calls = %d, want 1", analyticsClient.calls["42"])
	}
	if len(redisStore.published) != 1 {
		t.Fatalf("published count = %d, want 1", len(redisStore.published))
	}
	if got := redisStore.published[0].text; got != "recommendation" {
		t.Fatalf("published text = %q", got)
	}
	if got := redisStore.last["42"]; got != "recommendation" {
		t.Fatalf("cache = %q", got)
	}
	if got := testutil.ToFloat64(recorder.Sent); got != 1 {
		t.Fatalf("sent counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.Failed); got != 0 {
		t.Fatalf("failed counter = %v, want 0", got)
	}
}

func TestServiceSkipsDuplicateCache(t *testing.T) {
	svc, users, analyticsClient, redisStore, _ := newTestService(t, nil)
	users.ids = []string{"42"}
	redisStore.last["42"] = "previous recommendation"

	svc.processChat(context.Background(), "42", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))

	if analyticsClient.calls["42"] != 0 {
		t.Fatalf("analytics calls = %d, want 0", analyticsClient.calls["42"])
	}
	if len(redisStore.published) != 0 {
		t.Fatalf("published count = %d, want 0", len(redisStore.published))
	}
}

func TestServiceHandlesAnalyticsError(t *testing.T) {
	svc, users, analyticsClient, _, recorder := newTestService(t, nil)
	users.ids = []string{"42"}
	analyticsClient.err = errors.New("analytics unavailable")

	svc.processChat(context.Background(), "42", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))

	if got := testutil.ToFloat64(recorder.Failed); got != 1 {
		t.Fatalf("failed counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.Sent); got != 0 {
		t.Fatalf("sent counter = %v, want 0", got)
	}
}

func TestServiceHandlesPublishError(t *testing.T) {
	svc, users, _, redisStore, recorder := newTestService(t, nil)
	users.ids = []string{"42"}
	redisStore.failPublish = true
	redisStore.publishErr = errors.New("publish unavailable")

	svc.processChat(context.Background(), "42", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))

	if got := testutil.ToFloat64(recorder.Failed); got != 1 {
		t.Fatalf("failed counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.Sent); got != 0 {
		t.Fatalf("sent counter = %v, want 0", got)
	}
}

func TestServiceCountsSentOnlyAfterPublish(t *testing.T) {
	svc, users, _, redisStore, recorder := newTestService(t, nil)
	users.ids = []string{"42"}
	redisStore.failStore = true
	redisStore.storeErr = errors.New("cache unavailable")

	svc.processChat(context.Background(), "42", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))

	if got := testutil.ToFloat64(recorder.Sent); got != 1 {
		t.Fatalf("sent counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.Failed); got != 1 {
		t.Fatalf("failed counter = %v, want 1", got)
	}
	if len(redisStore.published) != 1 {
		t.Fatalf("published count = %d, want 1", len(redisStore.published))
	}
}

func TestServiceConcurrentChatProcessingLimit(t *testing.T) {
	svc, users, analyticsClient, redisStore, _ := newTestService(t, nil)
	users.ids = []string{"1", "2", "3", "4", "5", "6"}
	analyticsClient.delay = 20 * time.Millisecond
	redisStore.delay = 20 * time.Millisecond
	svc.maxConcurrency = 2

	start := time.Now()
	svc.handleEvent(context.Background(), "event")
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Fatalf("processing completed too quickly with concurrency limit: %v", elapsed)
	}
	if len(redisStore.published) != 6 {
		t.Fatalf("published count = %d, want 6", len(redisStore.published))
	}
}

func newTestService(t *testing.T, logger *logrus.Logger) (*Service, *fakeUsers, *fakeAnalytics, *fakeRedis, *metrics.Recorder) {
	t.Helper()
	if logger == nil {
		logger = logrus.New()
		logger.SetOutput(io.Discard)
	}
	users := &fakeUsers{ids: []string{}}
	analyticsClient := &fakeAnalytics{resp: analytics.AnalyticsResponse{ActiveDays: 1}}
	llmClient := &fakeLLM{resp: "recommendation"}
	redisStore := &fakeRedis{last: map[string]string{}, locks: map[string]struct{}{}}
	recorder := metrics.NewRecorder()
	svc := NewService(Config{
		UsersClient:     users,
		AnalyticsClient: analyticsClient,
		LLMClient:       llmClient,
		RedisStore:      redisStore,
		Metrics:         recorder,
		Logger:          logger,
		LLMModel:        "test-model",
		JobTimeout:      time.Minute,
		MaxConcurrency:  5,
		PromptBuilder:   NewPromptBuilder(),
	})
	return svc, users, analyticsClient, redisStore, recorder
}

type fakeUsers struct {
	ids []string
	err error
}

func (f *fakeUsers) ActiveChatIDs(ctx context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	ids := append([]string(nil), f.ids...)
	return ids, nil
}

type fakeAnalytics struct {
	mu    sync.Mutex
	calls map[string]int
	resp  analytics.AnalyticsResponse
	err   error
	delay time.Duration
}

func (f *fakeAnalytics) Aggregates(ctx context.Context, chatID string, since, until time.Time) (analytics.AnalyticsResponse, error) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[chatID]++
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return analytics.AnalyticsResponse{}, ctx.Err()
		}
	}
	if f.err != nil {
		return analytics.AnalyticsResponse{}, f.err
	}
	return f.resp, nil
}

type fakeLLM struct {
	mu    sync.Mutex
	calls int
	resp  string
	err   error
	delay time.Duration
}

func (f *fakeLLM) GenerateRecommendation(ctx context.Context, prompt string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.resp, nil
}

type fakeRedis struct {
	mu          sync.Mutex
	last        map[string]string
	locks       map[string]struct{}
	published   []published
	failCheck   bool
	failLock    bool
	failPublish bool
	failStore   bool
	checkErr    error
	lockErr     error
	publishErr  error
	storeErr    error
	delay       time.Duration
}

type published struct {
	chatID string
	text   string
}

func (f *fakeRedis) Subscribe(ctx context.Context) redispubsub.PubSub { return nil }
func (f *fakeRedis) CheckLastRecommendation(ctx context.Context, chatID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCheck {
		return false, f.checkErr
	}
	_, ok := f.last[chatID]
	return ok, nil
}
func (f *fakeRedis) AcquireProcessingLock(ctx context.Context, chatID string) (bool, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failLock {
		return false, func() {}, f.lockErr
	}
	if _, ok := f.locks[chatID]; ok {
		return false, func() {}, nil
	}
	f.locks[chatID] = struct{}{}
	return true, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.locks, chatID)
	}, nil
}
func (f *fakeRedis) PublishRecommendation(ctx context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPublish {
		return f.publishErr
	}
	f.published = append(f.published, published{chatID: "", text: text})
	return nil
}
func (f *fakeRedis) StoreLastRecommendation(ctx context.Context, chatID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failStore {
		return f.storeErr
	}
	f.last[chatID] = text
	return nil
}
func (f *fakeRedis) Check(ctx context.Context) error { return nil }
func (f *fakeRedis) Close() error                    { return nil }
