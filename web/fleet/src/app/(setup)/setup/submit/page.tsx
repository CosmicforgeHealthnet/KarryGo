// src/app/(setup)/setup/submit/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertOctagon,
  FileCheck,
  ListChecks,
  Send,
  ArrowLeft,
} from "lucide-react";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";

// ==================== TYPES ====================

type ChecklistItem = {
  id: string;
  label: string;
  complete: boolean;
  required: boolean;
  status: "Complete" | "Incomplete" | "Pending" | "Blocked";
  notes: string | null;
};

type SubmitState = {
  canSubmit: boolean;
  isSubmitted: boolean;
  isApproved: boolean;
  isRejected: boolean;
  checklist: ChecklistItem[];
  submissionDate: string | null;
  reviewDate: string | null;
  reviewerNotes: string | null;
  loading: boolean;
  error: string | null;
};

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Complete: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Incomplete: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Blocked: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
};

// Map backend stages to checklist items
const STAGE_TO_CHECKLIST: Record<string, { id: string; label: string; required: boolean }> = {
  OWNER_CONTACT: { id: "C-001", label: "Company created", required: true },
  BUSINESS_PROFILE: { id: "C-002", label: "Company profile filled", required: true },
  AUTHORIZED_REPRESENTATIVE: { id: "C-003", label: "Authorized representative added", required: true },
  OPERATING_CAPABILITIES: { id: "C-004", label: "Operating capabilities configured", required: true },
  HEADQUARTERS: { id: "C-005", label: "Headquarters address set", required: true },
  BUSINESS_DOCUMENTS: { id: "C-006", label: "Business documents uploaded", required: true },
};

// Stages required for submission (matching backend)
const REQUIRED_STAGES = [
  "OWNER_CONTACT",
  "BUSINESS_PROFILE",
  "AUTHORIZED_REPRESENTATIVE",
  "OPERATING_CAPABILITIES",
  "HEADQUARTERS",
  "BUSINESS_DOCUMENTS",
];

const VERIFICATION_STATUS_MAP: Record<string, { label: string; isSubmitted: boolean; isApproved: boolean; isRejected: boolean }> = {
  SUBMITTED: { label: "Awaiting Review", isSubmitted: true, isApproved: false, isRejected: false },
  UNDER_REVIEW: { label: "Under Review", isSubmitted: true, isApproved: false, isRejected: false },
  APPROVED: { label: "Approved", isSubmitted: true, isApproved: true, isRejected: false },
  REJECTED: { label: "Rejected", isSubmitted: true, isApproved: false, isRejected: true },
  CHANGES_REQUIRED: { label: "Changes Required", isSubmitted: true, isApproved: false, isRejected: true },
};

export default function SubmitForReviewPage() {
  const router = useRouter();
  const { organization } = useOrganizationStore();
  const { setStageStatuses } = useOnboardingStore();

  const [state, setState] = useState<SubmitState>({
    canSubmit: false,
    isSubmitted: false,
    isApproved: false,
    isRejected: false,
    checklist: [],
    submissionDate: null,
    reviewDate: null,
    reviewerNotes: null,
    loading: true,
    error: null,
  });

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitSuccess, setSubmitSuccess] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Fetch real onboarding status
  useEffect(() => {
    async function fetchOnboardingStatus() {
      if (!organization?.organizationId) {
        setState((prev) => ({ ...prev, loading: false, error: "No organization found." }));
        return;
      }

      try {
        const response = await organizationService.getOnboardingStatus(organization.organizationId);

        // Build checklist from stages
        const checklist: ChecklistItem[] = REQUIRED_STAGES.map((stageName) => {
          const stage = response.stages.find((s) => s.stage === stageName);
          const meta = STAGE_TO_CHECKLIST[stageName];

          const isComplete = stage?.status === "COMPLETE";
          const isBlocked = stage?.status === "BLOCKED";
          const isInProgress = stage?.status === "IN_PROGRESS";
          const isNotStarted = stage?.status === "NOT_STARTED" || !stage;

          let status: "Complete" | "Incomplete" | "Pending" | "Blocked" = "Pending";
          let notes: string | null = null;

          if (isComplete) {
            status = "Complete";
          } else if (isBlocked) {
            status = "Blocked";
            notes = stage?.blockingIssues?.join(", ") || "Blocked";
          } else if (isInProgress) {
            status = "Pending";
            notes = "In progress";
          } else if (isNotStarted) {
            status = "Incomplete";
            notes = "Not started";
          }

          return {
            id: meta?.id || `C-${stageName}`,
            label: meta?.label || stageName.replace(/_/g, " "),
            complete: isComplete,
            required: meta?.required ?? true,
            status,
            notes,
          };
        });

        // Check verification status
        const verificationStatus = response.verification?.status || "NOT_SUBMITTED";
        const verificationMap = VERIFICATION_STATUS_MAP[verificationStatus] || {
          label: "Not Submitted",
          isSubmitted: false,
          isApproved: false,
          isRejected: false,
        };

        // Check if all required stages are complete
        const allRequiredComplete = REQUIRED_STAGES.every((stageName) => {
          const stage = response.stages.find((s) => s.stage === stageName);
          return stage?.status === "COMPLETE";
        });

        // Check allowedNextActions
        const canSubmit = response.allowedNextActions?.includes("SUBMIT_FOR_VERIFICATION") ?? false;

        // Determine if already submitted
        const isSubmitted = verificationMap.isSubmitted || response.organizationStatus === "BUSINESS_SUBMITTED";
        const isApproved = verificationMap.isApproved || response.organizationStatus === "APPROVED" || response.organizationStatus === "ACTIVE";
        const isRejected = verificationMap.isRejected || response.organizationStatus === "REJECTED";

        // Update store
        const statusMap: Record<string, string> = {};
        response.stages.forEach((stage: any) => {
          const stepId = STAGE_TO_CHECKLIST[stage.stage]?.id;
          if (stepId) {
            statusMap[stepId] = stage.status;
          }
        });
        setStageStatuses(statusMap);

        setState({
          canSubmit: canSubmit && !isSubmitted && !isApproved,
          isSubmitted,
          isApproved,
          isRejected,
          checklist,
          submissionDate: response.verification?.submissionVersion ? new Date().toISOString() : null,
          reviewDate: null,
        reviewerNotes: (response.verification?.issues?.length ?? 0) > 0
  ? `${response.verification?.issues?.length ?? 0} issue(s) need attention`
  : null,
          loading: false,
          error: null,
        });

      } catch (err: any) {
        const message = err?.response?.data?.error?.message || err?.message || "Failed to load submission status.";
        setState((prev) => ({ ...prev, loading: false, error: message }));
      }
    }

    fetchOnboardingStatus();
  }, [organization?.organizationId, setStageStatuses]);

  // Handle actual submission
  const handleSubmit = async () => {
    if (!state.canSubmit || !organization?.organizationId) return;

    setIsSubmitting(true);
    setSubmitError(null);
    setSubmitSuccess(false);

    try {
      const response = await organizationService.submitOrganization(organization.organizationId);

      // Update state after successful submission
      setState((prev) => ({
        ...prev,
        isSubmitted: true,
        canSubmit: false,
        submissionDate: new Date().toISOString(),
      }));

      setSubmitSuccess(true);

      // Show success message
      toast("Company submitted for review successfully! 🎉");

      // Redirect to homepage after 2 seconds
      setTimeout(() => {
        router.push("/");
      }, 2000);

    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to submit. Please try again.";
      setSubmitError(message);
      toast(`Error: ${message}`);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleBack = () => {
    router.push("/setup");
  };

  // Loading state
  if (state.loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading submission status...</p>
      </div>
    );
  }

  // Error state
  if (state.error) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-red/20 bg-red-bg/30 py-14 px-4">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-4 text-[15px] font-semibold text-ink-950">Something went wrong</h3>
        <p className="mt-1.5 text-sm text-ink-500 text-center max-w-md">{state.error}</p>
        <button
          onClick={() => window.location.reload()}
          className="mt-5 rounded-lg bg-primary px-5 py-2 text-[13px] font-semibold text-white transition hover:bg-primary/90"
        >
          Try again
        </button>
      </div>
    );
  }

  // No organization
  if (!organization) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No organization found</h3>
        <p className="mt-1 text-sm text-ink-500">Please create a company first.</p>
        <button
          onClick={() => router.push("/setup/create-company")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Create Company
        </button>
      </div>
    );
  }

  const isComplete = state.isApproved;
  const isSubmitted = state.isSubmitted;
  const isRejected = state.isRejected;

  // Get blockers from checklist
  const blockers = state.checklist
    .filter((item) => item.required && !item.complete)
    .map((item) => item.notes || item.label);

  const completedCount = state.checklist.filter((i) => i.complete).length;
  const totalItems = state.checklist.length;

  const formatDate = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  return (
    <div className="w-full space-y-6">
      {/* ===== HEADER ===== */}
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Submit for Review</span>
        </p>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Submit for Review
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Review your readiness checklist and submit your company for verification.
        </p>
      </div>

      {/* ===== SUCCESS MESSAGE ===== */}
      {submitSuccess && (
        <div className="rounded-2xl border border-green/30 bg-green-bg/40 p-5 shadow-sm animate-in fade-in slide-in-from-top-2 duration-300">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-green-bg p-2 flex-shrink-0">
              <CheckCircle2 className="h-5 w-5 text-green" />
            </div>
            <div className="flex-1">
              <h4 className="text-sm font-semibold text-green">Submitted Successfully! 🎉</h4>
              <p className="mt-1 text-sm text-green/80">
                Your company has been submitted for review. You will be redirected to the dashboard shortly.
              </p>
              {state.submissionDate && (
                <p className="mt-1 text-xs text-green/60">
                  Submitted on {formatDate(state.submissionDate)}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ===== SUBMITTED STATE ===== */}
      {isSubmitted && !isComplete && !isRejected && !submitSuccess && (
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
              {state.submissionDate && (
                <p className="mt-1 text-xs text-amber/60">
                  Submitted on {formatDate(state.submissionDate)}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ===== REJECTED STATE ===== */}
      {isRejected && (
        <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red-bg p-2 flex-shrink-0">
              <AlertOctagon className="h-5 w-5 text-red" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-red">Changes Required</h4>
              <p className="mt-1 text-sm text-red/80">
                Your submission needs attention. Please address the issues below and re-submit.
              </p>
              {state.reviewerNotes && (
                <p className="mt-1 text-xs text-red/60">{state.reviewerNotes}</p>
              )}
              <button
                onClick={() => router.push("/setup")}
                className="mt-3 rounded-lg bg-red px-4 py-1.5 text-xs font-semibold text-white transition hover:bg-red/90"
              >
                Fix Issues
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== APPROVED STATE ===== */}
      {isComplete && (
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
                className="mt-3 rounded-lg bg-green px-4 py-1.5 text-xs font-semibold text-white transition hover:bg-green/90"
              >
                Go to Dashboard
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== CHECKLIST ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <ListChecks className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Readiness Checklist</span>
            <span className="text-xs text-ink-400">
              {completedCount}/{totalItems} complete
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {state.checklist.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;

            return (
              <div key={item.id} className="flex items-center gap-4 p-4">
                <div className="flex-shrink-0">
                  <div className={`rounded-full p-2 ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={`font-medium ${item.complete ? "text-ink-950" : "text-ink-700"}`}>
                      {item.label}
                    </span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      {item.status}
                    </span>
                    {item.required && (
                      <span className="text-[9px] text-red font-medium">Required</span>
                    )}
                  </div>
                  {item.notes && (
                    <div className="mt-0.5 text-xs text-ink-500">{item.notes}</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== BLOCKERS ===== */}
      {blockers.length > 0 && !isSubmitted && !isComplete && !submitSuccess && (
        <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red-bg p-2 flex-shrink-0">
              <AlertOctagon className="h-5 w-5 text-red" />
            </div>
            <div className="flex-1">
              <h4 className="text-sm font-semibold text-red">Blockers</h4>
              <ul className="mt-2 space-y-1">
                {blockers.map((blocker, index) => (
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

      {/* ===== SUBMIT ERROR ===== */}
      {submitError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {submitError}
        </div>
      )}

      {/* ===== SUBMIT ACTION ===== */}
      {!isSubmitted && !isComplete && !submitSuccess && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="text-[15px] font-semibold text-ink-950">Ready to Submit?</h3>
              <p className="mt-0.5 text-sm text-ink-600">
                {state.canSubmit
                  ? "All required steps are complete. You can now submit your company for review."
                  : "Complete all required steps before submitting for review."}
              </p>
            </div>
            <button
              onClick={handleSubmit}
              disabled={!state.canSubmit || isSubmitting}
              className={`flex h-10 items-center gap-2 rounded-xl px-5 text-sm font-semibold text-white shadow-sm transition ${
                state.canSubmit && !isSubmitting
                  ? "bg-primary shadow-primary/20 hover:bg-primary/90"
                  : "bg-ink-400 cursor-not-allowed"
              }`}
            >
              {isSubmitting ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Submitting...
                </>
              ) : (
                <>
                  <Send className="h-4 w-4" />
                  Submit for Review
                </>
              )}
            </button>
          </div>
        </div>
      )}

      {/* ===== BACK ===== */}
      <button
        onClick={handleBack}
        className="inline-flex items-center gap-2 rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Setup
      </button>

      {/* ===== FOOTER ===== */}
      <div className="flex items-center gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <FileCheck className="h-4 w-4" />
        {organization?.organizationId ? `Organization: ${organization.organizationId}` : "Submit for Review"}
      </div>
    </div>
  );
}

// Simple toast function
function toast(message: string) {
  // You can replace this with a proper toast library like sonner or react-hot-toast
  console.log("[Toast]", message);
  
  // Simple browser alert as fallback
  if (typeof window !== 'undefined') {
    // Create a temporary toast element
    const toast = document.createElement('div');
    toast.className = 'fixed bottom-4 right-4 z-50 rounded-xl bg-ink-950 px-6 py-3 text-sm text-white shadow-lg animate-in slide-in-from-bottom-4 duration-300';
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(() => {
      toast.classList.add('animate-out', 'slide-out-to-bottom-4', 'duration-300');
      setTimeout(() => toast.remove(), 300);
    }, 4000);
  }
}