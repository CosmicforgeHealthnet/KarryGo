// src/app/(dashboard)/finance/payroll/statements/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FileText,
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
  Users,
  MoreHorizontal,
  CalendarDays,
  Download,
  Printer,
  User,
  DollarSign,
  Building2,
  Tag,
  Calendar,
  FileCheck,
  Wallet,
  CreditCard,
  Receipt,
} from "lucide-react";

type CompensationStatement = {
  id: string;
  providerId: string;
  providerName: string;
  providerInitials: string;
  role: string;
  branch: string;
  period: string;
  totalAmount: number;
  currency: string;
  items: {
    id: string;
    type: "Trip Settlement" | "Compensation" | "Bonus" | "Reimbursement" | "Adjustment";
    description: string;
    amount: number;
    date: string;
    reference: string;
  }[];
  status: "Generated" | "Viewed" | "Downloaded";
  generatedAt: string;
  viewedAt: string | null;
  downloadedAt: string | null;
};

// Mock data
const STATEMENTS: CompensationStatement[] = [
  {
    id: "CS-001",
    providerId: "P-044",
    providerName: "Peter Okafor",
    providerInitials: "PO",
    role: "Dispatch Rider",
    branch: "Ikeja",
    period: "August 2026",
    totalAmount: 284000,
    currency: "NGN",
    items: [
      { id: "CI-001", type: "Trip Settlement", description: "Trip KG-8491", amount: 120000, date: "2026-08-21", reference: "KG-8491" },
      { id: "CI-002", type: "Trip Settlement", description: "Trip KG-8487", amount: 85000, date: "2026-08-20", reference: "KG-8487" },
      { id: "CI-003", type: "Bonus", description: "Monthly performance bonus", amount: 44000, date: "2026-08-20", reference: "BONUS-003" },
      { id: "CI-004", type: "Compensation", description: "Base compensation - August", amount: 35000, date: "2026-08-20", reference: "COMP-008" },
    ],
    status: "Generated",
    generatedAt: "2026-08-21 09:30",
    viewedAt: null,
    downloadedAt: null,
  },
  {
    id: "CS-002",
    providerId: "P-038",
    providerName: "Musa Bello",
    providerInitials: "MB",
    role: "Hauling Driver",
    branch: "Apapa",
    period: "August 2026",
    totalAmount: 156000,
    currency: "NGN",
    items: [
      { id: "CI-005", type: "Trip Settlement", description: "Trip KG-8479", amount: 45000, date: "2026-08-20", reference: "KG-8479" },
      { id: "CI-006", type: "Compensation", description: "Base compensation - August", amount: 95000, date: "2026-08-20", reference: "COMP-008" },
      { id: "CI-007", type: "Bonus", description: "Monthly performance bonus", amount: 16000, date: "2026-08-20", reference: "BONUS-003" },
    ],
    status: "Viewed",
    generatedAt: "2026-08-21 09:30",
    viewedAt: "2026-08-21 10:00",
    downloadedAt: null,
  },
  {
    id: "CS-003",
    providerId: "P-052",
    providerName: "Amina Yusuf",
    providerInitials: "AY",
    role: "Dispatch Supervisor",
    branch: "Yaba",
    period: "August 2026",
    totalAmount: 42000,
    currency: "NGN",
    items: [
      { id: "CI-008", type: "Trip Settlement", description: "Trip KG-8452", amount: 28000, date: "2026-08-19", reference: "KG-8452" },
      { id: "CI-009", type: "Bonus", description: "Monthly performance bonus", amount: 7000, date: "2026-08-20", reference: "BONUS-003" },
      { id: "CI-010", type: "Compensation", description: "Base compensation - August", amount: 7000, date: "2026-08-20", reference: "COMP-008" },
    ],
    status: "Downloaded",
    generatedAt: "2026-08-21 09:30",
    viewedAt: "2026-08-21 09:45",
    downloadedAt: "2026-08-21 10:30",
  },
  {
    id: "CS-004",
    providerId: "P-061",
    providerName: "Chinedu Uche",
    providerInitials: "CU",
    role: "Dispatch Rider",
    branch: "Lekki",
    period: "August 2026",
    totalAmount: 238000,
    currency: "NGN",
    items: [
      { id: "CI-011", type: "Trip Settlement", description: "Trip KG-8441", amount: 32000, date: "2026-08-18", reference: "KG-8441" },
      { id: "CI-012", type: "Trip Settlement", description: "Trip KG-8417", amount: 28000, date: "2026-08-17", reference: "KG-8417" },
      { id: "CI-013", type: "Compensation", description: "Base compensation - August", amount: 140000, date: "2026-08-20", reference: "COMP-008" },
      { id: "CI-014", type: "Bonus", description: "Monthly performance bonus", amount: 38000, date: "2026-08-20", reference: "BONUS-003" },
    ],
    status: "Generated",
    generatedAt: "2026-08-21 09:30",
    viewedAt: null,
    downloadedAt: null,
  },
  {
    id: "CS-005",
    providerId: "P-029",
    providerName: "Kabiru Ahmed",
    providerInitials: "KA",
    role: "Hauling Driver",
    branch: "Lekki",
    period: "August 2026",
    totalAmount: 89000,
    currency: "NGN",
    items: [
      { id: "CI-015", type: "Trip Settlement", description: "Trip KG-8417", amount: 80000, date: "2026-08-17", reference: "KG-8417" },
      { id: "CI-016", type: "Bonus", description: "Monthly performance bonus", amount: 9000, date: "2026-08-20", reference: "BONUS-003" },
    ],
    status: "Viewed",
    generatedAt: "2026-08-21 09:30",
    viewedAt: "2026-08-21 10:15",
    downloadedAt: null,
  },
  {
    id: "CS-006",
    providerId: "P-073",
    providerName: "Esther Obi",
    providerInitials: "EO",
    role: "Fleet Manager",
    branch: "Ikeja",
    period: "August 2026",
    totalAmount: 210000,
    currency: "NGN",
    items: [
      { id: "CI-017", type: "Compensation", description: "Base compensation - August", amount: 200000, date: "2026-08-20", reference: "COMP-008" },
      { id: "CI-018", type: "Bonus", description: "Monthly performance bonus", amount: 10000, date: "2026-08-20", reference: "BONUS-003" },
    ],
    status: "Downloaded",
    generatedAt: "2026-08-21 09:30",
    viewedAt: "2026-08-21 09:50",
    downloadedAt: "2026-08-21 11:00",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Generated: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Viewed: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Eye className="h-3.5 w-3.5" />,
  },
  Downloaded: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <Download className="h-3.5 w-3.5" />,
  },
};

const ITEM_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  "Trip Settlement": { bg: "bg-green-bg", text: "text-green" },
  Compensation: { bg: "bg-blue-bg", text: "text-blue" },
  Bonus: { bg: "bg-amber-bg", text: "text-amber" },
  Reimbursement: { bg: "bg-purple-bg", text: "text-purple" },
  Adjustment: { bg: "bg-red-bg", text: "text-red" },
};

const STATUS_FILTERS = ["All", "Generated", "Viewed", "Downloaded"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const PERIODS = ["August 2026", "July 2026", "June 2026"];

export default function ProviderStatementsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedPeriod, setSelectedPeriod] = useState("August 2026");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CompensationStatement[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(STATEMENTS);
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
      item.providerId.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesPeriod = item.period === selectedPeriod;

    return matchesSearch && matchesStatus && matchesBranch && matchesPeriod;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading statements...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const goToStatementDetail = (statementId: string) => {
    router.push(`/finance/payroll/statements/${statementId}`);
  };

  // Stats
  const totalStatements = data.length;
  const totalAmount = data.reduce((acc, item) => acc + item.totalAmount, 0);
  const generatedCount = data.filter((item) => item.status === "Generated").length;
  const downloadedCount = data.filter((item) => item.status === "Downloaded").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Provider Compensation Statements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Provider compensation statements with approved items and company compensation details.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <FileText className="h-3.5 w-3.5" />
            Export All
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Statements</div>
          <div className="text-lg font-bold text-ink-950">{totalStatements}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Amount</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Generated</div>
          <div className="text-lg font-bold text-blue">{generatedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Downloaded</div>
          <div className="text-lg font-bold text-green">{downloadedCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by provider name or ID..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Generated", "Viewed", "Downloaded"];
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
          onClick={() => {
            const periods = ["August 2026", "July 2026", "June 2026"];
            const currentIndex = periods.indexOf(selectedPeriod);
            const nextIndex = (currentIndex + 1) % periods.length;
            setSelectedPeriod(periods[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CalendarDays className="h-4 w-4 text-ink-400" />
          {selectedPeriod}
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

      {/* ===== STATEMENTS CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileText className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No statements found</h3>
          <p className="mt-1 text-sm text-ink-500">No statements match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Generated;
            const formattedTotal = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(item.totalAmount);

            return (
              <div
                key={item.id}
                onClick={() => goToStatementDetail(item.id)}
                className="group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 border-line bg-white hover:border-primary/30"
              >
                {/* Glow effect */}
                <div className="absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 bg-primary/5" />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${
                        statusStyle.bg
                      } ${statusStyle.text}`}
                    >
                      <FileText className="h-5 w-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.providerName}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.providerId}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span className="text-[12px] text-ink-500">{item.role}</span>
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">{item.branch}</span>
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">{item.period}</span>
                      </div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                  >
                    {statusStyle.icon}
                    {item.status}
                  </span>
                </div>

                <div className="relative z-10 mt-3 flex items-center justify-between">
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Amount</div>
                    <div className="text-lg font-bold text-ink-950">{formattedTotal}</div>
                  </div>
                  <div className="text-right">
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Items</div>
                    <div className="text-sm font-semibold text-ink-950">{item.items.length}</div>
                  </div>
                </div>

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Generated: {item.generatedAt}
                  </div>
                  <div className="flex items-center gap-1 text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors">
                    View <ArrowRight className="h-3.5 w-3.5 group-hover:translate-x-0.5 transition-transform" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileText className="h-4 w-4" />
          FIN-13 · Provider Compensation Statements
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} statements shown
        </span>
      </div>
    </div>
  );
}