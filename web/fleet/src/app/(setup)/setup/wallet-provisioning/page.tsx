// src/app/(setup)/setup/wallet-provisioning/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  X,
  Wallet,
  ArrowRight,
} from "lucide-react";

// ==================== TYPES ====================

type WalletProvisioningState = {
  status: "Pending" | "InProgress" | "Provisioned" | "Failed" | "NotStarted";
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

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-5 w-5" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-5 w-5" /> },
  Provisioned: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-5 w-5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-5 w-5" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-5 w-5" /> },
};

export default function WalletProvisioningPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<WalletProvisioningState | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isRetrying, setIsRetrying] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_WALLET_STATE);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleRetry = async () => {
    setIsRetrying(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setIsRetrying(false);
    toast("Wallet provisioning retry requested (mock)");
  };

  const handleBack = () => {
    router.push("/setup");
  };

  const handleNext = () => {
    router.push("/setup/submit");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading wallet status...</p>
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

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.NotStarted;
  const isComplete = data.status === "Provisioned";
  const isFailed = data.status === "Failed";

  const formatTime = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button onClick={handleBack} className="hover:text-primary transition-colors">
            Onboarding
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Wallet Provisioning</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Wallet Provisioning
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Company wallet setup and provisioning status.
        </p>
      </div>

      {/* ===== STATUS CARD ===== */}
      <div className={`rounded-2xl border p-6 shadow-sm ${
        isComplete ? "border-green/20 bg-green-bg/30" :
        isFailed ? "border-red/20 bg-red-bg/30" :
        "border-blue/20 bg-blue-bg/30"
      }`}>
        <div className="flex items-start gap-4">
          <div className={`rounded-full p-3 flex-shrink-0 ${
            isComplete ? "bg-green-bg" :
            isFailed ? "bg-red-bg" :
            "bg-blue-bg"
          }`}>
            {statusStyle.icon}
          </div>
          <div className="flex-1">
            <div className="flex items-center gap-3">
              <span className={`text-lg font-semibold ${
                isComplete ? "text-green" :
                isFailed ? "text-red" :
                "text-blue"
              }`}>
                {data.status}
              </span>
              {data.startedAt && (
                <span className="text-xs text-ink-500">
                  Started {formatTime(data.startedAt)}
                </span>
              )}
            </div>
            {data.notes && (
              <p className="mt-1 text-sm text-ink-600">{data.notes}</p>
            )}
            {data.completedAt && (
              <p className="mt-1 text-xs text-ink-500">
                Completed {formatTime(data.completedAt)}
              </p>
            )}
            {data.errorMessage && (
              <div className="mt-2 rounded-lg bg-red-bg/30 p-3">
                <p className="text-sm text-red">{data.errorMessage}</p>
              </div>
            )}
            {data.retryCount > 0 && (
              <p className="mt-1 text-xs text-ink-400">
                Retry attempts: {data.retryCount}
                {data.lastRetryAt && ` · Last retry: ${formatTime(data.lastRetryAt)}`}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* ===== WALLET INFO ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <Wallet className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">About Wallet Provisioning</span>
          </div>
        </div>
        <div className="p-4 space-y-3">
          <p className="text-sm text-ink-600">
            Company wallet provisioning is required for financial operations including:
          </p>
          <ul className="space-y-1.5 text-sm text-ink-600">
            <li className="flex items-start gap-2">
              <CheckCircle2 className="h-4 w-4 text-green mt-0.5 flex-shrink-0" />
              <span>Receive and send payments</span>
            </li>
            <li className="flex items-start gap-2">
              <CheckCircle2 className="h-4 w-4 text-green mt-0.5 flex-shrink-0" />
              <span>Manage provider sub-wallets</span>
            </li>
            <li className="flex items-start gap-2">
              <CheckCircle2 className="h-4 w-4 text-green mt-0.5 flex-shrink-0" />
              <span>Process allocations and withdrawals</span>
            </li>
          </ul>
          <div className="mt-2 rounded-lg border border-amber/20 bg-amber-bg/30 p-3">
            <p className="text-xs text-amber/80">
              <span className="font-semibold">Note:</span> Wallet balance is never fabricated. Only authoritative balances from Payment Wallet are displayed.
            </p>
          </div>
        </div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="flex flex-wrap gap-3">
        {isFailed && (
          <button
            onClick={handleRetry}
            disabled={isRetrying}
            className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
              isRetrying
                ? "bg-ink-400 cursor-not-allowed"
                : "bg-primary shadow-primary/20 hover:bg-primary/90"
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
            className="flex items-center gap-2 rounded-xl bg-green px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
          >
            Next Step <ArrowRight className="h-4 w-4" />
          </button>
        )}
        <button
          onClick={handleBack}
          className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
        >
          Back
        </button>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Wallet className="h-4 w-4" />
          ORG-07 · Wallet Provisioning
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