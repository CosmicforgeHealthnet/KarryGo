package ownercommandops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
)

type supportOwner struct{}

func (supportOwner) Spec() admincommand.Spec { return admincommand.SupportSpec() }
func (supportOwner) Execute(context.Context, adminread.Query, admincommand.Command) (admincommand.Receipt, error) {
	return admincommand.Receipt{}, errors.New("not used")
}
func (supportOwner) Readback(context.Context, adminread.Query, string, string, string) (admincommand.Receipt, error) {
	return admincommand.Receipt{}, errors.New("not used")
}
func (supportOwner) History(context.Context, adminread.Query, string, string) (admincommand.History, error) {
	return admincommand.History{}, errors.New("not used")
}

func TestPostgresSupportRemedyRequiresWalletConfirmedRefund(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	s := &Service{DB: db, Owner: supportOwner{}}
	q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}}
	dispute := uuid.NewString()
	insert := func(action, status, caseRef, amount string) string {
		id := uuid.NewString()
		receipt, rejection := "null", "null"
		if status == "CONFIRMED" {
			receipt = `'{}'::jsonb`
		}
		if status == "REJECTED" {
			rejection = `'owner_refused'`
		}
		_, err := db.Exec(ctx, `INSERT INTO admin_owner_command_operations(id,principal,owner,contract,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,subject_kind,subject_id,case_ref,action,version_key,expected_version,idempotency_key,intent_digest,operation_digest,command,status,claim_token,receipt,rejection_code)
VALUES($1,'cosmicforge-hq-karryam-admin','payment-wallet-service','wallet.admin-command.v1','staff-1','case-1','karryam','NG','Z1','org-1','org-1','journals',$2,$3,$4,'k',0,$5,$6,$6,jsonb_build_object('params',jsonb_build_object('amount_minor',$7::text)),$8,gen_random_uuid(),`+receipt+`,`+rejection+`)`,
			id, uuid.NewString(), caseRef, action, "idem-"+id, strings.Repeat("a", 64), amount, status)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	cmd := func(walletOp, amount string) admincommand.Command {
		return admincommand.Command{Action: "RECORD_DISPUTE_REMEDY", SubjectKind: "disputes", SubjectID: dispute, Params: map[string]string{"wallet_operation_id": walletOp, "amount_minor": amount}}
	}
	// Nothing in Wallet yet: refused.
	if err := s.walletRemedyConfirmed(ctx, q, cmd(uuid.NewString(), "5000")); !errors.Is(err, ErrRemedyUnconfirmed) {
		t.Fatalf("unbacked remedy accepted: %v", err)
	}
	insert("REQUEST_REFUND", "CONFIRMED", dispute, "5000")
	pending := insert("APPROVE_REFUND", "UNKNOWN_OUTCOME", "", "0")
	if err := s.walletRemedyConfirmed(ctx, q, cmd(pending, "5000")); !errors.Is(err, ErrRemedyUnconfirmed) {
		t.Fatalf("unconfirmed approval accepted: %v", err)
	}
	approved := insert("APPROVE_REFUND", "CONFIRMED", "", "0")
	if err := s.walletRemedyConfirmed(ctx, q, cmd(approved, "9999")); !errors.Is(err, ErrRemedyUnconfirmed) {
		t.Fatalf("amount mismatch accepted: %v", err)
	}
	if err := s.walletRemedyConfirmed(ctx, q, cmd(approved, "5000")); err != nil {
		t.Fatalf("confirmed refund refused: %v", err)
	}
	other := adminread.Query{Scope: adminread.Scope{Market: "GH", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}}
	if err := s.walletRemedyConfirmed(ctx, other, cmd(approved, "5000")); !errors.Is(err, ErrRemedyUnconfirmed) {
		t.Fatalf("cross-market refund accepted: %v", err)
	}
	// Non-remedy actions are unaffected.
	if err := s.walletRemedyConfirmed(ctx, q, admincommand.Command{Action: "ASSIGN_CASE"}); err != nil {
		t.Fatal(err)
	}
}
