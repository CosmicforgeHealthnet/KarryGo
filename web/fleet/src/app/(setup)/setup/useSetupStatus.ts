"use client";

import { useCallback, useEffect, useState } from "react";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";
import { SETUP_STEPS, isStepComplete, type StageStatuses } from "./steps";

// Setup progress for the current company, read from the backend.
export function useSetupStatus() {
  const { organization } = useOrganizationStore();
  const { setStageStatuses } = useOnboardingStore();
  const organizationId = organization?.organizationId;

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [stages, setStages] = useState<StageStatuses>({});
  const [canSubmit, setCanSubmit] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  type OnboardingStatus = Awaited<ReturnType<typeof organizationService.getOnboardingStatus>>;

  const apply = useCallback(
    (status: OnboardingStatus) => {
      const map: StageStatuses = {};
      status.stages.forEach((s) => {
        map[s.stage] = s.status;
      });
      setStages(map);
      setStageStatuses(map);
      setCanSubmit(status.allowedNextActions?.includes("SUBMIT_FOR_VERIFICATION") ?? false);
      setSubmitted(
        ["BUSINESS_SUBMITTED", "UNDER_REVIEW"].includes(status.organizationStatus) ||
          ["SUBMITTED", "UNDER_REVIEW"].includes(status.verification?.status ?? "")
      );
      setError(null);
    },
    [setStageStatuses]
  );

  // Loads progress; the returned function ignores a reply that arrives late.
  const fetchStatus = useCallback(() => {
    let active = true;
    if (organizationId) {
      organizationService
        .getOnboardingStatus(organizationId)
        .then((status) => active && apply(status))
        .catch((err) => active && setError(apiErrorMessage(err, "We couldn't load your progress.")))
        .finally(() => active && setLoading(false));
    }
    return () => {
      active = false;
    };
  }, [organizationId, apply]);

  useEffect(() => fetchStatus(), [fetchStatus]);

  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchStatus();
  }, [fetchStatus]);

  const workSteps = SETUP_STEPS.filter((s) => s.stages.length > 0);
  const doneCount = workSteps.filter((s) => isStepComplete(s, stages)).length;

  return {
    organization,
    loading: organizationId ? loading : false,
    error,
    stages,
    canSubmit,
    submitted,
    setSubmitted,
    workSteps,
    doneCount,
    allDone: doneCount === workSteps.length,
    reload,
  };
}

// The API's error message, or a fallback.
export function apiErrorMessage(err: unknown, fallback: string): string {
  const message = (err as { response?: { data?: { error?: { message?: string } } } })?.response?.data?.error?.message;
  return message || fallback;
}
