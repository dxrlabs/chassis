// Package pubsub is a GCP Pub/Sub-backed broker.Bus implementation.
// Subject maps to a topic ID within projectID. Subscribe creates (or reuses)
// a subscription named "{group}-{subject}".
package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/dxrlabs/chassis/broker"
)

const drainTimeout = 10 * time.Second

// Option customizes the Pub/Sub client.
type Option func(*options)

type options struct {
	group string
}

// WithGroup sets the subscription name prefix (default "chassis").
func WithGroup(group string) Option {
	return func(o *options) { o.group = group }
}

// Client is a GCP Pub/Sub-backed Publisher and Subscriber.
type Client struct {
	client  *pubsub.Client
	log     *slog.Logger
	group   string
	project string

	mu      sync.Mutex
	cancels []context.CancelFunc
	wg      sync.WaitGroup
}

var _ broker.Bus = (*Client)(nil)

// New creates a Pub/Sub client for projectID.
// Credentials follow Application Default Credentials.
func New(ctx context.Context, projectID string, log *slog.Logger, opts ...Option) (*Client, func(), error) {
	if projectID == "" {
		return nil, nil, fmt.Errorf("pubsub: projectID is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "broker/pubsub")

	cfg := options{group: "chassis"}
	for _, opt := range opts {
		opt(&cfg)
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("pubsub: client: %w", err)
	}

	c := &Client{client: client, log: log, group: cfg.group, project: projectID}
	cleanup := func() { c.drainAndClose() }
	log.Info("ready", "project", projectID, "group", cfg.group)
	return c, cleanup, nil
}

// Publish publishes payload to topic ID = subject (topic must already exist).
func (c *Client) Publish(ctx context.Context, subject string, payload []byte) error {
	topic := c.client.Topic(subject)
	result := topic.Publish(ctx, &pubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish %s: %w", subject, err)
	}
	return nil
}

// Subscribe receives messages from topic = subject via a durable subscription.
func (c *Client) Subscribe(subject string, handler broker.Handler) error {
	if handler == nil {
		return fmt.Errorf("pubsub: handler is required")
	}

	subID := c.group + "-" + subject
	sub := c.client.Subscription(subID)
	exists, err := sub.Exists(context.Background())
	if err != nil {
		return fmt.Errorf("pubsub: subscription exists: %w", err)
	}
	if !exists {
		topic := c.client.Topic(subject)
		sub, err = c.client.CreateSubscription(context.Background(), subID, pubsub.SubscriptionConfig{
			Topic:       topic,
			AckDeadline: 20 * time.Second,
		})
		if err != nil {
			return fmt.Errorf("pubsub: create subscription: %w", err)
		}
	}

	subCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancels = append(c.cancels, cancel)
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		err := sub.Receive(subCtx, func(ctx context.Context, msg *pubsub.Message) {
			c.wg.Add(1)
			defer c.wg.Done()
			if err := handler(ctx, subject, msg.Data); err != nil {
				c.log.Error("handler failed", "subject", subject, "err", err)
				msg.Nack()
				return
			}
			msg.Ack()
		})
		if err != nil && subCtx.Err() == nil {
			c.log.Error("receive stopped", "subject", subject, "err", err)
		}
	}()

	c.log.Info("subscribed", "subject", subject, "subscription", subID)
	return nil
}

func (c *Client) drainAndClose() {
	c.mu.Lock()
	cancels := append([]context.CancelFunc(nil), c.cancels...)
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(drainTimeout):
		c.log.Warn("handler drain timed out", "timeout", drainTimeout)
	}

	_ = c.client.Close()
	c.log.Info("closed")
}
