// src/app/(dashboard)/finance/claims/page.tsx

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
  Shield,
  AlertTriangle,
  Info,
  Truck,
  Package,
  HandCoins,
} from "lucide-react";

type Claim = {
  id: string;
  reference: string;
  type: "Incident" | "Damage" | "Loss" | "Liability" | "Theft";
  policyRef: string | null;
  amount: number;
  currency: string;
  status: "Draft" | "Submitted" | "Under Review" | "Approved" | "Rejected" | "Settled" | "Reconciling";
  description: string;
  incidentDate: string;
  submittedAt: string;
  updatedAt: string;
  createdBy: string;
  assignedTo: string | null;
  approvedBy: string | null;
  approvedAt: string | null;
  settledAt: string | null;
  settlementRef: string | null;
  insurer: string | null;
  branch: string;
  documents: number;
  notes: string | null;
  relatedRefs: {
    type: string;
    ref: string;
  }[];
};

// Mock data
const CLAIMS: Claim[] = [
  {
    id: "CLM-001",
    reference: "CLM-2026-001",
    type: "Incident",
    policyRef: "POL-2026-001",
    amount: 250000,
    currency: "NGN",
    status: "Under Review",
    description: "Vehicle accident - Truck TR-098",
    incidentDate: "2026-08-15",
    submittedAt: "2026-08-16",
    updatedAt: "2026-08-20",
    createdBy: "O. Adebayo",
    assignedTo: "F. Daniel",
    approvedBy: null,
    approvedAt: null,
    settledAt: null,
    settlementRef: null,
    insurer: "Leading Insurance Co.",
    branch: "Apapa",
    documents: 3,
    notes: "Accident at Apapa junction. Third party involved.",
    relatedRefs: [
      { type: "Asset", ref: "TR-098" },
      { type: "Incident", ref: "INC-771" },
    ],
  },
  {
    id: "CLM-002",
    reference: "CLM-2026-002",
    type: "Damage",
    policyRef: "POL-2026-001",
    amount: 45000,
    currency: "NGN",
    status: "Approved",
    description: "Cargo damage - Delivery KG-8491",
    incidentDate: "2026-08-10",
    submittedAt: "2026-08-12",
    updatedAt: "2026-08-18",
    createdBy: "F. Daniel",
    assignedTo: "O. Adebayo",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-18",
    settledAt: null,
    settlementRef: null,
    insurer: "Leading Insurance Co.",
    branch: "Ikeja",
    documents: 2,
    notes: "Cargo damaged during delivery. Photos attached.",
    relatedRefs: [
      { type: "Trip", ref: "KG-8491" },
      { type: "Order", ref: "OR-129" },
    ],
  },
  {
    id: "CLM-003",
    reference: "CLM-2026-003",
    type: "Theft",
    policyRef: "POL-2026-002",
    amount: 180000,
    currency: "NGN",
    status: "Submitted",
    description: "Theft of parts from warehouse",
    incidentDate: "2026-08-05",
    submittedAt: "2026-08-07",
    updatedAt: "2026-08-07",
    createdBy: "A. Yusuf",
    assignedTo: null,
    approvedBy: null,
    approvedAt: null,
    settledAt: null,
    settlementRef: null,
    insurer: "SecureGuard Insurance",
    branch: "Ikeja",
    documents: 1,
    notes: "Parts stolen from storage zone B. Police report filed.",
    relatedRefs: [
      { type: "Warehouse", ref: "WH-001" },
    ],
  },
  {
    id: "CLM-004",
    reference: "CLM-2026-004",
    type: "Liability",
    policyRef: "POL-2026-003",
    amount: 150000,
    currency: "NGN",
    status: "Settled",
    description: "Third party liability claim",
    incidentDate: "2026-07-20",
    submittedAt: "2026-07-22",
    updatedAt: "2026-08-14",
    createdBy: "F. Daniel",
    assignedTo: "O. Adebayo",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-10",
    settledAt: "2026-08-14",
    settlementRef: "SET-2026-001",
    insurer: "Leading Insurance Co.",
    branch: "Lekki",
    documents: 4,
    notes: "Settled with third party. Full payment made.",
    relatedRefs: [
      { type: "Asset", ref: "VN-041" },
    ],
  },
  {
    id: "CLM-005",
    reference: "CLM-2026-005",
    type: "Loss",
    policyRef: null,
    amount: 0,
    currency: "NGN",
    status: "Draft",
    description: "Lost shipment - Private order",
    incidentDate: "2026-08-18",
    submittedAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "A. Yusuf",
    assignedTo: null,
    approvedBy: null,
    approvedAt: null,
    settledAt: null,
    settlementRef: null,
    insurer: null,
    branch: "Ikeja",
    documents: 0,
    notes: "Shipment tracking lost at transit point.",
    relatedRefs: [
      { type: "Shipment", ref: "SH-8821" },
    ],
  },
  {
    id: "CLM-006",
    reference: "CLM-2026-006",
    type: "Damage",
    policyRef: "POL-2026-001",
    amount: 120000,
    currency: "NGN",
    status: "Rejected",
    description: "Vehicle damage - Van VN-029",
    incidentDate: "2026-07-28",
    submittedAt: "2026-07-30",
    updatedAt: "2026-08-05",
    createdBy: "O. Adebayo",
    assignedTo: "F. Daniel",
    approvedBy: "F. Daniel",
    approvedAt: "2026-08-05",
    settledAt: null,
    settlementRef: null,
    insurer: "Leading Insurance Co.",
    branch: "Yaba",
    documents: 2,
    notes: "Rejected due to policy exclusion.",
    relatedRefs: [
      { type: "Asset", ref: "VN-029" },
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
  Submitted: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Send className="h-3.5 w-3.5" />,
  },
  "Under Review": {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Approved: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Settled: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <HandCoins className="h-3.5 w-3.5" />,
  },
  Reconciling: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Incident: { bg: "bg-red-bg", text: "text-red", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Damage: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Loss: { bg: "bg-purple-bg", text: "text-purple", icon: <Info className="h-3.5 w-3.5" /> },
  Liability: { bg: "bg-blue-bg", text: "text-blue", icon: <Shield className="h-3.5 w-3.5" /> },
  Theft: { bg: "bg-red-bg", text: "text-red", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Draft", "Submitted", "Under Review", "Approved", "Rejected", "Settled", "Reconciling"];
const TYPE_FILTERS = ["All", "Incident", "Damage", "Loss", "Liability", "Theft"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ClaimsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Claim[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(CLAIMS);
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
      item.insurer?.toLowerCase().includes(search.toLowerCase()) ||
      item.type.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesType && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading claims...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedType !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedType("All");
    setSelectedBranch("All branches");
  };

  const goToClaimDetail = (claimId: string) => {
    router.push(`/finance/claims/${claimId}`);
  };

  const goToCreateClaim = () => {
    router.push("/finance/claims/create");
  };

  // Stats
  const totalAmount = data.reduce((acc, item) => acc + item.amount, 0);
  const pendingCount = data.filter((item) => item.status === "Submitted" || item.status === "Under Review").length;
  const approvedCount = data.filter((item) => item.status === "Approved").length;
  const settledCount = data.filter((item) => item.status === "Settled").length;
  const rejectedCount = data.filter((item) => item.status === "Rejected").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Claims
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Incident/policy/reference, amount, documents, insurer/settlement reference and state.
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
            onClick={goToCreateClaim}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            New Claim
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Claims</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Settled</div>
          <div className="text-lg font-bold text-green">{settledCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, description, or insurer..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Submitted", "Under Review", "Approved", "Rejected", "Settled", "Reconciling"];
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
            const types = ["All", "Incident", "Damage", "Loss", "Liability", "Theft"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedType}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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

      {/* ===== CLAIMS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No claims found</h3>
          <p className="mt-1 text-sm text-ink-500">No claims match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Description</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Updated</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Incident;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.amount);

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToClaimDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.insurer || "—"}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">{item.description}</div>
                        <div className="text-[11px] text-ink-500">{item.policyRef || "No policy"}</div>
                      </td>
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
          <FileCheck className="h-4 w-4" />
          FIN-15 · Claims
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} claims shown
        </span>
      </div>
    </div>
  );
}