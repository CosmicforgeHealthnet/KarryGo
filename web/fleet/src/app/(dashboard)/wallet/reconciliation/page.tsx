// src/app/(dashboard)/wallet/reconciliation/page.tsx

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
} from "lucide-react";

type ReconciliationCase = {
  id: string;
  reference: string;
  type: "Transaction" | "Allocation" | "Withdrawal" | "Balance";
  subject: string;
  authority: string;
  discrepancy: "Gap" | "Mismatch" | "Stale" | "Unknown";
  state: "Pending" | "Investigating" | "Resolved" | "Closed";
  attempts: number;
  lastAttempt: string | null;
  createdAt: string;
  updatedAt: string;
  description: string;
  resolvedBy: string | null;
  resolution: string | null;
};

// Mock data
const RECONCILIATION_CASES: ReconciliationCase[] = [
  {
    id: "REC-001",
    reference: "TX-87996",
    type: "Transaction",
    subject: "Refund - Private order",
    authority: "Payment Wallet",
    discrepancy: "Mismatch",
    state: "Investigating",
    attempts: 3,
    lastAttempt: "12 min ago",
    createdAt: "2026-08-21 09:30",
    updatedAt: "2026-08-21 09:42",
    description: "Transaction amount mismatch between local projection and Payment Wallet authority.",
    resolvedBy: null,
    resolution: null,
  },
  {
    id: "REC-002",
    type: "Allocation",
    reference: "ALL-004",
    subject: "Provider allocation - K. Ahmed",
    authority: "Payment Wallet",
    discrepancy: "Stale",
    state: "Pending",
    attempts: 1,
    lastAttempt: "2 hours ago",
    createdAt: "2026-08-21 07:00",
    updatedAt: "2026-08-21 07:00",
    description: "Allocation status has not been updated from Payment Wallet for over 2 hours.",
    resolvedBy: null,
    resolution: null,
  },
  {
    id: "REC-003",
    type: "Withdrawal",
    reference: "W-1037",
    subject: "Corporate withdrawal - UBA account",
    authority: "Payment Wallet",
    discrepancy: "Unknown",
    state: "Resolved",
    attempts: 2,
    lastAttempt: "1 day ago",
    createdAt: "2026-08-20 10:00",
    updatedAt: "2026-08-20 14:30",
    description: "Withdrawal outcome unknown. Read-back confirmed successful.",
    resolvedBy: "F. Daniel",
    resolution: "Confirmed successful via read-back.",
  },
  {
    id: "REC-004",
    type: "Balance",
    reference: "WAL-001",
    subject: "Company wallet balance",
    authority: "Payment Wallet",
    discrepancy: "Gap",
    state: "Investigating",
    attempts: 5,
    lastAttempt: "3 hours ago",
    createdAt: "2026-08-19 16:00",
    updatedAt: "2026-08-20 09:00",
    description: "Balance gap detected between local projection and Payment Wallet authority.",
    resolvedBy: null,
    resolution: null,
  },
  {
    id: "REC-005",
    type: "Transaction",
    reference: "TX-87988",
    subject: "Provider allocation - M. Bello",
    authority: "Payment Wallet",
    discrepancy: "Stale",
    state: "Closed",
    attempts: 1,
    lastAttempt: "2 days ago",
    createdAt: "2026-08-18 14:00",
    updatedAt: "2026-08-19 09:00",
    description: "Stale projection resolved after Payment Wallet sync.",
    resolvedBy: "System",
    resolution: "Automatically resolved after sync.",
  },
];

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Investigating: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Activity className="h-3.5 w-3.5" />,
  },
  Resolved: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Closed: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
};

const DISCREPANCY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Gap: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Mismatch: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Stale: { bg: "bg-blue-bg", text: "text-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Unknown: { bg: "bg-purple-bg", text: "text-purple", icon: <Info className="h-3.5 w-3.5" /> },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Transaction: { bg: "bg-blue-bg", text: "text-blue" },
  Allocation: { bg: "bg-green-bg", text: "text-green" },
  Withdrawal: { bg: "bg-amber-bg", text: "text-amber" },
  Balance: { bg: "bg-purple-bg", text: "text-purple" },
};

const STATE_FILTERS = ["All", "Pending", "Investigating", "Resolved", "Closed"];
const TYPE_FILTERS = ["All", "Transaction", "Allocation", "Withdrawal", "Balance"];
const DISCREPANCY_FILTERS = ["All", "Gap", "Mismatch", "Stale", "Unknown"];

export default function WalletReconciliationPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedState, setSelectedState] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedDiscrepancy, setSelectedDiscrepancy] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ReconciliationCase[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [reconciling, setReconciling] = useState<string | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(RECONCILIATION_CASES);
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
      item.subject.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase());

    const matchesState = selectedState === "All" || item.state === selectedState;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesDiscrepancy = selectedDiscrepancy === "All" || item.discrepancy === selectedDiscrepancy;

    return matchesSearch && matchesState && matchesType && matchesDiscrepancy;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading reconciliation cases...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedState !== "All" || selectedType !== "All" || selectedDiscrepancy !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedState("All");
    setSelectedType("All");
    setSelectedDiscrepancy("All");
  };

  const goToCaseDetail = (caseId: string) => {
    router.push(`/wallet/reconciliation/${caseId}`);
  };

  const handleReconcile = (caseId: string) => {
    setReconciling(caseId);
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
  const pendingCount = data.filter((item) => item.state === "Pending" || item.state === "Investigating").length;
  const resolvedCount = data.filter((item) => item.state === "Resolved" || item.state === "Closed").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Wallet Reconciliation
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Stale, unknown, and discrepant wallet operations with authoritative read-back state.
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Cases</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Open</div>
          <div className="text-lg font-bold text-amber">{pendingCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resolved</div>
          <div className="text-lg font-bold text-green">{resolvedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Avg Attempts</div>
          <div className="text-lg font-bold text-ink-950">
            {(data.reduce((acc, item) => acc + item.attempts, 0) / data.length).toFixed(1)}
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
            placeholder="Search by reference, subject, or description..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const states = ["All", "Pending", "Investigating", "Resolved", "Closed"];
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
            const types = ["All", "Transaction", "Allocation", "Withdrawal", "Balance"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Wallet className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const discrepancies = ["All", "Gap", "Mismatch", "Stale", "Unknown"];
            const currentIndex = discrepancies.indexOf(selectedDiscrepancy);
            const nextIndex = (currentIndex + 1) % discrepancies.length;
            setSelectedDiscrepancy(discrepancies[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedDiscrepancy}
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
              {(selectedState !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0) + (selectedDiscrepancy !== "All" ? 1 : 0)}
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
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Discrepancy</div>
              <div className="flex gap-1.5">
                {DISCREPANCY_FILTERS.map((disc) => (
                  <button
                    key={disc}
                    onClick={() => setSelectedDiscrepancy(disc)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedDiscrepancy === disc
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {disc}
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

      {/* ===== CASES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <RefreshCw className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No reconciliation cases found</h3>
          <p className="mt-1 text-sm text-ink-500">No cases match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Case</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Subject</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Discrepancy</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Attempts</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">State</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Updated</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const stateStyle = STATE_STYLES[item.state] || STATE_STYLES.Pending;
                  const discrepancyStyle = DISCREPANCY_STYLES[item.discrepancy] || DISCREPANCY_STYLES.Unknown;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Transaction;
                  const isOpen = item.state === "Pending" || item.state === "Investigating";

                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line hover:bg-surface-2 transition-colors group"
                    >
                      <td
                        className="px-4 py-3 cursor-pointer"
                        onClick={() => goToCaseDetail(item.id)}
                      >
                        <div>
                          <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                          <div className="flex items-center gap-2 text-[11px] text-ink-500">
                            <span className={`rounded-full px-2 py-0.5 text-[9px] font-medium ${typeStyle.bg} ${typeStyle.text}`}>
                              {item.type}
                            </span>
                            <span className="text-ink-300">·</span>
                            <span>{item.authority}</span>
                          </div>
                        </div>
                      </td>
                      <td
                        className="px-4 py-3 text-ink-600 cursor-pointer"
                        onClick={() => goToCaseDetail(item.id)}
                      >
                        {item.subject}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${discrepancyStyle.bg} ${discrepancyStyle.text}`}>
                          {discrepancyStyle.icon}
                          {item.discrepancy}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.attempts}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
                          {stateStyle.icon}
                          {item.state}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-[12px]">{item.updatedAt}</td>
                      <td className="px-4 py-3 text-right flex items-center gap-1">
                        <button
                          onClick={() => goToCaseDetail(item.id)}
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
          WAL-12 · Wallet Reconciliation Cases
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} cases shown
        </span>
      </div>
    </div>
  );
}