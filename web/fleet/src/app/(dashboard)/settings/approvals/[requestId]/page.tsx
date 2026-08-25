// src/app/(dashboard)/settings/approvals/[requestId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  FileCheck,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  ArrowLeft,
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
  FileText,
  File,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  AlertOctagon,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
  History,
  GitBranch,
  X,
} from "lucide-react";

// ==================== TYPES ====================

type ApprovalDetail = {
  id: string;
  reference: string;
  action: string;
  resource: string;
  resourceId: string | null;
  requester: {
    id: string;
    name: string;
    email: string;
  };
  status: "Pending" | "Approved" | "Rejected" | "Cancelled" | "Expired" | "Executed";
  urgency: "Urgent" | "High" | "Normal" | "Low";
  approvalsRequired: number;
  approvalOrder: "Sequential" | "Parallel" | "Any";
  approvals: {
    id: string;
    approver: {
      id: string;
      name: string;
    };
    decision: "Pending" | "Approved" | "Rejected";
    reason: string | null;
    timestamp: string | null;
  }[];
  eligibilityReference: string | null;
  expiresAt: string | null;
  createdAt: string;
  updatedAt: string;
  executedAt: string | null;
  executedBy: string | null;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_APPROVAL_DETAIL: Record<string, ApprovalDetail> = {
  "AR-001": {
    id: "AR-001",
    reference: "APR-2026-001",
    action: "Staff Role Change",
    resource: "S-005 · Chinedu Uche",
    resourceId: "S-005",
    requester: {
      id: "S-001",
      name: "Favour Daniel",
      email: "favour@apexlogistics.com",
    },
    status: "Pending",
    urgency: "High",
    approvalsRequired: 2,
    approvalOrder: "Sequential",
    approvals: [
      {
        id: "AP-001",
        approver: { id: "S-002", name: "Oluwaseun Adebayo" },
        decision: "Approved",
        reason: "Role change aligns with operational needs.",
        timestamp: "2026-08-23T09:30:00Z",
      },
      {
        id: "AP-002",
        approver: { id: "S-006", name: "Esther Obi" },
        decision: "Pending",
        reason: null,
        timestamp: null,
      },
    ],
    eligibilityReference: null,
    expiresAt: "2026-08-26T09:00:00Z",
    createdAt: "2026-08-23T09:00:00Z",
    updatedAt: "2026-08-23T09:30:00Z",
    executedAt: null,
    executedBy: null,
    branch: "Ikeja",
    tags: ["Staff", "Role"],
  },
  "AR-002": {
    id: "AR-002",
    reference: "APR-2026-002",
    action: "Wallet Withdrawal",
    resource: "₦1,200,000 · UBA ••3901",
    resourceId: null,
    requester: {
      id: "S-002",
      name: "Oluwaseun Adebayo",
      email: "seun@apexlogistics.com",
    },
    status: "Pending",
    urgency: "Urgent",
    approvalsRequired: 2,
    approvalOrder: "Sequential",
    approvals: [
      {
        id: "AP-003",
        approver: { id: "S-001", name: "Favour Daniel" },
        decision: "Pending",
        reason: null,
        timestamp: null,
      },
      {
        id: "AP-004",
        approver: { id: "S-006", name: "Esther Obi" },
        decision: "Pending",
        reason: null,
        timestamp: null,
      },
    ],
    eligibilityReference: "Approval threshold: ₦1,000,000",
    expiresAt: "2026-08-25T08:30:00Z",
    createdAt: "2026-08-23T08:30:00Z",
    updatedAt: "2026-08-23T08:30:00Z",
    executedAt: null,
    executedBy: null,
    branch: "Ikeja",
    tags: ["Wallet", "Finance"],
  },
  "AR-004": {
    id: "AR-004",
    reference: "APR-2026-004",
    action: "Staff Suspension",
    resource: "S-007 · Kabiru Ahmed",
    resourceId: "S-007",
    requester: {
      id: "S-001",
      name: "Favour Daniel",
      email: "favour@apexlogistics.com",
    },
    status: "Approved",
    urgency: "High",
    approvalsRequired: 2,
    approvalOrder: "Sequential",
    approvals: [
      {
        id: "AP-005",
        approver: { id: "S-002", name: "Oluwaseun Adebayo" },
        decision: "Approved",
        reason: "Performance issues documented.",
        timestamp: "2026-08-22T16:30:00Z",
      },
      {
        id: "AP-006",
        approver: { id: "S-006", name: "Esther Obi" },
        decision: "Approved",
        reason: "Compliance review confirms suspension is warranted.",
        timestamp: "2026-08-22T17:00:00Z",
      },
    ],
    eligibilityReference: "Suspension policy AP-004",
    expiresAt: "2026-08-24T16:00:00Z",
    createdAt: "2026-08-22T16:00:00Z",
    updatedAt: "2026-08-22T17:00:00Z",
    executedAt: null,
    executedBy: null,
    branch: "Apapa",
    tags: ["Staff", "Suspension"],
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-4 w-4" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-4 w-4" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-4 w-4" /> },
  Executed: { bg: "bg-blue-bg", text: "text-blue", icon: <Check className="h-4 w-4" /> },
};

const URGENCY_STYLES: Record<string, { bg: string; text: string }> = {
  Urgent: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const DECISION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
};

// ===== SIMPLE TOAST FUNCTION =====
const toast = (message: string) => {
  if (typeof window !== "undefined") {
    const toastEl = document.createElement("div");
    toastEl.className = "fixed bottom-4 right-4 bg-ink-950 text-white px-4 py-2 rounded-xl text-sm shadow-lg z-50 animate-in slide-in-from-bottom-5";
    toastEl.textContent = message;
    document.body.appendChild(toastEl);
    setTimeout(() => {
      toastEl.classList.add("opacity-0", "transition-opacity", "duration-300");
      setTimeout(() => toastEl.remove(), 300);
    }, 3000);
  }
};

export default function ApprovalRequestDetailPage() {
  const params = useParams();
  const router = useRouter();
  const requestId = params.requestId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ApprovalDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_APPROVAL_DETAIL[requestId] || MOCK_APPROVAL_DETAIL["AR-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [requestId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading approval details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Request not found</h3>
        <p className="mt-1 text-sm text-ink-500">No approval request found with this ID.</p>
        <button
          onClick={() => router.push("/settings/approvals")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Approvals
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Pending;
  const urgencyStyle = URGENCY_STYLES[data.urgency] || URGENCY_STYLES.Normal;

  const formatTime = (date: string | null) => {
    if (!date) return "Never";
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

  const handleApprove = () => {
    toast("Request approved (mock)");
  };

  const handleReject = () => {
    toast("Request rejected (mock)");
  };

  const handleCancel = () => {
    toast("Request cancelled (mock)");
  };

  const handleExecute = () => {
    toast("Request executed (mock)");
  };

  const handleBack = () => {
    router.push("/settings/approvals");
  };

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/approvals")}
              className="hover:text-primary transition-colors"
            >
              Approvals
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Approval Request Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.status}
            </span>
            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${urgencyStyle.bg} ${urgencyStyle.text}`}>
              {data.urgency}
            </span>
            <span className="text-[10px] text-ink-500">{data.action}</span>
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
          {data.status === "Pending" && (
            <>
              <button
                onClick={handleApprove}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
              >
                <CheckCircle2 className="h-3.5 w-3.5" />
                Approve
              </button>
              <button
                onClick={handleReject}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-red px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-red/20 transition hover:bg-red/90"
              >
                <X className="h-3.5 w-3.5" />
                Reject
              </button>
              <button
                onClick={handleCancel}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
              >
                <X className="h-3.5 w-3.5" />
                Cancel
              </button>
            </>
          )}
          {data.status === "Approved" && (
            <button
              onClick={handleExecute}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-blue px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-blue/20 transition hover:bg-blue/90"
            >
              <Check className="h-3.5 w-3.5" />
              Execute
            </button>
          )}
          <button
            onClick={handleBack}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resource</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.resource}</div>
          {data.resourceId && (
            <div className="mt-0.5 text-xs text-ink-500 font-mono">{data.resourceId}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Requester</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.requester.name}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.requester.email}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Approvals</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.approvals.filter((a) => a.decision === "Approved").length}/{data.approvalsRequired}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">Order: {data.approvalOrder}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expiry</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.expiresAt ? formatDateTime(data.expiresAt) : "No expiry"}
          </div>
          {data.eligibilityReference && (
            <div className="mt-0.5 text-xs text-ink-500">{data.eligibilityReference}</div>
          )}
        </div>
      </div>

      {/* ===== APPROVALS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <ListChecks className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Approvals</span>
            <span className="text-xs text-ink-400">
              {data.approvals.filter((a) => a.decision !== "Pending").length}/{data.approvals.length} decided
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {data.approvals.map((approval) => {
            const decisionStyle = DECISION_STYLES[approval.decision] || DECISION_STYLES.Pending;

            return (
              <div key={approval.id} className="flex items-center gap-4 p-4">
                <div className="flex-shrink-0">
                  <div className={`rounded-full p-2 ${decisionStyle.bg} ${decisionStyle.text}`}>
                    {decisionStyle.icon}
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-ink-950">{approval.approver.name}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${decisionStyle.bg} ${decisionStyle.text}`}>
                      {approval.decision}
                    </span>
                  </div>
                  {approval.reason && (
                    <div className="mt-0.5 text-xs text-ink-500">{approval.reason}</div>
                  )}
                  {approval.timestamp && (
                    <div className="mt-0.5 text-[10px] text-ink-400">Decided {formatTime(approval.timestamp)}</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileCheck className="h-4 w-4" />
          IAM-10 · Approval Request Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}