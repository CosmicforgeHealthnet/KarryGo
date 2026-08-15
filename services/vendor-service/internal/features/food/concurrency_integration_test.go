package food

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The tests below are deterministic by construction: each one either starts
// both writers from the SAME loaded version (so exactly one expected-version
// fence can win) or runs genuinely parallel goroutines whose outcome is
// counted rather than timed. None sleeps.

// TestConcurrency_SimultaneousItemUpdatesLeaveOneWinner proves the
// expected-version fence, not merely that both writes "worked".
func TestConcurrency_SimultaneousItemUpdatesLeaveOneWinner(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Jollof Rice", 250000)

	var (
		wait      sync.WaitGroup
		mutex     sync.Mutex
		succeeded int
		conflicts int
	)
	for _, name := range []string{"first", "second"} {
		wait.Add(1)
		go func(name string) {
			defer wait.Done()
			_, err := env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{
				Name: stringPtr("Jollof Rice " + name), ExpectedVersion: item.Version})
			mutex.Lock()
			defer mutex.Unlock()
			switch {
			case err == nil:
				succeeded++
			case errorCode(t, err) == CodeStaleVersion:
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(name)
	}
	wait.Wait()
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("expected exactly one winner and one conflict, got %d/%d", succeeded, conflicts)
	}
	final, _, _, err := env.service.GetItem(ctx, actor, globalScope(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Version != item.Version+1 {
		t.Fatalf("expected exactly one version bump, got %d", final.Version)
	}
}

// TestConcurrency_DuplicateAttachIsRefusedExactlyOnce proves the relationship
// uniqueness is a database fact rather than a check-then-insert race.
func TestConcurrency_DuplicateAttachIsRefusedExactlyOnce(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)
	group := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)

	var (
		wait      sync.WaitGroup
		mutex     sync.Mutex
		succeeded int
		duplicate int
	)
	for index := 0; index < 4; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := env.service.AttachModifierGroup(ctx, actor, globalScope(), item.ID, group.ID)
			mutex.Lock()
			defer mutex.Unlock()
			switch {
			case err == nil:
				succeeded++
			case errorCode(t, err) == CodeDuplicateRelationship:
				duplicate++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wait.Wait()
	if succeeded != 1 || duplicate != 3 {
		t.Fatalf("expected one attachment and three duplicates, got %d/%d", succeeded, duplicate)
	}
	attached, err := env.service.ListAttachedModifierGroups(ctx, actor, globalScope(), item.ID)
	if err != nil || len(attached) != 1 {
		t.Fatalf("expected exactly one attachment row: %v %+v", err, attached)
	}
}

// TestConcurrency_ParallelCreatesProduceContiguousPositions proves the parent
// row lock serialises position allocation, so ordering never collides or gaps.
func TestConcurrency_ParallelCreatesProduceContiguousPositions(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")

	const writers = 8
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			if _, err := env.service.CreateSection(ctx, actor, globalScope(), menu.ID,
				CreateSectionInput{Name: "Section " + itoa(index)}); err != nil {
				t.Errorf("create section %d: %v", index, err)
			}
		}(index)
	}
	wait.Wait()

	sections, err := env.service.ListSections(ctx, actor, globalScope(), menu.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != writers {
		t.Fatalf("expected %d sections, got %d", writers, len(sections))
	}
	for index, section := range sections {
		if section.Position != index {
			t.Fatalf("expected contiguous positions, got %d at index %d", section.Position, index)
		}
	}
}

// TestConcurrency_ArchiveAndReorderRaceStaysConsistent runs the two operations
// that both renumber the same collection.
func TestConcurrency_ArchiveAndReorderRaceStaysConsistent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	one := env.seedSection(t, businessA, staffA, menu, "One")
	two := env.seedSection(t, businessA, staffA, menu, "Two")
	three := env.seedSection(t, businessA, staffA, menu, "Three")

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		// May legitimately fail if the archive lands first and removes an id
		// from the live set; what must never happen is a corrupt ordering.
		_, _ = env.service.ReorderSections(ctx, actor, globalScope(), menu.ID,
			[]string{three.ID, two.ID, one.ID})
	}()
	go func() {
		defer wait.Done()
		_, _ = env.service.ArchiveSection(ctx, actor, globalScope(), menu.ID, two.ID, two.Version)
	}()
	wait.Wait()

	live, err := env.service.ListSections(ctx, actor, globalScope(), menu.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	for index, section := range live {
		if section.Position != index {
			t.Fatalf("live sections must stay contiguous, got %d at %d (%+v)", section.Position, index, live)
		}
	}
	// Positions are unique across the whole menu, archived rows included.
	rows, err := env.pool.Query(ctx, `SELECT position FROM food_menu_sections WHERE menu_id=$1`, menu.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[int]bool{}
	for rows.Next() {
		var position int
		if err = rows.Scan(&position); err != nil {
			t.Fatal(err)
		}
		if seen[position] {
			t.Fatalf("duplicate position %d survived the race", position)
		}
		seen[position] = true
	}
	_ = three
}

// TestOutbox_StateAndEventCommitTogether proves the outbox row is written in
// the mutation's own transaction: a rolled-back mutation leaves no event.
func TestOutbox_StateAndEventCommitTogether(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")

	if types := env.outboxTypes(t, menu.ID); len(types) != 1 || types[0] != "food.menu.created" {
		t.Fatalf("expected exactly one creation event, got %v", types)
	}
	var schemaVersion int
	var aggregateType string
	if err := env.pool.QueryRow(ctx,
		`SELECT schema_version, aggregate_type FROM vendor_outbox WHERE aggregate_id=$1`, menu.ID).
		Scan(&schemaVersion, &aggregateType); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != 1 || aggregateType != "food_menu" {
		t.Fatalf("unexpected envelope: v%d %s", schemaVersion, aggregateType)
	}

	// A failed mutation writes neither state nor event. The failure is forced
	// with a stale version, which is a real client error rather than a fault
	// injected into the database.
	before := len(env.outboxTypes(t, menu.ID))
	if _, err := env.service.UpdateMenu(ctx, actor, globalScope(), menu.ID,
		UpdateMenuInput{Name: stringPtr("Never"), ExpectedVersion: 999}); err == nil {
		t.Fatal("expected the stale update to fail")
	}
	if after := len(env.outboxTypes(t, menu.ID)); after != before {
		t.Fatalf("a failed mutation must leave no orphan event: %d -> %d", before, after)
	}
	if len(env.auditActions(t, menu.ID)) != 1 {
		t.Fatal("a failed mutation must leave no orphan audit row")
	}

	// And the audit row is in the same transaction as the state change: a
	// direct rollback loses both.
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = env.service.record(ctx, tx, actor, evidence{
		action: "food.menu.updated", eventType: "food.menu.updated", aggregateType: "food_menu",
		aggregateID: menu.ID, branchID: branchA1, version: 2, payload: map[string]any{"probe": true},
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
		t.Fatal(err)
	}
	if got := len(env.outboxTypes(t, menu.ID)); got != before {
		t.Fatalf("a rolled-back transaction must leave no event, got %d", got)
	}
}

// TestAudit_CarriesActorTenantAndNoSecrets checks the evidence shape.
func TestAudit_CarriesActorTenantAndNoSecrets(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")

	var actorType, actorID, vendorID, branchID, correlationID string
	var after []byte
	if err := env.pool.QueryRow(ctx, `
		SELECT actor_type, COALESCE(actor_id,''), COALESCE(vendor_id,''), COALESCE(branch_id,''),
			COALESCE(correlation_id,''), COALESCE(after_data::text,'')::bytea
		FROM vendor_audit_log WHERE aggregate_id=$1`, menu.ID).
		Scan(&actorType, &actorID, &vendorID, &branchID, &correlationID, &after); err != nil {
		t.Fatal(err)
	}
	if actorType != "vendor_staff" || actorID != staffA || vendorID != businessA || branchID != branchA1 {
		t.Fatalf("unexpected audit actor/tenant: %s %s %s %s", actorType, actorID, vendorID, branchID)
	}
	if correlationID != "corr_food_test" {
		t.Fatalf("expected the correlation id to be preserved, got %q", correlationID)
	}
	// The recorded state is catalogue configuration only: no token, no secret,
	// no customer identity.
	for _, forbidden := range []string{"token", "secret", "otp", "password", "customer"} {
		if containsFold(string(after), forbidden) {
			t.Errorf("audit payload must not contain %q: %s", forbidden, string(after))
		}
	}
}

func containsFold(haystack, needle string) bool {
	lower := make([]byte, len(haystack))
	for index := 0; index < len(haystack); index++ {
		char := haystack[index]
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		lower[index] = char
	}
	return indexOf(string(lower), needle) >= 0
}

func indexOf(haystack, needle string) int {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}
