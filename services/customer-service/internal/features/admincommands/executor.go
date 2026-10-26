// Package admincommands implements the Customer owner's closed administrative
// command dictionary (customer.admin-command.v1, shared/go/admincommand).
//
// Every action is a named domain transition on Customer-owned state with a
// real effect the rest of customer-service consults:
//   - account standing (restriction + remediation) gates OTP login, refresh
//     and booking eligibility on the internal customer context;
//   - privacy requests are tracked RECEIVED -> IN_PROGRESS -> FULFILLED |
//     REJECTED; consent-withdrawal fulfilment withdraws the consents this
//     service owns;
//   - support decisions are versioned, case-bound records.
//
// There is no generic state setter. The shared framework authorizes,
// approves, version-fences per subject and writes the immutable ledger row in
// the same transaction as these effects.
package admincommands

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	authrepositories "cosmicforge/logistics/services/customer-service/internal/features/auth/repositories"
	marketingconsent "cosmicforge/logistics/services/customer-service/internal/features/marketing_consent"
	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	Owner       = "customer-service"
	Prefix      = "CUSTOMER"
	SubjectKind = "customers"

	RestrictionNone    = "NONE"
	RestrictionLogin   = "LOGIN_BLOCKED"
	RestrictionBooking = "BOOKING_HOLD"
	RemediationNone    = "NONE"
	RemediationReverif = "REVERIFY_CONTACT"
	RemediationReset   = "RESET_SESSIONS"
	RemediationProfile = "PROFILE_CORRECTION"

	PrivacyReceived   = "RECEIVED"
	PrivacyInProgress = "IN_PROGRESS"
	PrivacyFulfilled  = "FULFILLED"
	PrivacyRejected   = "REJECTED"

	// FulfilmentScope records that a fulfilment covers customer-service
	// records only; other owners' data needs each owner's own authorized
	// contract and is never touched from here.
	FulfilmentScope = "CUSTOMER_SERVICE_RECORDS_ONLY"
)

// Spec is the customer owner's fixed command dictionary.
func Spec() admincommand.Spec {
	spec, _ := admincommand.ForOwner(Owner)
	return spec
}

// Executor is the customer owner's admincommand.Executor.
type Executor struct{}

var _ admincommand.Executor = Executor{}

func (Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	if in.SubjectKind != SubjectKind || !admincommand.CanonicalID(in.SubjectID) {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	switch in.Action {
	case "RESTRICT_ACCOUNT", "LIFT_RESTRICTION", "REQUIRE_REMEDIATION", "COMPLETE_REMEDIATION":
		return applyStanding(ctx, tx, in)
	case "OPEN_PRIVACY_REQUEST", "ACCEPT_PRIVACY_REQUEST", "FULFIL_PRIVACY_REQUEST", "REJECT_PRIVACY_REQUEST":
		return applyPrivacy(ctx, tx, in)
	case "RECORD_SUPPORT_DECISION":
		return applySupport(ctx, tx, in)
	}
	return admincommand.Transition{}, admincommand.ErrRejected
}

// Current reports the customer's account standing version and state token.
// A customer with no administrative decision is version 0, ACTIVE. It only
// reads, so it is safe inside the framework's read-only snapshot.
func (Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	if kind != SubjectKind || !admincommand.CanonicalID(id) {
		return 0, "", admincommand.ErrNotFound
	}
	var version *int64
	var state *string
	err := tx.QueryRow(ctx, `SELECT s.version,s.state FROM customers c LEFT JOIN customer_account_standing s ON s.customer_id=c.id WHERE c.id=$1`, id).Scan(&version, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", admincommand.ErrNotFound
	}
	if err != nil {
		return 0, "", err
	}
	if version == nil {
		return 0, standingState(RestrictionNone, RemediationNone), nil
	}
	return *version, *state, nil
}

// standingState is the deterministic UPPER_SNAKE token for a standing.
func standingState(restriction, remediation string) string {
	base := "ACTIVE"
	if restriction != RestrictionNone {
		base = restriction
	}
	if remediation != RemediationNone {
		return base + "_REMEDIATION_" + remediation
	}
	return base
}

type standing struct {
	version                  int64
	restriction, remediation string
	requiredAt               *time.Time
}

func applyStanding(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var empty admincommand.Transition
	// The customer row lock serializes against concurrent refresh-session
	// creation (the customer_sessions guard takes FOR SHARE on this row).
	var onboarding string
	err := tx.QueryRow(ctx, `SELECT onboarding_status FROM customers WHERE id=$1 FOR UPDATE`, in.SubjectID).Scan(&onboarding)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, admincommand.ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	current := standing{restriction: RestrictionNone, remediation: RemediationNone}
	err = tx.QueryRow(ctx, `SELECT version,restriction,remediation,remediation_required_at FROM customer_account_standing WHERE customer_id=$1 FOR UPDATE`, in.SubjectID).Scan(&current.version, &current.restriction, &current.remediation, &current.requiredAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	if current.version != in.ExpectedVersion {
		return empty, admincommand.ErrConflict
	}
	next := current
	var revoked int64
	outcome := ""
	evidence := in.Params["evidence_ref"]
	switch in.Action {
	case "RESTRICT_ACCOUNT":
		restriction := in.Params["restriction"]
		// Changing an existing restriction requires an explicit lift first, so
		// every restriction episode has its own approved start and end.
		if current.restriction != RestrictionNone || (restriction != RestrictionLogin && restriction != RestrictionBooking) {
			return empty, admincommand.ErrRejected
		}
		next.restriction = restriction
		outcome = "BOOKING_HOLD_APPLIED"
		if restriction == RestrictionLogin {
			if revoked, err = authrepositories.RevokeAllCustomerSessions(ctx, tx, in.SubjectID); err != nil {
				return empty, err
			}
			outcome = "LOGIN_BLOCKED_SESSIONS_REVOKED"
		}
	case "LIFT_RESTRICTION":
		if current.restriction == RestrictionNone {
			return empty, admincommand.ErrRejected
		}
		next.restriction = RestrictionNone
		outcome = "RESTRICTION_LIFTED"
	case "REQUIRE_REMEDIATION":
		remediation := in.Params["remediation"]
		if current.remediation != RemediationNone {
			return empty, admincommand.ErrRejected
		}
		at := in.Now
		next.remediation, next.requiredAt = remediation, &at
		switch remediation {
		case RemediationReset, RemediationReverif:
			// Every refresh session ends; the customer must prove control of a
			// contact again through OTP before receiving a new session.
			if revoked, err = authrepositories.RevokeAllCustomerSessions(ctx, tx, in.SubjectID); err != nil {
				return empty, err
			}
			outcome = "REMEDIATION_REQUIRED_SESSIONS_REVOKED"
		case RemediationProfile:
			// Reuses the existing onboarding gate: the app must continue
			// profile setup, and a profile update flips it back to complete.
			if _, err := tx.Exec(ctx, `UPDATE customers SET onboarding_status=$2,updated_at=now() WHERE id=$1`, in.SubjectID, profilemodels.OnboardingProfileNeeded); err != nil {
				return empty, err
			}
			outcome = "REMEDIATION_REQUIRED_PROFILE_REOPENED"
		default:
			return empty, admincommand.ErrRejected
		}
	case "COMPLETE_REMEDIATION":
		if current.remediation == RemediationNone {
			return empty, admincommand.ErrRejected
		}
		switch current.remediation {
		case RemediationProfile:
			if onboarding != profilemodels.OnboardingComplete {
				return empty, admincommand.ErrRejected
			}
		case RemediationReverif:
			// Completion needs proof the customer re-verified a contact: an OTP
			// login after the requirement created a new refresh session.
			var verified bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_sessions WHERE customer_id=$1 AND created_at>$2)`, in.SubjectID, *current.requiredAt).Scan(&verified); err != nil {
				return empty, err
			}
			if !verified {
				return empty, admincommand.ErrRejected
			}
		}
		next.remediation, next.requiredAt = RemediationNone, nil
		outcome = "REMEDIATION_COMPLETED"
	default:
		return empty, admincommand.ErrRejected
	}
	prior := standingState(current.restriction, current.remediation)
	resulting := standingState(next.restriction, next.remediation)
	version := in.ExpectedVersion + 1
	if current.version == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO customer_account_standing(customer_id,version,restriction,remediation,remediation_required_at,state,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, in.SubjectID, version, next.restriction, next.remediation, next.requiredAt, resulting, in.Now)
	} else {
		_, err = tx.Exec(ctx, `UPDATE customer_account_standing SET version=$2,restriction=$3,remediation=$4,remediation_required_at=$5,state=$6,updated_at=$7 WHERE customer_id=$1 AND version=$8`, in.SubjectID, version, next.restriction, next.remediation, next.requiredAt, resulting, in.Now, current.version)
	}
	if err != nil {
		return empty, conflictOr(err)
	}
	var evidenceArg any
	if evidence != "" {
		evidenceArg = evidence
	}
	if _, err := tx.Exec(ctx, `INSERT INTO customer_account_standing_history(customer_id,decision_id,action,reason_code,prior_version,recorded_version,prior_state,resulting_state,restriction,remediation,evidence_ref,sessions_revoked) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, in.SubjectID, in.DecisionID, in.Action, in.ReasonCode, current.version, version, prior, resulting, next.restriction, next.remediation, evidenceArg, revoked); err != nil {
		return empty, conflictOr(err)
	}
	return admincommand.Transition{PriorVersion: current.version, PriorState: prior, ResultingState: resulting, Outcome: outcome}, nil
}

type privacyCase struct {
	customer, state, requestType string
	version                      int64
	receivedOn                   time.Time
}

func applyPrivacy(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var empty admincommand.Transition
	if !admincommand.CanonicalID(in.CaseRef) {
		return empty, admincommand.ErrInvalid
	}
	if err := requireCustomer(ctx, tx, in.SubjectID); err != nil {
		return empty, err
	}
	var c privacyCase
	err := tx.QueryRow(ctx, `SELECT customer_id::text,state,request_type,version,received_on FROM customer_privacy_requests WHERE case_ref=$1 FOR UPDATE`, in.CaseRef).Scan(&c.customer, &c.state, &c.requestType, &c.version, &c.receivedOn)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	if in.Action == "OPEN_PRIVACY_REQUEST" {
		if exists || in.ExpectedVersion != 0 {
			return empty, admincommand.ErrConflict
		}
		if taken, err := supportCaseExists(ctx, tx, in.CaseRef); err != nil {
			return empty, err
		} else if taken {
			return empty, admincommand.ErrConflict
		}
		received, err := time.Parse("2006-01-02", in.Params["received_on"])
		if err != nil {
			return empty, admincommand.ErrInvalid
		}
		if received.After(in.Now) {
			return empty, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `INSERT INTO customer_privacy_requests(case_ref,customer_id,request_type,channel,received_on,state,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'RECEIVED',1,$6,$6)`, in.CaseRef, in.SubjectID, in.Params["request_type"], in.Params["channel"], received, in.Now); err != nil {
			return empty, conflictOr(err)
		}
		detail := map[string]any{"request_type": in.Params["request_type"], "channel": in.Params["channel"], "received_on": in.Params["received_on"]}
		if err := privacyHistory(ctx, tx, in, 0, "NONE", PrivacyReceived, detail); err != nil {
			return empty, err
		}
		return admincommand.Transition{PriorVersion: 0, PriorState: "NONE", ResultingState: PrivacyReceived, Outcome: "PRIVACY_REQUEST_RECEIVED"}, nil
	}
	// A case owned by another customer is indistinguishable from a missing
	// one: no command may learn or move a foreign subject's case.
	if !exists || c.customer != in.SubjectID {
		return empty, admincommand.ErrNotFound
	}
	if c.version != in.ExpectedVersion {
		return empty, admincommand.ErrConflict
	}
	version := c.version + 1
	detail := map[string]any{}
	var next, outcome string
	switch in.Action {
	case "ACCEPT_PRIVACY_REQUEST":
		if c.state != PrivacyReceived {
			return empty, admincommand.ErrRejected
		}
		due, err := time.Parse("2006-01-02", in.Params["due_on"])
		if err != nil {
			return empty, admincommand.ErrInvalid
		}
		if due.Before(c.receivedOn) {
			return empty, admincommand.ErrRejected
		}
		next, outcome = PrivacyInProgress, "PRIVACY_REQUEST_ACCEPTED"
		detail["due_on"] = in.Params["due_on"]
		if _, err := tx.Exec(ctx, `UPDATE customer_privacy_requests SET state=$2,due_on=$3,version=$4,updated_at=$5 WHERE case_ref=$1`, in.CaseRef, next, due, version, in.Now); err != nil {
			return empty, err
		}
	case "FULFIL_PRIVACY_REQUEST":
		if c.state != PrivacyInProgress {
			return empty, admincommand.ErrRejected
		}
		var withdrawn int64
		outcome = "DISPOSITION_RECORDED_CUSTOMER_SERVICE_ONLY"
		if c.requestType == "CONSENT_WITHDRAWAL" {
			var err error
			if withdrawn, err = withdrawConsents(ctx, tx, in.SubjectID, in.Now, in.DecisionID); err != nil {
				return empty, err
			}
			outcome = "CONSENTS_WITHDRAWN"
		}
		next = PrivacyFulfilled
		detail["evidence_ref"], detail["fulfilment_scope"], detail["consents_withdrawn"] = in.Params["evidence_ref"], FulfilmentScope, withdrawn
		if _, err := tx.Exec(ctx, `UPDATE customer_privacy_requests SET state=$2,evidence_ref=$3,fulfilment_scope=$4,consents_withdrawn=$5,version=$6,updated_at=$7 WHERE case_ref=$1`, in.CaseRef, next, in.Params["evidence_ref"], FulfilmentScope, withdrawn, version, in.Now); err != nil {
			return empty, err
		}
	case "REJECT_PRIVACY_REQUEST":
		if c.state != PrivacyReceived && c.state != PrivacyInProgress {
			return empty, admincommand.ErrRejected
		}
		next, outcome = PrivacyRejected, "PRIVACY_REQUEST_REJECTED"
		detail["rejection"] = in.Params["rejection"]
		if _, err := tx.Exec(ctx, `UPDATE customer_privacy_requests SET state=$2,rejection=$3,version=$4,updated_at=$5 WHERE case_ref=$1`, in.CaseRef, next, in.Params["rejection"], version, in.Now); err != nil {
			return empty, err
		}
	default:
		return empty, admincommand.ErrRejected
	}
	if err := privacyHistory(ctx, tx, in, c.version, c.state, next, detail); err != nil {
		return empty, err
	}
	return admincommand.Transition{PriorVersion: c.version, PriorState: c.state, ResultingState: next, Outcome: outcome}, nil
}

// withdrawConsents turns off every consent customer-service owns for the
// customer: trusted-contact trip sharing and emergency alerts, the
// safety-share default and every opted-in marketing channel. Each changed row's own version advances so clients
// holding an older version cannot silently restore a withdrawn consent.
func withdrawConsents(ctx context.Context, tx pgx.Tx, customer string, now time.Time, decision string) (int64, error) {
	contacts, err := tx.Exec(ctx, `UPDATE emergency_contacts SET trip_sharing_enabled=false,emergency_alerts_enabled=false,version=version+1,updated_at=$2 WHERE customer_id=$1 AND (trip_sharing_enabled OR emergency_alerts_enabled)`, customer, now)
	if err != nil {
		return 0, err
	}
	prefs, err := tx.Exec(ctx, `UPDATE customer_mobility_preferences SET safety_share_default=false,version=version+1,updated_at=$2 WHERE customer_id=$1 AND safety_share_default`, customer, now)
	if err != nil {
		return 0, err
	}
	// Marketing consent (Section 17): every opted-in channel is withdrawn
	// with its own version advance and an append-only history row bound to
	// this decision.
	marketing, err := marketingconsent.WithdrawAllTx(ctx, tx, customer, now, decision)
	if err != nil {
		return 0, err
	}
	return contacts.RowsAffected() + prefs.RowsAffected() + marketing, nil
}

func privacyHistory(ctx context.Context, tx pgx.Tx, in admincommand.Execution, prior int64, priorState, next string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO customer_privacy_request_transitions(case_ref,customer_id,decision_id,action,reason_code,prior_version,recorded_version,prior_state,resulting_state,detail) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, in.CaseRef, in.SubjectID, in.DecisionID, in.Action, in.ReasonCode, prior, prior+1, priorState, next, raw)
	return conflictOr(err)
}

func applySupport(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var empty admincommand.Transition
	if err := requireCustomer(ctx, tx, in.SubjectID); err != nil {
		return empty, err
	}
	decision := in.Params["decision"]
	var customer, current string
	var version int64
	err := tx.QueryRow(ctx, `SELECT customer_id::text,version,decision FROM customer_support_decision_cases WHERE case_ref=$1 FOR UPDATE`, in.CaseRef).Scan(&customer, &version, &current)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	prior := "NONE"
	if !exists {
		if in.ExpectedVersion != 0 {
			return empty, admincommand.ErrNotFound
		}
		if taken, err := privacyCaseExists(ctx, tx, in.CaseRef); err != nil {
			return empty, err
		} else if taken {
			return empty, admincommand.ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO customer_support_decision_cases(case_ref,customer_id,version,decision,created_at,updated_at) VALUES($1,$2,1,$3,$4,$4)`, in.CaseRef, in.SubjectID, decision, in.Now); err != nil {
			return empty, conflictOr(err)
		}
	} else {
		if customer != in.SubjectID {
			return empty, admincommand.ErrNotFound
		}
		if version != in.ExpectedVersion {
			return empty, admincommand.ErrConflict
		}
		// Re-recording the identical decision is a no-op, not a new decision.
		if current == decision {
			return empty, admincommand.ErrRejected
		}
		prior = current
		if _, err := tx.Exec(ctx, `UPDATE customer_support_decision_cases SET version=$2,decision=$3,updated_at=$4 WHERE case_ref=$1`, in.CaseRef, version+1, decision, in.Now); err != nil {
			return empty, err
		}
	}
	var note any
	if n := in.Params["note"]; n != "" {
		note = n
	}
	if _, err := tx.Exec(ctx, `INSERT INTO customer_support_decisions(case_ref,customer_id,decision_id,reason_code,prior_version,recorded_version,prior_decision,decision,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, in.CaseRef, in.SubjectID, in.DecisionID, in.ReasonCode, in.ExpectedVersion, in.ExpectedVersion+1, prior, decision, note); err != nil {
		return empty, conflictOr(err)
	}
	outcome := "SUPPORT_DECISION_RECORDED"
	if decision == "ESCALATED_TO_RESTRICTION_REVIEW" {
		// Recorded only: opening a Customer review requires that workflow's own
		// signed review-state context and capability; it is not opened from here.
		outcome = "RESTRICTION_REVIEW_ESCALATION_RECORDED"
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: prior, ResultingState: decision, Outcome: outcome}, nil
}

func requireCustomer(ctx context.Context, tx pgx.Tx, id string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customers WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return admincommand.ErrNotFound
	}
	return nil
}

// A case reference names exactly one case kind; reusing a privacy case ref
// for a support decision (or the reverse) is an identity conflict.
func supportCaseExists(ctx context.Context, tx pgx.Tx, ref string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_support_decision_cases WHERE case_ref=$1)`, ref).Scan(&exists)
	return exists, err
}
func privacyCaseExists(ctx context.Context, tx pgx.Tx, ref string) (bool, error) {
	if !admincommand.CanonicalID(ref) {
		return false, nil
	}
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_privacy_requests WHERE case_ref=$1)`, ref).Scan(&exists)
	return exists, err
}

// conflictOr maps a uniqueness violation (a concurrent identical case
// creation, a reused decision) to an identity conflict and a schema check
// violation to a domain rejection.
func conflictOr(err error) error {
	if err == nil {
		return nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return admincommand.ErrConflict
	}
	if errors.As(err, &pg) && pg.Code == "23514" {
		return admincommand.ErrRejected
	}
	return err
}
