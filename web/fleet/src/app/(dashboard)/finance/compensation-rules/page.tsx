// src/app/(dashboard)/finance/compensation-rules/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Scale,
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
  Briefcase,
  MapPin,
} from "lucide-react";

type CompensationRule = {
  id: string;
  reference: string;
  name: string;
  description: string;
  serviceType: "Dispatch" | "Hauling" | "Warehouse" | "All";
  relationship: "Provider" | "Employee" | "Contractor";
  rateType: "Fixed" | "PerTrip" | "PerHour" | "PerKilometer" | "Percentage";
  rateValue: number;
  currency: string;
  effectiveFrom: string;
  effectiveTo: string | null;
  status: "Active" | "Inactive" | "Draft" | "Archived";
  appliesTo: string[];
  branch: string;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  version: number;
};

// Mock data
const COMPENSATION_RULES: CompensationRule[] = [
  {
    id: "CR-001",
    reference: "CR-2026-001",
    name: "Dispatch Rider - Per Trip",
    description: "Standard per-trip compensation for dispatch riders.",
    serviceType: "Dispatch",
    relationship: "Provider",
    rateType: "PerTrip",
    rateValue: 4000,
    currency: "NGN",
    effectiveFrom: "2026-01-01",
    effectiveTo: null,
    status: "Active",
    appliesTo: ["Dispatch Rider", "Dispatch Supervisor"],
    branch: "All branches",
    createdAt: "2026-01-01",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-01-05",
    version: 3,
  },
  {
    id: "CR-002",
    reference: "CR-2026-002",
    name: "Hauling Driver - Per Kilometer",
    description: "Per-kilometer compensation for hauling drivers.",
    serviceType: "Hauling",
    relationship: "Provider",
    rateType: "PerKilometer",
    rateValue: 180,
    currency: "NGN",
    effectiveFrom: "2026-01-01",
    effectiveTo: null,
    status: "Active",
    appliesTo: ["Hauling Driver"],
    branch: "All branches",
    createdAt: "2026-01-01",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-01-05",
    version: 2,
  },
  {
    id: "CR-003",
    reference: "CR-2026-003",
    name: "Fleet Manager - Monthly Salary",
    description: "Fixed monthly salary for fleet managers.",
    serviceType: "All",
    relationship: "Employee",
    rateType: "Fixed",
    rateValue: 250000,
    currency: "NGN",
    effectiveFrom: "2026-01-01",
    effectiveTo: null,
    status: "Active",
    appliesTo: ["Fleet Manager"],
    branch: "Ikeja",
    createdAt: "2026-01-01",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-01-05",
    version: 1,
  },
  {
    id: "CR-004",
    reference: "CR-2026-004",
    name: "Warehouse Supervisor - Hourly",
    description: "Hourly compensation for warehouse supervisors.",
    serviceType: "Warehouse",
    relationship: "Employee",
    rateType: "PerHour",
    rateValue: 2500,
    currency: "NGN",
    effectiveFrom: "2026-02-01",
    effectiveTo: null,
    status: "Active",
    appliesTo: ["Warehouse Supervisor"],
    branch: "All branches",
    createdAt: "2026-02-01",
    updatedAt: "2026-08-20",
    createdBy: "O. Adebayo",
    approvedBy: "F. Daniel",
    approvedAt: "2026-02-05",
    version: 1,
  },
  {
    id: "CR-005",
    reference: "CR-2026-005",
    name: "Dispatch Rider - Weekend Bonus",
    description: "Bonus rate for weekend dispatch shifts.",
    serviceType: "Dispatch",
    relationship: "Provider",
    rateType: "Percentage",
    rateValue: 20,
    currency: "NGN",
    effectiveFrom: "2026-07-01",
    effectiveTo: null,
    status: "Active",
    appliesTo: ["Dispatch Rider"],
    branch: "All branches",
    createdAt: "2026-07-01",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    approvedBy: "O. Adebayo",
    approvedAt: "2026-07-05",
    version: 1,
  },
  {
    id: "CR-006",
    reference: "CR-2026-006",
    name: "Hauling Driver - Overtime",
    description: "Overtime rate for hauling drivers.",
    serviceType: "Hauling",
    relationship: "Provider",
    rateType: "PerHour",
    rateValue: 3500,
    currency: "NGN",
    effectiveFrom: "2026-03-01",
    effectiveTo: "2026-12-31",
    status: "Active",
    appliesTo: ["Hauling Driver"],
    branch: "Apapa",
    createdAt: "2026-03-01",
    updatedAt: "2026-08-20",
    createdBy: "M. Bello",
    approvedBy: "F. Daniel",
    approvedAt: "2026-03-05",
    version: 1,
  },
  {
    id: "CR-007",
    reference: "CR-2026-007",
    name: "Dispatch Rider - Trial Rate",
    description: "Trial rate for new dispatch riders.",
    serviceType: "Dispatch",
    relationship: "Provider",
    rateType: "PerTrip",
    rateValue: 3000,
    currency: "NGN",
    effectiveFrom: "2026-08-01",
    effectiveTo: "2026-10-31",
    status: "Draft",
    appliesTo: ["Dispatch Rider"],
    branch: "Lekki",
    createdAt: "2026-08-20",
    updatedAt: "2026-08-20",
    createdBy: "A. Yusuf",
    approvedBy: null,
    approvedAt: null,
    version: 1,
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Inactive: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Archived: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Archive className="h-3.5 w-3.5" />,
  },
};

const SERVICE_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Dispatch: { bg: "bg-blue-bg", text: "text-blue" },
  Hauling: { bg: "bg-amber-bg", text: "text-amber" },
  Warehouse: { bg: "bg-green-bg", text: "text-green" },
  All: { bg: "bg-purple-bg", text: "text-purple" },
};

const RATE_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Fixed: { bg: "bg-blue-bg", text: "text-blue" },
  PerTrip: { bg: "bg-green-bg", text: "text-green" },
  PerHour: { bg: "bg-amber-bg", text: "text-amber" },
  PerKilometer: { bg: "bg-purple-bg", text: "text-purple" },
  Percentage: { bg: "bg-pink-bg", text: "text-pink" },
};

const STATUS_FILTERS = ["All", "Active", "Inactive", "Draft", "Archived"];
const SERVICE_TYPE_FILTERS = ["All", "Dispatch", "Hauling", "Warehouse", "All"];
const RATE_TYPE_FILTERS = ["All", "Fixed", "PerTrip", "PerHour", "PerKilometer", "Percentage"];

export default function CompensationRulesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedServiceType, setSelectedServiceType] = useState("All");
  const [selectedRateType, setSelectedRateType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CompensationRule[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(COMPENSATION_RULES);
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
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.serviceType.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesServiceType = selectedServiceType === "All" || item.serviceType === selectedServiceType;
    const matchesRateType = selectedRateType === "All" || item.rateType === selectedRateType;

    return matchesSearch && matchesStatus && matchesServiceType && matchesRateType;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading compensation rules...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedServiceType !== "All" || selectedRateType !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedServiceType("All");
    setSelectedRateType("All");
  };

  const goToRuleDetail = (ruleId: string) => {
    router.push(`/finance/compensation-rules/${ruleId}`);
  };

  const goToCreateRule = () => {
    router.push("/finance/compensation-rules/create");
  };

  // Stats
  const totalActive = data.filter((item) => item.status === "Active").length;
  const totalDraft = data.filter((item) => item.status === "Draft").length;
  const totalInactive = data.filter((item) => item.status === "Inactive").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Compensation Rules
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Provider/staff compensation policy versions, applicable service/relationship and active period.
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
            onClick={goToCreateRule}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Rule
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Rules</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Draft</div>
          <div className="text-lg font-bold text-blue">{totalDraft}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Inactive</div>
          <div className="text-lg font-bold text-ink-500">{totalInactive}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, reference, or service type..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Inactive", "Draft", "Archived"];
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
            const serviceTypes = ["All", "Dispatch", "Hauling", "Warehouse", "All"];
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
            const rateTypes = ["All", "Fixed", "PerTrip", "PerHour", "PerKilometer", "Percentage"];
            const currentIndex = rateTypes.indexOf(selectedRateType);
            const nextIndex = (currentIndex + 1) % rateTypes.length;
            setSelectedRateType(rateTypes[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Percent className="h-4 w-4 text-ink-400" />
          {selectedRateType}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedServiceType !== "All" ? 1 : 0) + (selectedRateType !== "All" ? 1 : 0)}
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

      {/* ===== COMPENSATION RULES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Scale className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No compensation rules found</h3>
          <p className="mt-1 text-sm text-ink-500">No compensation rules match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Rule</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Service</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Rate</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Value</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Version</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const serviceStyle = SERVICE_TYPE_STYLES[item.serviceType] || SERVICE_TYPE_STYLES.All;
                  const rateStyle = RATE_TYPE_STYLES[item.rateType] || RATE_TYPE_STYLES.Fixed;
                  const formattedRate = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.rateValue);

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRuleDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950">{item.name}</div>
                        <div className="text-[11px] text-ink-500">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${serviceStyle.bg} ${serviceStyle.text}`}>
                          {item.serviceType}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${rateStyle.bg} ${rateStyle.text}`}>
                          {item.rateType}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-semibold text-ink-950">
                        {item.rateType === "Percentage" ? `${item.rateValue}%` : formattedRate}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
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
          <Scale className="h-4 w-4" />
          FIN-09 · Compensation Rules
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} rules shown
        </span>
      </div>
    </div>
  );
}