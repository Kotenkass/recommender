package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"recommender/internal/analytics"
	"recommender/internal/config"
	"recommender/internal/llm"
	"recommender/internal/logging"
	"recommender/internal/metrics"
	"recommender/internal/recommender"
	redisstore "recommender/internal/redis"
	"recommender/internal/users"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const (
	serviceName       = "recommender"
	shutdownTimeout   = 10 * time.Second
	httpServerTimeout = 30 * time.Second
)

func main() {
	logger := logging.Configure(serviceName)

	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Fatal("invalid configuration")
	}

	redisClient, err := redisstore.NewClientFromURL(cfg.RedisURL)
	if err != nil {
		logger.WithError(err).Fatal("failed to create redis client")
	}
	redisStore := redisstore.NewStore(redisClient)

	limiter := rate.NewLimiter(rate.Limit(cfg.LLMRPS), cfg.LLMRPS)
	llmClient := llm.NewClient(llm.DefaultEndpoint, cfg.OpenRouterAPIKey, cfg.LLMModel, cfg.LLMTimeout, limiter)

	service := recommender.NewService(recommender.Config{
		UsersClient:     users.NewClient(cfg.UsersServiceURL, users.NewHTTPClient(cfg.LLMTimeout)),
		AnalyticsClient: analytics.NewClient(cfg.AnalyticsServiceURL, analytics.NewHTTPClient(cfg.LLMTimeout)),
		LLMClient:       llmClient,
		RedisStore:      redisStore,
		Metrics:         metrics.NewDefaultRecorder(),
		Logger:          logger,
		LLMModel:        cfg.LLMModel,
		JobTimeout:      cfg.JobTimeout,
		MaxConcurrency:  cfg.MaxConcurrency,
		PromptBuilder:   recommender.NewPromptBuilder(),
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           routes(logger, redisStore),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       httpServerTimeout,
		WriteTimeout:      httpServerTimeout,
		IdleTimeout:       httpServerTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.WithField("addr", cfg.Addr).Info("http server listening")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	runErr := make(chan error, 1)
	go func() {
		if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			runErr <- err
			return
		}
		runErr <- nil
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil {
			logger.WithError(err).Error("http server failed")
		}
	case err := <-runErr:
		if err != nil {
			logger.WithError(err).Error("recommendation service failed")
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := service.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Warn("service shutdown timed out")
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Warn("http server shutdown failed")
	}
	if err := redisStore.Close(); err != nil {
		logger.WithError(err).Warn("redis close failed")
	}
	logger.Info("service stopped")
}

func routes(logger *logrus.Logger, redisStore *redisstore.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := redisStore.Check(ctx); err != nil {
			logger.WithError(err).Warn("readiness check failed")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "redis unavailable"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}
