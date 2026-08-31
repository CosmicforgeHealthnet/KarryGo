package crm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Section 8I cross-module certification.
//
// Every earlier section proved its own privacy behaviour. What none of them
// could prove is the property that actually matters to a customer who asks to
// be forgotten: that ALL of CRM agrees. A relationship redacted in Section 8A
// is only really gone if the tag engine, the audience builder, the task queue,
// the loyalty ledger, the campaign dispatcher, the scoring materializer and the
// export generator each independently decline to act on it — and no section
// test can establish that about the others.

// populatedRelationship builds one customer with state in every implemented
// CRM module, so a single privacy action can be observed against all of them.
type populatedRelationship struct {
	profile   Profile
	tag       Tag
	segment   Segment
	corporate CorporateProfile
	contact   CorporateContact
}

func (e *testEnv) populateEveryModule(t *testing.T, ref string) populatedRelationship {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	out := populatedRelationship{}

	// 8A: a relationship with real observed commercial history, so the
	// intelligence models have something to score.
	out.profile = e.scoredCustomer(t, businessA, "vs_a", ref, 8, 250000,
		300*24*time.Hour, 20*24*time.Hour)

	// 8B: a tag and a dynamic audience that selects them.
	tag, err := e.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "VIP " + ref})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	out.tag = tag
	if _, err = e.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID,
		[]string{out.profile.ID}); err != nil {
		t.Fatalf("assign tag: %v", err)
	}
	out.segment = e.intelligenceSegment(t, businessA, "vs_a", "Buyers "+ref,
		`{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":1}]}}`)

	// 8C: an interaction and an open task.
	if _, err = e.service.CreateInteraction(ctx, actor, globalScope(), CreateInteractionInput{
		CustomerProfileRef: out.profile.ID, Type: InteractionNote,
		Direction: DirectionInternal, Subject: "Account review"}); err != nil {
		t.Fatalf("create interaction: %v", err)
	}
	if _, err = e.service.CreateTask(ctx, actor, globalScope(), CreateTaskInput{
		CustomerProfileRef: out.profile.ID, Title: "Follow up", AssignedStaffRef: "vs_a"}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// 8G: a corporate contact referencing the same relationship.
	out.corporate = e.activeCorporate(t, businessA, "vs_a", "Acme "+ref)
	out.contact = e.addCorporateContact(t, businessA, "vs_a", out.corporate.ID,
		out.profile.ID, ContactRoleProcurement, true)

	return out
}

// TestSection8I_RedactionIsHonouredByEveryModule is the global privacy
// certification. It asks one question of the whole CRM: after a redaction, can
// any module still act on this person?
func TestSection8I_RedactionIsHonouredByEveryModule(t *testing.T) {
	env, store, stepUp := newExportEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// A cohort large enough for the intelligence models, with one subject.
	subject := env.populateEveryModule(t, "cus_8i_subject")
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_8i_peer%d", index),
			4+index, 120000, 300*24*time.Hour, time.Duration(index+1)*20*24*time.Hour)
	}
	env.materialize(t)
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	// Establish that every module really is holding state for this person, so
	// the assertions after redaction are meaningful rather than vacuous.
	before := env.segmentMembers(t, businessA, subject.segment.ID)
	if !containsString(before, subject.profile.ID) {
		t.Fatal("the subject is not in the audience; the certification would prove nothing")
	}
	if score, ok := env.scoreOf(t, businessA, subject.profile.ID, ModelRFM,
		IntelligenceScopeBusiness, ""); !ok || score.State != ScoreCurrent {
		t.Fatalf("the subject has no current score before redaction: %+v", score)
	}

	// The redaction, through the accepted Section 8A authority path.
	victim := subject.profile
	if err := env.applyPrivacy(t, businessA, "vs_a", &victim, PrivacyInput{
		SourceService: "customer-service", SourceVersion: victim.Version + 1,
		PrivacySubjectRef: victim.ID, Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	t.Run("segmentation drops the relationship", func(t *testing.T) {
		env.materialize(t)
		for _, member := range env.segmentMembers(t, businessA, subject.segment.ID) {
			if member == victim.ID {
				t.Fatal("a redacted relationship is still an audience member")
			}
		}
	})

	t.Run("intelligence is non-actionable", func(t *testing.T) {
		for _, model := range AllModelKeys {
			if _, ok := env.scoreOf(t, businessA, victim.ID, model, IntelligenceScopeBusiness, ""); ok {
				t.Errorf("a redacted relationship still has a current %s score", model)
			}
		}
		reloaded := env.reload(t, businessA, victim.ID)
		intelligence, err := env.service.CustomerIntelligenceFor(ctx, actor, globalScope(), reloaded, "")
		if err != nil {
			t.Fatal(err)
		}
		if intelligence.RFM != nil || intelligence.RFMState != ScoreNotAvailable {
			t.Errorf("Customer 360 exposed intelligence for a redacted relationship: %+v", intelligence)
		}
	})

	t.Run("a rebuild cannot resurrect the relationship", func(t *testing.T) {
		// The anti-resurrection property: recomputing from scratch must not
		// bring the person back, because the cohort query itself excludes them.
		env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
		if _, ok := env.scoreOf(t, businessA, victim.ID, ModelRFM, IntelligenceScopeBusiness, ""); ok {
			t.Fatal("an intelligence rebuild resurrected a redacted relationship")
		}
	})

	t.Run("corporate contact is non-actionable", func(t *testing.T) {
		contacts, _, err := env.service.ListCorporateContacts(ctx, actor, globalScope(),
			subject.corporate.ID, "", "", true, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, contact := range contacts {
			if contact.CustomerProfileID != victim.ID {
				continue
			}
			found = true
			if !contact.PrivacySuppressed || contact.Actionable() {
				t.Errorf("a redacted contact is still actionable: %+v", contact)
			}
			if contact.SafeDisplayLabel != nil {
				t.Errorf("a redacted contact still carries a label: %v", *contact.SafeDisplayLabel)
			}
		}
		if !found {
			t.Fatal("the corporate contact vanished entirely; it should remain, suppressed")
		}
	})

	t.Run("exports exclude the relationship", func(t *testing.T) {
		for _, dataset := range []string{ExportCustomerSummary, ExportIntelligenceScores} {
			filter := ExportFilter{}
			if dataset == ExportIntelligenceScores {
				filter.ModelKey = ModelRFM
			}
			export := env.createExport(t, stepUp, CreateExportInput{
				ExportType: dataset, Format: FormatCSV,
				ScopeType: IntelligenceScopeBusiness, Filter: filter})
			env.generateExports(t)
			ready := env.reloadExport(t, export.ID)
			if ready.Status != ExportReady {
				t.Fatalf("%s export finished as %s (%v)", dataset, ready.Status, ready.ErrorCode)
			}
			content, _ := store.contentOf(*ready.ArtifactRef)
			if strings.Contains(content, victim.ID) {
				t.Errorf("%s export contains a redacted relationship", dataset)
			}
		}
	})

	t.Run("a new tag cannot resurrect the relationship", func(t *testing.T) {
		// Assigning a tag to a redacted relationship must not make it
		// targetable again. Whether the write is refused or merely inert, the
		// audience is what matters.
		_, _ = env.service.BulkAssignTag(ctx, actor, globalScope(), subject.tag.ID,
			[]string{victim.ID})
		env.materialize(t)
		for _, member := range env.segmentMembers(t, businessA, subject.segment.ID) {
			if member == victim.ID {
				t.Fatal("tagging resurrected a redacted relationship into an audience")
			}
		}
	})

	t.Run("a late order fact cannot resurrect the relationship", func(t *testing.T) {
		// The authority path itself: a projection arriving after the redaction
		// must not rebuild an actionable relationship.
		func() {
			defer func() { _ = recover() }()
			env.seedOrder(t, businessA, "vs_a", victim, "ord_after_redaction", branchA1,
				900000, 900000, env.now, 99)
		}()
		env.materialize(t)
		for _, member := range env.segmentMembers(t, businessA, subject.segment.ID) {
			if member == victim.ID {
				t.Fatal("a late order fact resurrected a redacted relationship")
			}
		}
		env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
		if _, ok := env.scoreOf(t, businessA, victim.ID, ModelRFM, IntelligenceScopeBusiness, ""); ok {
			t.Fatal("a late order fact produced a score for a redacted relationship")
		}
	})
}

// TestSection8I_MarketingSuppressionStopsContactWithoutDestroyingRecords is the
// §11 cross-module rule: suppression is about reachability, not erasure.
func TestSection8I_MarketingSuppressionStopsContactWithoutDestroyingRecords(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	subject := env.populateEveryModule(t, "cus_8i_suppress")
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_8i_sup%d", index),
			4+index, 120000, 300*24*time.Hour, time.Duration(index+1)*20*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	victim := subject.profile
	if err := env.applyPrivacy(t, businessA, "vs_a", &victim, PrivacyInput{
		SourceService: "customer-service", SourceVersion: victim.Version + 1,
		PrivacySubjectRef: victim.ID, Action: PrivacyActionMarketingSuppress, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("marketing suppress: %v", err)
	}

	// Records survive. Suppression that quietly deleted the loyalty ledger, the
	// interaction history or the corporate relationship would destroy lawful
	// business records to achieve something the campaign rules already do.
	var interactions, corporateContacts int
	if err := env.pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM crm_interactions WHERE business_id=$1 AND customer_profile_id=$2),
			(SELECT count(*) FROM corporate_customer_contacts WHERE business_id=$1 AND customer_profile_id=$2)`,
		businessA, victim.ID).Scan(&interactions, &corporateContacts); err != nil {
		t.Fatal(err)
	}
	if interactions == 0 {
		t.Error("marketing suppression destroyed interaction history")
	}
	if corporateContacts == 0 {
		t.Error("marketing suppression destroyed the corporate relationship")
	}
	// Intelligence survives too: an operator still needs to understand their
	// book, and a score is not a message.
	reloaded := env.reload(t, businessA, victim.ID)
	intelligence, err := env.service.CustomerIntelligenceFor(ctx, actor, globalScope(), reloaded, "")
	if err != nil {
		t.Fatal(err)
	}
	if intelligence.ChurnState == ScoreNotAvailable && intelligence.Churn == nil {
		t.Error("marketing suppression destroyed the churn score; suppression is about contact, not knowledge")
	}

	// But the customer is not contactable.
	summary, err := env.service.ConsentSummaryFor(ctx, actor, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	for channel, decision := range summary.Marketing {
		if decision.Eligible {
			t.Errorf("a marketing-suppressed customer is still eligible on %s", channel)
		}
	}
}

// TestSection8I_NoCanonicalCustomerPIIInTheCRMSchema is the §7 and §69 authority
// scan, run against the live schema rather than against a reading of the code.
//
// Vendor CRM projects a relationship; customer-service owns the person. A
// column here that could hold an email address or a phone number would be a
// second copy of an authority this service does not have — and the moment one
// exists, every privacy guarantee becomes a promise about two places instead of
// one.
func TestSection8I_NoCanonicalCustomerPIIInTheCRMSchema(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	rows, err := env.pool.Query(ctx, `SELECT c.table_name, c.column_name
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_schema=c.table_schema AND t.table_name=c.table_name AND t.table_type='BASE TABLE'
		WHERE c.table_schema='public'
		  AND (c.table_name LIKE 'crm[_]%' OR c.table_name LIKE 'corporate[_]%'
		       OR c.table_name = 'vendor_customer_profiles')
		  AND c.column_name ~ '(email|phone|mobile|msisdn|date_of_birth|dob|government_id|national_id|passport|password|secret|card|iban|bank_account|wallet|balance_minor)'
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var findings []string
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		findings = append(findings, table+"."+column)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("the CRM schema holds columns that look like canonical customer or financial authority:\n%s",
			strings.Join(findings, "\n"))
	}

	// The corporate organization's own address is the one address CRM stores,
	// and it belongs to a business rather than to a person. Its presence is
	// expected; its absence would mean the scan above is not reaching the
	// tables it claims to check.
	var siteAddressColumns int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='corporate_customer_locations'
		  AND column_name IN ('address_line1','city','country_code')`).Scan(&siteAddressColumns); err != nil {
		t.Fatal(err)
	}
	if siteAddressColumns != 3 {
		t.Fatalf("the scan did not reach the corporate site table (found %d address columns)", siteAddressColumns)
	}
}

// TestSection8I_NoMonetaryAuthorityInTheCRMSchema keeps the Wallet boundary.
// Loyalty points are points; a currency or amount column here would make this
// service a second, unreconciled ledger.
func TestSection8I_NoMonetaryAuthorityInTheCRMSchema(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	rows, err := env.pool.Query(context.Background(), `SELECT c.table_name, c.column_name
		FROM information_schema.columns c
		WHERE c.table_schema='public'
		  AND c.table_name LIKE 'crm[_]loyalty%'
		  AND c.column_name ~ '(currency|amount|minor|kobo|cash|money|payout|settlement|wallet)'
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var findings []string
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		findings = append(findings, table+"."+column)
	}
	if len(findings) != 0 {
		t.Fatalf("loyalty holds monetary columns; points are not money:\n%s", strings.Join(findings, "\n"))
	}
}
