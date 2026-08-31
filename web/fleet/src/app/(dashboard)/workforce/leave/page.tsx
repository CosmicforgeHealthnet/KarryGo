// src/app/(dashboard)/workforce/leave/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Search,
  Plus,
  Calendar,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Eye,
  Filter,
  ArrowRight,
  CalendarDays,
  User,
  FileText,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Users,
  Briefcase,
  Shield,
  Activity,
  Clock as ClockIcon,
  UserCheck,
  UserX,
  MessageSquare,
  Send,
} from "lucide-react";

type LeaveRequest = {
  id: string;
  personId: string;
  name: string;
  initials: string;
  type: "Annual Leave" | "Sick Leave" | "Personal Leave" | "Maternity Leave" | "Other";
  startDate: string;
  endDate: string;
  days: number;
  status: "Pending" | "Approved" | "Rejected" | "Cancelled";
  reason: string;
  submittedAt: string;
  approvedBy: string | null;
  branch: string;
  role: string;
};

// Mock data
const LEAVE_REQUESTS: LeaveRequest[] = [
  {
    id: "LV-001",
    personId: "P-044",
    name: "Peter Okafor",
    initials: "PO",
    type: "Annual Leave",
    startDate: "2026-09-01",
    endDate: "2026-09-05",
    days: 5,
    status: "Pending",
    reason: "Family vacation. Planning to travel to the village.",
    submittedAt: "2026-08-15T10:30:00Z",
    approvedBy: null,
    branch: "Ikeja",
    role: "Dispatch Rider",
  },
  {
    id: "LV-002",
    personId: "P-038",
    name: "Musa Bello",
    initials: "MB",
    type: "Sick Leave",
    startDate: "2026-08-22",
    endDate: "2026-08-23",
    days: 2,
    status: "Approved",
    reason: "Hospital appointment. Doctor's note attached.",
    submittedAt: "2026-08-18T09:15:00Z",
    approvedBy: "T. Adeyemi",
    branch: "Apapa",
    role: "Hauling Driver",
  },
  {
    id: "LV-003",
    personId: "P-052",
    name: "Amina Yusuf",
    initials: "AY",
    type: "Personal Leave",
    startDate: "2026-08-25",
    endDate: "2026-08-25",
    days: 1,
    status: "Pending",
    reason: "Personal errands. Need to attend to urgent family matter.",
    submittedAt: "2026-08-20T14:20:00Z",
    approvedBy: null,
    branch: "Yaba",
    role: "Dispatch Supervisor",
  },
  {
    id: "LV-004",
    personId: "P-061",
    name: "Chinedu Uche",
    initials: "CU",
    type: "Annual Leave",
    startDate: "2026-08-10",
    endDate: "2026-08-14",
    days: 5,
    status: "Approved",
    reason: "Annual leave. Traveling to the East.",
    submittedAt: "2026-07-25T11:00:00Z",
    approvedBy: "F. Daniel",
    branch: "Lekki",
    role: "Dispatch Rider",
  },
  {
    id: "LV-005",
    personId: "P-029",
    name: "Kabiru Ahmed",
    initials: "KA",
    type: "Sick Leave",
    startDate: "2026-08-19",
    endDate: "2026-08-21",
    days: 3,
    status: "Rejected",
    reason: "Influenza. Unable to work.",
    submittedAt: "2026-08-17T08:45:00Z",
    approvedBy: "T. Adeyemi",
    branch: "Apapa",
    role: "Hauling Driver",
  },
  {
    id: "LV-006",
    personId: "P-073",
    name: "Esther Obi",
    initials: "EO",
    type: "Maternity Leave",
    startDate: "2026-10-01",
    endDate: "2027-01-15",
    days: 45,
    status: "Pending",
    reason: "Maternity leave. Expecting delivery in October.",
    submittedAt: "2026-08-10T16:00:00Z",
    approvedBy: null,
    branch: "Ikeja",
    role: "Fleet Manager",
  },
  {
    id: "LV-007",
    personId: "P-088",
    name: "Tunde Fatoki",
    initials: "TF",
    type: "Personal Leave",
    startDate: "2026-08-18",
    endDate: "2026-08-18",
    days: 1,
    status: "Cancelled",
    reason: "Funeral attendance. Later cancelled.",
    submittedAt: "2026-08-15T13:30:00Z",
    approvedBy: "F. Daniel",
    branch: "Ikeja",
    role: "Maintenance Tech",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
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
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Cancelled: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
};

const TYPE_BADGES: Record<string, { bg: string; text: string }> = {
  "Annual Leave": { bg: "bg-primary-bg", text: "text-primary" },
  "Sick Leave": { bg: "bg-red-bg", text: "text-red" },
  "Personal Leave": { bg: "bg-amber-bg", text: "text-amber" },
  "Maternity Leave": { bg: "bg-purple-bg", text: "text-purple" },
  "Other": { bg: "bg-surface-2", text: "text-ink-600" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Pending", "Approved", "Rejected", "Cancelled"];
const TYPE_FILTERS = ["All", "Annual Leave", "Sick Leave", "Personal Leave", "Maternity Leave", "Other"];

export default function LeaveManagementPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<LeaveRequest[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(LEAVE_REQUESTS);
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
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.reason.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesType = selectedType === "All" || item.type === selectedType;

    return matchesSearch && matchesBranch && matchesStatus && matchesType;
  });

  const stats = {
    total: data.length,
    pending: data.filter((d) => d.status === "Pending").length,
    approved: data.filter((d) => d.status === "Approved").length,
    rejected: data.filter((d) => d.status === "Rejected").length,
  };

  // Navigation handlers
  const goToPerson = (personId: string) => {
    router.push(`/workforce/${personId}`);
  };

  const handleApprove = (id: string) => {
    if (window.confirm("Approve this leave request?")) {
      const updated = data.map((item) =>
        item.id === id ? { ...item, status: "Approved" as const, approvedBy: "Current User" } : item
      );
      setData(updated);
    }
  };

  const handleReject = (id: string) => {
    if (window.confirm("Reject this leave request?")) {
      const updated = data.map((item) =>
        item.id === id ? { ...item, status: "Rejected" as const, approvedBy: "Current User" } : item
      );
      setData(updated);
    }
  };

  const handleCancel = (id: string) => {
    if (window.confirm("Cancel this leave request?")) {
      const updated = data.map((item) =>
        item.id === id ? { ...item, status: "Cancelled" as const } : item
      );
      setData(updated);
    }
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading leave requests...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    selectedType !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
    setSelectedType("All");
  };

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="absolute -top-1 -right-1">
                <Sparkles className="h-4 w-4 text-amber animate-pulse" />
              </div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                Leave Management
              </h1>
            </div>
            <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-[11px] font-semibold text-primary">
              {data.length}
            </span>
            <div className="flex items-center gap-1 rounded-full bg-amber-bg px-2.5 py-0.5 text-[10px] font-semibold text-amber">
              <Clock className="h-3 w-3" />
              {stats.pending} pending
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Manage leave requests, approvals, and unavailability across the workforce.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            Request Leave
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Pending</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.pending}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <AlertCircle className="h-3 w-3" />
            <span>Awaiting review</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Approved</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.approved}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+3 this month</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5">
              <XCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Rejected</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.rejected}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>Follow up needed</span>
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[240px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, ID, or reason..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          {selectedBranch}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-[12px] font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0) +
                (selectedType !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-[12px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
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

      {/* ===== LEAVE REQUESTS LIST ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Calendar className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No leave requests found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No leave requests match your current filters.
          </p>
          <button
            onClick={clearFilters}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="space-y-2.5">
          {filteredData.map((request) => {
            const statusStyle = STATUS_STYLES[request.status] || STATUS_STYLES.Pending;
            const typeStyle = TYPE_BADGES[request.type] || TYPE_BADGES.Other;
            const isPending = request.status === "Pending";

            return (
              <div
                key={request.id}
                className="flex flex-col gap-2 rounded-xl border border-line bg-white p-3.5 transition hover:shadow-sm sm:flex-row sm:items-start sm:justify-between"
              >
                {/* Left - Person & Details */}
                <div className="min-w-0 flex-1">
                  <div
                    onClick={() => goToPerson(request.personId)}
                    className="flex cursor-pointer items-center gap-3"
                  >
                    <div
                      className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white ${
                        request.status === "Pending" ? "bg-amber" :
                        request.status === "Approved" ? "bg-green" :
                        request.status === "Rejected" ? "bg-red" :
                        "bg-ink-400"
                      }`}
                    >
                      {request.initials}
                    </div>
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-[14px] font-semibold text-ink-950">{request.name}</span>
                        <span className="font-mono text-[10px] text-ink-400">{request.personId}</span>
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {request.status}
                        </span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-2 text-[12px] text-ink-500">
                        <span className="flex items-center gap-1">
                          <Briefcase className="h-3.5 w-3.5" />
                          {request.role}
                        </span>
                        <span className="text-ink-300">·</span>
                        <span className="flex items-center gap-1">
                          <MapPin className="h-3.5 w-3.5" />
                          {request.branch}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                {/* Middle - Leave Details */}
                <div className="flex flex-wrap items-center gap-3 sm:ml-4">
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                    {request.type}
                  </span>
                  <div className="flex items-center gap-1 text-[12px] text-ink-500">
                    <CalendarDays className="h-3.5 w-3.5" />
                    <span>{request.startDate} → {request.endDate}</span>
                  </div>
                  <div className="text-[12px] font-semibold text-ink-950">
                    {request.days} {request.days === 1 ? "day" : "days"}
                  </div>
                </div>

                {/* Right - Actions */}
                <div className="flex flex-wrap items-center gap-1.5">
                  {isPending && (
                    <>
                      <button
                        onClick={() => handleApprove(request.id)}
                        className="flex items-center gap-1 rounded-lg bg-green px-2.5 py-1.5 text-[11px] font-semibold text-white hover:bg-green/90 transition-colors"
                      >
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        Approve
                      </button>
                      <button
                        onClick={() => handleReject(request.id)}
                        className="flex items-center gap-1 rounded-lg bg-red px-2.5 py-1.5 text-[11px] font-semibold text-white hover:bg-red/90 transition-colors"
                      >
                        <XCircle className="h-3.5 w-3.5" />
                        Reject
                      </button>
                    </>
                  )}
                  {!isPending && request.status !== "Cancelled" && (
                    <button
                      onClick={() => handleCancel(request.id)}
                      className="flex items-center gap-1 rounded-lg border border-line bg-white/70 px-2.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
                    >
                      <X className="h-3.5 w-3.5" />
                      Cancel
                    </button>
                  )}
                  <button
                    onClick={() => goToPerson(request.personId)}
                    className="flex items-center gap-1 rounded-lg border border-line bg-white/70 px-2.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
                  >
                    <Eye className="h-3.5 w-3.5" />
                    View
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}