// Package rabbitmq is an AMQP-backed broker.Bus implementation.
// Subject maps to a routing key on a topic exchange (default "chassis").
package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dxrlabs/chassis/broker"
	amqp "github.com/rabbitmq/amqp091-go"
)

const drainTimeout = 10 * time.Second

// Option customizes the RabbitMQ client.
type Option func(*options)

type options struct {
	exchange string
}

// WithExchange sets the topic exchange name (default "chassis").
func WithExchange(name string) Option {
	return func(o *options) { o.exchange = name }
}

// Client is a RabbitMQ-backed Publisher and Subscriber.
type Client struct {
	conn     *amqp.Connection
	pub      *amqp.Channel
	log      *slog.Logger
	exchange string

	mu      sync.Mutex
	cancels []context.CancelFunc
	wg      sync.WaitGroup
}

var _ broker.Bus = (*Client)(nil)

// New dials RabbitMQ at url (amqp://…) and declares a topic exchange.
func New(ctx context.Context, url string, log *slog.Logger, opts ...Option) (*Client, func(), error) {
	if url == "" {
		return nil, nil, fmt.Errorf("rabbitmq: url is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "broker/rabbitmq")

	cfg := options{exchange: "chassis"}
	for _, opt := range opts {
		opt(&cfg)
	}

	select {
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("rabbitmq: connect canceled: %w", ctx.Err())
	default:
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("rabbitmq: channel: %w", err)
	}

	if err := ch.ExchangeDeclare(cfg.exchange, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}

	c := &Client{conn: conn, pub: ch, log: log, exchange: cfg.exchange}
	cleanup := func() { c.drainAndClose() }
	log.Info("connected", "exchange", cfg.exchange)
	return c, cleanup, nil
}

// Publish publishes payload with routing key = subject.
func (c *Client) Publish(ctx context.Context, subject string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("rabbitmq: publish: %w", err)
	}
	err := c.pub.PublishWithContext(ctx, c.exchange, subject, false, false, amqp.Publishing{
		ContentType:  "application/octet-stream",
		Body:         payload,
		DeliveryMode: amqp.Persistent,
	})
	if err != nil {
		return fmt.Errorf("rabbitmq: publish %s: %w", subject, err)
	}
	return nil
}

// Subscribe binds an exclusive queue to subject (routing key) and consumes.
func (c *Client) Subscribe(subject string, handler broker.Handler) error {
	if handler == nil {
		return fmt.Errorf("rabbitmq: handler is required")
	}

	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq: subscribe channel: %w", err)
	}

	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq: declare queue: %w", err)
	}
	if err := ch.QueueBind(q.Name, subject, c.exchange, false, nil); err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq: bind: %w", err)
	}

	deliveries, err := ch.Consume(q.Name, "", false, true, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq: consume: %w", err)
	}

	subCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancels = append(c.cancels, cancel)
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() { _ = ch.Close() }()
		for {
			select {
			case <-subCtx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				c.wg.Add(1)
				func() {
					defer c.wg.Done()
					if err := handler(subCtx, subject, d.Body); err != nil {
						c.log.Error("handler failed", "subject", subject, "err", err)
						_ = d.Nack(false, false)
						return
					}
					_ = d.Ack(false)
				}()
			}
		}
	}()

	c.log.Info("subscribed", "subject", subject)
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

	_ = c.pub.Close()
	_ = c.conn.Close()
	c.log.Info("closed")
}
