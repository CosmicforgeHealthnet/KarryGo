// src/app/(dashboard)/finance/budgets/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  PieChart,
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
} from "lucide-react";

type Budget = {
  id: string;
  reference: string;
  costCentre: string;
  category: string;
  period: string;
  approvedAmount: number;
  committed: number;
  actual: number;
  remaining: number;
  currency: string;
  status: "Draft" | "Active" | "Approved" | "Closed";
  utilization: number;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  branch: string;
  notes: string | null;
  lineItems: {
    id: string;
    description: string;
    allocated: number;
    spent: number;
    committed: number;
    remaining: number;
  }[];
};

// Mock data
const BUDGETS: Budget[] = [
  {
    id: "BUD-001",
    reference: "BUD-2026-001",
    costCentre: "Operations",
    category: "Fuel",
    period: "Q3 2026",
    approvedAmount: 1200000,
    committed: 280000,
    actual: 285000,
    remaining: 635000,
    currency: "NGN",
    status: "Active",
    utilization: 47,
    createdAt: "2026-07-01",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-05",
    branch: "Ikeja",
    notes: "Fuel budget for all fleet vehicles.",
    lineItems: [
      { id: "LI-001", description: "Monthly fuel allocation", allocated: 400000, spent: 285000, committed: 0, remaining: 115000 },
      { id: "LI-002", description: "Reserve fuel", allocated: 200000, spent: 0, committed: 0, remaining: 200000 },
    ],
  },
  {
    id: "BUD-002",
    reference: "BUD-2026-002",
    costCentre: "Fleet",
    category: "Maintenance",
    period: "Q3 2026",
    approvedAmount: 1800000,
    committed: 450000,
    actual: 450000,
    remaining: 900000,
    currency: "NGN",
    status: "Active",
    utilization: 50,
    createdAt: "2026-07-01",
    updatedAt: "2026-08-16",
    createdBy: "O. Adebayo",
    approvedBy: "F. Daniel",
    approvedAt: "2026-07-03",
    branch: "Apapa",
    notes: "Fleet maintenance and repairs.",
    lineItems: [
      { id: "LI-003", description: "Scheduled maintenance", allocated: 900000, spent: 450000, committed: 0, remaining: 450000 },
      { id: "LI-004", description: "Emergency repairs", allocated: 600000, spent: 0, committed: 0, remaining: 600000 },
    ],
  },
  {
    id: "BUD-003",
    reference: "BUD-2026-003",
    costCentre: "Admin",
    category: "Office Supplies",
    period: "Q3 2026",
    approvedAmount: 300000,
    committed: 0,
    actual: 95000,
    remaining: 205000,
    currency: "NGN",
    status: "Active",
    utilization: 32,
    createdAt: "2026-07-01",
    updatedAt: "2026-08-14",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-05",
    branch: "Ikeja",
    notes: "Office supplies and stationery.",
    lineItems: [
      { id: "LI-005", description: "Stationery", allocated: 150000, spent: 95000, committed: 0, remaining: 55000 },
      { id: "LI-006", description: "Printing supplies", allocated: 150000, spent: 0, committed: 0, remaining: 150000 },
    ],
  },
  {
    id: "BUD-004",
    reference: "BUD-2026-004",
    costCentre: "IT",
    category: "Software",
    period: "Q3 2026",
    approvedAmount: 600000,
    committed: 240000,
    actual: 240000,
    remaining: 120000,
    currency: "NGN",
    status: "Approved",
    utilization: 80,
    createdAt: "2026-07-15",
    updatedAt: "2026-08-08",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-20",
    branch: "Ikeja",
    notes: "Software subscriptions and licenses.",
    lineItems: [
      { id: "LI-007", description: "Fleet management software", allocated: 240000, spent: 240000, committed: 0, remaining: 0 },
      { id: "LI-008", description: "Accounting software", allocated: 360000, spent: 0, committed: 0, remaining: 360000 },
    ],
  },
  {
    id: "BUD-005",
    reference: "BUD-2026-005",
    costCentre: "Operations",
    category: "Travel",
    period: "Q3 2026",
    approvedAmount: 150000,
    committed: 0,
    actual: 0,
    remaining: 150000,
    currency: "NGN",
    status: "Draft",
    utilization: 0,
    createdAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "A. Yusuf",
    approvedBy: null,
    approvedAt: null,
    branch: "Ikeja",
    notes: "Staff travel and transportation.",
    lineItems: [
      { id: "LI-009", description: "Local travel", allocated: 100000, spent: 0, committed: 0, remaining: 100000 },
      { id: "LI-010", description: "Miscellaneous", allocated: 50000, spent: 0, committed: 0, remaining: 50000 },
    ],
  },
  {
    id: "BUD-006",
    reference: "BUD-2026-006",
    costCentre: "Operations",
    category: "Fuel",
    period: "Q4 2026",
    approvedAmount: 1500000,
    committed: 0,
    actual: 0,
    remaining: 1500000,
    currency: "NGN",
    status: "Draft",
    utilization: 0,
    createdAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: null,
    approvedAt: null,
    branch: "Ikeja",
    notes: "Q4 Fuel budget.",
    lineItems: [
      { id: "LI-011", description: "Monthly fuel allocation", allocated: 500000, spent: 0, committed: 0, remaining: 500000 },
      { id: "LI-012", description: "Reserve fuel", allocated: 250000, spent: 0, committed: 0, remaining: 250000 },
    ],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Approved: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Closed: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

const CATEGORY_STYLES: Record<string, { bg: string; text: string }> = {
  Fuel: { bg: "bg-amber-bg", text: "text-amber" },
  Maintenance: { bg: "bg-blue-bg", text: "text-blue" },
  "Office Supplies": { bg: "bg-green-bg", text: "text-green" },
  Software: { bg: "bg-purple-bg", text: "text-purple" },
  Travel: { bg: "bg-pink-bg", text: "text-pink" },
};

const STATUS_FILTERS = ["All", "Draft", "Active", "Approved", "Closed"];
const CATEGORY_FILTERS = ["All", "Fuel", "Maintenance", "Office Supplies", "Software", "Travel"];
const COST_CENTRE_FILTERS = ["All", "Operations", "Fleet", "Admin", "IT"];

export default function BudgetsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedCostCentre, setSelectedCostCentre] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Budget[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(BUDGETS);
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
      item.costCentre.toLowerCase().includes(search.toLowerCase()) ||
      item.category.toLowerCase().includes(search.toLowerCase()) ||
      item.period.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;
    const matchesCostCentre = selectedCostCentre === "All" || item.costCentre === selectedCostCentre;

    return matchesSearch && matchesStatus && matchesCategory && matchesCostCentre;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading budgets...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedCategory !== "All" || selectedCostCentre !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedCategory("All");
    setSelectedCostCentre("All");
  };

  const goToBudgetDetail = (budgetId: string) => {
    router.push(`/finance/budgets/${budgetId}`);
  };

  const goToCreateBudget = () => {
    router.push("/finance/budgets/create");
  };

  // Stats
  const totalBudget = data.reduce((acc, item) => acc + item.approvedAmount, 0);
  const totalSpent = data.reduce((acc, item) => acc + item.actual, 0);
  const totalCommitted = data.reduce((acc, item) => acc + item.committed, 0);
  const totalRemaining = data.reduce((acc, item) => acc + item.remaining, 0);
  const avgUtilization = Math.round(data.reduce((acc, item) => acc + item.utilization, 0) / data.length);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Budgets
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Cost centre/category/period, approved amount, committed/actual projection and state.
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
            onClick={goToCreateBudget}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Budget
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Budget</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalBudget)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Spent</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalSpent)}
          </div>
          <div className="text-[10px] text-ink-400">{Math.round((totalSpent / totalBudget) * 100)}% of budget</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Committed</div>
          <div className="text-lg font-bold text-amber">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalCommitted)}
          </div>
          <div className="text-[10px] text-ink-400">Pending approval</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Remaining</div>
          <div className="text-lg font-bold text-green">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalRemaining)}
          </div>
          <div className="text-[10px] text-ink-400">Avg utilization {avgUtilization}%</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, cost centre, or category..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Active", "Approved", "Closed"];
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
            const categories = ["All", "Fuel", "Maintenance", "Office Supplies", "Software", "Travel"];
            const currentIndex = categories.indexOf(selectedCategory);
            const nextIndex = (currentIndex + 1) % categories.length;
            setSelectedCategory(categories[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Tag className="h-4 w-4 text-ink-400" />
          {selectedCategory}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const costCentres = ["All", "Operations", "Fleet", "Admin", "IT"];
            const currentIndex = costCentres.indexOf(selectedCostCentre);
            const nextIndex = (currentIndex + 1) % costCentres.length;
            setSelectedCostCentre(costCentres[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Building2 className="h-4 w-4 text-ink-400" />
          {selectedCostCentre}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedCategory !== "All" ? 1 : 0) + (selectedCostCentre !== "All" ? 1 : 0)}
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
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Category</div>
              <div className="flex gap-1.5">
                {CATEGORY_FILTERS.map((category) => (
                  <button
                    key={category}
                    onClick={() => setSelectedCategory(category)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedCategory === category
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {category}
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

      {/* ===== BUDGETS CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <PieChart className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No budgets found</h3>
          <p className="mt-1 text-sm text-ink-500">No budgets match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
            const categoryStyle = CATEGORY_STYLES[item.category] || CATEGORY_STYLES.Fuel;
            const utilization = item.utilization;
            const isOverUtilized = utilization > 90;
            const isUnderUtilized = utilization < 30;

            return (
              <div
                key={item.id}
                onClick={() => goToBudgetDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  isOverUtilized
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : isUnderUtilized
                    ? "border-blue/20 bg-blue-bg/10 hover:border-blue/40"
                    : item.status === "Active"
                    ? "border-green/20 bg-white hover:border-green/40"
                    : "border-line bg-white/60 hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  isOverUtilized ? "bg-red-500/5" :
                  isUnderUtilized ? "bg-blue-500/5" :
                  item.status === "Active" ? "bg-green-500/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl ${
                        isOverUtilized
                          ? "bg-red-bg text-red"
                          : isUnderUtilized
                          ? "bg-blue-bg text-blue"
                          : item.status === "Active"
                          ? "bg-green-bg text-green"
                          : "bg-primary-bg text-primary"
                      }`}
                    >
                      <PieChart className="h-5 w-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.reference}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.period}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}
                        >
                          {item.category}
                        </span>
                        <span className="text-[10px] text-ink-400">·</span>
                        <span className="text-[10px] text-ink-400">{item.costCentre}</span>
                        <span className="text-[10px] text-ink-400">·</span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <DollarSign className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Approved: {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.approvedAmount)}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Tag className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Remaining: {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.remaining)}</span>
                  </div>
                </div>

                {/* Utilization Bar */}
                <div className="relative z-10 mt-3 border-t border-line pt-2.5">
                  <div className="flex items-center justify-between text-[12px]">
                    <span className="text-ink-500">Utilization</span>
                    <span className={`font-semibold ${
                      utilization >= 90 ? "text-red" :
                      utilization >= 70 ? "text-amber" :
                      "text-green"
                    }`}>
                      {utilization}%
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className={`h-full rounded-full transition-all ${
                        utilization >= 90 ? "bg-red" :
                        utilization >= 70 ? "bg-amber" :
                        "bg-green"
                      }`}
                      style={{ width: `${utilization}%` }}
                    />
                  </div>
                </div>

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <CalendarDays className="h-3.5 w-3.5" />
                    Updated: {item.updatedAt}
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
          <PieChart className="h-4 w-4" />
          FIN-08 · Budgets
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} budgets shown
        </span>
      </div>
    </div>
  );
}