// src/app/(setup)/setup/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Building2,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  User,
  Briefcase,
  FileText,
  ShieldCheck,
  Wallet,
  FileCheck,
  ListChecks,
  Lock,
  PlusCircle,
} from "lucide-react";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";
import { useAuthStore } from "@/lib/stores/authStore";

type OnboardingStep = {
  id: string;
  label: string;
  description: string;
  status: "Completed" | "InProgress" | "Pending" | "Blocked" | "Failed" | "NotStarted";
  path: string;
  icon: React.ReactNode;
  isBlocking: boolean;
  requiredForNext: boolean;
  blockedReason?: string;
};

type ApiStage = {
  stage: string;
  status: string;
  completedAt: string | null;
  missingFields?: string[];
  blockingIssues?: string[];
};

type OnboardingState = {
  organizationId: string;
  organizationStatus: string;
  organizationVersion: number;
  stages: ApiStage[];
  allowedNextActions: string[];
  verification: {
    status: string;
    submissionVersion: number;
    issues: unknown[];
  } | null;
  walletProvisioning: {
    status: string;
  } | null;
  updatedAt: string;
};

const STAGE_MAP: Record<string, { id: string; label: string; description: string; path: string; icon: React.ReactNode; isBlocking: boolean; requiredForNext: boolean }> = {
  CREATE_COMPANY: {
    id: "create-company",
    label: "Create Company",
    description: "Legal name, trading name, registration reference, country, company type, timezone, default currency and owner information.",
    path: "/setup/create-company",
    icon: <Building2 className="h-5 w-5" />,
    isBlocking: true,
    requiredForNext: true,
  },
  PROFILE: {
    id: "profile",
    label: "Company Profile",
    description: "Company identity, operating contact data, active status, version and edit eligibility.",
    path: "/setup/profile",
    icon: <User className="h-5 w-5" />,
    isBlocking: true,
    requiredForNext: true,
  },
  CAPABILITIES: {
    id: "capabilities",
    label: "Operating Capabilities",
    description: "Enabled logistics capabilities and their current availability.",
    path: "/setup/capabilities",
    icon: <Briefcase className="h-5 w-5" />,
    isBlocking: false,
    requiredForNext: false,
  },
  DOCUMENTS: {
    id: "documents",
    label: "Business Documents",
    description: "Required and uploaded documents, document type, status and replacement requirement.",
    path: "/setup/business-docs",
    icon: <FileText className="h-5 w-5" />,
    isBlocking: true,
    requiredForNext: true,
  },
  VERIFICATION: {
    id: "verification",
    label: "Business Verification",
    description: "Submission version, current verification state, issues and requested corrections.",
    path: "/setup/verification",
    icon: <ShieldCheck className="h-5 w-5" />,
    isBlocking: true,
    requiredForNext: true,
  },
  WALLET: {
    id: "wallet-provisioning",
    label: "Wallet Provisioning",
    description: "Provisioning state and retry history only - never a fabricated balance.",
    path: "/setup/wallet-provisioning",
    icon: <Wallet className="h-5 w-5" />,
    isBlocking: true,
    requiredForNext: true,
  },
  SUBMIT: {
    id: "submit",
    label: "Submit for Review",
    description: "Readiness checklist and unresolved blockers.",
    path: "/setup/submit",
    icon: <FileCheck className="h-5 w-5" />,
    isBlocking: true,
    requiredForNext: true,
  },
};

const STATUS_MAP: Record<string, "Completed" | "InProgress" | "Pending" | "Blocked" | "Failed" | "NotStarted"> = {
  COMPLETED: "Completed",
  IN_PROGRESS: "InProgress",
  PENDING: "Pending",
  BLOCKED: "Blocked",
  FAILED: "Failed",
  NOT_STARTED: "NotStarted",
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Blocked: { bg: "bg-red-bg", text: "text-red", icon: <Lock className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-4 w-4" /> },
};

function getStatusFromApi(apiStatus: string): "Completed" | "InProgress" | "Pending" | "Blocked" | "Failed" | "NotStarted" {
  return STATUS_MAP[apiStatus] || "NotStarted";
}

function getStepStatus(stage: ApiStage | undefined): "Completed" | "InProgress" | "Pending" | "Blocked" | "Failed" | "NotStarted" {
  if (!stage) return "NotStarted";
  return getStatusFromApi(stage.status);
}

function isStepBlocked(stage: ApiStage | undefined): boolean {
  if (!stage) return false;
  return stage.status === "BLOCKED" && stage.blockingIssues && stage.blockingIssues.length > 0;
}

function getBlockedReason(stage: ApiStage | undefined): string | undefined {
  if (!stage || !stage.blockingIssues) return undefined;
  return stage.blockingIssues.join(", ");
}

export default function OnboardingHomePage() {
  const router = useRouter();
  const { organizationId, setLoading, setError } = useOnboardingStore();
  const { accessToken } = useAuthStore();

  const [loading, setLoadingState] = useState(true);
  const [data, setData] = useState<OnboardingState | null>(null);
  const [error, setErrorState] = useState<string | null>(null);

  useEffect(() => {
    async function fetchOnboardingStatus() {
      if (!organizationId) {
        setLoadingState(false);
        return;
      }

      setLoadingState(true);
      setErrorState(null);

      try {
        const response = await organizationService.getOnboardingStatus(organizationId);
        setData(response);
      } catch (err: any) {
        const message = err?.response?.data?.error?.message || err?.message || "Failed to load onboarding status.";
        setErrorState(message);
        setError(message);
      } finally {
        setLoadingState(false);
      }
    }

    fetchOnboardingStatus();
  }, [organizationId, setError]);

  const goToStep = (path: string, status: string) => {
    if (status === "Blocked") {
      return;
    }
    router.push(path);
  };

  const handleResume = () => {
    if (!data) return;
    
    const stages = data.stages;
    const inProgressStage = stages.find((s) => s.status === "IN_PROGRESS");
    if (inProgressStage) {
      const step = STAGE_MAP[inProgressStage.stage];
      if (step) {
        router.push(step.path);
        return;
      }
    }

    const pendingStage = stages.find((s) => s.status === "PENDING" || s.status === "NOT_STARTED");
    if (pendingStage) {
      const step = STAGE_MAP[pendingStage.stage];
      if (step) {
        router.push(step.path);
        return;
      }
    }
  };

  const handleSubmit = () => {
    if (data?.allowedNextActions?.includes("submit")) {
      router.push("/setup/submit");
    }
  };

  if (loading) {
    return (
      <div className="w-full space-y-6">
        <div className="space-y-2">
          <div className="h-7 w-64 animate-pulse rounded-lg bg-surface-2" />
          <div className="h-4 w-96 animate-pulse rounded-lg bg-surface-2" />
        </div>
        <div className="h-28 animate-pulse rounded-2xl bg-surface-2" />
        <div className="h-72 animate-pulse rounded-2xl bg-surface-2" />
        <div className="h-20 animate-pulse rounded-2xl bg-surface-2" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-red/20 bg-red-bg/30 py-14 px-4">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-4 text-[15px] font-semibold text-ink-950">Something went wrong</h3>
        <p className="mt-1.5 text-sm text-ink-500 text-center max-w-md">{error}</p>
        <button
          onClick={() => window.location.reload()}
          className="mt-5 rounded-lg bg-primary px-5 py-2 text-[13px] font-semibold text-white transition hover:bg-primary/90"
        >
          Try again
        </button>
      </div>
    );
  }

  // Empty state - no organization created yet
  if (!data) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface-2/40 px-4 py-16 text-center">
        <div className="relative">
          <div className="rounded-full bg-primary-bg p-6">
            <Building2 className="h-12 w-12 text-primary" strokeWidth={1.5} />
          </div>
          <div className="absolute -right-1 -top-1 rounded-full bg-green-bg p-1.5">
            <PlusCircle className="h-4 w-4 text-green" />
          </div>
        </div>
        <h3 className="mt-5 text-xl font-semibold text-ink-950">Start your company setup</h3>
        <p className="mt-2 max-w-md text-sm text-ink-500">
          Get started by creating your company profile. You'll be able to add capabilities, documents, and more in the next steps.
        </p>
        <button
          onClick={() => router.push("/setup/create-company")}
          className="mt-6 inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
        >
          <PlusCircle className="h-4 w-4" />
          Create Company
        </button>
        <p className="mt-4 text-xs text-ink-400">
          This will set up your organization in the KarryGo Fleet OS platform.
        </p>
      </div>
    );
  }

  // Build steps from API data
  const steps: OnboardingStep[] = data.stages.map((stage) => {
    const meta = STAGE_MAP[stage.stage];
    const status = getStepStatus(stage);
    const blocked = isStepBlocked(stage);
    const blockedReason = getBlockedReason(stage);

    return {
      id: meta?.id || stage.stage.toLowerCase(),
      label: meta?.label || stage.stage.replace(/_/g, " "),
      description: meta?.description || "",
      path: meta?.path || `/setup/${stage.stage.toLowerCase()}`,
      icon: meta?.icon || <Building2 className="h-5 w-5" />,
      isBlocking: meta?.isBlocking ?? true,
      requiredForNext: meta?.requiredForNext ?? true,
      status: blocked ? "Blocked" : status,
      blockedReason: blocked ? blockedReason : undefined,
    };
  });

  const completedCount = steps.filter((s) => s.status === "Completed").length;
  const totalSteps = steps.length;
  const progressPct = Math.round((completedCount / totalSteps) * 100);

  const isSubmitted = data.verification?.status === "SUBMITTED";
  const isApproved = data.verification?.status === "APPROVED";
  const isRejected = data.verification?.status === "REJECTED";
  const isVerifying = data.verification?.status === "IN_PROGRESS" || data.verification?.status === "PENDING";

  const walletFailed = data.walletProvisioning?.status === "FAILED";

  const canSubmit = data.allowedNextActions?.includes("submit") ?? false;

  const progressColor = progressPct >= 80 ? "#139a5d" : progressPct >= 50 ? "#d97706" : "#df2f3f";
  const progressColorLight = progressPct >= 80 ? "#4ade80" : progressPct >= 50 ? "#fbbf24" : "#f87171";

  return (
    <div className="w-full space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Welcome to KarryGo Fleet OS
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Let&apos;s get your company set up. Complete each step to start using the platform.
        </p>
      </div>

      {/* Overall Progress */}
      <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-5">
            <div className="relative h-20 w-20 flex-shrink-0">
              <svg className="h-20 w-20 -rotate-90 transform" viewBox="0 0 80 80">
                <defs>
                  <linearGradient id="progress-ring-gradient" x1="0%" y1="0%" x2="100%" y2="100%">
                    <stop offset="0%" stopColor={progressColorLight} />
                    <stop offset="100%" stopColor={progressColor} />
                  </linearGradient>
                </defs>
                <circle cx="40" cy="40" r="32" fill="none" stroke="#E5E7EB" strokeWidth="6" />
                <circle
                  cx="40"
                  cy="40"
                  r="32"
                  fill="none"
                  stroke="url(#progress-ring-gradient)"
                  strokeWidth="6"
                  strokeDasharray={`${progressPct * 2.01} 201.06`}
                  strokeLinecap="round"
                  className="transition-[stroke-dasharray] duration-700 ease-out"
                />
              </svg>
              <span className="absolute inset-0 flex items-center justify-center text-lg font-bold text-ink-950">
                {progressPct}%
              </span>
            </div>
            <div>
              <div className="text-sm font-medium text-ink-500">Overall Progress</div>
              <div className="flex items-center gap-1.5 text-lg font-bold text-ink-950">
                {isApproved ? (
                  <>
                    <CheckCircle2 className="h-5 w-5 text-green" />
                    Company Approved!
                  </>
                ) : isSubmitted ? (
                  <>
                    <Clock className="h-5 w-5 text-amber" />
                    Awaiting Review
                  </>
                ) : isRejected ? (
                  <>
                    <AlertCircle className="h-5 w-5 text-red" />
                    Needs Attention
                  </>
                ) : (
                  "In Progress"
                )}
              </div>
              {data.verification?.issues && data.verification.issues.length > 0 && (
                <div className="mt-0.5 text-xs text-red">
                  {data.verification.issues.length} issue(s) need attention
                </div>
              )}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {!isSubmitted && !isApproved && !isRejected && (
              <button
                onClick={handleResume}
                className="flex h-10 items-center gap-2 rounded-xl bg-primary px-5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                Resume Setup
              </button>
            )}
            {isSubmitted && !isApproved && (
              <button
                onClick={() => router.push("/setup/verification")}
                className="flex h-10 items-center gap-2 rounded-xl bg-amber px-5 text-sm font-semibold text-white shadow-sm shadow-amber/20 transition hover:bg-amber/90"
              >
                Check Status
              </button>
            )}
            {isRejected && (
              <button
                onClick={() => router.push("/setup/verification")}
                className="flex h-10 items-center gap-2 rounded-xl bg-red px-5 text-sm font-semibold text-white shadow-sm shadow-red/20 transition hover:bg-red/90"
              >
                Fix Issues
              </button>
            )}
            {isApproved && (
              <button
                onClick={() => router.push("/")}
                className="flex h-10 items-center gap-2 rounded-xl bg-green px-5 text-sm font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
              >
                Go to Dashboard
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Steps */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <ListChecks className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Onboarding Steps</span>
            <span className="text-xs text-ink-400">
              {completedCount}/{totalSteps} complete
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {steps.map((step) => {
            const statusStyle = STATUS_STYLES[step.status] || STATUS_STYLES.NotStarted;
            const isClickable = step.status !== "Blocked";
            const isCurrent = step.status === "InProgress";

            return (
              <div
                key={step.id}
                onClick={() => isClickable && goToStep(step.path, step.status)}
                className={`relative flex items-center gap-4 p-4 transition ${
                  isCurrent ? "bg-primary-bg/40" : ""
                } ${
                  isClickable ? "cursor-pointer hover:bg-surface-2" : "cursor-not-allowed opacity-70"
                }`}
              >
                {isCurrent && (
                  <span className="absolute inset-y-0 left-0 w-1 rounded-r bg-primary" />
                )}
                <div className="flex-shrink-0">
                  <div className={`rounded-full p-2 ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={`font-medium ${isCurrent ? "text-primary" : "text-ink-950"}`}>
                      {step.label}
                    </span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      {step.status}
                    </span>
                    {step.isBlocking && step.status !== "Completed" && step.status !== "Blocked" && (
                      <span className="text-[9px] text-red font-medium">Required</span>
                    )}
                  </div>
                  <div className="mt-0.5 text-xs text-ink-500 line-clamp-1">{step.description}</div>
                  {step.status === "Blocked" && step.blockedReason && (
                    <div className="mt-1.5 flex items-start gap-1.5 text-xs text-red">
                      <AlertCircle className="h-3.5 w-3.5 flex-shrink-0 mt-[1px]" />
                      <span>{step.blockedReason}</span>
                    </div>
                  )}
                </div>
                <div className="flex-shrink-0">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      if (step.status !== "Blocked") {
                        router.push(step.path);
                      }
                    }}
                    className={`text-sm font-medium ${
                      step.status === "Blocked"
                        ? "text-ink-400 cursor-not-allowed"
                        : "text-primary hover:underline flex items-center gap-1"
                    }`}
                  >
                    {step.status === "Completed" ? "View" :
                     step.status === "InProgress" ? "Continue" :
                     step.status === "Blocked" ? "Blocked" :
                     "Start"}
                    {step.status !== "Blocked" && <ArrowRight className="h-3.5 w-3.5" />}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Wallet Retry */}
      {walletFailed && (
        <div className="flex items-center justify-between rounded-2xl border border-red/20 bg-red-bg/30 p-4 shadow-sm">
          <div className="flex items-center gap-2 text-sm text-red">
            <AlertCircle className="h-4 w-4" />
            Wallet provisioning failed.
          </div>
          <button
            onClick={() => router.push("/setup/wallet-provisioning")}
            className="rounded-lg border border-primary/20 bg-primary-bg px-3 py-1 text-xs font-medium text-primary hover:bg-primary/10 transition-colors"
          >
            Retry
          </button>
        </div>
      )}

      {/* Submit Action */}
      {!isSubmitted && !isApproved && !isRejected && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="text-[15px] font-semibold text-ink-950">Ready to Submit?</h3>
              <p className="mt-0.5 text-sm text-ink-600">
                {canSubmit
                  ? "All required steps are complete. You can now submit your company for review."
                  : "Complete all required steps before submitting for review."}
              </p>
            </div>
            <button
              onClick={handleSubmit}
              disabled={!canSubmit}
              className={`flex h-10 items-center gap-2 rounded-xl px-5 text-sm font-semibold text-white shadow-sm transition ${
                canSubmit
                  ? "bg-primary shadow-primary/20 hover:bg-primary/90"
                  : "bg-ink-400 cursor-not-allowed"
              }`}
            >
              <FileCheck className="h-4 w-4" />
              Submit for Review
            </button>
          </div>
        </div>
      )}
    </div>
  );
}