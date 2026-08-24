// src/app/(dashboard)/finance/expenses/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Wallet,
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
  User,
  Mail,
  Phone,
  Receipt,
  CreditCard,
  Briefcase,
  Tag,
  Calendar,
  Send,
  Edit3,
  Trash2,
} from "lucide-react";

type Expense = {
  id: string;
  reference: string;
  category: string;
  description: string;
  amount: number;
  currency: string;
  status: "Approved" | "Pending" | "Rejected" | "Draft";
  date: string;
  submitter: string;
  submitterId: string;
  branch: string;
  costCentre: string;
  receipt: boolean;
  orderRef: string | null;
  assetRef: string | null;
  providerRef: string | null;
  approvedBy: string | null;
  approvedAt: string | null;
};

// Mock data
const EXPENSES: Expense[] = [
  {
    id: "EXP-001",
    reference: "EXP-2026-001",
    category: "Fuel",
    description: "Fuel for fleet vehicles - August week 3",
    amount: 285000,
    currency: "NGN",
    status: "Approved",
    date: "2026-08-18",
    submitter: "M. Bello",
    submitterId: "P-038",
    branch: "Ikeja",
    costCentre: "Operations",
    receipt: true,
    orderRef: null,
    assetRef: "TR-112",
    providerRef: null,
    approvedBy: "F. Daniel",
    approvedAt: "2026-08-19",
  },
  {
    id: "EXP-002",
    reference: "EXP-2026-002",
    category: "Maintenance",
    description: "Emergency brake repair - Truck TR-098",
    amount: 450000,
    currency: "NGN",
    status: "Pending",
    date: "2026-08-16",
    submitter: "O. Adebayo",
    submitterId: "P-052",
    branch: "Apapa",
    costCentre: "Fleet",
    receipt: true,
    orderRef: "WO-1917",
    assetRef: "TR-098",
    providerRef: "P-029",
    approvedBy: null,
    approvedAt: null,
  },
  {
    id: "EXP-003",
    reference: "EXP-2026-003",
    category: "Office Supplies",
    description: "Stationery and printing supplies",
    amount: 95000,
    currency: "NGN",
    status: "Approved",
    date: "2026-08-14",
    submitter: "F. Daniel",
    submitterId: "P-073",
    branch: "Ikeja",
    costCentre: "Admin",
    receipt: true,
    orderRef: null,
    assetRef: null,
    providerRef: null,
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-15",
  },
  {
    id: "EXP-004",
    reference: "EXP-2026-004",
    category: "Fuel",
    description: "Fuel for Lekki operations",
    amount: 180000,
    currency: "NGN",
    status: "Approved",
    date: "2026-08-12",
    submitter: "M. Bello",
    submitterId: "P-038",
    branch: "Lekki",
    costCentre: "Operations",
    receipt: true,
    orderRef: null,
    assetRef: "VN-041",
    providerRef: null,
    approvedBy: "F. Daniel",
    approvedAt: "2026-08-13",
  },
  {
    id: "EXP-005",
    reference: "EXP-2026-005",
    category: "Maintenance",
    description: "Scheduled oil change - Fleet",
    amount: 320000,
    currency: "NGN",
    status: "Rejected",
    date: "2026-08-10",
    submitter: "O. Adebayo",
    submitterId: "P-052",
    branch: "Ikeja",
    costCentre: "Fleet",
    receipt: false,
    orderRef: null,
    assetRef: "BR-204",
    providerRef: null,
    approvedBy: "F. Daniel",
    approvedAt: "2026-08-11",
  },
  {
    id: "EXP-006",
    reference: "EXP-2026-006",
    category: "Software",
    description: "Fleet management software subscription",
    amount: 240000,
    currency: "NGN",
    status: "Pending",
    date: "2026-08-08",
    submitter: "F. Daniel",
    submitterId: "P-073",
    branch: "Ikeja",
    costCentre: "IT",
    receipt: true,
    orderRef: null,
    assetRef: null,
    providerRef: null,
    approvedBy: null,
    approvedAt: null,
  },
  {
    id: "EXP-007",
    reference: "EXP-2026-007",
    category: "Travel",
    description: "Staff travel expenses - Ikeja to Apapa",
    amount: 45000,
    currency: "NGN",
    status: "Draft",
    date: "2026-08-20",
    submitter: "A. Yusuf",
    submitterId: "P-044",
    branch: "Ikeja",
    costCentre: "Operations",
    receipt: false,
    orderRef: null,
    assetRef: null,
    providerRef: null,
    approvedBy: null,
    approvedAt: null,
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Approved: {
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
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
};

const CATEGORY_STYLES: Record<string, { bg: string; text: string }> = {
  Fuel: { bg: "bg-amber-bg", text: "text-amber" },
  Maintenance: { bg: "bg-blue-bg", text: "text-blue" },
  "Office Supplies": { bg: "bg-green-bg", text: "text-green" },
  Software: { bg: "bg-purple-bg", text: "text-purple" },
  Travel: { bg: "bg-pink-bg", text: "text-pink" },
};

const STATUS_FILTERS = ["All", "Approved", "Pending", "Rejected", "Draft"];
const CATEGORY_FILTERS = ["All", "Fuel", "Maintenance", "Office Supplies", "Software", "Travel"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ExpensesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Expense[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(EXPENSES);
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
      item.submitter.toLowerCase().includes(search.toLowerCase()) ||
      item.category.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesCategory && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading expenses...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedCategory !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedCategory("All");
    setSelectedBranch("All branches");
  };

  const goToExpenseDetail = (expenseId: string) => {
    router.push(`/finance/expenses/${expenseId}`);
  };

  const goToCreateExpense = () => {
    router.push("/finance/expenses/create");
  };

  // Stats
  const totalAmount = data.reduce((acc, item) => acc + item.amount, 0);
  const pendingAmount = data.filter((item) => item.status === "Pending").reduce((acc, item) => acc + item.amount, 0);
  const approvedAmount = data.filter((item) => item.status === "Approved").reduce((acc, item) => acc + item.amount, 0);
  const pendingCount = data.filter((item) => item.status === "Pending").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Expenses
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Category, cost centre, branch, asset/provider/order reference, amount, receipt, submitter and state.
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
            onClick={goToCreateExpense}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            New Expense
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Expenses</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{pendingCount}</div>
          <div className="text-[10px] text-ink-400">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(pendingAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Approved</div>
          <div className="text-lg font-bold text-green">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(approvedAmount)}
          </div>
          <div className="text-[10px] text-ink-400">{data.filter((d) => d.status === "Approved").length} items</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Rejected</div>
          <div className="text-lg font-bold text-red">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(
              data.filter((item) => item.status === "Rejected").reduce((acc, item) => acc + item.amount, 0)
            )}
          </div>
          <div className="text-[10px] text-ink-400">{data.filter((d) => d.status === "Rejected").length} items</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, description, or submitter..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Approved", "Pending", "Rejected", "Draft"];
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedCategory !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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

      {/* ===== EXPENSES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Wallet className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No expenses found</h3>
          <p className="mt-1 text-sm text-ink-500">No expenses match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Description</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Category</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Date</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const categoryStyle = CATEGORY_STYLES[item.category] || CATEGORY_STYLES.Fuel;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.amount);

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToExpenseDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.submitter}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">{item.description}</div>
                        <div className="text-[11px] text-ink-500">{item.costCentre} · {item.branch}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                          {item.category}
                        </span>
                        {item.receipt && (
                          <span className="ml-1 rounded-full bg-surface-2 px-1.5 py-0.5 text-[9px] text-ink-400">
                            📎
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 font-semibold text-ink-950">{formattedAmount}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-[12px]">{item.date}</td>
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
          <Wallet className="h-4 w-4" />
          FIN-06 · Expenses
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} expenses shown
        </span>
      </div>
    </div>
  );
}