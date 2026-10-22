package admininterventions

import (
	"context"
	"errors"
	"time"

	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/logging"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	KindProviders        = "providers"
	KindVehicles         = "vehicles"
	KindIndependentRides = "independent-rides"
)

// Incident states (OPEN -> REMEDIATION_IN_PROGRESS -> RESOLVED -> CLOSED).
const (
	IncidentOpen        = "OPEN"
	IncidentRemediation = "REMEDIATION_IN_PROGRESS"
	IncidentResolved    = "RESOLVED"
	IncidentClosed      = "CLOSED"
)

// Subject standing states reported to the Bridge.
const (
	StandingActive                 = "ACTIVE"
	StandingOnlineSuspended        = "ONLINE_SUSPENDED"
	StandingAssignmentsSuspended   = "ASSIGNMENTS_SUSPENDED"
	StandingOutOfService           = "OUT_OF_SERVICE"
	StandingReverificationRequired = "REVERIFICATION_REQUIRED"
	StandingDocumentInvalid        = "DOCUMENT_INVALID"
	StandingIncidentOpen           = "INCIDENT_OPEN"
)

var lagos = func() *time.Location {
	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		// Africa/Lagos has been a fixed UTC+1 (WAT) offset without DST for
		// decades; the embedded tzdata makes this branch unreachable.
		return time.FixedZone("WAT", 3600)
	}
	return loc
}()

// ExpiryInstant is the canonical interpretation of a document expires_on
// date: the document is valid through the END of that calendar day in
// Africa/Lagos. The returned instant is the exclusive bound (the following
// midnight in Africa/Lagos); a document is in force while now < instant. It
// is stored as timestamptz in taxi_admin_document_decisions.expires_at and is
// authoritative for administrative gates; the native free-text
// taxi_providers.license_expiry_date is never rewritten or interpreted.
func ExpiryInstant(date string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", date, lagos)
	if err != nil {
		return time.Time{}, err
	}
	return day.AddDate(0, 0, 1), nil
}

func lagosDay(t time.Time) time.Time {
	y, m, d := t.In(lagos).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, lagos)
}

func parseDay(date string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", date, lagos)
}

// documentSubject is the closed document-to-subject mapping: the driver's
// licence belongs to the provider; registration, insurance and
// roadworthiness belong to the vehicle.
func documentSubject(document string) string {
	switch document {
	case "DRIVER_LICENSE":
		return KindProviders
	case "VEHICLE_REGISTRATION", "INSURANCE", "ROADWORTHINESS":
		return KindVehicles
	}
	return ""
}

// OnlineStatusStore is the slice of the Redis availability store the
// executor uses to take a restricted provider out of the live pool.
type OnlineStatusStore interface {
	GetStatus(ctx context.Context, providerID string) (availabilityrepositories.ProviderStatus, bool, error)
	SetOffline(ctx context.Context, providerID string) error
}

// Executor implements the Taxi owner's closed admincommand dictionary.
type Executor struct {
	// Availability is optional. When present a newly blocked provider who is
	// in the live pool is removed from it (best effort; the durable Postgres
	// transition and the SQL gates are authoritative either way).
	Availability OnlineStatusStore
}

var _ admincommand.Executor = (*Executor)(nil)

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (e *Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	if in.Spec.VersionOf == admincommand.VersionCase {
		return e.applyIncident(ctx, tx, in)
	}
	return e.applyStanding(ctx, tx, in)
}

func (e *Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	version, restriction, _, err := readStanding(ctx, tx, kind, id, false)
	if err != nil {
		return 0, "", err
	}
	state, err := standingState(ctx, tx, kind, id, restriction, time.Now())
	return version, state, err
}

// subject verifies the subject exists in Taxi's own records and returns the
// provider that owns it (the provider itself, or the vehicle's provider).
func subject(ctx context.Context, q querier, kind, id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", admincommand.ErrNotFound
	}
	var provider string
	var err error
	switch kind {
	case KindProviders:
		err = q.QueryRow(ctx, `SELECT id::text FROM taxi_providers WHERE id=$1::uuid FOR KEY SHARE`, id).Scan(&provider)
	case KindVehicles:
		err = q.QueryRow(ctx, `SELECT provider_id::text FROM vehicles WHERE id=$1::uuid FOR KEY SHARE`, id).Scan(&provider)
	case KindIndependentRides:
		err = q.QueryRow(ctx, `SELECT COALESCE(provider_id::text,'') FROM taxi_bookings WHERE id=$1::uuid AND NOT is_corporate_managed AND organization_id IS NULL`, id).Scan(&provider)
	default:
		return "", admincommand.ErrNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", admincommand.ErrNotFound
	}
	return provider, err
}

func readStanding(ctx context.Context, q querier, kind, id string, lock bool) (int64, string, bool, error) {
	sql := `SELECT version,COALESCE(restriction,'') FROM taxi_admin_subject_standing WHERE subject_kind=$1 AND subject_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	var version int64
	var restriction string
	err := q.QueryRow(ctx, sql, kind, id).Scan(&version, &restriction)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	return version, restriction, err == nil, err
}

func standingState(ctx context.Context, q querier, kind, id, restriction string, at time.Time) (string, error) {
	switch restriction {
	case "SUSPEND_ONLINE":
		return StandingOnlineSuspended, nil
	case "NO_NEW_ASSIGNMENTS":
		return StandingAssignmentsSuspended, nil
	case "OUT_OF_SERVICE":
		return StandingOutOfService, nil
	}
	if kind == KindIndependentRides {
		var open bool
		err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM taxi_admin_incidents WHERE subject_kind=$1 AND subject_id=$2 AND state IN ('OPEN','REMEDIATION_IN_PROGRESS'))`, kind, id).Scan(&open)
		if err != nil {
			return "", err
		}
		if open {
			return StandingIncidentOpen, nil
		}
		return StandingActive, nil
	}
	var block string
	if err := q.QueryRow(ctx, `SELECT COALESCE(taxi_admin_document_block($1,$2,$3),'')`, kind, id, at).Scan(&block); err != nil {
		return "", err
	}
	switch block {
	case "REVERIFICATION_REQUIRED":
		return StandingReverificationRequired, nil
	case "DOCUMENT_INVALID":
		return StandingDocumentInvalid, nil
	}
	return StandingActive, nil
}

func (e *Executor) applyStanding(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	kind, id := in.SubjectKind, in.SubjectID
	if kind != KindProviders && kind != KindVehicles {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	provider, err := subject(ctx, tx, kind, id)
	if err != nil {
		return admincommand.Transition{}, err
	}
	version, restriction, _, err := readStanding(ctx, tx, kind, id, true)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	prior, err := standingState(ctx, tx, kind, id, restriction, in.Now)
	if err != nil {
		return admincommand.Transition{}, err
	}
	next := version + 1
	resulting := restriction
	var outcome string
	var after func() error
	switch in.Action {
	case "RESTRICT_PROVIDER", "RESTRICT_VEHICLE":
		if restriction != "" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		resulting = in.Params["restriction"]
		outcome = "PROVIDER_RESTRICTED"
		if kind == KindVehicles {
			outcome = "VEHICLE_RESTRICTED"
		}
		after = func() error { return restrictionHistory(ctx, tx, in, next, restriction, resulting) }
	case "LIFT_PROVIDER_RESTRICTION", "LIFT_VEHICLE_RESTRICTION":
		if restriction == "" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		resulting = ""
		outcome = "PROVIDER_RESTRICTION_LIFTED"
		if kind == KindVehicles {
			outcome = "VEHICLE_RESTRICTION_LIFTED"
		}
		after = func() error { return restrictionHistory(ctx, tx, in, next, restriction, resulting) }
	case "RECORD_DOCUMENT_DECISION":
		document, decision := in.Params["document"], in.Params["decision"]
		if documentSubject(document) != kind {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		var expiresOn, expiresAt any
		if raw, ok := in.Params["expires_on"]; ok {
			at, err := ExpiryInstant(raw)
			if err != nil {
				return admincommand.Transition{}, admincommand.ErrRejected
			}
			expiresOn, expiresAt = raw, at
			switch decision {
			case "VERIFIED":
				if !at.After(in.Now) {
					return admincommand.Transition{}, admincommand.ErrRejected
				}
			case "EXPIRED":
				if at.After(in.Now) {
					return admincommand.Transition{}, admincommand.ErrRejected
				}
			}
		} else if decision == "VERIFIED" {
			// A verification without a canonical expiry would be in force forever.
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		var evidence any
		if ref, ok := in.Params["evidence_ref"]; ok {
			evidence = ref
		}
		outcome = "DOCUMENT_" + decision
		after = func() error {
			_, err := tx.Exec(ctx, `INSERT INTO taxi_admin_document_decisions(decision_id,subject_kind,subject_id,document,decision,expires_on,expires_at,evidence_ref,standing_version,staff_ref,reason_code,recorded_at) VALUES($1,$2,$3,$4,$5,$6::date,$7,$8,$9,$10,$11,$12)`,
				in.DecisionID, kind, id, document, decision, expiresOn, expiresAt, evidence, next, in.Staff, in.ReasonCode, in.Now)
			return err
		}
	case "REQUEST_REVERIFICATION":
		document := in.Params["document"]
		if documentSubject(document) != kind {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		var open bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM taxi_admin_reverification_requests r WHERE r.subject_kind=$1 AND r.subject_id=$2 AND r.document=$3 AND NOT EXISTS(SELECT 1 FROM taxi_admin_document_decisions d WHERE d.subject_kind=r.subject_kind AND d.subject_id=r.subject_id AND d.document=r.document AND d.decision='VERIFIED' AND d.standing_version>r.standing_version))`, kind, id, document).Scan(&open); err != nil {
			return admincommand.Transition{}, err
		}
		if open {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		outcome = "REVERIFICATION_REQUESTED"
		after = func() error {
			_, err := tx.Exec(ctx, `INSERT INTO taxi_admin_reverification_requests(decision_id,subject_kind,subject_id,document,standing_version,staff_ref,reason_code,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
				in.DecisionID, kind, id, document, next, in.Staff, in.ReasonCode, in.Now)
			return err
		}
	default:
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err := writeStanding(ctx, tx, kind, id, version, resulting, in.Now); err != nil {
		return admincommand.Transition{}, err
	}
	if err := after(); err != nil {
		return admincommand.Transition{}, err
	}
	if err := e.enforce(ctx, tx, kind, id, provider, in.Now); err != nil {
		return admincommand.Transition{}, err
	}
	state, err := standingState(ctx, tx, kind, id, resulting, in.Now)
	if err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: version, PriorState: prior, ResultingState: state, Outcome: outcome}, nil
}

func writeStanding(ctx context.Context, tx pgx.Tx, kind, id string, version int64, restriction string, now time.Time) error {
	var value any
	if restriction != "" {
		value = restriction
	}
	var tag pgconn.CommandTag
	var err error
	if version == 0 {
		tag, err = tx.Exec(ctx, `INSERT INTO taxi_admin_subject_standing(subject_kind,subject_id,version,restriction,restricted_at,updated_at) VALUES($1,$2,1,$3::text,CASE WHEN $3::text IS NULL THEN NULL ELSE $4::timestamptz END,$4) ON CONFLICT DO NOTHING`, kind, id, value, now)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE taxi_admin_subject_standing SET version=version+1,restriction=$4::text,restricted_at=CASE WHEN $4::text IS NULL THEN NULL WHEN restriction IS NOT DISTINCT FROM $4::text THEN restricted_at ELSE $5::timestamptz END,updated_at=$5 WHERE subject_kind=$1 AND subject_id=$2 AND version=$3`, kind, id, version, value, now)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return admincommand.ErrConflict
	}
	return nil
}

func restrictionHistory(ctx context.Context, tx pgx.Tx, in admincommand.Execution, version int64, prior, resulting string) error {
	null := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	_, err := tx.Exec(ctx, `INSERT INTO taxi_admin_restriction_history(decision_id,subject_kind,subject_id,standing_version,action,prior_restriction,resulting_restriction,staff_ref,reason_code,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		in.DecisionID, in.SubjectKind, in.SubjectID, version, in.Action, null(prior), null(resulting), in.Staff, in.ReasonCode, in.Now)
	return err
}

// enforce takes a provider who is now administratively blocked from being
// online out of availability, using the service's own operational-state
// machine. Only an AVAILABLE (idle, online) provider is moved to OFFLINE; a
// provider mid-offer or mid-trip is left exactly where they are -- trip state
// is never touched -- and is kept from new work by the assignment gate and
// from going online again by the online gate.
func (e *Executor) enforce(ctx context.Context, tx pgx.Tx, kind, id, provider string, at time.Time) error {
	if provider == "" {
		return nil
	}
	var live availabilityrepositories.ProviderStatus
	liveKnown, liveOnline := false, false
	if e.Availability != nil {
		status, ok, err := e.Availability.GetStatus(ctx, provider)
		if err == nil {
			live, liveKnown, liveOnline = status, true, ok
		}
	}
	vehicle := ""
	switch kind {
	case KindVehicles:
		if liveKnown && liveOnline && live.VehicleID != "" && live.VehicleID != id {
			return nil // online on a different, unaffected vehicle
		}
		vehicle = id
	case KindProviders:
		if liveKnown && liveOnline {
			vehicle = live.VehicleID
		}
	}
	var reason string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(taxi_admin_online_block($1,NULLIF($2,''),$3),'')`, provider, vehicle, at).Scan(&reason); err != nil {
		return err
	}
	if reason == "" {
		return nil
	}
	state, version := string(availabilitymodels.StateOffline), int64(0)
	err := tx.QueryRow(ctx, `SELECT state,version FROM provider_operational_state WHERE provider_id=$1::uuid FOR UPDATE`, provider).Scan(&state, &version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if state == string(availabilitymodels.StateAvailable) {
		repo := availabilityrepositories.NewPostgresOperationalStateRepository(nil)
		if _, err := repo.Transition(ctx, tx, provider, availabilitymodels.StateOffline, "admin_intervention:"+reason, nil, version); err != nil {
			return err
		}
	}
	if e.Availability != nil && (state == string(availabilitymodels.StateAvailable) || state == string(availabilitymodels.StateOffline)) && (!liveKnown || liveOnline) {
		// Best effort, before commit. If the command later fails to commit
		// the driver merely has to go online again through the gate, which
		// then admits them; the durable state and SQL gates stay correct.
		if err := e.Availability.SetOffline(ctx, provider); err != nil {
			logging.Error("taxi-admin-intervention", "remove provider=%s from live availability: %v", provider, err)
		}
	}
	return nil
}

type incident struct {
	kind, id, state string
	version         int64
}

func (e *Executor) applyIncident(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var cur incident
	err := tx.QueryRow(ctx, `SELECT subject_kind,subject_id,state,version FROM taxi_admin_incidents WHERE case_ref=$1 FOR UPDATE`, in.CaseRef).Scan(&cur.kind, &cur.id, &cur.state, &cur.version)
	today := lagosDay(in.Now)
	if errors.Is(err, pgx.ErrNoRows) {
		if in.Action != "OPEN_INCIDENT" {
			return admincommand.Transition{}, admincommand.ErrNotFound
		}
		if in.ExpectedVersion != 0 {
			return admincommand.Transition{}, admincommand.ErrConflict
		}
		if _, err := subject(ctx, tx, in.SubjectKind, in.SubjectID); err != nil {
			return admincommand.Transition{}, err
		}
		occurred, err := parseDay(in.Params["occurred_on"])
		if err != nil || occurred.After(today) {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		var provider, vehicle, booking any
		switch in.SubjectKind {
		case KindProviders:
			provider = in.SubjectID
		case KindVehicles:
			vehicle = in.SubjectID
		case KindIndependentRides:
			booking = in.SubjectID
		}
		tag, err := tx.Exec(ctx, `INSERT INTO taxi_admin_incidents(case_ref,subject_kind,subject_id,provider_id,vehicle_id,booking_id,category,severity,occurred_on,state,version,opened_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::date,'OPEN',1,$10,$10) ON CONFLICT DO NOTHING`,
			in.CaseRef, in.SubjectKind, in.SubjectID, provider, vehicle, booking, in.Params["category"], in.Params["severity"], in.Params["occurred_on"], in.Now)
		if err != nil {
			return admincommand.Transition{}, err
		}
		if tag.RowsAffected() != 1 {
			return admincommand.Transition{}, admincommand.ErrConflict
		}
		if err := incidentTransition(ctx, tx, in, nil, IncidentOpen, 1); err != nil {
			return admincommand.Transition{}, err
		}
		return admincommand.Transition{PriorVersion: 0, PriorState: "NONE", ResultingState: IncidentOpen, Outcome: "INCIDENT_OPENED"}, nil
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	// A case is bound to exactly one subject; a different subject's command
	// must not learn that the case exists.
	if cur.kind != in.SubjectKind || cur.id != in.SubjectID {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	if cur.version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	next := cur.version + 1
	var to, outcome string
	var resolution any
	switch in.Action {
	case "START_REMEDIATION":
		if cur.state != IncidentOpen && cur.state != IncidentRemediation {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		due, err := parseDay(in.Params["due_on"])
		if err != nil || due.Before(today) {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `INSERT INTO taxi_admin_incident_remediations(decision_id,case_ref,remediation,due_on,incident_version,recorded_at) VALUES($1,$2,$3,$4::date,$5,$6)`,
			in.DecisionID, in.CaseRef, in.Params["remediation"], in.Params["due_on"], next, in.Now); err != nil {
			return admincommand.Transition{}, err
		}
		to, outcome = IncidentRemediation, "REMEDIATION_STARTED"
	case "RESOLVE_INCIDENT":
		r := in.Params["resolution"]
		// REMEDIATED is only a truthful resolution once a remediation exists.
		if r == "REMEDIATED" && cur.state != IncidentRemediation || cur.state != IncidentOpen && cur.state != IncidentRemediation {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		resolution, to, outcome = r, IncidentResolved, "INCIDENT_RESOLVED"
	case "CLOSE_INCIDENT":
		if cur.state != IncidentResolved {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		to, outcome = IncidentClosed, "INCIDENT_CLOSED"
	default:
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	tag, err := tx.Exec(ctx, `UPDATE taxi_admin_incidents SET state=$3,version=version+1,updated_at=$4,
		resolution=COALESCE($5::text,resolution),
		resolved_at=CASE WHEN $3='RESOLVED' THEN $4::timestamptz ELSE resolved_at END,
		closed_at=CASE WHEN $3='CLOSED' THEN $4::timestamptz ELSE closed_at END
		WHERE case_ref=$1 AND version=$2`, in.CaseRef, cur.version, to, in.Now, resolution)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if tag.RowsAffected() != 1 {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	from := cur.state
	if err := incidentTransition(ctx, tx, in, &from, to, next); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: cur.version, PriorState: cur.state, ResultingState: to, Outcome: outcome}, nil
}

func incidentTransition(ctx context.Context, tx pgx.Tx, in admincommand.Execution, from *string, to string, version int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO taxi_admin_incident_transitions(decision_id,case_ref,action,from_state,to_state,incident_version,staff_ref,reason_code,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		in.DecisionID, in.CaseRef, in.Action, from, to, version, in.Staff, in.ReasonCode, in.Now)
	return err
}
