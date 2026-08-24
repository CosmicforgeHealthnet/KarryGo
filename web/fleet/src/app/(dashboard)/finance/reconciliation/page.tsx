// src/app/(dashboard)/finance/reconciliation/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Search,
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
  Wallet,
  Users,
  MoreHorizontal,
  CalendarDays,
  FileText,
  AlertTriangle,
  Info,
  TrendingUp,
  TrendingDown,
  Activity,
  Building2,
  Tag,
  Landmark,
  Receipt,
  CreditCard,
  HandCoins,
} from "lucide-react";

type ReconciliationItem = {
  id: string;
  reference: string;
  type: "Invoice" | "Payment" | "Expense" | "Allocation" | "Withdrawal" | "Payroll" | "Settlement";
  source: string;
  target: string;
  amount: number;
  currency: string;
  state: "Pending" | "Matching" | "Reconciled" | "Failed" | "Reconciling";
  createdAt: string;
  updatedAt: string;
  attempts: number;
  lastAttempt: string | null;
  description: string;
  resolvedBy: string | null;
  resolution: string | null;
  sourceRef: string;
  targetRef: string;
};

// Mock data
const RECONCILIATION_ITEMS: ReconciliationItem[] = [
  {
    id: "REC-001",
    reference: "INV-002",
    type: "Invoice",
    source: "KarryGo Platform",
    target: "Payment Gateway",
    amount: 840000,
    currency: "NGN",
    state: "Pending",
    createdAt: "2026-08-21 09:00",
    updatedAt: "2026-08-21 09:00",
    attempts: 0,
    lastAttempt: null,
    description: "Invoice payment pending confirmation from Payment Gateway.",
    resolvedBy: null,
    resolution: null,
    sourceRef: "INV-2026-002",
    targetRef: "PG-2026-015",
  },
  {
    id: "REC-002",
    reference: "TX-87996",
    type: "Payment",
    source: "Payment Gateway",
    target: "Company Wallet",
    amount: 42000,
    currency: "NGN",
    state: "Reconciling",
    createdAt: "2026-08-20 14:30",
    updatedAt: "2026-08-21 08:00",
    attempts: 3,
    lastAttempt: "2 hours ago",
    description: "Refund payment - Private order. Amount mismatch detected.",
    resolvedBy: null,
    resolution: null,
    sourceRef: "PG-2026-014",
    targetRef: "WAL-001",
  },
  {
    id: "REC-003",
    reference: "W-1037",
    type: "Withdrawal",
    source: "Company Wallet",
    target: "Payment Gateway",
    amount: 2000000,
    currency: "NGN",
    state: "Reconciled",
    createdAt: "2026-08-19 10:00",
    updatedAt: "2026-08-20 14:30",
    attempts: 2,
    lastAttempt: "1 day ago",
    description: "Corporate withdrawal reconciled successfully.",
    resolvedBy: "F. Daniel",
    resolution: "Confirmed successful via read-back.",
    sourceRef: "WAL-001",
    targetRef: "PG-2026-013",
  },
  {
    id: "REC-004",
    reference: "ALL-004",
    type: "Allocation",
    source: "Company Wallet",
    target: "Provider Sub-wallet",
    amount: 12000,
    currency: "NGN",
    state: "Matching",
    createdAt: "2026-08-18 16:00",
    updatedAt: "2026-08-20 09:00",
    attempts: 5,
    lastAttempt: "3 hours ago",
    description: "Provider allocation discrepancy. Amount mismatch.",
    resolvedBy: null,
    resolution: null,
    sourceRef: "WAL-001",
    targetRef: "PW-005",
  },
  {
    id: "REC-005",
    reference: "EXP-002",
    type: "Expense",
    source: "Finance Service",
    target: "Company Wallet",
    amount: 450000,
    currency: "NGN",
    state: "Failed",
    createdAt: "2026-08-17 11:00",
    updatedAt: "2026-08-18 08:00",
    attempts: 2,
    lastAttempt: "2 days ago",
    description: "Expense posting failed. Wallet service unavailable.",
    resolvedBy: "System",
    resolution: "Retry scheduled.",
    sourceRef: "EXP-2026-002",
    targetRef: "WAL-001",
  },
  {
    id: "REC-006",
    reference: "PP-006",
    type: "Payroll",
    source: "Payroll Service",
    target: "Provider Sub-wallets",
    amount: 2800000,
    currency: "NGN",
    state: "Pending",
    createdAt: "2026-08-20 16:00",
    updatedAt: "2026-08-20 16:00",
    attempts: 0,
    lastAttempt: null,
    description: "Payroll posting pending approval.",
    resolvedBy: null,
    resolution: null,
    sourceRef: "PP-2026-008",
    targetRef: "PW-001,PW-002",
  },
  {
    id: "REC-007",
    reference: "SET-004",
    type: "Settlement",
    source: "Finance Service",
    target: "Subcontractor Wallet",
    amount: 620000,
    currency: "NGN",
    state: "Reconciling",
    createdAt: "2026-08-16 09:00",
    updatedAt: "2026-08-18 10:00",
    attempts: 4,
    lastAttempt: "1 day ago",
    description: "Subcontractor settlement reconciliation in progress.",
    resolvedBy: null,
    resolution: null,
    sourceRef: "SET-2026-004",
    targetRef: "SW-001",
  },
  {
    id: "REC-008",
    reference: "INV-003",
    type: "Invoice",
    source: "KarryGo Platform",
    target: "Payment Gateway",
    amount: 2100000,
    currency: "NGN",
    state: "Reconciled",
    createdAt: "2026-07-31 08:00",
    updatedAt: "2026-08-02 10:00",
    attempts: 1,
    lastAttempt: "3 days ago",
    description: "Invoice reconciled with payment gateway.",
    resolvedBy: "F. Daniel",
    resolution: "Payment confirmed.",
    sourceRef: "INV-2026-003",
    targetRef: "PG-2026-012",
  },
];

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Matching: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Activity className="h-3.5 w-3.5" />,
  },
  Reconciled: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Reconciling: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Invoice: { bg: "bg-blue-bg", text: "text-blue" },
  Payment: { bg: "bg-green-bg", text: "text-green" },
  Expense: { bg: "bg-amber-bg", text: "text-amber" },
  Allocation: { bg: "bg-purple-bg", text: "text-purple" },
  Withdrawal: { bg: "bg-red-bg", text: "text-red" },
  Payroll: { bg: "bg-pink-bg", text: "text-pink" },
  Settlement: { bg: "bg-teal-bg", text: "text-teal" },
};

const STATE_FILTERS = ["All", "Pending", "Matching", "Reconciled", "Failed", "Reconciling"];
const TYPE_FILTERS = ["All", "Invoice", "Payment", "Expense", "Allocation", "Withdrawal", "Payroll", "Settlement"];

export default function FinancialReconciliationPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedState, setSelectedState] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ReconciliationItem[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [reconciling, setReconciling] = useState<string | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(RECONCILIATION_ITEMS);
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
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.source.toLowerCase().includes(search.toLowerCase()) ||
      item.target.toLowerCase().includes(search.toLowerCase());

    const matchesState = selectedState === "All" || item.state === selectedState;
    const matchesType = selectedType === "All" || item.type === selectedType;

    return matchesSearch && matchesState && matchesType;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading reconciliation data...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedState !== "All" || selectedType !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedState("All");
    setSelectedType("All");
  };

  const goToReconciliationDetail = (id: string) => {
    router.push(`/finance/reconciliation/${id}`);
  };

  const handleReconcile = (id: string) => {
    setReconciling(id);
    setTimeout(() => {
      setReconciling(null);
      toast("Reconciliation requested");
    }, 1500);
  };

  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  // Stats
  const totalItems = data.length;
  const pendingCount = data.filter((item) => item.state === "Pending" || item.state === "Matching").length;
  const reconciledCount = data.filter((item) => item.state === "Reconciled").length;
  const failedCount = data.filter((item) => item.state === "Failed").length;
  const totalAmount = data.reduce((acc, item) => acc + item.amount, 0);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Financial Reconciliation
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            External posting operations, projection freshness, unknown outcomes, failed/retryable states and reconciliation cases.
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
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Items</div>
          <div className="text-lg font-bold text-ink-950">{totalItems}</div>
          <div className="text-[10px] text-ink-400">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Open</div>
          <div className="text-lg font-bold text-amber">{pendingCount}</div>
          <div className="text-[10px] text-ink-400">Pending or matching</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reconciled</div>
          <div className="text-lg font-bold text-green">{reconciledCount}</div>
          <div className="text-[10px] text-ink-400">Successfully resolved</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Failed</div>
          <div className="text-lg font-bold text-red">{failedCount}</div>
          <div className="text-[10px] text-ink-400">Requires attention</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, description, or source/target..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const states = ["All", "Pending", "Matching", "Reconciled", "Failed", "Reconciling"];
            const currentIndex = states.indexOf(selectedState);
            const nextIndex = (currentIndex + 1) % states.length;
            setSelectedState(states[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Activity className="h-4 w-4 text-ink-400" />
          {selectedState}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const types = ["All", "Invoice", "Payment", "Expense", "Allocation", "Withdrawal", "Payroll", "Settlement"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Tag className="h-4 w-4 text-ink-400" />
          {selectedType}
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
              {(selectedState !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0)}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">State</div>
              <div className="flex gap-1.5">
                {STATE_FILTERS.map((state) => (
                  <button
                    key={state}
                    onClick={() => setSelectedState(state)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedState === state
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {state}
                  </button>
                ))}
              </div>
            </div>
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
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== RECONCILIATION TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <RefreshCw className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No reconciliation items found</h3>
          <p className="mt-1 text-sm text-ink-500">No items match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1000px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Source → Target</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">State</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Attempts</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const stateStyle = STATE_STYLES[item.state] || STATE_STYLES.Pending;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Invoice;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.amount);
                  const isOpen = item.state === "Pending" || item.state === "Matching" || item.state === "Reconciling";
                  const isFailed = item.state === "Failed";

                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line hover:bg-surface-2 transition-colors group"
                    >
                      <td
                        className="px-4 py-3 cursor-pointer"
                        onClick={() => goToReconciliationDetail(item.id)}
                      >
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.sourceRef}</div>
                      </td>
                      <td
                        className="px-4 py-3 cursor-pointer"
                        onClick={() => goToReconciliationDetail(item.id)}
                      >
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {item.type}
                        </span>
                      </td>
                      <td
                        className="px-4 py-3 text-ink-600 cursor-pointer"
                        onClick={() => goToReconciliationDetail(item.id)}
                      >
                        <span className="text-xs">{item.source}</span>
                        <span className="mx-1 text-ink-300">→</span>
                        <span className="text-xs">{item.target}</span>
                      </td>
                      <td
                        className="px-4 py-3 font-semibold text-ink-950 cursor-pointer"
                        onClick={() => goToReconciliationDetail(item.id)}
                      >
                        {formattedAmount}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
                          {stateStyle.icon}
                          {item.state}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600">
                        {item.attempts}
                        {item.lastAttempt && (
                          <div className="text-[10px] text-ink-400">{item.lastAttempt}</div>
                        )}
                      </td>
                      <td className="px-4 py-3 text-right flex items-center gap-1 justify-end">
                        <button
                          onClick={() => goToReconciliationDetail(item.id)}
                          className="text-ink-400 group-hover:text-primary transition-colors"
                        >
                          <Eye className="h-4 w-4" />
                        </button>
                        {isOpen && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleReconcile(item.id);
                            }}
                            disabled={reconciling === item.id}
                            className={`rounded-lg px-2.5 py-1 text-[10px] font-medium transition ${
                              reconciling === item.id
                                ? "bg-ink-300 text-white"
                                : "bg-primary text-white hover:bg-primary/90"
                            }`}
                          >
                            {reconciling === item.id ? (
                              <RefreshCw className="h-3 w-3 animate-spin" />
                            ) : (
                              "Reconcile"
                            )}
                          </button>
                        )}
                        {isFailed && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              toast("Retrying failed reconciliation...");
                            }}
                            className="rounded-lg bg-amber px-2.5 py-1 text-[10px] font-medium text-white transition hover:bg-amber/90"
                          >
                            Retry
                          </button>
                        )}
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
          <RefreshCw className="h-4 w-4" />
          FIN-16 · Financial Reconciliation
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} items shown
        </span>
      </div>
    </div>
  );
}