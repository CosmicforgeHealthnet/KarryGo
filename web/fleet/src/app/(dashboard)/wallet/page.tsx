// src/app/(dashboard)/wallet/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
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
  ArrowUpRight,
  ArrowDownRight,
  CreditCard,
  Users,
  FileText,
  MoreHorizontal,
  CalendarDays,
  Plus,
} from "lucide-react";

type WalletOverview = {
  balance: number;
  currency: string;
  available: number;
  pending: number;
  reserved: number;
  lastUpdated: string;
  state: "Active" | "Pending" | "Reconciling" | "Unavailable";
  recentTransactions: {
    id: string;
    type: "Credit" | "Debit" | "Allocation" | "Withdrawal";
    description: string;
    amount: number;
    status: "Completed" | "Pending" | "Reconciling" | "Failed";
    timestamp: string;
    reference: string;
  }[];
  stats: {
    totalIn: number;
    totalOut: number;
    allocationCount: number;
    withdrawalCount: number;
  };
};

// Mock data
const MOCK_WALLET: WalletOverview = {
  balance: 18420000,
  currency: "NGN",
  available: 17280000,
  pending: 840000,
  reserved: 300000,
  lastUpdated: "12 sec ago",
  state: "Active",
  recentTransactions: [
    { id: "TX-88021", type: "Credit", description: "Trip settlement - KG-8491", amount: 18400, status: "Completed", timestamp: "12 sec ago", reference: "KG-8491" },
    { id: "TX-88018", type: "Withdrawal", description: "Withdrawal to UBA ••3901", amount: -750000, status: "Pending", timestamp: "4 min ago", reference: "W-1042" },
    { id: "TX-88011", type: "Allocation", description: "Provider allocation - P. Okafor", amount: -120000, status: "Completed", timestamp: "16 min ago", reference: "ALL-001" },
    { id: "TX-87996", type: "Credit", description: "Refund - Private order", amount: 42000, status: "Reconciling", timestamp: "31 min ago", reference: "PO-1092" },
    { id: "TX-87988", type: "Debit", description: "Provider allocation - M. Bello", amount: -85000, status: "Completed", timestamp: "1 hour ago", reference: "ALL-002" },
  ],
  stats: {
    totalIn: 8420000,
    totalOut: 3680000,
    allocationCount: 8,
    withdrawalCount: 3,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Reconciling: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const TRANSACTION_TYPE_ICONS: Record<string, { icon: React.ReactNode; color: string }> = {
  Credit: { icon: <ArrowDownRight className="h-3.5 w-3.5" />, color: "text-green" },
  Debit: { icon: <ArrowUpRight className="h-3.5 w-3.5" />, color: "text-red" },
  Allocation: { icon: <Users className="h-3.5 w-3.5" />, color: "text-blue" },
  Withdrawal: { icon: <Wallet className="h-3.5 w-3.5" />, color: "text-amber" },
};

const WALLET_STATE_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Reconciling: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Unavailable: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function WalletOverviewPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WalletOverview | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_WALLET);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading wallet data...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Wallet className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Wallet unavailable</h3>
        <p className="mt-1 text-sm text-ink-500">The wallet data is currently unavailable. Please try again later.</p>
        <button
          onClick={() => setSecondsAgo(0)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          <RefreshCw className="inline h-3.5 w-3.5 mr-1" />
          Refresh
        </button>
      </div>
    );
  }

  const stateStyle = WALLET_STATE_STYLES[data.state] || WALLET_STATE_STYLES.Active;
  const formattedBalance = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.balance);

  const formattedAvailable = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.available);

  const formattedPending = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.pending);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
              Company Wallet
            </h1>
            <span
              className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${stateStyle.bg} ${stateStyle.text}`}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${stateStyle.dot}`} />
              {data.state}
            </span>
          </div>
          <p className="mt-1 text-sm text-ink-600">
            Authoritative wallet balance and transaction history.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button
            onClick={() => router.push("/wallet/allocations/create")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Allocation
          </button>
        </div>
      </div>

      {/* ===== BALANCE CARDS ===== */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div className="group rounded-2xl border border-line bg-gradient-to-br from-primary/5 to-transparent p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Balance</div>
          <div className="mt-1 text-2xl font-bold text-ink-950 sm:text-3xl">
            {formattedBalance}
          </div>
          <div className="mt-2 flex items-center gap-1.5 text-xs text-ink-400">
            <span className={`flex items-center gap-0.5 ${data.balance > 0 ? "text-green" : "text-red"}`}>
              {data.balance > 0 ? <TrendingUp className="h-3 w-3" /> : <TrendingDown className="h-3 w-3" />}
              {data.balance > 0 ? "+" : ""}
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(Math.abs(data.balance))}
            </span>
            <span>· {data.currency}</span>
          </div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Available</div>
          <div className="mt-1 text-2xl font-bold text-green sm:text-3xl">
            {formattedAvailable}
          </div>
          <div className="mt-2 text-xs text-ink-400">Ready for use</div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="mt-1 text-2xl font-bold text-amber sm:text-3xl">
            {formattedPending}
          </div>
          <div className="mt-2 text-xs text-ink-400">Awaiting confirmation</div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Reserved</div>
          <div className="mt-1 text-2xl font-bold text-ink-950 sm:text-3xl">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.reserved)}
          </div>
          <div className="mt-2 text-xs text-ink-400">Held for commitments</div>
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Inflows</div>
          <div className="text-lg font-bold text-green">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.stats.totalIn)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Outflows</div>
          <div className="text-lg font-bold text-red">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.stats.totalOut)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Allocations</div>
          <div className="text-lg font-bold text-ink-950">{data.stats.allocationCount}</div>
          <div className="text-[10px] text-ink-400">Total instructions</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Withdrawals</div>
          <div className="text-lg font-bold text-ink-950">{data.stats.withdrawalCount}</div>
          <div className="text-[10px] text-ink-400">Requests</div>
        </div>
      </div>

      {/* ===== RECENT TRANSACTIONS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-[15px] font-semibold text-ink-950">Recent Transactions</h3>
          <button
            onClick={() => router.push("/wallet/transactions")}
            className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="space-y-2">
          {data.recentTransactions.map((tx) => {
            const statusStyle = STATUS_STYLES[tx.status] || STATUS_STYLES.Completed;
            const typeStyle = TRANSACTION_TYPE_ICONS[tx.type] || TRANSACTION_TYPE_ICONS.Credit;
            const isCredit = tx.amount > 0;
            const formattedAmount = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(Math.abs(tx.amount));

            return (
              <div
                key={tx.id}
                onClick={() => router.push(`/wallet/transactions/${tx.id}`)}
                className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2 group"
              >
                <div className="flex items-center gap-3">
                  <div className={`${typeStyle.color}`}>{typeStyle.icon}</div>
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{tx.description}</div>
                    <div className="flex items-center gap-2 text-[11px] text-ink-500">
                      <span className="font-mono">{tx.reference}</span>
                      <span className="text-ink-300">·</span>
                      <span>{tx.timestamp}</span>
                    </div>
                  </div>
                </div>
                <div className="text-right">
                  <div className={`text-sm font-semibold ${isCredit ? "text-green" : "text-red"}`}>
                    {isCredit ? "+" : "-"} {formattedAmount}
                  </div>
                  <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                    {tx.status}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Wallet className="h-4 w-4" />
          WAL-01 · Company Wallet Overview
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Authoritative balance from Payment Wallet
        </span>
      </div>
    </div>
  );
}