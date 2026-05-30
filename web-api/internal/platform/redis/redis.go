package redis

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/infrastructure/config"
)

type Client struct{ *redis.Client }

func NewClient(i do.Injector) (*Client, error) {
	cfg := do.MustInvoke[*config.Config](i)
	c := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Client{Client: c}, nil
}

func (c *Client) Shutdown() error { return c.Client.Close() }
func (c *Client) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.Client.Ping(ctx).Err()
}

// NewPublisher builds a Watermill Redis Streams publisher (used by outbox relay).
func NewPublisher(i do.Injector) (message.Publisher, error) {
	c := do.MustInvoke[*Client](i)
	log := do.MustInvoke[*slog.Logger](i)
	return redisstream.NewPublisher(
		redisstream.PublisherConfig{Client: c.Client},
		watermill.NewSlogLogger(log),
	)
}

// NewSubscriber builds a Watermill Redis Streams subscriber for a consumer group.
func NewSubscriber(i do.Injector, group string) (message.Subscriber, error) {
	c := do.MustInvoke[*Client](i)
	log := do.MustInvoke[*slog.Logger](i)
	// ClaimInterval=0 disables Watermill's idle-message auto-claim: with
	// fast-arriving batches it races with the in-process handler and ACKs
	// messages without invoking it (visible in the echo pipeline: 3 image
	// events publish in quick succession, only the first dispatches a command,
	// the other two are silently ACKed by the claim worker).
	return redisstream.NewSubscriber(
		redisstream.SubscriberConfig{
			Client:        c.Client,
			ConsumerGroup: group,
			ClaimInterval: 24 * 365 * time.Hour, // ≈ never within a process lifetime
		},
		watermill.NewSlogLogger(log),
	)
}
