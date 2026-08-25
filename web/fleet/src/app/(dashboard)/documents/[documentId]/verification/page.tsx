// src/app/(dashboard)/documents/[documentId]/verification/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  XCircle,
  Minus,
  Edit3,
  ExternalLink,
  History,
  ChevronUp,
  ChevronDown,
  FileCheck,
  Info,
} from "lucide-react";

// ==================== TYPES ====================

type DocumentVerification = {
  documentId: string;
  documentReference: string;
  documentName: string;
  documentType: string;
  currentVerificationStatus: "Pending" | "Verified" | "Rejected" | "NotRequired";
  currentVerifiedBy: string | null;
  currentVerifiedAt: string | null;
  currentVerificationNotes: string | null;
  verificationHistory: {
    id: string;
    action: "Requested" | "Verified" | "Rejected" | "Reopened";
    performedBy: string;
    performedAt: string;
    notes: string | null;
  }[];
  pendingRequests: {
    id: string;
    requestedBy: string;
    requestedAt: string;
    reason: string | null;
  }[];
  recommendedAction: "Verify" | "Reject" | "RequestChanges" | "NoAction" | "ExternalVerification";
  externalVerificationStatus: "NotStarted" | "Pending" | "Completed" | "Failed";
  externalVerificationReference: string | null;
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_VERIFICATION_DATA: Record<string, DocumentVerification> = {
  "DOC-001": {
    documentId: "DOC-001",
    documentReference: "DOC-2026-001",
    documentName: "Vehicle Registration - BR-204",
    documentType: "PDF",
    currentVerificationStatus: "Verified",
    currentVerifiedBy: "F. Daniel",
    currentVerifiedAt: "2026-08-15T10:00:00Z",
    currentVerificationNotes: "Registration verified against government database. All details match.",
    verificationHistory: [
      {
        id: "VH-001",
        action: "Requested",
        performedBy: "M. Bello",
        performedAt: "2026-08-10T14:30:00Z",
        notes: "Initial verification request.",
      },
      {
        id: "VH-002",
        action: "Verified",
        performedBy: "F. Daniel",
        performedAt: "2026-08-15T10:00:00Z",
        notes: "Document verified against government database. All details match.",
      },
    ],
    pendingRequests: [],
    recommendedAction: "NoAction",
    externalVerificationStatus: "Completed",
    externalVerificationReference: "GOV-2026-8842",
    sourceFreshness: "2026-08-15T10:00:00Z",
  },
  "DOC-005": {
    documentId: "DOC-005",
    documentReference: "DOC-2026-005",
    documentName: "Driver License - P-044",
    documentType: "Image",
    currentVerificationStatus: "Pending",
    currentVerifiedBy: null,
    currentVerifiedAt: null,
    currentVerificationNotes: null,
    verificationHistory: [
      {
        id: "VH-003",
        action: "Requested",
        performedBy: "P. Okafor",
        performedAt: "2026-08-22T08:00:00Z",
        notes: "Driver license uploaded for verification.",
      },
    ],
    pendingRequests: [
      {
        id: "PR-001",
        requestedBy: "P. Okafor",
        requestedAt: "2026-08-22T08:00:00Z",
        reason: "Driver license uploaded for verification.",
      },
    ],
    recommendedAction: "Verify",
    externalVerificationStatus: "NotStarted",
    externalVerificationReference: null,
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  "DOC-009": {
    documentId: "DOC-009",
    documentReference: "DOC-2026-009",
    documentName: "W9 Form - Provider P-071",
    documentType: "PDF",
    currentVerificationStatus: "Pending",
    currentVerifiedBy: null,
    currentVerifiedAt: null,
    currentVerificationNotes: null,
    verificationHistory: [
      {
        id: "VH-004",
        action: "Requested",
        performedBy: "K. Ahmed",
        performedAt: "2026-08-22T15:00:00Z",
        notes: "W9 form uploaded for verification.",
      },
    ],
    pendingRequests: [
      {
        id: "PR-002",
        requestedBy: "K. Ahmed",
        requestedAt: "2026-08-22T15:00:00Z",
        reason: "W9 form uploaded for verification.",
      },
    ],
    recommendedAction: "Verify",
    externalVerificationStatus: "NotStarted",
    externalVerificationReference: null,
    sourceFreshness: "2026-08-22T15:00:00Z",
  },
};

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  NotRequired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Minus className="h-4 w-4" /> },
};

type RecommendedActionStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const RECOMMENDED_ACTION_STYLES: Record<string, RecommendedActionStyle> = {
  Verify: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Reject: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  RequestChanges: { bg: "bg-amber-bg", text: "text-amber", icon: <Edit3 className="h-4 w-4" /> },
  NoAction: { bg: "bg-surface-2", text: "text-ink-500", icon: <Minus className="h-4 w-4" /> },
  ExternalVerification: { bg: "bg-blue-bg", text: "text-blue", icon: <ExternalLink className="h-4 w-4" /> },
};

export default function DocumentVerificationPage() {
  const params = useParams();
  const router = useRouter();
  const documentId = params.documentId as string;

  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<DocumentVerification | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [verificationNotes, setVerificationNotes] = useState<string>("");
  const [verificationAction, setVerificationAction] = useState<"verify" | "reject" | null>(null);
  const [showHistory, setShowHistory] = useState<boolean>(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_VERIFICATION_DATA[documentId] || MOCK_VERIFICATION_DATA["DOC-005"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [documentId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading verification details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Verification data not found</h3>
        <p className="mt-1 text-sm text-ink-500">No verification data found for this document.</p>
        <button
          onClick={() => router.push(`/documents/${documentId}`)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Document
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.currentVerificationStatus] || STATUS_STYLES.Pending;
  const recommendedStyle = RECOMMENDED_ACTION_STYLES[data.recommendedAction] || RECOMMENDED_ACTION_STYLES.NoAction;

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

  const getActionIcon = (action: string) => {
    switch (action) {
      case "Requested": return <Clock className="h-4 w-4 text-amber" />;
      case "Verified": return <CheckCircle2 className="h-4 w-4 text-green" />;
      case "Rejected": return <XCircle className="h-4 w-4 text-red" />;
      case "Reopened": return <RefreshCw className="h-4 w-4 text-blue" />;
      default: return <Info className="h-4 w-4 text-ink-400" />;
    }
  };

  const handleVerify = () => {
    setVerificationAction("verify");
    toast("Document verified (mock)");
    setTimeout(() => {
      router.push(`/documents/${documentId}`);
    }, 1500);
  };

  const handleReject = () => {
    setVerificationAction("reject");
    toast("Document rejected (mock)");
    setTimeout(() => {
      router.push(`/documents/${documentId}`);
    }, 1500);
  };

  const handleExternalVerification = () => {
    toast("External verification requested (mock)");
  };

  const handleRequestChanges = () => {
    toast("Changes requested (mock)");
  };

  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push(`/documents/${documentId}`)}
              className="hover:text-primary transition-colors"
            >
              Document
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">Verification</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Document Verification
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className="text-sm text-ink-600">{data.documentName}</span>
            <span className="text-xs text-ink-400 font-mono">{data.documentReference}</span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.currentVerificationStatus}
            </span>
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
        </div>
      </div>

      {/* ===== STATUS OVERVIEW ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Current Status</div>
          <div className="mt-1 flex items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.currentVerificationStatus}
            </span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Verified By</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.currentVerifiedBy || "—"}
          </div>
          {data.currentVerifiedAt && (
            <div className="mt-0.5 text-xs text-ink-500">{formatDateTime(data.currentVerifiedAt)}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Recommended Action</div>
          <div className="mt-1 flex items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${recommendedStyle.bg} ${recommendedStyle.text}`}>
              {recommendedStyle.icon}
              {data.recommendedAction}
            </span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">External Verification</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.externalVerificationStatus}</div>
          {data.externalVerificationReference && (
            <div className="mt-0.5 text-xs text-ink-500 font-mono">Ref: {data.externalVerificationReference}</div>
          )}
        </div>
      </div>

      {/* ===== VERIFICATION NOTES ===== */}
      {data.currentVerificationNotes && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Verification Notes</h3>
          <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.currentVerificationNotes}</p>
        </div>
      )}

      {/* ===== PENDING REQUESTS ===== */}
      {data.pendingRequests.length > 0 && (
        <div className="rounded-2xl border border-amber/20 bg-amber-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-amber-bg p-2 flex-shrink-0">
              <Clock className="h-5 w-5 text-amber" />
            </div>
            <div className="flex-1">
              <h4 className="text-sm font-semibold text-amber">Pending Verification Requests</h4>
              <div className="mt-2 space-y-2">
                {data.pendingRequests.map((request) => (
                  <div key={request.id} className="text-sm text-amber/80">
                    <span className="font-medium">{request.requestedBy}</span>
                    <span className="mx-2">·</span>
                    <span>{formatTime(request.requestedAt)}</span>
                    {request.reason && (
                      <div className="mt-0.5 text-xs text-amber/60">{request.reason}</div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ===== VERIFICATION ACTIONS ===== */}
      {data.currentVerificationStatus === "Pending" && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Verification Actions</h3>
          <div className="mt-3 space-y-4">
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Notes
              </label>
              <textarea
                value={verificationNotes}
                onChange={(e) => setVerificationNotes(e.target.value)}
                placeholder="Add verification notes..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary resize-none min-h-[80px]"
              />
            </div>
            <div className="flex flex-wrap gap-3">
              <button
                onClick={handleVerify}
                disabled={!!verificationAction}
                className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                  verificationAction
                    ? "bg-ink-400 cursor-not-allowed"
                    : "bg-green shadow-green/20 hover:bg-green/90"
                }`}
              >
                {verificationAction === "verify" ? (
                  <>
                    <RefreshCw className="h-4 w-4 animate-spin" />
                    Verifying...
                  </>
                ) : (
                  <>
                    <CheckCircle2 className="h-4 w-4" />
                    Verify Document
                  </>
                )}
              </button>
              <button
                onClick={handleReject}
                disabled={!!verificationAction}
                className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                  verificationAction
                    ? "bg-ink-400 cursor-not-allowed"
                    : "bg-red shadow-red/20 hover:bg-red/90"
                }`}
              >
                {verificationAction === "reject" ? (
                  <>
                    <RefreshCw className="h-4 w-4 animate-spin" />
                    Rejecting...
                  </>
                ) : (
                  <>
                    <XCircle className="h-4 w-4" />
                    Reject
                  </>
                )}
              </button>
              <button
                onClick={handleRequestChanges}
                className="flex items-center gap-2 rounded-xl border border-amber/20 bg-amber-bg px-6 py-2.5 text-sm font-medium text-amber hover:bg-amber/10 transition-colors"
              >
                <Edit3 className="h-4 w-4" />
                Request Changes
              </button>
              <button
                onClick={handleExternalVerification}
                className="flex items-center gap-2 rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
              >
                <ExternalLink className="h-4 w-4" />
                External Verification
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== VERIFICATION HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowHistory(!showHistory)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <History className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Verification History</span>
              <span className="ml-2 text-sm text-ink-500">{data.verificationHistory.length} events</span>
            </div>
          </div>
          {showHistory ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showHistory && (
          <div className="border-t border-line p-4">
            <div className="relative">
              <div className="absolute left-[11px] top-0 bottom-0 w-0.5 bg-line" />
              <div className="space-y-4">
                {data.verificationHistory.map((event) => (
                  <div key={event.id} className="flex gap-3">
                    <div className="relative flex-shrink-0">
                      <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                        {getActionIcon(event.action)}
                      </div>
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-ink-950">
                        <span className="font-medium">{event.action}</span>
                        {event.notes && (
                          <span className="text-ink-600"> — {event.notes}</span>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center gap-2 text-xs text-ink-500">
                        <span>by {event.performedBy}</span>
                        <span>· {formatTime(event.performedAt)}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileCheck className="h-4 w-4" />
          DAT-04 · Document Verification · {data.documentReference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}