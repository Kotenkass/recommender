package users

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client fetches active chat IDs from the users service.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

// usersListResponse matches the actual response from the users service.
type usersListResponse struct {
	Data []userDTO `json:"data"`
}

type userDTO struct {
	ChatID int64 `json:"chatID"`
}

func (c *Client) ActiveChatIDs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/users", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("users service returned status %d", resp.StatusCode)
	}

	var listResp usersListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, err
	}

	chatIDs := make([]string, 0, len(listResp.Data))
	for _, u := range listResp.Data {
		chatIDs = append(chatIDs, strconv.FormatInt(u.ChatID, 10))
	}
	return chatIDs, nil
}

// UsersResponse and UnmarshalJSON below are kept for backward compatibility.
type UsersResponse struct {
	ChatIDs []string `json:"chat_ids"`
}

func (r *UsersResponse) UnmarshalJSON(data []byte) error {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil
	}
	if data[0] == '[' {
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			return err
		}
		r.ChatIDs = normalizeChatIDs(ids)
		return nil
	}
	type alias UsersResponse
	var aux alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.ChatIDs = normalizeChatIDs(aux.ChatIDs)
	return nil
}

func normalizeChatIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
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

func QueryValues(values url.Values) string {
	return values.Encode()
}