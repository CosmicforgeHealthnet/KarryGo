// Package policyingest is the one contract by which an execution service
// takes a network/product/capability policy published by Admin and turns it
// into an immutable local snapshot.
//
// It exists because matching must never make Admin a synchronous dependency.
// An assignment that has to ask a control plane whether a truck may take a job
// stops dispatching the moment that control plane blinks, and a control plane
// that is on the critical path of every assignment is a control plane whose
// outage is a total outage. So policy is PUSHED, applied locally and read
// locally, and the only thing this package guarantees is that what lands
// locally is authentic, ordered and immutable.
//
// Four properties do the work, and each of them exists because the obvious
// implementation gets it wrong:
//
//   - event_id deduplication, because delivery is at-least-once and applying
//     the same publication twice would bump versions that committed work is
//     pinned to.
//   - source_version monotonicity, because a delayed publication that
//     overtakes a newer one would silently roll the network back.
//   - checksum verification, because a policy that arrives corrupted is worse
//     than one that does not arrive: the first is applied and believed.
//   - an explicit RECONCILIATION_REQUIRED outcome, because a read-back that
//     times out has NOT failed -- Admin may have served it -- and retrying a
//     read is safe while assuming failure is not.
//
// Pricing is absent by construction: a Document carries capability and product
// facts only. Fares live in the quote contracts and stay there.
package policyingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// EventName is the canonical fact Admin publishes when a catalogue version
// becomes effective.
//
// Nothing on this branch published a network.* event before, so this is the
// first use of the name rather than a rename of an existing one. Recovery
// events, which DO already exist, keep their established authority-prefixed
// spelling instead (taxi.recovery.started and friends) -- a shared vocabulary
// is worth having, but not worth breaking live consumers for.
const EventName = "network.catalog.published"

// SchemaVersion is the envelope schema this package understands.
//
// A publication carrying a different version is REJECTED rather than
// best-effort parsed. A control-plane document half-understood by an execution
// service is how a capability rule silently stops being enforced.
const SchemaVersion = 1

// IngressPath is where each authority mounts its publication sink, appended to
// that service's own already-authenticated internal base.
const IngressPath = "/internal/policy/publications"

var (
	// ErrInvalidPublication means the publication is malformed or fails a
	// check that no retry could fix.
	ErrInvalidPublication = errors.New("policyingest: publication is invalid")
	// ErrChecksumMismatch means the document does not match its declared
	// checksum.
	ErrChecksumMismatch = errors.New("policyingest: document checksum mismatch")
	// ErrUnsupportedSchema means the publication speaks a schema version this
	// build does not understand.
	ErrUnsupportedSchema = errors.New("policyingest: unsupported schema version")
)

// Publication is the payload of a network.catalog.published event.
//
// Document and DocumentRef are alternatives: a small catalogue travels inline,
// a large one travels as a reference the consumer reads back over HMAC. Both
// paths end at the same immutable snapshot, and the checksum is what makes the
// two equivalent rather than merely similar.
type Publication struct {
	// PolicyKind names which local snapshot family this belongs to, using the
	// consuming service's own kind vocabulary (DISPATCH_CAPABILITY,
	// HAULING_CAPABILITY, ...). A publication for a kind this service does not
	// keep is ignored rather than rejected: Admin broadcasts one catalogue to
	// every authority, and each takes only its own.
	PolicyKind string `json:"policy_kind"`

	// PolicyVersion is the control plane's own immutable version label. It is
	// what committed work pins, so it is a string rather than a number: the
	// value's meaning belongs to Admin, not to its consumers.
	PolicyVersion string `json:"policy_version"`

	// Checksum is the lowercase hex SHA-256 of the canonical document.
	Checksum string `json:"checksum"`

	// EffectiveAt is when this version starts being the current one. A future
	// value is legitimate and is stored as-is; readers select the newest
	// snapshot whose effective_at has passed.
	EffectiveAt time.Time `json:"effective_at"`

	// Document is the inline catalogue, when it travelled with the event.
	Document json.RawMessage `json:"document,omitempty"`

	// DocumentRef is an authoritative URL to read back, when it did not. It
	// must be absolute and is only ever fetched over the service's own HMAC
	// client, never anonymously.
	DocumentRef string `json:"document_ref,omitempty"`
}

// Outcome is what ingestion did with one publication.
type Outcome string

const (
	// OutcomeApplied means a new immutable snapshot was stored.
	OutcomeApplied Outcome = "APPLIED"

	// OutcomeDuplicate means this event_id was already processed. It is a
	// SUCCESS: the caller is an at-least-once pipeline, and turning a replay
	// into an error would make it retry the same message forever.
	OutcomeDuplicate Outcome = "DUPLICATE"

	// OutcomeStale means a newer source_version for this kind is already
	// applied. Also a success, and also a no-op.
	OutcomeStale Outcome = "STALE"

	// OutcomeIgnored means the publication is for a policy kind this service
	// does not keep.
	OutcomeIgnored Outcome = "IGNORED"

	// OutcomeRejected means the publication failed a check no retry can fix:
	// bad schema, bad checksum, malformed document.
	OutcomeRejected Outcome = "REJECTED"

	// OutcomeReconciliationRequired means the document could not be read back
	// and the result is UNKNOWN rather than failed.
	//
	// It is deliberately distinct from REJECTED. A read-back that timed out
	// may well have been served; recording it as a rejection would abandon a
	// policy version the network has genuinely moved to, and leave this
	// service matching against a stale catalogue with nothing to indicate it.
	// The publication is stored pending and retried.
	OutcomeReconciliationRequired Outcome = "RECONCILIATION_REQUIRED"
)

// Terminal reports whether an outcome is final for this event.
func (o Outcome) Terminal() bool { return o != OutcomeReconciliationRequired }

// Stable reason codes attached to an outcome, so an operator reading the
// inbox sees WHY rather than only WHAT.
const (
	ReasonApplied              = "POLICY_APPLIED"
	ReasonDuplicateEvent       = "DUPLICATE_EVENT"
	ReasonStaleSourceVersion   = "STALE_SOURCE_VERSION"
	ReasonUnknownPolicyKind    = "UNKNOWN_POLICY_KIND"
	ReasonUnsupportedSchema    = "UNSUPPORTED_SCHEMA_VERSION"
	ReasonChecksumMismatch     = "CHECKSUM_MISMATCH"
	ReasonMalformedDocument    = "MALFORMED_DOCUMENT"
	ReasonPricingFieldRejected = "PRICING_FIELD_REJECTED"
	ReasonReadBackUnavailable  = "READ_BACK_UNAVAILABLE"
	ReasonReadBackRefused      = "READ_BACK_REFUSED"
)

// Validate checks everything decidable without the document in hand.
//
// It deliberately does NOT check the checksum: at this point the document may
// still be a reference that has not been read back, and refusing before the
// fetch would make every referenced publication unusable.
func (p Publication) Validate() error {
	switch {
	case strings.TrimSpace(p.PolicyKind) == "":
		return fmt.Errorf("%w: policy_kind", ErrInvalidPublication)
	case strings.TrimSpace(p.PolicyVersion) == "", len(p.PolicyVersion) > 128:
		return fmt.Errorf("%w: policy_version", ErrInvalidPublication)
	case strings.TrimSpace(p.Checksum) == "":
		return fmt.Errorf("%w: checksum", ErrInvalidPublication)
	case p.EffectiveAt.IsZero():
		return fmt.Errorf("%w: effective_at", ErrInvalidPublication)
	case len(p.Document) == 0 && strings.TrimSpace(p.DocumentRef) == "":
		// A publication that carries neither the document nor a way to get it
		// describes nothing and can never be completed.
		return fmt.Errorf("%w: document or document_ref is required", ErrInvalidPublication)
	}
	if ref := strings.TrimSpace(p.DocumentRef); ref != "" {
		if !strings.HasPrefix(ref, "http://") && !strings.HasPrefix(ref, "https://") {
			return fmt.Errorf("%w: document_ref must be absolute", ErrInvalidPublication)
		}
	}
	return nil
}

// NormalizedKind upper-cases the policy kind for comparison.
func (p Publication) NormalizedKind() string {
	return strings.ToUpper(strings.TrimSpace(p.PolicyKind))
}

// Checksum computes the canonical checksum of a document.
//
// Canonicalization is a re-marshal through Go's map ordering rather than the
// raw bytes, so a publisher that reformats its JSON -- different whitespace,
// different key order -- still produces the same checksum as its consumer
// computes. Comparing raw bytes would make the check fail for reasons that
// have nothing to do with the content, and a check that cries wolf gets
// switched off.
func Checksum(document json.RawMessage) (string, error) {
	var canonical any
	if err := json.Unmarshal(document, &canonical); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPublication, err)
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPublication, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyChecksum compares a document against a declared checksum.
func VerifyChecksum(document json.RawMessage, declared string) error {
	computed, err := Checksum(document)
	if err != nil {
		return err
	}
	if !strings.EqualFold(computed, strings.TrimSpace(declared)) {
		return fmt.Errorf("%w: computed %s, declared %s", ErrChecksumMismatch, computed, declared)
	}
	return nil
}

// pricingFieldNames are keys a catalogue document must never carry.
//
// Policy describes what an asset may do, never what the work costs. The check
// is here rather than in a review checklist because a pricing field that slips
// into a capability document would be silently persisted into an immutable
// snapshot and become very hard to retract.
var pricingFieldNames = []string{
	"price", "prices", "pricing", "fare", "fares", "tariff", "tariffs",
	"rate_card", "ratecard", "rate_cards", "surge", "commission", "commissions",
	"payout", "payouts", "fee_minor", "amount_minor", "base_fare", "per_km",
}

// RejectPricingFields fails a document that carries any pricing key at any
// depth.
func RejectPricingFields(document json.RawMessage) error {
	var decoded any
	if err := json.Unmarshal(document, &decoded); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPublication, err)
	}
	if offender := findPricingKey(decoded); offender != "" {
		return fmt.Errorf("%w: document carries pricing field %q", ErrInvalidPublication, offender)
	}
	return nil
}

func findPricingKey(node any) string {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			lowered := strings.ToLower(strings.TrimSpace(key))
			for _, forbidden := range pricingFieldNames {
				if lowered == forbidden {
					return key
				}
			}
			if offender := findPricingKey(value); offender != "" {
				return offender
			}
		}
	case []any:
		for _, value := range typed {
			if offender := findPricingKey(value); offender != "" {
				return offender
			}
		}
	}
	return ""
}

// Result is what an ingestion attempt produced.
type Result struct {
	Outcome       Outcome `json:"outcome"`
	ReasonCode    string  `json:"reason_code"`
	PolicyKind    string  `json:"policy_kind,omitempty"`
	PolicyVersion string  `json:"policy_version,omitempty"`
	// Applied reports whether this attempt created a new snapshot. It is
	// false for every no-op outcome, which is what a publisher checks to tell
	// "you already had this" from "I have just given you this".
	Applied bool `json:"applied"`
}
