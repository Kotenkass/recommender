package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	DefaultEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	DefaultModel    = "anthropic/claude-haiku-4.5"
	DefaultTimeout  = 20 * time.Second

	MaxAttempts = 3
)

var (
	ErrNoChoices     = errors.New("llm response contains no choices")
	ErrEmptyContent  = errors.New("llm response choice content is empty")
	ErrMalformedJSON = errors.New("llm response is malformed JSON")
	ErrNonRetryable  = errors.New("non-retryable LLM error")
)

type LLMClient interface {
	GenerateRecommendation(ctx context.Context, prompt string) (string, error)
}

type Client struct {
	endpoint string
	apiKey   string
	model    string
	timeout  time.Duration
	http     *http.Client
	limiter  *rate.Limiter
}

type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message ChatMessage `json:"message"`
}

type HTTPStatusError struct {
	StatusCode int
	RetryAfter string
}

func (e *HTTPStatusError) Error() string {
	if e.RetryAfter != "" {
		return fmt.Sprintf("llm returned status %d with Retry-After %q", e.StatusCode, e.RetryAfter)
	}
	return fmt.Sprintf("llm returned status %d", e.StatusCode)
}

func NewClient(endpoint, apiKey, model string, timeout time.Duration, limiter *rate.Limiter) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if model == "" {
		model = DefaultModel
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if limiter == nil {
		limiter = rate.NewLimiter(rate.Limit(5), 5)
	}
	return &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		apiKey:   apiKey,
		model:    model,
		timeout:  timeout,
		http:     &http.Client{Timeout: timeout},
		limiter:  limiter,
	}
}

func (c *Client) GenerateRecommendation(ctx context.Context, prompt string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return "", err
		}

		reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
		content, _, err := c.do(reqCtx, prompt)
		cancel()
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == MaxAttempts {
			return "", err
		}
		delay := retryDelay(attempt, err)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
	}
	return "", lastErr
}

func (c *Client) do(ctx context.Context, prompt string) (string, time.Duration, error) {
	started := time.Now()
	reqBody := ChatCompletionRequest{
		Model: c.model,
		Messages: []ChatMessage{
			{Role: "system", Content: "You are an empathetic recommendation assistant. Respond only in Russian."},
			{Role: "user", Content: prompt},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", time.Since(started), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Since(started), &HTTPStatusError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Since(started), err
	}
	content, err := ParseCompletionResponse(body)
	if err != nil {
		return "", time.Since(started), err
	}
	return content, time.Since(started), nil
}

func ParseCompletionResponse(data []byte) (string, error) {
	var response ChatCompletionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("%w: %w", ErrMalformedJSON, err)
	}
	if len(response.Choices) == 0 {
		return "", ErrNoChoices
	}
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return "", ErrEmptyContent
	}
	return content, nil
}

func isRetryable(err error) bool {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.StatusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryDelay(attempt int, err error) time.Duration {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusTooManyRequests && statusErr.RetryAfter != "" {
		if retryAfter, ok := parseRetryAfter(statusErr.RetryAfter); ok {
			return retryAfter
		}
	}
	switch attempt {
	case 2:
		return 500 * time.Millisecond
	case 3:
		return time.Second
	default:
		return 0
	}
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(parsed)
	if delay < 0 {
		return 0, true
	}
	return delay, true
}
