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
  AlertTriangle,
  ArrowRight,
  User,
  Briefcase,
  FileText,
  ShieldCheck,
  Wallet,
  FileCheck,
  ListChecks,
  X,
} from "lucide-react";

// ==================== TYPES ====================

type OnboardingStep = {
  id: string;
  label: string;
  description: string;
  status: "Completed" | "InProgress" | "Pending" | "Blocked" | "NotStarted";
  path: string;
  icon: React.ReactNode;
  isBlocking: boolean;
  requiredForNext: boolean;
};

type OnboardingState = {
  currentStep: string;
  steps: OnboardingStep[];
  overallProgress: number;
  verificationState: "Pending" | "InProgress" | "Verified" | "Rejected" | "NotStarted";
  walletProvisioningState: "Pending" | "InProgress" | "Provisioned" | "Failed" | "NotStarted";
  canSubmit: boolean;
  isSubmitted: boolean;
  isApproved: boolean;
  blockers: string[];
  notes: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_ONBOARDING_STATE: OnboardingState = {
  currentStep: "profile",
  overallProgress: 42,
  verificationState: "Pending",
  walletProvisioningState: "NotStarted",
  canSubmit: false,
  isSubmitted: false,
  isApproved: false,
  blockers: ["Business documents required", "Company profile incomplete"],
  notes: "Awaiting business registration documents",
  steps: [
    {
      id: "create-company",
      label: "Create Company",
      description: "Legal name, trading name, registration reference, country, company type, timezone, default currency and owner information.",
      status: "Completed",
      path: "/setup/create-company",
      icon: <Building2 className="h-5 w-5" />,
      isBlocking: true,
      requiredForNext: true,
    },
    {
      id: "profile",
      label: "Company Profile",
      description: "Company identity, operating contact data, active status, version and edit eligibility.",
      status: "InProgress",
      path: "/setup/profile",
      icon: <User className="h-5 w-5" />,
      isBlocking: true,
      requiredForNext: true,
    },
    {
      id: "capabilities",
      label: "Operating Capabilities",
      description: "Enabled logistics capabilities and their current availability.",
      status: "Pending",
      path: "/setup/capabilities",
      icon: <Briefcase className="h-5 w-5" />,
      isBlocking: false,
      requiredForNext: false,
    },
    {
      id: "documents",
      label: "Business Documents",
      description: "Required and uploaded documents, document type, status and replacement requirement.",
      status: "Blocked",
      path: "/setup/business-docs",
      icon: <FileText className="h-5 w-5" />,
      isBlocking: true,
      requiredForNext: true,
    },
    {
      id: "verification",
      label: "Business Verification",
      description: "Submission version, current verification state, issues and requested corrections.",
      status: "Pending",
      path: "/setup/verification",
      icon: <ShieldCheck className="h-5 w-5" />,
      isBlocking: true,
      requiredForNext: true,
    },
    {
      id: "wallet-provisioning",
      label: "Wallet Provisioning",
      description: "Provisioning state and retry history only - never a fabricated balance.",
      status: "NotStarted",
      path: "/setup/wallet-provisioning",
      icon: <Wallet className="h-5 w-5" />,
      isBlocking: true,
      requiredForNext: true,
    },
    {
      id: "submit",
      label: "Submit for Review",
      description: "Readiness checklist and unresolved blockers.",
      status: "Pending",
      path: "/setup/submit",
      icon: <FileCheck className="h-5 w-5" />,
      isBlocking: true,
      requiredForNext: true,
    },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Blocked: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-4 w-4" /> },
};

const VERIFICATION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-4 w-4" /> },
};

const WALLET_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Provisioned: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-4 w-4" /> },
};

function toast(message: string) {
  console.log(message);
}

export default function OnboardingHomePage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<OnboardingState | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_ONBOARDING_STATE);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading onboarding progress...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Building2 className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No onboarding data found</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load your onboarding progress.</p>
        <button
          onClick={() => setSecondsAgo(0)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  const verificationStyle = VERIFICATION_STYLES[data.verificationState] || VERIFICATION_STYLES.NotStarted;
  const walletStyle = WALLET_STYLES[data.walletProvisioningState] || WALLET_STYLES.NotStarted;

  const goToStep = (path: string, status: string) => {
    if (status === "Blocked") {
      toast("This step is blocked. Please resolve blockers first.");
      return;
    }
    router.push(path);
  };

  const handleResume = () => {
    const currentStep = data.steps.find((s) => s.status === "InProgress");
    if (currentStep) {
      router.push(currentStep.path);
    } else {
      const nextStep = data.steps.find((s) => s.status === "Pending" || s.status === "NotStarted");
      if (nextStep) {
        router.push(nextStep.path);
      }
    }
  };

  const handleSubmit = () => {
    if (data.canSubmit) {
      router.push("/setup/submit");
    } else {
      toast("Complete all required steps before submitting.");
    }
  };

  const handleRetryWallet = () => {
    toast("Wallet provisioning retry requested (mock)");
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Welcome to KarryGo Fleet OS
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Let's get your company set up. Complete each step to start using the platform.
        </p>
      </div>

      {/* ===== OVERALL PROGRESS ===== */}
      <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-5">
            <div className="relative h-20 w-20 flex-shrink-0">
              <svg className="h-20 w-20 -rotate-90 transform" viewBox="0 0 80 80">
                <circle cx="40" cy="40" r="32" fill="none" stroke="#E5E7EB" strokeWidth="6" />
                <circle
                  cx="40"
                  cy="40"
                  r="32"
                  fill="none"
                  stroke={data.overallProgress >= 80 ? "#139a5d" : data.overallProgress >= 50 ? "#d97706" : "#df2f3f"}
                  strokeWidth="6"
                  strokeDasharray={`${data.overallProgress * 2.01} 201.06`}
                  strokeLinecap="round"
                />
              </svg>
              <span className="absolute inset-0 flex items-center justify-center text-lg font-bold text-ink-950">
                {data.overallProgress}%
              </span>
            </div>
            <div>
              <div className="text-sm font-medium text-ink-500">Overall Progress</div>
              <div className="text-lg font-bold text-ink-950">
                {data.isApproved ? "✅ Company Approved!" : data.isSubmitted ? "⏳ Awaiting Review" : "In Progress"}
              </div>
              {data.notes && (
                <div className="mt-0.5 text-xs text-ink-500">{data.notes}</div>
              )}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {!data.isSubmitted && !data.isApproved && (
              <button
                onClick={handleResume}
                className="flex h-10 items-center gap-2 rounded-xl bg-primary px-5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                Resume Setup
              </button>
            )}
            {data.isSubmitted && !data.isApproved && (
              <button
                onClick={() => router.push("/setup/verification")}
                className="flex h-10 items-center gap-2 rounded-xl bg-amber px-5 text-sm font-semibold text-white shadow-sm shadow-amber/20 transition hover:bg-amber/90"
              >
                Check Status
              </button>
            )}
            {data.isApproved && (
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

      {/* ===== STATUS OVERVIEW ===== */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Verification</div>
          <div className="mt-1 flex items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
              {verificationStyle.icon}
              {data.verificationState}
            </span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Wallet</div>
          <div className="mt-1 flex items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${walletStyle.bg} ${walletStyle.text}`}>
              {walletStyle.icon}
              {data.walletProvisioningState}
            </span>
            {data.walletProvisioningState === "Failed" && (
              <button
                onClick={handleRetryWallet}
                className="rounded-lg border border-primary/20 bg-primary-bg px-2 py-0.5 text-[10px] font-medium text-primary hover:bg-primary/10 transition-colors"
              >
                Retry
              </button>
            )}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Submit</div>
          <div className="mt-1 flex items-center gap-2">
            {data.isSubmitted ? (
              <span className="flex items-center gap-1 text-green">
                <CheckCircle2 className="h-4 w-4" />
                Submitted
              </span>
            ) : data.canSubmit ? (
              <span className="flex items-center gap-1 text-blue">Ready</span>
            ) : (
              <span className="flex items-center gap-1 text-amber">Not Ready</span>
            )}
          </div>
        </div>
      </div>

      {/* ===== BLOCKERS ===== */}
      {data.blockers.length > 0 && (
        <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red-bg p-2 flex-shrink-0">
              <AlertCircle className="h-5 w-5 text-red" />
            </div>
            <div className="flex-1">
              <h4 className="text-sm font-semibold text-red">Blockers</h4>
              <ul className="mt-2 space-y-1">
                {data.blockers.map((blocker, index) => (
                  <li key={index} className="text-sm text-red/80 flex items-start gap-2">
                    <span className="text-red text-base">•</span>
                    <span>{blocker}</span>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* ===== STEPS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <ListChecks className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Onboarding Steps</span>
            <span className="text-xs text-ink-400">
              {data.steps.filter((s) => s.status === "Completed").length}/{data.steps.length} complete
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {data.steps.map((step) => {
            const statusStyle = STATUS_STYLES[step.status] || STATUS_STYLES.NotStarted;
            const isClickable = step.status !== "Blocked";

            return (
              <div
                key={step.id}
                onClick={() => isClickable && goToStep(step.path, step.status)}
                className={`flex items-center gap-4 p-4 transition ${
                  isClickable ? "cursor-pointer hover:bg-surface-2" : "cursor-not-allowed opacity-60"
                }`}
              >
                <div className="flex-shrink-0">
                  <div className={`rounded-full p-2 ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-ink-950">{step.label}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      {step.status}
                    </span>
                    {step.isBlocking && step.status !== "Completed" && (
                      <span className="text-[9px] text-red font-medium">Required</span>
                    )}
                  </div>
                  <div className="mt-0.5 text-xs text-ink-500 line-clamp-1">{step.description}</div>
                </div>
                <div className="flex-shrink-0">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      if (step.status !== "Blocked") {
                        router.push(step.path);
                      } else {
                        toast("This step is blocked. Please resolve blockers first.");
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

      {/* ===== SUBMIT ACTION ===== */}
      {!data.isSubmitted && !data.isApproved && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="text-[15px] font-semibold text-ink-950">Ready to Submit?</h3>
              <p className="mt-0.5 text-sm text-ink-600">
                {data.canSubmit
                  ? "All required steps are complete. You can now submit your company for review."
                  : "Complete all required steps before submitting for review."}
              </p>
            </div>
            <button
              onClick={handleSubmit}
              disabled={!data.canSubmit}
              className={`flex h-10 items-center gap-2 rounded-xl px-5 text-sm font-semibold text-white shadow-sm transition ${
                data.canSubmit
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

      {/* ===== SUBMITTED STATE ===== */}
      {data.isSubmitted && !data.isApproved && (
        <div className="rounded-2xl border border-amber/20 bg-amber-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-amber-bg p-2 flex-shrink-0">
              <Clock className="h-5 w-5 text-amber" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-amber">Awaiting Review</h4>
              <p className="mt-1 text-sm text-amber/80">
                Your company has been submitted for review. The verification team will review your application.
              </p>
              <button
                onClick={() => router.push("/setup/verification")}
                className="mt-2 text-sm font-medium text-amber hover:underline flex items-center gap-1"
              >
                Check status <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== APPROVED STATE ===== */}
      {data.isApproved && (
        <div className="rounded-2xl border border-green/20 bg-green-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-green-bg p-2 flex-shrink-0">
              <CheckCircle2 className="h-5 w-5 text-green" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-green">Company Approved! 🎉</h4>
              <p className="mt-1 text-sm text-green/80">
                Your company has been verified and approved. You can now start using the Fleet OS platform.
              </p>
              <button
                onClick={() => router.push("/")}
                className="mt-2 text-sm font-medium text-green hover:underline flex items-center gap-1"
              >
                Go to Dashboard <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Building2 className="h-4 w-4" />
          ORG-01 · Onboarding Home
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.steps.filter((s) => s.status === "Completed").length}/{data.steps.length} complete
        </span>
      </div>
    </div>
  );
}