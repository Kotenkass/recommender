package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultLLMModel    = "anthropic/claude-haiku-4.5"
	DefaultLLMTimeout  = 20 * time.Second
	DefaultJobTimeout  = 10 * time.Minute
	DefaultLLMRPS      = 5
	DefaultConcurrency = 5
	MinLLMRPS          = 1
	MinMaxConcurrency  = 1
)

// Config contains all runtime configuration loaded from environment variables.
type Config struct {
	RedisURL            string
	UsersServiceURL     string
	AnalyticsServiceURL string
	OpenRouterAPIKey    string
	LLMModel            string
	LLMTimeout          time.Duration
	JobTimeout          time.Duration
	LLMRPS              int
	MaxConcurrency      int
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadFromEnv(os.Environ())
}

// LoadFromEnv is useful for tests and accepts environment entries in KEY=VALUE form.
func LoadFromEnv(env []string) (Config, error) {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}

	cfg := Config{
		RedisURL:            getEnv(values, "REDIS_URL"),
		UsersServiceURL:     getEnv(values, "USERS_SERVICE_URL"),
		AnalyticsServiceURL: getEnv(values, "ANALYTICS_SERVICE_URL"),
		OpenRouterAPIKey:    getEnv(values, "OPENROUTER_API_KEY"),
		LLMModel:            firstNonEmpty(getEnv(values, "LLM_MODEL"), DefaultLLMModel),
		LLMTimeout:          DefaultLLMTimeout,
		JobTimeout:          DefaultJobTimeout,
		LLMRPS:              DefaultLLMRPS,
		MaxConcurrency:      DefaultConcurrency,
	}

	if strings.TrimSpace(cfg.RedisURL) == "" {
		return cfg, errors.New("REDIS_URL is required")
	}
	if strings.TrimSpace(cfg.UsersServiceURL) == "" {
		return cfg, errors.New("USERS_SERVICE_URL is required")
	}
	if strings.TrimSpace(cfg.AnalyticsServiceURL) == "" {
		return cfg, errors.New("ANALYTICS_SERVICE_URL is required")
	}
	if strings.TrimSpace(cfg.OpenRouterAPIKey) == "" {
		return cfg, errors.New("OPENROUTER_API_KEY is required")
	}

	if raw := getEnv(values, "LLM_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return cfg, fmt.Errorf("invalid LLM_TIMEOUT %q: %w", raw, err)
		}
		cfg.LLMTimeout = d
	}

	if raw := getEnv(values, "JOB_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return cfg, fmt.Errorf("invalid JOB_TIMEOUT %q: %w", raw, err)
		}
		cfg.JobTimeout = d
	}

	if raw := getEnv(values, "LLM_RPS"); raw != "" {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < MinLLMRPS {
			return cfg, fmt.Errorf("invalid LLM_RPS %q: must be >= %d", raw, MinLLMRPS)
		}
		cfg.LLMRPS = n
	}

	if raw := getEnv(values, "MAX_CONCURRENCY"); raw != "" {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < MinMaxConcurrency {
			return cfg, fmt.Errorf("invalid MAX_CONCURRENCY %q: must be >= %d", raw, MinMaxConcurrency)
		}
		cfg.MaxConcurrency = n
	}

	return cfg, nil
}

func getEnv(values map[string]string, key string) string {
	return strings.TrimSpace(values[key])
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
}
