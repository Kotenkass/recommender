package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client, err := NewClientFromURL("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewClientFromURL: %v", err)
	}
	return NewStore(client), mr
}

func TestDuplicateCacheHandling(t *testing.T) {
	store, mr := newTestStore(t)
	defer store.Close()

	if ok, err := store.CheckLastRecommendation(context.Background(), "42"); err != nil || ok {
		t.Fatalf("initial cache check ok=%v err=%v, want false nil", ok, err)
	}
	if ok, release, err := store.AcquireProcessingLock(context.Background(), "42"); err != nil || !ok {
		t.Fatalf("first lock ok=%v err=%v, want true nil", ok, err)
	} else {
		defer release()
	}
	if ok, _, err := store.AcquireProcessingLock(context.Background(), "42"); err != nil || ok {
		t.Fatalf("second lock ok=%v err=%v, want false nil", ok, err)
	}
	if err := store.PublishRecommendation(context.Background(), "42", "recommendation"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := store.StoreLastRecommendation(context.Background(), "42", "recommendation"); err != nil {
		t.Fatalf("store: %v", err)
	}
	if ok, err := store.CheckLastRecommendation(context.Background(), "42"); err != nil || !ok {
		t.Fatalf("cache check ok=%v err=%v, want true nil", ok, err)
	}

	if err := mr.Set(LastRecommendKeyPattern("42"), "recommendation"); err != nil {
		t.Fatalf("set: %v", err)
	}
	mr.SetTTL(LastRecommendKeyPattern("42"), LastRecommendationTTL)
	val := mr.TTL(LastRecommendKeyPattern("42"))
	if val < 5*24*time.Hour || val > 6*24*time.Hour {
		t.Fatalf("ttl = %v, want about 6 days", val)
	}
}

func TestPublishRecommendationPayload(t *testing.T) {
	store, mr := newTestStore(t)
	defer store.Close()

	client, err := NewClientFromURL("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewClientFromURL: %v", err)
	}
	defer client.Close()

	pubsub := client.Subscribe(context.Background(), "send_message")
	defer pubsub.Close()
	ch := pubsub.Channel()

	if err := store.PublishRecommendation(context.Background(), "7", "text"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case msg := <-ch:
		if msg == nil || msg.Channel != "send_message" || msg.Payload == "" {
			t.Fatalf("unexpected message: %#v", msg)
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload["chat_id"] != "7" || payload["text"] != "text" {
			t.Fatalf("payload = %#v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish message")
	}
}
