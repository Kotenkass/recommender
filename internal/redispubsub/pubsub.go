package redispubsub

import goredis "github.com/redis/go-redis/v9"

// PubSub is the subset of go-redis PubSub consumed by the service.
type PubSub interface {
	Channel(...goredis.ChannelOption) <-chan *goredis.Message
	Close() error
}
