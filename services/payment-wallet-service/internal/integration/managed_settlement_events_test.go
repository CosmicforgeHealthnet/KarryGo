package integration_test

// Section 12 WAL-019 / WAL-017A: the durable events a managed settlement
// produces, and the durable record of a refusal.
//
// Run against the real database because the property being certified is a
// transactional one: the event and the money commit together, or neither does.
// A mock cannot fail that.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

type outboxEvent struct {
	EventType        string
	AggregateID      string
	AggregateVersion int64
	OrganizationID   string
	Payload          map[string]any
}

func settlementEvents(t *testing.T, pool *pgxpool.Pool, instructionID string) []outboxEvent {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT event_type, aggregate_id, aggregate_version, COALESCE(organization_id,''), payload
		FROM finance_outbox
		WHERE aggregate_type = 'MANAGED_SETTLEMENT' AND aggregate_id = $1
		ORDER BY created_at, id`, instructionID)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	var events []outboxEvent
	for rows.Next() {
		var event outboxEvent
		var raw []byte
		if err := rows.Scan(&event.EventType, &event.AggregateID, &event.AggregateVersion,
			&event.OrganizationID, &raw); err != nil {
			t.Fatal(err)
		}
		// The outbox column holds the whole envelope; the settlement's own
		// fields live in its payload.
		var envelope struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("outbox row is not an envelope: %v", err)
		}
		if envelope.Payload == nil {
			t.Fatalf("envelope carries no payload: %s", raw)
		}
		event.Payload = envelope.Payload
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestPostingASettlementEmitsExactlyOnePostedEvent(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_event_posted"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)

	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err != nil {
		t.Fatal(err)
	}

	events := settlementEvents(t, pool, instruction.InstructionID)
	if len(events) != 1 {
		t.Fatalf("emitted %d events, want 1: %+v", len(events), events)
	}
	event := events[0]
	if event.EventType != walletrepositories.EventManagedSettlementPosted {
		t.Fatalf("event type = %q", event.EventType)
	}
	if event.AggregateVersion != instruction.InstructionVersion {
		t.Fatalf("aggregate version = %d, want the instruction version %d",
			event.AggregateVersion, instruction.InstructionVersion)
	}
	if event.OrganizationID != instruction.OrganizationID {
		t.Fatalf("organization = %q, want %q", event.OrganizationID, instruction.OrganizationID)
	}

	// Every field the contract names must actually be on the wire, or a
	// consumer cannot correlate the event with anything.
	for _, field := range []string{
		"instructionId", "instructionVersion", "sourceService", "sourceJobId",
		"organizationId", "providerId", "currency", "status", "occurredAt", "payloadVersion",
	} {
		if _, present := event.Payload[field]; !present {
			t.Errorf("payload is missing %q: %+v", field, event.Payload)
		}
	}
	if event.Payload["status"] != walletmodels.ManagedSettlementPosted {
		t.Errorf("payload status = %v, want POSTED", event.Payload["status"])
	}

	// The payload carries answers, never the inputs that produced them. A
	// consumer that could recompute an entitlement would eventually be asked
	// to, and then two authorities would be deciding one number.
	for _, forbidden := range []string{"rateBps", "rate_bps", "commissionBasis", "rules", "plan"} {
		if _, present := event.Payload[forbidden]; present {
			t.Errorf("payload leaks the compensation input %q", forbidden)
		}
	}
}

// The caller's retries are not events. A consumer counting postings must see
// one posting, because one journal was posted.
func TestResendingAPostedInstructionEmitsNoSecondEvent(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_event_retry"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)

	for attempt := 0; attempt < 3; attempt++ {
		if _, err := repo.PostManagedSettlement(context.Background(), instruction); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if events := settlementEvents(t, pool, instruction.InstructionID); len(events) != 1 {
		t.Fatalf("three sends emitted %d events, want 1", len(events))
	}
}

// A reversal is its own fact. The posted event is not retracted, because it
// remains true: the money moved, and then it moved back.
func TestReversalEmitsItsOwnEventWithoutRetractingThePosting(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_event_reversed"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)
	ctx := context.Background()

	if _, err := repo.PostManagedSettlement(ctx, instruction); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReverseManagedSettlement(ctx,
		instruction.InstructionID, instruction.InstructionVersion, "OVERPAID"); err != nil {
		t.Fatal(err)
	}

	events := settlementEvents(t, pool, instruction.InstructionID)
	if len(events) != 2 {
		t.Fatalf("emitted %d events, want posted then reversed: %+v", len(events), events)
	}
	if events[0].EventType != walletrepositories.EventManagedSettlementPosted {
		t.Fatalf("first event = %q, want the posting", events[0].EventType)
	}
	if events[1].EventType != walletrepositories.EventManagedSettlementReversed {
		t.Fatalf("second event = %q, want the reversal", events[1].EventType)
	}
	if events[1].Payload["reasonCode"] != "OVERPAID" {
		t.Errorf("reversal event lost its reason: %+v", events[1].Payload)
	}

	// Reversing twice moves the money once, and announces it once.
	if _, err := repo.ReverseManagedSettlement(ctx,
		instruction.InstructionID, instruction.InstructionVersion, "OVERPAID"); err != nil {
		t.Fatal(err)
	}
	if events := settlementEvents(t, pool, instruction.InstructionID); len(events) != 2 {
		t.Fatalf("a second reversal emitted a third event: %d", len(events))
	}
}

// A terminal refusal is a durable fact Fleet must learn even if the response is
// lost: the instruction is wrong and someone has to correct it.
func TestTerminalRefusalIsRecordedAndAnnouncedOnce(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_event_refused"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	ctx := context.Background()

	// Resolved against the wrong figure: 8,000 claimed where 9,000 is
	// distributable. Resending this unchanged can never post.
	refused := commissionInstruction(jobID)
	refused.GrossBasisMinor = 8000
	refused.ProviderShareMinor = 1000
	refused.OrganizationShareMinor = 7000

	if _, err := repo.PostManagedSettlement(ctx, refused); err == nil {
		t.Fatal("an instruction resolved against the wrong basis was accepted")
	}

	events := settlementEvents(t, pool, refused.InstructionID)
	if len(events) != 1 || events[0].EventType != walletrepositories.EventManagedSettlementFailed {
		t.Fatalf("emitted %+v, want one failed event", events)
	}
	if events[0].Payload["status"] != walletmodels.ManagedSettlementFailedTerminal {
		t.Errorf("failed event status = %v", events[0].Payload["status"])
	}
	if events[0].Payload["failureCode"] == nil || events[0].Payload["failureCode"] == "" {
		t.Errorf("failed event carries no failure code: %+v", events[0].Payload)
	}

	// WAL-017A. Resending the same wrong instruction records nothing further
	// and announces nothing further: one refusal per instruction, not one per
	// retry.
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := repo.PostManagedSettlement(ctx, refused); err == nil {
			t.Fatal("the refused instruction was later accepted")
		}
	}
	if events := settlementEvents(t, pool, refused.InstructionID); len(events) != 1 {
		t.Fatalf("retrying a refused instruction emitted %d events, want 1", len(events))
	}

	var recorded int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM managed_settlements WHERE instruction_id = $1`, refused.InstructionID,
	).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("recorded %d settlement rows for one refused instruction, want 1", recorded)
	}

	// And the refusal must not have moved any money.
	var journals int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_transactions WHERE source_reference = $1`, jobID,
	).Scan(&journals); err != nil {
		t.Fatal(err)
	}
	if journals != 0 {
		t.Fatalf("a refused instruction posted %d journals", journals)
	}
}

// The money is still there to be settled correctly afterwards. A refusal that
// poisoned the job would turn a correctable mistake into a stuck payment.
func TestAJobRefusedOnceCanStillBeSettledByACorrectedInstruction(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_event_corrected"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	ctx := context.Background()

	wrong := commissionInstruction(jobID)
	wrong.GrossBasisMinor = 8000
	wrong.ProviderShareMinor = 1000
	wrong.OrganizationShareMinor = 7000
	if _, err := repo.PostManagedSettlement(ctx, wrong); err == nil {
		t.Fatal("the wrong instruction was accepted")
	}

	// Fleet corrects it and reissues under a new instruction version.
	corrected := commissionInstruction(jobID)
	corrected.InstructionVersion = 2
	corrected.IdempotencyKey = "settlement:v2:" + jobID
	settlement, err := repo.PostManagedSettlement(ctx, corrected)
	if err != nil {
		t.Fatalf("the corrected instruction was refused: %v", err)
	}
	if settlement.Status != walletmodels.ManagedSettlementPosted {
		t.Fatalf("corrected settlement status = %q", settlement.Status)
	}
}
