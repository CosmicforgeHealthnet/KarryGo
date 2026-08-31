package crm

import (
	"context"
	"testing"
)

// Contacts are where an organization relationship touches an individual, so
// these tests are mostly about the boundary: no personal data is copied,
// privacy decisions are honoured immediately, and an identity merge leaves
// exactly one actionable relationship.

func TestCorporateContact_LinksAProfileWithoutCopyingPersonalData(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)

	if contact.CustomerProfileID != person.ID || contact.OriginalProfileID != person.ID {
		t.Fatalf("the contact does not reference the CRM relationship: %+v", contact)
	}
	if !contact.IsPrimary || contact.Status != CorporateChildActive {
		t.Fatalf("unexpected contact state: %+v", contact)
	}

	// The contacts table holds no personal data at all — not because a field
	// was omitted, but because there is nowhere to put one.
	var columns int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN ('corporate_customer_contacts','corporate_customer_memberships')
		  AND column_name ~ '(email|phone|msisdn|address|first_name|last_name|full_name|date_of_birth|password|token)'`).
		Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatalf("the corporate contact schema holds %d personal-data columns", columns)
	}

	// The presentation label is joined from the Section 8A profile at read
	// time, so it cannot go stale and lives in exactly one place.
	read, err := env.service.GetCorporateContact(ctx, actorFor(businessA, "vs_a"), globalScope(), profile.ID, contact.ID)
	if err != nil {
		t.Fatalf("read contact: %v", err)
	}
	if read.SafeDisplayLabel != person.SafeDisplayLabel {
		t.Fatalf("the display label was not resolved from the profile: %v", read.SafeDisplayLabel)
	}
}

func TestCorporateContact_RejectsUnusableProfiles(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")

	t.Run("another tenant's relationship", func(t *testing.T) {
		foreign := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_foreign"))
		if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
			CorporateContactInput{CustomerProfileRef: foreign.ID}); err == nil {
			t.Fatal("another tenant's relationship was linked as a contact")
		} else if !isCRMCode(err, CodeCorporateContactProfile) {
			t.Fatalf("expected an invalid-profile rejection, got %v", err)
		}
	})

	t.Run("a redacted relationship", func(t *testing.T) {
		erased := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))
		if err := env.applyPrivacy(t, businessA, "vs_a", &erased, PrivacyInput{
			SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
			Action: PrivacyActionRedact, ObservedAt: env.now}); err != nil {
			t.Fatalf("redact: %v", err)
		}
		if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
			CorporateContactInput{CustomerProfileRef: erased.ID}); err == nil {
			t.Fatal("a redacted relationship was linked as a contact")
		} else if !isCRMCode(err, CodeCorporateContactRedacted) {
			t.Fatalf("expected a redaction rejection, got %v", err)
		}
	})

	t.Run("a merged-away relationship", func(t *testing.T) {
		guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_merged"))
		survivor := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_survivor"))
		if err := env.applyLink(t, businessA, "vs_a",
			linkInput("evt_contact_merge", guestSubject("gst_merged"), customerSubject("cus_survivor"), 4)); err != nil {
			t.Fatalf("merge: %v", err)
		}
		// Refused rather than silently redirected: quietly linking a different
		// person than the caller named would be worse than telling them.
		if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
			CorporateContactInput{CustomerProfileRef: guest.ID}); err == nil {
			t.Fatal("a merged-away relationship was linked as a contact")
		} else if !isCRMCode(err, CodeCorporateContactMerged) {
			t.Fatalf("expected a merge rejection, got %v", err)
		}
		if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
			CorporateContactInput{CustomerProfileRef: survivor.ID}); err != nil {
			t.Fatalf("the survivor should be linkable: %v", err)
		}
	})

	t.Run("an unknown relationship", func(t *testing.T) {
		if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
			CorporateContactInput{CustomerProfileRef: "vcp_does_not_exist"}); err == nil {
			t.Fatal("an unknown relationship was linked as a contact")
		}
	})
}

func TestCorporateContact_DuplicateAndPrimaryRules(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	first := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_one"))
	second := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_two"))

	original := env.addCorporateContact(t, businessA, "vs_a", profile.ID, first.ID, ContactRoleProcurement, true)
	if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
		CorporateContactInput{CustomerProfileRef: first.ID}); err == nil {
		t.Fatal("the same person was linked twice to one account")
	} else if !isCRMCode(err, CodeCorporateContactDup) {
		t.Fatalf("expected a duplicate rejection, got %v", err)
	}

	// Promoting a second person demotes the incumbent inside the same
	// transaction, so "one primary contact" holds even under concurrent writes.
	promoted := env.addCorporateContact(t, businessA, "vs_a", profile.ID, second.ID, ContactRoleManagement, true)
	if !promoted.IsPrimary {
		t.Fatal("the new contact should be primary")
	}
	if env.reloadCorporateContact(t, businessA, profile.ID, original.ID).IsPrimary {
		t.Fatal("the previous primary contact was not demoted")
	}
	var primaries int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
		WHERE business_id=$1 AND corporate_customer_id=$2 AND is_primary_contact AND status='ACTIVE'`,
		businessA, profile.ID).Scan(&primaries); err != nil {
		t.Fatal(err)
	}
	if primaries != 1 {
		t.Fatalf("expected exactly one primary contact, got %d", primaries)
	}

	// The same person may be a contact of a DIFFERENT account.
	other := env.activeCorporate(t, businessA, "vs_a", "Beta Schools")
	if _, err := env.service.CreateCorporateContact(ctx, actor, globalScope(), other.ID,
		CorporateContactInput{CustomerProfileRef: first.ID}); err != nil {
		t.Fatalf("one person should be able to represent two organizations: %v", err)
	}
}

func TestCorporateContact_UpdateAndArchive(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)
	membership := env.addCorporateMembership(t, businessA, "vs_a", profile.ID,
		CorporateMembershipInput{ContactRef: contact.ID, MembershipRole: MembershipApprover})

	updated, err := env.service.UpdateCorporateContact(ctx, actor, globalScope(), profile.ID, contact.ID,
		CorporateContactInput{RelationshipRole: ContactRoleFinance, ExpectedVersion: contact.Version})
	if err != nil {
		t.Fatalf("update contact: %v", err)
	}
	if updated.RelationshipRole != ContactRoleFinance {
		t.Fatalf("the role was not updated: %s", updated.RelationshipRole)
	}
	// A stale expected_version conflicts rather than silently winning.
	if _, err = env.service.UpdateCorporateContact(ctx, actor, globalScope(), profile.ID, contact.ID,
		CorporateContactInput{RelationshipRole: ContactRoleOther, ExpectedVersion: contact.Version}); err == nil {
		t.Fatal("a stale contact version should conflict")
	}
	// Archiving is its own command; an ordinary edit cannot retire a contact.
	if _, err = env.service.UpdateCorporateContact(ctx, actor, globalScope(), profile.ID, contact.ID,
		CorporateContactInput{Status: CorporateChildArchived, ExpectedVersion: updated.Version}); err == nil {
		t.Fatal("an edit should not be able to archive a contact")
	}

	archived, err := env.service.ArchiveCorporateContact(ctx, actor, globalScope(), profile.ID, contact.ID,
		updated.Version)
	if err != nil {
		t.Fatalf("archive contact: %v", err)
	}
	if archived.Status != CorporateChildArchived || archived.IsPrimary {
		t.Fatalf("unexpected archived state: %+v", archived)
	}
	// Memberships stop being actionable with the contact and survive as
	// evidence of the arrangement.
	if reloaded := env.reloadMembership(t, businessA, membership.ID); reloaded.Status != CorporateChildInactive {
		t.Fatalf("the membership should have been deactivated, got %s", reloaded.Status)
	}
	// The person can be linked again after the old relationship was archived.
	if _, err = env.service.CreateCorporateContact(ctx, actor, globalScope(), profile.ID,
		CorporateContactInput{CustomerProfileRef: person.ID}); err != nil {
		t.Fatalf("re-linking after archive should be allowed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Memberships
// ---------------------------------------------------------------------------

func TestCorporateMembership_BindsWithinOneAccountOnly(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	acme := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	beta := env.activeCorporate(t, businessA, "vs_a", "Beta Schools")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	contact := env.addCorporateContact(t, businessA, "vs_a", acme.ID, person.ID, ContactRoleProcurement, false)

	acmeSite := env.addCorporateLocation(t, businessA, "vs_a", acme.ID, "HQ", "Head office")
	acmeCenter := env.addCorporateCostCenter(t, businessA, "vs_a", acme.ID, "OPS", "Operations")
	betaSite := env.addCorporateLocation(t, businessA, "vs_a", beta.ID, "HQ", "Campus")
	betaCenter := env.addCorporateCostCenter(t, businessA, "vs_a", beta.ID, "OPS", "Operations")

	membership := env.addCorporateMembership(t, businessA, "vs_a", acme.ID, CorporateMembershipInput{
		ContactRef: contact.ID, LocationRef: acmeSite.ID, CostCenterRef: acmeCenter.ID,
		MembershipRole: MembershipApprover})
	if membership.LocationID == nil || *membership.LocationID != acmeSite.ID {
		t.Fatalf("the location was not bound: %+v", membership)
	}

	// Cross-account references are refused. The composite foreign keys make
	// them unstorable; these errors are the explaining layer over that.
	if _, err := env.service.CreateCorporateMembership(ctx, actor, globalScope(), acme.ID,
		CorporateMembershipInput{ContactRef: contact.ID, LocationRef: betaSite.ID}); err == nil {
		t.Fatal("a membership bound another account's location")
	}
	if _, err := env.service.CreateCorporateMembership(ctx, actor, globalScope(), acme.ID,
		CorporateMembershipInput{ContactRef: contact.ID, CostCenterRef: betaCenter.ID}); err == nil {
		t.Fatal("a membership bound another account's cost centre")
	}
	// A contact from another account, presented under this one, is not found.
	betaPerson := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_beta"))
	betaContact := env.addCorporateContact(t, businessA, "vs_a", beta.ID, betaPerson.ID, ContactRoleOther, false)
	if _, err := env.service.CreateCorporateMembership(ctx, actor, globalScope(), acme.ID,
		CorporateMembershipInput{ContactRef: betaContact.ID}); err == nil {
		t.Fatal("a membership bound another account's contact")
	}

	// A duplicate arrangement is refused rather than written twice.
	if _, err := env.service.CreateCorporateMembership(ctx, actor, globalScope(), acme.ID,
		CorporateMembershipInput{ContactRef: contact.ID, LocationRef: acmeSite.ID,
			CostCenterRef: acmeCenter.ID, MembershipRole: MembershipApprover}); err == nil {
		t.Fatal("a duplicate membership was accepted")
	} else if !isCRMCode(err, CodeCorporateDuplicate) {
		t.Fatalf("expected a duplicate rejection, got %v", err)
	}
	// A different role at the same place is a different arrangement.
	if _, err := env.service.CreateCorporateMembership(ctx, actor, globalScope(), acme.ID,
		CorporateMembershipInput{ContactRef: contact.ID, LocationRef: acmeSite.ID,
			CostCenterRef: acmeCenter.ID, MembershipRole: MembershipRequester}); err != nil {
		t.Fatalf("a different membership role should be allowed: %v", err)
	}
}

func TestCorporateMembership_UpdateAndArchive(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, false)
	site := env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "HQ", "Head office")
	membership := env.addCorporateMembership(t, businessA, "vs_a", profile.ID,
		CorporateMembershipInput{ContactRef: contact.ID, LocationRef: site.ID})

	updated, err := env.service.UpdateCorporateMembership(ctx, actor, globalScope(), profile.ID, membership.ID,
		CorporateMembershipInput{MembershipRole: MembershipManager, ExpectedVersion: membership.Version})
	if err != nil {
		t.Fatalf("update membership: %v", err)
	}
	if updated.MembershipRole != MembershipManager {
		t.Fatalf("the membership role was not updated: %s", updated.MembershipRole)
	}
	cleared, err := env.service.UpdateCorporateMembership(ctx, actor, globalScope(), profile.ID, membership.ID,
		CorporateMembershipInput{ClearLocation: true, ExpectedVersion: updated.Version})
	if err != nil {
		t.Fatalf("clear location: %v", err)
	}
	if cleared.LocationID != nil {
		t.Fatal("the location was not cleared")
	}
	archived, err := env.service.ArchiveCorporateMembership(ctx, actor, globalScope(), profile.ID, membership.ID,
		cleared.Version)
	if err != nil {
		t.Fatalf("archive membership: %v", err)
	}
	if archived.Status != CorporateChildArchived {
		t.Fatalf("unexpected archived state: %s", archived.Status)
	}
}

// ---------------------------------------------------------------------------
// Privacy and identity merge
// ---------------------------------------------------------------------------

// TestCorporateContact_RedactionSuppressesThePersonNotTheOrganization is the
// central privacy property of Section 8G. A person's erasure must not delete a
// company's account.
func TestCorporateContact_RedactionSuppressesThePersonNotTheOrganization(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))
	other := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_kept"))
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)
	kept := env.addCorporateContact(t, businessA, "vs_a", profile.ID, other.ID, ContactRoleOperations, false)
	membership := env.addCorporateMembership(t, businessA, "vs_a", profile.ID,
		CorporateMembershipInput{ContactRef: contact.ID, MembershipRole: MembershipApprover})
	site := env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "HQ", "Head office")
	interaction := env.addCorporateInteraction(t, businessA, "vs_a", profile.ID,
		CreateCorporateInteractionInput{ContactRef: contact.ID})

	reloaded := env.reload(t, businessA, person.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &reloaded, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
		Action: PrivacyActionRedact, ObservedAt: env.now}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	// The organization is untouched.
	if env.reloadCorporate(t, businessA, profile.ID).Status != CorporateActive {
		t.Fatal("redacting a person disturbed the corporate account")
	}
	if _, err := env.service.GetCorporateLocation(ctx, actor, globalScope(), profile.ID, site.ID); err != nil {
		t.Fatalf("redacting a person removed the organization's site: %v", err)
	}
	if _, err := env.service.GetCorporateInteraction(ctx, actor, globalScope(), profile.ID, interaction.ID); err != nil {
		t.Fatalf("redacting a person removed commercial history: %v", err)
	}

	// The person stops being actionable.
	suppressed := env.reloadCorporateContact(t, businessA, profile.ID, contact.ID)
	if !suppressed.PrivacySuppressed || suppressed.IsPrimary || suppressed.Actionable() {
		t.Fatalf("the contact was not suppressed: %+v", suppressed)
	}
	if reloadedMembership := env.reloadMembership(t, businessA, membership.ID); reloadedMembership.Status != CorporateChildInactive {
		t.Fatalf("the membership stayed actionable after redaction: %s", reloadedMembership.Status)
	}

	// They disappear from the ordinary contact list, and their label is gone.
	contacts, _, err := env.service.ListCorporateContacts(ctx, actor, globalScope(), profile.ID, "", "", false, 0, 0)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if len(contacts) != 1 || contacts[0].ID != kept.ID {
		t.Fatalf("a redacted contact is still listed: %+v", contacts)
	}
	withSuppressed, _, err := env.service.ListCorporateContacts(ctx, actor, globalScope(), profile.ID, "", "", true, 0, 0)
	if err != nil {
		t.Fatalf("compliance list: %v", err)
	}
	if len(withSuppressed) != 2 {
		t.Fatalf("the compliance list should still account for the relationship: %d", len(withSuppressed))
	}
	for _, entry := range withSuppressed {
		if entry.PrivacySuppressed && entry.SafeDisplayLabel != nil {
			t.Fatal("a redacted contact still exposes a presentation label")
		}
	}

	// Nothing new may be recorded against them.
	if _, err = env.service.CreateCorporateMembership(ctx, actor, globalScope(), profile.ID,
		CorporateMembershipInput{ContactRef: contact.ID, MembershipRole: MembershipMember}); err == nil {
		t.Fatal("a membership was created for a redacted contact")
	}
	if _, err = env.service.CreateCorporateInteraction(ctx, actor, globalScope(), profile.ID,
		CreateCorporateInteractionInput{Type: InteractionNote, Direction: DirectionInternal,
			Subject: "Note", OccurredAt: env.now, ContactRef: contact.ID}); err == nil {
		t.Fatal("an interaction was linked to a redacted contact")
	}
	if _, err = env.service.CreateCorporateTask(ctx, actor, globalScope(), profile.ID,
		CreateCorporateTaskInput{Title: "Call them", ContactRef: contact.ID}); err == nil {
		t.Fatal("a task was linked to a redacted contact")
	}
	if _, err = env.service.UpdateCorporateContact(ctx, actor, globalScope(), profile.ID, contact.ID,
		CorporateContactInput{RelationshipRole: ContactRoleOther, ExpectedVersion: suppressed.Version}); err == nil {
		t.Fatal("a redacted contact was edited")
	}
}

// TestCorporateContact_IdentityMergeLeavesOneActionableRelationship covers both
// merge shapes: the survivor already being a contact of the same account, and
// the survivor not being one.
func TestCorporateContact_IdentityMergeLeavesOneActionableRelationship(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	both := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	onlySource := env.activeCorporate(t, businessA, "vs_a", "Beta Schools")

	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_1"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))

	// Account 1: both people are contacts, and the SOURCE is the primary.
	sourceContact := env.addCorporateContact(t, businessA, "vs_a", both.ID, guest.ID, ContactRoleProcurement, true)
	targetContact := env.addCorporateContact(t, businessA, "vs_a", both.ID, customer.ID, ContactRoleOperations, false)
	site := env.addCorporateLocation(t, businessA, "vs_a", both.ID, "HQ", "Head office")
	sourceMembership := env.addCorporateMembership(t, businessA, "vs_a", both.ID, CorporateMembershipInput{
		ContactRef: sourceContact.ID, LocationRef: site.ID, MembershipRole: MembershipApprover})

	// Account 2: only the source is a contact.
	loneContact := env.addCorporateContact(t, businessA, "vs_a", onlySource.ID, guest.ID, ContactRoleFinance, true)

	if err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_corp_merge", guestSubject("gst_1"), customerSubject("cus_1"), 4)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	t.Run("where both were contacts the duplicate is retired", func(t *testing.T) {
		duplicate := env.reloadCorporateContact(t, businessA, both.ID, sourceContact.ID)
		if duplicate.Status != CorporateChildArchived {
			t.Fatalf("the source relationship is still live: %s", duplicate.Status)
		}
		survivor := env.reloadCorporateContact(t, businessA, both.ID, targetContact.ID)
		if survivor.Status != CorporateChildActive {
			t.Fatalf("the surviving relationship was disturbed: %s", survivor.Status)
		}
		// The vendor's decision about who is primary survives the merge.
		if !survivor.IsPrimary {
			t.Fatal("the account lost its primary contact to the merge")
		}
		// Exactly one live relationship for one human.
		var live int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
			WHERE business_id=$1 AND corporate_customer_id=$2 AND status <> 'ARCHIVED'`,
			businessA, both.ID).Scan(&live); err != nil {
			t.Fatal(err)
		}
		if live != 1 {
			t.Fatalf("expected one live contact after the merge, got %d", live)
		}
		// The source's membership followed the person to the survivor.
		moved := env.reloadMembership(t, businessA, sourceMembership.ID)
		if moved.ContactID != targetContact.ID {
			t.Fatalf("the membership did not follow the merge: %s", moved.ContactID)
		}
		if moved.Status == CorporateChildArchived {
			t.Fatal("the membership was discarded rather than moved")
		}
	})

	t.Run("where only the source was a contact the relationship moves", func(t *testing.T) {
		moved := env.reloadCorporateContact(t, businessA, onlySource.ID, loneContact.ID)
		if moved.CustomerProfileID != customer.ID {
			t.Fatalf("the relationship did not canonicalize: %s", moved.CustomerProfileID)
		}
		if moved.Status != CorporateChildActive || !moved.IsPrimary {
			t.Fatalf("the moved relationship lost its state: %+v", moved)
		}
		// The history of who it was created against stays readable.
		if moved.OriginalProfileID != guest.ID {
			t.Fatalf("the original profile reference was rewritten: %s", moved.OriginalProfileID)
		}
	})

	t.Run("the merged-away person is no longer independently actionable", func(t *testing.T) {
		var live int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM corporate_customer_contacts
			WHERE business_id=$1 AND customer_profile_id=$2 AND status <> 'ARCHIVED'`,
			businessA, guest.ID).Scan(&live); err != nil {
			t.Fatal(err)
		}
		if live != 0 {
			t.Fatalf("%d live relationships still point at the merged-away person", live)
		}
	})

	t.Run("the corporate accounts are untouched", func(t *testing.T) {
		for _, id := range []string{both.ID, onlySource.ID} {
			if env.reloadCorporate(t, businessA, id).Status != CorporateActive {
				t.Fatal("a merge disturbed a corporate account")
			}
		}
		_ = actor
	})
}
