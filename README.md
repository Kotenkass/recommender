# Recommender

Production-ready Go service that listens for Redis `weekly_reco` events, fetches active users, enriches each user with weekly analytics, generates a one-sentence Russian recommendation through OpenRouter, and publishes it to Redis `send_message`.

## Features

- Redis pub/sub subscription to `weekly_reco`.
- Redis duplicate prevention with per-chat atomic lock and 6-day last-recommendation TTL.
- Concurrent per-chat processing with a configurable semaphore.
- Shared OpenRouter request rate limiter.
- Retry with exponential backoff for retryable OpenRouter HTTP statuses.
- Per-request HTTP timeouts and overall job timeout.
- JSON structured logs with safe metadata only.
- Prometheus metrics on `/metrics`.
- Graceful shutdown for SIGINT/SIGTERM.
- Multi-stage Docker build running as a non-root user.

## Configuration

Set these environment variables:

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `ADDR` | no | `:8080` | HTTP listen address |
| `REDIS_URL` | yes | - | Redis connection URL, for example `redis://localhost:6379/0` |
| `USERS_SERVICE_URL` | yes | - | Users service base URL |
| `ANALYTICS_SERVICE_URL` | yes | - | Analytics service base URL |
| `OPENROUTER_API_KEY` | yes | - | OpenRouter API key |
| `LLM_MODEL` | no | `anthropic/claude-haiku-4.5` | OpenRouter model |
| `LLM_TIMEOUT` | no | `20s` | Per OpenRouter request timeout |
| `JOB_TIMEOUT` | no | `10m` | Overall recommendation job timeout |
| `LLM_RPS` | no | `5` | Shared LLM requests per second limit |
| `MAX_CONCURRENCY` | no | `5` | Maximum concurrent chat processing |

Copy `.env.example` and fill in local values:

```bash
cp .env.example .env
```

## Run locally

```bash
go mod download
go run ./cmd/recommender
```

The service exposes:

- `GET /healthz` — process liveness.
- `GET /readyz` — liveness plus Redis dependency check.
- `GET /metrics` — Prometheus metrics.

## Docker

Build and run:

```bash
docker build -f docker/Dockerfile -t recommender .
docker run --env-file .env -p 8080:8080 recommender
```

## Redis flow

1. Subscribe to `weekly_reco`.
2. For every event, fetch active chat IDs from `GET /users`.
3. For each chat ID, calculate:
   - `since` = job start time minus 7 days.
   - `until` = job start time.
4. Fetch analytics from `GET /aggregates?chat_id=...&since=...&until=...`.
5. Build a concise prompt from summarized analytics.
6. Generate one short Russian recommendation through OpenRouter.
7. Publish to Redis channel `send_message` as:

```json
{"chat_id":"123","text":"Рекомендация"}
```

8. Store the recommendation in `recommender:last_recommendation:{chat_id}` with a 6-day TTL only after successful publish.

## Metrics

- `reco_sent_total` — incremented only after Redis `PUBLISH send_message` succeeds.
- `reco_failed_total` — incremented when a chat fails.
- `llm_latency_seconds` — OpenRouter request latency histogram.

`chat_id` is intentionally not used as a Prometheus label.

## Logging

Logs are JSON and include `service_name=recommender`. The service never logs OpenRouter API keys, authorization headers, full prompts, or full LLM responses at INFO level. LLM success logs contain only safe fields: `chat_id`, response length, model name, and latency.

## Tests

```bash
go test ./...
```

Tests use mocks and local HTTP/Redis test doubles. They do not call OpenRouter.
