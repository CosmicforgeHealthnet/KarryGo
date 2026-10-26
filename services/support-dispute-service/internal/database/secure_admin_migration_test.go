package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cosmicforge/logistics/services/support-dispute-service/internal/testdb"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminowner"
)

func read(t *testing.T, parts ...string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(append([]string{testdb.MigrationsDir()}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func sha(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// The owner ledger, context and outbox schemas are shared contracts: 009 must
// carry each one verbatim, with its digest, so a drift fails here first.
func TestSecureAdminMigrationEmbedsSharedTemplatesVerbatim(t *testing.T) {
	text := read(t, "009_secure_admin_boundary.sql")
	for name, schema := range map[string]string{"admincontext": admincontext.Schema, "adminowner": adminowner.Schema, "admincommand": admincommand.Schema} {
		if !strings.Contains(text, schema) || !strings.Contains(text, "-- shared-template-sha256: "+sha(schema)+"\n") {
			t.Fatalf("009 drifted from shared/go/%s/schema.sql", name)
		}
	}
	if sha(admincommand.Schema) != admincommand.SchemaSHA256() {
		t.Fatal("digest helper disagrees")
	}
	if !strings.Contains(text, adminevents.OutboxSchema) {
		t.Fatal("009 drifted from shared/go/adminevents.OutboxSchema")
	}
	// Nothing is seeded: no grant, policy or approval rows.
	lowered := strings.ToLower(text)
	for _, forbidden := range []string{"insert into support_admin_staff_grants", "insert into admin_owner_", "insert into admin_owner_policy"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("009 seeds %q", forbidden)
		}
	}
}

func TestSecureAdminMigrationReplayRollbackAndReapply(t *testing.T) {
	pool := testdb.Fresh(t)
	ctx := context.Background()
	// Second boot: every file again, nothing may fail or duplicate.
	testdb.Apply(ctx, t, pool)
	exists := func(table string) bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	for _, table := range []string{"admin_owner_command_decisions", "admin_owner_context_heads", "admin_event_outbox", "support_admin_staff_grants", "support_admin_reply_deliveries", "support_admin_case_emissions"} {
		if !exists(table) {
			t.Fatalf("missing %s", table)
		}
	}
	var grants int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM support_admin_staff_grants`).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("grants seeded: %d %v", grants, err)
	}
	// Existing messages keep a contiguous per-case order across replays.
	var caseID string
	if err := pool.QueryRow(ctx, `INSERT INTO complaints(complainant_type,complainant_id,service_type,subject,description) VALUES('customer','c-1','taxi','s','d') RETURNING id::text`).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO support_chat_messages(complaint_id,sender_type,sender_id,content) VALUES($1,'customer','c-1','m')`, caseID); err != nil {
			t.Fatal(err)
		}
	}
	testdb.Apply(ctx, t, pool)
	down := read(t, "down", "009_secure_admin_boundary.down.sql")
	// Undelivered events refuse rollback.
	if _, err := pool.Exec(ctx, down); err == nil || !strings.Contains(err.Error(), "Undelivered") {
		t.Fatalf("pending events dropped: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE admin_event_outbox SET status='PUBLISHED'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, down); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, table := range []string{"admin_owner_command_decisions", "admin_event_outbox", "support_admin_staff_grants"} {
		if exists(table) {
			t.Fatalf("rollback left %s", table)
		}
	}
	testdb.Apply(ctx, t, pool)
	var sequences string
	if err := pool.QueryRow(ctx, `SELECT string_agg(sequence::text, ',' ORDER BY sequence) FROM support_chat_messages WHERE complaint_id=$1`, caseID).Scan(&sequences); err != nil || sequences != "1,2,3" {
		t.Fatalf("reapply backfill sequences=%q err=%v", sequences, err)
	}
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT message_seq FROM complaints WHERE id=$1`, caseID).Scan(&seq); err != nil || seq != 3 {
		t.Fatalf("message_seq=%d err=%v", seq, err)
	}
	// Populated owner evidence refuses rollback.
	if _, err := pool.Exec(ctx, `INSERT INTO admin_owner_command_decisions(idempotency_key,operation_id,decision_id,owner,contract,subject_kind,subject_id,action,version_scope,version_key,recorded_version,operation_digest,intent_digest,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,receipt)
 VALUES('k',gen_random_uuid(),gen_random_uuid(),'support-dispute-service','c','cases',$2,'ASSIGN_CASE','SUBJECT','',2,$1,$1,'s','p','karryam','m','z','o','t','pol',1,$1,'{}')`, strings.Repeat("a", 64), caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE admin_event_outbox SET status='PUBLISHED'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, down); err == nil {
		t.Fatal("populated command ledger dropped")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM admin_owner_command_decisions`); err == nil {
		t.Fatal("command evidence deletable")
	}
}
