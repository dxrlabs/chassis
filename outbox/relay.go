package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/dxrlabs/chassis/broker"
)

const (
	defaultPollInterval = time.Second
	defaultBatchSize    = 100
	defaultLease        = 30 * time.Second
	defaultStopTimeout  = 15 * time.Second
)

// Relay polls the outbox and publishes via broker.Publisher.
// Multiple relays are safe when the Store uses lease + SKIP LOCKED (or SQLite IMMEDIATE).
// Delivery is at-least-once: consumers should use InboxHandler.
type Relay struct {
	store    Store
	pub      broker.Publisher
	log      *slog.Logger
	interval time.Duration
	batch    int
	lease    time.Duration
	id       string

	cancel context.CancelFunc
	done   chan struct{}
}

// Option customizes Relay.
type Option func(*Relay)

// WithPollInterval sets how often pending rows are claimed.
func WithPollInterval(d time.Duration) Option {
	return func(r *Relay) { r.interval = d }
}

// WithBatchSize sets max rows per poll.
func WithBatchSize(n int) Option {
	return func(r *Relay) { r.batch = n }
}

// WithLease sets how long a claimed row stays locked before another relay may retry.
func WithLease(d time.Duration) Option {
	return func(r *Relay) { r.lease = d }
}

// WithRelayID sets an identifier for logs (defaults to hostname).
func WithRelayID(id string) Option {
	return func(r *Relay) { r.id = id }
}

// NewRelay starts a background poller. Cleanup cancels and waits for the loop.
func NewRelay(store Store, pub broker.Publisher, log *slog.Logger, opts ...Option) (*Relay, func(), error) {
	if store == nil {
		return nil, nil, fmt.Errorf("outbox: store is required")
	}
	if pub == nil {
		return nil, nil, fmt.Errorf("outbox: publisher is required")
	}
	if log == nil {
		log = slog.Default()
	}

	host, _ := os.Hostname()
	r := &Relay{
		store:    store,
		pub:      pub,
		log:      log.With("component", "outbox"),
		interval: defaultPollInterval,
		batch:    defaultBatchSize,
		lease:    defaultLease,
		id:       host,
		done:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.log = r.log.With("relay_id", r.id)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go r.loop(ctx)

	cleanup := func() {
		cancel()
		select {
		case <-r.done:
			r.log.Info("relay stopped")
		case <-time.After(defaultStopTimeout):
			r.log.Warn("relay stop timed out")
		}
	}
	return r, cleanup, nil
}

func (r *Relay) loop(ctx context.Context) {
	defer close(r.done)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	r.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.poll(ctx)
		}
	}
}

func (r *Relay) poll(ctx context.Context) {
	msgs, err := r.store.ClaimPending(ctx, r.batch, r.lease)
	if err != nil {
		r.log.Error("claim pending", "err", err)
		return
	}
	for _, m := range msgs {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := r.pub.Publish(ctx, m.Subject, m.Payload); err != nil {
			// Leave locked_until set; after lease expiry another relay retries (at-least-once).
			r.log.Error("publish", "id", m.ID, "subject", m.Subject, "err", err)
			return
		}
		if err := r.store.MarkPublished(ctx, m.ID); err != nil {
			r.log.Error("mark published", "id", m.ID, "err", err)
			return
		}
	}
}
