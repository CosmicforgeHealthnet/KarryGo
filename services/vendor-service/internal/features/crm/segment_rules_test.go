package crm

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// testRegistry builds a fully-integrated registry so field availability does
// not mask an operator or type failure under test.
func testRegistry() *SegmentFieldRegistry {
	return NewSegmentFieldRegistry(Flags{
		CustomerProjectionEnabled: true,
		ConsentIntegrationEnabled: true,
	})
}

func businessScope() SegmentScope { return SegmentScope{Type: ScopeBusiness} }

func validate(t *testing.T, rule string) (*ValidatedRule, error) {
	t.Helper()
	return ValidateRule(json.RawMessage(rule), testRegistry(), DefaultRuleLimits(), businessScope())
}

// ruleCodeOf extracts the stable machine code from a rejection.
func ruleCodeOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected a rejection, got none")
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Details == nil {
		t.Fatalf("rejection carries no details: %v", err)
	}
	code, _ := appErr.Details["code"].(string)
	return code
}

func TestRule_ValidAndOrNot(t *testing.T) {
	rule, err := validate(t, `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":5},
		{"op":"OR","children":[
			{"field":"relationship.capability_type","operator":"IN","value":["RESTAURANT","PASTRY_BAKERY"]},
			{"op":"NOT","children":[{"field":"profile.subject_type","operator":"EQ","value":"GUEST"}]}
		]}
	]}}`)
	if err != nil {
		t.Fatalf("expected a valid rule, got %v", err)
	}
	if rule.Hash == "" {
		t.Fatal("a validated rule must carry a definition hash")
	}
	wantDependencies := []string{"commercial.completed_order_count", "profile.subject_type", "relationship.capability_type"}
	if strings.Join(rule.Dependencies, ",") != strings.Join(wantDependencies, ",") {
		t.Fatalf("dependencies = %v; want %v", rule.Dependencies, wantDependencies)
	}
}

// TestRule_HashIsCanonical is what lets the hash be used as a definition
// identity: two documents meaning the same thing must not look like two
// different definitions and trigger a needless rematerialization.
func TestRule_HashIsCanonical(t *testing.T) {
	compact, err := validate(t, `{"schema_version":1,"root":{"field":"commercial.net_spend_minor","operator":"GTE","value":{"currency":"NGN","minor":10000}}}`)
	if err != nil {
		t.Fatal(err)
	}
	spaced, err := validate(t, `{
		"schema_version": 1,
		"root": { "operator": "GTE", "field": "commercial.net_spend_minor",
		          "value": { "minor": 10000, "currency": "NGN" } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if compact.Hash != spaced.Hash {
		t.Fatalf("formatting changed the definition hash: %s vs %s", compact.Hash, spaced.Hash)
	}

	different, err := validate(t, `{"schema_version":1,"root":{"field":"commercial.net_spend_minor","operator":"GTE","value":{"currency":"NGN","minor":10001}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if different.Hash == compact.Hash {
		t.Fatal("a changed value must change the definition hash")
	}
}

// TestRule_CanonicalFormRoundTrips is a load-bearing guard, not a nicety: the
// canonical document is what gets persisted, and every materialization
// re-validates the stored rule before running it. A canonical form that cannot
// be re-parsed would make every saved segment unevaluable the moment its first
// rebuild came due — and would do so silently, at rebuild time, long after the
// rule was accepted.
func TestRule_CanonicalFormRoundTrips(t *testing.T) {
	rules := []string{
		`{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"GTE","value":5}}`,
		`{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"BETWEEN","value":[2,10]}}`,
		`{"schema_version":1,"root":{"field":"commercial.net_spend_minor","operator":"GTE","value":{"currency":"NGN","minor":100000}}}`,
		`{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"WITHIN_LAST","value":{"amount":30,"unit":"DAY"}}}`,
		`{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"BEFORE","value":"2026-01-01T00:00:00Z"}}`,
		`{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"EXISTS"}}`,
		`{"schema_version":1,"root":{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"crmtag_vip"}}}`,
		`{"schema_version":1,"root":{"field":"profile.subject_type","operator":"IN","value":["CUSTOMER","GUEST"]}}`,
		`{"schema_version":1,"root":{"field":"consent.marketing.email.eligible","operator":"EQ","value":true}}`,
		`{"schema_version":1,"root":{"op":"AND","children":[
			{"field":"commercial.completed_order_count","operator":"GTE","value":5},
			{"op":"NOT","children":[{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"crmtag_x"}}]}
		]},"exclusion":{"field":"profile.status","operator":"EQ","value":"ARCHIVED"}}`,
	}
	for _, source := range rules {
		original, err := validate(t, source)
		if err != nil {
			t.Fatalf("rule %s was rejected: %v", source, err)
		}
		reparsed, err := validate(t, string(original.Canonical))
		if err != nil {
			t.Fatalf("canonical form of %s did not re-validate: %v\ncanonical=%s", source, err, original.Canonical)
		}
		if reparsed.Hash != original.Hash {
			t.Fatalf("re-parsing the canonical form changed the hash for %s", source)
		}
	}
}

func TestRule_StructuralRejections(t *testing.T) {
	cases := []struct {
		name, rule, wantCode string
	}{
		{"unsupported schema version", `{"schema_version":99,"root":{"field":"profile.status","operator":"EQ","value":"ACTIVE"}}`, CodeRuleInvalid},
		{"missing root", `{"schema_version":1}`, CodeRuleInvalid},
		{"empty group", `{"schema_version":1,"root":{"op":"AND","children":[]}}`, CodeRuleInvalid},
		{"NOT with two children", `{"schema_version":1,"root":{"op":"NOT","children":[
			{"field":"profile.status","operator":"EQ","value":"ACTIVE"},
			{"field":"profile.status","operator":"EQ","value":"ARCHIVED"}]}}`, CodeRuleInvalid},
		{"group and condition in one node", `{"schema_version":1,"root":{"op":"AND","children":[
			{"field":"profile.status","operator":"EQ","value":"ACTIVE"}],"field":"profile.status","operator":"EQ","value":"ACTIVE"}}`, CodeRuleInvalid},
		{"unknown group operator", `{"schema_version":1,"root":{"op":"XOR","children":[
			{"field":"profile.status","operator":"EQ","value":"ACTIVE"}]}}`, CodeRuleInvalid},
		{"unknown member", `{"schema_version":1,"root":{"field":"profile.status","operator":"EQ","value":"ACTIVE"},"sql":"DROP TABLE"}`, CodeRuleInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := validate(t, test.rule)
			if got := ruleCodeOf(t, err); got != test.wantCode {
				t.Fatalf("code = %s; want %s (err=%v)", got, test.wantCode, err)
			}
		})
	}
}

// TestRule_InjectionAttemptsAreRejected is the central security assertion of
// the DSL: nothing a client writes reaches SQL as anything but a bind
// parameter, and a field or operator that is not in the registry simply does
// not exist.
func TestRule_InjectionAttemptsAreRejected(t *testing.T) {
	cases := []struct {
		name, rule, wantCode string
	}{
		{
			"sql injected through the field name",
			`{"schema_version":1,"root":{"field":"commercial.net_spend_minor); DROP TABLE crm_tags; --","operator":"GTE","value":{"currency":"NGN","minor":1}}}`,
			CodeFieldNotSupported,
		},
		{
			"sql injected through the operator",
			`{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"GTE OR 1=1","value":5}}`,
			CodeOperatorNotSup,
		},
		{
			"unmapped column name",
			`{"schema_version":1,"root":{"field":"customer_ref","operator":"EQ","value":"cus_1"}}`,
			CodeFieldNotSupported,
		},
		{
			"raw sql where a rule was expected",
			`{"schema_version":1,"root":{"field":"1=1; SELECT * FROM vendor_users","operator":"EQ","value":"x"}}`,
			CodeFieldNotSupported,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := validate(t, test.rule)
			if got := ruleCodeOf(t, err); got != test.wantCode {
				t.Fatalf("code = %s; want %s (err=%v)", got, test.wantCode, err)
			}
		})
	}
}

// TestRule_ForbiddenPersonalFieldsAreNotRegistered proves the fields Section 8B
// must never expose are absent from the registry entirely, rather than being
// present and merely filtered somewhere downstream.
func TestRule_ForbiddenPersonalFieldsAreNotRegistered(t *testing.T) {
	forbidden := []string{
		"customer.email", "customer.phone", "customer.address", "customer.date_of_birth",
		"profile.customer_ref", "profile.guest_ref", "profile.external_ref",
		"privacy.subject_ref", "privacy.state", "consent.override",
		"wallet.balance", "bank.account_number", "verification.document",
		"support.evidence", "notes", "raw_event_payload",
	}
	registry := testRegistry()
	for _, key := range forbidden {
		if _, found := registry.Field(key); found {
			t.Errorf("field %q is registered; personal, authority-owned and internal fields must be unreachable from a rule", key)
		}
	}
}

// TestRule_RegistryExposesNoSQL guards the field registry's read contract: the
// listing a builder UI consumes must never carry a table or column name.
func TestRule_RegistryExposesNoSQL(t *testing.T) {
	registry := testRegistry()
	for _, field := range registry.Fields() {
		blob := field.Key + " " + field.Label + " " + field.Description
		for _, leak := range []string{
			"vendor_customer_profiles", "crm_customer_", "crm_consent_", "crm_privacy_",
			"crm_segment", "crm_tags", "vendor_branches",
			"SELECT", "JOIN", "EXISTS", "COALESCE", "NULLIF",
		} {
			if strings.Contains(blob, leak) {
				t.Errorf("field %s exposes internal SQL detail %q", field.Key, leak)
			}
		}
	}
}

func TestRule_ComputedTraitRegistryMetadataIsComplete(t *testing.T) {
	registry := testRegistry()
	for _, field := range registry.Fields() {
		if field.Version < 1 {
			t.Errorf("field %s has no version", field.Key)
		}
		if len(field.Scopes) == 0 || field.NullableBehavior == "" || field.BranchBehavior == "" || field.PrivacyBehavior == "" {
			t.Errorf("field %s has incomplete trait metadata: scopes=%v nullable=%q branch=%q privacy=%q",
				field.Key, field.Scopes, field.NullableBehavior, field.BranchBehavior, field.PrivacyBehavior)
		}
		if field.compile == nil {
			t.Errorf("field %s has no query compiler", field.Key)
		}
		if field.Label == "" || field.Description == "" || field.Type == "" || field.Source == "" || len(field.Operators) == 0 {
			t.Errorf("field %s is missing its builder contract", field.Key)
		}
	}
}

func TestRule_OperatorTypeCompatibility(t *testing.T) {
	cases := []struct {
		name, rule, wantCode string
	}{
		{"HAS_TAG on a numeric field", `{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"HAS_TAG","value":{"tag_id":"crmtag_1"}}}`, CodeOperatorNotSup},
		{"GTE on an enum field", `{"schema_version":1,"root":{"field":"profile.status","operator":"GTE","value":"ACTIVE"}}`, CodeOperatorNotSup},
		{"WITHIN_LAST on an integer field", `{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"WITHIN_LAST","value":{"amount":30,"unit":"DAY"}}}`, CodeOperatorNotSup},
		{"IN on a timestamp field", `{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"IN","value":["2026-01-01T00:00:00Z"]}}`, CodeOperatorNotSup},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := validate(t, test.rule)
			if got := ruleCodeOf(t, err); got != test.wantCode {
				t.Fatalf("code = %s; want %s (err=%v)", got, test.wantCode, err)
			}
		})
	}
}

func TestRule_ValueTypeRejections(t *testing.T) {
	cases := []struct {
		name, rule, wantCode string
	}{
		{"string where an integer belongs", `{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"GTE","value":"5"}}`, CodeValueInvalid},
		{"bare number for money", `{"schema_version":1,"root":{"field":"commercial.net_spend_minor","operator":"GTE","value":1000}}`, CodeValueInvalid},
		{"bad currency", `{"schema_version":1,"root":{"field":"commercial.net_spend_minor","operator":"GTE","value":{"currency":"ngn","minor":1000}}}`, CodeValueInvalid},
		{"negative money", `{"schema_version":1,"root":{"field":"commercial.net_spend_minor","operator":"GTE","value":{"currency":"NGN","minor":-1}}}`, CodeValueInvalid},
		{"unparsable timestamp", `{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"BEFORE","value":"yesterday"}}`, CodeValueInvalid},
		{"unknown enum value", `{"schema_version":1,"root":{"field":"profile.subject_type","operator":"EQ","value":"ROBOT"}}`, CodeValueInvalid},
		{"unknown capability", `{"schema_version":1,"root":{"field":"relationship.capability_type","operator":"IN","value":["RESTAURANT","SPACESHIP"]}}`, CodeValueInvalid},
		{"bad duration unit", `{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"WITHIN_LAST","value":{"amount":30,"unit":"FORTNIGHT"}}}`, CodeValueInvalid},
		{"zero duration", `{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"WITHIN_LAST","value":{"amount":0,"unit":"DAY"}}}`, CodeValueInvalid},
		{"empty IN list", `{"schema_version":1,"root":{"field":"profile.subject_type","operator":"IN","value":[]}}`, CodeValueInvalid},
		{"BETWEEN with one bound", `{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"BETWEEN","value":[1]}}`, CodeValueInvalid},
		{"inverted BETWEEN", `{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"BETWEEN","value":[10,1]}}`, CodeValueInvalid},
		{"value supplied to EXISTS", `{"schema_version":1,"root":{"field":"commercial.last_completed_order_at","operator":"EXISTS","value":true}}`, CodeValueInvalid},
		{"tag reference without a tag", `{"schema_version":1,"root":{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"   "}}}`, CodeValueInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := validate(t, test.rule)
			if got := ruleCodeOf(t, err); got != test.wantCode {
				t.Fatalf("code = %s; want %s (err=%v)", got, test.wantCode, err)
			}
		})
	}
}

// TestRule_ValuesContainingSQLStayData proves a value that looks like SQL is
// accepted as data where the field allows free text, and never becomes code:
// the compiled statement binds it, so the compiled SQL text cannot contain it.
func TestRule_ValuesContainingSQLStayData(t *testing.T) {
	payload := "ACTIVE' OR '1'='1"
	rule, err := validate(t, `{"schema_version":1,"root":{"field":"profile.acquisition_source_type","operator":"EQ","value":"`+payload+`"}}`)
	if err != nil {
		// Rejection is also a correct outcome — the token charset refuses it.
		if got := ruleCodeOf(t, err); got != CodeValueInvalid {
			t.Fatalf("unexpected rejection code %s: %v", got, err)
		}
		return
	}
	builder := &sqlBuilder{asOf: time.Now().UTC(), scope: businessScope()}
	compiled := CompileRule(rule, builder)
	if strings.Contains(compiled, payload) {
		t.Fatalf("the value was interpolated into SQL: %s", compiled)
	}
	found := false
	for _, arg := range builder.args {
		if text, ok := arg.(string); ok && text == payload {
			found = true
		}
	}
	if !found {
		t.Fatal("the value did not travel as a bind parameter")
	}
}

func TestRule_ComplexityLimits(t *testing.T) {
	limits := DefaultRuleLimits()

	t.Run("too deep", func(t *testing.T) {
		rule := `{"field":"profile.status","operator":"EQ","value":"ACTIVE"}`
		for depth := 0; depth < limits.MaxDepth+2; depth++ {
			rule = `{"op":"AND","children":[` + rule + `]}`
		}
		_, err := validate(t, `{"schema_version":1,"root":`+rule+`}`)
		if got := ruleCodeOf(t, err); got != CodeTooComplex {
			t.Fatalf("code = %s; want %s", got, CodeTooComplex)
		}
	})

	t.Run("too many nodes", func(t *testing.T) {
		children := make([]string, 0, limits.MaxNodes+5)
		for index := 0; index < limits.MaxNodes+5; index++ {
			children = append(children, `{"field":"profile.status","operator":"EQ","value":"ACTIVE"}`)
		}
		_, err := validate(t, `{"schema_version":1,"root":{"op":"AND","children":[`+strings.Join(children, ",")+`]}}`)
		if got := ruleCodeOf(t, err); got != CodeTooComplex {
			t.Fatalf("code = %s; want %s", got, CodeTooComplex)
		}
	})

	t.Run("too many OR branches", func(t *testing.T) {
		children := make([]string, 0, limits.MaxOrChildren+1)
		for index := 0; index < limits.MaxOrChildren+1; index++ {
			children = append(children, `{"field":"profile.status","operator":"EQ","value":"ACTIVE"}`)
		}
		_, err := validate(t, `{"schema_version":1,"root":{"op":"OR","children":[`+strings.Join(children, ",")+`]}}`)
		if got := ruleCodeOf(t, err); got != CodeTooComplex {
			t.Fatalf("code = %s; want %s", got, CodeTooComplex)
		}
	})

	t.Run("too many IN values", func(t *testing.T) {
		values := make([]string, 0, limits.MaxInValues+1)
		for index := 0; index < limits.MaxInValues+1; index++ {
			values = append(values, `"CUSTOMER"`)
		}
		_, err := validate(t, `{"schema_version":1,"root":{"field":"profile.subject_type","operator":"IN","value":[`+strings.Join(values, ",")+`]}}`)
		if got := ruleCodeOf(t, err); got != CodeTooComplex {
			t.Fatalf("code = %s; want %s", got, CodeTooComplex)
		}
	})

	t.Run("too many nested NOTs", func(t *testing.T) {
		rule := `{"field":"profile.status","operator":"EQ","value":"ACTIVE"}`
		for depth := 0; depth < limits.MaxNotDepth+1; depth++ {
			rule = `{"op":"NOT","children":[` + rule + `]}`
		}
		_, err := validate(t, `{"schema_version":1,"root":`+rule+`}`)
		if got := ruleCodeOf(t, err); got != CodeTooComplex {
			t.Fatalf("code = %s; want %s", got, CodeTooComplex)
		}
	})

	t.Run("too many tag references", func(t *testing.T) {
		children := make([]string, 0, limits.MaxTagReferences+1)
		for index := 0; index < limits.MaxTagReferences+1; index++ {
			children = append(children, `{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"crmtag_`+itoa(index)+`"}}`)
		}
		_, err := validate(t, `{"schema_version":1,"root":{"op":"AND","children":[`+strings.Join(children, ",")+`]}}`)
		if got := ruleCodeOf(t, err); got != CodeTooComplex {
			t.Fatalf("code = %s; want %s", got, CodeTooComplex)
		}
	})
}

// TestRule_MaximumValidComplexityIsAccepted pins the other side of the limits:
// a rule exactly at the ceiling must still work, so the guard rejects abuse
// rather than legitimate use.
func TestRule_MaximumValidComplexityIsAccepted(t *testing.T) {
	limits := DefaultRuleLimits()
	children := make([]string, 0, limits.MaxOrChildren)
	for index := 0; index < limits.MaxOrChildren; index++ {
		children = append(children, `{"field":"profile.status","operator":"EQ","value":"ACTIVE"}`)
	}
	rule := `{"schema_version":1,"root":{"op":"OR","children":[` + strings.Join(children, ",") + `]}}`
	if _, err := validate(t, rule); err != nil {
		t.Fatalf("a rule at the documented ceiling was rejected: %v", err)
	}
}

// TestRule_ExclusionArmIsValidatedToo makes sure the exclusion arm is not a
// back door around the registry and the limits.
func TestRule_ExclusionArmIsValidatedToo(t *testing.T) {
	_, err := validate(t, `{"schema_version":1,
		"root":{"field":"profile.status","operator":"EQ","value":"ACTIVE"},
		"exclusion":{"field":"customer.email","operator":"EQ","value":"a@b.test"}}`)
	if got := ruleCodeOf(t, err); got != CodeFieldNotSupported {
		t.Fatalf("code = %s; want %s", got, CodeFieldNotSupported)
	}

	rule, err := validate(t, `{"schema_version":1,
		"root":{"field":"commercial.completed_order_count","operator":"GTE","value":1},
		"exclusion":{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"crmtag_do_not_contact"}}}`)
	if err != nil {
		t.Fatalf("valid exclusion rejected: %v", err)
	}
	if !segmentDependsOn(rule.Dependencies, "crm.tags") {
		t.Fatal("the exclusion arm's fields must appear in the dependency set")
	}
}

// TestRule_BranchOnlyFieldsRespectScope keeps a business-scope rule from
// reaching a field that only means something inside one branch.
func TestRule_ConsentFieldsRegisteredPerChannel(t *testing.T) {
	registry := testRegistry()
	for _, channel := range MarketingChannels {
		key := "consent.marketing." + strings.ToLower(string(channel))
		for _, suffix := range []string{".decision", ".eligible"} {
			if _, found := registry.Field(key + suffix); !found {
				t.Errorf("consent field %s is not registered", key+suffix)
			}
		}
	}
	if _, found := registry.Field("consent.marketing.email.override"); found {
		t.Fatal("a consent override field exists; eligibility must not be assertable from a rule")
	}
}

// TestRule_ConsentUnavailableWhenIntegrationDisabled proves a disabled consent
// authority makes the field unanswerable rather than making everyone eligible.
func TestRule_ConsentUnavailableWhenIntegrationDisabled(t *testing.T) {
	registry := NewSegmentFieldRegistry(Flags{CustomerProjectionEnabled: true})
	rule, err := ValidateRule(json.RawMessage(
		`{"schema_version":1,"root":{"field":"consent.marketing.email.eligible","operator":"EQ","value":true}}`),
		registry, DefaultRuleLimits(), businessScope())
	if err != nil {
		t.Fatal(err)
	}
	if len(rule.UnavailableFields) != 1 || rule.UnavailableFields[0] != "consent.marketing.email.eligible" {
		t.Fatalf("unavailable fields = %v; want the consent field", rule.UnavailableFields)
	}
	builder := &sqlBuilder{asOf: time.Now().UTC(), scope: businessScope(), availability: registry.availability}
	if compiled := CompileRule(rule, builder); !strings.Contains(compiled, "NULL") {
		t.Fatalf("a disabled consent integration must compile to an undecidable condition, got %s", compiled)
	}
}

func TestRule_ExplanationIsDeterministic(t *testing.T) {
	rule, err := validate(t, `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":5},
		{"field":"commercial.last_completed_order_at","operator":"WITHIN_LAST","value":{"amount":30,"unit":"DAY"}}
	]}}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "Completed order count is at least 5 AND Last completed order is within the last 30 days"
	if got := DescribeRule(rule); got != want {
		t.Fatalf("explanation = %q; want %q", got, want)
	}
	if DescribeRule(rule) != want {
		t.Fatal("the explanation is not deterministic")
	}
}

func TestRule_NegatedGuardIsFlagged(t *testing.T) {
	rule, err := validate(t, `{"schema_version":1,"root":{"op":"NOT","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":5}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !rule.NegatedAvailabilityGuarded {
		t.Fatal("a NOT over an availability-guarded field must be flagged so preview can warn")
	}

	plain, err := validate(t, `{"schema_version":1,"root":{"op":"NOT","children":[
		{"field":"profile.subject_type","operator":"EQ","value":"GUEST"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if plain.NegatedAvailabilityGuarded {
		t.Fatal("a NOT over an always-answerable field must not raise the warning")
	}
}

// TestProfileColumnListsStayInSync guards a bug class that no integration test
// would catch until a future Section 8A column was added: the member query joins
// another table, so it uses a qualified copy of the shared profile column list.
// If the two drift, the member scanner silently reads the wrong columns.
func TestProfileColumnListsStayInSync(t *testing.T) {
	strip := func(list string) []string {
		var out []string
		for _, column := range strings.Split(list, ",") {
			column = strings.TrimSpace(strings.ReplaceAll(column, "\n", ""))
			out = append(out, strings.TrimPrefix(column, "p."))
		}
		return out
	}
	shared := strip(profileCols)
	qualified := strip(qualifiedProfileCols)
	if len(shared) != len(qualified) {
		t.Fatalf("profileCols has %d columns, qualifiedProfileCols has %d", len(shared), len(qualified))
	}
	for index := range shared {
		if shared[index] != qualified[index] {
			t.Fatalf("column %d differs: %q vs %q", index, shared[index], qualified[index])
		}
	}
	for _, column := range strings.Split(qualifiedProfileCols, ",") {
		if !strings.Contains(column, "p.") {
			t.Fatalf("qualifiedProfileCols entry %q is not table-qualified", strings.TrimSpace(column))
		}
	}
}
