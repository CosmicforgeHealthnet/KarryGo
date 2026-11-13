"use client";
import { useOnboarding } from "./onboarding-context";
import { SETUP_STEPS, isStepComplete } from "./steps";
import { errorMessage } from "./onboarding-ui";
export function useSetupStatus() {
  const { data, reload } = useOnboarding();
  const stages = Object.fromEntries(
    (data.status?.stages ?? []).map((s) => [s.stage, s.status]),
  );
  const workSteps = SETUP_STEPS.filter((s) => s.stages.length);
  const doneCount = workSteps.filter((s) => isStepComplete(s, stages)).length;
  return {
    organization: data.organization,
    loading: false,
    error: null,
    stages,
    canSubmit:
      data.status?.allowedNextActions.includes("SUBMIT_FOR_VERIFICATION") ??
      false,
    workSteps,
    doneCount,
    allDone: doneCount === workSteps.length,
    reload,
  };
}
export function apiErrorMessage(error: unknown, fallback: string) {
  return error ? errorMessage(error).message : fallback;
}
