// Package campaignmodels holds the campaign fulfilment vocabulary and pure
// rules (Section 17). Nothing here performs I/O.
package campaignmodels

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincampaign"
)

// Campaign revision states (admincampaign.States). DELIVERED is never
// produced: no provider delivery receipts exist.
const (
	StateAccepted   = "ACCEPTED_BY_KARRYGO"
	StatePlanned    = "PLANNED"
	StateQueued     = "QUEUED"
	StateSent       = "SENT"
	StateCompleted  = "COMPLETED"
	StateRejected   = "REJECTED"
	StateFailed     = "FAILED"
	StateSuppressed = "SUPPRESSED"
	StateCancelled  = "CANCELLED"
)

// Recipient states.
const (
	RecipientPlanned    = "PLANNED"
	RecipientQueued     = "QUEUED"
	RecipientSent       = "SENT"
	RecipientLoggedOnly = "LOGGED_ONLY"
	RecipientFailed     = "FAILED"
	RecipientSuppressed = "SUPPRESSED"
	RecipientCancelled  = "CANCELLED"
)

// Rejection and suppression reason codes.
const (
	RejectTemplateMismatch     = "TEMPLATE_MISMATCH"
	RejectAudienceDimension    = "AUDIENCE_DIMENSION_UNAVAILABLE"
	RejectCategoryNotPermitted = "CATEGORY_NOT_PERMITTED"
	RejectWindowClosed         = "SEND_WINDOW_CLOSED"

	SuppressConsentWithdrawn = "CONSENT_WITHDRAWN"
	SuppressAccountInactive  = "ACCOUNT_INACTIVE"
	SuppressNoContact        = "NO_CONTACT_ADDRESS"
	SuppressMarketChanged    = "MARKET_CHANGED"
	SuppressFrequencyCap     = "FREQUENCY_CAP"
	SuppressCampaignCap      = "CAMPAIGN_CAP"
	SuppressChannelDisabled  = "CHANNEL_DISABLED"
	SuppressNoDevice         = "NO_ACTIVE_DEVICE"
	SuppressWindowClosed     = "SEND_WINDOW_CLOSED"
	SuppressUnknown          = "ELIGIBILITY_UNKNOWN"

	DeliveryConfirmationUnavailable = "UNAVAILABLE"
	ConversionUnavailable           = "UNAVAILABLE"
)

// EventType is the message pipeline event type of every campaign message.
const EventType = "campaign.marketing"

// SourceService is the source_service recorded on campaign messages.
const SourceService = "notification-service"

// RecipientType is the only audience recipient type (CUSTOMERS_OPTED_IN).
const RecipientType = "customer"

// ActiveStates are revisions the engine still works on.
var ActiveStates = []string{StateAccepted, StatePlanned, StateQueued, StateSent}

// Terminal reports whether a revision state can no longer change.
func Terminal(state string) bool {
	switch state {
	case StateCompleted, StateRejected, StateFailed, StateSuppressed, StateCancelled:
		return true
	}
	return false
}

// ContentSHA256 is the content hash an intent binds: sha256(title NUL body).
func ContentSHA256(title, body string) string {
	sum := sha256.Sum256([]byte(title + "\x00" + body))
	return hex.EncodeToString(sum[:])
}

// MessageIdempotencyKey is the pipeline key of one recipient on one channel.
func MessageIdempotencyKey(campaignID string, revision int64, recipient, channel string) string {
	return "campaign:" + campaignID + ":" + strconv.FormatInt(revision, 10) + ":" + recipient + ":" + channel
}

// Counts are the truthful per-revision tallies.
type Counts struct {
	Audience   int64
	Eligible   int64
	Suppressed int64
	Queued     int64
	Sent       int64
	Failed     int64
	LoggedOnly int64
	Planned    int64
	Cancelled  int64
	Reasons    map[string]int64
}

// Revision is the persisted view of one revision.
type Revision struct {
	CampaignID     string
	Revision       int64
	Intent         admincampaign.Intent
	IntentDigest   string
	IdempotencyKey string
	State          string
	RejectionCode  *string
	ContentTitle   *string
	ContentBody    *string
	PlanCursor     PlanCursor
	PlannedAt      *time.Time
	Counts         Counts
	Version        int64
	UpdatedAt      time.Time
}

// Status renders the contract status. Delivery and conversion facts are
// UNAVAILABLE and delivered_count is null: nothing receipts a delivery.
func (r Revision) Status(asOf time.Time) admincampaign.Status {
	reasons := map[string]int64{}
	for k, v := range r.Counts.Reasons {
		reasons[k] = v
	}
	s := admincampaign.Status{
		Contract: admincampaign.Contract, CampaignID: r.CampaignID, Revision: r.Revision, IntentDigest: r.IntentDigest,
		State: r.State, AudienceCount: r.Counts.Audience, EligibleCount: r.Counts.Eligible, SuppressedCount: r.Counts.Suppressed,
		SuppressionReasons: reasons, QueuedCount: r.Counts.Queued, SentCount: r.Counts.Sent, FailedCount: r.Counts.Failed, LoggedOnlyCount: r.Counts.LoggedOnly,
		DeliveredCount: nil, DeliveryConfirmation: DeliveryConfirmationUnavailable, ConversionReporting: ConversionUnavailable,
		AsOf: asOf.UTC(),
	}
	if r.RejectionCode != nil {
		s.RejectionCode = *r.RejectionCode
	}
	return s
}

// PlanCursor tracks audience paging across worker ticks. Pairs are walked in
// sorted (market, channel) order.
type PlanCursor struct {
	Pair   int    `json:"pair"`
	Cursor string `json:"cursor"`
}

// Pair is one (market, channel) audience query.
type Pair struct{ Market, Channel string }

// Pairs returns the deterministic audience query order of an intent.
func Pairs(i admincampaign.Intent) []Pair {
	markets := append([]string{}, i.Markets...)
	channels := append([]string{}, i.Channels...)
	sort.Strings(markets)
	sort.Strings(channels)
	out := make([]Pair, 0, len(markets)*len(channels))
	seen := map[Pair]bool{}
	for _, m := range markets {
		for _, c := range channels {
			p := Pair{m, c}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// ResolveState derives the revision state from its recipient counts once
// planning has finished. It never returns DELIVERED.
func ResolveState(current string, c Counts) string {
	if Terminal(current) {
		return current
	}
	switch {
	case c.Planned > 0 && c.Queued+c.Sent+c.Failed+c.LoggedOnly == 0:
		return StatePlanned
	case c.Planned > 0:
		return StateQueued
	case c.Queued > 0 && c.Sent > 0:
		return StateSent
	case c.Queued > 0:
		return StateQueued
	case c.Sent > 0:
		return StateCompleted
	case c.Failed > 0 && c.LoggedOnly == 0:
		return StateFailed
	case c.LoggedOnly > 0:
		return StateCompleted
	case c.Audience > 0 && c.Suppressed+c.Cancelled == c.Audience && c.Cancelled < c.Audience:
		return StateSuppressed
	default:
		return StateCompleted
	}
}

// QuietDeferral reports whether t falls inside the quiet window and, if so,
// when the window ends. The window is evaluated in the intent's IANA zone,
// so it follows that zone's local clock (including DST) and may span
// midnight. Without quiet hours nothing is deferred: no default is invented.
func QuietDeferral(q *admincampaign.QuietHours, t time.Time) (bool, time.Time) {
	if q == nil {
		return false, time.Time{}
	}
	loc, err := time.LoadLocation(q.Timezone)
	if err != nil {
		// An unloadable zone cannot be evaluated; defer an hour rather than
		// risk sending inside the customer's quiet period.
		return true, t.Add(time.Hour)
	}
	start, okS := clock(q.Start)
	end, okE := clock(q.End)
	if !okS || !okE {
		return true, t.Add(time.Hour)
	}
	local := t.In(loc)
	minute := local.Hour()*60 + local.Minute()
	inside := false
	if start < end {
		inside = minute >= start && minute < end
	} else {
		inside = minute >= start || minute < end
	}
	if !inside {
		return false, time.Time{}
	}
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	resume := day.Add(time.Duration(end) * time.Minute)
	if !resume.After(local) {
		resume = time.Date(local.Year(), local.Month(), local.Day()+1, end/60, end%60, 0, 0, loc)
	}
	return true, resume.UTC()
}

func clock(v string) (int, bool) {
	parts := strings.Split(v, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, false
	}
	h := int(parts[0][0]-'0')*10 + int(parts[0][1]-'0')
	m := int(parts[1][0]-'0')*10 + int(parts[1][1]-'0')
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
