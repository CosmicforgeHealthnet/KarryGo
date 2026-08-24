// src/app/(dashboard)/finance/payroll/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
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
  Briefcase,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
  Plus,
  DollarSign,
  Building2,
  Tag,
  Calendar,
  Edit3,
  Trash2,
  Save,
  Send,
  Percent,
  BarChart3,
  Landmark,
  User,
  MapPin,
  Calculator,
  CreditCard,
  Receipt,
} from "lucide-react";

type PayrollPeriod = {
  id: string;
  reference: string;
  period: string;
  startDate: string;
  endDate: string;
  totalAmount: number;
  currency: string;
  providerCount: number;
  status: "Draft" | "Calculated" | "Approved" | "Posted" | "Reconciling" | "Failed";
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  postedAt: string | null;
  branch: string;
  notes: string | null;
  items: {
    id: string;
    providerId: string;
    providerName: string;
    amount: number;
    status: "Pending" | "Approved" | "Posted" | "Reconciling" | "Failed";
  }[];
};

// Mock data
const PAYROLL_PERIODS: PayrollPeriod[] = [
  {
    id: "PP-001",
    reference: "PP-2026-008",
    period: "August 2026",
    startDate: "2026-08-01",
    endDate: "2026-08-31",
    totalAmount: 2800000,
    currency: "NGN",
    providerCount: 12,
    status: "Calculated",
    createdAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: null,
    approvedAt: null,
    postedAt: null,
    branch: "All branches",
    notes: "August 2026 payroll. Includes all active providers.",
    items: [
      { id: "PI-001", providerId: "P-044", providerName: "Peter Okafor", amount: 284000, status: "Pending" },
      { id: "PI-002", providerId: "P-038", providerName: "Musa Bello", amount: 156000, status: "Pending" },
      { id: "PI-003", providerId: "P-052", providerName: "Amina Yusuf", amount: 42000, status: "Pending" },
    ],
  },
  {
    id: "PP-002",
    reference: "PP-2026-007",
    period: "July 2026",
    startDate: "2026-07-01",
    endDate: "2026-07-31",
    totalAmount: 2650000,
    currency: "NGN",
    providerCount: 11,
    status: "Approved",
    createdAt: "2026-07-20",
    updatedAt: "2026-07-25",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-25",
    postedAt: null,
    branch: "All branches",
    notes: "July 2026 payroll. Approved by finance.",
    items: [
      { id: "PI-004", providerId: "P-044", providerName: "Peter Okafor", amount: 272000, status: "Approved" },
      { id: "PI-005", providerId: "P-038", providerName: "Musa Bello", amount: 148000, status: "Approved" },
      { id: "PI-006", providerId: "P-052", providerName: "Amina Yusuf", amount: 38000, status: "Approved" },
    ],
  },
  {
    id: "PP-003",
    reference: "PP-2026-006",
    period: "June 2026",
    startDate: "2026-06-01",
    endDate: "2026-06-30",
    totalAmount: 2400000,
    currency: "NGN",
    providerCount: 10,
    status: "Posted",
    createdAt: "2026-06-20",
    updatedAt: "2026-06-28",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-06-25",
    postedAt: "2026-06-28",
    branch: "All branches",
    notes: "June 2026 payroll. Posted to provider sub-wallets.",
    items: [
      { id: "PI-007", providerId: "P-044", providerName: "Peter Okafor", amount: 260000, status: "Posted" },
      { id: "PI-008", providerId: "P-038", providerName: "Musa Bello", amount: 142000, status: "Posted" },
      { id: "PI-009", providerId: "P-052", providerName: "Amina Yusuf", amount: 35000, status: "Posted" },
    ],
  },
  {
    id: "PP-004",
    reference: "PP-2026-005",
    period: "May 2026",
    startDate: "2026-05-01",
    endDate: "2026-05-31",
    totalAmount: 2100000,
    currency: "NGN",
    providerCount: 9,
    status: "Reconciling",
    createdAt: "2026-05-20",
    updatedAt: "2026-05-28",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-05-25",
    postedAt: "2026-05-28",
    branch: "All branches",
    notes: "May 2026 payroll. Reconciliation in progress.",
    items: [
      { id: "PI-010", providerId: "P-044", providerName: "Peter Okafor", amount: 248000, status: "Reconciling" },
      { id: "PI-011", providerId: "P-038", providerName: "Musa Bello", amount: 135000, status: "Reconciling" },
      { id: "PI-012", providerId: "P-052", providerName: "Amina Yusuf", amount: 32000, status: "Reconciling" },
    ],
  },
  {
    id: "PP-005",
    reference: "PP-2026-004",
    period: "April 2026",
    startDate: "2026-04-01",
    endDate: "2026-04-30",
    totalAmount: 1800000,
    currency: "NGN",
    providerCount: 8,
    status: "Failed",
    createdAt: "2026-04-20",
    updatedAt: "2026-04-28",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-04-25",
    postedAt: null,
    branch: "All branches",
    notes: "April 2026 payroll. Posting failed due to wallet issues.",
    items: [
      { id: "PI-013", providerId: "P-044", providerName: "Peter Okafor", amount: 236000, status: "Failed" },
      { id: "PI-014", providerId: "P-038", providerName: "Musa Bello", amount: 128000, status: "Failed" },
      { id: "PI-015", providerId: "P-052", providerName: "Amina Yusuf", amount: 30000, status: "Failed" },
    ],
  },
  {
    id: "PP-006",
    reference: "PP-2026-009",
    period: "September 2026",
    startDate: "2026-09-01",
    endDate: "2026-09-30",
    totalAmount: 0,
    currency: "NGN",
    providerCount: 0,
    status: "Draft",
    createdAt: "2026-08-21",
    updatedAt: "2026-08-21",
    createdBy: "F. Daniel",
    approvedBy: null,
    approvedAt: null,
    postedAt: null,
    branch: "All branches",
    notes: "September 2026 payroll. Draft.",
    items: [],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Calculated: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Calculator className="h-3.5 w-3.5" />,
  },
  Approved: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Posted: {
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

const STATUS_FILTERS = ["All", "Draft", "Calculated", "Approved", "Posted", "Reconciling", "Failed"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function PayrollPeriodsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<PayrollPeriod[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PAYROLL_PERIODS);
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
      item.period.toLowerCase().includes(search.toLowerCase()) ||
      item.branch.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading payroll periods...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const goToPayrollPeriod = (periodId: string) => {
    router.push(`/finance/payroll/${periodId}`);
  };

  const goToCreatePayroll = () => {
    router.push("/finance/payroll/create");
  };

  // Stats
  const totalPeriods = data.length;
  const calculatedCount = data.filter((item) => item.status === "Calculated").length;
  const approvedCount = data.filter((item) => item.status === "Approved").length;
  const postedCount = data.filter((item) => item.status === "Posted").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Payroll Periods
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Period, population, calculation status, approval state, total approved items and posting progress.
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
            onClick={goToCreatePayroll}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Period
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Periods</div>
          <div className="text-lg font-bold text-ink-950">{totalPeriods}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Calculated</div>
          <div className="text-lg font-bold text-amber">{calculatedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Approved</div>
          <div className="text-lg font-bold text-green">{approvedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Posted</div>
          <div className="text-lg font-bold text-green">{postedCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by period or reference..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Calculated", "Approved", "Posted", "Reconciling", "Failed"];
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
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Building2 className="h-4 w-4 text-ink-400" />
          {selectedBranch}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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

      {/* ===== PAYROLL PERIODS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No payroll periods found</h3>
          <p className="mt-1 text-sm text-ink-500">No payroll periods match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Period</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Providers</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Updated</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.totalAmount);
                  const isDraft = item.status === "Draft";

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToPayrollPeriod(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950">{item.period}</div>
                        <div className="text-[11px] text-ink-500">{item.startDate} - {item.endDate}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="font-mono text-[12px] text-ink-600">{item.reference}</span>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.providerCount}</td>
                      <td className="px-4 py-3 font-semibold text-ink-950">{formattedAmount}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-[12px]">{item.updatedAt}</td>
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
          <Users className="h-4 w-4" />
          FIN-10 · Payroll Periods
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} periods shown
        </span>
      </div>
    </div>
  );
}