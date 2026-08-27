package recommender

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"recommender/internal/analytics"
	"recommender/internal/metrics"
	"recommender/internal/redispubsub"

	"github.com/sirupsen/logrus"
)

// UsersClient fetches active chat IDs from the users service.
type UsersClient interface {
	ActiveChatIDs(ctx context.Context) ([]string, error)
}

// AnalyticsClient fetches weekly analytics for a chat ID.
type AnalyticsClient interface {
	Aggregates(ctx context.Context, chatID string, since, until time.Time) (analytics.AnalyticsResponse, error)
}

// LLMClient generates one short recommendation from a prompt.
type LLMClient interface {
	GenerateRecommendation(ctx context.Context, prompt string) (string, error)
}

// RedisStore abstracts Redis pub/sub, duplicate prevention, and publish/cache operations.
type RedisStore interface {
	Subscribe(ctx context.Context) redispubsub.PubSub
	CheckLastRecommendation(ctx context.Context, chatID string) (bool, error)
	AcquireProcessingLock(ctx context.Context, chatID string) (bool, func(), error)
	PublishRecommendation(ctx context.Context, chatID, text string) error
	StoreLastRecommendation(ctx context.Context, chatID, text string) error
	Check(ctx context.Context) error
	Close() error
}

// Config contains dependencies and runtime limits for the recommender job orchestrator.
type Config struct {
	UsersClient     UsersClient
	AnalyticsClient AnalyticsClient
	LLMClient       LLMClient
	RedisStore      RedisStore
	Metrics         *metrics.Recorder
	Logger          *logrus.Logger

	LLMModel       string
	JobTimeout     time.Duration
	MaxConcurrency int
	PromptBuilder  PromptBuilder
}

// Service coordinates weekly recommendation jobs.
type Service struct {
	users          UsersClient
	analytics      AnalyticsClient
	llm            LLMClient
	redis          RedisStore
	metrics        *metrics.Recorder
	logger         *logrus.Logger
	promptBuilder  PromptBuilder
	llmModel       string
	jobTimeout     time.Duration
	maxConcurrency int

	mu       sync.Mutex
	pubsub   redispubsub.PubSub
	stopOnce sync.Once
	stopCh   chan struct{}
	active   sync.WaitGroup
	stopped  atomic.Bool
}

func NewService(cfg Config) *Service {
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 10 * time.Minute
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 5
	}
	if cfg.PromptBuilder == (PromptBuilder{}) {
		cfg.PromptBuilder = NewPromptBuilder()
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.New()
		cfg.Logger.SetOutput(io.Discard)
	}
	if cfg.Metrics == nil {
		cfg.Metrics = metrics.NewRecorder()
	}
	return &Service{
		users:          cfg.UsersClient,
		analytics:      cfg.AnalyticsClient,
		llm:            cfg.LLMClient,
		redis:          cfg.RedisStore,
		metrics:        cfg.Metrics,
		logger:         cfg.Logger,
		promptBuilder:  cfg.PromptBuilder,
		llmModel:       cfg.LLMModel,
		jobTimeout:     cfg.JobTimeout,
		maxConcurrency: cfg.MaxConcurrency,
		stopCh:         make(chan struct{}),
	}
}

func (s *Service) Run(ctx context.Context) error {
	pubsub := s.redis.Subscribe(ctx)
	s.mu.Lock()
	s.pubsub = pubsub
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.pubsub = nil
		s.mu.Unlock()
		_ = pubsub.Close()
	}()

	ch := pubsub.Channel()
	for {
		select {
		case <-s.stopCh:
			return nil
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			s.handleEvent(ctx, msg.Payload)
		}
	}
}

// Stop prevents new Redis events from being accepted and waits for active jobs until ctx expires.
func (s *Service) Stop(ctx context.Context) error {
	if s.stopped.Swap(true) {
		return s.wait(ctx)
	}
	close(s.stopCh)

	s.mu.Lock()
	pubsub := s.pubsub
	s.mu.Unlock()
	if pubsub != nil {
		_ = pubsub.Close()
	}
	return s.wait(ctx)
}

func (s *Service) wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) handleEvent(ctx context.Context, payload string) {
	s.logger.Info("received weekly recommendation event")

	jobCtx, cancel := context.WithTimeout(ctx, s.jobTimeout)
	defer cancel()

	started := time.Now().UTC()
	since := started.AddDate(0, 0, -7)
	until := started

	chatIDs, err := s.users.ActiveChatIDs(jobCtx)
	if err != nil {
		s.logger.WithError(err).Error("failed to fetch active users")
		return
	}
	if len(chatIDs) == 0 {
		s.logger.Info("no active users")
		return
	}

	sem := make(chan struct{}, s.maxConcurrency)
	var wg sync.WaitGroup
	for _, chatID := range chatIDs {
		chatID := chatID
		s.active.Add(1)
		wg.Add(1)
		go func() {
			defer s.active.Done()
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				s.processChat(jobCtx, chatID, since, until)
			case <-jobCtx.Done():
				return
			}
		}()
	}
	wg.Wait()
}

func (s *Service) processChat(ctx context.Context, chatID string, since, until time.Time) {
	log := s.logger.WithField("chat_id", chatID)

	acquired, release, err := s.redis.AcquireProcessingLock(ctx, chatID)
	if err != nil {
		log.WithError(err).Error("failed to acquire processing lock")
		s.metrics.FailedRecommendation()
		return
	}
	if !acquired {
		log.Info("chat is already being processed")
		return
	}
	defer release()

	exists, err := s.redis.CheckLastRecommendation(ctx, chatID)
	if err != nil {
		log.WithError(err).Error("failed to check last recommendation cache")
		s.metrics.FailedRecommendation()
		return
	}
	if exists {
		log.Info("skipped duplicate recommendation")
		return
	}

	resp, err := s.analytics.Aggregates(ctx, chatID, since, until)
	if err != nil {
		log.WithError(err).Error("failed to fetch analytics")
		s.metrics.FailedRecommendation()
		return
	}
	prompt := s.promptBuilder.Build(since, until, resp.SummaryText())

	started := time.Now()
	recommendation, err := s.llm.GenerateRecommendation(ctx, prompt)
	latency := time.Since(started)
	s.metrics.ObserveLLM(s.llmModel, latency)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{"model": s.llmModel, "latency_seconds": latency.Seconds()}).Error("failed to generate recommendation")
		s.metrics.FailedRecommendation()
		return
	}
	log.WithFields(logrus.Fields{"response_length": len([]rune(recommendation)), "model": s.llmModel, "latency_seconds": latency.Seconds()}).Info("generated recommendation")

	if err := s.redis.PublishRecommendation(ctx, chatID, recommendation); err != nil {
		log.WithError(err).Error("failed to publish recommendation")
		s.metrics.FailedRecommendation()
		return
	}
	s.metrics.SentRecommendation()

	if err := s.redis.StoreLastRecommendation(ctx, chatID, recommendation); err != nil {
		log.WithError(err).Error("failed to store last recommendation cache")
		s.metrics.FailedRecommendation()
		return
	}
	log.Info("published and cached recommendation")
}

// NewServiceForTest is a small constructor for package-internal tests.
func NewServiceForTest(cfg Config) *Service {
	return NewService(cfg)
}
