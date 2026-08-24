// src/app/(dashboard)/compliance/safety-reports/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
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
  FileText,
  FileSearch,
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
  AlertOctagon,
  MoreHorizontal,
  User,
  Car,
  FileCheck,
  XCircle,
  Flag,
  EyeOff,
  UserCheck,
  UserX,
  MapPin,
  Clock as ClockIcon,
} from "lucide-react";

// ==================== TYPES ====================

type SafetyReport = {
  id: string;
  reference: string;
  type: "Hazard" | "NearMiss" | "UnsafeCondition" | "Incident" | "SafetyConcern";
  severity: "Critical" | "High" | "Medium" | "Low";
  title: string;
  description: string;
  location: string;
  branch: string;
  workReference: string | null;
  reportedBy: string | null;
  isAnonymous: boolean;
  status: "Open" | "UnderReview" | "Assigned" | "Resolved" | "Closed";
  assignedTo: string | null;
  createdAt: string;
  updatedAt: string;
  evidenceCount: number;
  followUpActions: string[];
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_REPORTS: SafetyReport[] = [
  {
    id: "SR-001",
    reference: "SR-2026-001",
    type: "Hazard",
    severity: "High",
    title: "Slippery floor at Ikeja depot",
    description: "Oil spill near fuel pump area. Multiple providers reported near-misses.",
    location: "Ikeja Depot - Fuel Bay",
    branch: "Ikeja",
    workReference: "KG-8491",
    reportedBy: "A. Yusuf",
    isAnonymous: false,
    status: "Assigned",
    assignedTo: "F. Daniel",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T09:15:00Z",
    evidenceCount: 2,
    followUpActions: ["Clean spill area", "Apply anti-slip coating", "Review safety protocol"],
    tags: ["Spill", "SlipHazard", "Depot"],
  },
  {
    id: "SR-002",
    reference: "SR-2026-002",
    type: "NearMiss",
    severity: "High",
    title: "Near miss - Vehicle BR-204",
    description: "Provider reported near collision with pedestrian at Lekki junction. Emergency brake prevented accident.",
    location: "Lekki Junction",
    branch: "Lekki",
    workReference: "KG-8487",
    reportedBy: "P. Okafor",
    isAnonymous: false,
    status: "Open",
    assignedTo: null,
    createdAt: "2026-08-22T08:45:00Z",
    updatedAt: "2026-08-22T08:45:00Z",
    evidenceCount: 0,
    followUpActions: ["Review dashcam footage", "Contact provider for statement"],
    tags: ["Vehicle", "Pedestrian", "NearMiss"],
  },
  {
    id: "SR-003",
    reference: "SR-2026-003",
    type: "UnsafeCondition",
    severity: "Critical",
    title: "Unsafe vehicle - TR-112",
    description: "Truck TR-112 reported with brake issues during pre-trip inspection. Vehicle should not operate.",
    location: "Apapa Depot",
    branch: "Apapa",
    workReference: "KG-8512",
    reportedBy: "K. Ahmed",
    isAnonymous: false,
    status: "Closed",
    assignedTo: "M. Bello",
    createdAt: "2026-08-19T16:20:00Z",
    updatedAt: "2026-08-21T11:00:00Z",
    evidenceCount: 3,
    followUpActions: ["Inspect brakes", "Repair/replace brake pads", "Pass inspection"],
    tags: ["Vehicle", "Brakes", "Maintenance"],
  },
  {
    id: "SR-004",
    reference: "SR-2026-004",
    type: "SafetyConcern",
    severity: "Medium",
    title: "Lighting issue at Yaba warehouse",
    description: "Anonymous report about poor lighting in storage area. Multiple staff concerned.",
    location: "Yaba Warehouse - Zone C",
    branch: "Yaba",
    workReference: null,
    reportedBy: null,
    isAnonymous: true,
    status: "UnderReview",
    assignedTo: "O. Adebayo",
    createdAt: "2026-08-20T10:00:00Z",
    updatedAt: "2026-08-21T14:30:00Z",
    evidenceCount: 1,
    followUpActions: ["Inspect lighting", "Replace faulty bulbs", "Increase lighting coverage"],
    tags: ["Lighting", "Warehouse", "Anonymous"],
  },
  {
    id: "SR-005",
    reference: "SR-2026-005",
    type: "Incident",
    severity: "Critical",
    title: "Road incident - Provider P-044",
    description: "Provider P-044 involved in collision with another vehicle. No injuries reported but vehicle damaged.",
    location: "Ikeja - Opebi Road",
    branch: "Ikeja",
    workReference: "KG-8479",
    reportedBy: "F. Daniel",
    isAnonymous: false,
    status: "Open",
    assignedTo: null,
    createdAt: "2026-08-22T06:30:00Z",
    updatedAt: "2026-08-22T07:45:00Z",
    evidenceCount: 4,
    followUpActions: ["Gather witness statements", "Obtain police report", "Assess vehicle damage"],
    tags: ["Incident", "Collision", "Provider"],
  },
  {
    id: "SR-006",
    reference: "SR-2026-006",
    type: "Hazard",
    severity: "Low",
    title: "Loose railing at Lekki depot",
    description: "Handrail on loading bay is loose and needs tightening.",
    location: "Lekki Depot - Loading Bay",
    branch: "Lekki",
    workReference: null,
    reportedBy: "E. Obi",
    isAnonymous: false,
    status: "Closed",
    assignedTo: "M. Bello",
    createdAt: "2026-08-18T09:30:00Z",
    updatedAt: "2026-08-20T16:00:00Z",
    evidenceCount: 0,
    followUpActions: ["Tighten railing", "Inspect all railings"],
    tags: ["Maintenance", "LoadingBay"],
  },
  {
    id: "SR-007",
    reference: "SR-2026-007",
    type: "NearMiss",
    severity: "Medium",
    title: "Near miss - Warehouse forklift",
    description: "Forklift nearly collided with pedestrian in warehouse aisle. No injury but close call.",
    location: "Yaba Warehouse - Aisle 4",
    branch: "Yaba",
    workReference: "WH-2026-884",
    reportedBy: null,
    isAnonymous: true,
    status: "Resolved",
    assignedTo: "O. Adebayo",
    createdAt: "2026-08-17T11:15:00Z",
    updatedAt: "2026-08-19T13:00:00Z",
    evidenceCount: 2,
    followUpActions: ["Review forklift protocol", "Additional training for operators"],
    tags: ["Forklift", "Warehouse", "NearMiss"],
  },
  {
    id: "SR-008",
    reference: "SR-2026-008",
    type: "SafetyConcern",
    severity: "High",
    title: "Security concern - Apapa depot",
    description: "Unauthorized access reported at depot perimeter. Security breach detected.",
    location: "Apapa Depot - South Gate",
    branch: "Apapa",
    workReference: null,
    reportedBy: "K. Ahmed",
    isAnonymous: false,
    status: "UnderReview",
    assignedTo: "F. Daniel",
    createdAt: "2026-08-22T00:30:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    evidenceCount: 5,
    followUpActions: ["Review CCTV footage", "Increase security patrols", "Repair gate sensor"],
    tags: ["Security", "Breach", "Depot"],
  },
];

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Hazard: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  NearMiss: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  UnsafeCondition: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  Incident: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  SafetyConcern: { bg: "bg-purple-bg", text: "text-purple", icon: <Flag className="h-3.5 w-3.5" /> },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertOctagon className="h-3.5 w-3.5" />,
  },
  High: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  Medium: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Low: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Info className="h-3.5 w-3.5" />,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  UnderReview: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Assigned: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_FILTERS = ["All", "Hazard", "NearMiss", "UnsafeCondition", "Incident", "SafetyConcern"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "UnderReview", "Assigned", "Resolved", "Closed"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function SafetyReportsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SafetyReport[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [showAnonymous, setShowAnonymous] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_REPORTS);
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
      (item.reportedBy && item.reportedBy.toLowerCase().includes(search.toLowerCase())) ||
      item.location.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesType && matchesSeverity && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading safety reports...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedType !== "All" ||
    selectedSeverity !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedType !== "All" ? 1 : 0) +
    (selectedSeverity !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

  const formatDate = (date: string) => {
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

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
  const totalOpen = data.filter((d) => d.status === "Open").length;
  const totalCritical = data.filter((d) => d.severity === "Critical").length;
  const totalUnderReview = data.filter((d) => d.status === "UnderReview").length;
  const totalResolved = data.filter((d) => d.status === "Resolved" || d.status === "Closed").length;
  const totalAnonymous = data.filter((d) => d.isAnonymous).length;

  const goToReportDetail = (id: string) => {
    router.push(`/compliance/safety-reports/${id}`);
  };

  const goToCreateReport = () => {
    router.push("/compliance/safety-reports/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Safety Reports
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Hazard, near miss, unsafe condition, and safety concern reports. Anonymous reports preserve reporter identity.
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
            onClick={goToCreateReport}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Report Safety Concern
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Under Review</div>
          <div className="text-lg font-bold text-amber">{totalUnderReview}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Critical</div>
          <div className="text-lg font-bold text-red">{totalCritical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Anonymous</div>
          <div className="text-lg font-bold text-purple">{totalAnonymous}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, reporter..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Hazard", "NearMiss", "UnsafeCondition", "Incident", "SafetyConcern"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Flag className="h-4 w-4 text-ink-400" />
          {selectedType}
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
            const statuses = ["All", "Open", "UnderReview", "Assigned", "Resolved", "Closed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileCheck className="h-4 w-4 text-ink-400" />
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

      {/* ===== REPORTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileSearch className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No safety reports found</h3>
          <p className="mt-1 text-sm text-ink-500">No reports match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Report</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Evidence</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Hazard;
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Open;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToReportDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="flex items-center gap-1.5 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          {item.isAnonymous && (
                            <span className="flex items-center gap-0.5 text-[9px] text-purple font-medium">
                              <EyeOff className="h-3 w-3" />
                              Anonymous
                            </span>
                          )}
                          {item.reportedBy && !item.isAnonymous && (
                            <span className="text-[9px] text-ink-400">by {item.reportedBy}</span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
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
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs text-center">
                        {item.evidenceCount > 0 ? (
                          <span className="flex items-center justify-center gap-1 text-primary font-medium">
                            <FileText className="h-3.5 w-3.5" />
                            {item.evidenceCount}
                          </span>
                        ) : (
                          <span className="text-ink-400">—</span>
                        )}
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
          <Shield className="h-4 w-4" />
          CMP-06 · Safety Reports
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} reports shown
        </span>
      </div>
    </div>
  );
}