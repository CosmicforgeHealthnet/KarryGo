// src/app/(setup)/setup/verification/page.tsx
// Screen: ORG06 - Business Verification
// Rebuilt as a real two-column layout, same pass as Wallet Provisioning:
// the items ARE the primary content here (unlike Wallet, which is one
// pipeline), so they take the main column as full cards. The overall
// status becomes a focal sidebar summary with a radial progress ring —
// echoing the ring already used on the onboarding home page, so it reads
// as one product's device rather than a new one invented for this screen.

"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  X,
  ShieldCheck,
  FileCheck,
  ChevronLeft,
  ChevronRight,
  FileWarning,
  Circle,
  Info,
} from "lucide-react";

// ==================== TYPES ====================

type ItemStatus = "Pending" | "InProgress" | "Verified" | "Rejected" | "NotStarted";

type VerificationItem = {
  id: string;
  name: string;
  status: ItemStatus;
  description: string;
  required: boolean;
  updatedAt: string;
  notes: string | null;
};

type VerificationState = {
  status: ItemStatus;
  submittedAt: string | null;
  reviewedAt: string | null;
  reviewer: string | null;
  overallNotes: string | null;
  items: VerificationItem[];
};

// ==================== MOCK DATA ====================

const MOCK_VERIFICATION: VerificationState = {
  status: "Pending",
  submittedAt: "2026-08-22T10:00:00Z",
  reviewedAt: null,
  reviewer: null,
  overallNotes: "Awaiting document verification.",
  items: [
    {
      id: "V-001",
      name: "Business Registration",
      status: "Verified",
      description: "Company registration certificate verification.",
      required: true,
      updatedAt: "2026-08-22T10:30:00Z",
      notes: "Verified against government database.",
    },
    {
      id: "V-002",
      name: "Tax ID Verification",
      status: "Pending",
      description: "Tax identification number verification.",
      required: true,
      updatedAt: "2026-08-22T09:00:00Z",
      notes: "Awaiting verification.",
    },
    {
      id: "V-003",
      name: "Operating License",
      status: "Rejected",
      description: "Business operating license verification.",
      required: true,
      updatedAt: "2026-08-22T08:00:00Z",
      notes: "License number mismatch. Please re-upload document.",
    },
    {
      id: "V-004",
      name: "Insurance Policy",
      status: "NotStarted",
      description: "Business insurance policy verification.",
      required: false,
      updatedAt: "2026-08-21T14:30:00Z",
      notes: "Not yet verified.",
    },
    {
      id: "V-005",
      name: "Director Verification",
      status: "InProgress",
      description: "Director identity and background verification.",
      required: true,
      updatedAt: "2026-08-22T11:00:00Z",
      notes: "Background check in progress.",
    },
  ],
};

const STATUS_STYLES: Record<ItemStatus, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-4 w-4" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-400", icon: <Circle className="h-4 w-4" /> },
};

// ==================== SHARED PRIMITIVE ====================

function IconTile({
  icon,
  theme = "brand",
}: {
  icon: React.ReactNode;
  theme?: "brand" | "accent" | "amber" | "red" | "green";
}) {
  const gradient =
    theme === "brand"
      ? "from-primary to-accent"
      : theme === "accent"
      ? "from-accent to-accent-hover"
      : theme === "amber"
      ? "from-amber to-amber"
      : theme === "red"
      ? "from-red to-red"
      : "from-green to-green";

  return (
    <div
      className={`relative flex h-11 w-11 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br text-white ring-1 ring-white/20 ${gradient}`}
      style={{
        boxShadow:
          "inset 0 1.5px 0 rgba(255,255,255,0.6), inset 0 -9px 14px rgba(0,0,0,0.3), 0 10px 18px -6px rgba(0,0,0,0.4)",
      }}
    >
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background: "radial-gradient(130% 110% at 25% 12%, rgba(255,255,255,0.55), transparent 55%)",
          mixBlendMode: "overlay",
        }}
      />
      <span
        className="relative [&>svg]:h-5 [&>svg]:w-5"
        style={{ filter: "drop-shadow(0 1.5px 1.5px rgba(0,0,0,0.35)) drop-shadow(0 -1px 0 rgba(255,255,255,0.25))" }}
      >
        {icon}
      </span>
    </div>
  );
}

function formatRelative(date: string | null): string {
  if (!date) return "N/A";
  const diffMs = Date.now() - new Date(date).getTime();
  const diffMin = Math.floor(diffMs / 60000);
  const diffHr = Math.floor(diffMs / 3600000);
  const diffDay = Math.floor(diffMs / 86400000);
  if (diffMin < 1) return "Just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  return `${diffDay}d ago`;
}

// ==================== VERIFICATION ITEM CARD ====================

function VerificationItemCard({ item }: { item: VerificationItem }) {
  const style = STATUS_STYLES[item.status];
  const tileTheme =
    item.status === "Verified" ? "green" : item.status === "Rejected" ? "red" : item.status === "Pending" ? "amber" : "brand";

  return (
    <div
      className={`rounded-lg border p-5 transition-colors ${
        item.status === "Rejected" ? "border-red/30 bg-red-bg/10" : "border-line bg-white"
      }`}
    >
      <div className="flex items-start gap-3">
        <IconTile icon={style.icon} theme={tileTheme} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-ink-950">{item.name}</span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
              {item.status === "InProgress" ? "In Progress" : item.status === "NotStarted" ? "Not Started" : item.status}
            </span>
            {item.required && <span className="text-[10px] font-medium text-red">Required</span>}
          </div>
          <p className="mt-0.5 text-xs text-ink-500 sm:text-sm">{item.description}</p>

          {item.notes && item.status === "Rejected" ? (
            <div className="mt-2 flex items-start gap-1.5 rounded-lg bg-red-bg/40 px-3 py-2 text-xs text-red">
              <FileWarning className="h-3.5 w-3.5 flex-shrink-0 mt-[1px]" />
              <span>{item.notes}</span>
            </div>
          ) : item.notes ? (
            <p className="mt-2 text-xs text-ink-400">{item.notes}</p>
          ) : null}

          <p className="mt-2 text-[11px] text-ink-400">Updated {formatRelative(item.updatedAt)}</p>
        </div>
      </div>
    </div>
  );
}

// ==================== MAIN PAGE ====================

export default function VerificationPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<VerificationState | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_VERIFICATION);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleResubmit = async () => {
    setIsSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setIsSubmitting(false);
    toast("Verification resubmitted successfully! (mock)");
    router.push("/setup/verification");
  };

  const handleBack = () => router.push("/setup");
  const handleNext = () => router.push("/setup/wallet-provisioning");

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
        <div className="space-y-2">
          <div className="h-7 w-56 animate-pulse rounded bg-surface-2" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-surface-2" />
        </div>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div className="space-y-4 lg:col-span-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="h-24 animate-pulse rounded-lg bg-surface-2" />
            ))}
          </div>
          <div className="h-64 animate-pulse rounded-2xl bg-surface-2" />
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <ShieldCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No verification data found</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load verification status.</p>
        <button
          onClick={() => router.push("/setup")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Setup
        </button>
      </div>
    );
  }

  const isComplete = data.status === "Verified";
  const isRejected = data.status === "Rejected";
  const verifiedCount = data.items.filter((i) => i.status === "Verified").length;
  const totalCount = data.items.length;
  const progressPct = Math.round((verifiedCount / totalCount) * 100);

  const ringColor = isComplete ? "#139a5d" : isRejected ? "#df2f3f" : "#d97706";
  const ringColorLight = isComplete ? "#4ade80" : isRejected ? "#f87171" : "#fbbf24";
  const statusLabel = isComplete ? "Verified" : isRejected ? "Action Needed" : data.status === "InProgress" ? "In Review" : "Pending Review";
  const statusLabelColor = isComplete ? "text-green" : isRejected ? "text-red" : "text-amber";

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      {/* ===== HEADER ===== */}
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Business Verification</span>
        </p>
        <h1 className="mt-1 text-ink-950">Business Verification</h1>
        <p className="mt-1.5 text-ink-600">Track the verification status of your company and documents.</p>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* ===== MAIN: VERIFICATION ITEMS AS CARDS ===== */}
        <div className="space-y-4 lg:col-span-2">
          {data.items.map((item) => (
            <VerificationItemCard key={item.id} item={item} />
          ))}
        </div>

        {/* ===== SIDEBAR: FOCAL STATUS SUMMARY WITH RADIAL RING ===== */}
        <div className="space-y-4 lg:sticky lg:top-24 lg:self-start">
          <div className="rounded-lg border border-line bg-white p-6 text-center">
            <div className="relative mx-auto h-28 w-28">
              <svg className="h-28 w-28 -rotate-90 transform" viewBox="0 0 80 80">
                <defs>
                  <linearGradient id="verification-ring-gradient" x1="0%" y1="0%" x2="100%" y2="100%">
                    <stop offset="0%" stopColor={ringColorLight} />
                    <stop offset="100%" stopColor={ringColor} />
                  </linearGradient>
                </defs>
                <circle cx="40" cy="40" r="32" fill="none" stroke="#E5E7EB" strokeWidth="6" />
                <circle
                  cx="40"
                  cy="40"
                  r="32"
                  fill="none"
                  stroke="url(#verification-ring-gradient)"
                  strokeWidth="6"
                  strokeDasharray={`${progressPct * 2.01} 201.06`}
                  strokeLinecap="round"
                  className="transition-[stroke-dasharray] duration-700 ease-out"
                />
              </svg>
              <span className="absolute inset-0 flex flex-col items-center justify-center">
                <span className="text-xl font-bold text-ink-950">
                  {verifiedCount}/{totalCount}
                </span>
                <span className="text-[10px] text-ink-500">verified</span>
              </span>
            </div>

            <h2 className={`mt-4 text-lg font-bold ${statusLabelColor}`}>{statusLabel}</h2>
            {data.overallNotes && <p className="mt-1.5 text-xs text-ink-600">{data.overallNotes}</p>}

            <div className="mt-4 space-y-1.5 border-t border-line pt-4 text-left text-xs text-ink-500">
              {data.submittedAt && (
                <div className="flex items-center justify-between">
                  <span>Submitted</span>
                  <span className="font-medium text-ink-700">{formatRelative(data.submittedAt)}</span>
                </div>
              )}
              {data.reviewedAt && (
                <div className="flex items-center justify-between">
                  <span>Reviewed</span>
                  <span className="font-medium text-ink-700">
                    {formatRelative(data.reviewedAt)}
                    {data.reviewer ? ` · ${data.reviewer}` : ""}
                  </span>
                </div>
              )}
            </div>
          </div>

          {!isComplete && !isRejected && (
            <div className="flex items-start gap-2 rounded-lg border border-line bg-surface-2/40 p-3.5">
              <Info className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-ink-400" />
              <p className="text-xs text-ink-500">
                Our team is reviewing your submission. This page updates automatically as items are verified.
              </p>
            </div>
          )}
        </div>
      </div>

      {/* ===== NAVIGATION ===== */}
      <div className="flex flex-col-reverse items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 hover:bg-surface-2"
        >
          <ChevronLeft className="h-4 w-4" />
          Back
        </button>

        {isRejected && (
          <button
            onClick={handleResubmit}
            disabled={isSubmitting}
            className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
              isSubmitting ? "cursor-not-allowed bg-ink-400" : "bg-primary hover:bg-primary-hover"
            }`}
          >
            {isSubmitting ? (
              <>
                <RefreshCw className="h-4 w-4 animate-spin" />
                Resubmitting...
              </>
            ) : (
              <>
                <FileCheck className="h-4 w-4" />
                Resubmit for Review
              </>
            )}
          </button>
        )}

        {isComplete && (
          <button
            onClick={handleNext}
            className="inline-flex items-center justify-center gap-2 rounded-lg bg-green px-6 py-2 text-sm font-medium text-white transition hover:bg-green/90"
          >
            Continue
            <ChevronRight className="h-4 w-4" />
          </button>
        )}

        {!isRejected && !isComplete && (
          <span className="inline-flex items-center gap-1.5 text-xs text-ink-500">
            <Clock className="h-3.5 w-3.5" />
            Waiting on document review
          </span>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <ShieldCheck className="h-4 w-4" />
          ORG-06 · Business Verification
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}