// Package kafka is a Kafka-backed broker.Bus implementation.
// Subject maps to a topic name. Brokers is a comma-joined host list
// (e.g. "127.0.0.1:9092").
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dxrlabs/chassis/broker"
	"github.com/segmentio/kafka-go"
)

const drainTimeout = 10 * time.Second

// Option customizes the Kafka client.
type Option func(*options)

type options struct {
	groupID string
}

// WithGroupID sets the consumer group for Subscribe (default "chassis").
func WithGroupID(id string) Option {
	return func(o *options) { o.groupID = id }
}

// Client is a Kafka-backed Publisher and Subscriber.
type Client struct {
	brokers []string
	log     *slog.Logger
	groupID string
	writer  *kafka.Writer

	mu      sync.Mutex
	readers []*kafka.Reader
	cancels []context.CancelFunc
	wg      sync.WaitGroup
}

var _ broker.Bus = (*Client)(nil)

// New creates a Kafka client targeting brokers (comma-separated host:port).
func New(ctx context.Context, brokers string, log *slog.Logger, opts ...Option) (*Client, func(), error) {
	if brokers == "" {
		return nil, nil, fmt.Errorf("kafka: brokers is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "broker/kafka")

	cfg := options{groupID: "chassis"}
	for _, opt := range opts {
		opt(&cfg)
	}

	select {
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("kafka: connect canceled: %w", ctx.Err())
	default:
	}

	addrs := splitBrokers(brokers)
	w := &kafka.Writer{
		Addr:         kafka.TCP(addrs...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}

	c := &Client{brokers: addrs, log: log, groupID: cfg.groupID, writer: w}
	cleanup := func() { c.drainAndClose() }
	log.Info("ready", "brokers", addrs, "group", cfg.groupID)
	return c, cleanup, nil
}

// Publish writes payload to topic = subject.
func (c *Client) Publish(ctx context.Context, subject string, payload []byte) error {
	err := c.writer.WriteMessages(ctx, kafka.Message{
		Topic: subject,
		Value: payload,
	})
	if err != nil {
		return fmt.Errorf("kafka: publish %s: %w", subject, err)
	}
	return nil
}

// Subscribe consumes topic = subject in the configured consumer group.
func (c *Client) Subscribe(subject string, handler broker.Handler) error {
	if handler == nil {
		return fmt.Errorf("kafka: handler is required")
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  c.brokers,
		GroupID:  c.groupID,
		Topic:    subject,
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	subCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.readers = append(c.readers, r)
	c.cancels = append(c.cancels, cancel)
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			msg, err := r.FetchMessage(subCtx)
			if err != nil {
				if subCtx.Err() != nil {
					return
				}
				c.log.Error("fetch failed", "topic", subject, "err", err)
				continue
			}
			if err := handler(subCtx, msg.Topic, msg.Value); err != nil {
				c.log.Error("handler failed", "topic", subject, "err", err)
				continue
			}
			if err := r.CommitMessages(subCtx, msg); err != nil {
				c.log.Warn("commit failed", "topic", subject, "err", err)
			}
		}
	}()

	c.log.Info("subscribed", "topic", subject)
	return nil
}

func (c *Client) drainAndClose() {
	c.mu.Lock()
	cancels := append([]context.CancelFunc(nil), c.cancels...)
	readers := append([]*kafka.Reader(nil), c.readers...)
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

	for _, r := range readers {
		_ = r.Close()
	}
	_ = c.writer.Close()
	c.log.Info("closed")
}

func splitBrokers(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
