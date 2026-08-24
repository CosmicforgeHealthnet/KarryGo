// src/app/(dashboard)/crm/contracts/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FileCheck,
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
  FileCheck as FileCheckIcon,
  HandCoins,
  Calendar,
  Check,
  XCircle,
} from "lucide-react";

type Contract = {
  id: string;
  reference: string;
  customer: string;
  customerId: string;
  customerInitials: string;
  version: number;
  status: "Draft" | "Active" | "Expired" | "Terminated" | "Renewal Pending";
  amount: number;
  currency: string;
  startDate: string;
  endDate: string;
  renewalDate: string | null;
  serviceScope: string;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  branch: string;
  notes: string | null;
  quotationRef: string | null;
  slaRef: string | null;
};

// Mock data
const CONTRACTS: Contract[] = [
  {
    id: "CT-001",
    reference: "CT-2026-001",
    customer: "Charitos Hospital",
    customerId: "CUST-001",
    customerInitials: "CH",
    version: 1,
    status: "Active",
    amount: 1280000,
    currency: "NGN",
    startDate: "2026-08-01",
    endDate: "2027-07-31",
    renewalDate: "2027-06-01",
    serviceScope: "Medical supplies delivery - 12 months",
    createdAt: "2026-07-25",
    updatedAt: "2026-08-01",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-30",
    branch: "Ikeja",
    notes: "Annual contract for medical logistics.",
    quotationRef: "QT-2026-003",
    slaRef: "SLA-001",
  },
  {
    id: "CT-002",
    reference: "CT-2026-002",
    customer: "Oakfield Stores",
    customerId: "CUST-002",
    customerInitials: "OS",
    version: 1,
    status: "Draft",
    amount: 840000,
    currency: "NGN",
    startDate: "2026-09-01",
    endDate: "2026-11-30",
    renewalDate: null,
    serviceScope: "Retail logistics - 3 months",
    createdAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "A. Yusuf",
    approvedBy: null,
    approvedAt: null,
    branch: "Ikeja",
    notes: "Draft contract awaiting approval.",
    quotationRef: "QT-2026-001",
    slaRef: null,
  },
  {
    id: "CT-003",
    reference: "CT-2026-003",
    customer: "Greenline Foods",
    customerId: "CUST-003",
    customerInitials: "GF",
    version: 2,
    status: "Active",
    amount: 2100000,
    currency: "NGN",
    startDate: "2026-07-01",
    endDate: "2026-12-31",
    renewalDate: "2026-11-01",
    serviceScope: "Cold chain logistics - 6 months",
    createdAt: "2026-06-15",
    updatedAt: "2026-07-01",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-06-25",
    branch: "Ikeja",
    notes: "Version 2 includes cold chain requirements.",
    quotationRef: "QT-2026-005",
    slaRef: "SLA-003",
  },
  {
    id: "CT-004",
    reference: "CT-2026-004",
    customer: "Solaris Construction",
    customerId: "CUST-004",
    customerInitials: "SC",
    version: 1,
    status: "Terminated",
    amount: 560000,
    currency: "NGN",
    startDate: "2026-05-01",
    endDate: "2026-08-31",
    renewalDate: null,
    serviceScope: "Construction logistics - 4 months",
    createdAt: "2026-04-15",
    updatedAt: "2026-08-15",
    createdBy: "M. Bello",
    approvedBy: "F. Daniel",
    approvedAt: "2026-04-20",
    branch: "Lekki",
    notes: "Terminated early due to project completion.",
    quotationRef: "QT-2026-004",
    slaRef: null,
  },
  {
    id: "CT-005",
    reference: "CT-2026-005",
    customer: "Eze & Partners",
    customerId: "CUST-012",
    customerInitials: "EP",
    version: 1,
    status: "Active",
    amount: 5000000,
    currency: "NGN",
    startDate: "2026-09-01",
    endDate: "2027-08-31",
    renewalDate: "2027-07-01",
    serviceScope: "Warehouse & distribution - 12 months",
    createdAt: "2026-08-10",
    updatedAt: "2026-08-15",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-15",
    branch: "Ikeja",
    notes: "Major contract for warehouse services.",
    quotationRef: "QT-2026-008",
    slaRef: "SLA-005",
  },
  {
    id: "CT-006",
    reference: "CT-2026-006",
    customer: "Lagos Medical Center",
    customerId: "CUST-009",
    customerInitials: "LM",
    version: 1,
    status: "Draft",
    amount: 2800000,
    currency: "NGN",
    startDate: "2026-10-01",
    endDate: "2027-09-30",
    renewalDate: "2027-08-01",
    serviceScope: "Medical logistics - 12 months",
    createdAt: "2026-08-22",
    updatedAt: "2026-08-22",
    createdBy: "F. Daniel",
    approvedBy: null,
    approvedAt: null,
    branch: "Ikeja",
    notes: "New contract for Lagos Medical Center.",
    quotationRef: "QT-2026-012",
    slaRef: null,
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
  Expired: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Terminated: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  "Renewal Pending": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Calendar className="h-3.5 w-3.5" />,
  },
};

const STATUS_FILTERS = ["All", "Draft", "Active", "Expired", "Terminated", "Renewal Pending"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ContractsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Contract[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(CONTRACTS);
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
        <p className="mt-3 text-sm text-ink-600">Loading contracts...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const goToContractDetail = (contractId: string) => {
    router.push(`/crm/contracts/${contractId}`);
  };

  const goToCreateContract = () => {
    router.push("/crm/contracts/create");
  };

  // Stats
  const totalContracts = data.length;
  const activeCount = data.filter((d) => d.status === "Active").length;
  const draftCount = data.filter((d) => d.status === "Draft").length;
  const renewalPendingCount = data.filter((d) => d.status === "Renewal Pending").length;
  const totalAmount = data.reduce((acc, d) => acc + d.amount, 0);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Contracts
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Customer, effective dates, status, current version, service scope and renewal state.
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
            onClick={goToCreateContract}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Contract
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{totalContracts}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{activeCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Draft</div>
          <div className="text-lg font-bold text-blue">{draftCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Renewal Pending</div>
          <div className="text-lg font-bold text-amber">{renewalPendingCount}</div>
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
            const statuses = ["All", "Draft", "Active", "Expired", "Terminated", "Renewal Pending"];
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

      {/* ===== CONTRACT CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No contracts found</h3>
          <p className="mt-1 text-sm text-ink-500">No contracts match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
            const formattedAmount = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(item.amount);
            const isActive = item.status === "Active";
            const isExpiringSoon = new Date(item.endDate) < new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);
            const isExpired = new Date(item.endDate) < new Date();

            return (
              <div
                key={item.id}
                onClick={() => goToContractDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  isActive
                    ? "border-green/20 bg-white hover:border-green/40"
                    : item.status === "Draft"
                    ? "border-blue/20 bg-blue-bg/10 hover:border-blue/40"
                    : item.status === "Terminated"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : "border-line bg-white/60 hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  isActive ? "bg-green-500/5" :
                  item.status === "Draft" ? "bg-blue-500/5" :
                  item.status === "Terminated" ? "bg-red-500/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full text-[16px] font-bold text-white shadow-lg ${
                        isActive ? "bg-green" :
                        item.status === "Draft" ? "bg-blue" :
                        item.status === "Terminated" ? "bg-red" :
                        "bg-ink-400"
                      }`}
                    >
                      {item.customerInitials}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.customer}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.reference}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          {statusStyle.icon}
                          {item.status}
                        </span>
                        <span className="text-[11px] text-ink-400">v{item.version}</span>
                        {isExpired && (
                          <span className="text-[11px] text-red">Expired</span>
                        )}
                        {isExpiringSoon && !isExpired && (
                          <span className="text-[11px] text-amber">Expiring soon</span>
                        )}
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-2">
                  <div className="text-sm font-medium text-ink-950">{item.serviceScope}</div>
                  <div className="mt-1 flex flex-wrap items-center gap-3 text-[12px] text-ink-500">
                    <span className="flex items-center gap-1.5">
                      <Building2 className="h-3.5 w-3.5" />
                      {item.branch}
                    </span>
                    <span className="text-ink-300">·</span>
                    <span className="flex items-center gap-1.5">
                      <User className="h-3.5 w-3.5" />
                      {item.createdBy}
                    </span>
                  </div>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <DollarSign className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-semibold text-ink-950">{formattedAmount}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <CalendarDays className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.startDate} → {item.endDate}</span>
                  </div>
                  {item.renewalDate && (
                    <div className="flex items-center gap-1.5 text-[12px] text-ink-500 col-span-2">
                      <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                      <span>Renewal: {item.renewalDate}</span>
                    </div>
                  )}
                </div>

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
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
          <FileCheck className="h-4 w-4" />
          CRM-13 · Contracts
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} contracts shown
        </span>
      </div>
    </div>
  );
}