// Package restrictions owns Dispatch's authoritative projection of Fleet
// operational restrictions. The projection is deliberately small: Fleet owns
// policy and Dispatch owns enforcement at candidate and execution commit time.
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
)

var (
	ErrNotFound = errors.New("restriction not found")
	ErrConflict = errors.New("restriction operation conflicts with prior request")
	ErrInvalid  = errors.New("invalid restriction command")
)

type Command struct {
	OperationReference string `json:"operationReference"`
	OrganizationID     string `json:"organizationId"`
	SubjectType        string `json:"subjectType"`
	SubjectReference   string `json:"subjectReference"`
	ReasonCode         string `json:"reasonCode"`
	SourceReference    string `json:"sourceReference"`
	SourceVersion      int64  `json:"sourceVersion"`
}

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

type OperationView struct {
	OperationReference string `json:"operationReference"`
	RestrictionID      string `json:"restrictionId"`
	RequestedState     string `json:"requestedState"`
	Outcome            string `json:"outcome"`
	SourceVersion      int64  `json:"sourceVersion"`
	RestrictionVersion int64  `json:"restrictionVersion"`
}

type Service struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Service { return &Service{db: db} }

func validate(c Command) error {
	validType := map[string]bool{"ORGANIZATION": true, "PROVIDER": true, "ASSET": true, "SERVICE_CAPABILITY": true, "SHIPMENT": true}
	if strings.TrimSpace(c.OperationReference) == "" || strings.TrimSpace(c.OrganizationID) == "" ||
		!validType[c.SubjectType] || strings.TrimSpace(c.SubjectReference) == "" ||
		strings.TrimSpace(c.ReasonCode) == "" || strings.TrimSpace(c.SourceReference) == "" || c.SourceVersion < 1 {
		return ErrInvalid
	}
	return nil
}

func fingerprint(state string, c Command) string {
	b, _ := json.Marshal(struct {
		State string
		Command
	}{state, c})
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func (s *Service) Apply(ctx context.Context, c Command) (OperationView, error) {
	return s.command(ctx, StateApplied, c)
}

func (s *Service) Clear(ctx context.Context, c Command) (OperationView, error) {
	return s.command(ctx, StateCleared, c)
}

func (s *Service) command(ctx context.Context, state string, c Command) (OperationView, error) {
	if err := validate(c); err != nil {
		return OperationView{}, err
	}
	fp := fingerprint(state, c)
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return OperationView{}, err
	}
	defer tx.Rollback(ctx)

	if prior, priorFP, err := readOperation(ctx, tx, c.OperationReference); err == nil {
		if priorFP != fp {
			return OperationView{}, ErrConflict
		}
		return prior, tx.Commit(ctx)
	} else if !errors.Is(err, ErrNotFound) {
		return OperationView{}, err
	}

	var current View
	err = tx.QueryRow(ctx, `SELECT id::text,organization_id::text,subject_type,subject_reference,state,
		reason_code,source_reference,source_version,version,applied_at,cleared_at
		FROM dispatch_authority_restrictions
		WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 FOR UPDATE`,
		c.OrganizationID, c.SubjectType, c.SubjectReference).Scan(&current.ID, &current.OrganizationID,
		&current.SubjectType, &current.SubjectReference, &current.State, &current.ReasonCode,
		&current.SourceReference, &current.SourceVersion, &current.Version, &current.AppliedAt, &current.ClearedAt)

	outcome := "CONFIRMED"
	if errors.Is(err, pgx.ErrNoRows) {
		if state == StateCleared {
			return OperationView{}, ErrNotFound
		}
		err = tx.QueryRow(ctx, `INSERT INTO dispatch_authority_restrictions
			(organization_id,subject_type,subject_reference,state,reason_code,source_reference,source_version,applied_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,now())
			RETURNING id::text,version`, c.OrganizationID, c.SubjectType, c.SubjectReference, state,
			c.ReasonCode, c.SourceReference, c.SourceVersion).Scan(&current.ID, &current.Version)
	} else if err == nil {
		if c.SourceVersion < current.SourceVersion {
			outcome = "SUPERSEDED"
		} else if c.SourceVersion == current.SourceVersion && current.State != state {
			return OperationView{}, ErrConflict
		} else if c.SourceVersion > current.SourceVersion || current.State != state {
			err = tx.QueryRow(ctx, `UPDATE dispatch_authority_restrictions SET state=$2,reason_code=$3,
				source_reference=$4,source_version=$5,version=version+1,
				applied_at=CASE WHEN $2='APPLIED' THEN now() ELSE applied_at END,
				cleared_at=CASE WHEN $2='CLEARED' THEN now() ELSE NULL END,updated_at=now()
				WHERE id=$1::uuid RETURNING version`, current.ID, state, c.ReasonCode,
				c.SourceReference, c.SourceVersion).Scan(&current.Version)
		}
	}
	if err != nil {
		return OperationView{}, fmt.Errorf("store restriction: %w", err)
	}

	if outcome == "CONFIRMED" {
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_authority_restriction_history
			(restriction_id,organization_id,subject_type,subject_reference,state,reason_code,
			source_reference,source_version,restriction_version)
			VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9)`, current.ID, c.OrganizationID,
			c.SubjectType, c.SubjectReference, state, c.ReasonCode, c.SourceReference,
			c.SourceVersion, current.Version)
		if err != nil {
			return OperationView{}, err
		}
	}
	view := OperationView{c.OperationReference, current.ID, state, outcome, c.SourceVersion, current.Version}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_authority_restriction_operations
		(operation_reference,restriction_id,requested_state,request_fingerprint,outcome,source_version,restriction_version)
		VALUES($1,$2::uuid,$3,$4,$5,$6,$7)`, c.OperationReference, current.ID, state, fp, outcome,
		c.SourceVersion, current.Version)
	if err != nil {
		return OperationView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OperationView{}, err
	}
	return view, nil
}

func readOperation(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ref string) (OperationView, string, error) {
	var v OperationView
	var fp string
	err := q.QueryRow(ctx, `SELECT operation_reference,restriction_id::text,requested_state,outcome,
		source_version,restriction_version,request_fingerprint FROM dispatch_authority_restriction_operations
		WHERE operation_reference=$1`, ref).Scan(&v.OperationReference, &v.RestrictionID,
		&v.RequestedState, &v.Outcome, &v.SourceVersion, &v.RestrictionVersion, &fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, fp, ErrNotFound
	}
	return v, fp, err
}

func (s *Service) ReadOperation(ctx context.Context, ref string) (OperationView, error) {
	v, _, err := readOperation(ctx, s.db, ref)
	return v, err
}

func (s *Service) Read(ctx context.Context, organizationID, subjectType, subjectReference string) (View, error) {
	var v View
	err := s.db.QueryRow(ctx, `SELECT id::text,organization_id::text,subject_type,subject_reference,state,
		reason_code,source_reference,source_version,version,applied_at,cleared_at
		FROM dispatch_authority_restrictions WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3`,
		organizationID, subjectType, subjectReference).Scan(&v.ID, &v.OrganizationID, &v.SubjectType,
		&v.SubjectReference, &v.State, &v.ReasonCode, &v.SourceReference, &v.SourceVersion,
		&v.Version, &v.AppliedAt, &v.ClearedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// Restricted is the local commit-time gate. Empty organizationID means any
// applied restriction for the subject is authoritative (used by availability).
func (s *Service) Restricted(ctx context.Context, organizationID, subjectType, subjectReference string) (bool, error) {
	var restricted bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dispatch_authority_restrictions
		WHERE ($1='' OR organization_id=$1) AND subject_type=$2 AND subject_reference=$3 AND state='APPLIED')`,
		organizationID, subjectType, subjectReference).Scan(&restricted)
	return restricted, err
}
