package engine

import (
	"bytes"
	"encoding/json"
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/platform/canonical"
)

// Policy kinds. Each is a separate rule book with its own typed body.
//
// These are a CLOSED set, not a general rule language. A generic language would
// let two policies interact in ways nobody can test, and would make an approval
// screen impossible to read. A typed body means an approver can be shown exactly
// what will change.
const (
	KindTaxiTariff        = "taxi.tariff"
	KindDispatchTariff    = "dispatch.tariff"
	KindHaulageTariff     = "haulage.tariff"
	KindCommission        = "commission"
	KindQuoteLifecycle    = "quote.lifecycle"
	KindRegulatoryCharges = "regulatory.charges"
)

// Body is any decoded policy body.
type Body interface {
	Kind() string
	Validate() error
}

// Tariff is a body that can price a request.
type Tariff interface {
	Body
	// Evaluate returns the components, their exact sum, and any flags.
	Evaluate(Request) ([]Component, int64, []string, error)
}

// decoder builds a body from JSON.
type decoder func([]byte) (Body, error)

// registry maps (kind, schema version) to a decoder. A version's behaviour is
// frozen once it exists: changing what an evaluator does for an existing schema
// version would silently restate every quote priced under it, so a behaviour
// change requires a NEW schema version.
var registry = map[string]map[int]decoder{
	KindTaxiTariff:        {1: decodeInto[TaxiTariffV1]},
	KindDispatchTariff:    {1: decodeInto[DispatchTariffV1]},
	KindHaulageTariff:     {1: decodeInto[HaulageTariffV1]},
	KindCommission:        {1: decodeInto[CommissionV1]},
	KindQuoteLifecycle:    {1: decodeInto[QuoteLifecycleV1]},
	KindRegulatoryCharges: {1: decodeInto[RegulatoryChargesV1]},
	KindFuelAdjustment:    {1: decodeInto[FuelAdjustmentV1]},
}

// KnownKinds lists every supported kind.
func KnownKinds() []string {
	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	return kinds
}

// SupportedSchemaVersions lists the schema versions known for a kind.
func SupportedSchemaVersions(kind string) []int {
	versions := make([]int, 0, len(registry[kind]))
	for version := range registry[kind] {
		versions = append(versions, version)
	}
	return versions
}

// TariffKindFor returns the tariff kind that prices a scope.
func TariffKindFor(scope Scope) (string, error) {
	switch scope {
	case ScopeTaxi:
		return KindTaxiTariff, nil
	case ScopeDispatch:
		return KindDispatchTariff, nil
	case ScopeHaulage:
		return KindHaulageTariff, nil
	default:
		return "", fmt.Errorf("engine: no tariff kind for scope %q", scope)
	}
}

// Decode turns a stored body into its typed form, rejecting unknown fields.
//
// Strictness is the point. A body carrying "per_km_kobo_v2" because someone
// misremembered the field name must fail loudly at approval time, not be ignored
// and quietly priced at zero.
func Decode(kind string, schemaVersion int, body []byte) (Body, error) {
	versions, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("engine: unknown policy kind %q", kind)
	}
	decode, ok := versions[schemaVersion]
	if !ok {
		return nil, fmt.Errorf("engine: policy kind %q has no schema version %d", kind, schemaVersion)
	}
	decoded, err := decode(body)
	if err != nil {
		return nil, err
	}
	if err := decoded.Validate(); err != nil {
		return nil, err
	}
	return decoded, nil
}

// DecodeTariff decodes a body and asserts it can price.
func DecodeTariff(kind string, schemaVersion int, body []byte) (Tariff, error) {
	decoded, err := Decode(kind, schemaVersion, body)
	if err != nil {
		return nil, err
	}
	tariff, ok := decoded.(Tariff)
	if !ok {
		return nil, fmt.Errorf("engine: policy kind %q is not a tariff", kind)
	}
	return tariff, nil
}

// CanonicalBody re-encodes a validated body in canonical form and returns it with
// its hash.
//
// Storing the canonical re-encoding rather than the operator's original JSON is
// what makes body_hash stable: the same policy submitted with different key order
// or whitespace must seal identically, or an approval would be invalidated by
// reformatting.
func CanonicalBody(kind string, schemaVersion int, body []byte) ([]byte, string, error) {
	decoded, err := Decode(kind, schemaVersion, body)
	if err != nil {
		return nil, "", err
	}
	encoded, err := canonical.Marshal(decoded)
	if err != nil {
		return nil, "", err
	}
	hash, err := canonical.Hash(decoded)
	if err != nil {
		return nil, "", err
	}
	return encoded, hash, nil
}

func decodeInto[T Body](raw []byte) (Body, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("engine: decode %T: %w", value, err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("engine: decode %T: trailing content after the policy body", value)
	}
	return value, nil
}
