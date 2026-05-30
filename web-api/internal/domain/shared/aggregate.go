// Package shared holds domain primitives reused across aggregates: the base
// aggregate (which records domain events) and the DomainEvent contract.
// This package has NO infrastructure dependencies — pure domain.
package shared

import "time"

// DomainEvent is something that happened in the domain, expressed in past
// tense (UserRegistered, OrderPlaced). Aggregates record them; the UoW
// persists them to the outbox in the same transaction as the state change.
type DomainEvent interface {
	EventName() string
	OccurredAt() time.Time
	// AggregateID identifies the aggregate that emitted the event.
	AggregateID() string
}

// BaseEvent can be embedded by concrete events to satisfy the timestamp/id.
type BaseEvent struct {
	ID       string
	Occurred time.Time
}

func (b BaseEvent) OccurredAt() time.Time { return b.Occurred }
func (b BaseEvent) AggregateID() string   { return b.ID }

// AggregateRoot is embedded by aggregates to accumulate uncommitted events.
type AggregateRoot struct {
	events []DomainEvent
}

// Record appends a domain event to be pulled and persisted by the UoW.
func (a *AggregateRoot) Record(e DomainEvent) {
	a.events = append(a.events, e)
}

// PullEvents returns and clears the recorded events. The write repository
// calls this and hands the events to the outbox within the transaction.
func (a *AggregateRoot) PullEvents() []DomainEvent {
	out := a.events
	a.events = nil
	return out
}
