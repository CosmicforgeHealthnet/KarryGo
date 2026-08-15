package crm

import (
	"context"
	"testing"
	"time"
)

func (e *testEnv) activityProfile(t *testing.T, ref, branch string) Profile {
	t.Helper()
	p := e.seedProfile(t, businessA, "vs_a", customerSubject(ref))
	e.seedOrder(t, businessA, "vs_a", p, "ord_"+ref, branch, 10_000, 10_000, e.now.Add(-24*time.Hour), 1)
	return e.reload(t, businessA, p.ID)
}

func (e *testEnv) addStaff(t *testing.T, id, business string, branch *string, active bool) {
	t.Helper()
	status := "active"
	if !active {
		status = "suspended"
	}
	userID := "vu_" + id
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO vendor_users(id,email) VALUES($1,$1||'@example.test')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,$4)`, id, business, userID, status); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id) VALUES($1,$2,'role_crm_support',$3)`, `vra_`+id, id, branch); err != nil {
		t.Fatal(err)
	}
}

func basicInteraction(profile Profile, when time.Time) CreateInteractionInput {
	return CreateInteractionInput{CustomerProfileRef: profile.ID, Type: InteractionPhoneCall, Direction: DirectionOutbound,
		Outcome: OutcomeFollowUpRequired, Subject: "Follow-up call", Note: "Customer requested another conversation.", Visibility: VisibilityBusiness, OccurredAt: when}
}

func basicTask(profile Profile) CreateTaskInput {
	return CreateTaskInput{CustomerProfileRef: profile.ID, Visibility: VisibilityBusiness, Title: "Call customer", Description: "Follow up on the commercial request.", Priority: PriorityNormal}
}
