// src/app/(dashboard)/crm/quotations/page.tsx

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
  Plus,
  DollarSign,
  Building2,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Tag,
  Shield,
  CreditCard,
  Receipt,
  Star,
  UserPlus,
  Send,
  Edit3,
  Trash2,
  Zap,
  Target,
  PieChart,
  BarChart3,
  FileCheck,
  HandCoins,
} from "lucide-react";

type Quotation = {
  id: string;
  reference: string;
  customer: string;
  customerId: string;
  customerInitials: string;
  version: number;
  status: "Draft" | "Issued" | "Accepted" | "Rejected" | "Expired" | "Converted";
  amount: number;
  currency: string;
  validityDays: number;
  issueDate: string;
  expiryDate: string;
  acceptedDate: string | null;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  serviceScope: string;
  items: number;
  branch: string;
  notes: string | null;
  contractRef: string | null;
};

// Mock data
const QUOTATIONS: Quotation[] = [
  {
    id: "QT-001",
    reference: "QT-2026-012",
    customer: "Lagos Medical Center",
    customerId: "CUST-009",
    customerInitials: "LM",
    version: 1,
    status: "Issued",
    amount: 1200000,
    currency: "NGN",
    validityDays: 30,
    issueDate: "2026-08-19",
    expiryDate: "2026-09-18",
    acceptedDate: null,
    createdAt: "2026-08-19",
    updatedAt: "2026-08-19",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    serviceScope: "Medical logistics - 3 months",
    items: 4,
    branch: "Ikeja",
    notes: "Includes medical supply delivery services.",
    contractRef: null,
  },
  {
    id: "QT-002",
    reference: "QT-2026-008",
    customer: "Eze & Partners",
    customerId: "CUST-012",
    customerInitials: "EP",
    version: 2,
    status: "Accepted",
    amount: 950000,
    currency: "NGN",
    validityDays: 30,
    issueDate: "2026-08-10",
    expiryDate: "2026-09-09",
    acceptedDate: "2026-08-15",
    createdAt: "2026-08-10",
    updatedAt: "2026-08-15",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    serviceScope: "Warehouse services - 6 months",
    items: 3,
    branch: "Ikeja",
    notes: "Accepted. Contract being drafted.",
    contractRef: "CT-2026-005",
  },
  {
    id: "QT-003",
    reference: "QT-2026-005",
    customer: "Greenline Foods",
    customerId: "CUST-003",
    customerInitials: "GF",
    version: 1,
    status: "Expired",
    amount: 2100000,
    currency: "NGN",
    validityDays: 14,
    issueDate: "2026-07-15",
    expiryDate: "2026-07-29",
    acceptedDate: null,
    createdAt: "2026-07-15",
    updatedAt: "2026-07-29",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    serviceScope: "Cold chain logistics - 6 months",
    items: 5,
    branch: "Ikeja",
    notes: "Expired. Customer did not respond.",
    contractRef: null,
  },
  {
    id: "QT-004",
    reference: "QT-2026-003",
    customer: "Charitos Hospital",
    customerId: "CUST-001",
    customerInitials: "CH",
    version: 3,
    status: "Converted",
    amount: 1280000,
    currency: "NGN",
    validityDays: 30,
    issueDate: "2026-07-01",
    expiryDate: "2026-07-31",
    acceptedDate: "2026-07-10",
    createdAt: "2026-07-01",
    updatedAt: "2026-07-10",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    serviceScope: "Medical supplies delivery - 12 months",
    items: 6,
    branch: "Ikeja",
    notes: "Converted to contract.",
    contractRef: "CT-2026-001",
  },
  {
    id: "QT-005",
    reference: "QT-2026-001",
    customer: "Oakfield Stores",
    customerId: "CUST-002",
    customerInitials: "OS",
    version: 1,
    status: "Rejected",
    amount: 840000,
    currency: "NGN",
    validityDays: 14,
    issueDate: "2026-06-20",
    expiryDate: "2026-07-04",
    acceptedDate: null,
    createdAt: "2026-06-20",
    updatedAt: "2026-06-25",
    createdBy: "A. Yusuf",
    approvedBy: "F. Daniel",
    serviceScope: "Retail logistics - 3 months",
    items: 3,
    branch: "Ikeja",
    notes: "Rejected due to budget constraints.",
    contractRef: null,
  },
  {
    id: "QT-006",
    reference: "QT-2026-015",
    customer: "Ogunyemi Global",
    customerId: "CUST-013",
    customerInitials: "OG",
    version: 1,
    status: "Draft",
    amount: 1800000,
    currency: "NGN",
    validityDays: 30,
    issueDate: "2026-08-21",
    expiryDate: "2026-09-20",
    acceptedDate: null,
    createdAt: "2026-08-21",
    updatedAt: "2026-08-21",
    createdBy: "M. Bello",
    approvedBy: null,
    serviceScope: "Warehousing & distribution - 6 months",
    items: 4,
    branch: "Lekki",
    notes: "Draft. Awaiting approval.",
    contractRef: null,
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Issued: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Send className="h-3.5 w-3.5" />,
  },
  Accepted: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <X className="h-3.5 w-3.5" />,
  },
  Expired: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Converted: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <FileCheck className="h-3.5 w-3.5" />,
  },
};

const STATUS_FILTERS = ["All", "Draft", "Issued", "Accepted", "Rejected", "Expired", "Converted"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function QuotationsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Quotation[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(QUOTATIONS);
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
      item.customer.toLowerCase().includes(search.toLowerCase()) ||
      item.serviceScope.toLowerCase().includes(search.toLowerCase()) ||
      item.createdBy.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading quotations...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const goToQuotationDetail = (quotationId: string) => {
    router.push(`/crm/quotations/${quotationId}`);
  };

  const goToCreateQuotation = () => {
    router.push("/crm/quotations/create");
  };

  // Stats
  const totalQuotations = data.length;
  const draftCount = data.filter((d) => d.status === "Draft").length;
  const issuedCount = data.filter((d) => d.status === "Issued").length;
  const acceptedCount = data.filter((d) => d.status === "Accepted" || d.status === "Converted").length;
  const totalAmount = data.reduce((acc, d) => acc + d.amount, 0);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Quotations
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Quotation number/customer, version, status, validity, amount snapshot, service scope and acceptance state.
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
            onClick={goToCreateQuotation}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Quotation
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{totalQuotations}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Draft</div>
          <div className="text-lg font-bold text-blue">{draftCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Issued</div>
          <div className="text-lg font-bold text-amber">{issuedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Accepted</div>
          <div className="text-lg font-bold text-green">{acceptedCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, customer, or service..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Issued", "Accepted", "Rejected", "Expired", "Converted"];
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

      {/* ===== QUOTATIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileText className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No quotations found</h3>
          <p className="mt-1 text-sm text-ink-500">No quotations match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Customer</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Expiry</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Version</th>
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
                  }).format(item.amount);
                  const isExpiringSoon = new Date(item.expiryDate) < new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);
                  const isExpired = new Date(item.expiryDate) < new Date();

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToQuotationDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.serviceScope}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-white">
                            {item.customerInitials}
                          </div>
                          <div>
                            <div className="font-medium text-ink-950">{item.customer}</div>
                            <div className="text-[11px] text-ink-500 font-mono">{item.customerId}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3 font-semibold text-ink-950">{formattedAmount}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`text-[12px] ${isExpired ? "text-red" : isExpiringSoon ? "text-amber" : "text-ink-600"}`}>
                          {item.expiryDate}
                          {isExpired && <span className="ml-1 text-red">(Expired)</span>}
                          {!isExpired && isExpiringSoon && <span className="ml-1 text-amber">(Soon)</span>}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500">v{item.version}</td>
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
          <FileText className="h-4 w-4" />
          CRM-09 · Quotations
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} quotations shown
        </span>
      </div>
    </div>
  );
}