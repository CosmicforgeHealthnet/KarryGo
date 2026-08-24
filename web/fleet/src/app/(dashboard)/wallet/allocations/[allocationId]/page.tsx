// src/app/(dashboard)/wallet/allocations/[allocationId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  CreditCard,
  TrendingUp,
  TrendingDown,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  Wallet,
  Users,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
  Printer,
  User,
  Briefcase,
  MapPin,
  DollarSign,
  Send,
  ThumbsUp,
  ThumbsDown,
  XCircle,
} from "lucide-react";

type AllocationDetail = {
  id: string;
  reference: string;
  providerId: string;
  providerName: string;
  providerInitials: string;
  providerRole: string;
  providerBranch: string;
  providerBalance: number;
  type: "Trip Settlement" | "Compensation" | "Adjustment" | "Reimbursement" | "Bonus";
  amount: number;
  currency: string;
  status: "Pending" | "Approved" | "Executed" | "Reconciling" | "Failed" | "Cancelled";
  description: string;
  createdAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  executedAt: string | null;
  businessReference: string;
  reason: string;
  approvalHistory: {
    id: string;
    step: string;
    approver: string;
    decision: "Approved" | "Rejected" | "Pending";
    timestamp: string;
    comment: string | null;
  }[];
  metadata: {
    orderRef?: string;
    tripRef?: string;
    batchRef?: string;
    notes?: string;
  };
};

// Mock data
const MOCK_ALLOCATION: AllocationDetail = {
  id: "ALL-001",
  reference: "ALL-001",
  providerId: "P-044",
  providerName: "Peter Okafor",
  providerInitials: "PO",
  providerRole: "Dispatch Rider",
  providerBranch: "Ikeja",
  providerBalance: 284000,
  type: "Trip Settlement",
  amount: 120000,
  currency: "NGN",
  status: "Approved",
  description: "Trip settlement for KG-8491",
  createdAt: "2026-08-21 09:30",
  createdBy: "F. Daniel",
  approvedBy: "O. Adebayo",
  approvedAt: "2026-08-21 09:45",
  executedAt: null,
  businessReference: "KG-8491",
  reason: "Settlement for completed trip KG-8491. Provider Peter Okafor completed the trip successfully. All proof of delivery verified.",
  approvalHistory: [
    { id: "AP-001", step: "Requester", approver: "F. Daniel", decision: "Approved", timestamp: "2026-08-21 09:30", comment: "Trip verified, requesting approval." },
    { id: "AP-002", step: "Finance Review", approver: "O. Adebayo", decision: "Approved", timestamp: "2026-08-21 09:45", comment: "Amount matches trip rate. Approved." },
    { id: "AP-003", step: "Execution", approver: "System", decision: "Pending", timestamp: "2026-08-21 09:50", comment: "Awaiting execution." },
  ],
  metadata: {
    orderRef: "OR-129",
    tripRef: "KG-8491",
    notes: "Provider completed 342 jobs this month. Consistent performance.",
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
  },
  Approved: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Executed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Reconciling: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-4 w-4" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Cancelled: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <XCircle className="h-4 w-4" />,
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  "Trip Settlement": { bg: "bg-green-bg", text: "text-green" },
  Compensation: { bg: "bg-blue-bg", text: "text-blue" },
  Adjustment: { bg: "bg-amber-bg", text: "text-amber" },
  Reimbursement: { bg: "bg-purple-bg", text: "text-purple" },
  Bonus: { bg: "bg-pink-bg", text: "text-pink" },
};

const DECISION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <ThumbsUp className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <ThumbsDown className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
};

export default function AllocationDetailPage() {
  const router = useRouter();
  const params = useParams();
  const allocationId = params.allocationId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<AllocationDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_ALLOCATION, id: allocationId });
      setLoading(false);
    };
    loadData();
  }, [allocationId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading allocation details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <CreditCard className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Allocation not found</h3>
        <p className="mt-1 text-sm text-ink-500">The allocation you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/wallet/allocations")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to allocations
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Pending;
  const typeStyle = TYPE_STYLES[data.type] || TYPE_STYLES["Trip Settlement"];
  const formattedAmount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(Math.abs(data.amount));
  const isNegative = data.amount < 0;
  const isPending = data.status === "Pending";
  const isApproved = data.status === "Approved";
  const isExecuted = data.status === "Executed";

  const handleApprove = () => {
    setApproving(true);
    setTimeout(() => {
      setApproving(false);
      // In real app, would call API and refresh data
      toast("Allocation approved successfully");
    }, 1500);
  };

  const handleReject = () => {
    setRejecting(true);
    setTimeout(() => {
      setRejecting(false);
      // In real app, would call API and refresh data
      toast("Allocation rejected");
    }, 1500);
  };

  const handleExecute = () => {
    // In real app, would call API to execute
    toast("Execution requested");
  };

  // Simple toast function
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
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/wallet/allocations")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Allocation {data.reference}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}
              >
                {statusStyle.icon}
                {data.status}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.description}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {isPending && (
            <>
              <button
                onClick={handleReject}
                disabled={rejecting}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg/50 px-3.5 text-[11.5px] font-medium text-red transition hover:bg-red-bg"
              >
                {rejecting ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ThumbsDown className="h-3.5 w-3.5" />}
                Reject
              </button>
              <button
                onClick={handleApprove}
                disabled={approving}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                {approving ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ThumbsUp className="h-3.5 w-3.5" />}
                Approve
              </button>
            </>
          )}
          {isApproved && !isExecuted && (
            <button
              onClick={handleExecute}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
            >
              <Send className="h-3.5 w-3.5" />
              Execute
            </button>
          )}
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Amount Card */}
          <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span className={typeStyle.bg + " rounded-full px-2.5 py-0.5 text-[10px] font-semibold " + typeStyle.text}>
                {data.type}
              </span>
              <span className="text-ink-300">·</span>
              <span>{data.businessReference}</span>
            </div>
            <div className="mt-2">
              <span className={`text-3xl font-bold ${isNegative ? "text-red" : "text-ink-950"}`}>
                {isNegative ? "-" : ""}{formattedAmount}
              </span>
              <span className="ml-2 text-sm text-ink-400">{data.currency}</span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <User className="h-4 w-4" />
                Created by {data.createdBy}
              </span>
              <span className="text-ink-300">·</span>
              <span className="flex items-center gap-1.5">
                <CalendarDays className="h-4 w-4" />
                {data.createdAt}
              </span>
            </div>
          </div>

          {/* Provider Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Provider</h3>
            <div className="flex items-center gap-4">
              <div className="flex h-14 w-14 flex-shrink-0 items-center justify-center rounded-full bg-primary text-lg font-bold text-white shadow-lg shadow-primary/20">
                {data.providerInitials}
              </div>
              <div>
                <div className="text-base font-semibold text-ink-950">{data.providerName}</div>
                <div className="flex flex-wrap items-center gap-2 text-sm text-ink-500">
                  <span className="font-mono">{data.providerId}</span>
                  <span className="text-ink-300">·</span>
                  <span>{data.providerRole}</span>
                  <span className="text-ink-300">·</span>
                  <span>{data.providerBranch}</span>
                </div>
                <div className="mt-1 text-sm text-ink-500">
                  Balance: <span className="font-semibold text-ink-950">
                    {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.providerBalance)}
                  </span>
                </div>
              </div>
              <button
                onClick={() => router.push(`/wallet/providers/${data.providerId}`)}
                className="ml-auto flex items-center gap-1 text-xs font-medium text-primary hover:text-primary-hover transition-colors"
              >
                View Profile <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>

          {/* Approval History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Approval History</h3>
            <div className="space-y-3">
              {data.approvalHistory.map((step, index) => {
                const decisionStyle = DECISION_STYLES[step.decision] || DECISION_STYLES.Pending;
                return (
                  <div key={step.id} className="flex items-start gap-3">
                    <div className="flex flex-col items-center">
                      <div className={`h-2.5 w-2.5 rounded-full ${
                        step.decision === "Approved" ? "bg-green" :
                        step.decision === "Rejected" ? "bg-red" :
                        "bg-amber"
                      }`} />
                      {index < data.approvalHistory.length - 1 && (
                        <div className="h-full w-0.5 bg-line" />
                      )}
                    </div>
                    <div className="flex-1 pb-3">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-ink-950">{step.step}</span>
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${decisionStyle.bg} ${decisionStyle.text}`}>
                          {decisionStyle.icon}
                          {step.decision}
                        </span>
                      </div>
                      <div className="text-xs text-ink-500">
                        {step.approver} · {step.timestamp}
                      </div>
                      {step.comment && (
                        <div className="mt-1 text-xs text-ink-600 bg-surface-2 rounded-lg px-2.5 py-1.5">
                          "{step.comment}"
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Details</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.reference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Business Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.businessReference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Type</div>
                <div className="text-sm font-medium text-ink-950">{data.type}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Created</div>
                <div className="text-sm font-medium text-ink-950">{data.createdAt}</div>
              </div>
              {data.approvedAt && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Approved</div>
                  <div className="text-sm font-medium text-ink-950">{data.approvedAt}</div>
                </div>
              )}
              {data.executedAt && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Executed</div>
                  <div className="text-sm font-medium text-ink-950">{data.executedAt}</div>
                </div>
              )}
            </div>
          </div>

          {/* Reason */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Reason / Basis</h3>
            <p className="text-sm text-ink-600">{data.reason}</p>
          </div>

          {/* Metadata */}
          {data.metadata && Object.keys(data.metadata).length > 0 && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Metadata</h3>
              <div className="space-y-2">
                {data.metadata.orderRef && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Order</span>
                    <span className="font-medium text-ink-950 font-mono">{data.metadata.orderRef}</span>
                  </div>
                )}
                {data.metadata.tripRef && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Trip</span>
                    <span className="font-medium text-ink-950 font-mono">{data.metadata.tripRef}</span>
                  </div>
                )}
                {data.metadata.batchRef && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Batch</span>
                    <span className="font-medium text-ink-950 font-mono">{data.metadata.batchRef}</span>
                  </div>
                )}
                {data.metadata.notes && (
                  <div className="text-sm text-ink-600 mt-2 border-t border-line pt-2">
                    {data.metadata.notes}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => router.push(`/wallet/providers/${data.providerId}`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Users className="h-4 w-4" />
                  View Provider
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              {data.metadata.tripRef && (
                <button
                  onClick={() => router.push(`/operations/trips/${data.metadata.tripRef}`)}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <Clock className="h-4 w-4" />
                    View Trip
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <FileText className="h-4 w-4" />
                  Export Details
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <CreditCard className="h-4 w-4" />
          WAL-08 · Allocation Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.status}
        </span>
      </div>
    </div>
  );
}