// src/app/(dashboard)/system/jobs/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Download,
  Search,
  Server,
  ChevronDown,
  FlagTriangleRight,
  SlidersHorizontal,
  X,
  Clock,
  CheckCircle2,
  RotateCw,
  XCircle,
  Inbox,
  Eye,
} from "lucide-react";

// ==================== TYPES ====================

type BackgroundJob = {
  id: string;
  reference: string;
  type: string;
  subject: string;
  subjectId: string;
  state: "Queued" | "Running" | "Retrying" | "Completed" | "Failed" | "Cancelled" | "DeadLettered";
  priority: "Critical" | "High" | "Normal" | "Low";
  nextAttempt: string | null;
  attemptCount: number;
  maxAttempts: number;
  age: string;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  error: string | null;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_JOBS: BackgroundJob[] = [
  {
    id: "JOB-001",
    reference: "JOB-2026-001",
    type: "SyncJob",
    subject: "Provider Sync",
    subjectId: "P-044",
    state: "Running",
    priority: "High",
    nextAttempt: null,
    attemptCount: 1,
    maxAttempts: 3,
    age: "2m",
    createdAt: "2026-08-22T14:28:00Z",
    updatedAt: "2026-08-22T14:30:00Z",
    completedAt: null,
    error: null,
    branch: "Ikeja",
    tags: ["Sync", "Provider", "Running"],
  },
  {
    id: "JOB-002",
    reference: "JOB-2026-002",
    type: "VerificationJob",
    subject: "Document Verification",
    subjectId: "DOC-005",
    state: "Queued",
    priority: "Normal",
    nextAttempt: "2026-08-22T14:45:00Z",
    attemptCount: 0,
    maxAttempts: 5,
    age: "5m",
    createdAt: "2026-08-22T14:25:00Z",
    updatedAt: "2026-08-22T14:25:00Z",
    completedAt: null,
    error: null,
    branch: "Ikeja",
    tags: ["Verification", "Document", "Queued"],
  },
  {
    id: "JOB-003",
    reference: "JOB-2026-003",
    type: "ExportJob",
    subject: "Order Export",
    subjectId: "EXP-2026-001",
    state: "Retrying",
    priority: "High",
    nextAttempt: "2026-08-22T14:50:00Z",
    attemptCount: 2,
    maxAttempts: 3,
    age: "2h",
    createdAt: "2026-08-22T12:30:00Z",
    updatedAt: "2026-08-22T14:00:00Z",
    completedAt: null,
    error: "Connection timeout",
    branch: "All branches",
    tags: ["Export", "Retrying", "Timeout"],
  },
  {
    id: "JOB-004",
    reference: "JOB-2026-004",
    type: "ReconciliationJob",
    subject: "Wallet Reconciliation",
    subjectId: "WAL-001",
    state: "Completed",
    priority: "Normal",
    nextAttempt: null,
    attemptCount: 1,
    maxAttempts: 3,
    age: "3h",
    createdAt: "2026-08-22T11:00:00Z",
    updatedAt: "2026-08-22T11:30:00Z",
    completedAt: "2026-08-22T11:30:00Z",
    error: null,
    branch: "All branches",
    tags: ["Reconciliation", "Wallet", "Completed"],
  },
  {
    id: "JOB-005",
    reference: "JOB-2026-005",
    type: "NotificationJob",
    subject: "SLA Breach Notification",
    subjectId: "SLA-001",
    state: "Failed",
    priority: "High",
    nextAttempt: "2026-08-22T15:00:00Z",
    attemptCount: 3,
    maxAttempts: 3,
    age: "1h",
    createdAt: "2026-08-22T13:30:00Z",
    updatedAt: "2026-08-22T14:30:00Z",
    completedAt: null,
    error: "Email server error: 500 Internal Server Error",
    branch: "Ikeja",
    tags: ["Notification", "Failed", "Email"],
  },
  {
    id: "JOB-006",
    reference: "JOB-2026-006",
    type: "ImportJob",
    subject: "Bulk Order Import",
    subjectId: "IMP-003",
    state: "Cancelled",
    priority: "Normal",
    nextAttempt: null,
    attemptCount: 1,
    maxAttempts: 3,
    age: "1d",
    createdAt: "2026-08-21T09:00:00Z",
    updatedAt: "2026-08-21T09:30:00Z",
    completedAt: null,
    error: "Cancelled by user",
    branch: "Lekki",
    tags: ["Import", "Cancelled", "Bulk"],
  },
  {
    id: "JOB-007",
    reference: "JOB-2026-007",
    type: "WebhookJob",
    subject: "Order Webhook",
    subjectId: "WH-001",
    state: "DeadLettered",
    priority: "Normal",
    nextAttempt: null,
    attemptCount: 3,
    maxAttempts: 3,
    age: "45m",
    createdAt: "2026-08-22T14:15:00Z",
    updatedAt: "2026-08-22T14:45:00Z",
    completedAt: null,
    error: "Webhook delivery failed: 404 Not Found",
    branch: "All branches",
    tags: ["Webhook", "DeadLettered", "Delivery"],
  },
  {
    id: "JOB-008",
    reference: "JOB-2026-008",
    type: "ReportJob",
    subject: "Report Generation",
    subjectId: "RPT-004",
    state: "Queued",
    priority: "Low",
    nextAttempt: "2026-08-22T15:30:00Z",
    attemptCount: 0,
    maxAttempts: 2,
    age: "10m",
    createdAt: "2026-08-22T14:20:00Z",
    updatedAt: "2026-08-22T14:20:00Z",
    completedAt: null,
    error: null,
    branch: "All branches",
    tags: ["Report", "Queued", "Generation"],
  },
];

type StateStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATE_STYLES: Record<string, StateStyle> = {
  Queued: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Retrying: { bg: "bg-amber-bg", text: "text-amber", icon: <RotateCw className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
  DeadLettered: { bg: "bg-purple-bg", text: "text-purple", icon: <Inbox className="h-3.5 w-3.5" /> },
};

type PriorityStyle = {
  bg: string;
  text: string;
};

const PRIORITY_STYLES: Record<string, PriorityStyle> = {
  Critical: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATE_FILTERS = ["All", "Queued", "Running", "Retrying", "Completed", "Failed", "Cancelled", "DeadLettered"];
const PRIORITY_FILTERS = ["All", "Critical", "High", "Normal", "Low"];

export default function BackgroundJobsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<BackgroundJob[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedState, setSelectedState] = useState<string>("All");
  const [selectedPriority, setSelectedPriority] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_JOBS);
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
      item.subject.toLowerCase().includes(search.toLowerCase()) ||
      item.subjectId.toLowerCase().includes(search.toLowerCase()) ||
      item.type.toLowerCase().includes(search.toLowerCase());

    const matchesState = selectedState === "All" || item.state === selectedState;
    const matchesPriority = selectedPriority === "All" || item.priority === selectedPriority;

    return matchesSearch && matchesState && matchesPriority;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading background jobs...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedState !== "All" ||
    selectedPriority !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedState("All");
    setSelectedPriority("All");
  };

  const filterCount = (selectedState !== "All" ? 1 : 0) + (selectedPriority !== "All" ? 1 : 0);

  const formatTime = (date: string): string => {
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
  const totalActive = data.filter((d) => d.state === "Queued" || d.state === "Running" || d.state === "Retrying").length;
  const totalCompleted = data.filter((d) => d.state === "Completed").length;
  const totalFailed = data.filter((d) => d.state === "Failed" || d.state === "DeadLettered").length;

  const goToJobDetail = (id: string) => {
    router.push(`/system/jobs/${id}`);
  };

  const handleRetry = (id: string) => {
    toast(`Retrying job ${id} (mock)`);
  };

  const handleCancel = (id: string) => {
    toast(`Cancelling job ${id} (mock)`);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Background Jobs
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Job type, safe subject, state, priority, next attempt, attempt count and age.
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
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-blue">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Completed</div>
          <div className="text-lg font-bold text-green">{totalCompleted}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Failed</div>
          <div className="text-lg font-bold text-red">{totalFailed}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, subject, type..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const states = ["All", "Queued", "Running", "Retrying", "Completed", "Failed", "Cancelled", "DeadLettered"];
            const currentIndex = states.indexOf(selectedState);
            const nextIndex = (currentIndex + 1) % states.length;
            setSelectedState(states[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Server className="h-4 w-4 text-ink-400" />
          {selectedState}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const priorities = ["All", "Critical", "High", "Normal", "Low"];
            const currentIndex = priorities.indexOf(selectedPriority);
            const nextIndex = (currentIndex + 1) % priorities.length;
            setSelectedPriority(priorities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FlagTriangleRight className="h-4 w-4 text-ink-400" />
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

      {/* ===== JOBS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Server className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No jobs found</h3>
          <p className="mt-1 text-sm text-ink-500">No background jobs match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Job</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">State</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Priority</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Attempts</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const stateStyle = STATE_STYLES[item.state] || STATE_STYLES.Queued;
                  const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Normal;

                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line hover:bg-surface-2 transition-colors group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.subject}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          <span className="text-[9px] text-ink-400 font-mono">{item.subjectId}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="rounded-full px-2.5 py-0.5 text-[10px] font-semibold bg-surface-2 text-ink-600">
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
                          {stateStyle.icon}
                          {item.state}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
                          {item.priority}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">
                        {item.attemptCount}/{item.maxAttempts}
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{item.age}</td>
                      <td className="px-4 py-3 text-right flex items-center justify-end gap-1">
                        {(item.state === "Queued" || item.state === "Retrying" || item.state === "Failed" || item.state === "DeadLettered") && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleRetry(item.id);
                            }}
                            className="text-primary hover:text-primary/80 transition-colors p-1"
                            title="Retry"
                          >
                            <RefreshCw className="h-4 w-4" />
                          </button>
                        )}
                        {(item.state === "Queued" || item.state === "Running" || item.state === "Retrying") && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleCancel(item.id);
                            }}
                            className="text-red hover:text-red/80 transition-colors p-1"
                            title="Cancel"
                          >
                            <X className="h-4 w-4" />
                          </button>
                        )}
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            goToJobDetail(item.id);
                          }}
                          className="text-ink-400 hover:text-primary transition-colors p-1"
                        >
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
          <Server className="h-4 w-4" />
          SYS-05 · Background Jobs
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} jobs shown
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}