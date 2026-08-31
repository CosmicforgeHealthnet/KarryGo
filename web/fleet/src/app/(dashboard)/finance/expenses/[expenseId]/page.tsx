// src/app/(dashboard)/finance/expenses/[expenseId]/page.tsx

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
  Edit3,
  XCircle,
  Building2,
  Hash,
  Tag,
  Mail,
  Phone,
  Calendar,
  Package,
  Truck,
  Receipt,
  ThumbsUp,
  ThumbsDown,
} from "lucide-react";

type ExpenseDetail = {
  id: string;
  reference: string;
  category: string;
  description: string;
  amount: number;
  currency: string;
  status: "Approved" | "Pending" | "Rejected" | "Draft";
  date: string;
  submitter: string;
  submitterId: string;
  submitterEmail: string;
  submitterPhone: string;
  branch: string;
  costCentre: string;
  receipt: boolean;
  receiptUrl: string | null;
  orderRef: string | null;
  assetRef: string | null;
  providerRef: string | null;
  approvedBy: string | null;
  approvedAt: string | null;
  rejectionReason: string | null;
  notes: string | null;
  createdBy: string;
  history: {
    id: string;
    action: string;
    status: string;
    timestamp: string;
    user: string;
    comment: string | null;
  }[];
};

// Mock data
const MOCK_EXPENSE: ExpenseDetail = {
  id: "EXP-001",
  reference: "EXP-2026-001",
  category: "Fuel",
  description: "Fuel for fleet vehicles - August week 3",
  amount: 285000,
  currency: "NGN",
  status: "Approved",
  date: "2026-08-18",
  submitter: "M. Bello",
  submitterId: "P-038",
  submitterEmail: "musa.bello@apexlogistics.com",
  submitterPhone: "+234 800 123 4567",
  branch: "Ikeja",
  costCentre: "Operations",
  receipt: true,
  receiptUrl: "/receipts/EXP-2026-001.pdf",
  orderRef: null,
  assetRef: "TR-112",
  providerRef: null,
  approvedBy: "F. Daniel",
  approvedAt: "2026-08-19",
  rejectionReason: null,
  notes: "Fuel for all fleet vehicles. 4 bikes, 2 vans, 1 truck.",
  createdBy: "M. Bello",
  history: [
    { id: "HIS-001", action: "Expense submitted", status: "Pending", timestamp: "2026-08-18 09:00", user: "M. Bello", comment: "Fuel receipt attached." },
    { id: "HIS-002", action: "Expense reviewed", status: "Approved", timestamp: "2026-08-19 10:00", user: "F. Daniel", comment: "Approved. Valid fuel usage." },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Approved: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
  },
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-4 w-4" />,
  },
};

const CATEGORY_STYLES: Record<string, { bg: string; text: string }> = {
  Fuel: { bg: "bg-amber-bg", text: "text-amber" },
  Maintenance: { bg: "bg-blue-bg", text: "text-blue" },
  "Office Supplies": { bg: "bg-green-bg", text: "text-green" },
  Software: { bg: "bg-purple-bg", text: "text-purple" },
  Travel: { bg: "bg-pink-bg", text: "text-pink" },
};

export default function ExpenseDetailPage() {
  const router = useRouter();
  const params = useParams();
  const expenseId = params.expenseId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ExpenseDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [showRejectModal, setShowRejectModal] = useState(false);
  const [rejectionReason, setRejectionReason] = useState("");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_EXPENSE, id: expenseId });
      setLoading(false);
    };
    loadData();
  }, [expenseId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading expense details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Wallet className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Expense not found</h3>
        <p className="mt-1 text-sm text-ink-500">The expense you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/finance/expenses")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to expenses
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Draft;
  const categoryStyle = CATEGORY_STYLES[data.category] || CATEGORY_STYLES.Fuel;
  const formattedAmount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.amount);
  const isPending = data.status === "Pending";
  const isApproved = data.status === "Approved";
  const isDraft = data.status === "Draft";
  const isRejected = data.status === "Rejected";

  const handleApprove = () => {
    setApproving(true);
    setTimeout(() => {
      setApproving(false);
      toast("Expense approved successfully");
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
      toast("Expense rejected");
    }, 1500);
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
            onClick={() => router.push("/finance/expenses")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Expense {data.reference}
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
          {data.receipt && data.receiptUrl && (
            <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
              <Download className="h-3.5 w-3.5" />
              Receipt
            </button>
          )}
          {isDraft && (
            <button
              onClick={() => router.push(`/finance/expenses/${data.id}/edit`)}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit
            </button>
          )}
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
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Amount Card */}
          <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                {data.category}
              </span>
              <span className="text-ink-300">·</span>
              <span>{data.costCentre}</span>
              <span className="text-ink-300">·</span>
              <span>{data.branch}</span>
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
                {data.submitter}
              </span>
              <span className="text-ink-300">·</span>
              <span className="flex items-center gap-1.5">
                <CalendarDays className="h-4 w-4" />
                {data.date}
              </span>
              {data.receipt && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1.5 text-green">
                    <CheckCircle2 className="h-4 w-4" />
                    Receipt attached
                  </span>
                </>
              )}
            </div>
          </div>

          {/* History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">History</h3>
            <div className="space-y-3">
              {data.history.map((event, index) => (
                <div key={event.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-2.5 w-2.5 rounded-full ${
                      event.status === "Approved" ? "bg-green" :
                      event.status === "Pending" ? "bg-amber" :
                      event.status === "Rejected" ? "bg-red" :
                      "bg-blue"
                    }`} />
                    {index < data.history.length - 1 && (
                      <div className="h-full w-0.5 bg-line" />
                    )}
                  </div>
                  <div className="flex-1 pb-3">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-ink-950">{event.action}</span>
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        event.status === "Approved" ? "bg-green-bg text-green" :
                        event.status === "Pending" ? "bg-amber-bg text-amber" :
                        event.status === "Rejected" ? "bg-red-bg text-red" :
                        "bg-blue-bg text-blue"
                      }`}>
                        {event.status}
                      </span>
                    </div>
                    <div className="text-xs text-ink-500">
                      {event.user} · {event.timestamp}
                    </div>
                    {event.comment && (
                      <div className="mt-1 text-xs text-ink-600 bg-surface-2 rounded-lg px-2.5 py-1.5">
                        "{event.comment}"
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Expense Details</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.reference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Category</div>
                <div className="text-sm font-medium text-ink-950">
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                    {data.category}
                  </span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Cost Centre</div>
                <div className="text-sm font-medium text-ink-950">{data.costCentre}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Branch</div>
                <div className="text-sm font-medium text-ink-950">{data.branch}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Date</div>
                <div className="text-sm font-medium text-ink-950">{data.date}</div>
              </div>
            </div>
          </div>

          {/* References */}
          {(data.orderRef || data.assetRef || data.providerRef) && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">References</h3>
              <div className="space-y-2">
                {data.orderRef && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Order</span>
                    <span className="font-medium text-ink-950 font-mono">{data.orderRef}</span>
                  </div>
                )}
                {data.assetRef && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Asset</span>
                    <span className="font-medium text-ink-950 font-mono">{data.assetRef}</span>
                  </div>
                )}
                {data.providerRef && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Provider</span>
                    <span className="font-medium text-ink-950 font-mono">{data.providerRef}</span>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Submitter Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Submitter</h3>
            <div className="space-y-2">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-primary text-xs font-bold text-white">
                  {data.submitter.split(" ").map((n) => n[0]).join("")}
                </div>
                <div>
                  <div className="text-sm font-medium text-ink-950">{data.submitter}</div>
                  <div className="text-xs text-ink-500 font-mono">{data.submitterId}</div>
                </div>
              </div>
              <div className="flex items-center gap-2 text-xs text-ink-500">
                <Mail className="h-3.5 w-3.5" />
                {data.submitterEmail}
              </div>
              <div className="flex items-center gap-2 text-xs text-ink-500">
                <Phone className="h-3.5 w-3.5" />
                {data.submitterPhone}
              </div>
            </div>
          </div>

          {/* Notes */}
          {data.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
              <p className="text-sm text-ink-600">{data.notes}</p>
            </div>
          )}

          {/* Rejection Reason */}
          {data.rejectionReason && (
            <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-red mb-2">Rejection Reason</h3>
              <p className="text-sm text-red/80">{data.rejectionReason}</p>
            </div>
          )}

          {/* Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              {data.receipt && data.receiptUrl && (
                <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                  <span className="flex items-center gap-2">
                    <FileText className="h-4 w-4" />
                    View Receipt
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              {data.orderRef && (
                <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                  <span className="flex items-center gap-2">
                    <Package className="h-4 w-4" />
                    View Order
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              {data.assetRef && (
                <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                  <span className="flex items-center gap-2">
                    <Truck className="h-4 w-4" />
                    View Asset
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              {data.providerRef && (
                <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                  <span className="flex items-center gap-2">
                    <Users className="h-4 w-4" />
                    View Provider
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
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
              <h3 className="text-lg font-semibold text-ink-950">Reject Expense</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Please provide a reason for rejecting this expense.
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                Rejection Reason
              </label>
              <textarea
                value={rejectionReason}
                onChange={(e) => setRejectionReason(e.target.value)}
                placeholder="Explain why this expense is being rejected..."
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
                disabled={rejecting || !rejectionReason.trim()}
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
          FIN-07 · Expense Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.status}
        </span>
      </div>
    </div>
  );
}