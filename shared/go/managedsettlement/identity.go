package managedsettlement

import (
	"context"
	"strings"
)

// Identity is the stable financial identity of one completed job's settlement.
//
// It exists because a completion is settled from two places that must never
// disagree: the synchronous path that runs when the job completes, and the
// reconciliation worker that picks up whatever that path could not finish. If
// each derived its own identity, the worker would ask Wallet about a job
// Wallet has never heard of under that name, conclude nothing had settled, and
// settle it a second time.
//
// So both paths build the identity from the same two facts, and nothing else:
// which execution service owns the work, and that service's own id for the
// job. Neither changes after completion, which is what makes the identity
// stable across a restart, a retry or a version bump.
type Identity struct {
	SourceService string
	SourceJobID   string
}

// IdentityFor builds the identity, normalising the service name so a caller
// spelling it "taxi" and one spelling it "TAXI" address the same job.
func IdentityFor(sourceService, sourceJobID string) Identity {
	return Identity{
		SourceService: strings.ToUpper(strings.TrimSpace(sourceService)),
		SourceJobID:   strings.TrimSpace(sourceJobID),
	}
}

// Valid reports whether the identity names a job at all.
func (i Identity) Valid() bool { return i.SourceService != "" && i.SourceJobID != "" }

// Key is the identity as a single comparable string, for logs and for tests
// that assert two code paths produced the same one.
func (i Identity) Key() string { return i.SourceService + ":" + i.SourceJobID }

// Lookup asks Wallet about the job this identity names.
func (c Client) Lookup(ctx context.Context, identity Identity) (Outcome, error) {
	return c.ForJob(ctx, identity.SourceService, identity.SourceJobID)
}

// paymentSourceServices is, for each canonical execution service, every
// source_service spelling its payment intents are created under in
// payment-wallet-service.
//
// The settlement identity and the payment identity are deliberately different
// things. The settlement identity is canonical (TAXI, DISPATCH, HAULING plus
// the service's own job id) because Fleet, Wallet and the execution service
// must all name a settlement the same way. The payment intent, though, was
// created long before -- by the execution service, under the name it was
// provisioned with in Wallet -- and those names are neither canonical nor
// uniform: Taxi pays as "taxi-service", Dispatch as
// "dispatch-delivery-service", Hauling as "driver-hauling-service". The
// directory-style spellings and the canonical lower-case name are listed too,
// so an intent created under any of them resolves.
//
// The list is closed on purpose. Wallet resolves a canonical settlement to a
// payment only through this table, and refuses when more than one intent
// matches, so widening it can never make a settlement pick between two
// payments -- it can only make a payment findable.
var paymentSourceServices = map[string][]string{
	SourceTaxi:     {"taxi-service", "driver-taxi-service", "taxi"},
	SourceDispatch: {"dispatch-delivery-service", "driver-dispatch-delivery-service", "dispatch"},
	SourceHauling:  {"driver-hauling-service", "hauling-service", "hauling"},
}

// CanonicalSource maps any recognised spelling of an execution service --
// the canonical name in any case, or one of its payment source_service
// spellings -- onto its canonical name. ok is false for anything else.
func CanonicalSource(name string) (canonical string, ok bool) {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if _, known := paymentSourceServices[normalized]; known {
		return normalized, true
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	for canonicalName, spellings := range paymentSourceServices {
		for _, spelling := range spellings {
			if spelling == lower {
				return canonicalName, true
			}
		}
	}
	return "", false
}

// PaymentSourceServices returns the payment-intent source_service spellings
// for a canonical execution service, or nil when the name is not one. The
// returned slice is a copy.
func PaymentSourceServices(canonical string) []string {
	spellings := paymentSourceServices[strings.ToUpper(strings.TrimSpace(canonical))]
	if spellings == nil {
		return nil
	}
	return append([]string(nil), spellings...)
}
