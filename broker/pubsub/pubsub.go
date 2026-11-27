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

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/dxrlabs/chassis/broker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	client  *gcppubsub.Client
	log     *slog.Logger
	group   string
	project string

	mu         sync.Mutex
	publishers map[string]*gcppubsub.Publisher
	cancels    []context.CancelFunc
	wg         sync.WaitGroup
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

	client, err := gcppubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("pubsub: client: %w", err)
	}

	c := &Client{
		client:     client,
		log:        log,
		group:      cfg.group,
		project:    projectID,
		publishers: make(map[string]*gcppubsub.Publisher),
	}
	cleanup := func() { c.drainAndClose() }
	log.Info("ready", "project", projectID, "group", cfg.group)
	return c, cleanup, nil
}

// Publish publishes payload to topic ID = subject (topic must already exist).
func (c *Client) Publish(ctx context.Context, subject string, payload []byte) error {
	pub := c.publisher(subject)
	result := pub.Publish(ctx, &gcppubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish %s: %w", subject, err)
	}
	return nil
}

func (c *Client) publisher(subject string) *gcppubsub.Publisher {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.publishers[subject]; ok {
		return p
	}
	p := c.client.Publisher(subject)
	c.publishers[subject] = p
	return p
}

// Subscribe receives messages from topic = subject via a durable subscription.
func (c *Client) Subscribe(subject string, handler broker.Handler) error {
	if handler == nil {
		return fmt.Errorf("pubsub: handler is required")
	}

	subID := c.group + "-" + subject
	subName := fmt.Sprintf("projects/%s/subscriptions/%s", c.project, subID)
	topicName := fmt.Sprintf("projects/%s/topics/%s", c.project, subject)

	if err := c.ensureSubscription(context.Background(), subName, topicName); err != nil {
		return err
	}

	sub := c.client.Subscriber(subName)

	subCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancels = append(c.cancels, cancel)
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		err := sub.Receive(subCtx, func(ctx context.Context, msg *gcppubsub.Message) {
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

func (c *Client) ensureSubscription(ctx context.Context, subName, topicName string) error {
	_, err := c.client.SubscriptionAdminClient.GetSubscription(ctx, &pubsubpb.GetSubscriptionRequest{
		Subscription: subName,
	})
	if err == nil {
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return fmt.Errorf("pubsub: get subscription: %w", err)
	}
	_, err = c.client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:               subName,
		Topic:              topicName,
		AckDeadlineSeconds: 20,
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return fmt.Errorf("pubsub: create subscription: %w", err)
	}
	return nil
}

func (c *Client) drainAndClose() {
	c.mu.Lock()
	cancels := append([]context.CancelFunc(nil), c.cancels...)
	pubs := make([]*gcppubsub.Publisher, 0, len(c.publishers))
	for _, p := range c.publishers {
		pubs = append(pubs, p)
	}
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

	for _, p := range pubs {
		p.Stop()
	}
	_ = c.client.Close()
	c.log.Info("closed")
}
