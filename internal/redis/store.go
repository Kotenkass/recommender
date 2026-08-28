package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"recommender/internal/redispubsub"
)

const (
	WeeklyRecoChannel     = "weekly_reco"
	SendMessageChannel    = "send_message"
	ProcessingLockKey     = "recommender:processing_lock:%s"
	LastRecommendKey      = "recommender:last_recommendation:%s"
	LockTTL               = 10 * time.Minute
	LastRecommendationTTL = 6 * 24 * time.Hour
)

type RedisStore interface {
	Subscribe(ctx context.Context) redispubsub.PubSub
	CheckLastRecommendation(ctx context.Context, chatID string) (bool, error)
	AcquireProcessingLock(ctx context.Context, chatID string) (bool, func(), error)
	PublishRecommendation(ctx context.Context, text string) error
	StoreLastRecommendation(ctx context.Context, chatID, text string) error
	Check(ctx context.Context) error
	Close() error
}

type Store struct {
	client *goredis.Client
}

func NewStore(client *goredis.Client) *Store {
	return &Store{client: client}
}

func NewClientFromURL(rawURL string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	return goredis.NewClient(opts), nil
}

func (s *Store) Subscribe(ctx context.Context) redispubsub.PubSub {
	return s.client.Subscribe(ctx, WeeklyRecoChannel)
}

func (s *Store) CheckLastRecommendation(ctx context.Context, chatID string) (bool, error) {
	exists, err := s.client.Exists(ctx, LastRecommendKeyPattern(chatID)).Result()
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (s *Store) AcquireProcessingLock(ctx context.Context, chatID string) (bool, func(), error) {
	lockKey := fmt.Sprintf(ProcessingLockKey, chatID)
	token := time.Now().UTC().Format(time.RFC3339Nano)
	ok, err := s.client.SetNX(ctx, lockKey, token, LockTTL).Result()
	if err != nil || !ok {
		return ok, func() {}, err
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		current, err := s.client.Get(releaseCtx, lockKey).Result()
		if err == nil && current == token {
			_, _ = s.client.Del(releaseCtx, lockKey).Result()
		}
	}
	return true, release, nil
}

func (s *Store) PublishRecommendation(ctx context.Context, text string) error {
	return s.client.Publish(ctx, SendMessageChannel, text).Err()
}

func (s *Store) StoreLastRecommendation(ctx context.Context, chatID, text string) error {
	return s.client.Set(ctx, LastRecommendKeyPattern(chatID), text, LastRecommendationTTL).Err()
}

func (s *Store) Check(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *Store) Close() error {
	return s.client.Close()
}

func LastRecommendKeyPattern(chatID string) string {
	return fmt.Sprintf(LastRecommendKey, chatID)
}
