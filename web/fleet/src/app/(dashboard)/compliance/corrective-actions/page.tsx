// src/app/(dashboard)/compliance/corrective-actions/page.tsx

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
  UserCheck,
  UserX,
  MapPin,
  Clock as ClockIcon,
  Briefcase,
  Phone,
  Mail,
  ClipboardList,
  Link2,
  ExternalLink,
} from "lucide-react";

// ==================== TYPES ====================

type CorrectiveAction = {
  id: string;
  reference: string;
  title: string;
  description: string;
  status: "Pending" | "InProgress" | "Completed" | "Verified" | "Cancelled";
  priority: "Urgent" | "High" | "Normal" | "Low";
  assignedTo: string | null;
  createdBy: string;
  createdAt: string;
  dueDate: string | null;
  completedAt: string | null;
  completedBy: string | null;
  verifiedBy: string | null;
  verifiedAt: string | null;
  incidentLink: string | null;
  incidentReference: string | null;
  verificationRequired: boolean;
  tags: string[];
  branch: string;
  notes: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_ACTIONS: CorrectiveAction[] = [
  {
    id: "CA-001",
    reference: "CA-2026-001",
    title: "Obtain police report for incident INC-771",
    description: "Contact police station to obtain official report for the collision incident involving provider P-044.",
    status: "Completed",
    priority: "High",
    assignedTo: "F. Daniel",
    createdBy: "F. Daniel",
    createdAt: "2026-08-22T07:15:00Z",
    dueDate: "2026-08-23T17:00:00Z",
    completedAt: "2026-08-22T08:30:00Z",
    completedBy: "F. Daniel",
    verifiedBy: null,
    verifiedAt: null,
    incidentLink: "INC-771",
    incidentReference: "INC-2026-771",
    verificationRequired: false,
    tags: ["Police", "Document", "Incident"],
    branch: "Ikeja",
    notes: "Report obtained and uploaded to incident evidence.",
  },
  {
    id: "CA-002",
    reference: "CA-2026-002",
    title: "Review dashcam footage for INC-771",
    description: "Review dashcam footage from provider's vehicle to determine sequence of events leading to collision.",
    status: "InProgress",
    priority: "High",
    assignedTo: "F. Daniel",
    createdBy: "F. Daniel",
    createdAt: "2026-08-22T07:15:00Z",
    dueDate: "2026-08-24T17:00:00Z",
    completedAt: null,
    completedBy: null,
    verifiedBy: null,
    verifiedAt: null,
    incidentLink: "INC-771",
    incidentReference: "INC-2026-771",
    verificationRequired: true,
    tags: ["Video", "Investigation", "Incident"],
    branch: "Ikeja",
    notes: "Dashcam footage received. Review in progress.",
  },
  {
    id: "CA-003",
    reference: "CA-2026-003",
    title: "Assess vehicle damage and create work order",
    description: "Complete damage assessment on BR-204 and create work order for repairs.",
    status: "Pending",
    priority: "Normal",
    assignedTo: null,
    createdBy: "F. Daniel",
    createdAt: "2026-08-22T07:15:00Z",
    dueDate: "2026-08-25T17:00:00Z",
    completedAt: null,
    completedBy: null,
    verifiedBy: null,
    verifiedAt: null,
    incidentLink: "INC-771",
    incidentReference: "INC-2026-771",
    verificationRequired: false,
    tags: ["Vehicle", "Repair", "WorkOrder"],
    branch: "Ikeja",
    notes: "Awaiting assignment to maintenance team.",
  },
  {
    id: "CA-004",
    reference: "CA-2026-004",
    title: "Inspect brake system on TR-112",
    description: "Complete full brake system inspection on TR-112 following safety concern report.",
    status: "Completed",
    priority: "Urgent",
    assignedTo: "M. Bello",
    createdBy: "M. Bello",
    createdAt: "2026-08-19T17:00:00Z",
    dueDate: "2026-08-20T09:00:00Z",
    completedAt: "2026-08-20T08:30:00Z",
    completedBy: "M. Bello",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-21T10:00:00Z",
    incidentLink: "INC-777",
    incidentReference: "INC-2026-777",
    verificationRequired: true,
    tags: ["Vehicle", "Brakes", "Safety", "Inspection"],
    branch: "Apapa",
    notes: "Inspection complete. Brake pads worn below threshold. Work order created for repairs.",
  },
  {
    id: "CA-005",
    reference: "CA-2026-005",
    title: "Repair brake pads and rotors on TR-112",
    description: "Replace worn brake pads and damaged rotors on TR-112.",
    status: "Completed",
    priority: "Urgent",
    assignedTo: "M. Bello",
    createdBy: "M. Bello",
    createdAt: "2026-08-19T17:15:00Z",
    dueDate: "2026-08-20T17:00:00Z",
    completedAt: "2026-08-20T16:45:00Z",
    completedBy: "M. Bello",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-21T10:00:00Z",
    incidentLink: "INC-777",
    incidentReference: "INC-2026-777",
    verificationRequired: true,
    tags: ["Vehicle", "Repair", "Parts"],
    branch: "Apapa",
    notes: "All components replaced. Vehicle ready for post-repair inspection.",
  },
  {
    id: "CA-006",
    reference: "CA-2026-006",
    title: "Pass post-repair inspection on TR-112",
    description: "Complete post-repair inspection to verify brake system is fully operational.",
    status: "Verified",
    priority: "High",
    assignedTo: "M. Bello",
    createdBy: "M. Bello",
    createdAt: "2026-08-20T17:00:00Z",
    dueDate: "2026-08-21T12:00:00Z",
    completedAt: "2026-08-21T10:30:00Z",
    completedBy: "M. Bello",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-21T10:30:00Z",
    incidentLink: "INC-777",
    incidentReference: "INC-2026-777",
    verificationRequired: true,
    tags: ["Vehicle", "Inspection", "Safety"],
    branch: "Apapa",
    notes: "Vehicle passed all safety checks. Cleared for operation.",
  },
  {
    id: "CA-007",
    reference: "CA-2026-007",
    title: "Update fuel handling safety protocol",
    description: "Review and update safety protocol for fuel handling at all depots following spill incident.",
    status: "InProgress",
    priority: "High",
    assignedTo: "O. Adebayo",
    createdBy: "F. Daniel",
    createdAt: "2026-08-21T16:00:00Z",
    dueDate: "2026-08-28T17:00:00Z",
    completedAt: null,
    completedBy: null,
    verifiedBy: null,
    verifiedAt: null,
    incidentLink: "SR-001",
    incidentReference: "SR-2026-001",
    verificationRequired: true,
    tags: ["Safety", "Protocol", "Depot"],
    branch: "Ikeja",
    notes: "Draft protocol under review. Stakeholder feedback being collected.",
  },
  {
    id: "CA-008",
    reference: "CA-2026-008",
    title: "Apply anti-slip coating to fuel bay area",
    description: "Apply anti-slip coating to fuel bay area at Ikeja depot following slip hazard report.",
    status: "Cancelled",
    priority: "Normal",
    assignedTo: "M. Bello",
    createdBy: "F. Daniel",
    createdAt: "2026-08-21T16:15:00Z",
    dueDate: "2026-08-23T17:00:00Z",
    completedAt: null,
    completedBy: null,
    verifiedBy: null,
    verifiedAt: null,
    incidentLink: null,
    incidentReference: null,
    verificationRequired: false,
    tags: ["Maintenance", "Safety", "Depot"],
    branch: "Ikeja",
    notes: "Cancelled - alternative solution being implemented.",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  InProgress: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Verified: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <ShieldCheck className="h-3.5 w-3.5" />,
  },
  Cancelled: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  Urgent: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "Pending", "InProgress", "Completed", "Verified", "Cancelled"];
const PRIORITY_FILTERS = ["All", "Urgent", "High", "Normal", "Low"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function CorrectiveActionsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CorrectiveAction[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedPriority, setSelectedPriority] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_ACTIONS);
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
      (item.assignedTo && item.assignedTo.toLowerCase().includes(search.toLowerCase())) ||
      (item.incidentReference && item.incidentReference.toLowerCase().includes(search.toLowerCase()));

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesPriority = selectedPriority === "All" || item.priority === selectedPriority;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesPriority && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading corrective actions...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedPriority !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedPriority("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedPriority !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
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
  const totalPending = data.filter((d) => d.status === "Pending").length;
  const totalInProgress = data.filter((d) => d.status === "InProgress").length;
  const totalCompleted = data.filter((d) => d.status === "Completed" || d.status === "Verified").length;
  const totalUrgent = data.filter((d) => d.priority === "Urgent").length;
  const totalVerified = data.filter((d) => d.status === "Verified").length;

  const goToActionDetail = (id: string) => {
    router.push(`/compliance/corrective-actions/${id}`);
  };

  const goToCreateAction = () => {
    router.push("/compliance/corrective-actions/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Corrective Actions
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Actions to address incidents, safety concerns, and compliance gaps. Track from creation through verification.
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
            onClick={goToCreateAction}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Action
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">In Progress</div>
          <div className="text-lg font-bold text-blue">{totalInProgress}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Verified</div>
          <div className="text-lg font-bold text-purple">{totalVerified}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Urgent</div>
          <div className="text-lg font-bold text-red">{totalUrgent}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, assignee..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "InProgress", "Completed", "Verified", "Cancelled"];
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
          onClick={() => {
            const priorities = ["All", "Urgent", "High", "Normal", "Low"];
            const currentIndex = priorities.indexOf(selectedPriority);
            const nextIndex = (currentIndex + 1) % priorities.length;
            setSelectedPriority(priorities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Flag className="h-4 w-4 text-ink-400" />
          {selectedPriority}
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
            <button              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== ACTIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <ClipboardList className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No corrective actions found</h3>
          <p className="mt-1 text-sm text-ink-500">No actions match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Action</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Priority</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Assigned To</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Due</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                  const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Normal;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToActionDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          {item.incidentReference && (
                            <span className="flex items-center gap-0.5 text-[9px] text-blue font-medium">
                              <Link2 className="h-3 w-3" />
                              {item.incidentReference}
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
                          {item.priority}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.assignedTo || "Unassigned"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">
                        {item.dueDate ? formatDate(item.dueDate) : "No due date"}
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
          CMP-10 · Corrective Actions
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} actions shown
        </span>
      </div>
    </div>
  );
}