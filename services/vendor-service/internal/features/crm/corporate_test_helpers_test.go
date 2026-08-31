package crm

import (
	"context"
	"testing"
)

// Section 8G test helpers. They drive the real service methods rather than
// inserting rows, so a test that says "add a contact" exercises the same
// privacy fence, validation and audit path production does.

func (e *testEnv) createCorporate(t *testing.T, business, staff string, in CorporateProfileInput) CorporateProfile {
	t.Helper()
	if in.OrganizationName == "" {
		in.OrganizationName = "Acme Industries"
	}
	profile, err := e.service.CreateCorporateCustomer(context.Background(), actorFor(business, staff), globalScope(), in)
	if err != nil {
		t.Fatalf("create corporate customer %s: %v", in.OrganizationName, err)
	}
	return profile
}

// activeCorporate is the common fixture: a business-scoped account already in
// ACTIVE, because most behaviour under test is about a live relationship.
func (e *testEnv) activeCorporate(t *testing.T, business, staff, name string) CorporateProfile {
	t.Helper()
	profile := e.createCorporate(t, business, staff, CorporateProfileInput{OrganizationName: name})
	activated, err := e.service.TransitionCorporateCustomer(context.Background(), actorFor(business, staff),
		globalScope(), profile.ID, CorporateActive, profile.Version)
	if err != nil {
		t.Fatalf("activate corporate customer %s: %v", name, err)
	}
	return activated
}

// branchCorporate is the branch-scoped equivalent.
func (e *testEnv) branchCorporate(t *testing.T, business, staff, name string, branches ...string) CorporateProfile {
	t.Helper()
	profile := e.createCorporate(t, business, staff, CorporateProfileInput{
		OrganizationName: name, ScopeMode: CorporateScopeBranchSet, BranchRefs: branches})
	activated, err := e.service.TransitionCorporateCustomer(context.Background(), actorFor(business, staff),
		globalScope(), profile.ID, CorporateActive, profile.Version)
	if err != nil {
		t.Fatalf("activate branch corporate customer %s: %v", name, err)
	}
	return activated
}

func (e *testEnv) reloadCorporate(t *testing.T, business, corporateID string) CorporateProfile {
	t.Helper()
	profile, err := e.service.loadCorporate(context.Background(), e.pool, business, corporateID)
	if err != nil {
		t.Fatalf("reload corporate customer: %v", err)
	}
	return profile
}

func (e *testEnv) addCorporateContact(t *testing.T, business, staff, corporateID, profileID, role string, primary bool) CorporateContact {
	t.Helper()
	contact, err := e.service.CreateCorporateContact(context.Background(), actorFor(business, staff), globalScope(),
		corporateID, CorporateContactInput{CustomerProfileRef: profileID, RelationshipRole: role, IsPrimary: &primary})
	if err != nil {
		t.Fatalf("add corporate contact: %v", err)
	}
	return contact
}

func (e *testEnv) reloadCorporateContact(t *testing.T, business, corporateID, contactID string) CorporateContact {
	t.Helper()
	contact, err := scanCorporateContact(e.pool.QueryRow(context.Background(),
		`SELECT `+corporateContactCols+` FROM corporate_customer_contacts
		 WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3`, business, corporateID, contactID))
	if err != nil {
		t.Fatalf("reload corporate contact: %v", err)
	}
	return contact
}

func (e *testEnv) addCorporateLocation(t *testing.T, business, staff, corporateID, code, name string) CorporateLocation {
	t.Helper()
	location, err := e.service.CreateCorporateLocation(context.Background(), actorFor(business, staff), globalScope(),
		corporateID, CorporateLocationInput{Code: code, Name: name, LocationType: LocationOffice})
	if err != nil {
		t.Fatalf("add corporate location: %v", err)
	}
	return location
}

func (e *testEnv) addCorporateCostCenter(t *testing.T, business, staff, corporateID, code, name string) CorporateCostCenter {
	t.Helper()
	center, err := e.service.CreateCorporateCostCenter(context.Background(), actorFor(business, staff), globalScope(),
		corporateID, CorporateCostCenterInput{Code: code, Name: name})
	if err != nil {
		t.Fatalf("add corporate cost centre: %v", err)
	}
	return center
}

func (e *testEnv) addCorporateMembership(t *testing.T, business, staff, corporateID string, in CorporateMembershipInput) CorporateMembership {
	t.Helper()
	membership, err := e.service.CreateCorporateMembership(context.Background(), actorFor(business, staff),
		globalScope(), corporateID, in)
	if err != nil {
		t.Fatalf("add corporate membership: %v", err)
	}
	return membership
}

func (e *testEnv) reloadMembership(t *testing.T, business, membershipID string) CorporateMembership {
	t.Helper()
	membership, err := scanCorporateMembership(e.pool.QueryRow(context.Background(),
		`SELECT `+corporateMembershipCols+` FROM corporate_customer_memberships WHERE business_id=$1 AND id=$2`,
		business, membershipID))
	if err != nil {
		t.Fatalf("reload membership: %v", err)
	}
	return membership
}

func (e *testEnv) addCorporateInteraction(t *testing.T, business, staff, corporateID string, in CreateCorporateInteractionInput) Interaction {
	t.Helper()
	if in.Type == "" {
		in.Type = InteractionAccountReview
	}
	if in.Direction == "" {
		in.Direction = DirectionOutbound
	}
	if in.Subject == "" {
		in.Subject = "Quarterly account review"
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = e.now
	}
	interaction, err := e.service.CreateCorporateInteraction(context.Background(), actorFor(business, staff),
		globalScope(), corporateID, in)
	if err != nil {
		t.Fatalf("add corporate interaction: %v", err)
	}
	return interaction
}

func (e *testEnv) addCorporateTask(t *testing.T, business, staff, corporateID string, in CreateCorporateTaskInput) Task {
	t.Helper()
	if in.Title == "" {
		in.Title = "Call procurement"
	}
	task, err := e.service.CreateCorporateTask(context.Background(), actorFor(business, staff), globalScope(),
		corporateID, in)
	if err != nil {
		t.Fatalf("add corporate task: %v", err)
	}
	return task
}

func (e *testEnv) reloadCorporateTask(t *testing.T, business, taskID string) Task {
	t.Helper()
	task, err := scanTask(e.pool.QueryRow(context.Background(),
		`SELECT `+taskCols+` FROM crm_tasks WHERE business_id=$1 AND id=$2`, business, taskID))
	if err != nil {
		t.Fatalf("reload corporate task: %v", err)
	}
	return task
}

// seedBranchStaff creates an additional active staff member holding a
// branch-scoped role assignment, so assignee-eligibility rules can be tested
// against somebody who is not the owner.
func (e *testEnv) seedBranchStaff(t *testing.T, business, staffID, branchID string) string {
	t.Helper()
	ctx := context.Background()
	userID := "vu_" + staffID
	if _, err := e.pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$1||'@example.test')
		ON CONFLICT DO NOTHING`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status)
		VALUES($1,$2,$3,'active') ON CONFLICT DO NOTHING`, staffID, business, userID); err != nil {
		t.Fatal(err)
	}
	var branch *string
	if branchID != "" {
		branch = &branchID
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id)
		VALUES($1,$2,'role_branch_manager',$3) ON CONFLICT DO NOTHING`, "vra_"+staffID, staffID, branch); err != nil {
		t.Fatal(err)
	}
	return staffID
}
