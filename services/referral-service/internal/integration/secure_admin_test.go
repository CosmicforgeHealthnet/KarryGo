package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/referral-service/internal/features/admincommands"
	"cosmicforge/logistics/services/referral-service/internal/features/adminresources"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"github.com/google/uuid"
)

func TestReferralSecureAdminOwnerBoundary(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	person := uuid.NewString()
	staff := "referral-test-staff-" + uuid.NewString()
	_, err := pool.Exec(ctx, `INSERT INTO referral_persons(id,status,home_actor_type,home_actor_id) VALUES($1,'active','customer',$2)`, person, "customer-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"referral.persons.read", "referral.rewards.read", "referral.review.manage", "referral.reward.reverse"} {
		_, err = pool.Exec(ctx, `INSERT INTO referral_admin_staff_grants(id,staff_ref,permission,granted_by,approval_ref) VALUES($1,$2,$3,'test-owner','test-only-approval')`, uuid.NewString(), staff, permission)
		if err != nil {
			t.Fatal(err)
		}
	}

	scope := adminpolicy.Scope{Product: "karryam", Market: "TEST-MARKET", Zone: "TEST-ZONE", Organization: "TEST-ORGANIZATION", Tenant: "TEST-TENANT"}
	spec := admincommands.Spec()
	policy := policyfixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	policy.Document.ID = "section15-" + uuid.NewString()
	for _, capability := range []string{spec.ReadCapability, adminowner.ReferralProfile().Definitions["persons"].Capability} {
		rule := policy.Document.Rules[0]
		rule.Capability, rule.Permission = capability, capability
		if capability != spec.ReadCapability {
			rule.SourceContract = adminowner.ReferralProfile().Definitions["persons"].SourceContract
		}
		policy.Document.Rules = append(policy.Document.Rules, rule)
	}
	policy.Sign(t)
	issuer := contextfixture.New(t, spec.Owner)
	contexts := &admincontext.Store{DB: pool, Authority: issuer.Authority, Reader: adminresources.Source()}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(ctx, tx, "persons", person)
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := issuer.Approval("section15-"+uuid.NewString(), "persons", person, spec.Capability, binding, scope)
	approval.Capabilities = append(approval.Capabilities, spec.ReadCapability, adminowner.ReferralProfile().Definitions["persons"].Capability)
	if err = contexts.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}

	actor := adminpolicy.Actor{Product: "karryam", Staff: staff, Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	service := &admincommand.Service{Spec: spec, DB: pool, Policy: policy.Engine, Contexts: contexts, Executor: admincommands.Executor{Referral: newService(t, pool)}, WriteEnabled: true, ReadEnabled: true}
	command := admincommand.Command{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: "persons", SubjectID: person, Action: "FREEZE_PERSON", ExpectedVersion: 1, IdempotencyKey: uuid.NewString(), ReasonCode: "FRAUD_REVIEW", Params: map[string]string{}, RequestedAt: time.Now().UTC().Add(-time.Second)}

	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, runErr := service.Execute(ctx, actor, "case-1", scope, command)
			if runErr == nil && (receipt.PriorState != "ACTIVE" || receipt.ResultingState != "FROZEN" || receipt.RecordedVersion != 2) {
				runErr = errors.New("invalid owner receipt")
			}
			results <- runErr
		}()
	}
	wg.Wait()
	close(results)
	for runErr := range results {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}
	var status string
	var version int64
	if err = pool.QueryRow(ctx, `SELECT status,version FROM referral_persons WHERE id=$1`, person).Scan(&status, &version); err != nil || status != "frozen" || version != 2 {
		t.Fatalf("duplicate transition status=%s version=%d err=%v", status, version, err)
	}
	var decisions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM admin_owner_command_decisions WHERE subject_id=$1`, person).Scan(&decisions); err != nil || decisions != 1 {
		t.Fatalf("immutable decision count=%d err=%v", decisions, err)
	}

	stale := command
	stale.OperationID, stale.IdempotencyKey = uuid.NewString(), uuid.NewString()
	if _, err = service.Execute(ctx, actor, "case-1", scope, stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	wrong := scope
	wrong.Tenant = "OTHER-TENANT"
	if _, err = service.Execute(ctx, actor, "case-1", wrong, stale); err == nil {
		t.Fatal("cross-tenant command accepted")
	}

	clear := admincommand.Command{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: "persons", SubjectID: person, Action: "CLEAR_PERSON", ExpectedVersion: 2, IdempotencyKey: uuid.NewString(), ReasonCode: "REVIEW_CLEARED", Params: map[string]string{}, RequestedAt: time.Now().UTC().Add(-time.Second)}
	clear.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: "independent-referral-reviewer", IntentDigest: clear.IntentDigest(actor.Staff, "case-1", scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
	if receipt, runErr := service.Execute(ctx, actor, "case-1", scope, clear); runErr != nil || receipt.ResultingState != "ACTIVE" {
		t.Fatalf("approved clear failed: %+v %v", receipt, runErr)
	}
	selfApproved := clear
	selfApproved.OperationID, selfApproved.IdempotencyKey, selfApproved.Approval.ApprovalID = uuid.NewString(), uuid.NewString(), uuid.NewString()
	selfApproved.ExpectedVersion = 3
	selfApproved.Approval.ApproverRef = actor.Staff
	selfApproved.Approval.IntentDigest = selfApproved.IntentDigest(actor.Staff, "case-1", scope)
	if _, err = service.Execute(ctx, actor, "case-1", scope, selfApproved); err == nil {
		t.Fatal("self approval accepted")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM admin_owner_command_decisions WHERE subject_id=$1`, person); err == nil {
		t.Fatal("immutable command evidence was deletable")
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := adminresources.Source().Read(ctx, tx, "persons", person, true)
	_ = tx.Rollback(ctx)
	if err != nil || resource.Reference != person || resource.SourceVersion == nil || *resource.SourceVersion != 3 {
		t.Fatalf("source readback: %+v %v", resource, err)
	}

	// Regression: history runs in a read-only owner transaction; the native
	// grant check must not take a row lock there.
	history, err := service.History(ctx, actor, "case-1", scope, "persons", person)
	if err != nil || len(history.Decisions) != 2 || history.CurrentVersion != 3 {
		t.Fatalf("read-only command history: %+v %v", history, err)
	}

	// A paid reward reversal remains Referral's negative ledger request until
	// Wallet posts it. The Admin command must not manufacture Wallet success.
	reward := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO reward_ledger(id,person_id,beneficiary_actor_type,beneficiary_actor_id,reward_type,balance,amount_kobo,status,idempotency_key,wallet_state,wallet_reference) VALUES($1,$2,'customer',$3,'customer_referral_bonus','promotional',2500,'available',$4,'posted','wallet-posted-test')`, reward, person, "customer-"+uuid.NewString(), "paid-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rewardBinding, err := adminresources.Source().Binding(ctx, tx, "rewards", reward)
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rewardContext := issuer.Approval("section15-"+uuid.NewString(), "rewards", reward, spec.Capability, rewardBinding, scope)
	rewardContext.Capabilities = append(rewardContext.Capabilities, spec.ReadCapability)
	if err = contexts.Import(ctx, issuer.Sign(t, rewardContext), time.Now()); err != nil {
		t.Fatal(err)
	}
	reverse := admincommand.Command{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: "rewards", SubjectID: reward, Action: "REVERSE_REWARD", ExpectedVersion: 1, IdempotencyKey: uuid.NewString(), ReasonCode: "FRAUD_CONFIRMED", Params: map[string]string{}, RequestedAt: time.Now().UTC().Add(-time.Second)}
	reverse.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: "independent-reward-reviewer", IntentDigest: reverse.IntentDigest(actor.Staff, "case-1", scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
	if receipt, runErr := service.Execute(ctx, actor, "case-1", scope, reverse); runErr != nil || receipt.ResultingState != "REVERSED" {
		t.Fatalf("reward reversal request failed: %+v %v", receipt, runErr)
	}
	var reversalAmount int64
	var walletState string
	if err = pool.QueryRow(ctx, `SELECT amount_kobo,wallet_state FROM reward_ledger WHERE reverses_reward_id=$1`, reward).Scan(&reversalAmount, &walletState); err != nil || reversalAmount != -2500 || walletState != "pending" {
		t.Fatalf("wallet authority not preserved amount=%d state=%s err=%v", reversalAmount, walletState, err)
	}
}

func TestReferralAdminResourceDictionaryReadsNativeRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	person, referred := uuid.NewString(), uuid.NewString()
	actor, referredActor := uuid.NewString(), uuid.NewString()
	referral, reward, flag := uuid.NewString(), uuid.NewString(), uuid.NewString()
	code := "T" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO referral_persons(id,status,home_actor_type,home_actor_id) VALUES($1,'active','customer',$2)`, []any{person, actor}},
		{`INSERT INTO referral_persons(id,status,home_actor_type,home_actor_id) VALUES($1,'active','customer',$2)`, []any{referred, referredActor}},
		{`INSERT INTO referral_actors(actor_type,actor_id,person_id,registered_at,is_new_account,attribution_outcome) VALUES('customer',$1,$2,now(),true,'attributed')`, []any{actor, person}},
		{`INSERT INTO referral_actors(actor_type,actor_id,person_id,registered_at,is_new_account,attribution_outcome) VALUES('customer',$1,$2,now(),true,'attributed')`, []any{referredActor, referred}},
		{`INSERT INTO referral_codes(code,person_id) VALUES($1,$2)`, []any{code, person}},
		{`INSERT INTO referrals(id,referrer_person_id,referred_person_id,code,code_source,attributed_at,status) VALUES($1,$2,$3,$4,'typed',now(),'active')`, []any{referral, person, referred, code}},
		{`INSERT INTO referral_tracks(referral_id,model,status,jobs_required,jobs_counted) VALUES($1,'customer','qualified',1,1)`, []any{referral}},
		{`INSERT INTO reward_ledger(id,person_id,beneficiary_actor_type,beneficiary_actor_id,referral_id,reward_type,balance,amount_kobo,status,idempotency_key) VALUES($1,$2,'customer',$3,$4,'customer_referral_bonus','promotional',1000,'pending_qualification',$5)`, []any{reward, person, actor, referral, "resource-" + uuid.NewString()}},
		{`INSERT INTO fraud_flags(id,person_id,referral_id,flag_type,severity,status) VALUES($1,$2,$3,'manual','review','open')`, []any{flag, person, referral}},
		{`INSERT INTO referral_milestones(person_id,milestone,achieved_at,reward_id) VALUES($1,'qualified_5',now(),$2)`, []any{person, reward}},
		{`INSERT INTO reconciliation_mismatches(id,actor_type,actor_id,posted_credit_kobo,posted_reversal_kobo,in_flight_credit_kobo,in_flight_reversal_kobo,wallet_credit_kobo,wallet_reversal_kobo) VALUES($1,'customer',$2,1000,0,0,0,0,0)`, []any{uuid.NewString(), actor}},
		{`INSERT INTO referral_admin_actions(id,action,subject_kind,subject_id,performed_by,caller,reason) VALUES($1,'freeze_person','person',$2,'test-staff','test-owner','TEST_ONLY')`, []any{uuid.NewString(), person}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	var mismatch, action string
	if err := pool.QueryRow(ctx, `SELECT id FROM reconciliation_mismatches WHERE actor_id=$1`, actor).Scan(&mismatch); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM referral_admin_actions WHERE subject_id=$1`, person).Scan(&action); err != nil {
		t.Fatal(err)
	}
	references := map[string]string{"persons": person, "accounts": "customer:" + actor, "referrals": referral, "tracks": referral + ":customer", "rewards": reward, "fraud-flags": flag, "milestones": person + ":qualified_5", "reconciliation-mismatches": mismatch, "admin-actions": action}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for kind, reference := range references {
		resource, readErr := adminresources.Source().Read(ctx, tx, kind, reference, true)
		if readErr != nil || resource.Reference != reference || resource.SnapshotETag == "" {
			t.Fatalf("%s/%s: %+v %v", kind, reference, resource, readErr)
		}
		if _, bindErr := adminresources.Source().Binding(ctx, tx, kind, reference); bindErr != nil {
			t.Fatalf("binding %s: %v", kind, bindErr)
		}
	}
}
