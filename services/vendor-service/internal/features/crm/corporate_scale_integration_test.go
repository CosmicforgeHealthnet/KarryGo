package crm

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Section 8G scale tests. The fixture is deliberately larger than any unit
// fixture so pagination, filtering and the timeline are proven over more rows
// than a single page, and so the query plans are measured against a table with
// enough rows for the planner's choice to mean something.

const (
	scaleCorporateAccounts = 100
	scaleContactsPer       = 3
	scaleSitesPer          = 5
	scaleActivityAccounts  = 5
	scaleActivityPer       = 60
)

type corporateScaleFixture struct {
	accounts   []CorporateProfile
	timelineID string
}

// seedCorporateScale builds ~100 accounts, 300 contacts, 300 memberships, 500
// locations, 500 cost centres, 300 interactions and 300 tasks. It writes
// through the real service methods so the rows are exactly what production
// would have produced.
func seedCorporateScale(t *testing.T, env *testEnv) corporateScaleFixture {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	fixture := corporateScaleFixture{}

	statuses := []string{CorporateActive, CorporateActive, CorporateOnHold, CorporateInactive}
	types := []string{OrgCompany, OrgSchool, OrgHospital, OrgHotel, OrgRetailer}

	for index := 0; index < scaleCorporateAccounts; index++ {
		name := fmt.Sprintf("Scale Organization %s", pad(index))
		profile := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{
			OrganizationName: name,
			OrganizationType: types[index%len(types)],
		})
		if target := statuses[index%len(statuses)]; target != CorporateProspect {
			moved, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
				CorporateActive, profile.Version)
			if err != nil {
				t.Fatalf("activate %s: %v", name, err)
			}
			profile = moved
			if target != CorporateActive {
				if moved, err = env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
					target, profile.Version); err != nil {
					t.Fatalf("move %s to %s: %v", name, target, err)
				}
				profile = moved
			}
		}
		fixture.accounts = append(fixture.accounts, profile)

		for contactIndex := 0; contactIndex < scaleContactsPer; contactIndex++ {
			person := env.seedProfile(t, businessA, "vs_a",
				customerSubject(fmt.Sprintf("cus_scale_%d_%d", index, contactIndex)))
			contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID,
				ContactRoleProcurement, contactIndex == 0)
			env.addCorporateMembership(t, businessA, "vs_a", profile.ID,
				CorporateMembershipInput{ContactRef: contact.ID})
		}
		for siteIndex := 0; siteIndex < scaleSitesPer; siteIndex++ {
			env.addCorporateLocation(t, businessA, "vs_a", profile.ID,
				fmt.Sprintf("S%d", siteIndex), fmt.Sprintf("Site %d", siteIndex))
			env.addCorporateCostCenter(t, businessA, "vs_a", profile.ID,
				fmt.Sprintf("C%d", siteIndex), fmt.Sprintf("Centre %d", siteIndex))
		}
	}

	for index := 0; index < scaleActivityAccounts; index++ {
		profile := fixture.accounts[index]
		for activityIndex := 0; activityIndex < scaleActivityPer; activityIndex++ {
			if _, err := env.service.CreateCorporateInteraction(ctx, actor, globalScope(), profile.ID,
				CreateCorporateInteractionInput{Type: InteractionNote, Direction: DirectionInternal,
					Subject: fmt.Sprintf("Account review %d", activityIndex)}); err != nil {
				t.Fatalf("seed interaction: %v", err)
			}
			if _, err := env.service.CreateCorporateTask(ctx, actor, globalScope(), profile.ID,
				CreateCorporateTaskInput{Title: fmt.Sprintf("Follow up %d", activityIndex),
					AssignedStaffRef: "vs_a"}); err != nil {
				t.Fatalf("seed task: %v", err)
			}
		}
	}
	fixture.timelineID = fixture.accounts[0].ID
	return fixture
}

// TestCorporateScale_ListPaginationIsCompleteAndStable walks the whole account
// list one page at a time and proves every account is seen exactly once, in a
// total order, with no gap at a page boundary.
func TestCorporateScale_ListPaginationIsCompleteAndStable(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	fixture := seedCorporateScale(t, env)

	const page = 17
	seen := map[string]int{}
	var order []string
	for offset := 0; ; offset += page {
		results, hasMore, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
			CorporateListOptions{Limit: page, Offset: offset, Sort: "organization_name", Direction: "asc"})
		if err != nil {
			t.Fatalf("page at offset %d: %v", offset, err)
		}
		for _, profile := range results {
			seen[profile.ID]++
			order = append(order, profile.NormalizedName)
		}
		if !hasMore {
			if len(results) == 0 && offset > 0 {
				t.Fatalf("the final page at offset %d was empty while hasMore was set on the previous page", offset)
			}
			break
		}
		if len(results) != page {
			t.Fatalf("page at offset %d returned %d rows while claiming more follow", offset, len(results))
		}
		if offset > scaleCorporateAccounts*2 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != len(fixture.accounts) {
		t.Fatalf("paging saw %d distinct accounts; seeded %d", len(seen), len(fixture.accounts))
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("account %s appeared %d times across pages", id, count)
		}
	}
	for index := 1; index < len(order); index++ {
		if order[index] < order[index-1] {
			t.Fatalf("ordering broke at position %d: %q before %q", index, order[index-1], order[index])
		}
	}
}

// TestCorporateScale_FiltersAndSearchNarrowCorrectly proves the filters compose
// and that search is a bounded prefix/substring match over the normalized name,
// not a free-text SQL fragment.
func TestCorporateScale_FiltersAndSearchNarrowCorrectly(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	fixture := seedCorporateScale(t, env)

	byStatus := map[string]int{}
	byType := map[string]int{}
	for _, profile := range fixture.accounts {
		byStatus[profile.Status]++
		byType[profile.OrganizationType]++
	}
	for status, expected := range byStatus {
		results := drainCorporateList(t, env, CorporateListOptions{Status: status})
		if len(results) != expected {
			t.Errorf("status %s returned %d accounts; want %d", status, len(results), expected)
		}
		for _, profile := range results {
			if profile.Status != status {
				t.Errorf("status filter %s leaked a %s account", status, profile.Status)
			}
		}
	}
	for orgType, expected := range byType {
		results := drainCorporateList(t, env, CorporateListOptions{OrganizationType: orgType})
		if len(results) != expected {
			t.Errorf("type %s returned %d accounts; want %d", orgType, len(results), expected)
		}
	}

	// Composed filters intersect rather than union.
	composed := drainCorporateList(t, env, CorporateListOptions{Status: CorporateOnHold, OrganizationType: OrgCompany})
	for _, profile := range composed {
		if profile.Status != CorporateOnHold || profile.OrganizationType != OrgCompany {
			t.Fatalf("a composed filter returned %s/%s", profile.Status, profile.OrganizationType)
		}
	}

	exact := drainCorporateList(t, env, CorporateListOptions{Search: "Scale Organization " + pad(7)})
	if len(exact) != 1 {
		t.Fatalf("an exact-name search returned %d accounts", len(exact))
	}
	broad := drainCorporateList(t, env, CorporateListOptions{Search: "scale organization"})
	if len(broad) != len(fixture.accounts) {
		t.Fatalf("a common-prefix search returned %d accounts; want %d", len(broad), len(fixture.accounts))
	}
	// Search input is a bind parameter and is escaped, so wildcard and quote
	// characters match literally and cannot change the query.
	for _, hostile := range []string{"%", "_", "'", "' OR 1=1 --", "\\"} {
		results := drainCorporateList(t, env, CorporateListOptions{Search: hostile})
		if len(results) != 0 {
			t.Fatalf("search %q matched %d accounts; it should match none literally", hostile, len(results))
		}
	}

	// Sort and direction are allowlisted, so an unrecognised value falls back to
	// the default ordering. What matters is that the caller's text never reaches
	// SQL: each of these returns the ordinary first page and leaves the schema
	// intact, rather than executing anything.
	for _, opts := range []CorporateListOptions{
		{Sort: "c.updated_at; DROP TABLE corporate_customer_profiles"},
		{Sort: "secret_column"},
		{Sort: "created_at", Direction: "asc; DELETE FROM corporate_customer_profiles"},
	} {
		results, _, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(), opts)
		if err != nil {
			t.Fatalf("sort %q direction %q errored instead of falling back: %v", opts.Sort, opts.Direction, err)
		}
		if len(results) == 0 {
			t.Fatalf("sort %q returned no rows", opts.Sort)
		}
	}
	var survivors int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_profiles WHERE business_id=$1`,
		businessA).Scan(&survivors); err != nil {
		t.Fatal(err)
	}
	if survivors != len(fixture.accounts) {
		t.Fatalf("the account table holds %d rows after hostile input; want %d", survivors, len(fixture.accounts))
	}
}

// TestCorporateScale_ChildListsAndTimelineStayPinnedToTheirAccount is the
// nested-resource correctness check at a size where a missing account predicate
// would show up as another organization's rows.
func TestCorporateScale_ChildListsAndTimelineStayPinnedToTheirAccount(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	fixture := seedCorporateScale(t, env)
	subject := fixture.accounts[0]

	contacts, _, err := env.service.ListCorporateContacts(ctx, actor, globalScope(), subject.ID, "", "", false, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != scaleContactsPer {
		t.Fatalf("the account has %d contacts; want %d", len(contacts), scaleContactsPer)
	}
	for _, contact := range contacts {
		if contact.CorporateCustomerID != subject.ID {
			t.Fatalf("contact %s belongs to %s", contact.ID, contact.CorporateCustomerID)
		}
	}
	locations, _, err := env.service.ListCorporateLocations(ctx, actor, globalScope(), subject.ID, "", 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != scaleSitesPer {
		t.Fatalf("the account has %d locations; want %d", len(locations), scaleSitesPer)
	}
	centers, _, err := env.service.ListCorporateCostCenters(ctx, actor, globalScope(), subject.ID, "", 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(centers) != scaleSitesPer {
		t.Fatalf("the account has %d cost centres; want %d", len(centers), scaleSitesPer)
	}

	// The timeline pages over one account's interactions only, and every page
	// is newest-first without duplicates.
	const page = 13
	seen := map[string]int{}
	var previous string
	for offset := 0; offset < scaleActivityPer*2; offset += page {
		results, hasMore, err := env.service.ListCorporateInteractions(ctx, actor, globalScope(), subject.ID,
			InteractionListOptions{Limit: page, Offset: offset})
		if err != nil {
			t.Fatalf("timeline page %d: %v", offset, err)
		}
		for _, interaction := range results {
			if !interaction.IsCorporate() || interaction.CorporateCustomerID == nil ||
				*interaction.CorporateCustomerID != subject.ID {
				t.Fatal("the timeline returned an interaction that is not this account's")
			}
			seen[interaction.ID]++
			key := interaction.OccurredAt.Format("20060102150405.000000") + interaction.ID
			if previous != "" && key > previous {
				t.Fatalf("the timeline is not newest-first at %s", interaction.ID)
			}
			previous = key
		}
		if !hasMore {
			break
		}
	}
	if len(seen) != scaleActivityPer {
		t.Fatalf("the timeline yielded %d distinct interactions; want %d", len(seen), scaleActivityPer)
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("interaction %s appeared %d times", id, count)
		}
	}

	tasks, _, err := env.service.ListCorporateTasks(ctx, actor, globalScope(), subject.ID, TaskListOptions{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != scaleActivityPer {
		t.Fatalf("the account has %d tasks; want %d", len(tasks), scaleActivityPer)
	}

	summary, err := env.service.CorporateSummaryFor(ctx, actor, subject)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ContactCount != scaleContactsPer || summary.LocationCount != scaleSitesPer ||
		summary.CostCenterCount != scaleSitesPer {
		t.Fatalf("summary counts disagree with the fixture: %+v", summary)
	}
	if summary.OpenTaskCount != scaleActivityPer {
		t.Fatalf("summary open tasks = %d; want %d", summary.OpenTaskCount, scaleActivityPer)
	}
}

// TestCorporateScale_HotPathsAreIndexed asserts a property rather than pinning a
// plan: with sequential scans disabled the planner must still be able to answer
// each hot path, which is only true when an index covers it.
func TestCorporateScale_HotPathsAreIndexed(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	fixture := seedCorporateScale(t, env)
	subject := fixture.accounts[0]

	if _, err := env.pool.Exec(ctx, `ANALYZE corporate_customer_profiles, corporate_customer_contacts,
		corporate_customer_memberships, corporate_customer_locations, corporate_customer_cost_centers,
		crm_interactions, crm_tasks`); err != nil {
		t.Fatal(err)
	}

	paths := map[string]struct {
		query string
		args  []any
	}{
		"account list by status": {`SELECT id FROM corporate_customer_profiles
			WHERE business_id=$1 AND status=$2 ORDER BY updated_at DESC LIMIT 20`,
			[]any{businessA, CorporateActive}},
		"account by code": {`SELECT id FROM corporate_customer_profiles
			WHERE business_id=$1 AND corporate_customer_code=$2`, []any{businessA, subject.Code}},
		"contacts of an account": {`SELECT id FROM corporate_customer_contacts
			WHERE business_id=$1 AND corporate_customer_id=$2 AND status='ACTIVE'`, []any{businessA, subject.ID}},
		"accounts of a person": {`SELECT id FROM corporate_customer_contacts
			WHERE business_id=$1 AND customer_profile_id=$2`, []any{businessA, "cus_missing"}},
		"memberships of an account": {`SELECT id FROM corporate_customer_memberships
			WHERE business_id=$1 AND corporate_customer_id=$2`, []any{businessA, subject.ID}},
		"locations of an account": {`SELECT id FROM corporate_customer_locations
			WHERE business_id=$1 AND corporate_customer_id=$2`, []any{businessA, subject.ID}},
		"cost centres of an account": {`SELECT id FROM corporate_customer_cost_centers
			WHERE business_id=$1 AND corporate_customer_id=$2`, []any{businessA, subject.ID}},
		"corporate timeline": {`SELECT id FROM crm_interactions
			WHERE business_id=$1 AND corporate_customer_id=$2 ORDER BY occurred_at DESC LIMIT 20`,
			[]any{businessA, subject.ID}},
		"corporate tasks": {`SELECT id FROM crm_tasks
			WHERE business_id=$1 AND corporate_customer_id=$2 AND status='OPEN'`, []any{businessA, subject.ID}},
	}

	for name, path := range paths {
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		rows, err := tx.Query(ctx, `EXPLAIN `+path.query, path.args...)
		if err != nil {
			tx.Rollback(ctx)
			t.Fatalf("%s: %v", name, err)
		}
		var plan strings.Builder
		for rows.Next() {
			var line string
			if err = rows.Scan(&line); err != nil {
				rows.Close()
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			plan.WriteString(line)
			plan.WriteString("\n")
		}
		rows.Close()
		tx.Rollback(ctx)
		if strings.Contains(plan.String(), "Seq Scan") {
			t.Errorf("%s falls back to a sequential scan:\n%s", name, plan.String())
		}
	}
}

func drainCorporateList(t *testing.T, env *testEnv, opts CorporateListOptions) []CorporateProfile {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	var all []CorporateProfile
	opts.Limit = 50
	for offset := 0; ; offset += opts.Limit {
		opts.Offset = offset
		results, hasMore, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(), opts)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		all = append(all, results...)
		if !hasMore {
			return all
		}
		if offset > scaleCorporateAccounts*4 {
			t.Fatal("list did not terminate")
		}
	}
}
