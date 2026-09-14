package fleetpartner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

var (
	ErrNotFound               = errors.New("fleetpartner: not found")
	ErrIdempotencyConflict    = errors.New("fleetpartner: idempotency conflict")
	ErrDuplicateCompletedWork = errors.New("fleetpartner: duplicate completed work")
)

type Repository struct {
	db database.Querier
}

func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

func (r *Repository) UpdateOperatingMode(ctx context.Context, q database.Querier, organizationID, mode string, expectedVersion int64) (int64, error) {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_organization_settings
		SET operating_mode=$3, version=version+1, updated_at=now()
		WHERE organization_id=$1 AND version=$2`,
		organizationID, expectedVersion, mode)
	if err != nil {
		return 0, fmt.Errorf("fleetpartner: update operating mode: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, concurrency.ErrVersionMismatch
	}
	return expectedVersion + 1, nil
}

func (r *Repository) OperatingSettings(ctx context.Context, organizationID string) (string, map[string]bool, int64, error) {
	var mode string
	var policyRaw []byte
	var version int64
	err := r.db.QueryRow(ctx, `SELECT operating_mode, financial_visibility_policy, version FROM fleet_partner_organization_settings WHERE organization_id=$1`, organizationID).Scan(&mode, &policyRaw, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, 0, ErrNotFound
	}
	if err != nil {
		return "", nil, 0, fmt.Errorf("fleetpartner: read operating settings: %w", err)
	}
	policy := map[string]bool{}
	_ = json.Unmarshal(policyRaw, &policy)
	return mode, policy, version, nil
}

func (r *Repository) VerifyMembership(ctx context.Context, organizationID, membershipID, providerID, service string) (string, int64, error) {
	var status, relationship string
	var version int64
	var storedProvider *string
	err := r.db.QueryRow(ctx, `
		SELECT status, relationship_type, provider_id, version
		FROM fleet_partner_organization_provider_memberships
		WHERE organization_id=$1 AND id=$2 AND service_category=$3`,
		organizationID, membershipID, service).Scan(&status, &relationship, &storedProvider, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, fmt.Errorf("fleetpartner: verify membership: %w", err)
	}
	if storedProvider == nil || *storedProvider != providerID {
		return "", 0, ErrNotFound
	}
	return relationship, version, nil
}

func (r *Repository) CreatePlan(ctx context.Context, q database.Querier, plan CompensationPlan, rules []CompensationRule) (CompensationPlan, error) {
	if plan.ID == "" {
		plan.ID = ids.New("cplan")
	}
	if plan.Version == 0 {
		plan.Version = 1
	}
	if plan.Status == "" {
		plan.Status = PlanDraft
	}
	if plan.EffectiveFrom.IsZero() {
		plan.EffectiveFrom = time.Now().UTC()
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_compensation_plans
		(id,organization_id,name,description,currency,status,version,effective_from,effective_to,created_by,approved_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		plan.ID, plan.OrganizationID, plan.Name, plan.Description, plan.Currency, plan.Status,
		plan.Version, plan.EffectiveFrom, plan.EffectiveTo, plan.CreatedBy, plan.ApprovedBy)
	if err != nil {
		return CompensationPlan{}, fmt.Errorf("fleetpartner: create plan: %w", err)
	}
	snapshot := SnapshotJSON(map[string]any{"plan": plan, "rules": rules})
	if _, err = r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_compensation_plan_versions(id,organization_id,plan_id,plan_version,snapshot,actor_reference)
		VALUES($1,$2,$3,$4,$5,$6)`,
		ids.New("cpv"), plan.OrganizationID, plan.ID, plan.Version, snapshot, plan.CreatedBy); err != nil {
		return CompensationPlan{}, fmt.Errorf("fleetpartner: create plan version: %w", err)
	}
	for _, rule := range rules {
		if rule.ID == "" {
			rule.ID = ids.New("cpr")
		}
		rule.OrganizationID = plan.OrganizationID
		rule.PlanID = plan.ID
		rule.PlanVersion = plan.Version
		if rule.Status == "" {
			rule.Status = PlanActive
		}
		if rule.EffectiveFrom.IsZero() {
			rule.EffectiveFrom = plan.EffectiveFrom
		}
		roles, _ := json.Marshal(rule.ApplicableRoles)
		services, _ := json.Marshal(rule.ApplicableServices)
		cfg, _ := json.Marshal(rule.Config)
		if _, err = r.q(q).Exec(ctx, `
			INSERT INTO fleet_partner_compensation_plan_rules
			(id,organization_id,plan_id,plan_version,rule_type,applicable_roles,applicable_services,calculation_configuration,status,effective_from,effective_to,version)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1)`,
			rule.ID, rule.OrganizationID, rule.PlanID, rule.PlanVersion, rule.RuleType,
			roles, services, cfg, rule.Status, rule.EffectiveFrom, rule.EffectiveTo); err != nil {
			return CompensationPlan{}, fmt.Errorf("fleetpartner: create rule: %w", err)
		}
	}
	return plan, nil
}

func (r *Repository) SetPlanStatus(ctx context.Context, q database.Querier, organizationID, planID, status, approvedBy string, expectedVersion int64) (int64, error) {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_compensation_plans
		SET status=$4, approved_by=COALESCE(NULLIF($5,''),approved_by), version=version+1, updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND version=$3`,
		organizationID, planID, expectedVersion, status, approvedBy)
	if err != nil {
		return 0, fmt.Errorf("fleetpartner: set plan status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, concurrency.ErrVersionMismatch
	}
	return expectedVersion + 1, nil
}

func (r *Repository) AssignPlan(ctx context.Context, q database.Querier, a MembershipCompensationAssignment) (MembershipCompensationAssignment, error) {
	if a.ID == "" {
		a.ID = ids.New("mca")
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_membership_compensation_assignments
		(id,membership_id,organization_id,compensation_plan_id,compensation_plan_version,service_context,currency,effective_from,effective_to,status,assigned_by,approved_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, a.MembershipID, a.OrganizationID, a.CompensationPlanID, a.CompensationPlanVersion,
		a.ServiceContext, a.Currency, a.EffectiveFrom, a.EffectiveTo, a.Status, a.AssignedBy, a.ApprovedBy)
	if err != nil {
		return MembershipCompensationAssignment{}, fmt.Errorf("fleetpartner: assign plan: %w", err)
	}
	return a, nil
}

func (r *Repository) ResolveAssignment(ctx context.Context, membershipID, organizationID, service, currency string, at time.Time) (MembershipCompensationAssignment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id,membership_id,organization_id,compensation_plan_id,compensation_plan_version,service_context,currency,effective_from,effective_to,status,assigned_by,approved_by,created_at,updated_at
		FROM fleet_partner_membership_compensation_assignments
		WHERE membership_id=$1 AND organization_id=$2 AND status='ACTIVE'
		  AND currency=$3 AND service_context IN ('*',$4)
		  AND effective_from <= $5 AND (effective_to IS NULL OR effective_to > $5)
		ORDER BY CASE WHEN service_context=$4 THEN 0 ELSE 1 END, effective_from DESC
		LIMIT 1`, membershipID, organizationID, currency, service, at)
	var a MembershipCompensationAssignment
	err := row.Scan(&a.ID, &a.MembershipID, &a.OrganizationID, &a.CompensationPlanID,
		&a.CompensationPlanVersion, &a.ServiceContext, &a.Currency, &a.EffectiveFrom,
		&a.EffectiveTo, &a.Status, &a.AssignedBy, &a.ApprovedBy, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MembershipCompensationAssignment{}, ErrNotFound
	}
	if err != nil {
		return MembershipCompensationAssignment{}, fmt.Errorf("fleetpartner: resolve assignment: %w", err)
	}
	return a, nil
}

func (r *Repository) RulesForPlan(ctx context.Context, organizationID, planID string, planVersion int64) ([]CompensationRule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id,organization_id,plan_id,plan_version,rule_type,applicable_roles,applicable_services,calculation_configuration,status,effective_from,effective_to,version,created_at,updated_at
		FROM fleet_partner_compensation_plan_rules
		WHERE organization_id=$1 AND plan_id=$2 AND plan_version=$3 AND status='ACTIVE'`,
		organizationID, planID, planVersion)
	if err != nil {
		return nil, fmt.Errorf("fleetpartner: list rules: %w", err)
	}
	defer rows.Close()
	var out []CompensationRule
	for rows.Next() {
		var rule CompensationRule
		var roles, services, cfg []byte
		if err := rows.Scan(&rule.ID, &rule.OrganizationID, &rule.PlanID, &rule.PlanVersion,
			&rule.RuleType, &roles, &services, &cfg, &rule.Status, &rule.EffectiveFrom,
			&rule.EffectiveTo, &rule.Version, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(roles, &rule.ApplicableRoles)
		_ = json.Unmarshal(services, &rule.ApplicableServices)
		_ = json.Unmarshal(cfg, &rule.Config)
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repository) InsertCompletedWork(ctx context.Context, q database.Querier, record CompletedWorkRecord) (CompletedWorkRecord, bool, error) {
	if record.ID == "" {
		record.ID = ids.New("cwk")
	}
	if record.Version == 0 {
		record.Version = 1
	}
	if record.WorkType == "" {
		record.WorkType = "MANAGED_WORK"
	}
	if record.IngestionStatus == "" {
		record.IngestionStatus = "RECORDED"
	}
	err := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_completed_work_records
		(id,organization_id,membership_id,provider_id,source_service,source_job_id,source_assignment_id,work_type,completed_at,currency,eligible_basis_minor,source_version,ingestion_status,request_fingerprint,idempotency_key,correlation_id,version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,1)
		RETURNING created_at`,
		record.ID, record.OrganizationID, record.MembershipID, record.ProviderID, record.SourceService,
		record.SourceJobID, record.SourceAssignmentID, record.WorkType, record.CompletedAt,
		record.Currency, record.EligibleBasisMinor, record.SourceVersion, record.IngestionStatus,
		record.RequestFingerprint, record.IdempotencyKey, record.CorrelationID).Scan(&record.CreatedAt)
	if err == nil {
		return record, false, nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return CompletedWorkRecord{}, false, fmt.Errorf("fleetpartner: insert completed work: %w", err)
	}
	existing, loadErr := r.CompletedWorkByIdempotency(ctx, record.OrganizationID, record.IdempotencyKey)
	if loadErr != nil {
		return CompletedWorkRecord{}, false, ErrDuplicateCompletedWork
	}
	if existing.RequestFingerprint != record.RequestFingerprint {
		return CompletedWorkRecord{}, false, ErrIdempotencyConflict
	}
	return existing, true, nil
}

func (r *Repository) CompletedWorkByIdempotency(ctx context.Context, organizationID, key string) (CompletedWorkRecord, error) {
	row := r.db.QueryRow(ctx, `SELECT id,organization_id,membership_id,provider_id,source_service,source_job_id,source_assignment_id,work_type,completed_at,currency,eligible_basis_minor,source_version,ingestion_status,request_fingerprint,idempotency_key,correlation_id,version,created_at FROM fleet_partner_completed_work_records WHERE organization_id=$1 AND idempotency_key=$2`, organizationID, key)
	return scanCompletedWork(row)
}

func (r *Repository) InsertResolution(ctx context.Context, q database.Querier, res CompensationResolution) (CompensationResolution, error) {
	if res.ID == "" {
		res.ID = ids.New("cres")
	}
	res.ResolutionVersion = 1
	res.ResolvedAt = time.Now().UTC()
	res.Status = "RESOLVED"
	matched, _ := json.Marshal(res.MatchedRules)
	basis, _ := json.Marshal(res.CalculationBasis)
	visibility, _ := json.Marshal(res.VisibilityPolicySnapshot)
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_compensation_resolutions
		(id,organization_id,completed_work_id,membership_id,compensation_plan_id,compensation_plan_version,matched_rules,provider_share_minor,organization_share_minor,currency,calculation_basis,visibility_policy_snapshot,resolved_at,resolution_version,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		res.ID, res.OrganizationID, res.CompletedWorkID, res.MembershipID, res.CompensationPlanID,
		res.CompensationPlanVersion, matched, res.ProviderShareMinor, res.OrganizationShareMinor,
		res.Currency, basis, visibility, res.ResolvedAt, res.ResolutionVersion, res.Status)
	if err != nil {
		return CompensationResolution{}, fmt.Errorf("fleetpartner: insert resolution: %w", err)
	}
	return res, nil
}

func (r *Repository) InsertSettlementInstruction(ctx context.Context, q database.Querier, in ManagedWorkSettlementInstruction) (ManagedWorkSettlementInstruction, bool, error) {
	if in.InstructionID == "" {
		in.InstructionID = ids.New("mwsi")
	}
	in.Status = "CREATED"
	in.Version = 1
	visibility, _ := json.Marshal(in.VisibilityPolicySnapshot)
	err := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_managed_work_settlement_instructions
		(instruction_id,organization_id,membership_id,provider_id,source_service,source_job_id,source_assignment_id,compensation_plan_id,compensation_plan_version,compensation_resolution_version,gross_basis_minor,provider_share_minor,organization_share_minor,platform_share_minor,currency,visibility_policy_snapshot,status,idempotency_key,request_fingerprint,version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,1)
		RETURNING created_at`,
		in.InstructionID, in.OrganizationID, in.MembershipID, in.ProviderID, in.SourceService,
		in.SourceJobID, in.SourceAssignmentID, in.CompensationPlanID, in.CompensationPlanVersion,
		in.CompensationResolutionVersion, in.GrossBasisMinor, in.ProviderShareMinor,
		in.OrganizationShareMinor, in.PlatformShareMinor, in.Currency, visibility, in.Status,
		in.IdempotencyKey, in.RequestFingerprint).Scan(&in.CreatedAt)
	if err == nil {
		return in, false, nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return ManagedWorkSettlementInstruction{}, false, fmt.Errorf("fleetpartner: insert instruction: %w", err)
	}
	return ManagedWorkSettlementInstruction{}, false, ErrIdempotencyConflict
}

func scanCompletedWork(row pgx.Row) (CompletedWorkRecord, error) {
	var r CompletedWorkRecord
	err := row.Scan(&r.ID, &r.OrganizationID, &r.MembershipID, &r.ProviderID, &r.SourceService,
		&r.SourceJobID, &r.SourceAssignmentID, &r.WorkType, &r.CompletedAt, &r.Currency,
		&r.EligibleBasisMinor, &r.SourceVersion, &r.IngestionStatus, &r.RequestFingerprint,
		&r.IdempotencyKey, &r.CorrelationID, &r.Version, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CompletedWorkRecord{}, ErrNotFound
	}
	return r, err
}

// RecordWorkSettledComponents persists the components a completed job paid
// with its own money (CMP-027).
//
// Written in the same transaction as the resolution that produced them, so
// there is no window in which a component has been settled but payroll cannot
// see that it was. The unique index makes a replayed resolution a no-op rather
// than a second payment record.
func (r *Repository) RecordWorkSettledComponents(
	ctx context.Context, q database.Querier, components []WorkSettledComponent,
) error {
	for _, component := range components {
		if _, err := r.q(q).Exec(ctx, `
			INSERT INTO fleet_partner_work_settled_components
			(id,organization_id,membership_id,source_service,source_job_id,source_assignment_id,
			 completed_work_id,compensation_resolution_id,settlement_instruction_id,wallet_posting_operation_id,
			 rule_id,component,settlement_level,settlement_strategy,amount_minor,currency,settled_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			ON CONFLICT (organization_id,source_service,source_job_id,rule_id) DO NOTHING`,
			ids.New("wsc"), component.OrganizationID, component.MembershipID,
			component.SourceService, component.SourceJobID, component.SourceAssignmentID,
			component.CompletedWorkID, component.CompensationResolutionID,
			component.SettlementInstructionID, component.WalletPostingOperationID,
			component.RuleID, component.Component, component.SettlementLevel,
			component.Strategy, component.AmountMinor, component.Currency,
			time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("fleetpartner: record work settled component: %w", err)
		}
	}
	return nil
}

// WorkSettledComponentsInPeriod reads everything a membership was already paid
// with job money inside a payroll window.
//
// Payroll calls this before it calculates, and skips every component it names.
func (r *Repository) WorkSettledComponentsInPeriod(
	ctx context.Context, q database.Querier, organizationID, membershipID string, from, to time.Time,
) ([]WorkSettledComponent, error) {
	rows, err := r.q(q).Query(ctx, `
		SELECT organization_id,membership_id,source_service,source_job_id,source_assignment_id,
		       completed_work_id,compensation_resolution_id,settlement_instruction_id,wallet_posting_operation_id,
		       rule_id,component,settlement_level,settlement_strategy,amount_minor,currency
		FROM fleet_partner_work_settled_components
		WHERE organization_id=$1 AND ($2='' OR membership_id=$2)
		  AND settled_at >= $3 AND settled_at < $4
		ORDER BY settled_at`,
		organizationID, membershipID, from, to)
	if err != nil {
		return nil, fmt.Errorf("fleetpartner: read work settled components: %w", err)
	}
	defer rows.Close()
	var out []WorkSettledComponent
	for rows.Next() {
		var component WorkSettledComponent
		if err := rows.Scan(
			&component.OrganizationID, &component.MembershipID,
			&component.SourceService, &component.SourceJobID, &component.SourceAssignmentID,
			&component.CompletedWorkID, &component.CompensationResolutionID,
			&component.SettlementInstructionID, &component.WalletPostingOperationID,
			&component.RuleID, &component.Component, &component.SettlementLevel,
			&component.Strategy, &component.AmountMinor, &component.Currency,
		); err != nil {
			return nil, err
		}
		out = append(out, component)
	}
	return out, rows.Err()
}

// WorkSettledExclusionsForAssignments reads the components already paid with
// job money for a specific set of Fleet assignments.
//
// Payroll uses this rather than a time window: a job completed inside the
// period may be settled moments after the period's end, and a window read
// would miss it and pay it a second time.
func (r *Repository) WorkSettledExclusionsForAssignments(
	ctx context.Context, q database.Querier, organizationID string, assignmentIDs []string,
) (map[string]WorkSettledComponent, error) {
	if organizationID == "" || len(assignmentIDs) == 0 {
		return map[string]WorkSettledComponent{}, nil
	}
	rows, err := r.q(q).Query(ctx, `
		SELECT organization_id,membership_id,source_service,source_job_id,source_assignment_id,
		       completed_work_id,compensation_resolution_id,settlement_instruction_id,wallet_posting_operation_id,
		       rule_id,component,settlement_level,settlement_strategy,amount_minor,currency
		FROM fleet_partner_work_settled_components
		WHERE organization_id=$1 AND source_assignment_id = ANY($2)`,
		organizationID, assignmentIDs)
	if err != nil {
		return nil, fmt.Errorf("fleetpartner: read work settled exclusions: %w", err)
	}
	defer rows.Close()
	var settled []WorkSettledComponent
	for rows.Next() {
		var component WorkSettledComponent
		if err := rows.Scan(
			&component.OrganizationID, &component.MembershipID,
			&component.SourceService, &component.SourceJobID, &component.SourceAssignmentID,
			&component.CompletedWorkID, &component.CompensationResolutionID,
			&component.SettlementInstructionID, &component.WalletPostingOperationID,
			&component.RuleID, &component.Component, &component.SettlementLevel,
			&component.Strategy, &component.AmountMinor, &component.Currency,
		); err != nil {
			return nil, err
		}
		settled = append(settled, component)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return PayrollExclusions(settled), nil
}
