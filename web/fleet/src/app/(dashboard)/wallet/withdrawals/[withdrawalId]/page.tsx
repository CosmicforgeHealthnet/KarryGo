// src/app/(dashboard)/wallet/withdrawals/[withdrawalId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Wallet,
  TrendingUp,
  TrendingDown,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  Users,
  MapPin,
  Briefcase,
  CalendarDays,
  MoreHorizontal,
  Download,
  FileText,
  Printer,
  User,
  DollarSign,
  Send,
  ThumbsUp,
  ThumbsDown,
  XCircle,
  CreditCard,
  Building2,
  Hash,
  Tag,
} from "lucide-react";

type WithdrawalDetail = {
  id: string;
  reference: string;
  providerId: string;
  providerName: string;
  providerInitials: string;
  providerRole: string;
  providerBranch: string;
  type: "Provider" | "Company";
  amount: number;
  currency: string;
  status: "Pending" | "Approved" | "Processing" | "Completed" | "Rejected" | "Reconciling";
  description: string;
  destination: string;
  bankReference: string;
  createdAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  processedAt: string | null;
  completedAt: string | null;
  rejectionReason: string | null;
  reservationState: string;
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
    batchRef?: string;
    notes?: string;
  };
};

// Mock data
const MOCK_WITHDRAWAL: WithdrawalDetail = {
  id: "W-1042",
  reference: "W-1042",
  providerId: "P-044",
  providerName: "Peter Okafor",
  providerInitials: "PO",
  providerRole: "Dispatch Rider",
  providerBranch: "Ikeja",
  type: "Provider",
  amount: 750000,
  currency: "NGN",
  status: "Pending",
  description: "Monthly payout - August 2026",
  destination: "UBA ••3901",
  bankReference: "UBA ••3901",
  createdAt: "2026-08-21 09:30",
  createdBy: "P. Okafor",
  approvedBy: null,
  approvedAt: null,
  processedAt: null,
  completedAt: null,
  rejectionReason: null,
  reservationState: "Reserved",
  approvalHistory: [
    { id: "AP-001", step: "Requester", approver: "P. Okafor", decision: "Approved", timestamp: "2026-08-21 09:30", comment: "Withdrawal request submitted." },
    { id: "AP-002", step: "Manager Review", approver: "F. Daniel", decision: "Pending", timestamp: "2026-08-21 09:35", comment: null },
    { id: "AP-003", step: "Finance Approval", approver: "O. Adebayo", decision: "Pending", timestamp: "2026-08-21 09:40", comment: null },
  ],
  metadata: {
    notes: "Monthly payout for completed trips in August 2026.",
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
  Processing: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-4 w-4" />,
  },
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Reconciling: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-4 w-4" />,
  },
};

const DECISION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <ThumbsUp className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <ThumbsDown className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
};

export default function WithdrawalDetailPage() {
  const router = useRouter();
  const params = useParams();
  const withdrawalId = params.withdrawalId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WithdrawalDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [showRejectModal, setShowRejectModal] = useState(false);
  const [rejectionReason, setRejectionReason] = useState("");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_WITHDRAWAL, id: withdrawalId });
      setLoading(false);
    };
    loadData();
  }, [withdrawalId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading withdrawal details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Wallet className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Withdrawal not found</h3>
        <p className="mt-1 text-sm text-ink-500">The withdrawal request you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/wallet/withdrawals")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to withdrawals
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Pending;
  const formattedAmount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.amount);
  const isPending = data.status === "Pending";
  const isApproved = data.status === "Approved";
  const isProcessing = data.status === "Processing";
  const isCompleted = data.status === "Completed";
  const isRejected = data.status === "Rejected";

  const handleApprove = () => {
    setApproving(true);
    setTimeout(() => {
      setApproving(false);
      toast("Withdrawal approved successfully");
    }, 1500);
  };

  const handleReject = () => {
    if (!rejectionReason.trim()) {
      toast("Please provide a rejection reason");
      return;
    }
    setRejecting(true);
    setTimeout(() => {
      setRejecting(false);
      setShowRejectModal(false);
      setRejectionReason("");
      toast("Withdrawal rejected");
    }, 1500);
  };

  const handleReplay = () => {
    toast("Replay requested for dead-lettered withdrawal");
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
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/wallet/withdrawals")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Withdrawal {data.reference}
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
                onClick={() => setShowRejectModal(true)}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg/50 px-3.5 text-[11.5px] font-medium text-red transition hover:bg-red-bg"
              >
                <ThumbsDown className="h-3.5 w-3.5" />
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
          {isRejected && (
            <button
              onClick={handleReplay}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Replay
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
              <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                data.type === "Provider" ? "bg-blue-bg text-blue" : "bg-green-bg text-green"
              }`}>
                {data.type}
              </span>
              <span className="text-ink-300">·</span>
              <span>{data.destination}</span>
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formattedAmount}
              </span>
              <span className="ml-2 text-sm text-ink-400">{data.currency}</span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <User className="h-4 w-4" />
                {data.createdBy}
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
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Requestor</h3>
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
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Withdrawal Details</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.reference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Destination</div>
                <div className="text-sm font-medium text-ink-950">{data.destination}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Bank Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.bankReference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reservation State</div>
                <div className="text-sm font-medium text-ink-950">{data.reservationState}</div>
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
              {data.processedAt && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Processed</div>
                  <div className="text-sm font-medium text-ink-950">{data.processedAt}</div>
                </div>
              )}
              {data.completedAt && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Completed</div>
                  <div className="text-sm font-medium text-ink-950">{data.completedAt}</div>
                </div>
              )}
              {data.rejectionReason && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Rejection Reason</div>
                  <div className="text-sm font-medium text-red">{data.rejectionReason}</div>
                </div>
              )}
            </div>
          </div>

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
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <FileText className="h-4 w-4" />
                  Export Details
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <Printer className="h-4 w-4" />
                  Print Receipt
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== REJECT MODAL ===== */}
      {showRejectModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-red" />
              <h3 className="text-lg font-semibold text-ink-950">Reject Withdrawal</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Please provide a reason for rejecting this withdrawal request.
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                Rejection Reason
              </label>
              <textarea
                value={rejectionReason}
                onChange={(e) => setRejectionReason(e.target.value)}
                placeholder="Explain why this withdrawal is being rejected..."
                rows={3}
                className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none"
              />
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowRejectModal(false);
                  setRejectionReason("");
                }}
                className="flex-1 rounded-lg border border-line bg-white/70 px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-white"
              >
                Cancel
              </button>
              <button
                onClick={handleReject}
                disabled={rejecting}
                className={`flex-1 rounded-lg px-4 py-2 text-sm font-medium text-white transition ${
                  rejecting || !rejectionReason.trim()
                    ? "bg-ink-300 cursor-not-allowed"
                    : "bg-red hover:bg-red/90"
                }`}
              >
                {rejecting ? <RefreshCw className="h-4 w-4 animate-spin inline mr-1" /> : null}
                Reject
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Wallet className="h-4 w-4" />
          WAL-11 · Withdrawal Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.status}
        </span>
      </div>
    </div>
  );
}