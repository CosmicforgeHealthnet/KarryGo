// src/app/(dashboard)/wallet/transactions/[transactionId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  ArrowLeftRight,
  Clock,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Eye,
  ArrowRight,
  Wallet,
  Users,
  TrendingUp,
  TrendingDown,
  ArrowUpRight,
  ArrowDownRight,
  FileText,
  Printer,
  Download,
  CalendarDays,
  User,
  Building2,
  Hash,
  Tag,
  MoreHorizontal,
} from "lucide-react";

type TransactionDetail = {
  id: string;
  type: "Credit" | "Debit" | "Allocation" | "Withdrawal";
  description: string;
  amount: number;
  status: "Completed" | "Pending" | "Reconciling" | "Failed";
  timestamp: string;
  reference: string;
  category: string;
  counterparty: string;
  sourceVersion: string;
  freshness: string;
  metadata: {
    orderRef?: string;
    providerId?: string;
    tripRef?: string;
    batchRef?: string;
    notes?: string;
  };
  history: {
    date: string;
    action: string;
    status: string;
    user: string;
  }[];
};

// Mock data
const MOCK_TRANSACTION: TransactionDetail = {
  id: "TX-88021",
  type: "Credit",
  description: "Trip settlement - KG-8491",
  amount: 18400,
  status: "Completed",
  timestamp: "12 sec ago",
  reference: "KG-8491",
  category: "Trip Settlement",
  counterparty: "Dispatch Service",
  sourceVersion: "v2.4.1",
  freshness: "Authoritative",
  metadata: {
    orderRef: "OR-129",
    providerId: "P-044",
    tripRef: "KG-8491",
    notes: "Settlement completed successfully. Provider Peter Okafor.",
  },
  history: [
    { date: "2026-08-21 09:30", action: "Transaction created", status: "Pending", user: "System" },
    { date: "2026-08-21 09:31", action: "Authorized", status: "Completed", user: "F. Daniel" },
    { date: "2026-08-21 09:31", action: "Settled", status: "Completed", user: "System" },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Completed: {
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
  Reconciling: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <RefreshCw className="h-4 w-4" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
};

const TYPE_ICONS: Record<string, { icon: React.ReactNode; color: string }> = {
  Credit: { icon: <ArrowDownRight className="h-4 w-4" />, color: "text-green" },
  Debit: { icon: <ArrowUpRight className="h-4 w-4" />, color: "text-red" },
  Allocation: { icon: <Users className="h-4 w-4" />, color: "text-blue" },
  Withdrawal: { icon: <Wallet className="h-4 w-4" />, color: "text-amber" },
};

export default function TransactionDetailPage() {
  const router = useRouter();
  const params = useParams();
  const transactionId = params.transactionId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<TransactionDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_TRANSACTION, id: transactionId });
      setLoading(false);
    };
    loadData();
  }, [transactionId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading transaction details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <ArrowLeftRight className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Transaction not found</h3>
        <p className="mt-1 text-sm text-ink-500">The transaction you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/wallet/transactions")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to transactions
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Completed;
  const typeStyle = TYPE_ICONS[data.type] || TYPE_ICONS.Credit;
  const isCredit = data.amount > 0;
  const formattedAmount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(Math.abs(data.amount));

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/wallet/transactions")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Transaction {data.id}
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <FileText className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Amount Card */}
          <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span className={typeStyle.color}>{typeStyle.icon}</span>
              <span>{data.type}</span>
              <span className="text-ink-300">·</span>
              <span>{data.category}</span>
            </div>
            <div className="mt-2">
              <span className={`text-3xl font-bold ${isCredit ? "text-green" : "text-red"}`}>
                {isCredit ? "+" : "-"} {formattedAmount}
              </span>
              <span className="ml-2 text-sm text-ink-400">NGN</span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <Clock className="h-4 w-4" />
                {data.timestamp}
              </span>
              <span className="text-ink-300">·</span>
              <span className="flex items-center gap-1.5">
                <Hash className="h-4 w-4" />
                {data.reference}
              </span>
              <span className="text-ink-300">·</span>
              <span className="flex items-center gap-1.5">
                <Tag className="h-4 w-4" />
                {data.sourceVersion}
              </span>
            </div>
          </div>

          {/* History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">History</h3>
            <div className="space-y-3">
              {data.history.map((event, index) => (
                <div key={index} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className="h-2 w-2 rounded-full bg-primary mt-1.5" />
                    {index < data.history.length - 1 && (
                      <div className="h-full w-0.5 bg-line" />
                    )}
                  </div>
                  <div className="flex-1 pb-3">
                    <div className="text-sm font-medium text-ink-950">{event.action}</div>
                    <div className="flex items-center gap-2 text-xs text-ink-500">
                      <span>{event.date}</span>
                      <span className="text-ink-300">·</span>
                      <span>{event.user}</span>
                      <span className="text-ink-300">·</span>
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        event.status === "Completed" ? "bg-green-bg text-green" :
                        event.status === "Pending" ? "bg-amber-bg text-amber" :
                        "bg-blue-bg text-blue"
                      }`}>
                        {event.status}
                      </span>
                    </div>
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
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Transaction Details</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Counterparty</div>
                <div className="text-sm font-medium text-ink-950">{data.counterparty}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Category</div>
                <div className="text-sm font-medium text-ink-950">{data.category}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.reference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Source Version</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.sourceVersion}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Freshness</div>
                <div className="text-sm font-medium text-ink-950">{data.freshness}</div>
              </div>
            </div>
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
                {data.metadata.providerId && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-500">Provider</span>
                    <span className="font-medium text-ink-950 font-mono">{data.metadata.providerId}</span>
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
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <Eye className="h-4 w-4" />
                  View Related Record
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <RefreshCw className="h-4 w-4" />
                  Refresh
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
          <ArrowLeftRight className="h-4 w-4" />
          WAL-03 · Transaction Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Read-only
        </span>
      </div>
    </div>
  );
}