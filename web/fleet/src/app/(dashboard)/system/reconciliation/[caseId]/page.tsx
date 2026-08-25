// src/app/(dashboard)/system/reconciliation/[caseId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  GitMerge,
  RefreshCw,
  ArrowLeft,
  CheckCircle2,
  Clock,
  FileX,
  GitPullRequest,
  AlertTriangle,
  XCircle,
  GitBranch,
  AlertCircle,
  CheckCircle,
  AlertOctagon,
  Info,
  History,
  ChevronUp,
  ChevronDown,
  Database,
  Search,
} from "lucide-react";

// ==================== TYPES ====================

type ReconciliationDetail = {
  id: string;
  reference: string;
  subjectType: string;
  subjectId: string;
  subjectName: string;
  authority: string;
  discrepancyType: "Missing" | "Mismatch" | "Stale" | "Corrupted" | "VerificationFailed" | "SyncGap";
  description: string;
  state: "Open" | "Investigating" | "PendingResolution" | "Resolved" | "Closed" | "Failed";
  severity: "Critical" | "High" | "Medium" | "Low";
  attempts: number;
  lastAttemptAt: string | null;
  lastAttemptStatus: "Success" | "Failed" | "Pending" | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  resolution: string | null;
  attemptHistory: {
    id: string;
    attemptedAt: string;
    status: "Success" | "Failed" | "Pending";
    error: string | null;
    performedBy: string;
  }[];
  sourceMetadata: {
    key: string;
    localValue: string;
    authorityValue: string;
    status: "Match" | "Mismatch" | "Missing";
  }[];
  branch: string;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_DETAIL_DATA: Record<string, ReconciliationDetail> = {
  "RC-001": {
    id: "RC-001",
    reference: "RC-2026-001",
    subjectType: "Provider",
    subjectId: "P-044",
    subjectName: "Peter Okafor",
    authority: "Verification Service",
    discrepancyType: "Stale",
    description: "Provider metadata mismatch between local and authority records. Version dates do not align. Local shows last updated 2026-08-20, authority shows 2026-08-22.",
    state: "Open",
    severity: "High",
    attempts: 2,
    lastAttemptAt: "2026-08-21T10:00:00Z",
    lastAttemptStatus: "Failed",
    createdBy: "System",
    createdAt: "2026-08-20T09:00:00Z",
    updatedAt: "2026-08-21T10:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    attemptHistory: [
      {
        id: "AH-001",
        attemptedAt: "2026-08-20T10:00:00Z",
        status: "Failed",
        error: "Authority service timeout",
        performedBy: "System",
      },
      {
        id: "AH-002",
        attemptedAt: "2026-08-21T10:00:00Z",
        status: "Failed",
        error: "Metadata validation failed",
        performedBy: "System",
      },
    ],
    sourceMetadata: [
      { key: "Provider Name", localValue: "Peter Okafor", authorityValue: "Peter Okafor", status: "Match" },
      { key: "Phone Number", localValue: "08012345678", authorityValue: "08012345678", status: "Match" },
      { key: "Email", localValue: "peter@example.com", authorityValue: "peter.okafor@example.com", status: "Mismatch" },
      { key: "Last Updated", localValue: "2026-08-20", authorityValue: "2026-08-22", status: "Mismatch" },
    ],
    branch: "Ikeja",
    tags: ["Verification", "Stale", "Authority"],
    sourceFreshness: "2026-08-21T10:00:00Z",
  },
  "RC-005": {
    id: "RC-005",
    reference: "RC-2026-005",
    subjectType: "Document",
    subjectId: "DOC-009",
    subjectName: "W9 Form - P-071",
    authority: "Tax Service",
    discrepancyType: "VerificationFailed",
    description: "External verification failed. Tax ID validation returned error. Tax ID format does not match authority records.",
    state: "Open",
    severity: "Critical",
    attempts: 2,
    lastAttemptAt: "2026-08-22T15:30:00Z",
    lastAttemptStatus: "Failed",
    createdBy: "System",
    createdAt: "2026-08-22T15:00:00Z",
    updatedAt: "2026-08-22T15:30:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    attemptHistory: [
      {
        id: "AH-003",
        attemptedAt: "2026-08-22T15:15:00Z",
        status: "Failed",
        error: "Tax ID validation failed: Invalid format",
        performedBy: "System",
      },
      {
        id: "AH-004",
        attemptedAt: "2026-08-22T15:30:00Z",
        status: "Failed",
        error: "Tax ID validation failed: Invalid format",
        performedBy: "System",
      },
    ],
    sourceMetadata: [
      { key: "Tax ID", localValue: "12-3456789", authorityValue: "12-3456789", status: "Match" },
      { key: "Legal Name", localValue: "Kabiru Ahmed", authorityValue: "Kabiru A. Ahmed", status: "Mismatch" },
      { key: "Business Type", localValue: "Sole Proprietor", authorityValue: "Individual", status: "Mismatch" },
    ],
    branch: "All branches",
    tags: ["Verification", "Tax", "Critical"],
    sourceFreshness: "2026-08-22T15:30:00Z",
  },
};

type DiscrepancyStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const DISCREPANCY_STYLES: Record<string, DiscrepancyStyle> = {
  Missing: { bg: "bg-red-bg", text: "text-red", icon: <FileX className="h-4 w-4" /> },
  Mismatch: { bg: "bg-amber-bg", text: "text-amber", icon: <GitPullRequest className="h-4 w-4" /> },
  Stale: { bg: "bg-blue-bg", text: "text-blue", icon: <Clock className="h-4 w-4" /> },
  Corrupted: { bg: "bg-purple-bg", text: "text-purple", icon: <AlertTriangle className="h-4 w-4" /> },
  VerificationFailed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  SyncGap: { bg: "bg-amber-bg", text: "text-amber", icon: <GitBranch className="h-4 w-4" /> },
};

type StateStyle = {
  bg: string;
  text: string;
  dot: string;
  icon: React.ReactNode;
};

const STATE_STYLES: Record<string, StateStyle> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-4 w-4" /> },
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Search className="h-4 w-4" /> },
  PendingResolution: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-4 w-4" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <XCircle className="h-4 w-4" /> },
};

type SeverityStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const SEVERITY_STYLES: Record<string, SeverityStyle> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-4 w-4" /> },
};

type AttemptStatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const ATTEMPT_STATUS_STYLES: Record<string, AttemptStatusStyle> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
};

type MetadataStatusStyle = {
  bg: string;
  text: string;
};

const METADATA_STATUS_STYLES: Record<string, MetadataStatusStyle> = {
  Match: { bg: "bg-green-bg", text: "text-green" },
  Mismatch: { bg: "bg-red-bg", text: "text-red" },
  Missing: { bg: "bg-amber-bg", text: "text-amber" },
};

export default function ReconciliationCaseDetailPage() {
  const params = useParams();
  const router = useRouter();
  const caseId = params.caseId as string;

  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<ReconciliationDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [showAttempts, setShowAttempts] = useState<boolean>(true);
  const [showMetadata, setShowMetadata] = useState<boolean>(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_DETAIL_DATA[caseId] || MOCK_DETAIL_DATA["RC-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [caseId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading reconciliation case details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <GitMerge className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Case not found</h3>
        <p className="mt-1 text-sm text-ink-500">No reconciliation case found with this ID.</p>
        <button
          onClick={() => router.push("/system/reconciliation")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Reconciliation Cases
        </button>
      </div>
    );
  }

  const discrepancyStyle = DISCREPANCY_STYLES[data.discrepancyType] || DISCREPANCY_STYLES.Mismatch;
  const stateStyle = STATE_STYLES[data.state] || STATE_STYLES.Open;
  const severityStyle = SEVERITY_STYLES[data.severity] || SEVERITY_STYLES.Medium;

  const formatTime = (date: string): string => {
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

  const formatDateTime = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const handleRetry = () => {
    toast("Reconciliation retry requested (mock)");
  };

  const handleResolve = () => {
    toast("Reconciliation case resolved (mock)");
  };

  const handleBack = () => {
    router.push("/system/reconciliation");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/system/reconciliation")}
              className="hover:text-primary transition-colors"
            >
              Reconciliation
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Reconciliation Case Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${discrepancyStyle.bg} ${discrepancyStyle.text}`}>
              {discrepancyStyle.icon}
              {data.discrepancyType}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
              {stateStyle.icon}
              {data.state}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
              {severityStyle.icon}
              {data.severity}
            </span>
            <span className="text-[10px] text-ink-500">{data.subjectType}</span>
            <span className="text-[10px] text-ink-500">{data.branch}</span>
            {data.tags.map((tag) => (
              <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                {tag}
              </span>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button
            onClick={handleBack}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
          {data.state !== "Resolved" && data.state !== "Closed" && (
            <>
              <button
                onClick={handleRetry}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3.5 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10"
              >
                <RefreshCw className="h-3.5 w-3.5" />
                Retry
              </button>
              <button
                onClick={handleResolve}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                <CheckCircle2 className="h-3.5 w-3.5" />
                Resolve
              </button>
            </>
          )}
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Subject</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.subjectName}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.subjectType} · {data.subjectId}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Authority</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.authority}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.attempts} attempts</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Created</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{formatDateTime(data.createdAt)}</div>
          <div className="mt-0.5 text-xs text-ink-500">by {data.createdBy}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resolution</div>
          <div className="mt-1 text-sm text-ink-950">
            {data.resolvedAt ? (
              <span className="flex items-center gap-1 text-green">
                <CheckCircle2 className="h-4 w-4" />
                Resolved {formatTime(data.resolvedAt)}
              </span>
            ) : (
              <span className="text-ink-400">Pending</span>
            )}
          </div>
          {data.resolvedBy && (
            <div className="mt-0.5 text-xs text-ink-500">By: {data.resolvedBy}</div>
          )}
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
        <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
        {data.resolution && (
          <div className="mt-3 rounded-xl border border-green/20 bg-green-bg/30 p-3">
            <div className="text-xs font-medium text-green">Resolution</div>
            <p className="mt-1 text-sm text-ink-700">{data.resolution}</p>
          </div>
        )}
      </div>

      {/* ===== SOURCE METADATA ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowMetadata(!showMetadata)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-2">
              <Database className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Source Metadata</span>
              <span className="ml-2 text-sm text-ink-500">Local vs Authority comparison</span>
            </div>
          </div>
          {showMetadata ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showMetadata && (
          <div className="border-t border-line p-4">
            <div className="space-y-2">
              {data.sourceMetadata.map((item, idx) => {
                const statusStyle = METADATA_STATUS_STYLES[item.status] || METADATA_STATUS_STYLES.Mismatch;
                return (
                  <div
                    key={idx}
                    className="grid grid-cols-4 gap-3 rounded-xl border border-line p-3 hover:bg-surface-2 transition-colors"
                  >
                    <div className="flex items-center">
                      <span className="text-xs font-medium text-ink-950">{item.key}</span>
                    </div>
                    <div className="flex items-center">
                      <span className="text-xs text-ink-600 font-mono">{item.localValue}</span>
                    </div>
                    <div className="flex items-center">
                      <span className="text-xs text-ink-600 font-mono">{item.authorityValue}</span>
                    </div>
                    <div className="flex items-center justify-end">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        {item.status}
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {/* ===== ATTEMPT HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowAttempts(!showAttempts)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <History className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Attempt History</span>
              <span className="ml-2 text-sm text-ink-500">{data.attemptHistory.length} attempts</span>
            </div>
          </div>
          {showAttempts ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showAttempts && (
          <div className="border-t border-line p-4">
            <div className="relative">
              <div className="absolute left-[11px] top-0 bottom-0 w-0.5 bg-line" />
              <div className="space-y-4">
                {data.attemptHistory.map((attempt) => {
                  const attemptStyle = ATTEMPT_STATUS_STYLES[attempt.status] || ATTEMPT_STATUS_STYLES.Failed;
                  return (
                    <div key={attempt.id} className="flex gap-3">
                      <div className="relative flex-shrink-0">
                        <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                          {attemptStyle.icon}
                        </div>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-medium ${attemptStyle.text}`}>
                            {attempt.status}
                          </span>
                          <span className="text-xs text-ink-500">· {formatDateTime(attempt.attemptedAt)}</span>
                        </div>
                        {attempt.error && (
                          <div className="text-xs text-red-500">{attempt.error}</div>
                        )}
                        <div className="text-xs text-ink-400">by {attempt.performedBy}</div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <GitMerge className="h-4 w-4" />
          SYS-03 · Reconciliation Case Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}