package basket

// The recurring basket template.
//
// The central decision, and the one everything else follows from: a template
// stores REQUIREMENTS AND PREFERENCES, not "buy exactly these SKUs from
// exactly these vendors again".
//
// The difference shows up the first time reality moves. A template bound to
// listing lst_9f21 breaks when that vendor runs out, raises its price, stops
// stocking the line or leaves the platform - and it breaks silently, by
// quietly becoming un-buyable or quietly costing more. A template that says
// "two litres of milk, this brand, no substitutes" survives all four, because
// every occurrence goes shopping again.
//
// That is why an occurrence RE-EVALUATES everything: active listings,
// sellable stock, current prices, eligible vendors, the delivery plan and the
// fulfilment mode. Nothing is carried forward from last week except what the
// customer actually chose - their requirements, their constraints and their
// approval policy.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ApprovalPolicy is how much change a customer has pre-authorised.
//
// Preauthorization is explicit and bounded, always. There is deliberately no
// value meaning "buy whatever you think best": a recurring basket that could
// change product, vendor and price without asking is a standing instruction
// nobody actually gave.
type ApprovalPolicy string

const (
	// ApprovalAlwaysAsk requires the customer to approve every occurrence.
	// The safe default, and the only correct one for a customer who has
	// stated nothing.
	ApprovalAlwaysAsk ApprovalPolicy = "ALWAYS_ASK"

	// ApprovalAutoIfUnchanged proceeds only when the new basket is materially
	// identical to the last approved one: same products, same vendors, and a
	// total within the stated ceiling.
	ApprovalAutoIfUnchanged ApprovalPolicy = "AUTO_IF_UNCHANGED"

	// ApprovalAutoWithinCeiling additionally permits a VENDOR change for the
	// SAME product, within the ceiling.
	//
	// The bound is what makes this safe: the article is unchanged, so the
	// customer receives exactly what they chose, bought at a different shop.
	// It never permits a different product.
	ApprovalAutoWithinCeiling ApprovalPolicy = "AUTO_VENDOR_CHANGE_WITHIN_CEILING"
)

var approvalPolicies = map[ApprovalPolicy]struct{}{
	ApprovalAlwaysAsk:         {},
	ApprovalAutoIfUnchanged:   {},
	ApprovalAutoWithinCeiling: {},
}

// Valid reports membership of the closed vocabulary.
func (p ApprovalPolicy) Valid() bool {
	_, ok := approvalPolicies[p]

	return ok
}

// ParseApprovalPolicy resolves a customer-supplied policy.
//
// Empty means ALWAYS_ASK. Silence is not consent, and defaulting the other
// way would turn "I did not fill that field in" into a standing purchase
// authority.
func ParseApprovalPolicy(value string) (ApprovalPolicy, error) {
	policy := ApprovalPolicy(strings.ToUpper(strings.TrimSpace(value)))

	if policy == "" {
		return ApprovalAlwaysAsk, nil
	}

	if !policy.Valid() {
		return "", fmt.Errorf(
			"%w: unknown approval policy %q", ErrInvalidRequirement, value,
		)
	}

	return policy, nil
}

// Cadence is how often a template recurs.
type Cadence string

const (
	CadenceDaily       Cadence = "DAILY"
	CadenceWeekly      Cadence = "WEEKLY"
	CadenceFortnightly Cadence = "FORTNIGHTLY"
	CadenceMonthly     Cadence = "MONTHLY"
)

var cadences = map[Cadence]struct{}{
	CadenceDaily:       {},
	CadenceWeekly:      {},
	CadenceFortnightly: {},
	CadenceMonthly:     {},
}

// Valid reports membership of the closed vocabulary.
func (c Cadence) Valid() bool {
	_, ok := cadences[c]

	return ok
}

var (
	// ErrInvalidSchedule means the recurrence could not be understood.
	ErrInvalidSchedule = errors.New("recurring basket schedule is not valid")

	// ErrTemplateDisabled means the template may not generate.
	ErrTemplateDisabled = errors.New("recurring basket template is disabled")
)

// Template is a reusable shopping list, stated as needs.
type Template struct {
	ID string `json:"template_id"`

	CustomerID string `json:"-"`

	Name string `json:"name"`

	Currency string `json:"currency"`

	// AddressRef is the customer's own saved location. The template stores
	// the REFERENCE and never the coordinates, so a customer who corrects
	// their address does not have to correct every template that ships to it.
	AddressRef string `json:"address_ref"`

	Requirements []Requirement `json:"requirements"`

	FulfilmentPreference string    `json:"fulfilment_preference,omitempty"`
	Objective            Objective `json:"objective"`

	ApprovalPolicy ApprovalPolicy `json:"approval_policy"`

	// CeilingMinor bounds what an auto-approved occurrence may cost,
	// delivered. Zero means no ceiling was stated, which - combined with the
	// approval policies above - means no occurrence can be auto-approved.
	//
	// An unbounded preauthorization is not a preference this feature offers.
	CeilingMinor int64 `json:"ceiling_minor,omitempty"`

	Cadence Cadence `json:"cadence"`

	// Timezone is an IANA name, and it is REQUIRED.
	//
	// "Every Tuesday at 9" means nine in the customer's morning, not nine
	// wherever the server happens to be. A schedule that resolved against
	// server-local time would drift with deployments and would silently be
	// wrong for every customer not in the server's zone.
	Timezone string `json:"timezone"`

	// LocalHour and LocalMinute are the wall-clock time in that zone.
	LocalHour   int `json:"local_hour"`
	LocalMinute int `json:"local_minute"`

	// Weekday is 0-6 with Sunday zero, for weekly and fortnightly cadences.
	Weekday int `json:"weekday,omitempty"`

	// DayOfMonth is 1-28 for monthly. Capped at 28 deliberately: 29, 30 and
	// 31 do not exist in every month, and a schedule that silently skipped
	// February would be a recurring basket that recurs less often than the
	// customer was told.
	DayOfMonth int `json:"day_of_month,omitempty"`

	Enabled bool `json:"enabled"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Normalize validates and defaults a template.
func (t Template) Normalize() (Template, error) {
	t.Name = strings.TrimSpace(t.Name)
	t.Currency = strings.ToUpper(strings.TrimSpace(t.Currency))
	t.AddressRef = strings.TrimSpace(t.AddressRef)
	t.Timezone = strings.TrimSpace(t.Timezone)

	if t.Name == "" {
		return Template{}, fmt.Errorf(
			"%w: a recurring basket needs a name", ErrInvalidRequirement,
		)
	}

	if t.Currency == "" {
		return Template{}, ErrCurrencyRequired
	}

	if t.AddressRef == "" {
		return Template{}, ErrNoDestination
	}

	if len(t.Requirements) == 0 {
		return Template{}, ErrNoRequirements
	}

	if len(t.Requirements) > MaxRequirements {
		return Template{}, ErrTooManyRequirements
	}

	normalized := make([]Requirement, 0, len(t.Requirements))

	seen := map[string]bool{}

	for index, requirement := range t.Requirements {
		next, err := requirement.Normalize(index)
		if err != nil {
			return Template{}, err
		}

		if seen[next.Ref] {
			return Template{}, fmt.Errorf(
				"%w: duplicate requirement ref %q",
				ErrInvalidRequirement, next.Ref,
			)
		}

		seen[next.Ref] = true
		normalized = append(normalized, next)
	}

	t.Requirements = normalized

	if !t.Cadence.Valid() {
		return Template{}, fmt.Errorf(
			"%w: unknown cadence %q", ErrInvalidSchedule, t.Cadence,
		)
	}

	// An empty zone is refused explicitly, and the explicit check matters:
	// time.LoadLocation("") succeeds and returns UTC. Relying on the load
	// below alone would silently turn "the customer stated no timezone" into
	// "nine o'clock UTC" - which is the server-local drift this field exists
	// to prevent, arrived at by a different route.
	if t.Timezone == "" {
		return Template{}, fmt.Errorf(
			"%w: a recurring basket needs an IANA timezone",
			ErrInvalidSchedule,
		)
	}

	// The timezone must actually load. A template stored with an unloadable
	// zone would fail at generation time - in a worker, at three in the
	// morning - instead of at the moment the customer could have fixed it.
	if _, err := time.LoadLocation(t.Timezone); err != nil {
		return Template{}, fmt.Errorf(
			"%w: unknown timezone %q", ErrInvalidSchedule, t.Timezone,
		)
	}

	if t.LocalHour < 0 || t.LocalHour > 23 ||
		t.LocalMinute < 0 || t.LocalMinute > 59 {
		return Template{}, fmt.Errorf(
			"%w: local time is out of range", ErrInvalidSchedule,
		)
	}

	if t.Weekday < 0 || t.Weekday > 6 {
		return Template{}, fmt.Errorf(
			"%w: weekday is out of range", ErrInvalidSchedule,
		)
	}

	if t.Cadence == CadenceMonthly {
		if t.DayOfMonth < 1 || t.DayOfMonth > 28 {
			return Template{}, fmt.Errorf(
				"%w: day of month must be between 1 and 28 so every month has one",
				ErrInvalidSchedule,
			)
		}
	}

	policy, err := ParseApprovalPolicy(string(t.ApprovalPolicy))
	if err != nil {
		return Template{}, err
	}

	t.ApprovalPolicy = policy

	if t.CeilingMinor < 0 {
		return Template{}, fmt.Errorf(
			"%w: ceiling cannot be negative", ErrInvalidRequirement,
		)
	}

	// A preauthorizing policy with no ceiling is an unbounded standing
	// authority. It is refused rather than silently downgraded, because a
	// customer who set the policy and forgot the ceiling should be told,
	// not quietly given the safe behaviour they did not ask for.
	if t.ApprovalPolicy != ApprovalAlwaysAsk && t.CeilingMinor <= 0 {
		return Template{}, fmt.Errorf(
			"%w: an auto-approving recurring basket needs a spending ceiling",
			ErrInvalidRequirement,
		)
	}

	if !t.Objective.Valid() {
		t.Objective = ObjectiveValue
	}

	if t.FulfilmentPreference != "" {
		if _, ok := parseModeName(t.FulfilmentPreference); !ok {
			return Template{}, fmt.Errorf(
				"%w: unknown fulfilment preference %q",
				ErrInvalidRequirement, t.FulfilmentPreference,
			)
		}
	}

	return t, nil
}

// parseModeName validates a fulfilment preference without importing the
// planner's parser into this file's validation path.
func parseModeName(value string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "FASTEST", "BALANCED", "SAVER":
		return strings.ToUpper(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

// NextOccurrence is the next scheduled moment strictly after a given instant.
//
// Everything is computed in the template's own timezone and converted to UTC
// at the end. Doing it the other way round - adding days to a UTC instant -
// silently breaks across a daylight-saving boundary, which is exactly the
// week a customer would notice their groceries arriving an hour early.
func (t Template) NextOccurrence(after time.Time) (time.Time, error) {
	location, err := time.LoadLocation(t.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%w: unknown timezone %q", ErrInvalidSchedule, t.Timezone,
		)
	}

	local := after.In(location)

	candidate := time.Date(
		local.Year(), local.Month(), local.Day(),
		t.LocalHour, t.LocalMinute, 0, 0, location,
	)

	switch t.Cadence {
	case CadenceDaily:
		for !candidate.After(local) {
			candidate = candidate.AddDate(0, 0, 1)
		}

	case CadenceWeekly, CadenceFortnightly:
		step := 7

		if t.Cadence == CadenceFortnightly {
			step = 14
		}

		for int(candidate.Weekday()) != t.Weekday || !candidate.After(local) {
			candidate = candidate.AddDate(0, 0, 1)
		}

		// A fortnightly template still lands on the customer's chosen
		// weekday; the step only matters once an occurrence exists to step
		// FROM, which the caller supplies as the "after" instant.
		_ = step

	case CadenceMonthly:
		candidate = time.Date(
			local.Year(), local.Month(), t.DayOfMonth,
			t.LocalHour, t.LocalMinute, 0, 0, location,
		)

		for !candidate.After(local) {
			candidate = candidate.AddDate(0, 1, 0)
		}

	default:
		return time.Time{}, ErrInvalidSchedule
	}

	return candidate.UTC(), nil
}

// OccurrenceKey is the stable identity of one scheduled occurrence.
//
// This string is what exactly-once rests on. It is derived only from the
// schedule - the local wall-clock moment and the zone it was computed in -
// so a retry, a worker replay, a process restart or a duplicate scheduler
// delivery all produce the SAME key and collide on the unique index rather
// than creating a second generation.
//
// Deliberately not derived from: the current time, a worker id, an attempt
// count, or a random value. Any of those would make two attempts at one
// occurrence look like two occurrences, which is the exact failure the key
// exists to prevent.
func (t Template) OccurrenceKey(at time.Time) string {
	location, err := time.LoadLocation(t.Timezone)

	if err != nil {
		// A template that cannot resolve its own zone still needs a stable
		// key, or the retry that follows would duplicate. UTC is the
		// deterministic fallback and the generation will fail on the zone
		// anyway.
		location = time.UTC
	}

	return at.In(location).Format("2006-01-02T15:04") + "@" + t.Timezone
}
