// src/app/(setup)/setup/wallet-provisioning/page.tsx
// Screen: ORG07 - Wallet Provisioning
// Stage tracker restyled to match the "Assessment Journey" reference:
// badge above the active/completed node, larger circular nodes, a
// progress line that fills as stages complete (dashed ahead of the
// active stage), and a number + title + caption under each node.

"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  X,
  Wallet,
  ChevronLeft,
  ChevronRight,
  AlertTriangle,
  Check,
} from "lucide-react";

// ==================== TYPES ====================

type WalletStatus = "Pending" | "InProgress" | "Provisioned" | "Failed" | "NotStarted";

type WalletProvisioningState = {
  status: WalletStatus;
  startedAt: string | null;
  completedAt: string | null;
  retryCount: number;
  lastRetryAt: string | null;
  errorMessage: string | null;
  notes: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_WALLET_STATE: WalletProvisioningState = {
  status: "InProgress",
  startedAt: "2026-08-22T10:00:00Z",
  completedAt: null,
  retryCount: 0,
  lastRetryAt: null,
  errorMessage: null,
  notes: "Wallet provisioning in progress. This may take a few moments.",
};

const BENEFITS = [
  "Receive and send payments",
  "Manage provider sub-wallets",
  "Process allocations and withdrawals",
];

// Real pipeline stages, each with a caption for the tracker.
const STAGES = [
  { label: "Requested", caption: "Wallet setup submitted for provisioning", icon: Clock },
  { label: "Processing", caption: "Company wallet is being provisioned", icon: RefreshCw },
  { label: "Provisioned", caption: "Wallet is ready to use", icon: Wallet },
] as const;

function stageIndexFor(status: WalletStatus): number {
  switch (status) {
    case "NotStarted":
      return -1;
    case "Pending":
      return 0;
    case "InProgress":
      return 1;
    case "Provisioned":
      return 2;
    case "Failed":
      return 1; // failed mid-processing
  }
}

// ==================== STAGE TRACKER (signature element) ====================

function StageTracker({ status }: { status: WalletStatus }) {
  const activeIndex = stageIndexFor(status);
  const isFailed = status === "Failed";
  const progressPct = Math.round(((Math.max(activeIndex, 0) + (status === "Provisioned" ? 1 : 0)) / STAGES.length) * 100);

  return (
    <div>
      {/* Step counter + progress, mirrors the top-right badge in the reference */}
      <div className="mb-8 flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-semibold text-ink-950">
            Step {Math.min(activeIndex + 1, STAGES.length)} of {STAGES.length}
          </p>
          <p className="text-xs text-ink-500">
            {isFailed ? "Provisioning hit a snag" : status === "Provisioned" ? "All set" : "Setting up your wallet"}
          </p>
        </div>
        <div className="relative flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full border border-line bg-white">
          <svg viewBox="0 0 36 36" className="absolute h-full w-full -rotate-90">
            <circle cx="18" cy="18" r="15.5" fill="none" stroke="var(--line)" strokeWidth="3" />
            <circle
              cx="18"
              cy="18"
              r="15.5"
              fill="none"
              stroke={isFailed ? "var(--red)" : "var(--primary)"}
              strokeWidth="3"
              strokeLinecap="round"
              strokeDasharray={`${(progressPct / 100) * 97.4} 97.4`}
            />
          </svg>
          <span className="text-[10px] font-semibold text-ink-950">{progressPct}%</span>
        </div>
      </div>

      {/* Nodes — horizontally scrollable on narrow viewports instead of squishing */}
      <div className="-mx-1 overflow-x-auto pb-1 sm:overflow-visible">
        <div className="flex items-start px-1">
        {STAGES.map((stage, i) => {
          const isDone = i < activeIndex || (i === activeIndex && status === "Provisioned");
          const isFailedHere = isFailed && i === activeIndex;
          const isActive = i === activeIndex && !isDone && !isFailedHere;
          const StageIcon = stage.icon;

          // Segment i connects node i to node i+1: solid green once both
          // sides are done, a green→primary/red gradient curve at the
          // handoff into the active/failed node, dashed gray otherwise.
          const nextIsFailed = isFailed && i + 1 === activeIndex;
          const nextIsDone = i + 1 < activeIndex || (i + 1 === activeIndex && status === "Provisioned");
          const nextIsActive = i + 1 === activeIndex && !nextIsDone && !nextIsFailed;
          const isBridge = isDone && (nextIsActive || nextIsFailed);
          const isSolid = isDone && nextIsDone;
          const gradId = `seg-grad-${i}`;

          return (
            <div key={stage.label} className="contents">
              <div className={`flex w-28 flex-shrink-0 flex-col items-center sm:w-auto sm:flex-1 ${i === STAGES.length - 1 ? "sm:flex-none" : ""}`}>
                {/* Badge */}
                <div className="mb-2 h-5">
                  {isDone && !isFailedHere && (
                    <span className="whitespace-nowrap rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
                      Completed
                    </span>
                  )}
                  {isActive && (
                    <span className="whitespace-nowrap rounded-full bg-primary px-2.5 py-0.5 text-[10px] font-semibold text-white">
                      Current Step
                    </span>
                  )}
                  {isFailedHere && (
                    <span className="whitespace-nowrap rounded-full bg-red-bg px-2.5 py-0.5 text-[10px] font-semibold text-red">
                      Failed
                    </span>
                  )}
                </div>

                {/* Node */}
                <div className="relative">
                  {isActive && (
                    <span className="absolute inset-0 -m-1.5 rounded-full border-2 border-primary/25 animate-pulse" aria-hidden />
                  )}
                  <div
                    className={`relative flex h-14 w-14 flex-shrink-0 items-center justify-center rounded-full text-sm font-semibold ${
                      isDone
                        ? "bg-gradient-to-br from-green to-green/80 text-white"
                        : isFailedHere
                        ? "bg-gradient-to-br from-red to-red/80 text-white"
                        : isActive
                        ? "bg-gradient-to-br from-primary to-accent text-white"
                        : "border-2 border-line bg-white text-ink-300"
                    }`}
                    style={
                      isDone || isFailedHere || isActive
                        ? {
                            boxShadow:
                              "inset 0 1.5px 0 rgba(255,255,255,0.55), inset 0 -7px 11px rgba(0,0,0,0.28), 0 8px 14px -6px rgba(0,0,0,0.35)",
                          }
                        : undefined
                    }
                  >
                    {isDone ? (
                      <Check className="h-6 w-6" />
                    ) : isFailedHere ? (
                      <X className="h-6 w-6" />
                    ) : (
                      <StageIcon className={`h-5 w-5 ${isActive ? "[animation:spin_2s_linear_infinite]" : ""}`} />
                    )}
                  </div>
                </div>

                {/* Number */}
                <span className={`mt-3 h-1.5 w-1.5 rounded-full ${isDone ? "bg-green" : isFailedHere ? "bg-red" : isActive ? "bg-primary" : "bg-line"}`} />
                <span className={`mt-2 text-xs font-bold ${isDone ? "text-green" : isFailedHere ? "text-red" : isActive ? "text-primary" : "text-ink-300"}`}>
                  {String(i + 1).padStart(2, "0")}
                </span>

                {/* Title + caption */}
                <p className="mt-1 text-center text-[13px] font-semibold text-ink-950">{stage.label}</p>
                <p className="mt-0.5 hidden max-w-[9rem] text-center text-[11px] leading-snug text-ink-500 sm:block">{stage.caption}</p>
              </div>

              {i < STAGES.length - 1 && (
                <div className="relative mt-9 h-8 w-10 flex-shrink-0 sm:w-auto sm:flex-1">
                  <svg viewBox="0 0 100 32" preserveAspectRatio="none" className="h-full w-full">
                    {isBridge && (
                      <defs>
                        <linearGradient id={gradId} x1="0%" y1="0%" x2="100%" y2="0%">
                          <stop offset="0%" stopColor="var(--green)" />
                          <stop offset="100%" stopColor={nextIsFailed ? "var(--red)" : "var(--primary)"} />
                        </linearGradient>
                      </defs>
                    )}
                    <path
                      d="M0,16 C25,2 75,30 100,16"
                      fill="none"
                      stroke={isBridge ? `url(#${gradId})` : isSolid ? "var(--green)" : "var(--line)"}
                      strokeWidth={isSolid || isBridge ? 3 : 2.5}
                      strokeLinecap="round"
                      strokeDasharray={isSolid || isBridge ? undefined : "6 6"}
                    />
                  </svg>
                </div>
              )}
            </div>
          );
        })}
        </div>
      </div>
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

// ==================== MAIN PAGE ====================

export default function WalletProvisioningPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WalletProvisioningState | null>(null);
  const [isRetrying, setIsRetrying] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_WALLET_STATE);
      setLoading(false);
    };
    loadData();
  }, []);

  const handleRetry = async () => {
    setIsRetrying(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setIsRetrying(false);
    toast("Wallet provisioning retry requested (mock)");
  };

  const handleBack = () => router.push("/setup");
  const handleNext = () => router.push("/setup/submit");

  if (loading) {
    return (
      <div className="w-full space-y-6 sm:space-y-8">
        <div className="space-y-2">
          <div className="h-7 w-56 animate-pulse rounded bg-surface-2" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-surface-2" />
        </div>
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
          <div className="h-80 animate-pulse rounded-2xl bg-surface-2 xl:col-span-2" />
          <div className="h-80 animate-pulse rounded-2xl bg-surface-2" />
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Wallet className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No wallet data found</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load wallet provisioning status.</p>
        <button
          onClick={() => router.push("/setup")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Setup
        </button>
      </div>
    );
  }

  const isComplete = data.status === "Provisioned";
  const isFailed = data.status === "Failed";
  const hasMeta = Boolean(data.startedAt || data.completedAt || data.retryCount > 0 || data.errorMessage);

  return (
    <div className="w-full space-y-6 sm:space-y-8">
      {/* ===== HEADER ===== */}
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Wallet Provisioning</span>
        </p>
        <h1 className="mt-1 text-ink-950">Wallet Provisioning</h1>
        <p className="mt-1.5 text-ink-600">Company wallet setup and provisioning status.</p>
      </div>

      {/* xl, not lg: the setup sidebar already claims 18rem at lg, so the
          grid needs more viewport before splitting into two columns or the
          status card and sidebar get squeezed. */}
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        {/* ===== MAIN: STAGE TRACKER + FACTS STRIP ===== */}
        <div className="space-y-6 xl:col-span-2">
          <div className="rounded-lg border border-line bg-white p-6">
            <StageTracker status={data.status} />
          </div>

          {hasMeta && (
            <div className="rounded-lg border border-line bg-white p-5">
              <div className="flex flex-wrap items-center gap-2">
                {data.startedAt && (
                  <span className="rounded-full bg-surface-2 px-3 py-1 text-xs text-ink-600">
                    Started <span className="font-medium text-ink-950">{formatRelative(data.startedAt)}</span>
                  </span>
                )}
                {data.completedAt && (
                  <span className="rounded-full bg-surface-2 px-3 py-1 text-xs text-ink-600">
                    Completed <span className="font-medium text-ink-950">{formatRelative(data.completedAt)}</span>
                  </span>
                )}
                {data.retryCount > 0 && (
                  <span className="rounded-full bg-surface-2 px-3 py-1 text-xs text-ink-600">
                    Retries <span className="font-medium text-ink-950">{data.retryCount}</span>
                  </span>
                )}
              </div>

              {data.errorMessage && (
                <div className="mt-3 w-full rounded-r-lg border-l-4 border-red bg-red-bg/40 px-4 py-3 text-sm text-red">
                  {data.errorMessage}
                </div>
              )}
            </div>
          )}
        </div>

        {/* ===== SIDEBAR: SUPPORTING INFO (quiet, deliberately secondary) ===== */}
        <div className="space-y-4">
          <div className="rounded-lg border border-line bg-white p-5">
            <h3 className="text-sm font-semibold text-ink-950">What this unlocks</h3>
            <ul className="mt-3.5 space-y-2.5">
              {BENEFITS.map((benefit) => (
                <li key={benefit} className="flex items-start gap-2 text-sm text-ink-600">
                  <span className="mt-1.5 h-1 w-1 flex-shrink-0 rounded-full bg-ink-400" />
                  <span>{benefit}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="flex items-start gap-2 rounded-lg border border-amber/20 bg-amber-bg/40 p-4">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-amber" />
            <p className="text-xs text-amber">
              Wallet balance is never fabricated here — only authoritative balances from Payment Wallet are
              displayed, once provisioning finishes.
            </p>
          </div>
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

        {isFailed && (
          <button
            onClick={handleRetry}
            disabled={isRetrying}
            className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
              isRetrying ? "cursor-not-allowed bg-ink-400" : "bg-primary hover:bg-primary-hover"
            }`}
          >
            {isRetrying ? (
              <>
                <RefreshCw className="h-4 w-4 animate-spin" />
                Retrying...
              </>
            ) : (
              <>
                <RefreshCw className="h-4 w-4" />
                Retry Provisioning
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
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}