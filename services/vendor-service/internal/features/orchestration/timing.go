package orchestration

import (
	"time"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
)

// Just-in-time commit timing.
//
// The number this file computes -- commit_not_before -- decides when a provider
// is asked to come. Too early and they wait outside a kitchen that has not
// started; too late and the food sits cooling while nobody has been dispatched.
//
// It is explicitly NOT a one-shot alarm. It is recalculated on every evaluation
// from the current readiness prediction and the current supply estimate,
// because both move: a kitchen that finishes early should be collected early,
// and a kitchen that slips must not have a rider already on the way.

// CommitInput is everything the timing decision reads.
type CommitInput struct {
	Now time.Time

	// PredictedReadyStart is when the branch expects to be ready. The window's
	// START is used rather than its end: aiming at the late edge guarantees the
	// food waits, and the safety buffer below covers the spread instead.
	PredictedReadyStart time.Time
	PredictedReadyEnd   time.Time
	ConfidenceBand      readiness.ConfidenceBand

	// PickupETA is how long the authority says it takes to get a provider to
	// the pickup. It comes from the supply preview, which is an aggregate and
	// names nobody.
	PickupETA time.Duration

	// HandoverDuration is how long collection itself takes at this branch.
	HandoverDuration time.Duration

	// Deadline is the customer's promised delivery time, when there is one.
	Deadline time.Time

	// SupplyStatus is how confident the authority is that supply exists.
	SupplyStatus string

	// ReadinessStale widens the buffer without moving the target: a quiet order
	// is probably fine and nobody has said so.
	ReadinessStale bool
}

// CommitDecision is when to commit and why.
type CommitDecision struct {
	// CommitNotBefore is the earliest moment a mission may be created.
	CommitNotBefore time.Time

	// NextEvaluationAt is when to look again. It is always set: an
	// orchestration that stops evaluating is one that will never notice the
	// kitchen finished early.
	NextEvaluationAt time.Time

	// Buffer is the safety margin applied, exposed so an operator can see why
	// a commit is early or late rather than inferring it.
	Buffer time.Duration

	// Due reports whether commitment should happen now.
	Due bool

	// AtRisk means the deadline cannot be met even committing immediately.
	// It is surfaced rather than silently absorbed, because the honest response
	// is to tell somebody, not to pretend the timing works.
	AtRisk bool

	ReasonCode string
}

// Reason codes for a timing decision.
const (
	ReasonWaitingReadiness = "WAITING_READINESS"
	ReasonCommitWindowOpen = "COMMIT_WINDOW_OPEN"
	ReasonDeadlineAtRisk   = "DEADLINE_AT_RISK"
	ReasonSupplyThin       = "SUPPLY_THIN"
)

// minimumEvaluationInterval stops a hot loop when commitment is imminent, and
// maximumEvaluationInterval stops an orchestration sleeping through a change.
const (
	minimumEvaluationInterval = 30 * time.Second
	maximumEvaluationInterval = 10 * time.Minute
)

// PlanCommit computes when to commit.
//
// The target is: a provider should arrive as the order becomes ready. Working
// backwards, the mission must be created one pickup-ETA plus a safety buffer
// before the predicted ready time.
func PlanCommit(input CommitInput) CommitDecision {
	now := input.Now.UTC()
	buffer := safetyBuffer(input)

	target := input.PredictedReadyStart.UTC()
	if target.IsZero() {
		// Nothing to aim at yet. Keep waiting and look again soon rather than
		// committing against an unknown.
		return CommitDecision{
			NextEvaluationAt: now.Add(minimumEvaluationInterval),
			Buffer:           buffer,
			ReasonCode:       ReasonWaitingReadiness,
		}
	}

	lead := input.PickupETA + buffer
	commitAt := target.Add(-lead)
	if commitAt.Before(now) {
		// The window has already opened -- usually because the branch is
		// further along than when the plan was made. Committing now is correct;
		// this is the early-ready case the whole design exists to catch.
		commitAt = now
	}

	decision := CommitDecision{
		CommitNotBefore: commitAt,
		Buffer:          buffer,
		Due:             !commitAt.After(now),
		ReasonCode:      ReasonWaitingReadiness,
	}
	if decision.Due {
		decision.ReasonCode = ReasonCommitWindowOpen
	}

	// A deadline that cannot be met even by committing immediately is a fact
	// somebody needs to know now, not after the delivery is late.
	if !input.Deadline.IsZero() {
		earliestDelivery := now.Add(input.PickupETA).Add(input.HandoverDuration)
		if target.After(now) {
			earliestDelivery = target.Add(input.HandoverDuration)
		}
		if earliestDelivery.After(input.Deadline.UTC()) {
			decision.AtRisk = true
			decision.ReasonCode = ReasonDeadlineAtRisk
			// An at-risk order commits as early as it legitimately can: the
			// provider travelling while the branch finishes is the only
			// remaining way to recover time.
			decision.CommitNotBefore = now
			decision.Due = true
		}
	}

	// Thin supply means a commit may not match. Committing earlier gives the
	// authority more time to find someone, which is a different response from
	// giving up.
	if input.SupplyStatus == "LIMITED" && !decision.Due {
		earlier := decision.CommitNotBefore.Add(-2 * time.Minute)
		if earlier.Before(now) {
			earlier = now
		}
		decision.CommitNotBefore = earlier
		decision.Due = !earlier.After(now)
		decision.ReasonCode = ReasonSupplyThin
	}

	decision.NextEvaluationAt = nextEvaluation(now, decision.CommitNotBefore)
	return decision
}

// safetyBuffer is how much slack to leave.
//
// It scales with how little the prediction can be trusted, which is the point
// of carrying confidence at all: a branch with a measured history and a fresh
// signal earns a tight buffer, and one being guessed at does not.
func safetyBuffer(input CommitInput) time.Duration {
	buffer := 2 * time.Minute
	switch input.ConfidenceBand {
	case readiness.ConfidenceHigh:
		buffer = 90 * time.Second
	case readiness.ConfidenceMedium:
		buffer = 3 * time.Minute
	case readiness.ConfidenceLow:
		buffer = 6 * time.Minute
	}
	// A wide predicted window is itself an admission of uncertainty; half of it
	// is added so the buffer reflects what the predictor actually said rather
	// than only its confidence label.
	if !input.PredictedReadyEnd.IsZero() && input.PredictedReadyEnd.After(input.PredictedReadyStart) {
		buffer += input.PredictedReadyEnd.Sub(input.PredictedReadyStart) / 2
	}
	if input.ReadinessStale {
		buffer += 3 * time.Minute
	}
	buffer += input.HandoverDuration
	return buffer
}

// nextEvaluation decides how soon to look again.
//
// Cadence follows proximity: an orchestration an hour from commitment does not
// need checking every thirty seconds, and one two minutes away does.
func nextEvaluation(now, commitAt time.Time) time.Time {
	remaining := commitAt.Sub(now)
	switch {
	case remaining <= 0:
		return now.Add(minimumEvaluationInterval)
	case remaining <= 2*time.Minute:
		return now.Add(minimumEvaluationInterval)
	case remaining <= 15*time.Minute:
		return now.Add(time.Minute)
	case remaining <= time.Hour:
		return now.Add(5 * time.Minute)
	default:
		return now.Add(maximumEvaluationInterval)
	}
}

// WarmMatchLead is how far ahead of commitment supply starts being checked.
//
// Far enough that a thin market is discovered while there is still time to act,
// close enough that the answer is still true when commitment arrives. A preview
// taken an hour early describes a market that has since changed.
const WarmMatchLead = 12 * time.Minute

// ShouldWarmMatch reports whether supply should be previewed now.
func ShouldWarmMatch(now, commitAt time.Time) bool {
	if commitAt.IsZero() {
		return false
	}
	return !now.Before(commitAt.Add(-WarmMatchLead))
}
