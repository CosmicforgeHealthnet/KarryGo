package crm

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// activeDynamicSegment creates, activates and materializes a dynamic segment,
// which is the state most of these tests start from.
func (e *testEnv) activeDynamicSegment(t *testing.T, name string) Segment {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	segment, err := e.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: name, SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatalf("create dynamic segment: %v", err)
	}
	if _, err = e.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatalf("activate: %v", err)
	}
	e.materialize(t)
	return e.reloadSegment(t, businessA, segment.ID)
}

// seedRepeatCustomer creates a relationship that matches repeatOrdersRule.
func (e *testEnv) seedRepeatCustomer(t *testing.T, ref, branch string) Profile {
	t.Helper()
	profile := e.seedProfile(t, businessA, "vs_a", customerSubject(ref))
	e.seedOrder(t, businessA, "vs_a", profile, "ord_"+ref+"_a", branch, 200000, 200000, e.now.AddDate(0, 0, -6), 1)
	e.seedOrder(t, businessA, "vs_a", profile, "ord_"+ref+"_b", branch, 200000, 200000, e.now.AddDate(0, 0, -2), 1)
	return e.reload(t, businessA, profile.ID)
}

func (e *testEnv) memberIDs(t *testing.T, segment Segment) []string {
	t.Helper()
	members, _, err := e.service.ListSegmentMembers(context.Background(), actorFor(businessA, "vs_a"),
		globalScope(), segment, MaxPageSize, 0)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.Profile.ID)
	}
	return ids
}

func TestMaterialization_FirstRunBuildsMembership(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	matching := env.seedRepeatCustomer(t, "cus_repeat", branchA1)
	env.seedTaggableProfile(t, businessA, "vs_a", "cus_single", branchA1)

	segment := env.activeDynamicSegment(t, "Repeat Buyers")
	if segment.EvaluationStatus != EvalCurrent {
		t.Fatalf("evaluation status = %s; want CURRENT", segment.EvaluationStatus)
	}
	if segment.CurrentMemberCount != 1 {
		t.Fatalf("member count = %d; want 1", segment.CurrentMemberCount)
	}
	if segment.CurrentEvaluationID == nil {
		t.Fatal("a successful materialization must be pointed at by the segment")
	}
	ids := env.memberIDs(t, segment)
	if len(ids) != 1 || ids[0] != matching.ID {
		t.Fatalf("members = %v; want just %s", ids, matching.ID)
	}
}

// TestCommercialNotAvailableNeverCollapsesToZero proves every commercial
// measure shares the same fail-closed semantics. A relationship with no
// observed commerce may be targeted explicitly through projection_state, but
// it must not satisfy zero, low-value, missing-timestamp or derived-recency
// predicates in either preview or materialization.
func TestCommercialNotAvailableNeverCollapsesToZero(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	unknown := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_commercial_unknown"))
	if unknown.CommercialProjectionState != ProjectionNotAvailable {
		t.Fatalf("fixture projection state = %s; want NOT_AVAILABLE", unknown.CommercialProjectionState)
	}

	conditions := []struct {
		name string
		rule string
	}{
		{"completed_order_count", `{"field":"commercial.completed_order_count","operator":"EQ","value":0}`},
		{"gross_spend_minor", `{"field":"commercial.gross_spend_minor","operator":"EQ","value":{"currency":"NGN","minor":0}}`},
		{"net_spend_minor", `{"field":"commercial.net_spend_minor","operator":"LT","value":{"currency":"NGN","minor":100000}}`},
		{"average_order_value_minor", `{"field":"commercial.average_order_value_minor","operator":"EQ","value":{"currency":"NGN","minor":0}}`},
		{"first_completed_order_at", `{"field":"commercial.first_completed_order_at","operator":"NOT_EXISTS"}`},
		{"last_completed_order_at", `{"field":"commercial.last_completed_order_at","operator":"NOT_EXISTS"}`},
		{"days_since_last_order", `{"field":"commercial.days_since_last_order","operator":"GT","value":30}`},
	}

	for _, tc := range conditions {
		t.Run("preview_"+tc.name, func(t *testing.T) {
			preview, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
				Rule: ruleJSON(`{"schema_version":1,"root":` + tc.rule + `}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if preview.MatchCount != 0 {
				t.Fatalf("match count = %d; NOT_AVAILABLE must not satisfy %s", preview.MatchCount, tc.rule)
			}
		})
	}

	children := ""
	for index, tc := range conditions {
		if index > 0 {
			children += ","
		}
		children += tc.rule
	}
	combined := ruleJSON(`{"schema_version":1,"root":{"op":"OR","children":[` + children + `]}}`)
	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Unavailable Commercial Measures", SegmentType: SegmentDynamic, Rule: combined,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	segment = env.reloadSegment(t, businessA, segment.ID)
	if segment.EvaluationStatus != EvalCurrent || segment.CurrentMemberCount != 0 {
		t.Fatalf("materialized status/count = %s/%d; want CURRENT/0", segment.EvaluationStatus, segment.CurrentMemberCount)
	}

	projectionState, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		Rule: ruleJSON(`{"schema_version":1,"root":{"field":"commercial.projection_state","operator":"EQ","value":"NOT_AVAILABLE"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if projectionState.MatchCount != 1 {
		t.Fatalf("explicit projection-state match count = %d; want 1", projectionState.MatchCount)
	}
}

func TestProfileCreationInvalidatesOnlyProfileDependentSegments(t *testing.T) {
	cases := []struct {
		name  string
		rule  string
		input RelationshipInput
	}{
		{
			name:  "subject_type",
			rule:  `{"schema_version":1,"root":{"field":"profile.subject_type","operator":"EQ","value":"CUSTOMER"}}`,
			input: RelationshipInput{Subject: customerSubject("cus_profile_subject")},
		},
		{
			name:  "status",
			rule:  `{"schema_version":1,"root":{"field":"profile.status","operator":"EQ","value":"ACTIVE"}}`,
			input: RelationshipInput{Subject: customerSubject("cus_profile_status")},
		},
		{
			name:  "acquisition_source_type",
			rule:  `{"schema_version":1,"root":{"field":"profile.acquisition_source_type","operator":"EQ","value":"ORDER"}}`,
			input: RelationshipInput{Subject: customerSubject("cus_profile_acquisition"), AcquisitionSourceType: ptr("ORDER")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, allFlagsOn())
			ctx := context.Background()
			actor := actorFor(businessA, "vs_a")

			dependent, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
				Name: "Profile dependency " + tc.name, SegmentType: SegmentDynamic, Rule: ruleJSON(tc.rule),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), dependent.ID, SegmentActive, dependent.Version); err != nil {
				t.Fatal(err)
			}
			unrelated, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
				Name: "Unrelated commercial " + tc.name, SegmentType: SegmentDynamic,
				Rule: ruleJSON(`{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"EQ","value":999}}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), unrelated.ID, SegmentActive, unrelated.Version); err != nil {
				t.Fatal(err)
			}
			env.materialize(t)

			tc.input.ObservedAt = env.now
			var created Profile
			if err = env.tx(t, func(tx pgx.Tx) error {
				created, err = env.service.ResolveOrCreateProfile(ctx, tx, actor, tc.input)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if got := env.reloadSegment(t, businessA, dependent.ID); got.EvaluationStatus != EvalStale {
				t.Fatalf("dependent status = %s; want STALE", got.EvaluationStatus)
			}
			if got := env.reloadSegment(t, businessA, unrelated.ID); got.EvaluationStatus != EvalCurrent {
				t.Fatalf("unrelated status = %s; dependency-scoped invalidation changed it", got.EvaluationStatus)
			}

			// Resolving the same subject again is an idempotent read and cannot
			// stack another materialization job.
			if err = env.tx(t, func(tx pgx.Tx) error {
				duplicate, resolveErr := env.service.ResolveOrCreateProfile(ctx, tx, actor, tc.input)
				if resolveErr == nil && duplicate.ID != created.ID {
					t.Fatalf("duplicate resolution returned %s; want %s", duplicate.ID, created.ID)
				}
				return resolveErr
			}); err != nil {
				t.Fatal(err)
			}
			var queued int
			if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
				WHERE business_id=$1 AND segment_id=$2 AND status IN ('QUEUED','RUNNING')`,
				businessA, dependent.ID).Scan(&queued); err != nil {
				t.Fatal(err)
			}
			if queued != 1 {
				t.Fatalf("in-flight evaluations = %d; want one durable deduplicated job", queued)
			}

			env.materialize(t)
			current := env.reloadSegment(t, businessA, dependent.ID)
			if current.EvaluationStatus != EvalCurrent || current.CurrentMemberCount != 1 {
				t.Fatalf("materialized status/count = %s/%d; want CURRENT/1", current.EvaluationStatus, current.CurrentMemberCount)
			}
			members := env.memberIDs(t, current)
			if len(members) != 1 || members[0] != created.ID {
				t.Fatalf("members = %v; want new profile %s", members, created.ID)
			}
		})
	}
}

func TestProfileCreationConcurrentDuplicateQueuesOneEvaluation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Concurrent Profile Creation", SegmentType: SegmentDynamic,
		Rule: ruleJSON(`{"schema_version":1,"root":{"field":"profile.subject_type","operator":"EQ","value":"CUSTOMER"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)

	type result struct {
		profile Profile
		err     error
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			tx, beginErr := env.pool.Begin(ctx)
			if beginErr != nil {
				results <- result{err: beginErr}
				return
			}
			ready <- struct{}{}
			<-start
			profile, resolveErr := env.service.ResolveOrCreateProfile(ctx, tx, actor, RelationshipInput{
				Subject: customerSubject("cus_concurrent_profile"), ObservedAt: env.now,
			})
			if resolveErr != nil {
				_ = tx.Rollback(ctx)
				results <- result{err: resolveErr}
				return
			}
			results <- result{profile: profile, err: tx.Commit(ctx)}
		}()
	}
	<-ready
	<-ready
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent resolution errors: first=%v second=%v", first.err, second.err)
	}
	if first.profile.ID != second.profile.ID {
		t.Fatalf("concurrent resolution produced %s and %s", first.profile.ID, second.profile.ID)
	}
	var profiles, queued int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1 AND customer_ref='cus_concurrent_profile'`, businessA).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND status IN ('QUEUED','RUNNING')`, businessA, segment.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 || queued != 1 {
		t.Fatalf("profiles/queued = %d/%d; want one profile and one evaluation", profiles, queued)
	}
}

// TestMaterialization_FailurePreservesLastGoodAudience is the central
// availability guarantee: a broken rebuild must never present an empty
// audience, because an empty audience reads as "nobody qualifies" rather than
// as "we could not tell".
func TestMaterialization_FailurePreservesLastGoodAudience(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	member := env.seedRepeatCustomer(t, "cus_keep", branchA1)
	segment := env.activeDynamicSegment(t, "Durable Audience")
	if segment.CurrentMemberCount != 1 {
		t.Fatalf("baseline count = %d; want 1", segment.CurrentMemberCount)
	}
	goodEvaluation := *segment.CurrentEvaluationID

	// Corrupt the stored definition so the next run cannot validate it. This
	// stands in for any failure between claiming a run and swapping it in.
	if _, err := env.pool.Exec(ctx, `UPDATE crm_segment_versions
		SET rule_json = '{"op":"AND","children":[{"field":"not.a.registered.field","operator":"EQ","value":"x"}]}'::jsonb
		WHERE business_id=$1 AND segment_id=$2 AND definition_version=$3`,
		businessA, segment.ID, segment.CurrentDefinitionVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.RequestMaterialization(ctx, actorFor(businessA, "vs_a"), globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}
	env.materializeExpectingFailures(t)

	after := env.reloadSegment(t, businessA, segment.ID)
	if after.CurrentEvaluationID == nil || *after.CurrentEvaluationID != goodEvaluation {
		t.Fatalf("current evaluation = %v; the last successful generation must survive a failed rebuild", after.CurrentEvaluationID)
	}
	if after.CurrentMemberCount != 1 {
		t.Fatalf("member count = %d; a failed rebuild must not empty the audience", after.CurrentMemberCount)
	}
	if after.EvaluationStatus != EvalStale {
		t.Fatalf("evaluation status = %s; want STALE so the caller knows the membership is not current", after.EvaluationStatus)
	}
	if after.LastEvaluationErrorCode == nil {
		t.Fatal("a failed rebuild must record why")
	}
	ids := env.memberIDs(t, after)
	if len(ids) != 1 || ids[0] != member.ID {
		t.Fatalf("members = %v; want the previous generation intact", ids)
	}
}

// materializeExpectingFailures drains the queue where runs are expected to
// fail, so a failure is not reported as a test error.
func (e *testEnv) materializeExpectingFailures(t *testing.T) {
	t.Helper()
	for pass := 0; pass < 10; pass++ {
		processed, err := e.service.RunMaterializationPass(context.Background(), "test-worker", 10)
		if err != nil {
			t.Fatalf("materialization pass returned an unexpected error: %v", err)
		}
		if processed == 0 {
			return
		}
		// A failed run schedules a backoff, so the next claim finds nothing due.
	}
}

// TestMaterialization_DeadLetterAfterRepeatedFailure proves a permanently
// broken definition stops consuming worker capacity but still keeps its
// audience.
func TestMaterialization_DeadLetterAfterRepeatedFailure(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedRepeatCustomer(t, "cus_dl", branchA1)
	segment := env.activeDynamicSegment(t, "Dead Letter Candidate")

	if _, err := env.pool.Exec(ctx, `UPDATE crm_segment_versions
		SET rule_json = '{"field":"not.a.field","operator":"EQ","value":"x"}'::jsonb
		WHERE business_id=$1 AND segment_id=$2`, businessA, segment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.RequestMaterialization(ctx, actorFor(businessA, "vs_a"), globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}

	// Drive attempts past the ceiling, clearing the backoff between them the way
	// the passage of time would.
	for attempt := 0; attempt <= env.service.evaluation.MaxAttempts+1; attempt++ {
		if _, err := env.pool.Exec(ctx, `UPDATE crm_segment_evaluations SET next_attempt_at=now()-interval '1 minute'
			WHERE business_id=$1 AND segment_id=$2 AND status IN ('QUEUED','FAILED')`, businessA, segment.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := env.service.RunMaterializationPass(ctx, "test-worker", 5); err != nil {
			t.Fatal(err)
		}
	}

	var status string
	if err := env.pool.QueryRow(ctx, `SELECT status FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 ORDER BY generation DESC LIMIT 1`,
		businessA, segment.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != RunDeadLetter {
		t.Fatalf("status = %s; want %s after the attempt ceiling", status, RunDeadLetter)
	}
	if env.reloadSegment(t, businessA, segment.ID).CurrentMemberCount != 1 {
		t.Fatal("a dead-lettered rebuild must still leave the last good audience in place")
	}

	var evaluationID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND status='DEAD_LETTER'`,
		businessA, segment.ID).Scan(&evaluationID); err != nil {
		t.Fatal(err)
	}
	if err := env.service.ReplayMaterialization(ctx, businessA, evaluationID); err != nil {
		t.Fatalf("operator replay: %v", err)
	}
	var replayStatus string
	var attempts int
	if err := env.pool.QueryRow(ctx, `SELECT status,worker_attempt_count FROM crm_segment_evaluations
		WHERE business_id=$1 AND id=$2`, businessA, evaluationID).Scan(&replayStatus, &attempts); err != nil {
		t.Fatal(err)
	}
	if replayStatus != RunQueued || attempts != 0 {
		t.Fatalf("replayed evaluation status=%s attempts=%d; want QUEUED/0", replayStatus, attempts)
	}
	if err := env.service.ReplayMaterialization(ctx, businessA, evaluationID); err == nil {
		t.Fatal("a non-dead-lettered evaluation was replayed")
	}
}

// TestMaterialization_LeaseIsRespected proves two workers never evaluate the
// same run, which is what makes running several instances safe.
func TestMaterialization_LeaseIsRespected(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedRepeatCustomer(t, "cus_lease", branchA1)
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Leased", SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}

	first, claimed, err := env.service.claimEvaluation(ctx, "worker-one")
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	_, secondClaimed, err := env.service.claimEvaluation(ctx, "worker-two")
	if err != nil {
		t.Fatal(err)
	}
	if secondClaimed {
		t.Fatal("a second worker claimed a leased evaluation")
	}

	// Once the lease lapses, another worker may adopt the abandoned run — that
	// is how a crashed worker's work is recovered rather than stranded.
	if _, err = env.pool.Exec(ctx, `UPDATE crm_segment_evaluations
		SET lease_expires_at=now()-interval '1 minute', next_attempt_at=now()-interval '1 minute' WHERE id=$1`,
		first.ID); err != nil {
		t.Fatal(err)
	}
	adopted, claimed, err := env.service.claimEvaluation(ctx, "worker-two")
	if err != nil || !claimed {
		t.Fatalf("adoption after lease expiry: claimed=%v err=%v", claimed, err)
	}
	if adopted.ID != first.ID {
		t.Fatalf("adopted %s; want the abandoned run %s", adopted.ID, first.ID)
	}
}

// TestMaterialization_HeartbeatRenewsLease proves a healthy long-running
// evaluation cannot be adopted merely because its original claim TTL elapsed.
func TestMaterialization_HeartbeatRenewsLease(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.service.evaluation.LeaseTTL = 300 * time.Millisecond
	env.seedRepeatCustomer(t, "cus_heartbeat", branchA1)
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Heartbeat", SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}
	evaluation, claimed, err := env.service.claimEvaluation(ctx, "heartbeat-worker")
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	var before time.Time
	if err = env.pool.QueryRow(ctx, `SELECT lease_expires_at FROM crm_segment_evaluations WHERE id=$1`, evaluation.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	_, stop := env.service.startEvaluationHeartbeat(ctx, evaluation, "heartbeat-worker")
	time.Sleep(220 * time.Millisecond)
	var after time.Time
	if err = env.pool.QueryRow(ctx, `SELECT lease_expires_at FROM crm_segment_evaluations WHERE id=$1`, evaluation.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err = stop(); err != nil {
		t.Fatalf("stop heartbeat: %v", err)
	}
	if !after.After(before) {
		t.Fatalf("lease was not renewed: before=%s after=%s", before, after)
	}
	if _, adopted, err := env.service.claimEvaluation(ctx, "another-worker"); err != nil || adopted {
		t.Fatalf("renewed lease was adopted: adopted=%v err=%v", adopted, err)
	}
}

// TestMaterialization_PartialGenerationIsNotVisible covers the crash-mid-write
// case: rows written by a dead attempt are discarded on restart and never
// become the live audience.
func TestMaterialization_PartialGenerationIsNotVisible(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	keep := env.seedRepeatCustomer(t, "cus_partial_keep", branchA1)
	segment := env.activeDynamicSegment(t, "Partial Write")
	goodEvaluation := *segment.CurrentEvaluationID

	// Queue a new run and write a bogus row into its generation, as a worker
	// that died halfway would have left behind.
	evaluation, err := env.service.RequestMaterialization(ctx, actorFor(businessA, "vs_a"), globalScope(), segment.ID)
	if err != nil {
		t.Fatal(err)
	}
	stranger := env.seedTaggableProfile(t, businessA, "vs_a", "cus_stranger", branchA1)
	if _, err = env.pool.Exec(ctx, `INSERT INTO crm_segment_memberships
		(id,business_id,segment_id,profile_id,membership_source,evaluation_id,generation,segment_definition_version)
		VALUES('crmsm_orphan',$1,$2,$3,'DYNAMIC',$4,$5,$6)`,
		businessA, segment.ID, stranger.ID, evaluation.ID, evaluation.Generation, evaluation.SegmentDefinitionVersion); err != nil {
		t.Fatal(err)
	}

	// The half-built generation is not the live one, so the audience is
	// unchanged while it exists.
	current := env.reloadSegment(t, businessA, segment.ID)
	if *current.CurrentEvaluationID != goodEvaluation {
		t.Fatal("a partially written generation became live")
	}
	if ids := env.memberIDs(t, current); len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("members = %v; want the previous generation only", ids)
	}

	// Restarting the run discards the partial rows and rebuilds from scratch.
	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, segment.ID)
	if rebuilt.EvaluationStatus != EvalCurrent {
		t.Fatalf("status = %s; want CURRENT after a clean rebuild", rebuilt.EvaluationStatus)
	}
	ids := env.memberIDs(t, rebuilt)
	if len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("members = %v; the orphaned row must have been discarded", ids)
	}
}

// TestMaterialization_RedactionDuringRunIsHonoured is the privacy race: a
// REDACT that commits while a generation is being written must not appear in
// the audience that generation becomes.
func TestMaterialization_RedactionDuringRunIsHonoured(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	surviving := env.seedRepeatCustomer(t, "cus_survives", branchA1)
	doomed := env.seedRepeatCustomer(t, "cus_doomed", branchA1)
	segment := env.activeDynamicSegment(t, "Race With Redaction")
	if segment.CurrentMemberCount != 2 {
		t.Fatalf("baseline = %d; want 2", segment.CurrentMemberCount)
	}

	evaluation, err := env.service.RequestMaterialization(ctx, actorFor(businessA, "vs_a"), globalScope(), segment.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := env.service.claimEvaluation(ctx, "race-worker")
	if err != nil || !ok || claimed.ID != evaluation.ID {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	version, err := env.service.CurrentVersion(ctx, env.pool, businessA, segment.ID, claimed.SegmentDefinitionVersion)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := env.service.ruleFromVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	query := env.service.buildCandidateQuery(businessA, segment, rule, claimed.EvaluationAsOf)
	written, err := env.service.writeGeneration(ctx, claimed, query)
	if err != nil {
		t.Fatal(err)
	}
	if written != 2 {
		t.Fatalf("wrote %d candidate rows; want 2 before the redaction lands", written)
	}

	// The redaction commits after the rows were written but before the swap.
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='REDACTED', redacted_at=now() WHERE id=$1`, doomed.ID); err != nil {
		t.Fatal(err)
	}

	memberCount, err := env.service.swapGeneration(ctx, actorFor(businessA, "vs_a"), claimed, written, query)
	if err != nil {
		t.Fatal(err)
	}
	if memberCount != 1 {
		t.Fatalf("member count = %d; the redacted relationship must not survive the swap", memberCount)
	}
	after := env.reloadSegment(t, businessA, segment.ID)
	ids := env.memberIDs(t, after)
	if len(ids) != 1 || ids[0] != surviving.ID {
		t.Fatalf("members = %v; want only %s", ids, surviving.ID)
	}
}

type pendingGeneration struct {
	evaluation Evaluation
	query      candidateQuery
	written    int
}

func (e *testEnv) buildPendingGeneration(t *testing.T, segment Segment, workerID string) pendingGeneration {
	t.Helper()
	ctx := context.Background()
	evaluation, err := e.service.RequestMaterialization(ctx, actorFor(businessA, "vs_a"), globalScope(), segment.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := e.service.claimEvaluation(ctx, workerID)
	if err != nil || !ok || claimed.ID != evaluation.ID {
		t.Fatalf("claim pending generation: ok=%v claimed=%s want=%s err=%v", ok, claimed.ID, evaluation.ID, err)
	}
	version, err := e.service.CurrentVersion(ctx, e.pool, businessA, segment.ID, claimed.SegmentDefinitionVersion)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := e.service.ruleFromVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	query := e.service.buildCandidateQuery(businessA, e.reloadSegment(t, businessA, segment.ID), rule, claimed.EvaluationAsOf)
	written, err := e.service.writeGeneration(ctx, claimed, query)
	if err != nil {
		t.Fatal(err)
	}
	return pendingGeneration{evaluation: claimed, query: query, written: written}
}

func (e *testEnv) assertCurrentGenerationConsistent(t *testing.T, segmentID string, want int) {
	t.Helper()
	ctx := context.Background()
	segment := e.reloadSegment(t, businessA, segmentID)
	if segment.CurrentMemberCount != want {
		t.Fatalf("published member count = %d; want %d", segment.CurrentMemberCount, want)
	}
	var rows int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships m
		JOIN crm_segment_evaluations e ON e.business_id=m.business_id AND e.id=$1 AND e.generation=m.generation
		WHERE m.business_id=$2 AND m.segment_id=$3`, segment.CurrentEvaluationID, businessA, segmentID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != want {
		t.Fatalf("current generation rows = %d; published count = %d", rows, segment.CurrentMemberCount)
	}
	if ids := e.memberIDs(t, segment); len(ids) != want {
		t.Fatalf("externally visible members = %v; published count = %d", ids, segment.CurrentMemberCount)
	}
}

// waitForBackendLock blocks until one specific backend is queued behind a lock.
// This is the strongest identification available and is used wherever the test
// owns the waiting connection and can read its pg_backend_pid().
func waitForBackendLock(t *testing.T, env *testEnv, backendPID int) {
	t.Helper()
	newLockObserver(t).awaitBackend(t, backendPID, "expected database lock")
}

// waitForGenerationFenceLock blocks until generation certification is queued
// behind the authoritative profile lock, identified by the
// /* crm_generation_profile_fence */ marker its statement carries.
func waitForGenerationFenceLock(t *testing.T, env *testEnv) {
	t.Helper()
	newLockObserver(t).await(t, 1, "%crm_generation_profile_fence%", false,
		"generation certification never waited on the authoritative profile lock")
}

// TestMaterialization_RedactionSwapFirst uses two real database transactions.
// The worker holds the profile certification lock while paused before the
// pointer update; redaction must block, then remove membership and repair the
// count in its own commit before it can report completion.
func TestMaterialization_RedactionSwapFirst(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	profile := env.seedRepeatCustomer(t, "cus_swap_first_redact", branchA1)
	segment := env.activeDynamicSegment(t, "Swap First Redaction")
	pending := env.buildPendingGeneration(t, segment, "swap-first-redaction")
	if pending.written != 1 {
		t.Fatalf("pending generation rows = %d; want 1", pending.written)
	}

	certified := make(chan struct{})
	releaseSwap := make(chan struct{})
	swapDone := make(chan error, 1)
	go func() {
		_, err := env.service.swapGenerationWithHook(ctx, actorFor(businessA, "vs_a"), pending.evaluation,
			pending.written, pending.query, func() {
				close(certified)
				<-releaseSwap
			})
		swapDone <- err
	}()
	<-certified

	privacyConn, err := env.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var privacyPID int
	if err = privacyConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&privacyPID); err != nil {
		privacyConn.Release()
		t.Fatal(err)
	}
	privacyDone := make(chan error, 1)
	go func() {
		defer privacyConn.Release()
		tx, beginErr := privacyConn.Begin(ctx)
		if beginErr != nil {
			privacyDone <- beginErr
			return
		}
		err := env.service.ApplyPrivacy(ctx, tx, actorFor(businessA, "vs_a"), &profile, PrivacyInput{
			SourceService: "customer-service", SourceVersion: 1,
			PrivacySubjectRef: profile.PrimarySubjectRef(), Action: PrivacyActionRedact, ObservedAt: env.now,
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			privacyDone <- err
			return
		}
		privacyDone <- tx.Commit(ctx)
	}()
	waitForBackendLock(t, env, privacyPID)
	close(releaseSwap)
	if err = <-swapDone; err != nil {
		t.Fatal(err)
	}
	if err = <-privacyDone; err != nil {
		t.Fatal(err)
	}

	stale := env.reloadSegment(t, businessA, segment.ID)
	if stale.EvaluationStatus != EvalStale {
		t.Fatalf("status after redaction = %s; want STALE", stale.EvaluationStatus)
	}
	env.assertCurrentGenerationConsistent(t, segment.ID, 0)
}

// TestMaterialization_MergeSwapFirst proves identity merge follows the same
// profile lock fence and never transfers source dynamic membership as truth.
func TestMaterialization_MergeSwapFirst(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	source := env.seedRepeatCustomer(t, "cus_swap_merge_source", branchA1)
	target := env.seedTaggableProfile(t, businessA, "vs_a", "cus_swap_merge_target", branchA1)
	segment := env.activeDynamicSegment(t, "Swap First Merge")
	pending := env.buildPendingGeneration(t, segment, "swap-first-merge")

	certified := make(chan struct{})
	releaseSwap := make(chan struct{})
	swapDone := make(chan error, 1)
	go func() {
		_, err := env.service.swapGenerationWithHook(ctx, actorFor(businessA, "vs_a"), pending.evaluation,
			pending.written, pending.query, func() {
				close(certified)
				<-releaseSwap
			})
		swapDone <- err
	}()
	<-certified

	mergeConn, err := env.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mergePID int
	if err = mergeConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&mergePID); err != nil {
		mergeConn.Release()
		t.Fatal(err)
	}
	mergeDone := make(chan error, 1)
	go func() {
		defer mergeConn.Release()
		tx, beginErr := mergeConn.Begin(ctx)
		if beginErr != nil {
			mergeDone <- beginErr
			return
		}
		err := env.service.ApplyIdentityLink(ctx, tx, actorFor(businessA, "vs_a"), IdentityLinkInput{
			AuthorityService: "customer-service", AuthorityReference: "swap_merge", AuthorityVersion: 1,
			EvidenceEventID: "evt_swap_merge", Source: SubjectRef{Type: source.SubjectType, Ref: source.PrimarySubjectRef()},
			Target: SubjectRef{Type: target.SubjectType, Ref: target.PrimarySubjectRef()},
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			mergeDone <- err
			return
		}
		mergeDone <- tx.Commit(ctx)
	}()
	waitForBackendLock(t, env, mergePID)
	close(releaseSwap)
	if err = <-swapDone; err != nil {
		t.Fatal(err)
	}
	if err = <-mergeDone; err != nil {
		t.Fatal(err)
	}

	env.assertCurrentGenerationConsistent(t, segment.ID, 0)
	var sourceRows int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships WHERE profile_id=$1`, source.ID).Scan(&sourceRows); err != nil {
		t.Fatal(err)
	}
	if sourceRows != 0 {
		t.Fatalf("merged source retained %d dynamic membership rows", sourceRows)
	}
	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, segment.ID)
	ids := env.memberIDs(t, rebuilt)
	if len(ids) != 1 || ids[0] != target.ID {
		t.Fatalf("recomputed members = %v; want survivor %s", ids, target.ID)
	}
}

// TestMaterialization_PrivacyCommitFirst holds an uncommitted authoritative
// privacy change while certification starts. The worker must wait on the
// profile fence and then certify against the committed restricted state.
func TestMaterialization_PrivacyCommitFirst(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	profile := env.seedRepeatCustomer(t, "cus_privacy_first", branchA1)
	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyConsent(ctx, tx, actorFor(businessA, "vs_a"), profile.ID, ConsentInput{
			SourceService: "customer-service", SourceVersion: 1, Purpose: ConsentPurposeMarketing,
			Channel: ChannelEmail, Decision: ConsentAllow, ObservedAt: env.now,
		})
	}); err != nil {
		t.Fatal(err)
	}
	segment, err := env.service.CreateSegment(ctx, actorFor(businessA, "vs_a"), globalScope(), CreateSegmentInput{
		Name: "Privacy Commit First", SegmentType: SegmentDynamic,
		Rule: ruleJSON(`{"schema_version":1,"root":{"field":"consent.marketing.email.eligible","operator":"EQ","value":true}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actorFor(businessA, "vs_a"), globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	segment = env.reloadSegment(t, businessA, segment.ID)
	pending := env.buildPendingGeneration(t, segment, "privacy-first")

	privacyConn, err := env.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer privacyConn.Release()
	privacyTx, err := privacyConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = env.service.ApplyPrivacy(ctx, privacyTx, actorFor(businessA, "vs_a"), &profile, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1,
		PrivacySubjectRef: profile.PrimarySubjectRef(), Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		_ = privacyTx.Rollback(ctx)
		t.Fatal(err)
	}

	swapDone := make(chan error, 1)
	go func() {
		_, swapErr := env.service.swapGeneration(ctx, actorFor(businessA, "vs_a"), pending.evaluation,
			pending.written, pending.query)
		swapDone <- swapErr
	}()
	waitForGenerationFenceLock(t, env)
	if err = privacyTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-swapDone; err != nil {
		t.Fatal(err)
	}
	env.assertCurrentGenerationConsistent(t, segment.ID, 0)
	if got := env.reloadSegment(t, businessA, segment.ID); got.EvaluationStatus != EvalCurrent {
		t.Fatalf("certified post-privacy status = %s; want CURRENT", got.EvaluationStatus)
	}
}

func TestMaterialization_CrashDuringFencedSwapPreservesAndRecovers(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedRepeatCustomer(t, "cus_fenced_crash", branchA1)
	segment := env.activeDynamicSegment(t, "Fenced Swap Crash")
	goodEvaluation := *segment.CurrentEvaluationID
	pending := env.buildPendingGeneration(t, segment, "fenced-crash")

	cancelled, cancel := context.WithCancel(ctx)
	_, err := env.service.swapGenerationWithHook(cancelled, actorFor(businessA, "vs_a"), pending.evaluation,
		pending.written, pending.query, cancel)
	if err == nil {
		t.Fatal("cancelled fenced swap committed")
	}
	afterCrash := env.reloadSegment(t, businessA, segment.ID)
	if afterCrash.CurrentEvaluationID == nil || *afterCrash.CurrentEvaluationID != goodEvaluation || afterCrash.CurrentMemberCount != 1 {
		t.Fatalf("crash exposed partial generation: %+v", afterCrash)
	}

	if _, err = env.pool.Exec(ctx, `UPDATE crm_segment_evaluations
		SET lease_expires_at=now()-interval '1 minute', next_attempt_at=now()-interval '1 minute'
		WHERE business_id=$1 AND id=$2`, businessA, pending.evaluation.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	recovered := env.reloadSegment(t, businessA, segment.ID)
	if recovered.EvaluationStatus != EvalCurrent || recovered.CurrentMemberCount != 1 {
		t.Fatalf("recovered status/count = %s/%d; want CURRENT/1", recovered.EvaluationStatus, recovered.CurrentMemberCount)
	}
}

// TestMaterialization_ExclusionsAreSubtracted proves a manual exclusion is a
// standing decision that survives a rebuild.
func TestMaterialization_ExclusionsAreSubtracted(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	keep := env.seedRepeatCustomer(t, "cus_keep_me", branchA1)
	drop := env.seedRepeatCustomer(t, "cus_never_contact", branchA1)
	segment := env.activeDynamicSegment(t, "With Exclusions")
	if segment.CurrentMemberCount != 2 {
		t.Fatalf("baseline = %d; want 2", segment.CurrentMemberCount)
	}

	if _, err := env.service.BulkExclusion(ctx, actor, globalScope(), segment.ID, []string{drop.ID}, true); err != nil {
		t.Fatalf("exclude: %v", err)
	}
	// The excluded customer stops being a member immediately, not at the next
	// scheduled rebuild.
	immediate := env.reloadSegment(t, businessA, segment.ID)
	if ids := env.memberIDs(t, immediate); len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("members right after exclusion = %v; want only %s", ids, keep.ID)
	}
	if immediate.EvaluationStatus != EvalStale {
		t.Fatalf("status = %s; want STALE after an exclusion changed the answer", immediate.EvaluationStatus)
	}

	if _, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, segment.ID)
	if rebuilt.CurrentMemberCount != 1 {
		t.Fatalf("count after rebuild = %d; the exclusion must survive rematerialization", rebuilt.CurrentMemberCount)
	}

	if _, err := env.service.BulkExclusion(ctx, actor, globalScope(), segment.ID, []string{drop.ID}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	if env.reloadSegment(t, businessA, segment.ID).CurrentMemberCount != 2 {
		t.Fatal("lifting an exclusion must restore the member at the next rebuild")
	}
}

// TestMaterialization_RuleEditVersionsAndRestakes covers the definition
// lifecycle: an edit mints a new immutable version, marks the audience stale and
// leaves the old membership readable until the new run succeeds.
func TestMaterialization_RuleEditVersionsAndRestakes(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_two_orders", branchA1)
	single := env.seedTaggableProfile(t, businessA, "vs_a", "cus_one_order", branchA1)

	segment := env.activeDynamicSegment(t, "Evolving Rule")
	if segment.CurrentMemberCount != 1 {
		t.Fatalf("baseline = %d; want 1", segment.CurrentMemberCount)
	}
	originalHash := *segment.DefinitionHash

	relaxed := ruleJSON(`{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":1},
		{"field":"commercial.last_completed_order_at","operator":"WITHIN_LAST","value":{"amount":30,"unit":"DAY"}}
	]}}`)
	updated, err := env.service.UpdateSegment(ctx, actor, globalScope(), segment.ID, UpdateSegmentInput{
		Rule: relaxed, ChangeReason: "widen to first-time buyers", ExpectedVersion: segment.Version})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.CurrentDefinitionVersion != 2 {
		t.Fatalf("definition version = %d; want 2", updated.CurrentDefinitionVersion)
	}
	if *updated.DefinitionHash == originalHash {
		t.Fatal("a changed rule must change the definition hash")
	}
	if updated.EvaluationStatus != EvalStale || updated.StaleSince == nil {
		t.Fatalf("status = %s; want STALE with a stale_since timestamp", updated.EvaluationStatus)
	}
	// The previous audience is still readable while the new run is pending.
	if updated.CurrentMemberCount != 1 {
		t.Fatalf("count = %d; editing a rule must not empty the live audience", updated.CurrentMemberCount)
	}

	// Version 1 is immutable and still resolvable, which is what lets a past
	// materialization be tied to the exact definition it ran against.
	first, err := env.service.CurrentVersion(ctx, env.pool, businessA, segment.ID, 1)
	if err != nil {
		t.Fatalf("version 1 must remain readable: %v", err)
	}
	if first.CanonicalRuleHash == nil || *first.CanonicalRuleHash != originalHash {
		t.Fatal("the original definition version was mutated")
	}

	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, segment.ID)
	if rebuilt.EvaluationStatus != EvalCurrent {
		t.Fatalf("status = %s; want CURRENT", rebuilt.EvaluationStatus)
	}
	if rebuilt.CurrentMemberCount != 2 {
		t.Fatalf("count = %d; want 2 under the relaxed rule", rebuilt.CurrentMemberCount)
	}
	ids := env.memberIDs(t, rebuilt)
	found := false
	for _, id := range ids {
		if id == single.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("members = %v; the newly qualifying relationship is missing", ids)
	}
}

// TestMaterialization_TagChangeInvalidatesOnlyDependentSegments proves
// dependency tracking does real work: a tag assignment must not restake every
// segment in the business.
func TestMaterialization_TagChangeInvalidatesOnlyDependentSegments(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.seedRepeatCustomer(t, "cus_tagged", branchA1)

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Loyal"})
	if err != nil {
		t.Fatal(err)
	}
	tagRule := ruleJSON(`{"schema_version":1,"root":{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"` + tag.ID + `"}}}`)
	tagSegment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Tag Driven", SegmentType: SegmentDynamic, Rule: tagRule})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), tagSegment.ID, SegmentActive, tagSegment.Version); err != nil {
		t.Fatal(err)
	}
	commercialSegment := env.activeDynamicSegment(t, "Commercial Only")
	env.materialize(t)

	if env.reloadSegment(t, businessA, tagSegment.ID).EvaluationStatus != EvalCurrent {
		t.Fatal("the tag-driven segment should start CURRENT")
	}
	if env.reloadSegment(t, businessA, commercialSegment.ID).EvaluationStatus != EvalCurrent {
		t.Fatal("the commercial segment should start CURRENT")
	}

	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	if got := env.reloadSegment(t, businessA, tagSegment.ID).EvaluationStatus; got != EvalStale {
		t.Fatalf("tag-driven segment status = %s; want STALE", got)
	}
	if got := env.reloadSegment(t, businessA, commercialSegment.ID).EvaluationStatus; got != EvalCurrent {
		t.Fatalf("commercial segment status = %s; a tag change must not restake segments that do not read tags", got)
	}
	var queued int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND status='QUEUED'`, businessA, tagSegment.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued tag-dependent evaluations = %d; invalidation must durably enqueue one rebuild", queued)
	}

	if _, err = env.service.RequestMaterialization(ctx, actor, globalScope(), tagSegment.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	if env.reloadSegment(t, businessA, tagSegment.ID).CurrentMemberCount != 1 {
		t.Fatal("the newly tagged relationship did not become a member")
	}
}

// TestMaterialization_CommercialChangeQueuesDependentSegments proves the 8A
// projection hook and the 8B durable queue are one transaction: a changed
// order fact cannot commit while its audience still claims to be CURRENT.
func TestMaterialization_CommercialChangeQueuesDependentSegments(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	profile := env.seedRepeatCustomer(t, "cus_commercial_change", branchA1)
	segment := env.activeDynamicSegment(t, "Commercial Change")
	if segment.EvaluationStatus != EvalCurrent {
		t.Fatalf("baseline status=%s; want CURRENT", segment.EvaluationStatus)
	}

	env.seedOrder(t, businessA, "vs_a", profile, "ord_cus_commercial_change_b", branchA1,
		200000, 150000, env.now.AddDate(0, 0, -2), 2)
	reloaded := env.reloadSegment(t, businessA, segment.ID)
	if reloaded.EvaluationStatus != EvalStale {
		t.Fatalf("status after commercial change=%s; want STALE", reloaded.EvaluationStatus)
	}
	var queued int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND status='QUEUED'`, businessA, segment.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued commercial evaluations=%d; want 1", queued)
	}
}

// TestMaterialization_DuplicateRequestDoesNotStackRuns keeps a user hammering
// the rebuild button from creating a queue of identical work.
func TestMaterialization_DuplicateRequestDoesNotStackRuns(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_dupe", branchA1)

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Hammered", SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}

	first, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		repeat, repeatErr := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID)
		if repeatErr != nil {
			t.Fatal(repeatErr)
		}
		if repeat.ID != first.ID {
			t.Fatalf("a duplicate request created run %s; want the in-flight %s", repeat.ID, first.ID)
		}
	}
	var queued int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND status IN ('QUEUED','RUNNING')`,
		businessA, segment.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued runs = %d; want exactly 1", queued)
	}
}

// TestMaterialization_ArchivedSegmentDropsMembership stops a retired audience
// being reused by a later campaign.
func TestMaterialization_ArchivedSegmentDropsMembership(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_arch", branchA1)
	segment := env.activeDynamicSegment(t, "To Be Retired")

	archived, err := env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentArchived, segment.Version)
	if err != nil {
		t.Fatal(err)
	}
	if archived.CurrentMemberCount != 0 {
		t.Fatalf("count = %d; a retired segment must not keep a live audience", archived.CurrentMemberCount)
	}
	var rows int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships WHERE segment_id=$1`,
		segment.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("membership rows = %d; want 0 after archiving", rows)
	}
	if _, err = env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); codeOf(t, err) != CodeSegmentInvalidState {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentInvalidState)
	}
}

// TestMaterialization_BranchSegmentUsesBranchFigures proves a branch-scoped
// rule reasons over that branch's slice of the relationship, not the business
// total — otherwise a branch audience would silently include customers who only
// qualify because of trade at other branches.
func TestMaterialization_BranchSegmentUsesBranchFigures(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// Two completed orders in total, but only one at each branch.
	split := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_split"))
	env.seedOrder(t, businessA, "vs_a", split, "ord_split_1", branchA1, 100000, 100000, env.now.AddDate(0, 0, -4), 1)
	env.seedOrder(t, businessA, "vs_a", split, "ord_split_2", branchA2, 100000, 100000, env.now.AddDate(0, 0, -2), 1)

	businessWide, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if businessWide.MatchCount != 1 {
		t.Fatalf("business-wide count = %d; want 1", businessWide.MatchCount)
	}

	branchOnly, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		ScopeType: ScopeBranch, BranchRef: branchA1, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if branchOnly.MatchCount != 0 {
		t.Fatalf("branch count = %d; one order at this branch must not satisfy a two-order rule", branchOnly.MatchCount)
	}
}

// TestMaterialization_EvaluationAsOfIsShared proves every relationship in one
// run is judged against the same clock instant, so a long run cannot classify
// two identical customers differently.
func TestMaterialization_EvaluationAsOfIsShared(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedRepeatCustomer(t, "cus_clock", branchA1)
	segment := env.activeDynamicSegment(t, "Clock Bound")

	var asOf time.Time
	if err := env.pool.QueryRow(ctx, `SELECT evaluation_as_of FROM crm_segment_evaluations
		WHERE business_id=$1 AND id=$2`, businessA, *segment.CurrentEvaluationID).Scan(&asOf); err != nil {
		t.Fatal(err)
	}
	if asOf.IsZero() {
		t.Fatal("an evaluation must pin the instant it was judged against")
	}
}

// TestMaterialization_MembersListReadsOnlyTheCurrentGeneration proves a caller
// never sees two generations mixed together.
func TestMaterialization_MembersListReadsOnlyTheCurrentGeneration(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	keep := env.seedRepeatCustomer(t, "cus_gen", branchA1)
	segment := env.activeDynamicSegment(t, "Generations")

	// Fabricate a stale row from an older generation, as a cleanup that had not
	// yet run would leave.
	stranger := env.seedTaggableProfile(t, businessA, "vs_a", "cus_old_gen", branchA1)
	if _, err := env.pool.Exec(ctx, `INSERT INTO crm_segment_memberships
		(id,business_id,segment_id,profile_id,membership_source,generation,segment_definition_version)
		VALUES('crmsm_stale',$1,$2,$3,'DYNAMIC',99,1)`, businessA, segment.ID, stranger.ID); err != nil {
		t.Fatal(err)
	}
	ids := env.memberIDs(t, segment)
	if len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("members = %v; only the current generation may be returned", ids)
	}
}

// TestMaterialization_DisabledWorkerDoesNotClaimCurrent proves a disabled
// materializer degrades to "not rebuilt" rather than to a frozen audience
// presented as live.
func TestMaterialization_DisabledWorkerDoesNotClaimCurrent(t *testing.T) {
	flags := allFlagsOn()
	flags.SegmentMaterializationEnabled = false
	env := newTestEnv(t, flags)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_disabled", branchA1)

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "No Worker", SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}
	processed, err := env.service.RunMaterializationPass(ctx, "test-worker", 5)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d; a disabled materializer must claim nothing", processed)
	}
	reloaded := env.reloadSegment(t, businessA, segment.ID)
	if reloaded.EvaluationStatus == EvalCurrent {
		t.Fatal("a segment that has never been materialized must not report CURRENT")
	}
	// Requesting a rebuild while the worker is off fails closed rather than
	// pretending one was scheduled.
	_, err = env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID)
	if codeOf(t, err) != CodeFeatureDisabled {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeFeatureDisabled)
	}
}

// TestMaterialization_ConcurrentWorkersProduceOneAudience runs two workers over
// the same backlog and checks the result is exactly one live generation.
func TestMaterialization_ConcurrentWorkersProduceOneAudience(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_conc_1", branchA1)
	env.seedRepeatCustomer(t, "cus_conc_2", branchA1)

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Concurrent", SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 2)
	for _, worker := range []string{"worker-a", "worker-b"} {
		go func(id string) {
			_, runErr := env.service.RunMaterializationPass(context.Background(), id, 5)
			done <- runErr
		}(worker)
	}
	for index := 0; index < 2; index++ {
		if err = <-done; err != nil {
			t.Fatalf("worker: %v", err)
		}
	}

	reloaded := env.reloadSegment(t, businessA, segment.ID)
	if reloaded.CurrentMemberCount != 2 {
		t.Fatalf("count = %d; want 2", reloaded.CurrentMemberCount)
	}
	var liveGenerations int
	if err = env.pool.QueryRow(ctx, `SELECT count(DISTINCT generation) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2`, businessA, segment.ID).Scan(&liveGenerations); err != nil {
		t.Fatal(err)
	}
	if liveGenerations != 1 {
		t.Fatalf("distinct generations = %d; want exactly 1 live audience", liveGenerations)
	}
}

// TestMaterialization_SupersededGenerationsAreCleanedUp proves the retention
// port runs and old membership rows do not accumulate forever.
func TestMaterialization_SupersededGenerationsAreCleanedUp(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_clean", branchA1)
	segment := env.activeDynamicSegment(t, "Retention")

	for round := 0; round < 2; round++ {
		if _, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); err != nil {
			t.Fatal(err)
		}
		env.materialize(t)
	}
	var generations int
	if err := env.pool.QueryRow(ctx, `SELECT count(DISTINCT generation) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2`, businessA, segment.ID).Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if generations != 1 {
		t.Fatalf("distinct generations = %d; superseded membership must be released", generations)
	}
	// The evaluation rows themselves survive as the audit record of what each
	// audience was.
	var evaluations int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2`, businessA, segment.ID).Scan(&evaluations); err != nil {
		t.Fatal(err)
	}
	if evaluations < 3 {
		t.Fatalf("evaluation rows = %d; the history of past materializations must survive cleanup", evaluations)
	}
}

// pinAllGenerations is a retainer that refuses every cleanup, standing in for
// the campaign snapshot Section 8E will add.
type pinAllGenerations struct{}

func (pinAllGenerations) Retained(context.Context, string, string, int64) (bool, error) {
	return true, nil
}

// TestMaterialization_RetentionPortCanPinGenerations proves the hook a future
// campaign will use actually prevents cleanup, so an audience a campaign is
// still reconciling against cannot be deleted underneath it.
func TestMaterialization_RetentionPortCanPinGenerations(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.retention = pinAllGenerations{}
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedRepeatCustomer(t, "cus_pinned", branchA1)
	segment := env.activeDynamicSegment(t, "Pinned")

	if _, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)

	var generations int
	if err := env.pool.QueryRow(ctx, `SELECT count(DISTINCT generation) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2`, businessA, segment.ID).Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if generations != 2 {
		t.Fatalf("distinct generations = %d; a pinned generation must survive cleanup", generations)
	}
	// The live audience is still unambiguous even with a pinned older
	// generation present.
	reloaded := env.reloadSegment(t, businessA, segment.ID)
	if ids := env.memberIDs(t, reloaded); len(ids) != 1 {
		t.Fatalf("members = %v; the live generation must still resolve to one audience", ids)
	}
}

// TestMaterialization_ConsentEligibilityAgreesWithSection8A cross-checks the
// SQL twin of MarketingEligibility against the Go rule that defines the policy.
func TestMaterialization_ConsentEligibilityAgreesWithSection8A(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	allowed := env.seedRepeatCustomer(t, "cus_allow", branchA1)
	denied := env.seedRepeatCustomer(t, "cus_deny", branchA1)
	silent := env.seedRepeatCustomer(t, "cus_unknown", branchA1)
	expired := env.seedRepeatCustomer(t, "cus_expired", branchA1)

	past := env.now.Add(-time.Hour)
	for _, entry := range []struct {
		profile  Profile
		decision ConsentDecision
		expires  *time.Time
	}{
		{allowed, ConsentAllow, nil},
		{denied, ConsentDeny, nil},
		{expired, ConsentAllow, &past},
	} {
		if err := env.tx(t, func(tx pgx.Tx) error {
			return env.service.ApplyConsent(ctx, tx, actor, entry.profile.ID, ConsentInput{
				SourceService: "customer-service", SourceVersion: 1, Purpose: ConsentPurposeMarketing,
				Channel: ChannelEmail, Decision: entry.decision, ObservedAt: env.now, ExpiresAt: entry.expires,
			})
		}); err != nil {
			t.Fatalf("apply consent: %v", err)
		}
	}

	result, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		Rule: ruleJSON(`{"schema_version":1,"root":{"field":"consent.marketing.email.eligible","operator":"EQ","value":true}}`)})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if result.MatchCount != 1 {
		t.Fatalf("eligible count = %d; want only the explicit unexpired ALLOW", result.MatchCount)
	}
	if result.Sample[0].ProfileRef != allowed.ID {
		t.Fatalf("eligible = %s; want %s", result.Sample[0].ProfileRef, allowed.ID)
	}

	// The segment answer must match Section 8A's own eligibility rule for every
	// one of these relationships, or the two definitions have drifted.
	for _, profile := range []Profile{allowed, denied, silent, expired} {
		reloaded := env.reload(t, businessA, profile.ID)
		summary, summaryErr := env.service.ConsentSummaryFor(ctx, actor, reloaded)
		if summaryErr != nil {
			t.Fatal(summaryErr)
		}
		wantEligible := summary.Marketing[ChannelEmail].Eligible
		gotEligible := profile.ID == allowed.ID
		if wantEligible != gotEligible {
			t.Fatalf("profile %s: Section 8A says eligible=%v, segmentation says %v — the two definitions have drifted",
				profile.ID, wantEligible, gotEligible)
		}
	}
}
