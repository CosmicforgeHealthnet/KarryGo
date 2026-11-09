// src/app/(setup)/setup/submit/page.tsx — Review & submit: the last step.
"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AlertCircle, Check, CheckCircle2, Clock, Info, Loader2, RefreshCw } from "lucide-react";
import { organizationService } from "@/lib/services/organizationService";
import { StepFooter } from "@/components/onboarding/StepFooter";
import { useSetupStatus, apiErrorMessage } from "../useSetupStatus";
import { previousPath, isStepComplete } from "../steps";

export default function ReviewAndSubmitPage() {
  const router = useRouter();
  const { organization, loading, error, stages, canSubmit, submitted, setSubmitted, workSteps, doneCount, allDone, reload } =
    useSetupStatus();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!organization?.organizationId || !canSubmit) return;
    setIsSubmitting(true);
    setSubmitError(null);
    try {
      await organizationService.submitOrganization(organization.organizationId);
      setSubmitted(true);
    } catch (err) {
      setSubmitError(apiErrorMessage(err, "We couldn't submit your company. Please try again."));
    } finally {
      setIsSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex justify-center py-24">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-2xl border border-line bg-surface p-8 text-center">
        <p className="text-[15px] font-bold text-ink-950">We couldn&apos;t load your progress</p>
        <p className="mt-1 text-sm text-ink-500">{error}</p>
        <button onClick={reload} className="btn btn-primary btn-md mt-5">
          <RefreshCw />
          Try again
        </button>
      </div>
    );
  }

  if (submitted) {
    return (
      <div className="rounded-2xl border border-line bg-surface p-8 text-center shadow-[0_1px_2px_rgba(11,31,68,0.05)] sm:p-12">
        <span className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-primary-bg text-label-accent">
          <CheckCircle2 className="h-8 w-8" />
        </span>
        <h1 className="mt-5 text-ink-950">Submitted for review</h1>
        <p className="mx-auto mt-2 max-w-md text-ink-600">
          Thanks. Our team is now reviewing your company. You can&apos;t change your details while the review is in
          progress.
        </p>
        <button onClick={() => router.push("/")} className="btn btn-primary btn-lg mt-7">
          Go to dashboard
        </button>
      </div>
    );
  }

  const remaining = workSteps.length - doneCount;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-ink-950">Review & submit</h1>
        <p className="mt-2 text-ink-600">
          {allDone
            ? "Everything is in place. Submit your company and our team will review it."
            : `Finish ${remaining} more ${remaining === 1 ? "step" : "steps"} before you can submit.`}
        </p>
      </div>

      <div className="overflow-hidden rounded-2xl border border-line bg-surface shadow-[0_1px_2px_rgba(11,31,68,0.05)]">
        {workSteps.map((step, index) => {
          const done = isStepComplete(step, stages);
          return (
            <div key={step.id} className={`flex items-center gap-4 px-5 py-4 ${index > 0 ? "border-t border-line" : ""}`}>
              <span
                className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full ${
                  done ? "bg-label-accent text-surface" : "bg-amber-bg text-amber"
                }`}
              >
                {done ? <Check className="h-4 w-4" strokeWidth={3} /> : <Clock className="h-4 w-4" />}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-[15px] font-bold text-ink-950">{step.label}</span>
                <span className="mt-0.5 block text-[13px] text-ink-500">{done ? "Done" : "Not finished yet"}</span>
              </span>
              <Link href={step.path} className={`btn btn-sm ${done ? "btn-secondary" : "btn-primary"}`}>
                {done ? "Edit" : "Finish"}
              </Link>
            </div>
          );
        })}
      </div>

      {allDone && (
        <div className="flex gap-3 rounded-2xl bg-primary-bg p-4 text-[14px] text-ink-800">
          <Info className="mt-0.5 h-5 w-5 flex-shrink-0 text-label-accent" />
          <p>
            Once you submit, you won&apos;t be able to change your details while our team reviews your company.
          </p>
        </div>
      )}

      {submitError && (
        <p className="flex items-center gap-2 rounded-xl bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {submitError}
        </p>
      )}

      <StepFooter
        onBack={() => router.push(previousPath("review"))}
        onPrimary={handleSubmit}
        primaryLabel="Submit for review"
        primaryDisabled={!allDone || !canSubmit}
        primaryLoading={isSubmitting}
        loadingLabel="Submitting…"
      />
    </div>
  );
}
