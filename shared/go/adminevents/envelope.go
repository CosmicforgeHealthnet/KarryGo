// Package adminevents is the Secure Admin event contract (Section 22).
//
// An owner service appends a SourceEvent to its own transactional outbox in
// the same database transaction as the business change it describes. A relay
// delivers the stored bytes, HMAC v2 signed under the owner's own service
// identity, to the private Admin Bridge ingress. The Bridge validates the
// caller/source binding and the event against a compiled, allowlisted adapter,
// deduplicates on (source_service,event_id), fences on entity_version, keeps a
// minimized projection and republishes a minimized ExportEvent for HQ.
//
// Nothing here is a second domain truth: receipt of an event never proves a
// financial, eligibility or custody fact; owner read-back remains required.
package adminevents

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"

	"github.com/google/uuid"
)

const (
	// SourceSchema is the owner -> Bridge envelope version.
	SourceSchema = "karryam.owner-admin-event.v1"
	// ExportSchema is the Bridge -> HQ envelope version (Section 22.3).
	ExportSchema = "cosmicforge.product-admin-event.v1"
	Product      = "karryam"
	// MaxEventBytes bounds one stored/delivered event.
	MaxEventBytes = 16 << 10
)

var (
	ErrInvalid  = errors.New("invalid administrative event")
	ErrUnknown  = errors.New("unregistered administrative event schema")
	ErrMismatch = errors.New("administrative event source binding mismatch")
)

var opaque = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$`)
var eventType = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,6}\.v[1-9][0-9]?$`)

// Opaque reports whether v is a bounded opaque token (no spaces, no free text).
func Opaque(v string) bool { return opaque.MatchString(v) }

// Scope carries authorization-relevant geography/tenancy. Empty means the
// owner does not canonically record that dimension; it is never inferred.
type Scope struct {
	Market       string `json:"market"`
	Zone         string `json:"zone"`
	Organization string `json:"organization"`
	Tenant       string `json:"tenant"`
}

func (s Scope) valid() bool {
	for _, v := range []string{s.Market, s.Zone, s.Organization, s.Tenant} {
		if v != "" && !Opaque(v) {
			return false
		}
	}
	return true
}

// SourceEvent is what an owner stores and delivers. Payload keys must match the
// compiled adapter exactly; unknown keys quarantine the event rather than leak.
type SourceEvent struct {
	Schema        string                     `json:"schema"`
	EventID       string                     `json:"event_id"`
	EventType     string                     `json:"event_type"`
	SourceService string                     `json:"source_service"`
	Product       string                     `json:"product"`
	EntityType    string                     `json:"entity_type"`
	EntityRef     string                     `json:"entity_ref"`
	EntityVersion int64                      `json:"entity_version"`
	Deleted       bool                       `json:"deleted"`
	OccurredAt    time.Time                  `json:"occurred_at"`
	EmittedAt     time.Time                  `json:"emitted_at"`
	Scope         Scope                      `json:"scope"`
	CorrelationID string                     `json:"correlation_id"`
	Payload       map[string]json.RawMessage `json:"payload"`
}

// ExportEvent is the minimized HQ envelope (Section 22.3).
type ExportEvent struct {
	Schema         string         `json:"schema"`
	EventID        string         `json:"event_id"`
	Sequence       int64          `json:"sequence"`
	EventType      string         `json:"event_type"`
	SourceService  string         `json:"source_service"`
	Product        string         `json:"product"`
	EntityType     string         `json:"entity_type"`
	EntityRef      string         `json:"entity_ref"`
	EntityVersion  int64          `json:"entity_version"`
	Tombstone      bool           `json:"tombstone"`
	OccurredAt     time.Time      `json:"occurred_at"`
	EmittedAt      time.Time      `json:"emitted_at"`
	Market         string         `json:"market"`
	Scope          ExportScope    `json:"scope"`
	Classification string         `json:"classification"`
	CorrelationID  string         `json:"correlation_id"`
	Payload        map[string]any `json:"payload"`
}
type ExportScope struct {
	Zone         string `json:"zone"`
	Organization string `json:"organization"`
	Tenant       string `json:"tenant"`
}

var sourceFields = []string{"schema", "event_id", "event_type", "source_service", "product", "entity_type", "entity_ref", "entity_version", "deleted", "occurred_at", "emitted_at", "scope", "correlation_id", "payload"}

// Decode strictly parses one source event: exact keys, no duplicates, no
// trailing data, bounded size. Field semantics are validated by Validate.
func Decode(raw []byte) (SourceEvent, error) {
	var ev SourceEvent
	if len(raw) == 0 || len(raw) > MaxEventBytes {
		return ev, ErrInvalid
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil || len(keys) != len(sourceFields) {
		return ev, ErrInvalid
	}
	for _, k := range sourceFields {
		if _, ok := keys[k]; !ok {
			return ev, ErrInvalid
		}
	}
	if duplicateKeys(raw) {
		return ev, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return ev, ErrInvalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return ev, ErrInvalid
	}
	return ev, ev.Validate()
}

func duplicateKeys(raw []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return true
	}
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return true
		}
		k, _ := t.(string)
		if seen[k] {
			return true
		}
		seen[k] = true
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return true
		}
	}
	return false
}

// Validate checks envelope invariants independent of any adapter.
func (e SourceEvent) Validate() error {
	id, err := uuid.Parse(e.EventID)
	if e.Schema != SourceSchema || err != nil || id == uuid.Nil || id.String() != e.EventID || !eventType.MatchString(e.EventType) || !Opaque(e.SourceService) || e.Product != Product || !Opaque(e.EntityType) || !Opaque(e.EntityRef) || e.EntityVersion < 1 || e.OccurredAt.IsZero() || e.EmittedAt.IsZero() || e.EmittedAt.Before(e.OccurredAt.Add(-time.Minute)) || !e.Scope.valid() || (e.CorrelationID != "" && !Opaque(e.CorrelationID)) || len(e.Payload) > 32 {
		return ErrInvalid
	}
	return nil
}

// NewSourceEvent builds a validated event. Payload values are marshalled now so
// the stored bytes are exactly what is later delivered and verified.
func NewSourceEvent(source, typ, entityType, entityRef string, version int64, occurred time.Time, scope Scope, correlation string, payload map[string]any, deleted bool) (SourceEvent, error) {
	raw := map[string]json.RawMessage{}
	for k, v := range payload {
		b, err := json.Marshal(v)
		if err != nil {
			return SourceEvent{}, ErrInvalid
		}
		raw[k] = b
	}
	ev := SourceEvent{Schema: SourceSchema, EventID: uuid.NewString(), EventType: typ, SourceService: source, Product: Product, EntityType: entityType, EntityRef: entityRef, EntityVersion: version, Deleted: deleted, OccurredAt: occurred.UTC(), EmittedAt: time.Now().UTC(), Scope: scope, CorrelationID: correlation, Payload: raw}
	return ev, ev.Validate()
}
