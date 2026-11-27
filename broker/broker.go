// Package broker defines portable publish/subscribe contracts.
// Concrete drivers live in submodules (nats, rabbitmq, kafka, pubsub) and
// may import this module for Handler and compile-time interface checks.
// Drivers must not import each other.
package broker

import "context"

// Handler processes a single message. Semantics are at-most-once unless a
// driver documents stronger guarantees via its options.
type Handler func(ctx context.Context, subject string, payload []byte) error

// Publisher publishes opaque payloads to a subject (topic / routing key).
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

// Subscriber registers handlers for subjects.
type Subscriber interface {
	Subscribe(subject string, handler Handler) error
}

// Bus is the common Publisher+Subscriber surface apps usually inject.
type Bus interface {
	Publisher
	Subscriber
}
