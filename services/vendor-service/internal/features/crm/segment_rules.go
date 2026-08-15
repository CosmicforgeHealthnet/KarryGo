package crm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// hashRuleBytes produces the canonical definition hash. It is a plain content
// hash of the normalised document, so the same rule always yields the same
// identity regardless of how a client formatted it.
func hashRuleBytes(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// Stable Section 8B error codes. They travel in the error envelope's details
// map so a client can branch on a code while the human message stays free to
// change. Codes are shared with docs/openapi/section8b-crm-tags-segmentation.
const (
	CodeTagNotFound          = "CRM_TAG_NOT_FOUND"
	CodeTagAlreadyExists     = "CRM_TAG_ALREADY_EXISTS"
	CodeTagArchived          = "CRM_TAG_ARCHIVED"
	CodeTagScopeDenied       = "CRM_TAG_SCOPE_DENIED"
	CodeBulkTooLarge         = "CRM_BULK_OPERATION_TOO_LARGE"
	CodeSegmentNotFound      = "CRM_SEGMENT_NOT_FOUND"
	CodeSegmentAlreadyExists = "CRM_SEGMENT_ALREADY_EXISTS"
	CodeSegmentInvalidState  = "CRM_SEGMENT_INVALID_STATE"
	CodeSegmentRuleRequired  = "CRM_SEGMENT_RULE_REQUIRED"
	CodeSegmentRuleNotAllwd  = "CRM_SEGMENT_RULE_NOT_ALLOWED"
	CodeSegmentScopeDenied   = "CRM_SEGMENT_SCOPE_DENIED"
	CodeSegmentEvalRunning   = "CRM_SEGMENT_EVALUATION_IN_PROGRESS"
	CodeSegmentEvalFailed    = "CRM_SEGMENT_EVALUATION_FAILED"
	CodeSegmentStaticOnly    = "CRM_SEGMENT_STATIC_ONLY"
	CodeSegmentDynamicOnly   = "CRM_SEGMENT_DYNAMIC_ONLY"
	CodeSegmentExclusionDup  = "CRM_SEGMENT_EXCLUSION_EXISTS"
	CodeSegmentMemberMissing = "CRM_SEGMENT_MEMBER_NOT_FOUND"
	CodeProfileRedacted      = "CRM_PROFILE_REDACTED"
	CodeProfileMerged        = "CRM_PROFILE_MERGED"
	CodeVersionConflict      = "CRM_VERSION_CONFLICT"
	CodeCrossTenant          = "CRM_CROSS_TENANT_REFERENCE"
	CodeFeatureDisabled      = "CRM_SEGMENTATION_DISABLED"
)

// RuleSchemaVersion is the only DSL version this build accepts. It travels in
// every rule document so a future incompatible grammar can be introduced
// without silently reinterpreting stored definitions.
const RuleSchemaVersion = 1

// Group operators.
const (
	OpAND = "AND"
	OpOR  = "OR"
	OpNOT = "NOT"
)

// Condition operators. This is the complete allowlist: an operator that is not
// in this set cannot reach the compiler, and the compiler has no default case
// that would let an unknown operator through.
const (
	OperatorEQ         = "EQ"
	OperatorNEQ        = "NEQ"
	OperatorIN         = "IN"
	OperatorNotIN      = "NOT_IN"
	OperatorGT         = "GT"
	OperatorGTE        = "GTE"
	OperatorLT         = "LT"
	OperatorLTE        = "LTE"
	OperatorBETWEEN    = "BETWEEN"
	OperatorExists     = "EXISTS"
	OperatorNotExists  = "NOT_EXISTS"
	OperatorBefore     = "BEFORE"
	OperatorAfter      = "AFTER"
	OperatorWithinLast = "WITHIN_LAST"
	OperatorHasTag     = "HAS_TAG"
	OperatorNotHasTag  = "NOT_HAS_TAG"
)

// FieldType is the value domain of a registered field. It decides which
// operators are offered and how a value is parsed — a client never picks the
// type, and there is no "any"/JSON type by design.
type FieldType string

const (
	FieldBoolean      FieldType = "BOOLEAN"
	FieldInteger      FieldType = "INTEGER"
	FieldMoneyMinor   FieldType = "MONEY_MINOR"
	FieldEnum         FieldType = "ENUM"
	FieldTimestamp    FieldType = "TIMESTAMP"
	FieldTagReference FieldType = "TAG_REFERENCE"
)

// RuleLimits bounds rule complexity. The values are configuration, not product
// constants: a pathological rule is a denial-of-service against the
// materializer, so the ceiling is deliberately explicit and tunable rather than
// buried as magic numbers in the parser.
type RuleLimits struct {
	MaxDepth         int
	MaxNodes         int
	MaxOrChildren    int
	MaxInValues      int
	MaxNotDepth      int
	MaxTagReferences int
}

// DefaultRuleLimits are the initial guardrails documented in
// docs/FRONTEND_REQUIREMENTS.md.
func DefaultRuleLimits() RuleLimits {
	return RuleLimits{
		MaxDepth:         6,
		MaxNodes:         50,
		MaxOrChildren:    25,
		MaxInValues:      100,
		MaxNotDepth:      3,
		MaxTagReferences: 25,
	}
}

// RuleNode is one node of the rule tree: either a group (Op + Children) or a
// condition (Field + Operator + Value). It is deliberately a single struct so
// the wire format stays the flat, human-readable shape the specification
// documents, with validation — not the type system — rejecting hybrids.
type RuleNode struct {
	Op       string      `json:"op,omitempty"`
	Children []*RuleNode `json:"children,omitempty"`

	Field    string          `json:"field,omitempty"`
	Operator string          `json:"operator,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
}

// IsGroup reports whether this node was written as a group.
func (n *RuleNode) IsGroup() bool { return n != nil && n.Op != "" }

// Rule is a complete segment definition document.
type Rule struct {
	SchemaVersion int       `json:"schema_version"`
	Root          *RuleNode `json:"root"`
	Exclusion     *RuleNode `json:"exclusion,omitempty"`
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ruleError builds the stable validation envelope.
//
// Every rejection carries the offending node's path so a builder UI can point
// at it, and a stable machine code in details. It never carries an internal
// SQL fragment, table name or column name: a rejection must not become a way to
// learn the schema.
func ruleError(code, path, message string, details map[string]any) error {
	violation := apperrors.FieldViolation{Field: "rule", Message: message}
	if path != "" {
		violation.Field = "rule." + path
	}
	err := apperrors.Validation(message, []apperrors.FieldViolation{violation})
	err.Details = map[string]any{"code": code}
	for key, value := range details {
		err.Details[key] = value
	}
	if path != "" {
		err.Details["path"] = path
	}
	return err
}

// crmError builds a non-validation Section 8B error with a stable machine code
// alongside the human message, so clients branch on the code rather than on
// prose that is free to change.
func crmError(err *apperrors.Error, code string) error {
	if err.Details == nil {
		err.Details = map[string]any{}
	}
	err.Details["code"] = code
	return err
}

// Stable Section 8B rule error codes. They are the message field of the
// validation envelope so a client can branch on them without parsing prose.
const (
	CodeRuleInvalid       = "CRM_SEGMENT_RULE_INVALID"
	CodeFieldNotSupported = "CRM_SEGMENT_FIELD_NOT_SUPPORTED"
	CodeOperatorNotSup    = "CRM_SEGMENT_OPERATOR_NOT_SUPPORTED"
	CodeValueInvalid      = "CRM_SEGMENT_VALUE_INVALID"
	CodeTooComplex        = "CRM_SEGMENT_TOO_COMPLEX"
)

// Condition is one validated, typed condition ready for compilation. Reaching
// this type means the field exists in the registry, the operator is legal for
// that field's type, and the value has been parsed into a Go value — a
// compiler therefore never sees client text.
type Condition struct {
	Path     string
	Field    *SegmentField
	Operator string

	Bools   []bool
	Ints    []int64
	Times   []time.Time
	Strings []string

	// Duration carries WITHIN_LAST's window, already normalised to a Go
	// duration so the compiler does not interpolate a unit name.
	Duration time.Duration
	// DurationLabel keeps the window as the author wrote it ("30 days"), so an
	// explanation reads back what they asked for rather than an equivalent the
	// normaliser happened to pick.
	DurationLabel string
	// DurationAmount and DurationUnit are the author's own numbers, kept so the
	// canonical document re-parses through this same validator. A canonical form
	// that could not be revalidated would make every stored rule unreadable the
	// moment it had to be re-run.
	DurationAmount int64
	DurationUnit   string
}

// ValidatedNode is the compiled-ready mirror of a RuleNode.
type ValidatedNode struct {
	Path      string
	Op        string
	Children  []*ValidatedNode
	Condition *Condition
}

// ValidatedRule is a rule that has passed every structural, registry, type and
// complexity check. Nothing downstream re-parses client input.
type ValidatedRule struct {
	SchemaVersion int
	Root          *ValidatedNode
	Exclusion     *ValidatedNode

	// Dependencies is the sorted set of registry field keys the rule reads. It
	// is what lets an invalidation decide which segments a change affects
	// instead of restaking every segment in the business.
	Dependencies []string
	// Hash is the canonical definition hash: two rules that differ only in JSON
	// whitespace or key order produce the same value.
	Hash string
	// Canonical is the normalised document persisted in the version row.
	Canonical []byte
	// UnavailableFields lists dependencies whose backing projection is not
	// integrated in this deployment, so a preview can warn instead of quietly
	// returning nobody.
	UnavailableFields []string
	// NegatedAvailabilityGuarded is true when a NOT group wraps a condition
	// whose data may be unavailable. Callers surface it as a preview warning.
	NegatedAvailabilityGuarded bool
}

// ruleValidator carries the per-parse counters that enforce the limits.
type ruleValidator struct {
	registry *SegmentFieldRegistry
	limits   RuleLimits
	scope    SegmentScope

	nodes        int
	tagRefs      int
	dependencies map[string]bool
}

// SegmentScope is the reach a rule is evaluated at. A BRANCH-scoped rule reads
// that branch's slice of every commercial fact, never the business total.
type SegmentScope struct {
	Type     string
	BranchID string
}

// Branch reports whether the scope is limited to a single branch.
func (s SegmentScope) Branch() bool { return s.Type == ScopeBranch }

// Segment scope types.
const (
	ScopeBusiness = "BUSINESS"
	ScopeBranch   = "BRANCH"
)

// ValidateRule is the single entry point from untrusted JSON to a compilable
// rule. Every path out of it is either a ValidatedRule or a stable validation
// error; there is deliberately no "best effort" mode.
func ValidateRule(raw json.RawMessage, registry *SegmentFieldRegistry, limits RuleLimits, scope SegmentScope) (*ValidatedRule, error) {
	if len(raw) == 0 {
		return nil, ruleError(CodeRuleInvalid, "", "A segment rule is required.", nil)
	}
	var document Rule
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	// Unknown members are rejected outright: a typo'd key that is silently
	// ignored is how a rule ends up meaning something its author did not write.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, ruleError(CodeRuleInvalid, "", "The segment rule is not a valid rule document.", nil)
	}
	if document.SchemaVersion != RuleSchemaVersion {
		return nil, ruleError(CodeRuleInvalid, "schema_version",
			"Unsupported rule schema version.", map[string]any{"supported": RuleSchemaVersion})
	}
	if document.Root == nil {
		return nil, ruleError(CodeRuleInvalid, "root", "A segment rule must have a root node.", nil)
	}

	validator := &ruleValidator{registry: registry, limits: limits, scope: scope, dependencies: map[string]bool{}}
	root, err := validator.node(document.Root, "root", 1, 0)
	if err != nil {
		return nil, err
	}
	var exclusion *ValidatedNode
	if document.Exclusion != nil {
		// The exclusion arm is a rule in its own right and gets exactly the same
		// registry, operator, type and complexity treatment as the base rule.
		if exclusion, err = validator.node(document.Exclusion, "exclusion", 1, 0); err != nil {
			return nil, err
		}
	}

	dependencies := make([]string, 0, len(validator.dependencies))
	for key := range validator.dependencies {
		dependencies = append(dependencies, key)
	}
	sort.Strings(dependencies)

	canonical, err := canonicalRule(document.SchemaVersion, root, exclusion)
	if err != nil {
		return nil, err
	}

	validated := &ValidatedRule{
		SchemaVersion: document.SchemaVersion,
		Root:          root,
		Exclusion:     exclusion,
		Dependencies:  dependencies,
		Hash:          hashRuleBytes(canonical),
		Canonical:     canonical,
	}
	for _, key := range dependencies {
		if field, found := registry.Field(key); found && !field.Available(registry.availability) {
			validated.UnavailableFields = append(validated.UnavailableFields, key)
		}
	}
	validated.NegatedAvailabilityGuarded = negatesGuardedField(root, false) || negatesGuardedField(exclusion, false)
	return validated, nil
}

func (v *ruleValidator) node(node *RuleNode, path string, depth, notDepth int) (*ValidatedNode, error) {
	if node == nil {
		return nil, ruleError(CodeRuleInvalid, path, "A rule node is required.", nil)
	}
	v.nodes++
	if v.nodes > v.limits.MaxNodes {
		return nil, ruleError(CodeTooComplex, path, "The rule has too many conditions.",
			map[string]any{"max_nodes": v.limits.MaxNodes})
	}
	if depth > v.limits.MaxDepth {
		return nil, ruleError(CodeTooComplex, path, "The rule is nested too deeply.",
			map[string]any{"actual_depth": depth, "max_depth": v.limits.MaxDepth})
	}

	group := node.Op != ""
	condition := node.Field != "" || node.Operator != ""
	if group && condition {
		return nil, ruleError(CodeRuleInvalid, path, "A node is either a group or a condition, not both.", nil)
	}
	if group {
		return v.group(node, path, depth, notDepth)
	}
	return v.condition(node, path)
}

func (v *ruleValidator) group(node *RuleNode, path string, depth, notDepth int) (*ValidatedNode, error) {
	op := strings.ToUpper(strings.TrimSpace(node.Op))
	switch op {
	case OpAND, OpOR, OpNOT:
	default:
		return nil, ruleError(CodeRuleInvalid, path, "Unsupported group operator.", nil)
	}
	if len(node.Children) == 0 {
		return nil, ruleError(CodeRuleInvalid, path, "A group must contain at least one condition.", nil)
	}
	if op == OpNOT {
		if len(node.Children) != 1 {
			return nil, ruleError(CodeRuleInvalid, path, "A NOT group must contain exactly one child.", nil)
		}
		notDepth++
		if notDepth > v.limits.MaxNotDepth {
			return nil, ruleError(CodeTooComplex, path, "The rule nests NOT too deeply.",
				map[string]any{"max_not_depth": v.limits.MaxNotDepth})
		}
	}
	if op == OpOR && len(node.Children) > v.limits.MaxOrChildren {
		return nil, ruleError(CodeTooComplex, path, "The OR group has too many branches.",
			map[string]any{"max_or_children": v.limits.MaxOrChildren})
	}

	validated := &ValidatedNode{Path: path, Op: op}
	for index, child := range node.Children {
		childPath := path + ".children[" + strconv.Itoa(index) + "]"
		validatedChild, err := v.node(child, childPath, depth+1, notDepth)
		if err != nil {
			return nil, err
		}
		validated.Children = append(validated.Children, validatedChild)
	}
	return validated, nil
}

func (v *ruleValidator) condition(node *RuleNode, path string) (*ValidatedNode, error) {
	key := strings.TrimSpace(node.Field)
	if key == "" {
		return nil, ruleError(CodeRuleInvalid, path, "A condition must name a field.", nil)
	}
	field, found := v.registry.Field(key)
	if !found {
		// The message never distinguishes "no such field" from "field exists but
		// is forbidden": both answers would tell a probing client something
		// about the schema it is not entitled to know.
		return nil, ruleError(CodeFieldNotSupported, path, "This field cannot be used in a segment rule.", nil)
	}
	if field.BranchOnly && !v.scope.Branch() {
		return nil, ruleError(CodeFieldNotSupported, path, "This field is only available in a branch-scoped segment.", nil)
	}

	operator := strings.ToUpper(strings.TrimSpace(node.Operator))
	if !field.SupportsOperator(operator) {
		return nil, ruleError(CodeOperatorNotSup, path, "This operator cannot be used with this field.", nil)
	}

	condition := &Condition{Path: path, Field: field, Operator: operator}
	if err := v.value(condition, node.Value, path); err != nil {
		return nil, err
	}
	if field.Type == FieldTagReference {
		v.tagRefs++
		if v.tagRefs > v.limits.MaxTagReferences {
			return nil, ruleError(CodeTooComplex, path, "The rule references too many tags.",
				map[string]any{"max_tag_references": v.limits.MaxTagReferences})
		}
	}
	v.dependencies[field.Key] = true
	return &ValidatedNode{Path: path, Condition: condition}, nil
}

// value parses the typed literal for one condition. Every branch either
// produces a Go value of the field's declared type or fails; nothing is coerced
// and no string ever survives into SQL as anything but a bind parameter.
func (v *ruleValidator) value(condition *Condition, raw json.RawMessage, path string) error {
	field := condition.Field
	invalid := func(message string) error { return ruleError(CodeValueInvalid, path, message, nil) }

	switch condition.Operator {
	case OperatorExists, OperatorNotExists:
		// A presence test takes no operand; accepting one would let a client
		// smuggle an unvalidated literal past the type check.
		if len(raw) > 0 && string(raw) != "null" {
			return invalid("This operator does not take a value.")
		}
		return nil
	case OperatorWithinLast:
		var window struct {
			Amount int64  `json:"amount"`
			Unit   string `json:"unit"`
		}
		if err := strictUnmarshal(raw, &window); err != nil {
			return invalid("A time window must be an object with an amount and a unit.")
		}
		if window.Amount <= 0 || window.Amount > 3650 {
			return invalid("A time window amount must be between 1 and 3650.")
		}
		unit, supported := durationUnits[strings.ToUpper(strings.TrimSpace(window.Unit))]
		if !supported {
			return invalid("A time window unit must be DAY, WEEK, MONTH or YEAR.")
		}
		condition.Duration = time.Duration(window.Amount) * unit
		condition.DurationAmount = window.Amount
		condition.DurationUnit = strings.ToUpper(strings.TrimSpace(window.Unit))
		condition.DurationLabel = plural(window.Amount, strings.ToLower(condition.DurationUnit))
		return nil
	case OperatorHasTag, OperatorNotHasTag:
		var reference struct {
			TagID string `json:"tag_id"`
		}
		if err := strictUnmarshal(raw, &reference); err != nil {
			return invalid("A tag reference must be an object with a tag_id.")
		}
		id := strings.TrimSpace(reference.TagID)
		if id == "" || len(id) > 64 {
			return invalid("A tag reference must name a tag.")
		}
		condition.Strings = []string{id}
		return nil
	case OperatorIN, OperatorNotIN:
		var values []string
		if err := strictUnmarshal(raw, &values); err != nil {
			return invalid("This operator takes a list of values.")
		}
		if len(values) == 0 {
			return invalid("The value list must not be empty.")
		}
		if len(values) > v.limits.MaxInValues {
			return ruleError(CodeTooComplex, path, "The value list is too long.",
				map[string]any{"max_in_values": v.limits.MaxInValues})
		}
		for _, value := range values {
			if !field.AllowsEnumValue(value) {
				return invalid("Unsupported value for this field.")
			}
			condition.Strings = append(condition.Strings, value)
		}
		return nil
	case OperatorBETWEEN:
		var bounds []json.RawMessage
		if err := strictUnmarshal(raw, &bounds); err != nil || len(bounds) != 2 {
			return invalid("BETWEEN takes exactly two bounds.")
		}
		for _, bound := range bounds {
			if err := v.scalar(condition, bound, path); err != nil {
				return err
			}
		}
		if len(condition.Ints) == 2 && condition.Ints[0] > condition.Ints[1] {
			return invalid("The lower bound must not be greater than the upper bound.")
		}
		return nil
	default:
		return v.scalar(condition, raw, path)
	}
}

// scalar parses one literal of the field's declared type.
func (v *ruleValidator) scalar(condition *Condition, raw json.RawMessage, path string) error {
	field := condition.Field
	invalid := func(message string) error { return ruleError(CodeValueInvalid, path, message, nil) }

	switch field.Type {
	case FieldBoolean:
		var value bool
		if err := strictUnmarshal(raw, &value); err != nil {
			return invalid("This field takes true or false.")
		}
		condition.Bools = append(condition.Bools, value)
	case FieldInteger:
		var value int64
		if err := strictUnmarshal(raw, &value); err != nil {
			return invalid("This field takes a whole number.")
		}
		if value < 0 || value > 1_000_000_000 {
			return invalid("The value is out of range for this field.")
		}
		condition.Ints = append(condition.Ints, value)
	case FieldMoneyMinor:
		var money struct {
			Currency string `json:"currency"`
			Minor    int64  `json:"minor"`
		}
		if err := strictUnmarshal(raw, &money); err != nil {
			return invalid("A money value must be an object with a currency and a minor amount.")
		}
		// Currency is validated but never compared in SQL: CRM stores exactly
		// one currency per business and refuses cross-currency facts at
		// ingestion, so a mismatch here is a client error, not a conversion.
		if len(money.Currency) != 3 || strings.ToUpper(money.Currency) != money.Currency {
			return invalid("A money value needs a three-letter uppercase currency code.")
		}
		if money.Minor < 0 {
			return invalid("A money value must not be negative.")
		}
		condition.Strings = append(condition.Strings, money.Currency)
		condition.Ints = append(condition.Ints, money.Minor)
	case FieldTimestamp:
		var value string
		if err := strictUnmarshal(raw, &value); err != nil {
			return invalid("A timestamp must be an RFC3339 string.")
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return invalid("A timestamp must be an RFC3339 string.")
		}
		condition.Times = append(condition.Times, parsed.UTC())
	case FieldEnum:
		var value string
		if err := strictUnmarshal(raw, &value); err != nil {
			return invalid("This field takes a single value.")
		}
		if !field.AllowsEnumValue(value) {
			return invalid("Unsupported value for this field.")
		}
		condition.Strings = append(condition.Strings, value)
	default:
		return invalid("Unsupported value for this field.")
	}
	return nil
}

var durationUnits = map[string]time.Duration{
	"DAY":   24 * time.Hour,
	"WEEK":  7 * 24 * time.Hour,
	"MONTH": 30 * 24 * time.Hour,
	"YEAR":  365 * 24 * time.Hour,
}

// strictUnmarshal decodes into target and rejects trailing content and unknown
// object members.
func strictUnmarshal(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return fmt.Errorf("crm: missing value")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("crm: trailing content in value")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Canonicalisation and hashing
// ---------------------------------------------------------------------------

// canonicalNode is the normalised on-disk shape. Field order is fixed by the
// struct and values are re-emitted from the parsed Go values, so two documents
// that mean the same thing serialise identically — which is what makes the hash
// a reliable definition identity rather than a hash of client formatting.
type canonicalNode struct {
	Op       string           `json:"op,omitempty"`
	Children []*canonicalNode `json:"children,omitempty"`
	Field    string           `json:"field,omitempty"`
	Operator string           `json:"operator,omitempty"`
	Value    any              `json:"value,omitempty"`
}

type canonicalDocument struct {
	SchemaVersion int            `json:"schema_version"`
	Root          *canonicalNode `json:"root"`
	Exclusion     *canonicalNode `json:"exclusion,omitempty"`
}

func canonicalRule(schemaVersion int, root, exclusion *ValidatedNode) ([]byte, error) {
	document := canonicalDocument{SchemaVersion: schemaVersion, Root: canonicalise(root)}
	if exclusion != nil {
		document.Exclusion = canonicalise(exclusion)
	}
	return json.Marshal(document)
}

func canonicalise(node *ValidatedNode) *canonicalNode {
	if node == nil {
		return nil
	}
	if node.Condition == nil {
		out := &canonicalNode{Op: node.Op}
		for _, child := range node.Children {
			out.Children = append(out.Children, canonicalise(child))
		}
		return out
	}
	condition := node.Condition
	out := &canonicalNode{Field: condition.Field.Key, Operator: condition.Operator}
	switch condition.Operator {
	case OperatorExists, OperatorNotExists:
	case OperatorWithinLast:
		out.Value = map[string]any{"amount": condition.DurationAmount, "unit": condition.DurationUnit}
	case OperatorHasTag, OperatorNotHasTag:
		out.Value = map[string]any{"tag_id": condition.Strings[0]}
	case OperatorIN, OperatorNotIN:
		out.Value = condition.Strings
	case OperatorBETWEEN:
		out.Value = []any{canonicalScalar(condition, 0), canonicalScalar(condition, 1)}
	default:
		out.Value = canonicalScalar(condition, 0)
	}
	return out
}

// canonicalScalar renders one typed literal in exactly the shape the validator
// accepts, so a canonical document is always re-parsable. That round trip is
// what lets a stored definition be revalidated before every materialization
// instead of being trusted blind.
func canonicalScalar(condition *Condition, index int) any {
	switch condition.Field.Type {
	case FieldBoolean:
		if index < len(condition.Bools) {
			return condition.Bools[index]
		}
	case FieldInteger:
		if index < len(condition.Ints) {
			return condition.Ints[index]
		}
	case FieldMoneyMinor:
		if index < len(condition.Ints) && index < len(condition.Strings) {
			return map[string]any{"currency": condition.Strings[index], "minor": condition.Ints[index]}
		}
	case FieldTimestamp:
		if index < len(condition.Times) {
			return condition.Times[index].UTC().Format(time.RFC3339)
		}
	default:
		if index < len(condition.Strings) {
			return condition.Strings[index]
		}
	}
	return nil
}

// negatesGuardedField reports whether any availability-guarded condition sits
// under a NOT. Inside a NOT, SQL's three-valued logic turns "we do not know"
// into "not a match" rather than into a match, which is safe but can surprise
// an author who expected the negation to select the unknown rows — so callers
// surface this as a preview warning rather than an error.
func negatesGuardedField(node *ValidatedNode, negated bool) bool {
	if node == nil {
		return false
	}
	if node.Condition != nil {
		return negated && node.Condition.Field.AvailabilityGuard != ""
	}
	childNegated := negated
	if node.Op == OpNOT {
		childNegated = !negated
	}
	for _, child := range node.Children {
		if negatesGuardedField(child, childNegated) {
			return true
		}
	}
	return false
}

// walkConditions visits every condition in evaluation order, which is the order
// explanation paths are reported in.
func walkConditions(node *ValidatedNode, visit func(*Condition)) {
	if node == nil {
		return
	}
	if node.Condition != nil {
		visit(node.Condition)
		return
	}
	for _, child := range node.Children {
		walkConditions(child, visit)
	}
}

// DescribeRule renders a deterministic, human-readable summary of a rule from
// field and operator metadata. It is template-driven on purpose: an explanation
// a machine generated from the same structure it evaluates cannot drift from
// what the rule actually does.
func DescribeRule(rule *ValidatedRule) string {
	if rule == nil || rule.Root == nil {
		return ""
	}
	text := describeNode(rule.Root)
	if rule.Exclusion != nil {
		text += ", excluding customers where " + describeNode(rule.Exclusion)
	}
	return text
}

func describeNode(node *ValidatedNode) string {
	if node == nil {
		return ""
	}
	if node.Condition != nil {
		return node.Condition.Field.Explain(node.Condition)
	}
	if node.Op == OpNOT {
		return "NOT (" + describeNode(node.Children[0]) + ")"
	}
	parts := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		text := describeNode(child)
		if child.Condition == nil {
			text = "(" + text + ")"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " "+node.Op+" ")
}

func describeCondition(condition *Condition) string {
	label := condition.Field.Label
	switch condition.Operator {
	case OperatorExists:
		return label + " is known"
	case OperatorNotExists:
		return label + " is not known"
	case OperatorWithinLast:
		return label + " is within the last " + condition.DurationLabel
	case OperatorHasTag:
		return "customer is tagged " + condition.Strings[0]
	case OperatorNotHasTag:
		return "customer is not tagged " + condition.Strings[0]
	case OperatorIN:
		return label + " is one of " + strings.Join(condition.Strings, ", ")
	case OperatorNotIN:
		return label + " is none of " + strings.Join(condition.Strings, ", ")
	case OperatorBETWEEN:
		return label + " is between " + describeScalar(condition, 0) + " and " + describeScalar(condition, 1)
	}
	return label + " " + operatorPhrases[condition.Operator] + " " + describeScalar(condition, 0)
}

var operatorPhrases = map[string]string{
	OperatorEQ:     "is",
	OperatorNEQ:    "is not",
	OperatorGT:     "is greater than",
	OperatorGTE:    "is at least",
	OperatorLT:     "is less than",
	OperatorLTE:    "is at most",
	OperatorBefore: "is before",
	OperatorAfter:  "is after",
}

func describeScalar(condition *Condition, index int) string {
	switch condition.Field.Type {
	case FieldBoolean:
		if index < len(condition.Bools) && condition.Bools[index] {
			return "true"
		}
		return "false"
	case FieldInteger:
		if index < len(condition.Ints) {
			return strconv.FormatInt(condition.Ints[index], 10)
		}
	case FieldMoneyMinor:
		if index < len(condition.Ints) && index < len(condition.Strings) {
			return condition.Strings[index] + " " + strconv.FormatInt(condition.Ints[index], 10) + " (minor units)"
		}
	case FieldTimestamp:
		if index < len(condition.Times) {
			return condition.Times[index].UTC().Format(time.RFC3339)
		}
	default:
		if index < len(condition.Strings) {
			return condition.Strings[index]
		}
	}
	return ""
}

func plural(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.FormatInt(n, 10) + " " + unit + "s"
}
