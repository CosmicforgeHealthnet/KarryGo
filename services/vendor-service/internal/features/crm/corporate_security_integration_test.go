package crm

import (
	"context"
	"testing"
	"time"
)

// Every corporate read and command resolves its tenant from the server-side
// session and its corporate account from the path, never the body. These tests
// probe the boundary from outside: another tenant, another branch, and a nested
// resource borrowed from a different account.

func TestCorporateSecurity_TenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actorA := actorFor(businessA, "vs_a")
	actorB := actorFor(businessB, "vs_b")

	accountB := env.activeCorporate(t, businessB, "vs_b", "B Industries")
	personB := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_b"))
	contactB := env.addCorporateContact(t, businessB, "vs_b", accountB.ID, personB.ID, ContactRoleOther, false)
	siteB := env.addCorporateLocation(t, businessB, "vs_b", accountB.ID, "HQ", "Head office")
	centerB := env.addCorporateCostCenter(t, businessB, "vs_b", accountB.ID, "OPS", "Operations")
	interactionB := env.addCorporateInteraction(t, businessB, "vs_b", accountB.ID, CreateCorporateInteractionInput{})
	taskB := env.addCorporateTask(t, businessB, "vs_b", accountB.ID, CreateCorporateTaskInput{Title: "B work"})

	t.Run("reads", func(t *testing.T) {
		if _, err := env.service.GetCorporateCustomer(ctx, actorA, globalScope(), accountB.ID); err == nil {
			t.Fatal("tenant A read tenant B's corporate customer")
		} else if !isCRMCode(err, CodeCorporateNotFound) {
			// Not-found rather than forbidden: a forbidden would confirm the id
			// exists somewhere.
			t.Fatalf("expected a not-found response, got %v", err)
		}
		for name, err := range map[string]error{
			"contacts": func() error {
				_, _, e := env.service.ListCorporateContacts(ctx, actorA, globalScope(), accountB.ID, "", "", false, 0, 0)
				return e
			}(),
			"locations": func() error {
				_, _, e := env.service.ListCorporateLocations(ctx, actorA, globalScope(), accountB.ID, "", 0, 0)
				return e
			}(),
			"cost centres": func() error {
				_, _, e := env.service.ListCorporateCostCenters(ctx, actorA, globalScope(), accountB.ID, "", 0, 0)
				return e
			}(),
			"memberships": func() error {
				_, _, e := env.service.ListCorporateMemberships(ctx, actorA, globalScope(), accountB.ID, "", "", 0, 0)
				return e
			}(),
			"interactions": func() error {
				_, _, e := env.service.ListCorporateInteractions(ctx, actorA, globalScope(), accountB.ID, InteractionListOptions{})
				return e
			}(),
			"tasks": func() error {
				_, _, e := env.service.ListCorporateTasks(ctx, actorA, globalScope(), accountB.ID, TaskListOptions{})
				return e
			}(),
			"interaction by id": func() error {
				_, e := env.service.GetCorporateInteraction(ctx, actorA, globalScope(), accountB.ID, interactionB.ID)
				return e
			}(),
			"task by id": func() error {
				_, e := env.service.GetCorporateTask(ctx, actorA, globalScope(), accountB.ID, taskB.ID)
				return e
			}(),
		} {
			if err == nil {
				t.Errorf("tenant A read tenant B's %s", name)
			}
		}
	})

	t.Run("commands", func(t *testing.T) {
		for name, err := range map[string]error{
			"update": func() error {
				_, e := env.service.UpdateCorporateCustomer(ctx, actorA, globalScope(), accountB.ID,
					UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Hijacked"},
						ExpectedVersion: accountB.Version})
				return e
			}(),
			"archive": func() error {
				_, e := env.service.TransitionCorporateCustomer(ctx, actorA, globalScope(), accountB.ID,
					CorporateArchived, accountB.Version)
				return e
			}(),
			"scope change": func() error {
				_, _, e := env.service.ReplaceBranchScopes(ctx, actorA, globalScope(), accountB.ID,
					CorporateScopeBranchSet, []string{branchA1}, accountB.Version)
				return e
			}(),
			"add contact": func() error {
				personA := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_a_probe"))
				_, e := env.service.CreateCorporateContact(ctx, actorA, globalScope(), accountB.ID,
					CorporateContactInput{CustomerProfileRef: personA.ID})
				return e
			}(),
			"add location": func() error {
				_, e := env.service.CreateCorporateLocation(ctx, actorA, globalScope(), accountB.ID,
					CorporateLocationInput{Code: "X", Name: "X"})
				return e
			}(),
			"add cost centre": func() error {
				_, e := env.service.CreateCorporateCostCenter(ctx, actorA, globalScope(), accountB.ID,
					CorporateCostCenterInput{Code: "X", Name: "X"})
				return e
			}(),
			"add interaction": func() error {
				_, e := env.service.CreateCorporateInteraction(ctx, actorA, globalScope(), accountB.ID,
					CreateCorporateInteractionInput{Type: InteractionNote, Direction: DirectionInternal,
						Subject: "S", OccurredAt: env.now})
				return e
			}(),
			"add task": func() error {
				_, e := env.service.CreateCorporateTask(ctx, actorA, globalScope(), accountB.ID,
					CreateCorporateTaskInput{Title: "X"})
				return e
			}(),
			"archive contact": func() error {
				_, e := env.service.ArchiveCorporateContact(ctx, actorA, globalScope(), accountB.ID, contactB.ID, 0)
				return e
			}(),
			"archive location": func() error {
				_, e := env.service.ArchiveCorporateLocation(ctx, actorA, globalScope(), accountB.ID, siteB.ID, 0)
				return e
			}(),
			"archive cost centre": func() error {
				_, e := env.service.ArchiveCorporateCostCenter(ctx, actorA, globalScope(), accountB.ID, centerB.ID, 0)
				return e
			}(),
		} {
			if err == nil {
				t.Errorf("tenant A performed %s on tenant B's account", name)
			}
		}
	})

	t.Run("tenant B is unaffected", func(t *testing.T) {
		if _, err := env.service.GetCorporateCustomer(ctx, actorB, globalScope(), accountB.ID); err != nil {
			t.Fatalf("tenant B lost access to its own account: %v", err)
		}
		if env.reloadCorporate(t, businessB, accountB.ID).OrganizationName != "B Industries" {
			t.Fatal("tenant B's account was modified")
		}
	})

	t.Run("cross-tenant list isolation", func(t *testing.T) {
		accounts, _, err := env.service.ListCorporateCustomers(ctx, actorA, globalScope(), CorporateListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, account := range accounts {
			if account.ID == accountB.ID {
				t.Fatal("tenant B's account appeared in tenant A's list")
			}
		}
		// Nor through search, which is where a count would otherwise leak.
		found, _, err := env.service.ListCorporateCustomers(ctx, actorA, globalScope(),
			CorporateListOptions{Search: "B Industries"})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("search disclosed %d of another tenant's accounts", len(found))
		}
	})
}

func TestCorporateSecurity_BranchIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	one := env.branchCorporate(t, businessA, "vs_a", "Branch one account", branchA1)
	two := env.branchCorporate(t, businessA, "vs_a", "Branch two account", branchA2)
	wide := env.activeCorporate(t, businessA, "vs_a", "Business wide account")

	twoSite := env.addCorporateLocation(t, businessA, "vs_a", two.ID, "HQ", "Head office")
	twoInteraction := env.addCorporateInteraction(t, businessA, "vs_a", two.ID, CreateCorporateInteractionInput{})
	branchOne := branchScope(branchA1)

	t.Run("a branch actor sees only their own branch's accounts", func(t *testing.T) {
		accounts, _, err := env.service.ListCorporateCustomers(ctx, actor, branchOne, CorporateListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(accounts) != 1 || accounts[0].ID != one.ID {
			t.Fatalf("expected only the branch-one account, got %d", len(accounts))
		}
	})

	t.Run("another branch's account is not found", func(t *testing.T) {
		if _, err := env.service.GetCorporateCustomer(ctx, actor, branchOne, two.ID); err == nil {
			t.Fatal("a branch actor read another branch's account")
		}
		for name, err := range map[string]error{
			"contacts": func() error {
				_, _, e := env.service.ListCorporateContacts(ctx, actor, branchOne, two.ID, "", "", false, 0, 0)
				return e
			}(),
			"locations": func() error {
				_, _, e := env.service.ListCorporateLocations(ctx, actor, branchOne, two.ID, "", 0, 0)
				return e
			}(),
			"cost centres": func() error {
				_, _, e := env.service.ListCorporateCostCenters(ctx, actor, branchOne, two.ID, "", 0, 0)
				return e
			}(),
			"interactions": func() error {
				_, _, e := env.service.ListCorporateInteractions(ctx, actor, branchOne, two.ID, InteractionListOptions{})
				return e
			}(),
			"tasks": func() error {
				_, _, e := env.service.ListCorporateTasks(ctx, actor, branchOne, two.ID, TaskListOptions{})
				return e
			}(),
			"location by id": func() error {
				_, e := env.service.GetCorporateLocation(ctx, actor, branchOne, two.ID, twoSite.ID)
				return e
			}(),
			"interaction by id": func() error {
				_, e := env.service.GetCorporateInteraction(ctx, actor, branchOne, two.ID, twoInteraction.ID)
				return e
			}(),
			"scope change": func() error {
				_, _, e := env.service.ReplaceBranchScopes(ctx, actor, branchOne, two.ID,
					CorporateScopeBranchSet, []string{branchA1}, two.Version)
				return e
			}(),
		} {
			if err == nil {
				t.Errorf("a branch actor reached another branch's %s", name)
			}
		}
	})

	t.Run("a business-wide account is invisible at branch scope", func(t *testing.T) {
		// Its very existence describes commercial reach a branch grant does not
		// carry, so it is absent rather than locked.
		if _, err := env.service.GetCorporateCustomer(ctx, actor, branchOne, wide.ID); err == nil {
			t.Fatal("a branch actor read a business-wide account")
		}
	})

	t.Run("filtering by another branch is refused rather than answered", func(t *testing.T) {
		if _, _, err := env.service.ListCorporateCustomers(ctx, actor, branchOne,
			CorporateListOptions{BranchRef: branchA2}); err == nil {
			t.Fatal("a branch actor filtered by another branch")
		}
	})

	t.Run("a branch actor works normally within their branch", func(t *testing.T) {
		if _, err := env.service.GetCorporateCustomer(ctx, actor, branchOne, one.ID); err != nil {
			t.Fatalf("a branch actor lost access to their own account: %v", err)
		}
		if _, err := env.service.CreateCorporateLocation(ctx, actor, branchOne, one.ID,
			CorporateLocationInput{Code: "HQ", Name: "Head office"}); err != nil {
			t.Fatalf("a branch actor could not manage their own account: %v", err)
		}
	})
}

// TestCorporateSecurity_NestedResourceIsolation covers the subtlest IDOR: a
// resource id from account A presented under account B's path.
func TestCorporateSecurity_NestedResourceIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	acme := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	beta := env.activeCorporate(t, businessA, "vs_a", "Beta Schools")

	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	acmeContact := env.addCorporateContact(t, businessA, "vs_a", acme.ID, person.ID, ContactRoleProcurement, false)
	acmeSite := env.addCorporateLocation(t, businessA, "vs_a", acme.ID, "HQ", "Head office")
	acmeCenter := env.addCorporateCostCenter(t, businessA, "vs_a", acme.ID, "OPS", "Operations")
	acmeMembership := env.addCorporateMembership(t, businessA, "vs_a", acme.ID,
		CorporateMembershipInput{ContactRef: acmeContact.ID})
	acmeInteraction := env.addCorporateInteraction(t, businessA, "vs_a", acme.ID, CreateCorporateInteractionInput{})
	acmeTask := env.addCorporateTask(t, businessA, "vs_a", acme.ID, CreateCorporateTaskInput{Title: "Acme work"})

	// Every nested read is scoped by (business, corporate, id), so borrowing an
	// id from another account under this one's path finds nothing.
	for name, err := range map[string]error{
		"contact": func() error {
			_, e := env.service.GetCorporateContact(ctx, actor, globalScope(), beta.ID, acmeContact.ID)
			return e
		}(),
		"location": func() error {
			_, e := env.service.GetCorporateLocation(ctx, actor, globalScope(), beta.ID, acmeSite.ID)
			return e
		}(),
		"cost centre": func() error {
			_, e := env.service.GetCorporateCostCenter(ctx, actor, globalScope(), beta.ID, acmeCenter.ID)
			return e
		}(),
		"interaction": func() error {
			_, e := env.service.GetCorporateInteraction(ctx, actor, globalScope(), beta.ID, acmeInteraction.ID)
			return e
		}(),
		"task": func() error {
			_, e := env.service.GetCorporateTask(ctx, actor, globalScope(), beta.ID, acmeTask.ID)
			return e
		}(),
		"contact update": func() error {
			_, e := env.service.UpdateCorporateContact(ctx, actor, globalScope(), beta.ID, acmeContact.ID,
				CorporateContactInput{RelationshipRole: ContactRoleOther, ExpectedVersion: acmeContact.Version})
			return e
		}(),
		"location update": func() error {
			_, e := env.service.UpdateCorporateLocation(ctx, actor, globalScope(), beta.ID, acmeSite.ID,
				CorporateLocationInput{Code: "HQ", Name: "Hijacked", ExpectedVersion: acmeSite.Version})
			return e
		}(),
		"cost centre update": func() error {
			_, e := env.service.UpdateCorporateCostCenter(ctx, actor, globalScope(), beta.ID, acmeCenter.ID,
				CorporateCostCenterInput{Code: "OPS", Name: "Hijacked", ExpectedVersion: acmeCenter.Version})
			return e
		}(),
		"membership update": func() error {
			_, e := env.service.UpdateCorporateMembership(ctx, actor, globalScope(), beta.ID, acmeMembership.ID,
				CorporateMembershipInput{MembershipRole: MembershipManager, ExpectedVersion: acmeMembership.Version})
			return e
		}(),
		"membership archive": func() error {
			_, e := env.service.ArchiveCorporateMembership(ctx, actor, globalScope(), beta.ID, acmeMembership.ID, 0)
			return e
		}(),
		"task ownership proof": env.service.assertCorporateOwnsTask(ctx, actor, globalScope(), beta.ID, acmeTask.ID),
		"interaction ownership proof": env.service.assertCorporateOwnsInteraction(ctx, actor, globalScope(),
			beta.ID, acmeInteraction.ID),
	} {
		if err == nil {
			t.Errorf("a %s from another account was reachable under this one", name)
		}
	}

	// The originals are untouched.
	if env.reloadCorporateContact(t, businessA, acme.ID, acmeContact.ID).RelationshipRole != ContactRoleProcurement {
		t.Fatal("a nested-IDOR attempt modified the original contact")
	}
}

// TestCorporateSecurity_SchemaHoldsNoPersonalOrFinancialData is the structural
// guarantee behind every rule above: Section 8G cannot leak what it has nowhere
// to store.
func TestCorporateSecurity_SchemaHoldsNoPersonalOrFinancialData(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	var personal int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name LIKE 'corporate_customer%'
		  AND column_name ~ '(email|phone|msisdn|date_of_birth|password|government|national_id|passport|device_token|push_token)'`).
		Scan(&personal); err != nil {
		t.Fatal(err)
	}
	if personal != 0 {
		t.Fatalf("the corporate schema holds %d personal-data columns", personal)
	}

	// Organizational addresses are expected on locations; a personal address
	// column on contacts or memberships would not be.
	var contactAddress int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public'
		  AND table_name IN ('corporate_customer_contacts','corporate_customer_memberships','corporate_customer_profiles')
		  AND column_name ~ 'address'`).Scan(&contactAddress); err != nil {
		t.Fatal(err)
	}
	if contactAddress != 0 {
		t.Fatalf("contacts, memberships or profiles hold %d address columns", contactAddress)
	}

	var financial int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name LIKE 'corporate_customer%'
		  AND column_name ~ '(budget|credit|currency|balance|invoice|payment|wallet|payout|settlement|minor|kobo|price|discount)'`).
		Scan(&financial); err != nil {
		t.Fatal(err)
	}
	if financial != 0 {
		t.Fatalf("the corporate schema holds %d financial columns", financial)
	}

	// Nor any order or fulfilment surface.
	var commerce int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name LIKE 'corporate%'
		  AND table_name ~ '(order|invoice|quote|contract|catalog|purchase|approval)'`).Scan(&commerce); err != nil {
		t.Fatal(err)
	}
	if commerce != 0 {
		t.Fatalf("Section 8G created %d commerce tables", commerce)
	}
}

// TestCorporateSecurity_AuditAndEventsCarryRelationshipsNotPeople keeps the
// individual out of the trails a corporate relationship writes.
func TestCorporateSecurity_AuditAndEventsCarryRelationshipsNotPeople(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	published := &recordingEvents{}
	env.service = NewService(Options{DB: env.pool, Scopes: fixedScopes{"vs_a": {Global: true}}, Audit: env.audit,
		Events: published, Flags: allFlagsOn(), Now: func() time.Time { return env.now }})

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)
	env.addCorporateInteraction(t, businessA, "vs_a", profile.ID,
		CreateCorporateInteractionInput{Note: "Highly confidential renewal terms."})
	env.addCorporateTask(t, businessA, "vs_a", profile.ID,
		CreateCorporateTaskInput{Title: "Call", Description: "Sensitive internal reasoning."})

	dump := published.dump(t)
	// Free CRM text never reaches the event bus — Section 8C's rule, applied
	// unchanged to corporate activity.
	for _, forbidden := range []string{"Highly confidential renewal terms", "Sensitive internal reasoning"} {
		if containsFold(dump, forbidden) {
			t.Fatalf("a corporate event carries free CRM text: %s", dump)
		}
	}
	emitted := map[string]bool{}
	for _, eventType := range published.types {
		emitted[eventType] = true
	}
	for _, want := range []string{"vendor.crm.corporate.created", "vendor.crm.corporate.status.changed",
		"vendor.crm.corporate.contact.created"} {
		if !emitted[want] {
			t.Fatalf("expected a %s event, got %v", want, published.types)
		}
	}

	// The audit trail records the relationship, and never the note body.
	var blob string
	if err := env.pool.QueryRow(ctx, `SELECT coalesce(string_agg(coalesce(after_data::text,''),''),'')
		FROM vendor_audit_log WHERE vendor_id=$1`, businessA).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Highly confidential renewal terms", "Sensitive internal reasoning"} {
		if containsFold(blob, forbidden) {
			t.Fatalf("the audit log carries free CRM text: %s", blob)
		}
	}
}
