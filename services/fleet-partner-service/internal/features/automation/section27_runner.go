package automation

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

// JobRuleExecute is the Section 27 job type reserved for executing an
// approved automation rule. Registering it now -- even with no domain to
// call -- gives a future Section 24 implementation a handler-registration
// boundary to fill in, and gives anything that mistakenly enqueues this job
// type today a loud, traceable dead-letter instead of an unregistered-type
// failure or a silently vanished job.
const JobRuleExecute = "AUTOMATION_RULE_EXECUTE"

// RegisterWorkers attaches the automation runner boundary to the shared
// durable queue.
//
// This is infrastructure only: no automation domain exists yet (Section 24
// is not implemented -- internal/features/automation has no models,
// repositories or usecases beyond this file). The handler below classifies
// every execution as a permanent, non-retryable failure so a job never
// retry-loops or silently no-ops waiting for a capability that is not
// there. Nothing in this service enqueues AUTOMATION_RULE_EXECUTE today;
// the registration exists solely so Section 24, once built, has a stable
// job type and a place to replace this handler rather than invent one.
func RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(JobRuleExecute, handleRuleExecute)
}

func handleRuleExecute(_ context.Context, _ jobs.Job) error {
	return &jobs.ClassifiedError{
		Class:       jobs.ErrorClassPermanent,
		Code:        "automation_domain_not_implemented",
		SafeMessage: "Automation rule execution is not available: the automation domain (Section 24) is not implemented in this deployment.",
	}
}
