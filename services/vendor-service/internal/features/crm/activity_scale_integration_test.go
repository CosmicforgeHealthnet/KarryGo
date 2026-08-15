package crm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestActivityScale_TimelineTaskInboxAndIndexes(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	branches := []string{branchA1, branchA2, "brn_a3", "brn_a4", "brn_a5"}
	for _, branch := range branches[2:] {
		if _, err := env.pool.Exec(ctx, `INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status) VALUES($1,$2,$1,'1 Scale Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE')`, branch, businessA); err != nil {
			t.Fatal(err)
		}
	}
	const profiles = 300
	profileRows := make([][]any, 0, profiles)
	relationRows := make([][]any, 0, profiles)
	interactionRows := make([][]any, 0, profiles*5)
	taskRows := make([][]any, 0, profiles*2)
	for i := 0; i < profiles; i++ {
		profileID := fmt.Sprintf("vcp_scale_%04d", i)
		branch := branches[i%len(branches)]
		profileRows = append(profileRows, []any{profileID, businessA, "CUSTOMER", fmt.Sprintf("cus_scale_%04d", i)})
		relationRows = append(relationRows, []any{fmt.Sprintf("crmbr_scale_%04d", i), businessA, profileID, branch})
		for j := 0; j < 5; j++ {
			interactionRows = append(interactionRows, []any{fmt.Sprintf("crmi_scale_%04d_%02d", i, j), businessA, profileID, profileID, branch, "PHONE_CALL", "OUTBOUND", "SUCCESSFUL", fmt.Sprintf("Scale interaction %d", j), "Plain relationship note.", "BRANCH", env.now.Add(-time.Duration(j) * time.Hour), "vs_a"})
		}
		for j := 0; j < 2; j++ {
			due := env.now.Add(time.Duration(j*2-1) * time.Hour)
			taskRows = append(taskRows, []any{fmt.Sprintf("crmt_scale_%04d_%02d", i, j), businessA, profileID, profileID, branch, "BRANCH", fmt.Sprintf("Scale task %d", j), "Plain task description.", "OPEN", "NORMAL", "vs_a", due})
		}
	}
	if _, err := env.pool.CopyFrom(ctx, pgx.Identifier{"vendor_customer_profiles"}, []string{"id", "business_id", "subject_type", "customer_ref"}, pgx.CopyFromRows(profileRows)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.CopyFrom(ctx, pgx.Identifier{"crm_customer_branch_relations"}, []string{"id", "business_id", "profile_id", "branch_id"}, pgx.CopyFromRows(relationRows)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.CopyFrom(ctx, pgx.Identifier{"crm_interactions"}, []string{"id", "business_id", "customer_profile_id", "original_profile_id", "branch_id", "interaction_type", "direction", "outcome", "subject", "note_body", "visibility_scope", "occurred_at", "recorded_by_staff_id"}, pgx.CopyFromRows(interactionRows)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.CopyFrom(ctx, pgx.Identifier{"crm_tasks"}, []string{"id", "business_id", "customer_profile_id", "original_profile_id", "branch_id", "visibility_scope", "title", "description", "status", "priority", "created_by_staff_id", "due_at"}, pgx.CopyFromRows(taskRows)); err != nil {
		t.Fatal(err)
	}
	actor := actorFor(businessA, "vs_a")
	items, more, err := env.service.ListInteractions(ctx, actor, globalScope(), InteractionListOptions{CustomerProfileRef: "vcp_scale_0000", Limit: 3})
	if err != nil || len(items) != 3 || !more {
		t.Fatalf("timeline page=%d more=%t err=%v", len(items), more, err)
	}
	second, more2, err := env.service.ListInteractions(ctx, actor, globalScope(), InteractionListOptions{CustomerProfileRef: "vcp_scale_0000", Limit: 3, Offset: 3})
	if err != nil || len(second) != 2 || more2 || items[0].ID == second[0].ID {
		t.Fatalf("second page=%d more=%t err=%v", len(second), more2, err)
	}
	overdue := true
	tasks, _, err := env.service.ListTasks(ctx, actor, branchScope(branchA1), TaskListOptions{Overdue: &overdue, Limit: 200})
	if err != nil || len(tasks) != 60 {
		t.Fatalf("branch overdue tasks=%d want 60 err=%v", len(tasks), err)
	}
	profile, err := scanProfile(env.pool.QueryRow(ctx, `SELECT `+profileCols+` FROM vendor_customer_profiles WHERE id='vcp_scale_0000'`))
	if err != nil {
		t.Fatal(err)
	}
	view, err := env.service.Customer360For(ctx, actor, branchScope(branchA1), profile)
	if err != nil || view.RecentInteractions == nil || len(view.RecentInteractions.Items) != 5 || view.OpenTasks == nil || view.OpenTasks.Count != 2 {
		t.Fatalf("Customer360 interactions=%+v tasks=%+v err=%v", view.RecentInteractions, view.OpenTasks, err)
	}
	if _, err = env.pool.Exec(ctx, `SET enable_seqscan=off`); err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]struct {
		sql  string
		args []any
	}{"interaction": {"EXPLAIN SELECT id FROM crm_interactions WHERE business_id=$1 AND customer_profile_id=$2 AND display_suppressed=false ORDER BY occurred_at DESC,id DESC LIMIT 5", []any{businessA, "vcp_scale_0000"}}, "task": {"EXPLAIN SELECT id FROM crm_tasks WHERE business_id=$1 AND status IN ('OPEN','IN_PROGRESS') AND due_at<$2 AND display_suppressed=false ORDER BY due_at LIMIT 50", []any{businessA, env.now}}} {
		rows, queryErr := env.pool.Query(ctx, query.sql, query.args...)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		var plan []string
		for rows.Next() {
			var line string
			if queryErr = rows.Scan(&line); queryErr != nil {
				rows.Close()
				t.Fatal(queryErr)
			}
			plan = append(plan, line)
		}
		rows.Close()
		joined := strings.Join(plan, "\n")
		if !strings.Contains(joined, "Index") {
			t.Fatalf("%s query has no index path:\n%s", name, joined)
		}
	}
}
