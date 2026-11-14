// Package restrictions is Taxi's authority for Fleet compliance restrictions.
//
// Fleet (fleet-partner-service, internal/features/compliance) records an
// operational restriction and propagates it to every execution service that
// holds the provider's working state. Taxi keeps its own copy here, and the
// managed-execution and private-execution gates consult it, so a restricted
// driver, vehicle or organization is never offered or assigned that
// organization's work through Taxi.
//
// The wire contract is Hauling's exactly (driver-hauling-service
// internal/features/restrictions): the same request and response JSON, the
// same routes, and idempotency by operation reference -- a replay of the same
// command returns the original outcome, a different command under the same
// reference is a conflict.
package restrictions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StateApplied = "APPLIED"
	StateCleared = "CLEARED"

	OutcomeConfirmed  = "CONFIRMED"
	OutcomeSuperseded = "SUPERSEDED"

	SubjectOrganization = "ORGANIZATION"
	SubjectProvider     = "PROVIDER"
	SubjectAsset        = "ASSET"
)

var (
	ErrNotFound = errors.New("restriction not found")
	ErrConflict = errors.New("restriction operation conflict")
	ErrInvalid  = errors.New("invalid restriction command")
)

var subjectTypes = map[string]bool{"ORGANIZATION": true, "PROVIDER": true, "ASSET": true, "SERVICE_CAPABILITY": true, "SHIPMENT": true}

// Command is what Fleet sends to apply or clear a restriction.
type Command struct {
	OperationReference string `json:"operationReference"`
	OrganizationID     string `json:"organizationId"`
	SubjectType        string `json:"subjectType"`
	SubjectReference   string `json:"subjectReference"`
	ReasonCode         string `json:"reasonCode"`
	SourceReference    string `json:"sourceReference"`
	SourceVersion      int64  `json:"sourceVersion"`
}

// View is the current restriction for one subject in one organization.
type View struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	SubjectType      string     `json:"subjectType"`
	SubjectReference string     `json:"subjectReference"`
	State            string     `json:"state"`
	ReasonCode       string     `json:"reasonCode"`
	SourceReference  string     `json:"sourceReference"`
	SourceVersion    int64      `json:"sourceVersion"`
	Version          int64      `json:"version"`
	AppliedAt        *time.Time `json:"appliedAt,omitempty"`
	ClearedAt        *time.Time `json:"clearedAt,omitempty"`
}

// OperationView is the recorded outcome of one command. Fleet reads it back
// after an ambiguous call instead of re-sending.
type OperationView struct {
	OperationReference string `json:"operationReference"`
	RestrictionID      string `json:"restrictionId"`
	RequestedState     string `json:"requestedState"`
	Outcome            string `json:"outcome"`
	SourceVersion      int64  `json:"sourceVersion"`
	RestrictionVersion int64  `json:"restrictionVersion"`
}

// Subject names one thing a restriction can bind.
type Subject struct{ Type, Reference string }

// Service owns the restriction state.
type Service struct{ db *pgxpool.Pool }

// New builds the service.
func New(db *pgxpool.Pool) *Service { return &Service{db: db} }

func validate(c Command) error {
	if strings.TrimSpace(c.OperationReference) == "" || strings.TrimSpace(c.OrganizationID) == "" ||
		!subjectTypes[c.SubjectType] || strings.TrimSpace(c.SubjectReference) == "" ||
		strings.TrimSpace(c.ReasonCode) == "" || strings.TrimSpace(c.SourceReference) == "" || c.SourceVersion < 1 {
		return ErrInvalid
	}
	return nil
}

// fingerprint covers the requested state as well as the command, so an apply
// and a clear can never share an operation reference.
func fingerprint(state string, c Command) string {
	b, _ := json.Marshal(struct {
		State string
		Command
	}{state, c})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Apply records that the subject is restricted.
func (s *Service) Apply(ctx context.Context, c Command) (OperationView, error) {
	return s.command(ctx, StateApplied, c)
}

// Clear lifts a restriction. Clearing a subject Taxi never restricted is
// ErrNotFound: there is nothing to confirm.
func (s *Service) Clear(ctx context.Context, c Command) (OperationView, error) {
	return s.command(ctx, StateCleared, c)
}

func (s *Service) command(ctx context.Context, state string, c Command) (OperationView, error) {
	if err := validate(c); err != nil {
		return OperationView{}, err
	}
	v, err := s.commandOnce(ctx, state, c)
	if err == nil || errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
		return v, err
	}
	// Two concurrent first deliveries of the same operation: the loser fails
	// on serialization or the operation key, then reads what the winner
	// recorded rather than failing work that did succeed.
	if prior, priorFP, readErr := readOperation(ctx, s.db, c.OperationReference); readErr == nil {
		if priorFP != fingerprint(state, c) {
			return OperationView{}, ErrConflict
		}
		return prior, nil
	}
	return OperationView{}, err
}

func (s *Service) commandOnce(ctx context.Context, state string, c Command) (OperationView, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return OperationView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	v, err := commandTx(ctx, tx, state, c)
	if err != nil {
		return OperationView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OperationView{}, err
	}
	return v, nil
}

const viewColumns = `id::text,organization_id,subject_type,subject_reference,state,reason_code,source_reference,source_version,version,applied_at,cleared_at`

func scanView(row pgx.Row) (View, error) {
	var v View
	err := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.State, &v.ReasonCode,
		&v.SourceReference, &v.SourceVersion, &v.Version, &v.AppliedAt, &v.ClearedAt)
	return v, err
}

func commandTx(ctx context.Context, tx pgx.Tx, state string, c Command) (OperationView, error) {
	fp := fingerprint(state, c)
	if prior, priorFP, err := readOperation(ctx, tx, c.OperationReference); err == nil {
		if priorFP != fp {
			return OperationView{}, ErrConflict
		}
		return prior, nil
	} else if !errors.Is(err, ErrNotFound) {
		return OperationView{}, err
	}

	cur, err := scanView(tx.QueryRow(ctx, `SELECT `+viewColumns+` FROM taxi_authority_restrictions
		WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 FOR UPDATE`,
		c.OrganizationID, c.SubjectType, c.SubjectReference))
	outcome := OutcomeConfirmed
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if state == StateCleared {
			return OperationView{}, ErrNotFound
		}
		err = tx.QueryRow(ctx, `INSERT INTO taxi_authority_restrictions
			(organization_id,subject_type,subject_reference,state,reason_code,source_reference,source_version,applied_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,now()) RETURNING id::text,version`,
			c.OrganizationID, c.SubjectType, c.SubjectReference, state, c.ReasonCode, c.SourceReference, c.SourceVersion).
			Scan(&cur.ID, &cur.Version)
	case err == nil:
		switch {
		case c.SourceVersion < cur.SourceVersion:
			// Fleet has already moved past this command; record that it
			// arrived, change nothing.
			outcome = OutcomeSuperseded
		case c.SourceVersion == cur.SourceVersion && cur.State != state:
			return OperationView{}, ErrConflict
		case c.SourceVersion > cur.SourceVersion || cur.State != state:
			err = tx.QueryRow(ctx, `UPDATE taxi_authority_restrictions SET state=$2,reason_code=$3,source_reference=$4,
				source_version=$5,version=version+1,
				applied_at=CASE WHEN $2='APPLIED' THEN now() ELSE applied_at END,
				cleared_at=CASE WHEN $2='CLEARED' THEN now() ELSE NULL END,updated_at=now()
				WHERE id=$1::uuid RETURNING version`,
				cur.ID, state, c.ReasonCode, c.SourceReference, c.SourceVersion).Scan(&cur.Version)
		}
	}
	if err != nil {
		return OperationView{}, fmt.Errorf("store restriction: %w", err)
	}
	if outcome == OutcomeConfirmed {
		if _, err = tx.Exec(ctx, `INSERT INTO taxi_authority_restriction_history
			(restriction_id,organization_id,subject_type,subject_reference,state,reason_code,source_reference,source_version,restriction_version)
			VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9)`,
			cur.ID, c.OrganizationID, c.SubjectType, c.SubjectReference, state, c.ReasonCode, c.SourceReference, c.SourceVersion, cur.Version); err != nil {
			return OperationView{}, err
		}
	}
	v := OperationView{OperationReference: c.OperationReference, RestrictionID: cur.ID, RequestedState: state,
		Outcome: outcome, SourceVersion: c.SourceVersion, RestrictionVersion: cur.Version}
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_authority_restriction_operations
		(operation_reference,restriction_id,requested_state,request_fingerprint,outcome,source_version,restriction_version)
		VALUES($1,$2::uuid,$3,$4,$5,$6,$7)`,
		c.OperationReference, cur.ID, state, fp, outcome, c.SourceVersion, cur.Version); err != nil {
		return OperationView{}, err
	}
	return v, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readOperation(ctx context.Context, q queryRower, ref string) (OperationView, string, error) {
	var v OperationView
	var fp string
	err := q.QueryRow(ctx, `SELECT operation_reference,restriction_id::text,requested_state,outcome,source_version,
		restriction_version,request_fingerprint FROM taxi_authority_restriction_operations WHERE operation_reference=$1`, ref).
		Scan(&v.OperationReference, &v.RestrictionID, &v.RequestedState, &v.Outcome, &v.SourceVersion, &v.RestrictionVersion, &fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fp, ErrNotFound
	}
	return v, fp, err
}

// ReadOperation is Fleet's read-back of one command's outcome.
func (s *Service) ReadOperation(ctx context.Context, ref string) (OperationView, error) {
	v, _, err := readOperation(ctx, s.db, ref)
	return v, err
}

// Read returns the current restriction for one subject in one organization.
func (s *Service) Read(ctx context.Context, org, typ, ref string) (View, error) {
	v, err := scanView(s.db.QueryRow(ctx, `SELECT `+viewColumns+` FROM taxi_authority_restrictions
		WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3`, org, typ, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// Restricted reports whether the subject has an applied restriction from the
// organization. An empty organization matches a restriction from any
// organization, which is the safe answer when the caller cannot name one.
func (s *Service) Restricted(ctx context.Context, org, typ, ref string) (bool, error) {
	if strings.TrimSpace(ref) == "" {
		return false, nil
	}
	var result bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM taxi_authority_restrictions
		WHERE ($1='' OR organization_id=$1) AND subject_type=$2 AND subject_reference=$3 AND state='APPLIED')`,
		org, typ, ref).Scan(&result)
	return result, err
}

// AnyRestricted reports whether any of the subjects is restricted by the
// organization. Subjects with an empty reference are skipped.
func (s *Service) AnyRestricted(ctx context.Context, org string, subjects ...Subject) (bool, error) {
	for _, subject := range subjects {
		restricted, err := s.Restricted(ctx, org, subject.Type, subject.Reference)
		if err != nil || restricted {
			return restricted, err
		}
	}
	return false, nil
}
