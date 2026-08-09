package automation

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

// TestHandleRuleExecute_FailsClassifiedPermanent proves the automation
// runner boundary never silently succeeds and never retries forever: with
// no Section 24 automation domain implemented, every execution attempt is a
// classified PERMANENT failure that dead-letters on the first attempt.
func TestHandleRuleExecute_FailsClassifiedPermanent(t *testing.T) {
	err := handleRuleExecute(context.Background(), jobs.Job{JobType: JobRuleExecute})
	if err == nil {
		t.Fatal("expected an error: no automation domain exists to execute a rule")
	}
	var classified *jobs.ClassifiedError
	if !errors.As(err, &classified) {
		t.Fatalf("expected a *jobs.ClassifiedError, got %T: %v", err, err)
	}
	if classified.Class != jobs.ErrorClassPermanent {
		t.Fatalf("class = %s, want PERMANENT (must dead-letter, not retry-loop)", classified.Class)
	}
	if classified.Code != "automation_domain_not_implemented" {
		t.Fatalf("code = %s, want automation_domain_not_implemented", classified.Code)
	}
}

// TestRegisterWorkers_NilWorkerIsNoop proves registration tolerates a nil
// worker (the same defensive pattern every other Section 27 registrar in
// this service uses when a section is disabled).
func TestRegisterWorkers_NilWorkerIsNoop(t *testing.T) {
	RegisterWorkers(nil) // must not panic
}

// TestRegisterWorkers_BindsWithoutPanic proves RegisterWorkers can bind
// JobRuleExecute onto a real *jobs.Worker without requiring a database or
// any other dependency -- Worker.Register itself is exercised (and
// unit-tested for correct binding) in the jobs package; the handler's own
// failure behavior is proven directly by
// TestHandleRuleExecute_FailsClassifiedPermanent above.
func TestRegisterWorkers_BindsWithoutPanic(t *testing.T) {
	w := &jobs.Worker{}
	RegisterWorkers(w)
}
