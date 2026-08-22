package taxonomy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type governanceCommandEnvelope struct {
	Operation string `json:"operation"`

	CaseID string `json:"case_id,omitempty"`

	NamespaceCode NamespaceCode `json:"namespace_code,omitempty"`

	SubjectType string `json:"subject_type,omitempty"`
	SubjectRef  string `json:"subject_ref,omitempty"`

	CaseType GovernanceCaseType `json:"case_type,omitempty"`
	Priority GovernancePriority `json:"priority,omitempty"`

	ExpectedRevision int64 `json:"expected_revision,omitempty"`

	Transition GovernanceTransition `json:"transition,omitempty"`

	AssignedActorRef string `json:"assigned_actor_ref,omitempty"`

	ResolutionCode  string `json:"resolution_code,omitempty"`
	ResolutionNotes string `json:"resolution_notes,omitempty"`

	ReasonCode  string `json:"reason_code,omitempty"`
	Description string `json:"description,omitempty"`

	Metadata json.RawMessage `json:"metadata,omitempty"`
}

func normalizeGovernanceMetadata(
	raw json.RawMessage,
) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var object map[string]any

	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, ErrInvalidArgument
	}

	return json.Marshal(object)
}

func (s *Service) OpenGovernanceCase(
	ctx context.Context,
	input OpenGovernanceCaseInput,
) (GovernanceCommandResult, error) {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(input.SubjectType) == "" ||
		strings.TrimSpace(input.SubjectRef) == "" ||
		strings.TrimSpace(input.Actor.ActorRef) == "" ||
		strings.TrimSpace(input.Actor.CommandRef) == "" ||
		!validGovernanceCaseType(input.CaseType) ||
		!validGovernancePriority(input.Priority) {
		return GovernanceCommandResult{},
			ErrInvalidArgument
	}

	if input.NamespaceCode != NamespaceFood &&
		input.NamespaceCode != NamespaceShop {
		return GovernanceCommandResult{},
			ErrInvalidArgument
	}

	metadata, err :=
		normalizeGovernanceMetadata(
			input.Metadata,
		)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	subjectType := strings.ToUpper(
		strings.TrimSpace(input.SubjectType),
	)
	subjectRef := strings.TrimSpace(
		input.SubjectRef,
	)

	envelope := governanceCommandEnvelope{
		Operation: "OPEN_CASE",

		NamespaceCode: input.NamespaceCode,

		SubjectType: subjectType,

		SubjectRef: subjectRef,

		CaseType: input.CaseType,

		Priority: input.Priority,

		ReasonCode: valueOrEmpty(input.ReasonCode),

		Description: valueOrEmpty(input.Description),

		Metadata: metadata,
	}

	requestHash, err :=
		governanceRequestHash(envelope)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	tx, err := s.repo.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		},
	)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err = tx.Exec(
		ctx,
		`
        SELECT pg_advisory_xact_lock(
            hashtext($1)
        )
        `,
		"taxonomy-governance-command|"+
			strings.TrimSpace(
				input.Actor.CommandRef,
			),
	); err != nil {
		return GovernanceCommandResult{},
			err
	}

	replayed, result, err :=
		readGovernanceCommandReplay(
			ctx,
			tx,
			strings.TrimSpace(
				input.Actor.CommandRef,
			),
			requestHash,
		)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	if replayed {
		result.Replayed = true

		return result, nil
	}

	var namespaceID string
	var versionID string

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            ns.id,
            v.id
          FROM canonical_taxonomy_namespaces ns
          JOIN canonical_taxonomy_versions v
            ON v.namespace_id=ns.id
         WHERE ns.code=$1
           AND ns.status='ACTIVE'
           AND v.status='PUBLISHED'
         ORDER BY v.version_number DESC
         LIMIT 1
        `,
		input.NamespaceCode,
	).Scan(
		&namespaceID,
		&versionID,
	)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	if _, err = tx.Exec(
		ctx,
		`
        SELECT pg_advisory_xact_lock(
            hashtext($1)
        )
        `,
		strings.Join(
			[]string{
				"taxonomy-governance-subject",
				namespaceID,
				versionID,
				subjectType,
				subjectRef,
				string(input.CaseType),
			},
			"|",
		),
	); err != nil {
		return GovernanceCommandResult{},
			err
	}

	caseID := deterministicGovernanceCaseID(
		input.NamespaceCode,
		versionID,
		subjectType,
		subjectRef,
		input.CaseType,
	)

	var c GovernanceCase

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO canonical_taxonomy_governance_cases (
            id,
            namespace_id,
            version_id,
            subject_type,
            subject_ref,
            case_type,
            status,
            priority,
            reason_code,
            description,
            metadata,
            revision,
            opened_by_actor_ref
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,
            'OPEN',$7,
            NULLIF($8,''),
            NULLIF($9,''),
            $10,1,$11
        )
        RETURNING
            id,
            namespace_id,
            version_id,
            subject_type,
            subject_ref,
            case_type,
            status,
            priority,
            reason_code,
            description,
            resolution_code,
            resolution_notes,
            assigned_actor_ref,
            opened_by_actor_ref,
            resolved_by_actor_ref,
            metadata,
            revision,
            created_at,
            updated_at,
            resolved_at
        `,
		caseID,
		namespaceID,
		versionID,
		subjectType,
		subjectRef,
		input.CaseType,
		input.Priority,
		valueOrEmpty(input.ReasonCode),
		valueOrEmpty(input.Description),
		metadata,
		input.Actor.ActorRef,
	).Scan(
		&c.ID,
		&c.NamespaceID,
		&c.VersionID,
		&c.SubjectType,
		&c.SubjectRef,
		&c.CaseType,
		&c.Status,
		&c.Priority,
		&c.ReasonCode,
		&c.Description,
		&c.ResolutionCode,
		&c.ResolutionNotes,
		&c.AssignedActorRef,
		&c.OpenedByActorRef,
		&c.ResolvedByActorRef,
		&c.Metadata,
		&c.Revision,
		&c.CreatedAt,
		&c.UpdatedAt,
		&c.ResolvedAt,
	)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	result = GovernanceCommandResult{
		Case:     c,
		Replayed: false,
	}

	if err = insertGovernanceCommand(
		ctx,
		tx,
		input.Actor,
		"OPEN_CASE",
		caseID,
		requestHash,
		result,
	); err != nil {
		return GovernanceCommandResult{},
			err
	}

	if err = tx.Commit(ctx); err != nil {
		return GovernanceCommandResult{},
			err
	}

	return result, nil
}

func (s *Service) TransitionGovernanceCase(
	ctx context.Context,
	input TransitionGovernanceCaseInput,
) (GovernanceCommandResult, error) {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(input.CaseID) == "" ||
		input.ExpectedRevision < 1 ||
		strings.TrimSpace(input.Actor.ActorRef) == "" ||
		strings.TrimSpace(input.Actor.CommandRef) == "" ||
		!validGovernanceTransition(input.Transition) {
		return GovernanceCommandResult{},
			ErrInvalidArgument
	}

	metadata, err :=
		normalizeGovernanceMetadata(
			input.Metadata,
		)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	envelope := governanceCommandEnvelope{
		Operation: string(input.Transition),

		CaseID: strings.TrimSpace(input.CaseID),

		ExpectedRevision: input.ExpectedRevision,

		Transition: input.Transition,

		AssignedActorRef: valueOrEmpty(
			input.AssignedActorRef,
		),

		ResolutionCode: valueOrEmpty(
			input.ResolutionCode,
		),

		ResolutionNotes: valueOrEmpty(
			input.ResolutionNotes,
		),

		Metadata: metadata,
	}

	requestHash, err :=
		governanceRequestHash(envelope)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	tx, err := s.repo.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		},
	)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err = tx.Exec(
		ctx,
		`
        SELECT pg_advisory_xact_lock(
            hashtext($1)
        )
        `,
		"taxonomy-governance-command|"+
			strings.TrimSpace(
				input.Actor.CommandRef,
			),
	); err != nil {
		return GovernanceCommandResult{},
			err
	}

	replayed, result, err :=
		readGovernanceCommandReplay(
			ctx,
			tx,
			strings.TrimSpace(
				input.Actor.CommandRef,
			),
			requestHash,
		)
	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	if replayed {
		result.Replayed = true

		return result, nil
	}

	if _, err = tx.Exec(
		ctx,
		`
        SELECT pg_advisory_xact_lock(
            hashtext($1)
        )
        `,
		"taxonomy-governance-case|"+
			strings.TrimSpace(
				input.CaseID,
			),
	); err != nil {
		return GovernanceCommandResult{},
			err
	}

	var nextStatus GovernanceStatus
	var assignedActor string
	var resolutionCode string
	var resolutionNotes string
	var resolvedActor string

	switch input.Transition {
	case GovernanceTransitionStartReview:
		nextStatus =
			GovernanceStatusInReview

		assignedActor =
			valueOrEmpty(
				input.AssignedActorRef,
			)

		if assignedActor == "" {
			assignedActor =
				input.Actor.ActorRef
		}

	case GovernanceTransitionAssign:
		assignedActor =
			valueOrEmpty(
				input.AssignedActorRef,
			)

		if assignedActor == "" {
			return GovernanceCommandResult{},
				ErrInvalidArgument
		}

	case GovernanceTransitionResolve:
		nextStatus =
			GovernanceStatusResolved

		resolutionCode =
			strings.ToUpper(
				valueOrEmpty(
					input.ResolutionCode,
				),
			)

		if resolutionCode == "" {
			return GovernanceCommandResult{},
				ErrInvalidArgument
		}

		resolutionNotes =
			valueOrEmpty(
				input.ResolutionNotes,
			)

		resolvedActor =
			input.Actor.ActorRef

	case GovernanceTransitionReject:
		nextStatus =
			GovernanceStatusRejected

		resolutionCode =
			strings.ToUpper(
				valueOrEmpty(
					input.ResolutionCode,
				),
			)

		if resolutionCode == "" {
			return GovernanceCommandResult{},
				ErrInvalidArgument
		}

		resolutionNotes =
			valueOrEmpty(
				input.ResolutionNotes,
			)

		resolvedActor =
			input.Actor.ActorRef

	case GovernanceTransitionCancel:
		nextStatus =
			GovernanceStatusCancelled

		resolutionNotes =
			valueOrEmpty(
				input.ResolutionNotes,
			)

		resolvedActor =
			input.Actor.ActorRef
	}

	var c GovernanceCase

	if input.Transition ==
		GovernanceTransitionAssign {

		err = tx.QueryRow(
			ctx,
			`
            UPDATE canonical_taxonomy_governance_cases
               SET assigned_actor_ref=$1
             WHERE id=$2
               AND revision=$3
               AND status IN (
                   'OPEN',
                   'IN_REVIEW'
               )
            RETURNING
                id,
                namespace_id,
                version_id,
                subject_type,
                subject_ref,
                case_type,
                status,
                priority,
                reason_code,
                description,
                resolution_code,
                resolution_notes,
                assigned_actor_ref,
                opened_by_actor_ref,
                resolved_by_actor_ref,
                metadata,
                revision,
                created_at,
                updated_at,
                resolved_at
            `,
			assignedActor,
			input.CaseID,
			input.ExpectedRevision,
		).Scan(
			&c.ID,
			&c.NamespaceID,
			&c.VersionID,
			&c.SubjectType,
			&c.SubjectRef,
			&c.CaseType,
			&c.Status,
			&c.Priority,
			&c.ReasonCode,
			&c.Description,
			&c.ResolutionCode,
			&c.ResolutionNotes,
			&c.AssignedActorRef,
			&c.OpenedByActorRef,
			&c.ResolvedByActorRef,
			&c.Metadata,
			&c.Revision,
			&c.CreatedAt,
			&c.UpdatedAt,
			&c.ResolvedAt,
		)
	} else {
		err = tx.QueryRow(
			ctx,
			`
            UPDATE canonical_taxonomy_governance_cases
               SET status=$1,
                   assigned_actor_ref=
                       CASE
                           WHEN NULLIF($2,'') IS NULL
                               THEN assigned_actor_ref
                           ELSE $2
                       END,
                   resolution_code=
                       NULLIF($3,''),
                   resolution_notes=
                       NULLIF($4,''),
                   resolved_by_actor_ref=
                       NULLIF($5,''),
                   metadata=
                       CASE
                           WHEN $6::jsonb='{}'::jsonb
                               THEN metadata
                           ELSE $6::jsonb
                       END
             WHERE id=$7
               AND revision=$8
               AND status IN (
                   'OPEN',
                   'IN_REVIEW'
               )
            RETURNING
                id,
                namespace_id,
                version_id,
                subject_type,
                subject_ref,
                case_type,
                status,
                priority,
                reason_code,
                description,
                resolution_code,
                resolution_notes,
                assigned_actor_ref,
                opened_by_actor_ref,
                resolved_by_actor_ref,
                metadata,
                revision,
                created_at,
                updated_at,
                resolved_at
            `,
			nextStatus,
			assignedActor,
			resolutionCode,
			resolutionNotes,
			resolvedActor,
			metadata,
			input.CaseID,
			input.ExpectedRevision,
		).Scan(
			&c.ID,
			&c.NamespaceID,
			&c.VersionID,
			&c.SubjectType,
			&c.SubjectRef,
			&c.CaseType,
			&c.Status,
			&c.Priority,
			&c.ReasonCode,
			&c.Description,
			&c.ResolutionCode,
			&c.ResolutionNotes,
			&c.AssignedActorRef,
			&c.OpenedByActorRef,
			&c.ResolvedByActorRef,
			&c.Metadata,
			&c.Revision,
			&c.CreatedAt,
			&c.UpdatedAt,
			&c.ResolvedAt,
		)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return GovernanceCommandResult{},
			fmt.Errorf(
				"%w: governance case/revision/state conflict",
				ErrInvalidArgument,
			)
	}

	if err != nil {
		return GovernanceCommandResult{},
			err
	}

	result = GovernanceCommandResult{
		Case:     c,
		Replayed: false,
	}

	if err = insertGovernanceCommand(
		ctx,
		tx,
		input.Actor,
		string(input.Transition),
		input.CaseID,
		requestHash,
		result,
	); err != nil {
		return GovernanceCommandResult{},
			err
	}

	if err = tx.Commit(ctx); err != nil {
		return GovernanceCommandResult{},
			err
	}

	return result, nil
}

func readGovernanceCommandReplay(
	ctx context.Context,
	tx pgx.Tx,
	commandRef string,
	requestHash string,
) (
	bool,
	GovernanceCommandResult,
	error,
) {
	var existingHash string
	var snapshot []byte

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            request_hash,
            result_snapshot
          FROM canonical_taxonomy_governance_commands
         WHERE command_ref=$1
        `,
		commandRef,
	).Scan(
		&existingHash,
		&snapshot,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return false,
			GovernanceCommandResult{},
			nil
	}

	if err != nil {
		return false,
			GovernanceCommandResult{},
			err
	}

	if existingHash != requestHash {
		return false,
			GovernanceCommandResult{},
			fmt.Errorf(
				"%w: governance command_ref reused with different request",
				ErrInvalidArgument,
			)
	}

	var result GovernanceCommandResult

	if err = json.Unmarshal(
		snapshot,
		&result,
	); err != nil {
		return false,
			GovernanceCommandResult{},
			err
	}

	return true, result, nil
}

func insertGovernanceCommand(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	operation string,
	caseID string,
	requestHash string,
	result GovernanceCommandResult,
) error {
	snapshot, err :=
		json.Marshal(result)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO canonical_taxonomy_governance_commands (
            id,
            command_ref,
            actor_ref,
            operation,
            case_id,
            request_hash,
            result_snapshot
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,$7
        )
        `,
		semanticObjectID(
			"ctgovcmd_",
			strings.TrimSpace(
				actor.CommandRef,
			),
		),
		strings.TrimSpace(
			actor.CommandRef,
		),
		actor.ActorRef,
		operation,
		caseID,
		requestHash,
		snapshot,
	)

	return err
}
