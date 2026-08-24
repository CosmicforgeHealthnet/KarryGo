// src/app/(dashboard)/documents/[documentId]/retention/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  FileText,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Users,
  Building2,
  Tag,
  Calendar,
  Info,
  ListChecks,
  Download,
  Printer,
  Edit3,
  Trash2,
  Check,
  MoreHorizontal,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  File,
  FileArchive,
  FileImage,
  FileJson,
  FilePdf,
  FileSpreadsheet,
  FileText as FileTextIcon,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  Clock as ClockIcon,
  CheckCircle,
  XCircle,
  AlertOctagon,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
  History,
  GitBranch,
  FileUser,
  FileKey,
  FileClock,
  FileX,
  FileMinus,
  FilePlus,
  FileCog,
  FileCheck2,
  Trash,
  Trash2 as TrashIcon,
  Archive,
  Calendar as CalendarIcon,
  AlertTriangle as AlertTriangleIcon,
  Info as InfoIcon,
  Save,
  Undo,
  Timer,
  Hourglass,
  Ban,
  AlertOctagon as AlertOctagonIcon,
  ShieldAlert as ShieldAlertIcon,
} from "lucide-react";

// ==================== TYPES ====================

type DocumentRetention = {
  documentId: string;
  documentReference: string;
  documentName: string;
  documentType: string;
  currentRetentionPeriod: string;
  retentionClass: string;
  effectiveRetention: string;
  legalHoldActive: boolean;
  legalHoldReason: string | null;
  legalHoldCreatedAt: string | null;
  legalHoldCreatedBy: string | null;
  deletionScheduled: boolean;
  scheduledDeletionDate: string | null;
  deletionStatus: "NotScheduled" | "Pending" | "InProgress" | "Completed" | "Failed" | "Cancelled";
  deletionExecutionHistory: {
    id: string;
    attemptedAt: string;
    status: "Success" | "Failed" | "Cancelled";
    error: string | null;
    performedBy: string;
  }[];
  eligibleForDeletion: boolean;
  deletionBlockers: string[];
  platformMinimumRetention: string;
  companyOverride: string | null;
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_RETENTION_DATA: Record<string, DocumentRetention> = {
  "DOC-001": {
    documentId: "DOC-001",
    documentReference: "DOC-2026-001",
    documentName: "Vehicle Registration - BR-204",
    documentType: "PDF",
    currentRetentionPeriod: "7 years",
    retentionClass: "Regulatory",
    effectiveRetention: "7 years",
    legalHoldActive: false,
    legalHoldReason: null,
    legalHoldCreatedAt: null,
    legalHoldCreatedBy: null,
    deletionScheduled: false,
    scheduledDeletionDate: null,
    deletionStatus: "NotScheduled",
    deletionExecutionHistory: [],
    eligibleForDeletion: false,
    deletionBlockers: ["Regulatory retention period not met", "Document is actively referenced"],
    platformMinimumRetention: "5 years",
    companyOverride: "7 years",
    sourceFreshness: "2026-08-15T10:00:00Z",
  },
  "DOC-005": {
    documentId: "DOC-005",
    documentReference: "DOC-2026-005",
    documentName: "Driver License - P-044",
    documentType: "Image",
    currentRetentionPeriod: "3 years",
    retentionClass: "Compliance",
    effectiveRetention: "3 years",
    legalHoldActive: false,
    legalHoldReason: null,
    legalHoldCreatedAt: null,
    legalHoldCreatedBy: null,
    deletionScheduled: false,
    scheduledDeletionDate: null,
    deletionStatus: "NotScheduled",
    deletionExecutionHistory: [],
    eligibleForDeletion: false,
    deletionBlockers: ["Document is pending verification"],
    platformMinimumRetention: "3 years",
    companyOverride: null,
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  "DOC-007": {
    documentId: "DOC-007",
    documentReference: "DOC-2026-007",
    documentName: "Financial Report Q3 2026",
    documentType: "Spreadsheet",
    currentRetentionPeriod: "7 years",
    retentionClass: "Financial",
    effectiveRetention: "7 years",
    legalHoldActive: true,
    legalHoldReason: "Under audit investigation",
    legalHoldCreatedAt: "2026-08-20T09:00:00Z",
    legalHoldCreatedBy: "F. Daniel",
    deletionScheduled: false,
    scheduledDeletionDate: null,
    deletionStatus: "NotScheduled",
    deletionExecutionHistory: [],
    eligibleForDeletion: false,
    deletionBlockers: ["Legal hold active", "Retention period not met"],
    platformMinimumRetention: "5 years",
    companyOverride: "7 years",
    sourceFreshness: "2026-08-22T13:00:00Z",
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  NotScheduled: { bg: "bg-surface-2", text: "text-ink-500", icon: <Minus className="h-4 w-4" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <ClockIcon className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-4 w-4" /> },
};

const EXECUTION_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-4 w-4" /> },
};

export default function DocumentRetentionPage() {
  const params = useParams();
  const router = useRouter();
  const documentId = params.documentId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DocumentRetention | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showHistory, setShowHistory] = useState(true);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);
  const [confirmText, setConfirmText] = useState("");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_RETENTION_DATA[documentId] || MOCK_RETENTION_DATA["DOC-001"];
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
        <p className="mt-3 text-sm text-ink-600">Loading retention details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileClock className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Retention data not found</h3>
        <p className="mt-1 text-sm text-ink-500">No retention data found for this document.</p>
        <button
          onClick={() => router.push(`/documents/${documentId}`)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Document
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.deletionStatus] || STATUS_STYLES.NotScheduled;

  const formatTime = (date: string) => {
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

  const formatDateTime = (date: string | null) => {
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

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const handleScheduleDeletion = () => {
    setShowConfirmDialog(true);
  };

  const handleConfirmDeletion = () => {
    if (confirmText !== "DELETE") {
      toast('Please type "DELETE" to confirm');
      return;
    }
    toast("Deletion scheduled (mock)");
    setShowConfirmDialog(false);
    setConfirmText("");
  };

  const handleCancelScheduledDeletion = () => {
    toast("Deletion cancelled (mock)");
  };

  const handlePlaceLegalHold = () => {
    toast("Legal hold placed (mock)");
  };

  const handleRemoveLegalHold = () => {
    toast("Legal hold removed (mock)");
  };

  const handleSetRetention = () => {
    toast("Retention period updated (mock)");
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
            <span className="text-ink-700 font-medium">Retention</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Document Retention & Deletion
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className="text-sm text-ink-600">{data.documentName}</span>
            <span className="text-xs text-ink-400 font-mono">{data.documentReference}</span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.deletionStatus}
            </span>
            {data.legalHoldActive && (
              <span className="flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold bg-red-bg text-red">
                <ShieldAlertIcon className="h-3.5 w-3.5" />
                Legal Hold
              </span>
            )}
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

      {/* ===== RETENTION OVERVIEW ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Retention Period</div>
          <div className="mt-1 text-lg font-bold text-ink-950">{data.currentRetentionPeriod}</div>
          <div className="mt-0.5 text-xs text-ink-500">Class: {data.retentionClass}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Platform Minimum</div>
          <div className="mt-1 text-lg font-bold text-ink-950">{data.platformMinimumRetention}</div>
          {data.companyOverride && (
            <div className="mt-0.5 text-xs text-ink-500">Company: {data.companyOverride}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Legal Hold</div>
          <div className="mt-1 flex items-center gap-2">
            {data.legalHoldActive ? (
              <span className="flex items-center gap-1 text-sm font-semibold text-red">
                <AlertOctagonIcon className="h-4 w-4" />
                Active
              </span>
            ) : (
              <span className="text-sm text-ink-400">Inactive</span>
            )}
          </div>
          {data.legalHoldReason && (
            <div className="mt-0.5 text-xs text-ink-500">{data.legalHoldReason}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Deletion Scheduled</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.deletionScheduled ? (
              <span className="flex items-center gap-1 text-amber">
                <ClockIcon className="h-4 w-4" />
                {data.scheduledDeletionDate ? formatDate(data.scheduledDeletionDate) : "Pending"}
              </span>
            ) : (
              <span className="text-ink-400">Not scheduled</span>
            )}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            {data.eligibleForDeletion ? "Eligible" : "Not eligible"}
          </div>
        </div>
      </div>

      {/* ===== DELETION BLOCKERS ===== */}
      {data.deletionBlockers.length > 0 && (
        <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red-bg p-2 flex-shrink-0">
              <AlertOctagonIcon className="h-5 w-5 text-red" />
            </div>
            <div className="flex-1">
              <h4 className="text-sm font-semibold text-red">Deletion Blockers</h4>
              <ul className="mt-2 space-y-1">
                {data.deletionBlockers.map((blocker, index) => (
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

      {/* ===== ACTIONS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Actions</h3>
        <div className="mt-3 flex flex-wrap gap-3">
          {!data.legalHoldActive && (
            <button
              onClick={handlePlaceLegalHold}
              className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg px-4 py-2.5 text-sm font-medium text-red hover:bg-red/10 transition-colors"
            >
              <ShieldAlertIcon className="h-4 w-4" />
              Place Legal Hold
            </button>
          )}
          {data.legalHoldActive && (
            <button
              onClick={handleRemoveLegalHold}
              className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              <Shield className="h-4 w-4" />
              Remove Legal Hold
            </button>
          )}
          <button
            onClick={handleSetRetention}
            className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
          >
            <Save className="h-4 w-4" />
            Update Retention Period
          </button>
          {data.eligibleForDeletion && !data.deletionScheduled && !data.legalHoldActive && (
            <button
              onClick={handleScheduleDeletion}
              className="flex items-center gap-2 rounded-xl bg-red px-4 py-2.5 text-sm font-semibold text-white shadow-sm shadow-red/20 hover:bg-red/90 transition-colors"
            >
              <TrashIcon className="h-4 w-4" />
              Schedule Deletion
            </button>
          )}
          {data.deletionScheduled && (
            <button
              onClick={handleCancelScheduledDeletion}
              className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              <Undo className="h-4 w-4" />
              Cancel Scheduled Deletion
            </button>
          )}
        </div>
      </div>

      {/* ===== CONFIRM DELETION DIALOG ===== */}
      {showConfirmDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <div className="flex items-center gap-3 text-red">
              <AlertOctagonIcon className="h-6 w-6" />
              <h3 className="text-lg font-semibold">Confirm Deletion</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Are you sure you want to schedule this document for deletion? This action cannot be undone once executed.
            </p>
            <p className="mt-2 text-sm text-ink-600">
              Type <span className="font-bold text-red">DELETE</span> to confirm.
            </p>
            <input
              type="text"
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder='Type "DELETE" to confirm'
              className="mt-3 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
            />
            <div className="mt-4 flex gap-3">
              <button
                onClick={handleConfirmDeletion}
                className="flex-1 rounded-xl bg-red px-4 py-2.5 text-sm font-semibold text-white shadow-sm shadow-red/20 hover:bg-red/90 transition-colors"
              >
                Confirm Deletion
              </button>
              <button
                onClick={() => {
                  setShowConfirmDialog(false);
                  setConfirmText("");
                }}
                className="flex-1 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== EXECUTION HISTORY ===== */}
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
              <span className="font-semibold text-ink-950">Execution History</span>
              <span className="ml-2 text-sm text-ink-500">{data.deletionExecutionHistory.length} attempts</span>
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
            {data.deletionExecutionHistory.length === 0 ? (
              <div className="text-center py-4 text-sm text-ink-500">No execution history available.</div>
            ) : (
              <div className="space-y-3">
                {data.deletionExecutionHistory.map((attempt) => {
                  const attemptStyle = EXECUTION_STATUS_STYLES[attempt.status] || EXECUTION_STATUS_STYLES.Failed;
                  return (
                    <div
                      key={attempt.id}
                      className="flex items-center gap-3 rounded-xl border border-line p-3 hover:bg-surface-2 transition-colors"
                    >
                      <div className="flex-shrink-0">
                        {attemptStyle.icon}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-medium ${attemptStyle.text}`}>
                            {attempt.status}
                          </span>
                          <span className="text-xs text-ink-500">· {formatTime(attempt.attemptedAt)}</span>
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
            )}
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileClock className="h-4 w-4" />
          DAT-05 · Document Retention · {data.documentReference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}