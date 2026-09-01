package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client fetches weekly analytics for a chat ID.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

// DailyCount matches what the analytics service actually returns.
type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// AnalyticsResponse is what the recommender service uses internally.
type AnalyticsResponse struct {
	ChatID        string `json:"chat_id"`
	Since         string `json:"since"`
	Until         string `json:"until"`
	ActiveDays    int    `json:"active_days"`
	TotalMessages int    `json:"total_messages"`
}

func (c *Client) Aggregates(ctx context.Context, chatID string, since, until time.Time) (AnalyticsResponse, error) {
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("since", since.Format("2006-01-02"))
	values.Set("until", until.Format("2006-01-02"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/aggregates?"+values.Encode(), nil)
	if err != nil {
		return AnalyticsResponse{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return AnalyticsResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AnalyticsResponse{}, fmt.Errorf("analytics service returned status %d", resp.StatusCode)
	}

	var counts []DailyCount
	if err := json.NewDecoder(resp.Body).Decode(&counts); err != nil {
		return AnalyticsResponse{}, err
	}

	total := 0
	for _, c := range counts {
		total += int(c.Count)
	}

	return AnalyticsResponse{
		ChatID:        chatID,
		Since:         since.Format("2006-01-02"),
		Until:         until.Format("2006-01-02"),
		ActiveDays:    len(counts),
		TotalMessages: total,
	}, nil
}

func (r AnalyticsResponse) Empty() bool {
	return r.ActiveDays == 0 && r.TotalMessages == 0
}

func (r AnalyticsResponse) SummaryText() string {
	if r.Empty() {
		return "no activity data available"
	}
	return fmt.Sprintf("active %d days, %d total messages", r.ActiveDays, r.TotalMessages)
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
