// src/app/(dashboard)/wallet/providers/[providerId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  User,
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
  Plus,
  ArrowUpRight,
  ArrowDownRight,
  CreditCard,
} from "lucide-react";

type ProviderSubWalletDetail = {
  providerId: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  balance: number;
  currency: string;
  state: "Active" | "Pending" | "Suspended" | "Reconciling";
  lastUpdated: string;
  allocationCount: number;
  withdrawalCount: number;
  statementAvailable: boolean;
  recentAllocations: {
    id: string;
    type: "Allocation" | "Adjustment" | "Reversal";
    amount: number;
    description: string;
    status: "Completed" | "Pending" | "Reconciling";
    timestamp: string;
    reference: string;
  }[];
  stats: {
    totalAllocated: number;
    totalWithdrawn: number;
    pendingAllocations: number;
    availableBalance: number;
  };
};

// Mock data
const MOCK_PROVIDER: ProviderSubWalletDetail = {
  providerId: "P-044",
  name: "Peter Okafor",
  initials: "PO",
  role: "Dispatch Rider",
  branch: "Ikeja",
  balance: 284000,
  currency: "NGN",
  state: "Active",
  lastUpdated: "12 sec ago",
  allocationCount: 8,
  withdrawalCount: 3,
  statementAvailable: true,
  recentAllocations: [
    { id: "ALL-001", type: "Allocation", amount: 120000, description: "Trip settlement - KG-8491", status: "Completed", timestamp: "12 sec ago", reference: "KG-8491" },
    { id: "ALL-002", type: "Allocation", amount: 85000, description: "Trip settlement - KG-8487", status: "Completed", timestamp: "28 min ago", reference: "KG-8487" },
    { id: "ALL-003", type: "Adjustment", amount: -5000, description: "Fuel reimbursement adjustment", status: "Pending", timestamp: "1 hour ago", reference: "ADJ-001" },
    { id: "ALL-004", type: "Allocation", amount: 45000, description: "Trip settlement - KG-8479", status: "Reconciling", timestamp: "2 hours ago", reference: "KG-8479" },
    { id: "ALL-005", type: "Reversal", amount: -20000, description: "Overpayment reversal", status: "Completed", timestamp: "1 day ago", reference: "REV-001" },
  ],
  stats: {
    totalAllocated: 342000,
    totalWithdrawn: 58000,
    pendingAllocations: 5000,
    availableBalance: 284000,
  },
};

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
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
  Suspended: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Reconciling: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <RefreshCw className="h-4 w-4" />,
  },
};

const ALLOCATION_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
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
};

const ALLOCATION_TYPE_ICONS: Record<string, { icon: React.ReactNode; color: string }> = {
  Allocation: { icon: <ArrowDownRight className="h-3.5 w-3.5" />, color: "text-green" },
  Adjustment: { icon: <RefreshCw className="h-3.5 w-3.5" />, color: "text-amber" },
  Reversal: { icon: <ArrowUpRight className="h-3.5 w-3.5" />, color: "text-red" },
};

export default function ProviderSubWalletDetailPage() {
  const router = useRouter();
  const params = useParams();
  const providerId = params.providerId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProviderSubWalletDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_PROVIDER, providerId });
      setLoading(false);
    };
    loadData();
  }, [providerId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading provider wallet...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <User className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Provider not found</h3>
        <p className="mt-1 text-sm text-ink-500">No sub-wallet exists for provider {providerId}.</p>
        <button
          onClick={() => router.push("/wallet/providers")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to providers
        </button>
      </div>
    );
  }

  const stateStyle = STATE_STYLES[data.state] || STATE_STYLES.Active;
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
  }).format(data.stats.availableBalance);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/wallet/providers")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {data.name}
              </h1>
              <span className="font-mono text-xs text-ink-400 sm:text-sm">
                {data.providerId}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${stateStyle.bg} ${stateStyle.text}`}
              >
                {stateStyle.icon}
                {data.state}
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500 sm:text-sm">
              <span className="flex items-center gap-1.5">
                <Briefcase className="h-3.5 w-3.5" />
                {data.role}
              </span>
              <span className="flex items-center gap-1.5">
                <MapPin className="h-3.5 w-3.5" />
                {data.branch}
              </span>
              <span className="flex items-center gap-1.5">
                <Users className="h-3.5 w-3.5" />
                {data.allocationCount} allocations
              </span>
              <span className="flex items-center gap-1.5">
                <Wallet className="h-3.5 w-3.5" />
                {data.withdrawalCount} withdrawals
              </span>
            </div>
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
          <button
            onClick={() => router.push(`/wallet/providers/${providerId}/statement`)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <FileText className="h-3.5 w-3.5" />
            Statement
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            Allocate
          </button>
        </div>
      </div>

      {/* ===== BALANCE CARDS ===== */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div className="group rounded-2xl border border-line bg-gradient-to-br from-primary/5 to-transparent p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Balance</div>
          <div className="mt-1 text-2xl font-bold text-ink-950 sm:text-3xl">
            {formattedBalance}
          </div>
          <div className="mt-2 flex items-center gap-1.5 text-xs text-ink-400">
            <span className="text-green">
              <TrendingUp className="h-3 w-3" />
            </span>
            <span>+{data.allocationCount} allocations</span>
          </div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Available</div>
          <div className="mt-1 text-2xl font-bold text-green sm:text-3xl">
            {formattedAvailable}
          </div>
          <div className="mt-2 text-xs text-ink-400">Ready for withdrawal</div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Allocated</div>
          <div className="mt-1 text-2xl font-bold text-ink-950 sm:text-3xl">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.stats.totalAllocated)}
          </div>
          <div className="mt-2 text-xs text-ink-400">Lifetime allocations</div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[11px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Withdrawn</div>
          <div className="mt-1 text-2xl font-bold text-red sm:text-3xl">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.stats.totalWithdrawn)}
          </div>
          <div className="mt-2 text-xs text-ink-400">{data.withdrawalCount} withdrawals</div>
        </div>
      </div>

      {/* ===== RECENT ALLOCATIONS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-[15px] font-semibold text-ink-950">Recent Allocations</h3>
          <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1">
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="space-y-2">
          {data.recentAllocations.map((item) => {
            const statusStyle = ALLOCATION_STATUS_STYLES[item.status] || ALLOCATION_STATUS_STYLES.Completed;
            const typeStyle = ALLOCATION_TYPE_ICONS[item.type] || ALLOCATION_TYPE_ICONS.Allocation;
            const isCredit = item.amount > 0;
            const formattedAmount = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(Math.abs(item.amount));

            return (
              <div
                key={item.id}
                className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2 group"
                onClick={() => router.push(`/wallet/allocations/${item.id}`)}
              >
                <div className="flex items-center gap-3">
                  <span className={typeStyle.color}>{typeStyle.icon}</span>
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{item.description}</div>
                    <div className="flex items-center gap-2 text-[11px] text-ink-500">
                      <span className="font-mono">{item.reference}</span>
                      <span className="text-ink-300">·</span>
                      <span>{item.timestamp}</span>
                    </div>
                  </div>
                </div>
                <div className="text-right">
                  <div className={`text-sm font-semibold ${isCredit ? "text-green" : "text-red"}`}>
                    {isCredit ? "+" : "-"} {formattedAmount}
                  </div>
                  <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                    {item.status}
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
          <User className="h-4 w-4" />
          WAL-04 · Provider Sub-wallet Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.allocationCount} allocations
        </span>
      </div>
    </div>
  );
}