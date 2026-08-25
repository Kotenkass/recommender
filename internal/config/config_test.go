package config

import (
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	env := []string{
		"REDIS_URL=redis://localhost:6379/0",
		"USERS_SERVICE_URL=http://users",
		"ANALYTICS_SERVICE_URL=http://analytics",
		"OPENROUTER_API_KEY=secret",
		"LLM_MODEL=test-model",
		"LLM_TIMEOUT=5s",
		"JOB_TIMEOUT=2m",
		"LLM_RPS=7",
		"MAX_CONCURRENCY=3",
	}
	cfg, err := LoadFromEnv(env)
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.RedisURL != "redis://localhost:6379/0" || cfg.UsersServiceURL != "http://users" || cfg.AnalyticsServiceURL != "http://analytics" {
		t.Fatalf("unexpected URL config: %#v", cfg)
	}
	if cfg.OpenRouterAPIKey != "secret" || cfg.LLMModel != "test-model" {
		t.Fatalf("unexpected secret/model config: %#v", cfg)
	}
	if cfg.LLMTimeout != 5*time.Second || cfg.JobTimeout != 2*time.Minute || cfg.LLMRPS != 7 || cfg.MaxConcurrency != 3 {
		t.Fatalf("unexpected duration/limit config: %#v", cfg)
	}
}

func TestLoadFromEnvDefaults(t *testing.T) {
	env := []string{
		"REDIS_URL=redis://localhost:6379/0",
		"USERS_SERVICE_URL=http://users",
		"ANALYTICS_SERVICE_URL=http://analytics",
		"OPENROUTER_API_KEY=secret",
	}
	cfg, err := LoadFromEnv(env)
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if cfg.LLMModel != DefaultLLMModel || cfg.LLMTimeout != DefaultLLMTimeout || cfg.JobTimeout != DefaultJobTimeout || cfg.LLMRPS != DefaultLLMRPS || cfg.MaxConcurrency != DefaultConcurrency {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestLoadFromEnvValidation(t *testing.T) {
	_, err := LoadFromEnv([]string{"REDIS_URL=redis://localhost:6379/0", "USERS_SERVICE_URL=http://users", "ANALYTICS_SERVICE_URL=http://analytics"})
	if err == nil {
		t.Fatal("error is nil")
	}
	_, err = LoadFromEnv([]string{"REDIS_URL=redis://localhost:6379/0", "USERS_SERVICE_URL=http://users", "ANALYTICS_SERVICE_URL=http://analytics", "OPENROUTER_API_KEY=secret", "LLM_TIMEOUT=bad"})
	if err == nil {
		t.Fatal("error is nil")
	}
}
