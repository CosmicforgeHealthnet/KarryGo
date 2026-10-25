// Package models holds the pricing policy domain: a lineage of rule books, its
// successive immutable versions, and the record of which version is live.
package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Geography levels, most specific first. Resolution walks this order and takes
// the first match, so a zone rate overrides a market rate, which overrides a
// national one.
const (
	LevelZone    = "ZONE"
	LevelMarket  = "MARKET"
	LevelState   = "STATE"
	LevelCountry = "COUNTRY"
)

// Version statuses.
const (
	StatusDraft      = "DRAFT"
	StatusSubmitted  = "SUBMITTED"
	StatusApproved   = "APPROVED"
	StatusRejected   = "REJECTED"
	StatusSuperseded = "SUPERSEDED"
)

// Activation actions.
const (
	ActionActivate = "ACTIVATE"
	ActionRollback = "ROLLBACK"
)

// Domain errors.
var (
	// ErrNotFound covers an unknown policy, version or activation.
	ErrNotFound = errors.New("policies: not found")
	// ErrNotDraft is returned when an edit is attempted on a version whose
	// numbers are already fixed.
	ErrNotDraft = errors.New("policies: only a draft may be edited")
	// ErrNotApproved is returned when activation is attempted on a version that
	// has not been approved.
	ErrNotApproved = errors.New("policies: only an approved version may be activated")
	// ErrStaleVersion refuses a command whose expected immutable body hash no
	// longer identifies the source version the operator reviewed.
	ErrStaleVersion = errors.New("policies: stale policy version")
	// ErrAmbiguous means two lineages claim the same kind and place. Resolution
	// must never guess.
	ErrAmbiguous = errors.New("policies: more than one policy claims this kind and geography")
	// ErrNoActivePolicy means nothing is live for this kind and place at this
	// moment. Pricing refuses rather than inventing a default rate.
	ErrNoActivePolicy = errors.New("policies: no active policy version")
)

// Policy is a lineage: one kind of rule book for one place.
type Policy struct {
	ID             string
	Kind           string
	GeographyLevel string
	GeographyRef   string
	CreatedBy      string
	CreatedAt      time.Time
}

// Version is one edition of a rule book. Its body is frozen once it leaves DRAFT.
type Version struct {
	ID            string
	PolicyID      string
	VersionNo     int
	Kind          string
	SchemaVersion int
	Status        string
	Body          []byte
	BodyHash      string
	ClonedFrom    string
	CreatedBy     string
	ChangeReason  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Activation records that a version became live from a moment.
//
// A rollback is a new activation pointing at an older approved version rather
// than a deletion, so the timeline only ever reads forward and "what was live
// last Tuesday" has exactly one answer.
type Activation struct {
	ID                string
	PolicyID          string
	VersionID         string
	EffectiveFrom     time.Time
	Action            string
	ActivatedBy       string
	ApprovalRequestID string
	Supersedes        string
	Reason            string
	CreatedAt         time.Time
}

// Resolution is the outcome of resolving one kind for one place and moment.
type Resolution struct {
	Policy     Policy
	Version    Version
	Activation Activation
}

// GeographyChain is the place a price is being resolved for, most specific first.
// Empty levels are skipped, so a caller that knows only the market can still
// resolve.
type GeographyChain struct {
	ZoneID      string
	MarketID    string
	StateID     string
	CountryCode string
}

// Candidates returns the (level, ref) pairs to try, most specific first.
func (g GeographyChain) Candidates() [][2]string {
	var candidates [][2]string
	if g.ZoneID != "" {
		candidates = append(candidates, [2]string{LevelZone, g.ZoneID})
	}
	if g.MarketID != "" {
		candidates = append(candidates, [2]string{LevelMarket, g.MarketID})
	}
	if g.StateID != "" {
		candidates = append(candidates, [2]string{LevelState, g.StateID})
	}
	if g.CountryCode != "" {
		candidates = append(candidates, [2]string{LevelCountry, g.CountryCode})
	}
	return candidates
}

// Validate checks the chain names at least one place.
func (g GeographyChain) Validate() error {
	if len(g.Candidates()) == 0 {
		return errors.New("policies: a geography chain must name at least a country")
	}
	return nil
}

// ValidLevel reports whether level is a known geography level.
func ValidLevel(level string) bool {
	switch level {
	case LevelZone, LevelMarket, LevelState, LevelCountry:
		return true
	}
	return false
}

// Specificity orders levels for resolution; lower is more specific.
func Specificity(level string) int {
	switch level {
	case LevelZone:
		return 1
	case LevelMarket:
		return 2
	case LevelState:
		return 3
	case LevelCountry:
		return 4
	default:
		return 99
	}
}

// ValidateDraft checks the fields an operator supplies when creating a draft.
func ValidateDraft(kind, level, ref, createdBy, changeReason string, schemaVersion int) error {
	var problems []string
	if strings.TrimSpace(kind) == "" {
		problems = append(problems, "kind is required")
	}
	if !ValidLevel(level) {
		problems = append(problems, fmt.Sprintf("geography_level %q is unknown", level))
	}
	if strings.TrimSpace(ref) == "" {
		problems = append(problems, "geography_ref is required")
	}
	if strings.TrimSpace(createdBy) == "" {
		problems = append(problems, "created_by is required")
	}
	if strings.TrimSpace(changeReason) == "" {
		problems = append(problems, "a change reason is required: a rate change with no stated purpose cannot be reviewed later")
	}
	if schemaVersion < 1 {
		problems = append(problems, "schema_version must be at least 1")
	}
	if len(problems) > 0 {
		return fmt.Errorf("policies: %s", strings.Join(problems, "; "))
	}
	return nil
}
