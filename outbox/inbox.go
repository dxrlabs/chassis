package outbox

import (
	"context"

	"github.com/dxrlabs/chassis/broker"
)

// InboxHandler wraps handler with idempotent inbox claiming.
// messageID must be a stable idempotency key (e.g. strconv of outbox id, or producer hash).
// If the id was already claimed, the handler is skipped and nil is returned.
func InboxHandler(store Store, messageID func(subject string, payload []byte) string, handler broker.Handler) broker.Handler {
	return func(ctx context.Context, subject string, payload []byte) error {
		id := messageID(subject, payload)
		claimed, err := store.TryClaimInbox(ctx, id)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		return handler(ctx, subject, payload)
	}
}
