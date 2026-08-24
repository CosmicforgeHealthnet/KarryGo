// src/app/(dashboard)/comms/quality/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ClipboardCheck,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Users,
  Building2,
  Tag,
  Calendar,
  Info,
  ListChecks,
  Download,
  Printer,
  Edit3,
  Trash2,
  Check,
  MoreHorizontal,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  MessageCircle,
  UserCheck,
  UserX,
  Clock as ClockIcon,
  AlertOctagon,
  Shield,
  Flag,
  UserCog,
  CheckCircle,
  XCircle,
  Clock as ClockIcon2,
  FileSearch,
  ClipboardList,
  BookOpen,
} from "lucide-react";

// ==================== TYPES ====================

type QualityCase = {
  id: string;
  reference: string;
  title: string;
  description: string;
  category: "Complaint" | "NonConformance" | "ProcessFailure" | "ServiceFailure" | "CustomerExperience" | "Safety" | "Other";
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "Investigating" | "UnderReview" | "Resolved" | "Closed";
  owner: string | null;
  priority: "Urgent" | "High" | "Normal" | "Low";
  rootCauseStatus: "Unknown" | "Identified" | "Confirmed" | "Resolved";
  rootCause: string | null;
  relatedOperation: {
    type: "Trip" | "Order" | "Customer" | "Provider" | "Asset" | "Other";
    reference: string | null;
    id: string | null;
  } | null;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  messages: number;
  attachments: number;
  tags: string[];
  branch: string;
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_QUALITY_CASES: QualityCase[] = [
  {
    id: "QC-001",
    reference: "QC-2026-001",
    title: "Delivery quality complaint - Damaged goods",
    description: "Customer reported damaged goods from delivery KG-8491. Investigation required to identify root cause.",
    category: "Complaint",
    severity: "High",
    status: "Investigating",
    owner: "F. Daniel",
    priority: "High",
    rootCauseStatus: "Identified",
    rootCause: "Improper packaging during loading",
    relatedOperation: {
      type: "Trip",
      reference: "KG-8491",
      id: "KG-8491",
    },
    createdAt: "2026-08-22T11:00:00Z",
    updatedAt: "2026-08-22T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 4,
    attachments: 3,
    tags: ["Complaint", "Damage", "Packaging"],
    branch: "Lekki",
    sourceFreshness: "2026-08-22T14:00:00Z",
  },
  {
    id: "QC-002",
    reference: "QC-2026-002",
    title: "Process failure - Provider onboarding",
    description: "Provider onboarding process failed to complete for P-044. Required documents not verified within SLA.",
    category: "ProcessFailure",
    severity: "High",
    status: "Open",
    owner: "O. Adebayo",
    priority: "High",
    rootCauseStatus: "Unknown",
    rootCause: null,
    relatedOperation: {
      type: "Provider",
      reference: "P-044",
      id: "P-044",
    },
    createdAt: "2026-08-21T15:00:00Z",
    updatedAt: "2026-08-22T09:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 3,
    attachments: 1,
    tags: ["Process", "Onboarding", "Verification"],
    branch: "Ikeja",
    sourceFreshness: "2026-08-22T09:00:00Z",
  },
  {
    id: "QC-003",
    reference: "QC-2026-003",
    title: "Customer experience issue - Delivery delay",
    description: "Customer complained about repeated delivery delays. SLA breached 3 times this month.",
    category: "CustomerExperience",
    severity: "High",
    status: "UnderReview",
    owner: "F. Daniel",
    priority: "Urgent",
    rootCauseStatus: "Confirmed",
    rootCause: "Provider capacity planning insufficient",
    relatedOperation: {
      type: "Customer",
      reference: "C-002",
      id: "C-002",
    },
    createdAt: "2026-08-20T13:00:00Z",
    updatedAt: "2026-08-22T10:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 6,
    attachments: 2,
    tags: ["Customer", "SLA", "Delay"],
    branch: "Lekki",
    sourceFreshness: "2026-08-22T10:00:00Z",
  },
  {
    id: "QC-004",
    reference: "QC-2026-004",
    title: "Safety non-conformance - Vehicle TR-112",
    description: "TR-112 operated with faulty brakes. Pre-trip inspection not properly completed. Critical safety breach.",
    category: "NonConformance",
    severity: "Critical",
    status: "Investigating",
    owner: "M. Bello",
    priority: "Urgent",
    rootCauseStatus: "Identified",
    rootCause: "Inspection procedure not followed",
    relatedOperation: {
      type: "Asset",
      reference: "TR-112",
      id: "TR-112",
    },
    createdAt: "2026-08-19T16:30:00Z",
    updatedAt: "2026-08-21T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 5,
    attachments: 3,
    tags: ["Safety", "NonConformance", "Vehicle"],
    branch: "Apapa",
    sourceFreshness: "2026-08-21T14:00:00Z",
  },
  {
    id: "QC-005",
    reference: "QC-2026-005",
    title: "Service failure - Incorrect delivery address",
    description: "Provider delivered to incorrect address due to system address error. Customer had to reschedule.",
    category: "ServiceFailure",
    severity: "Medium",
    status: "Resolved",
    owner: "F. Daniel",
    priority: "Normal",
    rootCauseStatus: "Resolved",
    rootCause: "System address validation bug fixed",
    relatedOperation: {
      type: "Trip",
      reference: "KG-8479",
      id: "KG-8479",
    },
    createdAt: "2026-08-18T10:00:00Z",
    updatedAt: "2026-08-20T16:00:00Z",
    resolvedAt: "2026-08-20T16:00:00Z",
    resolvedBy: "F. Daniel",
    messages: 4,
    attachments: 1,
    tags: ["Service", "Address", "Resolution"],
    branch: "Ikeja",
    sourceFreshness: "2026-08-20T16:00:00Z",
  },
  {
    id: "QC-006",
    reference: "QC-2026-006",
    title: "Quality review - Dispatch planning process",
    description: "Dispatch plan approval process has bottlenecks causing delays. Review needed.",
    category: "ProcessFailure",
    severity: "Medium",
    status: "Open",
    owner: "O. Adebayo",
    priority: "Normal",
    rootCauseStatus: "Unknown",
    rootCause: null,
    relatedOperation: null,
    createdAt: "2026-08-20T09:00:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 2,
    attachments: 0,
    tags: ["Process", "Dispatch", "Review"],
    branch: "All branches",
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  {
    id: "QC-007",
    reference: "QC-2026-007",
    title: "Complaint - Billing error for provider",
    description: "Provider P-071 reported incorrect payment amount for trip KG-8512. Discrepancy found.",
    category: "Complaint",
    severity: "Medium",
    status: "Closed",
    owner: "F. Daniel",
    priority: "High",
    rootCauseStatus: "Resolved",
    rootCause: "Manual entry error corrected",
    relatedOperation: {
      type: "Provider",
      reference: "P-071",
      id: "P-071",
    },
    createdAt: "2026-08-17T11:00:00Z",
    updatedAt: "2026-08-19T17:00:00Z",
    resolvedAt: "2026-08-19T17:00:00Z",
    resolvedBy: "F. Daniel",
    messages: 5,
    attachments: 2,
    tags: ["Billing", "Provider", "Closed"],
    branch: "Apapa",
    sourceFreshness: "2026-08-19T17:00:00Z",
  },
  {
    id: "QC-008",
    reference: "QC-2026-008",
    title: "Safety concern - Warehouse slip hazard",
    description: "Oil spill reported in warehouse zone C. Multiple near-misses. Safety protocol review required.",
    category: "Safety",
    severity: "Critical",
    status: "Investigating",
    owner: "F. Daniel",
    priority: "Urgent",
    rootCauseStatus: "Identified",
    rootCause: "Lack of spill containment procedures",
    relatedOperation: {
      type: "Other",
      reference: null,
      id: null,
    },
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T11:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 4,
    attachments: 2,
    tags: ["Safety", "Warehouse", "Spill"],
    branch: "Yaba",
    sourceFreshness: "2026-08-22T11:00:00Z",
  },
];

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Complaint: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  NonConformance: { bg: "bg-purple-bg", text: "text-purple", icon: <XCircle className="h-3.5 w-3.5" /> },
  ProcessFailure: { bg: "bg-amber-bg", text: "text-amber", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  ServiceFailure: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  CustomerExperience: { bg: "bg-green-bg", text: "text-green", icon: <User className="h-3.5 w-3.5" /> },
  Safety: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-3.5 w-3.5" /> },
  Other: { bg: "bg-surface-2", text: "text-ink-500", icon: <MoreHorizontal className="h-3.5 w-3.5" /> },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-3.5 w-3.5" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <FileSearch className="h-3.5 w-3.5" /> },
  UnderReview: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <ClipboardList className="h-3.5 w-3.5" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <XCircle className="h-3.5 w-3.5" /> },
};

const ROOT_CAUSE_STYLES: Record<string, { bg: string; text: string }> = {
  Unknown: { bg: "bg-surface-2", text: "text-ink-500" },
  Identified: { bg: "bg-amber-bg", text: "text-amber" },
  Confirmed: { bg: "bg-blue-bg", text: "text-blue" },
  Resolved: { bg: "bg-green-bg", text: "text-green" },
};

const CATEGORY_FILTERS = ["All", "Complaint", "NonConformance", "ProcessFailure", "ServiceFailure", "CustomerExperience", "Safety", "Other"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "Investigating", "UnderReview", "Resolved", "Closed"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function QualityCasesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<QualityCase[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_QUALITY_CASES);
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
      item.title.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      (item.owner && item.owner.toLowerCase().includes(search.toLowerCase()));

    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;
    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesCategory && matchesSeverity && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading quality cases...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedCategory !== "All" ||
    selectedSeverity !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All");
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedCategory !== "All" ? 1 : 0) +
    (selectedSeverity !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

  const formatTime = (date: string) => {
    const d = new Date(date);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  // Stats
  const totalOpen = data.filter((d) => d.status === "Open" || d.status === "Investigating").length;
  const totalCritical = data.filter((d) => d.severity === "Critical").length;
  const totalResolved = data.filter((d) => d.status === "Resolved" || d.status === "Closed").length;
  const totalRootCauseIdentified = data.filter((d) => d.rootCauseStatus === "Identified" || d.rootCauseStatus === "Confirmed" || d.rootCauseStatus === "Resolved").length;

  const goToCaseDetail = (id: string) => {
    router.push(`/comms/quality/${id}`);
  };

  const goToCreateCase = () => {
    router.push("/comms/quality/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Internal Quality Cases
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Complaints, non-conformances, process failures, and quality concerns. Kept distinct from platform support.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
          <button
            onClick={goToCreateCase}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Case
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Open</div>
          <div className="text-lg font-bold text-red">{totalOpen}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Critical</div>
          <div className="text-lg font-bold text-red">{totalCritical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Root Cause Found</div>
          <div className="text-lg font-bold text-green">{totalRootCauseIdentified}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, owner..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const categories = ["All", "Complaint", "NonConformance", "ProcessFailure", "ServiceFailure", "CustomerExperience", "Safety", "Other"];
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
            const severities = ["All", "Critical", "High", "Medium", "Low"];
            const currentIndex = severities.indexOf(selectedSeverity);
            const nextIndex = (currentIndex + 1) % severities.length;
            setSelectedSeverity(severities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedSeverity}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Open", "Investigating", "UnderReview", "Resolved", "Closed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <ClipboardCheck className="h-4 w-4 text-ink-400" />
          {selectedStatus}
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
              {filterCount}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex gap-1.5 flex-wrap">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedBranch === branch
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branch}
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

      {/* ===== CASES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <ClipboardCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No quality cases found</h3>
          <p className="mt-1 text-sm text-ink-500">No cases match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1100px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Case</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Category</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Root Cause</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Owner</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const categoryStyle = CATEGORY_STYLES[item.category] || CATEGORY_STYLES.Other;
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Open;
                  const rootCauseStyle = ROOT_CAUSE_STYLES[item.rootCauseStatus] || ROOT_CAUSE_STYLES.Unknown;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToCaseDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="font-mono text-[10px] text-ink-500 mt-0.5">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                          {categoryStyle.icon}
                          {item.category}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                          {severityStyle.icon}
                          {item.severity}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${rootCauseStyle.bg} ${rootCauseStyle.text}`}>
                          {item.rootCauseStatus}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.owner || "Unassigned"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
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
          <ClipboardCheck className="h-4 w-4" />
          COM-06 · Internal Quality Cases
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} cases shown
        </span>
      </div>
    </div>
  );
}