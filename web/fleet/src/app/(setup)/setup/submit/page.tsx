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
} from "lucide-react";

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
  checklist: ChecklistItem[];
  submissionDate: string | null;
  reviewDate: string | null;
  reviewerNotes: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_SUBMIT_STATE: SubmitState = {
  canSubmit: false,
  isSubmitted: false,
  isApproved: false,
  submissionDate: null,
  reviewDate: null,
  reviewerNotes: null,
  checklist: [
    { id: "C-001", label: "Company created", complete: true, required: true, status: "Complete", notes: null },
    { id: "C-002", label: "Company profile filled", complete: false, required: true, status: "Incomplete", notes: "Legal name and operating contact required" },
    { id: "C-003", label: "Capabilities configured", complete: false, required: false, status: "Pending", notes: "Optional" },
    { id: "C-004", label: "Business documents uploaded", complete: false, required: true, status: "Blocked", notes: "Re-upload operating license" },
    { id: "C-005", label: "Verification passed", complete: false, required: true, status: "Pending", notes: "Awaiting verification" },
    { id: "C-006", label: "Wallet provisioned", complete: false, required: true, status: "Pending", notes: "In progress" },
  ],
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

export default function SubmitForReviewPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<SubmitState | null>(null);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_SUBMIT_STATE);
      setLoading(false);
    };
    loadData();
  }, []);

  const handleSubmit = async () => {
    if (!data?.canSubmit) return;

    setIsSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 2000));
    setIsSubmitting(false);

    setData((prev) => prev ? { ...prev, isSubmitted: true, submissionDate: new Date().toISOString() } : null);
    toast("Company submitted for review successfully! (mock)");
  };

  const handleBack = () => {
    router.push("/setup");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading submission status...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No submission data found</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load submission status.</p>
        <button
          onClick={() => router.push("/setup")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Setup
        </button>
      </div>
    );
  }

  const isComplete = data.isApproved;
  const isSubmitted = data.isSubmitted;

  // Derived from the checklist itself rather than tracked separately, so
  // the two can't drift: a required, incomplete item is a blocker by
  // definition, and its own note is the reason.
  const blockers = data.checklist
    .filter((item) => item.required && !item.complete)
    .map((item) => item.notes || item.label);

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

      {/* ===== SUBMITTED/APPROVED STATE ===== */}
      {isSubmitted && !isComplete && (
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
              {data.submissionDate && (
                <p className="mt-1 text-xs text-amber/60">
                  Submitted on {formatDate(data.submissionDate)}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

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
              {data.reviewDate && (
                <p className="mt-1 text-xs text-green/60">
                  Reviewed on {formatDate(data.reviewDate)}
                </p>
              )}
              {data.reviewerNotes && (
                <p className="mt-1 text-xs text-green/60">{data.reviewerNotes}</p>
              )}
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
              {data.checklist.filter((i) => i.complete).length}/{data.checklist.length} complete
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {data.checklist.map((item) => {
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
                    <span className={`font-medium ${
                      item.complete ? "text-ink-950" : "text-ink-700"
                    }`}>
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
      {blockers.length > 0 && !data.isSubmitted && (
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
              disabled={!data.canSubmit || isSubmitting}
              className={`flex h-10 items-center gap-2 rounded-xl px-5 text-sm font-semibold text-white shadow-sm transition ${
                data.canSubmit && !isSubmitting
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
        className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
      >
        Back to Setup
      </button>

      {/* ===== FOOTER ===== */}
      <div className="flex items-center gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <FileCheck className="h-4 w-4" />
        ORG-08 · Submit for Review
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}