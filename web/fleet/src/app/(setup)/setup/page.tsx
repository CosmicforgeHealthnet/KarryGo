// src/app/(setup)/setup/page.tsx — Setup home: where you are, and one button
// to carry on from where you stopped.
"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  Check,
  ChevronRight,
  Clock,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { useSetupStatus } from "./useSetupStatus";
import {
  SETUP_STEPS,
  CREATE_COMPANY_PATH,
  firstOpenStep,
  isStepComplete,
} from "./steps";

export default function SetupHomePage() {
  const router = useRouter();
  const {
    organization,
    loading,
    error,
    stages,
    submitted,
    workSteps,
    doneCount,
    allDone,
    reload,
  } = useSetupStatus();
  const hasCompany = !!organization?.organizationId;

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
        <p className="text-[15px] font-bold text-ink-950">
          We couldn&apos;t load your progress
        </p>
        <p className="mt-1 text-sm text-ink-500">{error}</p>
        <button onClick={reload} className="btn btn-primary btn-md mt-5">
          <RefreshCw />
          Try again
        </button>
      </div>
    );
  }

  const next = firstOpenStep(stages, hasCompany);
  const nextPath = hasCompany ? next.path : CREATE_COMPANY_PATH;
  const percent = Math.round((doneCount / workSteps.length) * 100);
  const companyName =
    organization && "legalName" in organization
      ? (organization as { legalName?: string }).legalName
      : undefined;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-ink-950">
          {companyName ? `Set up ${companyName}` : "Set up your company"}
        </h1>
        <p className="mt-2 text-ink-600">
          Complete these steps so we can verify your company. Your progress is
          saved as you go.
        </p>
      </div>

      {/* Progress and the one main action */}
      <div className="rounded-2xl border border-line bg-surface p-5 shadow-[0_1px_2px_rgba(11,31,68,0.05)] sm:p-6">
        {submitted ? (
          <div className="flex items-start gap-4">
            <span className="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full bg-amber-bg text-amber">
              <Clock className="h-5 w-5" />
            </span>
            <div>
              <p className="text-[17px] font-bold text-ink-950">
                Your company is being reviewed
              </p>
              <p className="mt-1 text-sm text-ink-600">
                You&apos;ve submitted everything. You can&apos;t change your
                details while the review is in progress.
              </p>
            </div>
          </div>
        ) : (
          <>
            <div className="flex items-end justify-between gap-4">
              <div>
                <p className="text-[13px] font-bold text-label-accent">
                  {allDone
                    ? "All steps done"
                    : `${doneCount} of ${workSteps.length} steps done`}
                </p>
                <p className="mt-1 text-[18px] font-bold text-ink-950">
                  {allDone
                    ? "Ready to submit for review"
                    : hasCompany
                      ? `Next: ${next.label}`
                      : "Start with your company details"}
                </p>
              </div>
              <span className="text-[13px] font-bold text-ink-500">
                {percent}%
              </span>
            </div>
            <div className="mt-4 h-2 overflow-hidden rounded-full bg-surface-3">
              <div
                className="h-full rounded-full bg-primary transition-all"
                style={{ width: `${percent}%` }}
              />
            </div>
            <button
              onClick={() => router.push(nextPath)}
              className="btn btn-primary btn-lg mt-5 w-full sm:w-auto"
            >
              {allDone
                ? "Review & submit"
                : doneCount === 0 && !hasCompany
                  ? "Start setup"
                  : "Continue setup"}
              <ArrowRight />
            </button>
          </>
        )}
      </div>

      {/* Every step, with its status; tap to open */}
      <div className="overflow-hidden rounded-2xl border border-line bg-surface shadow-[0_1px_2px_rgba(11,31,68,0.05)]">
        {SETUP_STEPS.map((step, index) => {
          const done = isStepComplete(step, stages);
          const isReview = step.stages.length === 0;
          const locked = !hasCompany && index > 0;
          const href =
            index === 0 && !hasCompany ? CREATE_COMPANY_PATH : step.path;
          const status = done
            ? "Done"
            : isReview
              ? allDone
                ? "Ready"
                : "Not yet"
              : step === next && hasCompany
                ? "Up next"
                : "To do";
          const row = (
            <div
              className={`flex items-center gap-4 px-5 py-4 ${locked ? "opacity-60" : ""}`}
            >
              <span
                className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full text-[13px] font-bold ${
                  done
                    ? "bg-label-accent text-surface"
                    : "border border-field-border text-ink-500"
                }`}
              >
                {done ? (
                  <Check className="h-4 w-4" strokeWidth={3} />
                ) : (
                  index + 1
                )}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-[15px] font-bold text-ink-950">
                  {step.label}
                </span>
                <span className="mt-0.5 block truncate text-[13px] text-ink-500">
                  {step.description}
                </span>
              </span>
              <span
                className={`hidden flex-shrink-0 rounded-full px-2.5 py-1 text-[12px] font-bold sm:inline-flex ${
                  done
                    ? "bg-primary-bg text-label-accent"
                    : status === "Up next" || status === "Ready"
                      ? "bg-ink-950 text-surface"
                      : "bg-surface-3 text-ink-500"
                }`}
              >
                {status}
              </span>
              {!locked && (
                <ChevronRight className="h-5 w-5 flex-shrink-0 text-ink-400" />
              )}
            </div>
          );
          return (
            <div
              key={step.id}
              className={index > 0 ? "border-t border-line" : ""}
            >
              {locked || submitted ? (
                row
              ) : (
                <Link
                  href={href}
                  className="block transition hover:bg-surface-2"
                >
                  {row}
                </Link>
              )}
            </div>
          );
        })}
      </div>

      <p className="text-center text-[13px] text-ink-500">
        Need help?{" "}
        <a
          href="mailto:support@karrygo.com"
          className="font-bold text-label-accent hover:underline"
        >
          Contact support
        </a>
      </p>
    </div>
  );
}
