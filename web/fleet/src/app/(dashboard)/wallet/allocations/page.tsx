// src/app/(dashboard)/wallet/allocations/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  CreditCard,
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
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
  Plus,
} from "lucide-react";

type Allocation = {
  id: string;
  reference: string;
  providerId: string;
  providerName: string;
  type: "Trip Settlement" | "Compensation" | "Adjustment" | "Reimbursement" | "Bonus";
  amount: number;
  currency: string;
  status: "Pending" | "Approved" | "Executed" | "Reconciling" | "Failed";
  description: string;
  createdAt: string;
  approvedBy: string | null;
  executedAt: string | null;
  businessReference: string;
};

// Mock data
const ALLOCATIONS: Allocation[] = [
  {
    id: "ALL-001",
    reference: "ALL-001",
    providerId: "P-044",
    providerName: "Peter Okafor",
    type: "Trip Settlement",
    amount: 120000,
    currency: "NGN",
    status: "Executed",
    description: "Trip settlement for KG-8491",
    createdAt: "2026-08-21 09:30",
    approvedBy: "F. Daniel",
    executedAt: "2026-08-21 09:35",
    businessReference: "KG-8491",
  },
  {
    id: "ALL-002",
    reference: "ALL-002",
    providerId: "P-038",
    providerName: "Musa Bello",
    type: "Trip Settlement",
    amount: 85000,
    currency: "NGN",
    status: "Executed",
    description: "Trip settlement for KG-8487",
    createdAt: "2026-08-21 08:45",
    approvedBy: "F. Daniel",
    executedAt: "2026-08-21 08:50",
    businessReference: "KG-8487",
  },
  {
    id: "ALL-003",
    reference: "ALL-003",
    providerId: "P-052",
    providerName: "Amina Yusuf",
    type: "Compensation",
    amount: 65000,
    currency: "NGN",
    status: "Pending",
    description: "Monthly compensation - August 2026",
    createdAt: "2026-08-20 16:30",
    approvedBy: null,
    executedAt: null,
    businessReference: "COMP-008",
  },
  {
    id: "ALL-004",
    reference: "ALL-004",
    providerId: "P-029",
    providerName: "Kabiru Ahmed",
    type: "Adjustment",
    amount: -12000,
    currency: "NGN",
    status: "Reconciling",
    description: "Fuel adjustment for TR-112",
    createdAt: "2026-08-20 14:00",
    approvedBy: "O. Adebayo",
    executedAt: "2026-08-20 14:15",
    businessReference: "ADJ-002",
  },
  {
    id: "ALL-005",
    reference: "ALL-005",
    providerId: "P-061",
    providerName: "Chinedu Uche",
    type: "Trip Settlement",
    amount: 45000,
    currency: "NGN",
    status: "Approved",
    description: "Trip settlement for KG-8479",
    createdAt: "2026-08-20 11:00",
    approvedBy: "F. Daniel",
    executedAt: null,
    businessReference: "KG-8479",
  },
  {
    id: "ALL-006",
    reference: "ALL-006",
    providerId: "P-073",
    providerName: "Esther Obi",
    type: "Reimbursement",
    amount: 28000,
    currency: "NGN",
    status: "Pending",
    description: "Vehicle maintenance reimbursement",
    createdAt: "2026-08-19 15:30",
    approvedBy: null,
    executedAt: null,
    businessReference: "RM-007",
  },
  {
    id: "ALL-007",
    reference: "ALL-007",
    providerId: "P-044",
    providerName: "Peter Okafor",
    type: "Bonus",
    amount: 50000,
    currency: "NGN",
    status: "Failed",
    description: "Performance bonus - Q3 2026",
    createdAt: "2026-08-19 10:00",
    approvedBy: "F. Daniel",
    executedAt: "2026-08-19 10:05",
    businessReference: "BONUS-003",
  },
  {
    id: "ALL-008",
    reference: "ALL-008",
    providerId: "P-038",
    providerName: "Musa Bello",
    type: "Trip Settlement",
    amount: 32000,
    currency: "NGN",
    status: "Executed",
    description: "Trip settlement for KG-8466",
    createdAt: "2026-08-18 16:00",
    approvedBy: "F. Daniel",
    executedAt: "2026-08-18 16:05",
    businessReference: "KG-8466",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Approved: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Executed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Reconciling: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  "Trip Settlement": { bg: "bg-green-bg", text: "text-green" },
  Compensation: { bg: "bg-blue-bg", text: "text-blue" },
  Adjustment: { bg: "bg-amber-bg", text: "text-amber" },
  Reimbursement: { bg: "bg-purple-bg", text: "text-purple" },
  Bonus: { bg: "bg-pink-bg", text: "text-pink" },
};

const TYPE_FILTERS = ["All", "Trip Settlement", "Compensation", "Adjustment", "Reimbursement", "Bonus"];
const STATUS_FILTERS = ["All", "Pending", "Approved", "Executed", "Reconciling", "Failed"];

export default function AllocationsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Allocation[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(ALLOCATIONS);
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
      item.providerName.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.providerId.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesType && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading allocations...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedType !== "All" || selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedStatus("All");
  };

  const goToAllocationDetail = (allocationId: string) => {
    router.push(`/wallet/allocations/${allocationId}`);
  };

  const goToCreateAllocation = () => {
    router.push("/wallet/allocations/create");
  };

  // Stats
  const totalAmount = data.reduce((acc, item) => acc + item.amount, 0);
  const pendingCount = data.filter((item) => item.status === "Pending").length;
  const executedCount = data.filter((item) => item.status === "Executed").length;
  const failedCount = data.filter((item) => item.status === "Failed").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Allocations
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Allocation instructions with type, provider, amount, source reference, approval/execution state and freshness.
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
            onClick={goToCreateAllocation}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Allocation
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Allocations</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Amount</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{pendingCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Executed</div>
          <div className="text-lg font-bold text-green">{executedCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by provider, reference, or description..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Trip Settlement", "Compensation", "Adjustment", "Reimbursement", "Bonus"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CreditCard className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Approved", "Executed", "Reconciling", "Failed"];
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

      {/* ===== ALLOCATIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <CreditCard className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No allocations found</h3>
          <p className="mt-1 text-sm text-ink-500">No allocations match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Provider</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Created</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES["Trip Settlement"];
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(Math.abs(item.amount));
                  const isNegative = item.amount < 0;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToAllocationDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.businessReference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">{item.providerName}</div>
                        <div className="text-[11px] text-ink-500 font-mono">{item.providerId}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`font-semibold ${isNegative ? "text-red" : "text-ink-950"}`}>
                          {isNegative ? "-" : ""}{formattedAmount}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-[12px]">{item.createdAt}</td>
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
          <CreditCard className="h-4 w-4" />
          WAL-06 · Allocations
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} allocations shown
        </span>
      </div>
    </div>
  );
}