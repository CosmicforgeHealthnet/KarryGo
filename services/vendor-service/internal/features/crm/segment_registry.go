package crm

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Field sources, reported to a builder UI so fields can be grouped.
const (
	SourceProfile      = "PROFILE"
	SourceCommercial   = "COMMERCIAL"
	SourceRelationship = "RELATIONSHIP"
	SourceTags         = "TAGS"
	SourceConsent      = "CONSENT"
	SourceIntelligence = "INTELLIGENCE"
	SourceCompletion   = "COMPLETION"
)

// availability is the deployment state the registry consults to decide whether
// a field can currently answer at all. Reporting a field as unavailable is very
// different from reporting it as false, and Section 8B keeps that distinction:
// a rule over an unintegrated projection matches nobody and says so.
type availability struct {
	CommercialProjection bool
	Consent              bool
	Intelligence         bool
}

// SegmentField is one field a vendor rule may reference.
//
// The registry is the entire attack surface between vendor input and SQL: a
// condition can only reach the database through a field that exists here, and a
// field's SQL is written in this file by hand. There is no path from a client
// string to an identifier, and none from a client value to anything but a bind
// parameter.
type SegmentField struct {
	Key         string
	Label       string
	Description string
	Type        FieldType
	Source      string
	// Version makes a field's query/explanation contract explicitly
	// evolvable without conflating it with the whole DSL schema version.
	Version int
	// Scopes documents where the field may be used. The initial registry is
	// available to BUSINESS and BRANCH segments; BranchBehavior explains how
	// the value is narrowed for the latter.
	Scopes           []string
	NullableBehavior string
	BranchBehavior   string
	PrivacyBehavior  string

	// Operators is the allowlist for this field. An operator absent from it is
	// rejected before any value is parsed.
	Operators []string
	// EnumValues, when non-empty, is the complete legal value set.
	EnumValues []string
	// BranchOnly marks a field that is only meaningful inside a branch-scoped
	// segment.
	BranchOnly bool

	// AvailabilityGuard names the deployment capability this field needs
	// ("commercial" or "consent"); empty means always answerable.
	AvailabilityGuard string

	// compile renders the field's SQL boolean expression. It is a Go function,
	// never a template filled from client input.
	compile func(condition *Condition, builder *sqlBuilder) string
}

// SupportsOperator reports whether an operator is legal for this field.
func (f SegmentField) SupportsOperator(operator string) bool {
	for _, allowed := range f.Operators {
		if allowed == operator {
			return true
		}
	}
	return false
}

// safeToken bounds free-form reference values (branch ids, acquisition source
// types). They still travel as bind parameters; the pattern exists so an
// obviously malformed value is rejected early with a clear message rather than
// silently matching nothing.
var safeToken = regexp.MustCompile(`^[A-Za-z0-9_.:@-]{1,64}$`)

// AllowsEnumValue reports whether a value is legal for this field.
func (f SegmentField) AllowsEnumValue(value string) bool {
	if len(f.EnumValues) == 0 {
		return safeToken.MatchString(value)
	}
	for _, allowed := range f.EnumValues {
		if allowed == value {
			return true
		}
	}
	return false
}

// Available reports whether the field's backing projection is integrated.
func (f SegmentField) Available(a availability) bool {
	switch f.AvailabilityGuard {
	case "commercial":
		return a.CommercialProjection
	case "consent":
		return a.Consent
	case "intelligence":
		return a.Intelligence
	}
	return true
}

// SegmentFieldRegistry is the closed set of referenceable fields.
type SegmentFieldRegistry struct {
	fields       map[string]*SegmentField
	order        []string
	availability availability
}

// NewSegmentFieldRegistry builds the registry for one deployment.
//
// Only fields backed by data Section 8A actually owns are registered. Catalogue,
// loyalty, interaction, task, campaign and score fields are deliberately absent
// until the packages that own them exist — offering a field that silently
// matches nobody would be worse than not offering it.
func NewSegmentFieldRegistry(flags Flags) *SegmentFieldRegistry {
	registry := &SegmentFieldRegistry{
		fields: map[string]*SegmentField{},
		availability: availability{
			CommercialProjection: flags.CustomerProjectionEnabled,
			Consent:              flags.ConsentIntegrationEnabled,
			Intelligence:         flags.IntelligenceEnabled,
		},
	}
	for _, field := range profileFields() {
		registry.add(field)
	}
	for _, field := range commercialFields() {
		registry.add(field)
	}
	for _, field := range relationshipFields() {
		registry.add(field)
	}
	registry.add(tagField())
	for _, field := range consentFields() {
		registry.add(field)
	}
	// Section 8H fields are registered only when this deployment actually
	// computes scores. Offering a field that could never answer would be worse
	// than not offering it: a rule referencing it would silently match nobody.
	if flags.IntelligenceEnabled {
		for _, field := range intelligenceFields() {
			registry.add(field)
		}
	}
	for _, field := range completionFields() {
		registry.add(field)
	}
	return registry
}

func completionFields() []*SegmentField {
	guard := "commercial"
	fields := []*SegmentField{{Key: "commercial.observed_order_count", Label: "Observed order count", Description: "Number of observed orders, including orders that are not completed.", Type: FieldInteger, Source: SourceCommercial, AvailabilityGuard: guard, Operators: numericOperators, compile: commercialNumeric("p.observed_order_count", "r.observed_order_count")},
		{Key: "service_recovery.unresolved_complaint_count", Label: "Unresolved complaints", Description: "Number of unresolved support complaints for the relationship.", Type: FieldInteger, Source: SourceCompletion, AvailabilityGuard: guard, Operators: numericOperators, compile: correlatedNumeric("crm_customer_service_recovery_projections", "unresolved_complaint_count")},
		{Key: "customer_value.band", Label: "Customer value band", Description: "Current customer value band derived from observed commercial history.", Type: FieldEnum, Source: SourceCompletion, AvailabilityGuard: guard, Operators: enumOperators, EnumValues: []string{"STANDARD", "ESTABLISHED", "PRIORITY", "STRATEGIC"}, compile: correlatedEnum("crm_customer_value_projections", "band")},
	}
	for _, name := range []string{"repeat", "lapsed", "high_value", "at_risk", "bulk_capable", "corporate", "recurring"} {
		fields = append(fields, &SegmentField{Key: "relationship_signal." + name, Label: "Relationship signal " + name, Description: "Whether the relationship currently has the " + name + " signal.", Type: FieldBoolean, Source: SourceCompletion, AvailabilityGuard: guard, Operators: booleanOperators, compile: correlatedBoolean("crm_relationship_signals", name+"_signal")})
	}
	return fields
}

func correlatedNumeric(table, column string) func(*Condition, *sqlBuilder) string {
	return func(c *Condition, b *sqlBuilder) string {
		return correlatedExpr(table, column, c, b, numericComparison)
	}
}
func correlatedEnum(table, column string) func(*Condition, *sqlBuilder) string {
	return func(c *Condition, b *sqlBuilder) string { return correlatedExpr(table, column, c, b, enumComparison) }
}
func correlatedBoolean(table, column string) func(*Condition, *sqlBuilder) string {
	return func(c *Condition, b *sqlBuilder) string {
		return correlatedExpr(table, column, c, b, booleanComparison)
	}
}
func correlatedExpr(table, column string, c *Condition, b *sqlBuilder, render func(string, *Condition, *sqlBuilder) string) string {
	value := render("x."+column, c, b)
	return "EXISTS (SELECT 1 FROM " + table + " x WHERE x.business_id=p.business_id AND x.profile_id=p.id AND x.state='CURRENT' AND " + value + ")"
}

func (r *SegmentFieldRegistry) add(field *SegmentField) {
	if field.Version == 0 {
		field.Version = 1
	}
	if len(field.Scopes) == 0 {
		field.Scopes = []string{ScopeBusiness, ScopeBranch}
	}
	if field.NullableBehavior == "" {
		field.NullableBehavior = "UNKNOWN_DOES_NOT_MATCH"
	}
	if field.PrivacyBehavior == "" {
		field.PrivacyBehavior = "EXCLUDES_REDACTED_AND_MERGED_SOURCE"
	}
	if field.BranchBehavior == "" {
		switch field.Source {
		case SourceCommercial, SourceRelationship, SourceTags, SourceIntelligence:
			field.BranchBehavior = "USES_BRANCH_SCOPED_PROJECTION"
		default:
			field.BranchBehavior = "SAME_VALUE_WITHIN_AUTHORIZED_SCOPE"
		}
	}
	r.fields[field.Key] = field
	r.order = append(r.order, field.Key)
}

// Explain is the field's deterministic explanation renderer. Keeping it on
// the registry entry ensures preview and stored-definition explanations use
// the same metadata as validation and SQL compilation.
func (f SegmentField) Explain(condition *Condition) string {
	return describeCondition(condition)
}

// Field looks up a field by key.
func (r *SegmentFieldRegistry) Field(key string) (*SegmentField, bool) {
	field, found := r.fields[key]
	return field, found
}

// Fields lists every registered field in a stable order.
func (r *SegmentFieldRegistry) Fields() []*SegmentField {
	out := make([]*SegmentField, 0, len(r.order))
	keys := append([]string(nil), r.order...)
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, r.fields[key])
	}
	return out
}

// AvailabilityOf reports whether a field can currently answer.
func (r *SegmentFieldRegistry) AvailabilityOf(field *SegmentField) bool {
	return field.Available(r.availability)
}

// ---------------------------------------------------------------------------
// SQL building
// ---------------------------------------------------------------------------

// sqlBuilder accumulates bind parameters. Callers never receive a way to append
// raw SQL text derived from input: bind returns a placeholder, and the only
// literal SQL in a compiled rule comes from this file.
type sqlBuilder struct {
	args []any
	// asOf pins the evaluation instant so every profile in one run is judged
	// against the same clock.
	asOf time.Time
	// branchID is set for a branch-scoped rule; the joined relation alias r is
	// only available then.
	branchID string
	scope    SegmentScope
	// availability is the deployment state, used to hard-gate fields whose
	// backing integration is switched off.
	availability availability
}

func (b *sqlBuilder) bind(value any) string {
	b.args = append(b.args, value)
	return "$" + itoa(len(b.args))
}

func (b *sqlBuilder) branch() bool { return b.scope.Branch() }

// commercialColumn picks the business-wide column or the branch-relation column
// for a commercial measure. A branch-scoped segment must never reason over the
// business total — that is the difference between "this branch's regulars" and
// a branch manager reading the whole company's book.
func (b *sqlBuilder) commercialColumn(business, branch string) string {
	if b.branch() {
		return branch
	}
	return business
}

// guardExpression wraps a condition so an unanswerable field yields SQL NULL
// rather than false. Postgres three-valued logic then does the right thing at
// every level: NULL AND false is false, NULL OR true is true, NOT NULL is NULL,
// and the outermost `IS TRUE` turns any surviving NULL into a non-match. That
// is why "we have no commercial data" can never be read as "spent nothing".
func guardExpression(guard, expression string) string {
	if guard == "" {
		return "(" + expression + ")"
	}
	return "(CASE WHEN " + guard + " THEN (" + expression + ") ELSE NULL END)"
}

func (b *sqlBuilder) commercialGuard() string {
	if b.branch() {
		return "r.id IS NOT NULL AND r.projection_state <> 'NOT_AVAILABLE'"
	}
	return "p.commercial_projection_state <> 'NOT_AVAILABLE'"
}

// CompileRule renders a validated rule into a parameterised SQL boolean
// expression. Nothing in the returned string originates from client input.
func CompileRule(rule *ValidatedRule, builder *sqlBuilder) string {
	base := compileNode(rule.Root, builder)
	if rule.Exclusion == nil {
		return base
	}
	// Exclusion is subtractive: a customer the exclusion arm cannot decide on
	// (NULL) is not excluded, so an unavailable exclusion field never silently
	// empties the audience.
	return "((" + base + ") AND NOT COALESCE((" + compileNode(rule.Exclusion, builder) + "), false))"
}

func compileNode(node *ValidatedNode, builder *sqlBuilder) string {
	if node == nil {
		return "true"
	}
	if node.Condition != nil {
		return compileCondition(node.Condition, builder)
	}
	if node.Op == OpNOT {
		return "(NOT (" + compileNode(node.Children[0], builder) + "))"
	}
	parts := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		parts = append(parts, compileNode(child, builder))
	}
	return "(" + strings.Join(parts, " "+node.Op+" ") + ")"
}

// compileCondition renders one condition, first applying the deployment-level
// availability gate.
//
// Consent is hard-gated: with no consent authority wired, Section 8A's rule is
// that no projection can be trusted, so the condition is unanswerable (NULL)
// rather than asserting UNKNOWN. Commercial data needs no such gate because it
// is guarded per row by commercial_projection_state, which is NOT_AVAILABLE for
// every relationship when nothing has ever been ingested — the same answer,
// derived from the data instead of from a flag.
func compileCondition(condition *Condition, builder *sqlBuilder) string {
	if condition.Field.AvailabilityGuard == "consent" && !builder.availability.Consent {
		return "NULL"
	}
	return condition.Field.compile(condition, builder)
}

// ---------------------------------------------------------------------------
// Field definitions
// ---------------------------------------------------------------------------

var (
	enumOperators       = []string{OperatorEQ, OperatorNEQ, OperatorIN, OperatorNotIN}
	numericOperators    = []string{OperatorEQ, OperatorNEQ, OperatorGT, OperatorGTE, OperatorLT, OperatorLTE, OperatorBETWEEN}
	timestampOperators  = []string{OperatorBefore, OperatorAfter, OperatorWithinLast, OperatorExists, OperatorNotExists}
	tagOperators        = []string{OperatorHasTag, OperatorNotHasTag}
	booleanOperators    = []string{OperatorEQ, OperatorNEQ}
	referenceOperators  = []string{OperatorEQ, OperatorIN}
	capabilityTypeValue = []string{"RESTAURANT", "FAST_FOOD", "CLOUD_KITCHEN", "PASTRY_BAKERY", "SHOP_GROCERY", "MARKET", "WHOLESALE", "CATERING", "MANUFACTURER"}
)

func profileFields() []*SegmentField {
	return []*SegmentField{
		{
			Key: "profile.subject_type", Label: "Subject type", Type: FieldEnum, Source: SourceProfile,
			Description: "Whether the relationship is with a registered customer, a guest checkout or an imported subject.",
			Operators:   enumOperators, EnumValues: []string{"CUSTOMER", "GUEST", "EXTERNAL"},
			compile: enumCompiler("p.subject_type", ""),
		},
		{
			Key: "profile.status", Label: "Relationship status", Type: FieldEnum, Source: SourceProfile,
			Description: "The vendor-facing lifecycle of the relationship.",
			Operators:   enumOperators, EnumValues: []string{"ACTIVE", "ARCHIVED"},
			compile: enumCompiler("p.status", ""),
		},
		{
			Key: "profile.acquisition_source_type", Label: "Acquisition source", Type: FieldEnum, Source: SourceProfile,
			Description: "How the relationship was first observed.",
			Operators:   append(append([]string{}, enumOperators...), OperatorExists, OperatorNotExists),
			compile:     enumCompiler("p.acquisition_source_type", ""),
		},
	}
}

func commercialFields() []*SegmentField {
	guard := "commercial"
	return []*SegmentField{
		{
			Key: "commercial.projection_state", Label: "Commercial data state", Type: FieldEnum, Source: SourceCommercial,
			Description: "Whether authoritative commercial data has been observed for this relationship.",
			Operators:   enumOperators,
			EnumValues:  []string{"NOT_AVAILABLE", "CURRENT", "STALE", "REBUILDING", "ERROR"},
			// Deliberately unguarded: this field's whole purpose is to let a rule
			// ask about availability itself.
			compile: func(condition *Condition, builder *sqlBuilder) string {
				column := builder.commercialColumn("p.commercial_projection_state", "COALESCE(r.projection_state, 'NOT_AVAILABLE')")
				return enumCompiler(column, "")(condition, builder)
			},
		},
		{
			Key: "commercial.completed_order_count", Label: "Completed order count", Type: FieldInteger, Source: SourceCommercial,
			Description: "Number of completed orders observed for this relationship.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: commercialNumeric("p.completed_order_count", "r.completed_order_count"),
		},
		{
			Key: "commercial.gross_spend_minor", Label: "Gross spend", Type: FieldMoneyMinor, Source: SourceCommercial,
			Description: "Gross completed spend in minor currency units.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: commercialNumeric("p.gross_spend_minor", "r.gross_spend_minor"),
		},
		{
			Key: "commercial.net_spend_minor", Label: "Net spend", Type: FieldMoneyMinor, Source: SourceCommercial,
			Description: "Net completed spend after refunds, in minor currency units.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: commercialNumeric("p.net_spend_minor", "r.net_spend_minor"),
		},
		{
			Key: "commercial.average_order_value_minor", Label: "Average order value", Type: FieldMoneyMinor, Source: SourceCommercial,
			Description: "Net spend divided by completed order count, in minor currency units.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: commercialNumeric("p.average_order_value_minor",
				"(r.net_spend_minor / NULLIF(r.completed_order_count, 0))"),
		},
		{
			Key: "commercial.first_completed_order_at", Label: "First completed order", Type: FieldTimestamp, Source: SourceCommercial,
			Description: "When the first completed order was observed.",
			Operators:   timestampOperators, AvailabilityGuard: guard,
			compile: commercialTimestamp("p.first_completed_order_at", "r.first_relationship_at"),
		},
		{
			Key: "commercial.last_completed_order_at", Label: "Last completed order", Type: FieldTimestamp, Source: SourceCommercial,
			Description: "When the most recent completed order was observed.",
			Operators:   timestampOperators, AvailabilityGuard: guard,
			compile: commercialTimestamp("p.last_completed_order_at", "r.last_relationship_at"),
		},
		{
			Key: "commercial.days_since_last_order", Label: "Days since last order", Type: FieldInteger, Source: SourceCommercial,
			Description: "Whole days between the evaluation instant and the last completed order.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: func(condition *Condition, builder *sqlBuilder) string {
				column := builder.commercialColumn("p.last_completed_order_at", "r.last_relationship_at")
				// Derived from the observed order timestamp only. It is never
				// computed from when the profile row happened to be created,
				// which would invent a recency the business never observed.
				expression := "FLOOR(EXTRACT(EPOCH FROM (" + builder.bind(builder.asOf) + "::timestamptz - " + column + ")) / 86400)::bigint"
				guardSQL := builder.commercialGuard() + " AND " + column + " IS NOT NULL"
				return guardExpression(guardSQL, numericComparison(expression, condition, builder))
			},
		},
	}
}

func relationshipFields() []*SegmentField {
	return []*SegmentField{
		{
			Key: "relationship.branch_ref", Label: "Related branch", Type: FieldEnum, Source: SourceRelationship,
			Description: "A branch the customer has an observed relationship with.",
			Operators:   referenceOperators,
			compile: func(condition *Condition, builder *sqlBuilder) string {
				// Always an EXISTS scoped to the same business, so a branch id
				// belonging to another tenant matches nothing rather than
				// erroring in a way that would confirm its existence.
				placeholders := make([]string, 0, len(condition.Strings))
				for _, value := range condition.Strings {
					placeholders = append(placeholders, builder.bind(value))
				}
				return "(EXISTS (SELECT 1 FROM crm_customer_branch_relations br WHERE br.business_id = p.business_id" +
					" AND br.profile_id = p.id AND br.branch_id IN (" + strings.Join(placeholders, ",") + ")))"
			},
		},
		{
			Key: "relationship.capability_type", Label: "Capability last used", Type: FieldEnum, Source: SourceRelationship,
			Description: "The vendor capability the relationship last transacted through.",
			Operators:   append(append([]string{}, enumOperators...), OperatorExists, OperatorNotExists),
			EnumValues:  capabilityTypeValue,
			compile: func(condition *Condition, builder *sqlBuilder) string {
				column := builder.commercialColumn("p.last_capability_type", "r.last_capability_type")
				return enumCompiler(column, "")(condition, builder)
			},
		},
	}
}

func tagField() *SegmentField {
	return &SegmentField{
		Key: "crm.tags", Label: "Customer tags", Type: FieldTagReference, Source: SourceTags,
		Description: "Vendor-defined tags assigned to the customer.",
		Operators:   tagOperators,
		compile: func(condition *Condition, builder *sqlBuilder) string {
			exists := "EXISTS (SELECT 1 FROM crm_customer_tag_memberships tm" +
				" WHERE tm.business_id = p.business_id AND tm.profile_id = p.id AND tm.tag_id = " +
				builder.bind(condition.Strings[0]) + ")"
			if condition.Operator == OperatorNotHasTag {
				return "(NOT " + exists + ")"
			}
			return "(" + exists + ")"
		},
	}
}

// consentFields registers one decision field and one eligibility field per
// marketing channel.
//
// There is deliberately no consent-override field: eligibility is derived, and
// a rule that could assert it would make the whole consent model advisory.
func consentFields() []*SegmentField {
	fields := make([]*SegmentField, 0, len(MarketingChannels)*2)
	for _, channel := range MarketingChannels {
		key := strings.ToLower(string(channel))
		fields = append(fields, &SegmentField{
			Key: "consent.marketing." + key + ".decision", Label: "Marketing consent decision (" + key + ")",
			Type: FieldEnum, Source: SourceConsent, AvailabilityGuard: "consent",
			Description: "The projected marketing consent decision for this channel.",
			Operators:   enumOperators, EnumValues: []string{"ALLOW", "DENY", "UNKNOWN"},
			compile: consentDecisionCompiler(channel),
		}, &SegmentField{
			Key: "consent.marketing." + key + ".eligible", Label: "Marketing eligible (" + key + ")",
			Type: FieldBoolean, Source: SourceConsent, AvailabilityGuard: "consent",
			Description: "Whether the customer may currently be marketed to on this channel, after consent and privacy.",
			Operators:   booleanOperators,
			compile:     consentEligibleCompiler(channel),
		})
	}
	return fields
}

// consentDecisionCompiler renders the effective decision, collapsing "no row"
// and "expired" into UNKNOWN exactly as ConsentProjection.EffectiveDecision
// does in Go, so the two never disagree.
func consentDecisionCompiler(channel ConsentChannel) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		expression := "COALESCE((SELECT CASE WHEN cp.expires_at IS NOT NULL AND cp.expires_at <= " +
			builder.bind(builder.asOf) + "::timestamptz THEN 'UNKNOWN' ELSE cp.decision END" +
			" FROM crm_consent_projections cp WHERE cp.business_id = p.business_id AND cp.profile_id = p.id" +
			" AND cp.purpose = 'MARKETING' AND cp.channel = " + builder.bind(string(channel)) + "), 'UNKNOWN')"
		return guardExpression("", enumComparison(expression, condition, builder))
	}
}

// consentEligibleCompiler mirrors MarketingEligibility's precedence in SQL.
//
// Redaction, merge, suppression, restriction and archival each independently
// deny, and only an unexpired, unstale explicit ALLOW permits. The Go rule
// remains the single definition of the policy; this is its set-based twin, and
// an integration test asserts the two agree over the same seeded profiles.
func consentEligibleCompiler(channel ConsentChannel) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		asOf := builder.bind(builder.asOf)
		staleCutoff := builder.bind(builder.asOf.Add(-ConsentStaleAfter))
		expression := "(p.privacy_state = 'ACTIVE' AND p.merge_state <> 'MERGED_SOURCE' AND p.status = 'ACTIVE'" +
			" AND NOT EXISTS (SELECT 1 FROM crm_privacy_projections pp WHERE pp.business_id = p.business_id" +
			" AND pp.profile_id = p.id AND pp.action = 'MARKETING_SUPPRESS' AND pp.processing_status = 'APPLIED')" +
			" AND EXISTS (SELECT 1 FROM crm_consent_projections cp WHERE cp.business_id = p.business_id" +
			" AND cp.profile_id = p.id AND cp.purpose = 'MARKETING' AND cp.channel = " + builder.bind(string(channel)) +
			" AND cp.decision = 'ALLOW'" +
			" AND (cp.expires_at IS NULL OR cp.expires_at > " + asOf + "::timestamptz)" +
			" AND cp.observed_at > " + staleCutoff + "::timestamptz))"

		want := len(condition.Bools) > 0 && condition.Bools[0]
		if condition.Operator == OperatorNEQ {
			want = !want
		}
		if want {
			return "(" + expression + ")"
		}
		return "(NOT " + expression + ")"
	}
}

// ---------------------------------------------------------------------------
// Comparison builders
// ---------------------------------------------------------------------------

func enumCompiler(column, guard string) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		return guardExpression(guard, enumComparison(column, condition, builder))
	}
}

func enumComparison(column string, condition *Condition, builder *sqlBuilder) string {
	switch condition.Operator {
	case OperatorExists:
		return column + " IS NOT NULL"
	case OperatorNotExists:
		return column + " IS NULL"
	case OperatorEQ:
		return column + " = " + builder.bind(condition.Strings[0])
	case OperatorNEQ:
		return column + " IS DISTINCT FROM " + builder.bind(condition.Strings[0])
	case OperatorIN:
		return column + " = ANY(" + builder.bind(condition.Strings) + ")"
	case OperatorNotIN:
		return "NOT (" + column + " = ANY(" + builder.bind(condition.Strings) + "))"
	}
	// Unreachable: the operator allowlist is checked during validation, and
	// every enum operator is handled above. Returning a hard false rather than
	// panicking keeps an unexpected future operator from selecting everyone.
	return "false"
}

func commercialNumeric(businessColumn, branchColumn string) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		column := builder.commercialColumn(businessColumn, branchColumn)
		return guardExpression(builder.commercialGuard(), numericComparison(column, condition, builder))
	}
}

func numericComparison(column string, condition *Condition, builder *sqlBuilder) string {
	switch condition.Operator {
	case OperatorBETWEEN:
		return column + " BETWEEN " + builder.bind(condition.Ints[0]) + " AND " + builder.bind(condition.Ints[1])
	case OperatorEQ:
		return column + " = " + builder.bind(condition.Ints[0])
	case OperatorNEQ:
		return column + " IS DISTINCT FROM " + builder.bind(condition.Ints[0])
	case OperatorGT:
		return column + " > " + builder.bind(condition.Ints[0])
	case OperatorGTE:
		return column + " >= " + builder.bind(condition.Ints[0])
	case OperatorLT:
		return column + " < " + builder.bind(condition.Ints[0])
	case OperatorLTE:
		return column + " <= " + builder.bind(condition.Ints[0])
	}
	return "false"
}

func booleanComparison(column string, condition *Condition, builder *sqlBuilder) string {
	want := len(condition.Bools) > 0 && condition.Bools[0]
	expression := column + " IS "
	if !want {
		expression += "NOT "
	}
	expression += "TRUE"
	if condition.Operator == OperatorNEQ {
		if want {
			expression = column + " IS NOT TRUE"
		} else {
			expression = column + " IS TRUE"
		}
	}
	return expression
}

func commercialTimestamp(businessColumn, branchColumn string) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		column := builder.commercialColumn(businessColumn, branchColumn)
		var expression string
		switch condition.Operator {
		case OperatorExists:
			expression = column + " IS NOT NULL"
		case OperatorNotExists:
			expression = column + " IS NULL"
		case OperatorBefore:
			expression = column + " < " + builder.bind(condition.Times[0]) + "::timestamptz"
		case OperatorAfter:
			expression = column + " > " + builder.bind(condition.Times[0]) + "::timestamptz"
		case OperatorWithinLast:
			expression = column + " >= " + builder.bind(builder.asOf.Add(-condition.Duration)) + "::timestamptz"
		default:
			expression = "false"
		}
		return guardExpression(builder.commercialGuard(), expression)
	}
}
