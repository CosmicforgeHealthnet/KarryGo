// src/app/(dashboard)/crm/slas/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Award,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  Eye,
  ArrowRight,
  TrendingUp,
  TrendingDown,
  Users,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
  Plus,
  Building2,
  User,
  Calendar,
  Target,
  Gauge,
  Filter,
} from "lucide-react";

type SLA = {
  id: string;
  reference: string;
  name: string;
  customer: string;
  customerId: string;
  customerInitials: string;
  contractRef: string | null;
  version: number;
  status: "Draft" | "Active" | "Expired" | "Archived";
  metric: string;
  target: number;
  unit: string;
  direction: "Higher" | "Lower";
  measurementWindow: string;
  effectiveFrom: string;
  effectiveTo: string | null;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  branch: string;
  notes: string | null;
};

// Mock data
const SLAS: SLA[] = [
  {
    id: "SLA-001",
    reference: "SLA-2026-001",
    name: "Medical Delivery On-Time Rate",
    customer: "Charitos Hospital",
    customerId: "CUST-001",
    customerInitials: "CH",
    contractRef: "CT-2026-001",
    version: 1,
    status: "Active",
    metric: "On-Time Delivery Rate",
    target: 98,
    unit: "%",
    direction: "Higher",
    measurementWindow: "Monthly",
    effectiveFrom: "2026-08-01",
    effectiveTo: "2027-07-31",
    createdAt: "2026-07-25",
    updatedAt: "2026-08-01",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-30",
    branch: "Ikeja",
    notes: "98% of medical deliveries must be completed on time.",
  },
  {
    id: "SLA-002",
    reference: "SLA-2026-002",
    name: "Cold Chain Temperature Compliance",
    customer: "Charitos Hospital",
    customerId: "CUST-001",
    customerInitials: "CH",
    contractRef: "CT-2026-001",
    version: 1,
    status: "Active",
    metric: "Temperature Compliance",
    target: 99.5,
    unit: "%",
    direction: "Higher",
    measurementWindow: "Daily",
    effectiveFrom: "2026-08-01",
    effectiveTo: "2027-07-31",
    createdAt: "2026-07-25",
    updatedAt: "2026-08-01",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-30",
    branch: "Ikeja",
    notes: "Cold chain monitoring required for medical supplies.",
  },
  {
    id: "SLA-003",
    reference: "SLA-2026-003",
    name: "Cold Chain Logistics SLA",
    customer: "Greenline Foods",
    customerId: "CUST-003",
    customerInitials: "GF",
    contractRef: "CT-2026-003",
    version: 2,
    status: "Active",
    metric: "Temperature Deviation",
    target: 1,
    unit: "°C",
    direction: "Lower",
    measurementWindow: "Hourly",
    effectiveFrom: "2026-07-01",
    effectiveTo: "2026-12-31",
    createdAt: "2026-06-15",
    updatedAt: "2026-07-01",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-06-25",
    branch: "Ikeja",
    notes: "Version 2 updated temperature thresholds.",
  },
  {
    id: "SLA-004",
    reference: "SLA-2026-004",
    name: "Construction Logistics SLA",
    customer: "Solaris Construction",
    customerId: "CUST-004",
    customerInitials: "SC",
    contractRef: "CT-2026-004",
    version: 1,
    status: "Expired",
    metric: "Delivery Accuracy",
    target: 95,
    unit: "%",
    direction: "Higher",
    measurementWindow: "Weekly",
    effectiveFrom: "2026-05-01",
    effectiveTo: "2026-08-31",
    createdAt: "2026-04-15",
    updatedAt: "2026-05-01",
    createdBy: "M. Bello",
    approvedBy: "F. Daniel",
    approvedAt: "2026-04-20",
    branch: "Lekki",
    notes: "Expired with contract termination.",
  },
  {
    id: "SLA-005",
    reference: "SLA-2026-005",
    name: "Warehouse & Distribution SLA",
    customer: "Eze & Partners",
    customerId: "CUST-012",
    customerInitials: "EP",
    contractRef: "CT-2026-005",
    version: 1,
    status: "Active",
    metric: "Order Fulfillment Rate",
    target: 99,
    unit: "%",
    direction: "Higher",
    measurementWindow: "Daily",
    effectiveFrom: "2026-09-01",
    effectiveTo: "2027-08-31",
    createdAt: "2026-08-10",
    updatedAt: "2026-08-15",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-08-15",
    branch: "Ikeja",
    notes: "99% fulfillment rate required for all warehouse orders.",
  },
  {
    id: "SLA-006",
    reference: "SLA-2026-006",
    name: "Medical Logistics SLA",
    customer: "Lagos Medical Center",
    customerId: "CUST-009",
    customerInitials: "LM",
    contractRef: null,
    version: 1,
    status: "Draft",
    metric: "Emergency Response Time",
    target: 30,
    unit: "min",
    direction: "Lower",
    measurementWindow: "Per Emergency",
    effectiveFrom: "2026-10-01",
    effectiveTo: "2027-09-30",
    createdAt: "2026-08-22",
    updatedAt: "2026-08-22",
    createdBy: "F. Daniel",
    approvedBy: null,
    approvedAt: null,
    branch: "Ikeja",
    notes: "Draft SLA for new medical contract.",
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
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
  Archived: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

type DirectionStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const DIRECTION_STYLES: Record<string, DirectionStyle> = {
  Higher: { bg: "bg-green-bg", text: "text-green", icon: <TrendingUp className="h-3.5 w-3.5" /> },
  Lower: { bg: "bg-red-bg", text: "text-red", icon: <TrendingDown className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Draft", "Active", "Expired", "Archived"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function SLAPage() {
  const router = useRouter();
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedBranch, setSelectedBranch] = useState<string>("All branches");
  const [showFilters, setShowFilters] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<SLA[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(SLAS);
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
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.customer.toLowerCase().includes(search.toLowerCase()) ||
      item.metric.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading SLAs...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const goToSLADetail = (slaId: string) => {
    router.push(`/crm/slas/${slaId}`);
  };

  const goToCreateSLA = () => {
    router.push("/crm/slas/create");
  };

  // Stats
  const totalSLAs = data.length;
  const activeCount = data.filter((d) => d.status === "Active").length;
  const draftCount = data.filter((d) => d.status === "Draft").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Service-level Agreements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Customer/contract, metric, target, direction, measurement window, status and effective version.
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
            onClick={goToCreateSLA}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create SLA
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total SLAs</div>
          <div className="text-lg font-bold text-ink-950">{totalSLAs}</div>
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">With Contract</div>
          <div className="text-lg font-bold text-ink-950">
            {data.filter((d) => d.contractRef).length}
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
            placeholder="Search by name, customer, or metric..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Active", "Expired", "Archived"];
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

      {/* ===== SLA CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Award className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No SLAs found</h3>
          <p className="mt-1 text-sm text-ink-500">No SLAs match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
            const directionStyle = DIRECTION_STYLES[item.direction] || DIRECTION_STYLES.Higher;

            return (
              <div
                key={item.id}
                onClick={() => goToSLADetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  item.status === "Active"
                    ? "border-green/20 bg-white hover:border-green/40"
                    : item.status === "Draft"
                    ? "border-blue/20 bg-blue-bg/10 hover:border-blue/40"
                    : item.status === "Expired"
                    ? "border-ink/20 bg-surface-2/10 hover:border-ink/40"
                    : "border-line bg-white/60 hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  item.status === "Active" ? "bg-green-500/5" :
                  item.status === "Draft" ? "bg-blue-500/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${
                        item.status === "Active" ? "bg-green-bg text-green" :
                        item.status === "Draft" ? "bg-blue-bg text-blue" :
                        "bg-surface-2 text-ink-400"
                      }`}
                    >
                      <Award className="h-5 w-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.name}
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
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-2">
                  <div className="text-sm font-medium text-ink-950">{item.customer}</div>
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
                    {item.contractRef && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span className="font-mono text-[11px] text-ink-500">{item.contractRef}</span>
                      </>
                    )}
                  </div>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Target className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-medium text-ink-950">
                      {item.target}{item.unit}
                    </span>
                    <span className={`flex items-center gap-0.5 ${directionStyle.bg} ${directionStyle.text} rounded-full px-1.5 py-0.5 text-[9px] font-medium`}>
                      {directionStyle.icon}
                      {item.direction}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.measurementWindow}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500 col-span-2">
                    <Gauge className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.metric}</span>
                  </div>
                </div>

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <CalendarDays className="h-3.5 w-3.5" />
                    {item.effectiveFrom} → {item.effectiveTo || "Ongoing"}
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
          <Award className="h-4 w-4" />
          CRM-15 · Service-level Agreements
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} SLAs shown
        </span>
      </div>
    </div>
  );
}