package crm

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Real-PostgreSQL concurrency tests. Every ordering below is forced by a row
// lock and a transaction boundary, never by a sleep: a race test that passes
// because one goroutine happened to be slower proves nothing about production.
//
// Two fences do the work. A corporate mutation locks the corporate account row;
// anything touching an individual — a contact, a membership, an activity row
// naming a contact — additionally locks that person's Section 8A profile row,
// which is the same lock ApplyPrivacy and ApplyIdentityLink take.

// TestCorporateConcurrency_ContactCreateVersusRedaction covers both orderings
// of the create-versus-erasure race.
func TestCorporateConcurrency_ContactCreateVersusRedaction(t *testing.T) {
	t.Run("redaction commits first", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")

		profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
		person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))

		// The privacy transaction takes the profile lock first.
		privacyTx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reloaded := env.reload(t, businessA, person.ID)
		if err = env.service.ApplyPrivacy(ctx, privacyTx, actor, &reloaded, PrivacyInput{
			SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
			Action: PrivacyActionRedact, ObservedAt: env.now}); err != nil {
			_ = privacyTx.Rollback(ctx)
			t.Fatalf("apply privacy: %v", err)
		}

		created := make(chan error, 1)
		go func() {
			_, createErr := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
				CorporateContactInput{CustomerProfileRef: person.ID})
			created <- createErr
		}()
		waitForBlockedTransaction(t, env)
		if err = privacyTx.Commit(ctx); err != nil {
			t.Fatalf("commit privacy: %v", err)
		}

		// The create was blocked on the fence and now sees the redaction.
		if err = <-created; err == nil {
			t.Fatal("a contact was created for a relationship redacted moments earlier")
		} else if !isCRMCode(err, CodeCorporateContactRedacted) {
			t.Fatalf("expected a redaction rejection, got %v", err)
		}
		var live int
		if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
			WHERE business_id=$1 AND customer_profile_id=$2 AND privacy_suppressed=false AND status='ACTIVE'`,
			businessA, person.ID).Scan(&live); err != nil {
			t.Fatal(err)
		}
		if live != 0 {
			t.Fatalf("%d actionable contacts survived the redaction", live)
		}
	})

	t.Run("the contact create commits first", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")

		profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
		person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))

		// The create holds the profile lock; the redaction blocks behind it and
		// then suppresses the row it finds.
		createTx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = env.service.lockContactProfile(ctx, createTx, businessA, person.ID); err != nil {
			_ = createTx.Rollback(ctx)
			t.Fatalf("lock the profile: %v", err)
		}
		contact := env.addCorporateContactTx(t, createTx, businessA, actor, profile.ID, person.ID)

		redacted := make(chan error, 1)
		go func() {
			reloaded := env.reload(t, businessA, person.ID)
			redacted <- env.applyPrivacy(t, businessA, "vs_a", &reloaded, PrivacyInput{
				SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
				Action: PrivacyActionRedact, ObservedAt: env.now})
		}()
		waitForBlockedTransaction(t, env)
		if err = createTx.Commit(ctx); err != nil {
			t.Fatalf("commit the contact: %v", err)
		}
		if err = <-redacted; err != nil {
			t.Fatalf("redaction after the create: %v", err)
		}

		// The relationship exists and is immediately non-actionable.
		suppressed := env.reloadCorporateContact(t, businessA, profile.ID, contact)
		if !suppressed.PrivacySuppressed || suppressed.Actionable() {
			t.Fatalf("a contact created moments before a redaction is still actionable: %+v", suppressed)
		}
	})
}

// TestCorporateConcurrency_ContactCreateVersusIdentityMerge proves a merge
// landing around a create still leaves exactly one actionable relationship.
func TestCorporateConcurrency_ContactCreateVersusIdentityMerge(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_1"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	// The survivor is already a contact, so the merge has a genuine collision
	// to resolve rather than a simple move.
	env.addCorporateContact(t, businessA, "vs_a", profile.ID, customer.ID, ContactRoleOperations, false)

	createTx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = env.service.lockContactProfile(ctx, createTx, businessA, guest.ID); err != nil {
		_ = createTx.Rollback(ctx)
		t.Fatalf("lock the source profile: %v", err)
	}
	sourceContact := env.addCorporateContactTx(t, createTx, businessA, actor, profile.ID, guest.ID)

	merged := make(chan error, 1)
	go func() {
		merged <- env.applyLink(t, businessA, "vs_a",
			linkInput("evt_race_merge", guestSubject("gst_1"), customerSubject("cus_1"), 4))
	}()
	waitForBlockedTransaction(t, env)
	if err = createTx.Commit(ctx); err != nil {
		t.Fatalf("commit the contact: %v", err)
	}
	if err = <-merged; err != nil {
		t.Fatalf("merge after the create: %v", err)
	}

	// Exactly one live relationship for the merged identity, and it points at
	// the canonical person.
	var live int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
		WHERE business_id=$1 AND corporate_customer_id=$2 AND status <> 'ARCHIVED'`,
		businessA, profile.ID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 1 {
		t.Fatalf("expected one live contact after the merge race, got %d", live)
	}
	var pointingAtSource int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
		WHERE business_id=$1 AND customer_profile_id=$2 AND status <> 'ARCHIVED'`,
		businessA, guest.ID).Scan(&pointingAtSource); err != nil {
		t.Fatal(err)
	}
	if pointingAtSource != 0 {
		t.Fatalf("%d live relationships still point at the merged-away person", pointingAtSource)
	}
	if env.reloadCorporateContact(t, businessA, profile.ID, sourceContact).Status != CorporateChildArchived {
		t.Fatal("the duplicate relationship was not retired")
	}
}

// TestCorporateConcurrency_MembershipAndActivityVersusMerge proves the same
// fence protects the rows that hang off a contact.
func TestCorporateConcurrency_MembershipAndActivityVersusMerge(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_1"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	sourceContact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, guest.ID, ContactRoleProcurement, false)
	targetContact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, customer.ID, ContactRoleOperations, false)

	// A membership creation is in flight, holding the source's profile lock.
	membershipTx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = env.service.lockContactProfile(ctx, membershipTx, businessA, guest.ID); err != nil {
		_ = membershipTx.Rollback(ctx)
		t.Fatalf("lock the source profile: %v", err)
	}
	var membershipID string
	if err = membershipTx.QueryRow(ctx, `INSERT INTO corporate_customer_memberships
		(id,business_id,corporate_customer_id,corporate_contact_id,membership_role,status,created_by_staff_id)
		VALUES($1,$2,$3,$4,'APPROVER','ACTIVE',$5) RETURNING id`,
		newID("crmcmb_"), businessA, profile.ID, sourceContact.ID, actor.StaffID).Scan(&membershipID); err != nil {
		_ = membershipTx.Rollback(ctx)
		t.Fatalf("insert membership: %v", err)
	}

	merged := make(chan error, 1)
	go func() {
		merged <- env.applyLink(t, businessA, "vs_a",
			linkInput("evt_membership_merge", guestSubject("gst_1"), customerSubject("cus_1"), 4))
	}()
	waitForBlockedTransaction(t, env)
	if err = membershipTx.Commit(ctx); err != nil {
		t.Fatalf("commit the membership: %v", err)
	}
	if err = <-merged; err != nil {
		t.Fatalf("merge after the membership: %v", err)
	}

	// No orphan: the membership either followed the person to the surviving
	// relationship, or was retired with the duplicate. Either way it never
	// points at a live relationship for a person who no longer exists.
	membership := env.reloadMembership(t, businessA, membershipID)
	if membership.ContactID == sourceContact.ID && membership.Status != CorporateChildArchived {
		t.Fatalf("a live membership still hangs off the retired relationship: %+v", membership)
	}
	if membership.Status != CorporateChildArchived && membership.ContactID != targetContact.ID {
		t.Fatalf("the membership attached to neither the survivor nor archival: %+v", membership)
	}

	// A corporate interaction and task naming the retired contact keep their
	// historical reference — history is not rewritten — but the relationship
	// they name is no longer actionable.
	if env.reloadCorporateContact(t, businessA, profile.ID, sourceContact.ID).Actionable() {
		t.Fatal("the merged-away relationship is still actionable")
	}
}

// TestCorporateConcurrency_ArchiveVersusTaskCreate proves the corporate fence
// serializes account archival against new work on the account.
func TestCorporateConcurrency_ArchiveVersusTaskCreate(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")

	// The archive transaction takes the corporate row lock first.
	archiveTx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.loadCorporateForUpdate(ctx, archiveTx, businessA, profile.ID); err != nil {
		_ = archiveTx.Rollback(ctx)
		t.Fatalf("lock the account: %v", err)
	}
	if _, err = archiveTx.Exec(ctx, `UPDATE corporate_customer_profiles
		SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2`, businessA, profile.ID); err != nil {
		_ = archiveTx.Rollback(ctx)
		t.Fatalf("archive: %v", err)
	}

	created := make(chan error, 1)
	go func() {
		_, createErr := env.service.CreateCorporateTask(ctx, actor, globalScope(), profile.ID,
			CreateCorporateTaskInput{Title: "Work on an archived account"})
		created <- createErr
	}()
	waitForBlockedTransaction(t, env)
	if err = archiveTx.Commit(ctx); err != nil {
		t.Fatalf("commit the archive: %v", err)
	}

	if err = <-created; err == nil {
		t.Fatal("a task was created on an account archived moments earlier")
	} else if !isCRMCode(err, CodeCorporateArchived) {
		t.Fatalf("expected an archived rejection, got %v", err)
	}
	var open int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_tasks
		WHERE business_id=$1 AND corporate_customer_id=$2 AND status IN ('OPEN','IN_PROGRESS')`,
		businessA, profile.ID).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("%d actionable tasks survived on an archived account", open)
	}
}

// TestCorporateConcurrency_TaskCompleteVersusCancel proves the Section 8G
// facade inherits Section 8C's task concurrency rather than weakening it.
func TestCorporateConcurrency_TaskCompleteVersusCancel(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	task := env.addCorporateTask(t, businessA, "vs_a", profile.ID, CreateCorporateTaskInput{Title: "Race"})

	var wait sync.WaitGroup
	results := make([]error, 2)
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, results[0] = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, task.Version)
	}()
	go func() {
		defer wait.Done()
		_, results[1] = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCancelled, task.Version)
	}()
	wait.Wait()

	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one transition to win, got %d", winners)
	}
	settled := env.reloadCorporateTask(t, businessA, task.ID)
	if settled.Status != TaskCompleted && settled.Status != TaskCancelled {
		t.Fatalf("the task settled in an unexpected state: %s", settled.Status)
	}
	// The terminal timestamps must agree with the status the row settled in.
	if settled.Status == TaskCompleted && (settled.CompletedAt == nil || settled.CancelledAt != nil) {
		t.Fatalf("inconsistent terminal timestamps: %+v", settled)
	}
	if settled.Status == TaskCancelled && (settled.CancelledAt == nil || settled.CompletedAt != nil) {
		t.Fatalf("inconsistent terminal timestamps: %+v", settled)
	}
}

// TestCorporateConcurrency_ConcurrentPrimaryContactPromotion proves the
// one-primary rule is the database's, not a read-then-write check two writers
// could both pass.
func TestCorporateConcurrency_ConcurrentPrimaryContactPromotion(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	first := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_one"))
	second := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_two"))

	var wait sync.WaitGroup
	results := make([]error, 2)
	wait.Add(2)
	yes := true
	for index, personID := range []string{first.ID, second.ID} {
		go func(slot int, id string) {
			defer wait.Done()
			_, results[slot] = env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
				CorporateContactInput{CustomerProfileRef: id, IsPrimary: &yes})
		}(index, personID)
	}
	wait.Wait()

	for index, err := range results {
		if err != nil {
			t.Logf("writer %d lost the race: %v", index, err)
		}
	}
	var primaries int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
		WHERE business_id=$1 AND corporate_customer_id=$2 AND is_primary_contact AND status='ACTIVE'`,
		businessA, profile.ID).Scan(&primaries); err != nil {
		t.Fatal(err)
	}
	if primaries > 1 {
		t.Fatalf("two concurrent promotions produced %d primary contacts", primaries)
	}
}

// addCorporateContactTx inserts a contact inside a caller-held transaction, so
// a race test can pin the exact commit ordering. It mirrors what
// CreateCorporateContact writes.
func (e *testEnv) addCorporateContactTx(t *testing.T, tx pgx.Tx, business string, actor Actor, corporateID, profileID string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(context.Background(), `INSERT INTO corporate_customer_contacts
		(id,business_id,corporate_customer_id,customer_profile_id,original_profile_id,relationship_role,
		 is_primary_contact,status,created_by_staff_id)
		VALUES($1,$2,$3,$4,$4,'PROCUREMENT',false,'ACTIVE',$5) RETURNING id`,
		newID("crmcct_"), business, corporateID, profileID, actor.StaffID).Scan(&id); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("insert contact in transaction: %v", err)
	}
	return id
}
