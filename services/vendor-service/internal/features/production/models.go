package production

import (
	"regexp"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Vocabularies
// ---------------------------------------------------------------------------

// Capabilities this platform serves. CLOUD_KITCHEN and PASTRY_BAKERY are the
// two Sections 10/11 own; RESTAURANT and FAST_FOOD are accepted so a Section 9
// kitchen can adopt production capacity later without a destructive migration.
var Capabilities = []string{"RESTAURANT", "FAST_FOOD", "CLOUD_KITCHEN", "PASTRY_BAKERY"}

var (
	ScopeTypes        = []string{"BRANCH", "CAPABILITY", "PRODUCTION_LINE", "ITEM_FAMILY", "CUSTOM_CLASS"}
	UnitTypes         = []string{"MEAL", "ITEM", "TRAY", "PACKAGE", "PRODUCTION_UNIT", "WORKLOAD_UNIT"}
	ProductionClasses = []string{"GENERAL", "HOT", "COLD", "BAKERY", "DECORATION", "PACKING", "CHILLING"}
	BlackoutReasons   = []string{"MAINTENANCE", "STAFF_SHORTAGE", "PRIVATE_PRODUCTION",
		"HOLIDAY", "EQUIPMENT_FAILURE", "CLOSURE", "OTHER"}
	SourceTypes = []string{"MANUAL", "BULK_PACKAGE", "CORPORATE_OCCURRENCE", "PASTRY_PRODUCT",
		"CUSTOM_CAKE_QUOTE", "PRODUCTION_JOB", "EXTERNAL_ORDER"}
	Priorities = []string{"NORMAL", "HIGH", "URGENT"}
)

// QueuePriorityRank is the stable operational ordering used by the kitchen
// queue.  Promised customer-ready time is always compared before this rank.
func QueuePriorityRank(priority string) int {
	switch strings.ToUpper(strings.TrimSpace(priority)) {
	case "URGENT":
		return 0
	case "HIGH":
		return 1
	default:
		return 2
	}
}

// ProductionCapabilityForOrderDomain maps the frozen commercial domain to a
// production capability without consulting mutable catalogue state. Grocery
// Shop allocations intentionally return false: they are not kitchen work.
func ProductionCapabilityForOrderDomain(domain string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(domain)) {
	case "FOOD", "RESTAURANT":
		return "RESTAURANT", true
	case "FAST_FOOD":
		return "FAST_FOOD", true
	case "CLOUD_KITCHEN":
		return "CLOUD_KITCHEN", true
	case "PASTRY_BAKERY":
		return "PASTRY_BAKERY", true
	default:
		return "", false
	}
}

type QueueCursor struct {
	PromisedReadyAt *time.Time
	PriorityRank    int
	CreatedAt       time.Time
	ID              string
}

// QueueCursorEnvelope is the wire-safe representation of the last item in a
// production queue page.  The cursor is opaque to callers; all ordering keys
// are validated before they are used in SQL.
type QueueCursorEnvelope struct {
	PromisedReadyAt *time.Time `json:"promised_ready_at,omitempty"`
	PriorityRank    int        `json:"priority_rank"`
	CreatedAt       time.Time  `json:"created_at"`
	ID              string     `json:"id"`
}

type QueueItem struct {
	ID                       string     `json:"id"`
	BranchID                 string     `json:"branch_id"`
	Capability               string     `json:"capability"`
	SourceType               string     `json:"source_type"`
	SourceRef                *string    `json:"source_ref,omitempty"`
	SourceVersion            *int64     `json:"source_version,omitempty"`
	Title                    string     `json:"title"`
	Status                   string     `json:"status"`
	Priority                 string     `json:"priority"`
	PromisedReadyAt          *time.Time `json:"promised_ready_at,omitempty"`
	ExpectedReadyWindowStart *time.Time `json:"expected_ready_window_start,omitempty"`
	ExpectedReadyWindowEnd   *time.Time `json:"expected_ready_window_end,omitempty"`
	ScheduledStart           *time.Time `json:"scheduled_start,omitempty"`
	ScheduledEnd             *time.Time `json:"scheduled_end,omitempty"`
	ReadyAt                  *time.Time `json:"ready_at,omitempty"`
	PlannedQuantity          int        `json:"planned_quantity"`
	ProducedQuantity         int        `json:"produced_quantity"`
	WasteQuantity            int        `json:"waste_quantity"`
	BlockedReason            string     `json:"blocked_reason,omitempty"`
	StepCount                int        `json:"step_count"`
	CompletedStepCount       int        `json:"completed_step_count"`
	BlockingStepCount        int        `json:"blocking_step_count"`
	Version                  int64      `json:"version"`
	CreatedAt                time.Time  `json:"created_at"`
}

// Reservation lifecycle. HELD and CONFIRMED consume capacity; the rest do not.
const (
	ReservationHeld      = "HELD"
	ReservationConfirmed = "CONFIRMED"
	ReservationReleased  = "RELEASED"
	ReservationCancelled = "CANCELLED"
	ReservationExpired   = "EXPIRED"
)

// Production job lifecycle.
const (
	JobPlanned    = "PLANNED"
	JobQueued     = "QUEUED"
	JobInProgress = "IN_PROGRESS"
	JobBlocked    = "BLOCKED"
	JobReady      = "READY"
	JobCompleted  = "COMPLETED"
	JobCancelled  = "CANCELLED"
)

const (
	StatusActive   = "ACTIVE"
	StatusPaused   = "PAUSED"
	StatusArchived = "ARCHIVED"
)

// Bounds. Each is also a database CHECK, so a direct writer cannot bypass them.
const (
	MaxCapacity        = 1000000
	MaxBufferMinutes   = 480
	MaxPoolsPerRequest = 16
	MaxWindowDays      = 366
)

var codePattern = regexp.MustCompile(`^[A-Z0-9_]{2,60}$`)

func inVocabulary(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeName(value string) string { return strings.Join(strings.Fields(value), " ") }

func validateName(field, value string, minLen, maxLen int) (string, *apperrors.FieldViolation) {
	name := normalizeName(value)
	if len(name) < minLen || len(name) > maxLen {
		return "", &apperrors.FieldViolation{Field: field,
			Message: "Name must be between " + itoa(minLen) + " and " + itoa(maxLen) + " characters."}
	}
	return name, nil
}

func validateCode(field, value string) (string, *apperrors.FieldViolation) {
	code := strings.ToUpper(strings.TrimSpace(value))
	if !codePattern.MatchString(code) {
		return "", &apperrors.FieldViolation{Field: field,
			Message: "Code must be 2-60 characters of A-Z, 0-9 or underscore."}
	}
	return code, nil
}

func validateCapacity(field string, value int) *apperrors.FieldViolation {
	if value < 0 || value > MaxCapacity {
		return &apperrors.FieldViolation{Field: field,
			Message: "Capacity must be between 0 and " + itoa(MaxCapacity) + "."}
	}
	return nil
}

// validateWindow rejects the two window shapes that would otherwise silently
// misbehave: a zero-or-negative interval, and one so long it would sweep in
// unrelated production.
func validateWindow(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "window", Message: "A start and end time are required."}}), CodeInvalidWindow)
	}
	if !end.After(start) {
		return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "window_end", Message: "The window must end after it starts."}}), CodeInvalidWindow)
	}
	if end.Sub(start) > time.Duration(MaxWindowDays)*24*time.Hour {
		return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "window", Message: "The window is too long."}}), CodeInvalidWindow)
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// ---------------------------------------------------------------------------
// Domain models
// ---------------------------------------------------------------------------

// CapacityPool is one constrained production resource.
type CapacityPool struct {
	ID                   string
	BusinessID           string
	BranchID             string
	Capability           string
	Name                 string
	Code                 string
	ScopeType            string
	ScopeRef             *string
	UnitType             string
	DefaultCapacity      int
	SetupBufferMinutes   int
	CleanupBufferMinutes int
	Status               string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ArchivedAt           *time.Time
	Version              int64
}

type CapacityPoolView struct {
	ID                   string     `json:"id"`
	BranchID             string     `json:"branch_id"`
	Capability           string     `json:"capability"`
	Name                 string     `json:"name"`
	Code                 string     `json:"code"`
	ScopeType            string     `json:"scope_type"`
	ScopeRef             *string    `json:"scope_ref"`
	UnitType             string     `json:"unit_type"`
	DefaultCapacity      int        `json:"default_capacity"`
	SetupBufferMinutes   int        `json:"setup_buffer_minutes"`
	CleanupBufferMinutes int        `json:"cleanup_buffer_minutes"`
	Status               string     `json:"status"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
	Version              int64      `json:"version"`
}

func CapacityPoolViewOf(p CapacityPool) CapacityPoolView {
	return CapacityPoolView{ID: p.ID, BranchID: p.BranchID, Capability: p.Capability,
		Name: p.Name, Code: p.Code, ScopeType: p.ScopeType, ScopeRef: p.ScopeRef,
		UnitType: p.UnitType, DefaultCapacity: p.DefaultCapacity,
		SetupBufferMinutes: p.SetupBufferMinutes, CleanupBufferMinutes: p.CleanupBufferMinutes,
		Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		ArchivedAt: p.ArchivedAt, Version: p.Version}
}

// Reservation is a committed claim on production capacity. It is not an order.
type Reservation struct {
	ID            string
	BusinessID    string
	BranchID      string
	Capability    string
	WindowStart   time.Time
	WindowEnd     time.Time
	SourceType    string
	SourceRef     *string
	Status        string
	HoldExpiresAt *time.Time
	Priority      string
	Note          string
	CommandRef    *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int64
}

// Consuming reports whether this reservation is currently taking capacity.
//
// An expired hold is treated as not consuming from the authoritative clock,
// without waiting for any worker to rewrite its status.
func (r Reservation) Consuming(at time.Time) bool {
	switch r.Status {
	case ReservationConfirmed:
		return true
	case ReservationHeld:
		return r.HoldExpiresAt == nil || r.HoldExpiresAt.After(at)
	default:
		return false
	}
}

type AllocationView struct {
	CapacityPoolID   string    `json:"capacity_pool_id"`
	ProductionLineID string    `json:"production_line_id,omitempty"`
	Quantity         int       `json:"quantity"`
	ReleasedQuantity int       `json:"released_quantity"`
	EffectiveStart   time.Time `json:"effective_start"`
	EffectiveEnd     time.Time `json:"effective_end"`
}

type ReservationView struct {
	ID            string           `json:"id"`
	BranchID      string           `json:"branch_id"`
	Capability    string           `json:"capability"`
	WindowStart   time.Time        `json:"window_start"`
	WindowEnd     time.Time        `json:"window_end"`
	SourceType    string           `json:"source_type"`
	SourceRef     *string          `json:"source_ref"`
	Status        string           `json:"status"`
	Consuming     bool             `json:"consuming_capacity"`
	HoldExpiresAt *time.Time       `json:"hold_expires_at"`
	Priority      string           `json:"priority"`
	Allocations   []AllocationView `json:"allocations"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	Version       int64            `json:"version"`
}

func ReservationViewOf(r Reservation, allocations []AllocationView, at time.Time) ReservationView {
	if allocations == nil {
		allocations = []AllocationView{}
	}
	return ReservationView{ID: r.ID, BranchID: r.BranchID, Capability: r.Capability,
		WindowStart: r.WindowStart, WindowEnd: r.WindowEnd, SourceType: r.SourceType,
		SourceRef: r.SourceRef, Status: r.Status, Consuming: r.Consuming(at),
		HoldExpiresAt: r.HoldExpiresAt, Priority: r.Priority, Allocations: allocations,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version}
}

// PoolAvailability explains one pool's position for a requested window.
type PoolAvailability struct {
	CapacityPoolID string `json:"capacity_pool_id"`
	Code           string `json:"code"`
	UnitType       string `json:"unit_type"`
	Maximum        int    `json:"maximum"`
	Confirmed      int    `json:"confirmed"`
	Held           int    `json:"held"`
	Remaining      int    `json:"remaining"`
	Requested      int    `json:"requested"`
	Sufficient     bool   `json:"sufficient"`
	// BlockingRuleRef names the rule that produced the binding maximum, when a
	// rule rather than the pool default did.
	BlockingRuleRef string `json:"blocking_rule_ref,omitempty"`
	BlackoutRef     string `json:"blackout_ref,omitempty"`
}

// CapacityPreview is the non-binding answer to "could this be reserved".
//
// A preview never guarantees a reservation: the mutation rechecks every pool
// under lock, because between a preview and a request somebody else may have
// taken the capacity.
type CapacityPreview struct {
	WindowStart      time.Time          `json:"window_start"`
	WindowEnd        time.Time          `json:"window_end"`
	Available        bool               `json:"available"`
	Pools            []PoolAvailability `json:"pools"`
	BlockingPool     string             `json:"blocking_pool_ref,omitempty"`
	BlockingReason   string             `json:"blocking_reason,omitempty"`
	CalendarRevision int64              `json:"calendar_revision"`
	EvaluatedAt      time.Time          `json:"evaluated_at"`
}

// PoolUtilization is one pool's load over a reporting window.
type PoolUtilization struct {
	CapacityPoolID string `json:"capacity_pool_id"`
	Code           string `json:"code"`
	UnitType       string `json:"unit_type"`
	Maximum        int    `json:"maximum"`
	Confirmed      int    `json:"confirmed"`
	Held           int    `json:"held"`
	Remaining      int    `json:"remaining"`
	// Basis points rather than a float, so the number is exact and no rounding
	// decision is hidden inside a percentage.
	UtilizationBasisPoints int `json:"utilization_basis_points"`
}
