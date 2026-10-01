package engine

import (
	"fmt"
	"time"
)

// QuoteLifecycleV1 is schema version 1 of how long a price tag stays valid.
//
// Validity is a commercial decision, not a platform constant: an immediate taxi
// quote should expire quickly in a volatile market, while a vendor order paid
// minutes later still needs its agreed delivery fee to hold. BindGrace covers
// exactly that case -- an order that was approved in time but paid for after
// expiry can still bind, provided the caller supplies proof of when the customer
// accepted.
type QuoteLifecycleV1 struct {
	SchemaVersion             int            `json:"schema_version"`
	DefaultTTLSeconds         int            `json:"default_ttl_seconds"`
	TTLSecondsByScopeChannel  map[string]int `json:"ttl_seconds_by_scope_channel,omitempty"`
	BindGraceSecondsByChannel map[string]int `json:"bind_grace_seconds_by_channel,omitempty"`
}

// Kind returns the policy kind this body belongs to.
func (QuoteLifecycleV1) Kind() string { return KindQuoteLifecycle }

// Validate reports every problem with the body at once.
func (q QuoteLifecycleV1) Validate() error {
	var problems []string
	if q.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	if q.DefaultTTLSeconds <= 0 {
		problems = append(problems, "default_ttl_seconds must be positive")
	}
	for key, seconds := range q.TTLSecondsByScopeChannel {
		scope, channel, ok := splitScopeChannel(key)
		if !ok || !scope.Valid() || !channel.Valid() {
			problems = append(problems, fmt.Sprintf("ttl_seconds_by_scope_channel key %q must be SCOPE/CHANNEL", key))
		}
		if seconds <= 0 {
			problems = append(problems, fmt.Sprintf("ttl for %q must be positive", key))
		}
	}
	for channel, seconds := range q.BindGraceSecondsByChannel {
		if !Channel(channel).Valid() {
			problems = append(problems, fmt.Sprintf("bind_grace_seconds_by_channel has unknown channel %q", channel))
		}
		if seconds < 0 {
			problems = append(problems, fmt.Sprintf("bind grace for %q cannot be negative", channel))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("quote.lifecycle v1: %s", join(problems))
	}
	return nil
}

// TTLFor returns how long a quote for this scope and channel stays valid.
func (q QuoteLifecycleV1) TTLFor(scope Scope, channel Channel) time.Duration {
	if seconds, ok := q.TTLSecondsByScopeChannel[string(scope)+"/"+string(channel)]; ok {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(q.DefaultTTLSeconds) * time.Second
}

// BindGraceFor returns how long after expiry a quote may still be bound, given
// proof that the customer accepted it before it expired.
func (q QuoteLifecycleV1) BindGraceFor(channel Channel) time.Duration {
	return time.Duration(q.BindGraceSecondsByChannel[string(channel)]) * time.Second
}
