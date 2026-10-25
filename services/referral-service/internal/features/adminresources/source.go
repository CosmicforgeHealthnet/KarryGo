// Package adminresources exposes Referral's fixed, privacy-minimized owner
// projections. It never returns raw identity fingerprints, referral codes,
// fraud evidence payloads or wallet request bodies.
package adminresources

import (
	"context"

	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "referral-service"

type SourceReader struct{ *adminowner.SQLSource }

func Source() *SourceReader {
	return &SourceReader{adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"persons":                   {Table: "referral_persons", StateColumn: "status", VersionColumn: "version", BindingColumns: []string{"home_actor_type", "home_actor_id"}, Dimensions: map[string]string{"home_actor_type": "home_actor_type"}, References: map[string]string{"home_actor": "home_actor_id"}},
		"accounts":                  {Table: "referral_actors", ReferenceExpression: "t.actor_type||':'||t.actor_id", StateColumn: "attribution_outcome", VersionColumn: "", UpdatedAtExpression: "t.created_at", BindingColumns: []string{"actor_type", "actor_id", "person_id"}, Dimensions: map[string]string{"actor_type": "actor_type", "is_new_account": "is_new_account", "attribution_outcome": "attribution_outcome"}, References: map[string]string{"person": "person_id"}},
		"referrals":                 {Table: "referrals", StateColumn: "status", VersionColumn: "version", BindingColumns: []string{"referrer_person_id", "referred_person_id"}, Dimensions: map[string]string{"code_source": "code_source"}, References: map[string]string{"referrer_person": "referrer_person_id", "referred_person": "referred_person_id"}},
		"tracks":                    {Table: "referral_tracks", ReferenceExpression: "t.referral_id||':'||t.model", StateColumn: "status", VersionColumn: "", BindingColumns: []string{"referral_id", "model"}, Dimensions: map[string]string{"model": "model", "window_starts_at": "window_starts_at", "window_ends_at": "window_ends_at"}, Counters: map[string]string{"jobs_required": "jobs_required", "jobs_counted": "jobs_counted"}, References: map[string]string{"referral": "referral_id"}},
		"rewards":                   {Table: "reward_ledger", StateColumn: "status", VersionColumn: "version", BindingColumns: []string{"person_id", "referral_id", "beneficiary_actor_type", "beneficiary_actor_id", "reverses_reward_id"}, Dimensions: map[string]string{"reward_type": "reward_type", "balance": "balance", "currency": "currency", "wallet_state": "wallet_state"}, Counters: map[string]string{"amount_minor": "amount_kobo"}, References: map[string]string{"person": "person_id", "referral": "referral_id", "beneficiary_actor": "beneficiary_actor_id", "reverses_reward": "reverses_reward_id", "wallet": "wallet_reference"}},
		"fraud-flags":               {Table: "fraud_flags", StateColumn: "status", VersionColumn: "version", UpdatedAtExpression: "COALESCE(t.resolved_at,t.created_at)", BindingColumns: []string{"person_id", "referral_id"}, Dimensions: map[string]string{"flag_type": "flag_type", "severity": "severity"}, References: map[string]string{"person": "person_id", "referral": "referral_id"}},
		"milestones":                {Table: "(SELECT x.*,CASE x.milestone WHEN 'qualified_5' THEN 5 ELSE 10 END AS threshold FROM referral_milestones x)", ReferenceExpression: "t.person_id||':'||t.milestone", StateColumn: "milestone", VersionColumn: "", CreatedAtExpression: "t.achieved_at", UpdatedAtExpression: "t.achieved_at", BindingColumns: []string{"person_id", "milestone", "reward_id"}, Dimensions: map[string]string{"reward_type": "milestone"}, Counters: map[string]string{"threshold": "threshold"}, References: map[string]string{"person": "person_id", "reward": "reward_id"}},
		"reconciliation-mismatches": {Table: "(SELECT m.*,a.person_id,CASE WHEN m.resolved_at IS NULL THEN 'OPEN' ELSE 'RESOLVED' END AS status,(m.posted_credit_kobo-m.posted_reversal_kobo+m.in_flight_credit_kobo-m.in_flight_reversal_kobo) AS referral_amount,(m.wallet_credit_kobo-m.wallet_reversal_kobo) AS wallet_amount,'WALLET_LEDGER_MISMATCH'::text AS reason FROM reconciliation_mismatches m JOIN referral_actors a ON a.actor_type=m.actor_type AND a.actor_id=m.actor_id)", StateColumn: "status", CreatedAtExpression: "t.first_seen_at", UpdatedAtExpression: "t.last_checked_at", BindingColumns: []string{"person_id", "actor_type", "actor_id"}, Dimensions: map[string]string{"reason": "reason"}, Counters: map[string]string{"referral_amount_minor": "referral_amount", "wallet_amount_minor": "wallet_amount"}, References: map[string]string{"person": "person_id"}},
		"admin-actions":             {Table: "referral_admin_actions", StateColumn: "action", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.created_at", BindingColumns: []string{"subject_kind", "subject_id"}, Dimensions: map[string]string{"action": "action", "subject_kind": "subject_kind"}, References: map[string]string{"subject": "subject_id", "operator": "performed_by"}},
	})}
}

var readPermission = map[string]string{
	"persons": "referral.persons.read", "accounts": "referral.accounts.read", "referrals": "referral.referrals.read", "tracks": "referral.tracks.read", "rewards": "referral.rewards.read", "fraud-flags": "referral.fraud_flags.read", "milestones": "referral.milestones.read", "reconciliation-mismatches": "referral.reconciliation.read", "admin-actions": "referral.admin_actions.read",
}

func requireGrant(ctx context.Context, tx pgx.Tx, staff, permission string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id FROM referral_admin_staff_grants WHERE staff_ref=$1 AND permission=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) ORDER BY id LIMIT 1 FOR SHARE`, staff, permission).Scan(&id)
}

func (s *SourceReader) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, _ string) error {
	permission := readPermission[kind]
	if permission == "" {
		return pgx.ErrNoRows
	}
	return requireGrant(ctx, tx, staff, permission)
}

func RequireMutationGrant(ctx context.Context, tx pgx.Tx, staff, permission string) error {
	return requireGrant(ctx, tx, staff, permission)
}

func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	p := adminowner.ReferralProfile()
	return adminowner.FromEnvironment(ctx, db, Source(), p.Definitions, Owner, p.Prefix, p.Path)
}

var _ adminowner.StaffAuthorizer = (*SourceReader)(nil)
