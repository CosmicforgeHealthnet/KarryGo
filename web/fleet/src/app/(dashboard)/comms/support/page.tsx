// src/app/(dashboard)/comms/support/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Tag,
  Info,
  Download,
  MoreHorizontal,
  User,
  Shield,
  AlertOctagon,
  XCircle,
  HelpCircle,
} from "lucide-react";

// ==================== TYPES ====================

type SupportCase = {
  id: string;
  reference: string;
  title: string;
  description: string;
  category: "Technical" | "Operational" | "Billing" | "Provider" | "Customer" | "Fleet" | "Compliance" | "Other";
  priority: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "InProgress" | "Escalated" | "Resolved" | "Closed" | "AwaitingResponse";
  source: "Email" | "Phone" | "InApp" | "API" | "Internal";
  organizationId: string;
  organizationName: string;
  branch: string;
  relatedTo: {
    type: "Provider" | "Customer" | "Trip" | "Order" | "Asset" | "Incident" | "Other";
    reference: string | null;
    id: string | null;
  } | null;
  assignedTo: string | null;
  assignedAt: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  messages: number;
  attachments: number;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_SUPPORT_CASES: SupportCase[] = [
  {
    id: "SUP-001",
    reference: "SUP-2026-001",
    title: "Unable to create new dispatch plan",
    description: "Dispatch plan creation is failing with an error. Users cannot schedule new routes.",
    category: "Technical",
    priority: "High",
    status: "InProgress",
    source: "InApp",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Ikeja",
    relatedTo: {
      type: "Other",
      reference: null,
      id: null,
    },
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-22T09:00:00Z",
    createdBy: "O. Adebayo",
    createdAt: "2026-08-22T08:30:00Z",
    updatedAt: "2026-08-22T10:15:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 3,
    attachments: 1,
    tags: ["Technical", "Dispatch", "Bug"],
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  {
    id: "SUP-002",
    reference: "SUP-2026-002",
    title: "Provider payment delay - P-044",
    description: "Provider P-044 has not received payment for completed trips. Finance team investigating.",
    category: "Billing",
    priority: "Critical",
    status: "Escalated",
    source: "Email",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Ikeja",
    relatedTo: {
      type: "Provider",
      reference: "P-044",
      id: "P-044",
    },
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-21T16:00:00Z",
    createdBy: "P. Okafor",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 5,
    attachments: 2,
    tags: ["Billing", "Provider", "Escalated"],
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  {
    id: "SUP-003",
    reference: "SUP-2026-003",
    title: "Customer complaint - Delivery damage",
    description: "Customer reports damaged goods from delivery KG-8491. Photos provided. Investigation required.",
    category: "Customer",
    priority: "High",
    status: "Open",
    source: "Phone",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Lekki",
    relatedTo: {
      type: "Trip",
      reference: "KG-8491",
      id: "KG-8491",
    },
    assignedTo: null,
    assignedAt: null,
    createdBy: "C. Adeyemi",
    createdAt: "2026-08-22T11:00:00Z",
    updatedAt: "2026-08-22T11:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 2,
    attachments: 3,
    tags: ["Customer", "Damage", "Complaint"],
    sourceFreshness: "2026-08-22T11:00:00Z",
  },
  {
    id: "SUP-004",
    reference: "SUP-2026-004",
    title: "Fleet maintenance request - TR-112",
    description: "TR-112 requires urgent maintenance. Brake issue reported. Work order requested.",
    category: "Fleet",
    priority: "High",
    status: "AwaitingResponse",
    source: "InApp",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Apapa",
    relatedTo: {
      type: "Asset",
      reference: "TR-112",
      id: "TR-112",
    },
    assignedTo: "M. Bello",
    assignedAt: "2026-08-20T09:00:00Z",
    createdBy: "K. Ahmed",
    createdAt: "2026-08-19T16:20:00Z",
    updatedAt: "2026-08-21T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 4,
    attachments: 2,
    tags: ["Fleet", "Maintenance", "Vehicle"],
    sourceFreshness: "2026-08-21T14:00:00Z",
  },
  {
    id: "SUP-005",
    reference: "SUP-2026-005",
    title: "API integration issue - Webhook delivery",
    description: "Webhook deliveries failing for order updates. Destination endpoint returning 500 errors.",
    category: "Technical",
    priority: "Medium",
    status: "InProgress",
    source: "API",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "All branches",
    relatedTo: {
      type: "Other",
      reference: null,
      id: null,
    },
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-21T10:00:00Z",
    createdBy: "System",
    createdAt: "2026-08-21T09:30:00Z",
    updatedAt: "2026-08-22T07:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 2,
    attachments: 0,
    tags: ["API", "Integration", "Webhook"],
    sourceFreshness: "2026-08-22T07:00:00Z",
  },
  {
    id: "SUP-006",
    reference: "SUP-2026-006",
    title: "Provider account suspension request",
    description: "Request to suspend provider account P-044 due to non-compliance.",
    category: "Provider",
    priority: "Medium",
    status: "Resolved",
    source: "Email",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Ikeja",
    relatedTo: {
      type: "Provider",
      reference: "P-044",
      id: "P-044",
    },
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-18T14:00:00Z",
    createdBy: "O. Adebayo",
    createdAt: "2026-08-18T13:00:00Z",
    updatedAt: "2026-08-20T17:00:00Z",
    resolvedAt: "2026-08-20T17:00:00Z",
    resolvedBy: "F. Daniel",
    messages: 6,
    attachments: 1,
    tags: ["Provider", "Suspension", "Resolved"],
    sourceFreshness: "2026-08-20T17:00:00Z",
  },
  {
    id: "SUP-007",
    reference: "SUP-2026-007",
    title: "Warehouse system access issue",
    description: "Warehouse staff unable to access inventory system. Role permissions investigation.",
    category: "Technical",
    priority: "High",
    status: "Open",
    source: "InApp",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Yaba",
    relatedTo: {
      type: "Other",
      reference: null,
      id: null,
    },
    assignedTo: null,
    assignedAt: null,
    createdBy: "J. Adekunle",
    createdAt: "2026-08-22T12:00:00Z",
    updatedAt: "2026-08-22T12:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    messages: 1,
    attachments: 0,
    tags: ["Access", "Warehouse", "Permissions"],
    sourceFreshness: "2026-08-22T12:00:00Z",
  },
  {
    id: "SUP-008",
    reference: "SUP-2026-008",
    title: "SLA compliance report request",
    description: "Customer requesting SLA compliance report for Q3 2026.",
    category: "Customer",
    priority: "Low",
    status: "Closed",
    source: "Email",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Lekki",
    relatedTo: {
      type: "Customer",
      reference: "C-002",
      id: "C-002",
    },
    assignedTo: "O. Adebayo",
    assignedAt: "2026-08-19T09:00:00Z",
    createdBy: "C. Adeyemi",
    createdAt: "2026-08-19T08:30:00Z",
    updatedAt: "2026-08-21T16:00:00Z",
    resolvedAt: "2026-08-21T16:00:00Z",
    resolvedBy: "O. Adebayo",
    messages: 4,
    attachments: 3,
    tags: ["SLA", "Customer", "Report"],
    sourceFreshness: "2026-08-21T16:00:00Z",
  },
];

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Technical: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Operational: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Billing: { bg: "bg-green-bg", text: "text-green", icon: <Tag className="h-3.5 w-3.5" /> },
  Provider: { bg: "bg-purple-bg", text: "text-purple", icon: <User className="h-3.5 w-3.5" /> },
  Customer: { bg: "bg-red-bg", text: "text-red", icon: <User className="h-3.5 w-3.5" /> },
  Fleet: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Compliance: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-3.5 w-3.5" /> },
  Other: { bg: "bg-surface-2", text: "text-ink-500", icon: <MoreHorizontal className="h-3.5 w-3.5" /> },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-3.5 w-3.5" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  InProgress: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Escalated: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  AwaitingResponse: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <XCircle className="h-3.5 w-3.5" /> },
};

const SOURCE_STYLES: Record<string, { bg: string; text: string }> = {
  Email: { bg: "bg-blue-bg", text: "text-blue" },
  Phone: { bg: "bg-green-bg", text: "text-green" },
  InApp: { bg: "bg-purple-bg", text: "text-purple" },
  API: { bg: "bg-amber-bg", text: "text-amber" },
  Internal: { bg: "bg-surface-2", text: "text-ink-500" },
};

const CATEGORY_FILTERS = ["All", "Technical", "Operational", "Billing", "Provider", "Customer", "Fleet", "Compliance", "Other"];
const PRIORITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "InProgress", "Escalated", "AwaitingResponse", "Resolved", "Closed"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function SupportCasesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SupportCase[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedPriority, setSelectedPriority] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_SUPPORT_CASES);
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
      item.organizationName.toLowerCase().includes(search.toLowerCase()) ||
      (item.assignedTo && item.assignedTo.toLowerCase().includes(search.toLowerCase()));

    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;
    const matchesPriority = selectedPriority === "All" || item.priority === selectedPriority;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesCategory && matchesPriority && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading support cases...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedCategory !== "All" ||
    selectedPriority !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All");
    setSelectedPriority("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedCategory !== "All" ? 1 : 0) +
    (selectedPriority !== "All" ? 1 : 0) +
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
  const totalOpen = data.filter((d) => d.status === "Open" || d.status === "InProgress").length;
  const totalCritical = data.filter((d) => d.priority === "Critical").length;
  const totalHigh = data.filter((d) => d.priority === "High").length;
  const totalResolved = data.filter((d) => d.status === "Resolved" || d.status === "Closed").length;
  const totalEscalated = data.filter((d) => d.status === "Escalated").length;

  const goToCaseDetail = (id: string) => {
    router.push(`/comms/support/${id}`);
  };

  const goToCreateCase = () => {
    router.push("/comms/support/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Platform Support Cases
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            KarryGo support cases related to your organization, providers, customers, and trips.
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
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-5">
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Escalated</div>
          <div className="text-lg font-bold text-purple">{totalEscalated}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resolved</div>
          <div className="text-lg font-bold text-green">{totalResolved}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, organization..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const categories = ["All", "Technical", "Operational", "Billing", "Provider", "Customer", "Fleet", "Compliance", "Other"];
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
            const priorities = ["All", "Critical", "High", "Medium", "Low"];
            const currentIndex = priorities.indexOf(selectedPriority);
            const nextIndex = (currentIndex + 1) % priorities.length;
            setSelectedPriority(priorities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedPriority}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Open", "InProgress", "Escalated", "AwaitingResponse", "Resolved", "Closed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <HelpCircle className="h-4 w-4 text-ink-400" />
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
            <HelpCircle className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No support cases found</h3>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Priority</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Assigned To</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const categoryStyle = CATEGORY_STYLES[item.category] || CATEGORY_STYLES.Other;
                  const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Medium;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Open;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToCaseDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          <span className="text-[9px] text-ink-400">{item.organizationName}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                          {categoryStyle.icon}
                          {item.category}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
                          {priorityStyle.icon}
                          {item.priority}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.assignedTo || "Unassigned"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
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
          <HelpCircle className="h-4 w-4" />
          COM-04 · Platform Support Cases
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} cases shown
        </span>
      </div>
    </div>
  );
}