// src/app/(dashboard)/wallet/transactions/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeftRight,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Wallet,
  Users,
  ArrowUpRight,
  ArrowDownRight,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
} from "lucide-react";

type Transaction = {
  id: string;
  type: "Credit" | "Debit" | "Allocation" | "Withdrawal";
  description: string;
  amount: number;
  status: "Completed" | "Pending" | "Reconciling" | "Failed";
  timestamp: string;
  reference: string;
  category: string;
  counterparty: string;
};

// Mock data
const TRANSACTIONS: Transaction[] = [
  { id: "TX-88021", type: "Credit", description: "Trip settlement - KG-8491", amount: 18400, status: "Completed", timestamp: "12 sec ago", reference: "KG-8491", category: "Trip Settlement", counterparty: "Dispatch Service" },
  { id: "TX-88018", type: "Withdrawal", description: "Withdrawal to UBA ••3901", amount: -750000, status: "Pending", timestamp: "4 min ago", reference: "W-1042", category: "Withdrawal", counterparty: "UBA ••3901" },
  { id: "TX-88011", type: "Allocation", description: "Provider allocation - P. Okafor", amount: -120000, status: "Completed", timestamp: "16 min ago", reference: "ALL-001", category: "Provider Allocation", counterparty: "P. Okafor" },
  { id: "TX-87996", type: "Credit", description: "Refund - Private order", amount: 42000, status: "Reconciling", timestamp: "31 min ago", reference: "PO-1092", category: "Refund", counterparty: "Private Ops" },
  { id: "TX-87988", type: "Debit", description: "Provider allocation - M. Bello", amount: -85000, status: "Completed", timestamp: "1 hour ago", reference: "ALL-002", category: "Provider Allocation", counterparty: "M. Bello" },
  { id: "TX-87972", type: "Credit", description: "Marketplace offer - MKT-24081", amount: 31200, status: "Completed", timestamp: "2 hours ago", reference: "MKT-24081", category: "Marketplace", counterparty: "Dispatch Service" },
  { id: "TX-87965", type: "Withdrawal", description: "Withdrawal to GTBank ••7421", amount: -250000, status: "Completed", timestamp: "3 hours ago", reference: "W-1038", category: "Withdrawal", counterparty: "GTBank ••7421" },
  { id: "TX-87958", type: "Allocation", description: "Provider allocation - A. Yusuf", amount: -65000, status: "Pending", timestamp: "4 hours ago", reference: "ALL-003", category: "Provider Allocation", counterparty: "A. Yusuf" },
  { id: "TX-87942", type: "Credit", description: "Private order - PO-1092", amount: 128000, status: "Completed", timestamp: "5 hours ago", reference: "PO-1092", category: "Private Order", counterparty: "Private Ops" },
  { id: "TX-87935", type: "Debit", description: "Provider allocation - K. Ahmed", amount: -45000, status: "Failed", timestamp: "6 hours ago", reference: "ALL-004", category: "Provider Allocation", counterparty: "K. Ahmed" },
];

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

const TYPE_ICONS: Record<string, { icon: React.ReactNode; color: string }> = {
  Credit: { icon: <ArrowDownRight className="h-3.5 w-3.5" />, color: "text-green" },
  Debit: { icon: <ArrowUpRight className="h-3.5 w-3.5" />, color: "text-red" },
  Allocation: { icon: <Users className="h-3.5 w-3.5" />, color: "text-blue" },
  Withdrawal: { icon: <Wallet className="h-3.5 w-3.5" />, color: "text-amber" },
};

const TYPE_FILTERS = ["All", "Credit", "Debit", "Allocation", "Withdrawal"];
const STATUS_FILTERS = ["All", "Completed", "Pending", "Reconciling", "Failed"];

export default function TransactionsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Transaction[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(TRANSACTIONS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.counterparty.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesType && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading transactions...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedType !== "All" || selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedStatus("All");
  };

  const goToTransactionDetail = (transactionId: string) => {
    router.push(`/wallet/transactions/${transactionId}`);
  };

  // Stats
  const totalCredit = data.filter((t) => t.type === "Credit").reduce((acc, t) => acc + t.amount, 0);
  const totalDebit = data.filter((t) => t.type === "Debit" || t.type === "Allocation" || t.type === "Withdrawal").reduce((acc, t) => acc + Math.abs(t.amount), 0);
  const pendingCount = data.filter((t) => t.status === "Pending").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Transactions
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Organization-scoped transaction history with direction, category, amount, status, and freshness.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Inflows</div>
          <div className="text-lg font-bold text-green">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalCredit)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Outflows</div>
          <div className="text-lg font-bold text-red">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalDebit)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Transaction Count</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{pendingCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by description, reference, or counterparty..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Credit", "Debit", "Allocation", "Withdrawal"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <ArrowLeftRight className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Completed", "Pending", "Reconciling", "Failed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
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

      {/* ===== TRANSACTIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <ArrowLeftRight className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No transactions found</h3>
          <p className="mt-1 text-sm text-ink-500">No transactions match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Transaction</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Timestamp</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Completed;
                  const typeStyle = TYPE_ICONS[item.type] || TYPE_ICONS.Credit;
                  const isCredit = item.amount > 0;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(Math.abs(item.amount));

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToTransactionDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950">{item.description}</div>
                        <div className="flex items-center gap-2 text-[11px] text-ink-500">
                          <span className="font-mono">{item.reference}</span>
                          <span className="text-ink-300">·</span>
                          <span>{item.category}</span>
                          <span className="text-ink-300">·</span>
                          <span>{item.counterparty}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.color}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`font-semibold ${isCredit ? "text-green" : "text-red"}`}>
                          {isCredit ? "+" : "-"} {formattedAmount}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-[12px]">{item.timestamp}</td>
                      <td className="px-4 py-3 text-right">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
                          <Eye className="h-4 w-4" />
                        </button>
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
          <ArrowLeftRight className="h-4 w-4" />
          WAL-02 · Company Wallet Transactions
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} transactions shown
        </span>
      </div>
    </div>
  );
}