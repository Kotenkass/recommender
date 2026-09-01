package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestActiveChatIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users" {
			t.Fatalf("path = %s, want /users", r.URL.Path)
		}
		resp := map[string]any{
			"data": []map[string]any{
				{"chatID": float64(1)},
				{"chatID": float64(2)},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ids, err := client.ActiveChatIDs(context.Background())
	if err != nil {
		t.Fatalf("ActiveChatIDs: %v", err)
	}
	if !reflect.DeepEqual(ids, []string{"1", "2"}) {
		t.Fatalf("ids = %#v, want [\"1\", \"2\"]", ids)
	}
}

func TestActiveChatIDsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ids, err := client.ActiveChatIDs(context.Background())
	if err != nil {
		t.Fatalf("ActiveChatIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %#v, want empty slice", ids)
	}
}

func TestUsersResponseUnmarshalArray(t *testing.T) {
	var response UsersResponse
	if err := json.Unmarshal([]byte(`["1","2","1"]`), &response); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(response.ChatIDs, []string{"1", "2"}) {
		t.Fatalf("chat ids = %#v", response.ChatIDs)
	}
}
