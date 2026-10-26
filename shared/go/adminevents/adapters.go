package adminevents

import (
	"encoding/json"
	"sort"
	"time"
)

// Data classifications (Section 30). RESTRICTED_PII, CREDENTIAL and
// HEALTH_SPECIAL_CATEGORY are never exportable: no adapter may carry them.
const (
	ClassPublic          = "PUBLIC"
	ClassInternal        = "INTERNAL"
	ClassRestrictedPII   = "RESTRICTED_PII"
	ClassFinancial       = "FINANCIAL_SENSITIVE"
	ClassHealth          = "HEALTH_SPECIAL_CATEGORY_IF_INTEGRATED"
	ClassSafetyEvidence  = "SAFETY_EVIDENCE"
	ClassCredential      = "CREDENTIAL"
	ClassOperationsMinim = "OPERATIONS_MINIMIZED"
)

func Exportable(class string) bool {
	switch class {
	case ClassPublic, ClassInternal, ClassOperationsMinim, ClassFinancial, ClassSafetyEvidence:
		return true
	}
	return false
}

// Field kinds are closed shapes. There is deliberately no free-text kind.
type FieldKind int

const (
	FieldOpaque FieldKind = iota + 1
	FieldOpaqueNullable
	FieldCount
	FieldTime
	FieldTimeNullable
	FieldBool
	FieldEnum
)

type Field struct {
	Kind     FieldKind
	Enum     []string
	Required bool
}

// Adapter maps one owner event type onto one minimized export type.
type Adapter struct {
	SourceService, SourceType string
	ExportType, EntityType    string
	Classification            string
	// MarketRequired rejects events that do not carry a canonical market.
	MarketRequired bool
	Fields         map[string]Field
}

var registry = map[string]Adapter{}

func key(source, typ string) string { return source + "\x00" + typ }

// Register is called from package init only (compiled allowlist).
func Register(a Adapter) {
	if !Opaque(a.SourceService) || !eventType.MatchString(a.SourceType) || !eventType.MatchString(a.ExportType) || !Opaque(a.EntityType) || !Exportable(a.Classification) || len(a.Fields) > 32 {
		panic("adminevents: invalid adapter registration")
	}
	if _, exists := registry[key(a.SourceService, a.SourceType)]; exists {
		panic("adminevents: duplicate adapter")
	}
	for name, f := range a.Fields {
		if !Opaque(name) || f.Kind < FieldOpaque || f.Kind > FieldEnum || (f.Kind == FieldEnum) != (len(f.Enum) > 0) {
			panic("adminevents: invalid adapter field")
		}
	}
	registry[key(a.SourceService, a.SourceType)] = a
}

// Lookup returns the compiled adapter for an owner/type pair.
func Lookup(source, typ string) (Adapter, bool) {
	a, ok := registry[key(source, typ)]
	return a, ok
}

// Adapters lists every registration in stable order (for contracts/tests).
func Adapters() []Adapter {
	out := make([]Adapter, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SourceService+out[i].SourceType < out[j].SourceService+out[j].SourceType
	})
	return out
}

// Minimize validates an event against its adapter and returns the exact
// allowlisted payload. Unknown or malformed fields fail (quarantine), so an
// owner schema change can never silently push new data to HQ.
func (a Adapter) Minimize(ev SourceEvent) (map[string]any, error) {
	if ev.SourceService != a.SourceService || ev.EventType != a.SourceType || ev.EntityType != a.EntityType {
		return nil, ErrMismatch
	}
	if a.MarketRequired && ev.Scope.Market == "" {
		return nil, ErrInvalid
	}
	out := map[string]any{}
	for name := range ev.Payload {
		if _, ok := a.Fields[name]; !ok {
			return nil, ErrUnknown
		}
	}
	for name, f := range a.Fields {
		raw, present := ev.Payload[name]
		if !present || string(raw) == "null" {
			if f.Required || !nullable(f.Kind) && present {
				return nil, ErrInvalid
			}
			out[name] = nil
			continue
		}
		v, ok := decodeField(f, raw)
		if !ok {
			return nil, ErrInvalid
		}
		out[name] = v
	}
	return out, nil
}

func nullable(k FieldKind) bool {
	return k == FieldOpaqueNullable || k == FieldTimeNullable || k == FieldCount || k == FieldBool || k == FieldEnum || k == FieldOpaque || k == FieldTime
}

func decodeField(f Field, raw json.RawMessage) (any, bool) {
	switch f.Kind {
	case FieldOpaque, FieldOpaqueNullable:
		var s string
		if json.Unmarshal(raw, &s) != nil || !Opaque(s) {
			return nil, false
		}
		return s, true
	case FieldEnum:
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, false
		}
		for _, e := range f.Enum {
			if e == s {
				return s, true
			}
		}
		return nil, false
	case FieldCount:
		var n int64
		if json.Unmarshal(raw, &n) != nil || n < 0 {
			return nil, false
		}
		return n, true
	case FieldBool:
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return nil, false
		}
		return b, true
	case FieldTime, FieldTimeNullable:
		var t time.Time
		if json.Unmarshal(raw, &t) != nil || t.IsZero() {
			return nil, false
		}
		return t.UTC(), true
	}
	return nil, false
}
