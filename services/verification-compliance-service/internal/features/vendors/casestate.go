package vendors

import (
	"errors"
	"regexp"
	"time"
)

// Vendor case statuses. These are the exact stored values; a vendor case is
// never plain "PENDING".
const (
	StatusNotStarted    = "NOT_STARTED"
	StatusInProgress    = "IN_PROGRESS"
	StatusPendingReview = "PENDING_REVIEW"
	StatusVerified      = "VERIFIED"
	StatusRejected      = "REJECTED"
	StatusExpired       = "EXPIRED"
	StatusSuspended     = "SUSPENDED"
)

var (
	// ErrIllegalTransition is a move the case state machine does not have.
	ErrIllegalTransition = errors.New("illegal vendor case transition")
	// ErrReasonRequired: REJECTED and SUSPENDED must carry a reason code.
	ErrReasonRequired = errors.New("a reason code is required for this transition")
	// ErrFutureExpiryRequired: VERIFIED must carry an expiry in the future.
	ErrFutureExpiryRequired = errors.New("a future expiry is required to verify a case")
	// ErrAppealRequired: a closed case reopens only through a named appeal.
	ErrAppealRequired = errors.New("reopening a closed case requires an appeal reference")
	// ErrAppealNotApplicable: an appeal reference only accompanies a reopen.
	ErrAppealNotApplicable = errors.New("an appeal reference is only valid when reopening a closed case")
	// ErrInvalidReason: reason codes are bounded tokens, never free text.
	ErrInvalidReason = errors.New("reason code must be an upper-case token")
)

// legal is the whole case state machine. Anything absent is illegal.
var legal = map[string]map[string]bool{
	StatusNotStarted:    {StatusInProgress: true, StatusPendingReview: true},
	StatusPendingReview: {StatusInProgress: true, StatusVerified: true, StatusRejected: true},
	StatusInProgress:    {StatusPendingReview: true, StatusVerified: true, StatusRejected: true},
	StatusVerified:      {StatusSuspended: true, StatusExpired: true},
	StatusRejected:      {StatusPendingReview: true},
	StatusExpired:       {StatusPendingReview: true},
	StatusSuspended:     {StatusPendingReview: true},
}

var reasonToken = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
var appealToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)

// KnownStatus reports whether v is one of the seven stored statuses.
func KnownStatus(v string) bool { _, ok := legal[v]; return ok }

// IsReopen reports whether from->to reopens a closed case.
func IsReopen(from, to string) bool {
	return to == StatusPendingReview && (from == StatusRejected || from == StatusExpired || from == StatusSuspended)
}

// Transition is one requested move of a vendor case.
type Transition struct {
	From, To   string
	ReasonCode string
	ExpiresAt  *time.Time
	AppealRef  string
	Now        time.Time
}

// Validate is the single native state machine used by the signed decision
// route, the Secure Admin command executor and the expiry sweep. It is pure:
// it never reads the database or the clock (Now is supplied).
func (t Transition) Validate() error {
	next, ok := legal[t.From]
	if !ok || !next[t.To] {
		return ErrIllegalTransition
	}
	if t.ReasonCode != "" && !reasonToken.MatchString(t.ReasonCode) {
		return ErrInvalidReason
	}
	reopen := IsReopen(t.From, t.To)
	switch {
	case reopen && t.AppealRef == "":
		return ErrAppealRequired
	case reopen && !appealToken.MatchString(t.AppealRef):
		return ErrAppealRequired
	case !reopen && t.AppealRef != "":
		return ErrAppealNotApplicable
	}
	switch t.To {
	case StatusVerified:
		if t.ExpiresAt == nil || !t.ExpiresAt.After(t.Now) {
			return ErrFutureExpiryRequired
		}
	case StatusRejected, StatusSuspended:
		if t.ReasonCode == "" {
			return ErrReasonRequired
		}
	}
	return nil
}
