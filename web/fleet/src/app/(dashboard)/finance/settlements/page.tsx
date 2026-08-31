// src/app/(dashboard)/finance/settlements/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  HandCoins,
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
  User,
  MapPin,
  Briefcase,
  CreditCard,
  Receipt,
  FileCheck,
  Truck,
  Package,
} from "lucide-react";

type SubcontractorSettlement = {
  id: string;
  reference: string;
  supplier: string;
  supplierId: string;
  supplierInitials: string;
  serviceType: "Hauling" | "Dispatch" | "Warehouse" | "Maintenance";
  jobRef: string;
  jobDescription: string;
  amount: number;
  currency: string;
  status: "Draft" | "Pending" | "Approved" | "Posted" | "Reconciling" | "Failed";
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  postedAt: string | null;
  paymentReference: string | null;
  paymentMethod: string | null;
  externalRef: string | null;
  branch: string;
  notes: string | null;
  lineItems: {
    id: string;
    description: string;
    quantity: number;
    unitPrice: number;
    total: number;
  }[];
};

// Mock data
const SETTLEMENTS: SubcontractorSettlement[] = [
  {
    id: "SET-001",
    reference: "SET-2026-001",
    supplier: "Greenline Logistics",
    supplierId: "SUP-001",
    supplierInitials: "GL",
    serviceType: "Hauling",
    jobRef: "HAUL-042",
    jobDescription: "Hauling services - Ikeja to Apapa",
    amount: 850000,
    currency: "NGN",
    status: "Approved",
    createdAt: "2026-08-18",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-20",
    postedAt: null,
    paymentReference: null,
    paymentMethod: null,
    externalRef: "PO-2026-042",
    branch: "Ikeja",
    notes: "Hauling services for August week 3.",
    lineItems: [
      { id: "LI-001", description: "Hauling - Ikeja to Apapa", quantity: 4, unitPrice: 212500, total: 850000 },
    ],
  },
  {
    id: "SET-002",
    reference: "SET-2026-002",
    supplier: "Solaris Logistics",
    supplierId: "SUP-002",
    supplierInitials: "SL",
    serviceType: "Maintenance",
    jobRef: "MAINT-019",
    jobDescription: "Fleet maintenance services",
    amount: 450000,
    currency: "NGN",
    status: "Pending",
    createdAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "O. Adebayo",
    approvedBy: null,
    approvedAt: null,
    postedAt: null,
    paymentReference: null,
    paymentMethod: null,
    externalRef: "PO-2026-041",
    branch: "Apapa",
    notes: "Scheduled fleet maintenance.",
    lineItems: [
      { id: "LI-002", description: "Truck TR-098 brake repair", quantity: 2, unitPrice: 150000, total: 300000 },
      { id: "LI-003", description: "Van VN-041 oil change", quantity: 1, unitPrice: 150000, total: 150000 },
    ],
  },
  {
    id: "SET-003",
    reference: "SET-2026-003",
    supplier: "BlueMarket Logistics",
    supplierId: "SUP-003",
    supplierInitials: "BL",
    serviceType: "Dispatch",
    jobRef: "DISP-038",
    jobDescription: "Dispatch services - Lekki",
    amount: 320000,
    currency: "NGN",
    status: "Posted",
    createdAt: "2026-08-15",
    updatedAt: "2026-08-19",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-17",
    postedAt: "2026-08-19",
    paymentReference: "PAY-2026-015",
    paymentMethod: "Bank Transfer",
    externalRef: "PO-2026-040",
    branch: "Lekki",
    notes: "Dispatch services for August week 2.",
    lineItems: [
      { id: "LI-004", description: "Dispatch - Lekki Phase 1", quantity: 10, unitPrice: 32000, total: 320000 },
    ],
  },
  {
    id: "SET-004",
    reference: "SET-2026-004",
    supplier: "Greenline Logistics",
    supplierId: "SUP-001",
    supplierInitials: "GL",
    serviceType: "Hauling",
    jobRef: "HAUL-039",
    jobDescription: "Hauling services - Apapa to Ikeja",
    amount: 620000,
    currency: "NGN",
    status: "Reconciling",
    createdAt: "2026-08-12",
    updatedAt: "2026-08-16",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-14",
    postedAt: "2026-08-16",
    paymentReference: "PAY-2026-013",
    paymentMethod: "Bank Transfer",
    externalRef: "PO-2026-038",
    branch: "Apapa",
    notes: "Hauling services for August week 2.",
    lineItems: [
      { id: "LI-005", description: "Hauling - Apapa to Ikeja", quantity: 3, unitPrice: 206667, total: 620000 },
    ],
  },
  {
    id: "SET-005",
    reference: "SET-2026-005",
    supplier: "Oakfield Maintenance",
    supplierId: "SUP-004",
    supplierInitials: "OM",
    serviceType: "Maintenance",
    jobRef: "MAINT-016",
    jobDescription: "Fleet maintenance services",
    amount: 180000,
    currency: "NGN",
    status: "Failed",
    createdAt: "2026-08-08",
    updatedAt: "2026-08-10",
    createdBy: "O. Adebayo",
    approvedBy: "F. Daniel",
    approvedAt: "2026-08-09",
    postedAt: "2026-08-10",
    paymentReference: null,
    paymentMethod: null,
    externalRef: "PO-2026-035",
    branch: "Ikeja",
    notes: "Posting failed due to account issues.",
    lineItems: [
      { id: "LI-006", description: "Tire replacement BR-204", quantity: 4, unitPrice: 45000, total: 180000 },
    ],
  },
  {
    id: "SET-006",
    reference: "SET-2026-006",
    supplier: "Solaris Logistics",
    supplierId: "SUP-002",
    supplierInitials: "SL",
    serviceType: "Warehouse",
    jobRef: "WH-042",
    jobDescription: "Warehouse services - Ikeja",
    amount: 240000,
    currency: "NGN",
    status: "Draft",
    createdAt: "2026-08-21",
    updatedAt: "2026-08-21",
    createdBy: "A. Yusuf",
    approvedBy: null,
    approvedAt: null,
    postedAt: null,
    paymentReference: null,
    paymentMethod: null,
    externalRef: "PO-2026-043",
    branch: "Ikeja",
    notes: "Warehouse staffing services for August.",
    lineItems: [
      { id: "LI-007", description: "Warehouse staffing", quantity: 4, unitPrice: 60000, total: 240000 },
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
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
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

const SERVICE_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Hauling: { bg: "bg-amber-bg", text: "text-amber" },
  Dispatch: { bg: "bg-blue-bg", text: "text-blue" },
  Warehouse: { bg: "bg-green-bg", text: "text-green" },
  Maintenance: { bg: "bg-purple-bg", text: "text-purple" },
};

const STATUS_FILTERS = ["All", "Draft", "Pending", "Approved", "Posted", "Reconciling", "Failed"];
const SERVICE_TYPE_FILTERS = ["All", "Hauling", "Dispatch", "Warehouse", "Maintenance"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function SubcontractorSettlementsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedServiceType, setSelectedServiceType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SubcontractorSettlement[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(SETTLEMENTS);
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
      item.supplier.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.jobDescription.toLowerCase().includes(search.toLowerCase()) ||
      item.jobRef.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesServiceType = selectedServiceType === "All" || item.serviceType === selectedServiceType;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesServiceType && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading settlements...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedServiceType !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedServiceType("All");
    setSelectedBranch("All branches");
  };

  const goToSettlementDetail = (settlementId: string) => {
    router.push(`/finance/settlements/${settlementId}`);
  };

  const goToCreateSettlement = () => {
    router.push("/finance/settlements/create");
  };

  // Stats
  const totalAmount = data.reduce((acc, item) => acc + item.amount, 0);
  const pendingCount = data.filter((item) => item.status === "Pending").length;
  const approvedCount = data.filter((item) => item.status === "Approved").length;
  const postedCount = data.filter((item) => item.status === "Posted").length;
  const failedCount = data.filter((item) => item.status === "Failed").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Subcontractor Settlements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Supplier, completed jobs, amount, approval, external posting/payment reference and state.
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
            onClick={goToCreateSettlement}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Settlement
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
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
            placeholder="Search by supplier, reference, or job..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Pending", "Approved", "Posted", "Reconciling", "Failed"];
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
            const serviceTypes = ["All", "Hauling", "Dispatch", "Warehouse", "Maintenance"];
            const currentIndex = serviceTypes.indexOf(selectedServiceType);
            const nextIndex = (currentIndex + 1) % serviceTypes.length;
            setSelectedServiceType(serviceTypes[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Briefcase className="h-4 w-4 text-ink-400" />
          {selectedServiceType}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedServiceType !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Service Type</div>
              <div className="flex gap-1.5">
                {SERVICE_TYPE_FILTERS.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedServiceType(type)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedServiceType === type
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

      {/* ===== SETTLEMENTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <HandCoins className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No settlements found</h3>
          <p className="mt-1 text-sm text-ink-500">No settlements match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[950px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Supplier</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Service</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Job</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const serviceStyle = SERVICE_TYPE_STYLES[item.serviceType] || SERVICE_TYPE_STYLES.Dispatch;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.amount);

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToSettlementDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.externalRef || "—"}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-white">
                            {item.supplierInitials}
                          </div>
                          <div>
                            <div className="font-medium text-ink-950">{item.supplier}</div>
                            <div className="text-[11px] text-ink-500 font-mono">{item.supplierId}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${serviceStyle.bg} ${serviceStyle.text}`}>
                          {item.serviceType}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">{item.jobRef}</div>
                        <div className="text-[11px] text-ink-500">{item.jobDescription}</div>
                      </td>
                      <td className="px-4 py-3 font-semibold text-ink-950">{formattedAmount}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
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
          <HandCoins className="h-4 w-4" />
          FIN-14 · Subcontractor Settlements
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} settlements shown
        </span>
      </div>
    </div>
  );
}