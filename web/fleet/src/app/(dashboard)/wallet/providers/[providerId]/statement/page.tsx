// src/app/(dashboard)/wallet/providers/[providerId]/statement/page.tsx

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
  Search,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
} from "lucide-react";

type StatementEntry = {
  id: string;
  date: string;
  type: "Allocation" | "Adjustment" | "Reversal" | "Withdrawal";
  description: string;
  amount: number;
  balance: number;
  reference: string;
  status: "Completed" | "Pending" | "Reconciling";
};

type ProviderStatement = {
  providerId: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  period: string;
  openingBalance: number;
  closingBalance: number;
  currency: string;
  entries: StatementEntry[];
  summary: {
    totalAllocations: number;
    totalAdjustments: number;
    totalReversals: number;
    totalWithdrawals: number;
    netChange: number;
  };
};

// Mock data
const MOCK_STATEMENT: ProviderStatement = {
  providerId: "P-044",
  name: "Peter Okafor",
  initials: "PO",
  role: "Dispatch Rider",
  branch: "Ikeja",
  period: "Aug 2026",
  openingBalance: 184000,
  closingBalance: 284000,
  currency: "NGN",
  summary: {
    totalAllocations: 342000,
    totalAdjustments: -5000,
    totalReversals: -20000,
    totalWithdrawals: -58000,
    netChange: 100000,
  },
  entries: [
    { id: "ENT-001", date: "2026-08-21 09:30", type: "Allocation", description: "Trip settlement - KG-8491", amount: 120000, balance: 284000, reference: "KG-8491", status: "Completed" },
    { id: "ENT-002", date: "2026-08-21 08:45", type: "Withdrawal", description: "Withdrawal request - W-1042", amount: -58000, balance: 164000, reference: "W-1042", status: "Pending" },
    { id: "ENT-003", date: "2026-08-21 08:00", type: "Allocation", description: "Trip settlement - KG-8487", amount: 85000, balance: 222000, reference: "KG-8487", status: "Completed" },
    { id: "ENT-004", date: "2026-08-20 16:30", type: "Adjustment", description: "Fuel reimbursement adjustment", amount: -5000, balance: 137000, reference: "ADJ-001", status: "Pending" },
    { id: "ENT-005", date: "2026-08-20 14:00", type: "Allocation", description: "Trip settlement - KG-8479", amount: 45000, balance: 142000, reference: "KG-8479", status: "Reconciling" },
    { id: "ENT-006", date: "2026-08-20 10:30", type: "Reversal", description: "Overpayment reversal", amount: -20000, balance: 97000, reference: "REV-001", status: "Completed" },
    { id: "ENT-007", date: "2026-08-19 15:00", type: "Allocation", description: "Trip settlement - KG-8466", amount: 32000, balance: 117000, reference: "KG-8466", status: "Completed" },
    { id: "ENT-008", date: "2026-08-19 11:00", type: "Allocation", description: "Trip settlement - KG-8452", amount: 28000, balance: 85000, reference: "KG-8452", status: "Completed" },
    { id: "ENT-009", date: "2026-08-18 16:00", type: "Allocation", description: "Trip settlement - KG-8441", amount: 32000, balance: 57000, reference: "KG-8441", status: "Completed" },
  ],
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
};

const TYPE_ICONS: Record<string, { icon: React.ReactNode; color: string }> = {
  Allocation: { icon: <ArrowDownRight className="h-3.5 w-3.5" />, color: "text-green" },
  Adjustment: { icon: <RefreshCw className="h-3.5 w-3.5" />, color: "text-amber" },
  Reversal: { icon: <ArrowUpRight className="h-3.5 w-3.5" />, color: "text-red" },
  Withdrawal: { icon: <Wallet className="h-3.5 w-3.5" />, color: "text-red" },
};

const TYPE_FILTERS = ["All", "Allocation", "Adjustment", "Reversal", "Withdrawal"];
const STATUS_FILTERS = ["All", "Completed", "Pending", "Reconciling"];

export default function ProviderStatementPage() {
  const router = useRouter();
  const params = useParams();
  const providerId = params.providerId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProviderStatement | null>(null);
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_STATEMENT, providerId });
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
        <p className="mt-3 text-sm text-ink-600">Loading provider statement...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileText className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Statement not found</h3>
        <p className="mt-1 text-sm text-ink-500">No statement exists for provider {providerId}.</p>
        <button
          onClick={() => router.push(`/wallet/providers/${providerId}`)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to provider
        </button>
      </div>
    );
  }

  const filteredEntries = data.entries.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesType && matchesStatus;
  });

  const hasActiveFilters = selectedType !== "All" || selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedStatus("All");
  };

  const formattedOpening = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.openingBalance);

  const formattedClosing = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.closingBalance);

  const netChange = data.closingBalance - data.openingBalance;
  const isPositive = netChange > 0;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/wallet/providers/${providerId}`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Provider Statement
              </h1>
              <span className="font-mono text-xs text-ink-400 sm:text-sm">
                {data.providerId}
              </span>
              <span className="flex items-center gap-1 rounded-full bg-primary-bg px-2.5 py-0.5 text-[10px] font-medium text-primary">
                {data.period}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.name} · {data.role} · {data.branch}</p>
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
            <Download className="h-3.5 w-3.5" />
            Export PDF
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <FileText className="h-3.5 w-3.5" />
            CSV
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Opening Balance</div>
          <div className="text-lg font-bold text-ink-950">{formattedOpening}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Closing Balance</div>
          <div className="text-lg font-bold text-ink-950">{formattedClosing}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Net Change</div>
          <div className={`text-lg font-bold ${isPositive ? "text-green" : "text-red"}`}>
            {isPositive ? "+" : ""}
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(Math.abs(netChange))}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Entries</div>
          <div className="text-lg font-bold text-ink-950">{data.entries.length}</div>
        </div>
      </div>

      {/* ===== SUMMARY ROW ===== */}
      <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
        <div className="flex flex-wrap items-center gap-6 text-sm">
          <div>
            <span className="text-ink-500">Total Allocations:</span>
            <span className="ml-1 font-semibold text-green">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalAllocations)}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Adjustments:</span>
            <span className="ml-1 font-semibold text-amber">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(Math.abs(data.summary.totalAdjustments))}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Reversals:</span>
            <span className="ml-1 font-semibold text-red">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(Math.abs(data.summary.totalReversals))}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Withdrawals:</span>
            <span className="ml-1 font-semibold text-red">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(Math.abs(data.summary.totalWithdrawals))}
            </span>
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by description or reference..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Allocation", "Adjustment", "Reversal", "Withdrawal"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Completed", "Pending", "Reconciling"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertCircle className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedType !== "All" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</div>
              <div className="flex gap-1.5">
                {TYPE_FILTERS.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedType(type)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedType === type
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {type}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
              <div className="flex gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedStatus === status
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== STATEMENT TABLE ===== */}
      {filteredEntries.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileText className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No entries found</h3>
          <p className="mt-1 text-sm text-ink-500">No statement entries match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[800px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Date</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Description</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Balance</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                </tr>
              </thead>
              <tbody>
                {filteredEntries.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Completed;
                  const typeStyle = TYPE_ICONS[item.type] || TYPE_ICONS.Allocation;
                  const isCredit = item.amount > 0;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(Math.abs(item.amount));
                  const formattedBalance = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.balance);

                  return (
                    <tr key={item.id} className="border-b border-line hover:bg-surface-2 transition-colors">
                      <td className="px-4 py-3 text-ink-600 text-[12px]">{item.date}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.color}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950">{item.description}</div>
                        <div className="text-[11px] text-ink-500 font-mono">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`font-semibold ${isCredit ? "text-green" : "text-red"}`}>
                          {isCredit ? "+" : "-"} {formattedAmount}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-semibold text-ink-950">{formattedBalance}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileText className="h-4 w-4" />
          WAL-05 · Provider Statement
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredEntries.length} entries shown
        </span>
      </div>
    </div>
  );
}