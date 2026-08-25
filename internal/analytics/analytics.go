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

func (c *Client) Aggregates(ctx context.Context, chatID string, since, until time.Time) (AnalyticsResponse, error) {
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("since", since.Format(time.RFC3339))
	values.Set("until", until.Format(time.RFC3339))

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

	var response AnalyticsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return AnalyticsResponse{}, err
	}
	return response, nil
}

type AnalyticsResponse struct {
	ChatID           string        `json:"chat_id"`
	Since            time.Time     `json:"since"`
	Until            time.Time     `json:"until"`
	PeriodSummary    Summary       `json:"period_summary"`
	Summary          Summary       `json:"summary"`
	Messages         MessageStats  `json:"messages"`
	Activity         ActivityStats `json:"activity"`
	ActiveDays       int           `json:"active_days"`
	TotalMessages    int           `json:"total_messages"`
	SentMessages     int           `json:"sent_messages"`
	ReceivedMessages int           `json:"received_messages"`
	MinutesActive    int           `json:"minutes_active"`
	TopChannels      []RankedItem  `json:"top_channels"`
	TopTopics        []RankedItem  `json:"top_topics"`
	Tags             []string      `json:"tags"`
	LastActiveAt     *time.Time    `json:"last_active_at"`
}

type Summary struct {
	ActiveDays       int          `json:"active_days"`
	TotalMessages    int          `json:"total_messages"`
	SentMessages     int          `json:"sent_messages"`
	ReceivedMessages int          `json:"received_messages"`
	MinutesActive    int          `json:"minutes_active"`
	TopChannels      []RankedItem `json:"top_channels"`
	TopTopics        []RankedItem `json:"top_topics"`
	Tags             []string     `json:"tags"`
	LastActiveAt     *time.Time   `json:"last_active_at"`
}

type MessageStats struct {
	Total    int `json:"total"`
	Sent     int `json:"sent"`
	Received int `json:"received"`
}

type ActivityStats struct {
	ActiveDays    int          `json:"active_days"`
	MinutesActive int          `json:"minutes_active"`
	TopChannels   []RankedItem `json:"top_channels"`
	TopTopics     []RankedItem `json:"top_topics"`
}

type RankedItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (r *AnalyticsResponse) UnmarshalJSON(data []byte) error {
	type alias AnalyticsResponse
	var aux alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*r = AnalyticsResponse(aux)
	applySummary(r, r.PeriodSummary)
	applySummary(r, r.Summary)
	applyMessages(r, r.Messages)
	applyActivity(r, r.Activity)
	return nil
}

func applySummary(r *AnalyticsResponse, s Summary) {
	if r.ActiveDays == 0 {
		r.ActiveDays = s.ActiveDays
	}
	if r.TotalMessages == 0 {
		r.TotalMessages = s.TotalMessages
	}
	if r.SentMessages == 0 {
		r.SentMessages = s.SentMessages
	}
	if r.ReceivedMessages == 0 {
		r.ReceivedMessages = s.ReceivedMessages
	}
	if r.MinutesActive == 0 {
		r.MinutesActive = s.MinutesActive
	}
	if len(r.TopChannels) == 0 {
		r.TopChannels = s.TopChannels
	}
	if len(r.TopTopics) == 0 {
		r.TopTopics = s.TopTopics
	}
	if len(r.Tags) == 0 {
		r.Tags = s.Tags
	}
	if r.LastActiveAt == nil {
		r.LastActiveAt = s.LastActiveAt
	}
}

func applyMessages(r *AnalyticsResponse, m MessageStats) {
	if r.TotalMessages == 0 {
		r.TotalMessages = m.Total
	}
	if r.SentMessages == 0 {
		r.SentMessages = m.Sent
	}
	if r.ReceivedMessages == 0 {
		r.ReceivedMessages = m.Received
	}
}

func applyActivity(r *AnalyticsResponse, a ActivityStats) {
	if r.ActiveDays == 0 {
		r.ActiveDays = a.ActiveDays
	}
	if r.MinutesActive == 0 {
		r.MinutesActive = a.MinutesActive
	}
	if len(r.TopChannels) == 0 {
		r.TopChannels = a.TopChannels
	}
	if len(r.TopTopics) == 0 {
		r.TopTopics = a.TopTopics
	}
}

func (r AnalyticsResponse) Empty() bool {
	return r.ChatID == "" && r.ActiveDays == 0 && r.TotalMessages == 0 && r.SentMessages == 0 && r.ReceivedMessages == 0 && r.MinutesActive == 0 && len(r.TopChannels) == 0 && len(r.TopTopics) == 0 && len(r.Tags) == 0 && r.LastActiveAt == nil
}

func (r AnalyticsResponse) SummaryText() string {
	parts := []string{}
	if r.ActiveDays > 0 {
		parts = append(parts, fmt.Sprintf("active %d days", r.ActiveDays))
	}
	if r.TotalMessages > 0 {
		parts = append(parts, fmt.Sprintf("%d total messages", r.TotalMessages))
	}
	if r.SentMessages > 0 {
		parts = append(parts, fmt.Sprintf("%d sent", r.SentMessages))
	}
	if r.ReceivedMessages > 0 {
		parts = append(parts, fmt.Sprintf("%d received", r.ReceivedMessages))
	}
	if r.MinutesActive > 0 {
		parts = append(parts, fmt.Sprintf("%d active minutes", r.MinutesActive))
	}
	if len(r.TopChannels) > 0 {
		parts = append(parts, fmt.Sprintf("top channel: %s", r.TopChannels[0].Name))
	}
	if len(r.TopTopics) > 0 {
		parts = append(parts, fmt.Sprintf("top topic: %s", r.TopTopics[0].Name))
	}
	if len(r.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("tags: %s", strings.Join(r.Tags, ", ")))
	}
	if r.LastActiveAt != nil {
		parts = append(parts, fmt.Sprintf("last active %s", r.LastActiveAt.Format(time.RFC3339)))
	}
	if len(parts) == 0 {
		return "no activity data available"
	}
	return strings.Join(parts, "; ")
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
