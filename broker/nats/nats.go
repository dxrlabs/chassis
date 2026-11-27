// Package nats is a NATS-backed broker.Bus implementation.
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dxrlabs/chassis/broker"
	"github.com/nats-io/nats.go"
)

const drainTimeout = 10 * time.Second

// Client is a NATS-backed Publisher and Subscriber.
type Client struct {
	nc  *nats.Conn
	log *slog.Logger

	mu   sync.Mutex
	subs []*nats.Subscription
	wg   sync.WaitGroup
}

var _ broker.Bus = (*Client)(nil)

// New connects to NATS at url and returns a Client with a drain-aware cleanup.
func New(ctx context.Context, url string, log *slog.Logger) (*Client, func(), error) {
	if url == "" {
		return nil, nil, fmt.Errorf("nats: url is required")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "broker/nats")

	select {
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("nats: connect canceled: %w", ctx.Err())
	default:
	}

	nc, err := nats.Connect(url,
		nats.Name("chassis-broker-nats"),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warn("disconnected", "err", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Info("reconnected")
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("nats: connect: %w", err)
	}

	c := &Client{nc: nc, log: log}
	cleanup := func() { c.drainAndClose() }
	log.Info("connected", "url", nc.ConnectedUrl())
	return c, cleanup, nil
}

// Publish sends payload to subject.
func (c *Client) Publish(ctx context.Context, subject string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("nats: publish: %w", err)
	}
	if err := c.nc.Publish(subject, payload); err != nil {
		return fmt.Errorf("nats: publish %s: %w", subject, err)
	}
	return nil
}

// Subscribe registers handler for subject.
func (c *Client) Subscribe(subject string, handler broker.Handler) error {
	if handler == nil {
		return fmt.Errorf("nats: handler is required")
	}

	sub, err := c.nc.Subscribe(subject, func(msg *nats.Msg) {
		c.wg.Add(1)
		defer c.wg.Done()
		if err := handler(context.Background(), msg.Subject, msg.Data); err != nil {
			c.log.Error("handler failed", "subject", msg.Subject, "err", err)
		}
	})
	if err != nil {
		return fmt.Errorf("nats: subscribe %s: %w", subject, err)
	}

	c.mu.Lock()
	c.subs = append(c.subs, sub)
	c.mu.Unlock()
	c.log.Info("subscribed", "subject", subject)
	return nil
}

func (c *Client) drainAndClose() {
	c.mu.Lock()
	subs := append([]*nats.Subscription(nil), c.subs...)
	c.mu.Unlock()

	for _, sub := range subs {
		if err := sub.Drain(); err != nil {
			c.log.Warn("subscription drain failed", "err", err)
		}
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

	if err := c.nc.Drain(); err != nil {
		c.log.Warn("connection drain failed", "err", err)
		c.nc.Close()
	}
	c.log.Info("closed")
}
