package logistics_test

// Cross-feature consistency proofs.
//
// These live in an external test package because they deliberately import the
// Vendor features the logistics package must never import: the point is to
// prove the feature-local vocabularies still agree with the canonical one
// without creating the dependency cycle that would let them drift.

import (
	"reflect"
	"strings"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

// TestEveryVendorAuthorityEnumSpellsTheSameAuthorities is the guard that VND-002
// asks for.
//
// Vendor grew four authority enums in four features plus the shared mission
// contract's own. They agree today. Nothing but this test keeps them agreeing:
// the first person to add a fifth authority, or to spell one differently, gets
// a failure here instead of a production mission with no executor.
func TestEveryVendorAuthorityEnumSpellsTheSameAuthorities(t *testing.T) {
	groups := map[string][]string{
		"orders.LogisticsService": {
			string(orders.LogisticsDispatch), string(orders.LogisticsTaxi), string(orders.LogisticsHauling),
		},
		"fulfilmentgroup.Authority": {
			string(fulfilmentgroup.AuthorityDispatch), string(fulfilmentgroup.AuthorityTaxi),
			string(fulfilmentgroup.AuthorityHauling), string(fulfilmentgroup.AuthorityVendor),
		},
		"fulfilment.Service": {
			string(fulfilment.ServiceDispatch), string(fulfilment.ServiceTaxi),
			string(fulfilment.ServiceHauling), string(fulfilment.ServiceVendorOwn),
		},
		"commercemission.Service": {
			string(commercemission.ServiceDispatch), string(commercemission.ServiceTaxi),
			string(commercemission.ServiceHauling),
		},
	}
	for name, values := range groups {
		for _, value := range values {
			if _, err := logistics.Parse(value); err != nil {
				t.Errorf("%s value %q does not resolve to a canonical authority: %v", name, value, err)
			}
		}
	}

	// The three logistics values of the custody-source enum must resolve too.
	// VENDOR and PLATFORM are reporters rather than execution authorities and
	// are deliberately excluded: custody says who observed a fact, which is a
	// wider question than who is executing.
	for _, value := range []string{
		string(fulfilmentgroup.SourceDispatch), string(fulfilmentgroup.SourceTaxi), string(fulfilmentgroup.SourceHauling),
	} {
		if _, err := logistics.Parse(value); err != nil {
			t.Errorf("custody source %q does not resolve to a canonical authority: %v", value, err)
		}
	}
	if !logistics.Equal(string(fulfilmentgroup.SourceVendor), string(fulfilment.ServiceVendorOwn)) {
		t.Error("the custody VENDOR source and the VENDOR_OWN_DELIVERY service must name the same party")
	}
}

// TestTheThreePlatformAuthoritiesAgreeAcrossEveryEnum pins the actual values
// rather than only their parseability, so a rename in one feature cannot pass
// by resolving to a different authority.
func TestTheThreePlatformAuthoritiesAgreeAcrossEveryEnum(t *testing.T) {
	cases := []struct {
		canonical logistics.Authority
		spellings []string
	}{
		{logistics.Dispatch, []string{
			string(orders.LogisticsDispatch), string(fulfilmentgroup.AuthorityDispatch),
			string(fulfilment.ServiceDispatch), string(commercemission.ServiceDispatch),
			string(fulfilmentgroup.SourceDispatch),
		}},
		{logistics.Taxi, []string{
			string(orders.LogisticsTaxi), string(fulfilmentgroup.AuthorityTaxi),
			string(fulfilment.ServiceTaxi), string(commercemission.ServiceTaxi),
			string(fulfilmentgroup.SourceTaxi), logistics.TaxiGoodsAlias,
		}},
		{logistics.Hauling, []string{
			string(orders.LogisticsHauling), string(fulfilmentgroup.AuthorityHauling),
			string(fulfilment.ServiceHauling), string(commercemission.ServiceHauling),
			string(fulfilmentgroup.SourceHauling),
		}},
	}
	for _, testCase := range cases {
		for _, spelling := range testCase.spellings {
			got, err := logistics.Parse(spelling)
			if err != nil {
				t.Fatalf("%q: %v", spelling, err)
			}
			if got != testCase.canonical {
				t.Fatalf("%q resolves to %q, want %q", spelling, got, testCase.canonical)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// VND-003 — Vendor never selects a provider
// -----------------------------------------------------------------------------

// TestNoVendorMissionRequestCarriesProviderSelectionAuthority is the absolute
// rule made mechanical.
//
// Vendor may say "this order uses Hauling". It may not say "use provider 123".
// Provider selection belongs to the execution service, to Fleet corporate
// assignment, or to the marketplace — and a field is all it would take to
// quietly move that decision here, because a request type with a provider id
// on it will eventually have one set.
//
// So the request types Vendor sends are walked recursively and any field whose
// name identifies a provider, driver, rider, vehicle, fleet member or assignee
// fails the test. Only an explicitly authorized future contract should change
// this, and changing it should be as visible as editing this test.
func TestNoVendorMissionRequestCarriesProviderSelectionAuthority(t *testing.T) {
	banned := map[string]bool{}
	for _, field := range logistics.ProviderSelectionFields() {
		banned[normalizeFieldName(field)] = true
	}

	subjects := map[string]reflect.Type{
		"logistics.MissionIntent":               reflect.TypeOf(logistics.MissionIntent{}),
		"orders.MissionRequest":                 reflect.TypeOf(orders.MissionRequest{}),
		"commercemission.Request":               reflect.TypeOf(commercemission.Request{}),
		"dispatchcommerce.MissionCreateRequest": reflect.TypeOf(dispatchcommerce.MissionCreateRequest{}),
		"dispatchcommerce.QuoteRequest":         reflect.TypeOf(dispatchcommerce.QuoteRequest{}),
	}
	for name, subject := range subjects {
		for _, path := range walkFieldNames(subject, name, map[reflect.Type]bool{}) {
			leaf := path[strings.LastIndex(path, ".")+1:]
			if banned[normalizeFieldName(leaf)] {
				t.Errorf("%s carries a provider-selection field: Vendor names the execution authority, never the provider (VND-003)", path)
			}
		}
	}
}

// TestTheProviderSelectionBanIsNotVacuous proves the walker would actually
// catch a violation. Without it, a walker that silently visited nothing would
// make the rule above pass forever while enforcing nothing.
func TestTheProviderSelectionBanIsNotVacuous(t *testing.T) {
	type nested struct {
		ProviderID string
	}
	type offending struct {
		MissionRef string
		Detail     nested
	}
	found := false
	for _, path := range walkFieldNames(reflect.TypeOf(offending{}), "offending", map[reflect.Type]bool{}) {
		if strings.HasSuffix(path, ".ProviderID") {
			found = true
		}
	}
	if !found {
		t.Fatal("the field walker does not reach nested fields, so the provider-selection ban would enforce nothing")
	}
	if len(logistics.ProviderSelectionFields()) == 0 {
		t.Fatal("the banned-field list is empty, so the ban would enforce nothing")
	}
}

func normalizeFieldName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", ""))
}

// walkFieldNames returns a dotted path for every field reachable from a struct
// type, following pointers, slices and maps, and guarding against recursive
// types.
func walkFieldNames(subject reflect.Type, prefix string, seen map[reflect.Type]bool) []string {
	for subject != nil && (subject.Kind() == reflect.Ptr || subject.Kind() == reflect.Slice || subject.Kind() == reflect.Array) {
		subject = subject.Elem()
	}
	if subject == nil || subject.Kind() != reflect.Struct || seen[subject] {
		return nil
	}
	seen[subject] = true
	defer delete(seen, subject)

	paths := make([]string, 0, subject.NumField())
	for i := 0; i < subject.NumField(); i++ {
		field := subject.Field(i)
		path := prefix + "." + field.Name
		paths = append(paths, path)
		paths = append(paths, walkFieldNames(field.Type, path, seen)...)
	}
	return paths
}
