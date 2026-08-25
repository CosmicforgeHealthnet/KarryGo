// src/app/(dashboard)/system/scheduled-tasks/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Download,
  Search,
  Play,
  ChevronDown,
  CalendarClock,
  CheckCircle2,
  SlidersHorizontal,
  X,
  Clock,
  XCircle,
  Eye,
} from "lucide-react";

// ==================== TYPES ====================

type ScheduledTask = {
  id: string;
  reference: string;
  name: string;
  description: string;
  type: string;
  enabled: boolean;
  schedule: string;
  timezone: string;
  nextRun: string;
  lastRun: string | null;
  lastResult: "Success" | "Failed" | "Running" | null;
  lastError: string | null;
  runCount: number;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_TASKS: ScheduledTask[] = [
  {
    id: "TASK-001",
    reference: "TASK-2026-001",
    name: "Provider Sync",
    description: "Synchronize provider data from authority service every hour.",
    type: "SyncJob",
    enabled: true,
    schedule: "0 * * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-22T15:00:00Z",
    lastRun: "2026-08-22T14:00:00Z",
    lastResult: "Success",
    lastError: null,
    runCount: 245,
    branch: "All branches",
    tags: ["Sync", "Provider", "Hourly"],
  },
  {
    id: "TASK-002",
    reference: "TASK-2026-002",
    name: "Document Verification",
    description: "Verify pending documents against authority service.",
    type: "VerificationJob",
    enabled: true,
    schedule: "*/15 * * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-22T14:45:00Z",
    lastRun: "2026-08-22T14:30:00Z",
    lastResult: "Success",
    lastError: null,
    runCount: 892,
    branch: "All branches",
    tags: ["Verification", "Document", "Every15"],
  },
  {
    id: "TASK-003",
    reference: "TASK-2026-003",
    name: "Export Cleanup",
    description: "Clean up expired exports older than 30 days.",
    type: "MaintenanceJob",
    enabled: true,
    schedule: "0 2 * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-23T02:00:00Z",
    lastRun: "2026-08-22T02:00:00Z",
    lastResult: "Success",
    lastError: null,
    runCount: 124,
    branch: "All branches",
    tags: ["Maintenance", "Cleanup", "Daily"],
  },
  {
    id: "TASK-004",
    reference: "TASK-2026-004",
    name: "Report Generation",
    description: "Generate daily operational reports.",
    type: "ReportJob",
    enabled: false,
    schedule: "0 6 * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-23T06:00:00Z",
    lastRun: "2026-08-21T06:00:00Z",
    lastResult: "Failed",
    lastError: "Report generation timeout",
    runCount: 45,
    branch: "All branches",
    tags: ["Report", "Daily", "Disabled"],
  },
  {
    id: "TASK-005",
    reference: "TASK-2026-005",
    name: "Wallet Reconciliation",
    description: "Reconcile wallet transactions with authority service.",
    type: "ReconciliationJob",
    enabled: true,
    schedule: "0 */4 * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-22T16:00:00Z",
    lastRun: "2026-08-22T12:00:00Z",
    lastResult: "Running",
    lastError: null,
    runCount: 89,
    branch: "All branches",
    tags: ["Reconciliation", "Wallet", "Every4h"],
  },
  {
    id: "TASK-006",
    reference: "TASK-2026-006",
    name: "Notification Cleanup",
    description: "Clean up stale notifications older than 7 days.",
    type: "MaintenanceJob",
    enabled: true,
    schedule: "0 1 * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-23T01:00:00Z",
    lastRun: "2026-08-22T01:00:00Z",
    lastResult: "Success",
    lastError: null,
    runCount: 67,
    branch: "All branches",
    tags: ["Maintenance", "Cleanup", "Notification"],
  },
  {
    id: "TASK-007",
    reference: "TASK-2026-007",
    name: "SLA Measurement",
    description: "Calculate SLA measurements for all active customers.",
    type: "SLAJob",
    enabled: true,
    schedule: "0 3 * * *",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-23T03:00:00Z",
    lastRun: "2026-08-22T03:00:00Z",
    lastResult: "Failed",
    lastError: "Data source unavailable",
    runCount: 23,
    branch: "All branches",
    tags: ["SLA", "Measurement", "Daily"],
  },
  {
    id: "TASK-008",
    reference: "TASK-2026-008",
    name: "Index Rebuild",
    description: "Rebuild search indexes for enterprise search.",
    type: "MaintenanceJob",
    enabled: false,
    schedule: "0 5 * * 0",
    timezone: "Africa/Lagos",
    nextRun: "2026-08-23T05:00:00Z",
    lastRun: "2026-08-16T05:00:00Z",
    lastResult: "Success",
    lastError: null,
    runCount: 12,
    branch: "All branches",
    tags: ["Maintenance", "Index", "Weekly"],
  },
];

type ResultStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const RESULT_STYLES: Record<string, ResultStyle> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
};

type TypeStyle = {
  bg: string;
  text: string;
};

const TYPE_STYLES: Record<string, TypeStyle> = {
  SyncJob: { bg: "bg-blue-bg", text: "text-blue" },
  VerificationJob: { bg: "bg-purple-bg", text: "text-purple" },
  MaintenanceJob: { bg: "bg-amber-bg", text: "text-amber" },
  ReportJob: { bg: "bg-green-bg", text: "text-green" },
  ReconciliationJob: { bg: "bg-red-bg", text: "text-red" },
  SLAJob: { bg: "bg-blue-bg", text: "text-blue" },
};

const STATUS_FILTERS = ["All", "Enabled", "Disabled"];
const TYPE_FILTERS = ["All", "SyncJob", "VerificationJob", "MaintenanceJob", "ReportJob", "ReconciliationJob", "SLAJob"];
const RESULT_FILTERS = ["All", "Success", "Failed", "Running"];

export default function ScheduledTasksPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<ScheduledTask[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedType, setSelectedType] = useState<string>("All");
  const [selectedResult, setSelectedResult] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_TASKS);
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
      item.type.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || (selectedStatus === "Enabled" ? item.enabled : !item.enabled);
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesResult = selectedResult === "All" || item.lastResult === selectedResult;

    return matchesSearch && matchesStatus && matchesType && matchesResult;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading scheduled tasks...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedType !== "All" ||
    selectedResult !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedType("All");
    setSelectedResult("All");
  };

  const filterCount =
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedType !== "All" ? 1 : 0) +
    (selectedResult !== "All" ? 1 : 0);

  const formatTime = (date: string | null): string => {
    if (!date) return "Never";
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

  const formatSchedule = (schedule: string): string => {
    const parts = schedule.split(" ");
    if (parts.length === 5) {
      return `${parts[0]} ${parts[1]} ${parts[2]} ${parts[3]} ${parts[4]}`;
    }
    return schedule;
  };

  // Stats
  const totalEnabled = data.filter((d) => d.enabled).length;
  const totalSuccess = data.filter((d) => d.lastResult === "Success").length;
  const totalFailed = data.filter((d) => d.lastResult === "Failed").length;

  const goToTaskDetail = (id: string) => {
    router.push(`/system/scheduled-tasks/${id}`);
  };

  const handleToggle = (id: string) => {
    toast(`Toggled task ${id} (mock)`);
  };

  const handleRunNow = (id: string) => {
    toast(`Running task ${id} now (mock)`);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Scheduled Tasks
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Task type, enabled state, schedule/timezone, next run and last result.
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Enabled</div>
          <div className="text-lg font-bold text-green">{totalEnabled}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Success</div>
          <div className="text-lg font-bold text-green">{totalSuccess}</div>
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
            placeholder="Search by name, reference, description..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Enabled", "Disabled"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Play className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const types = ["All", "SyncJob", "VerificationJob", "MaintenanceJob", "ReportJob", "ReconciliationJob", "SLAJob"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CalendarClock className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const results = ["All", "Success", "Failed", "Running"];
            const currentIndex = results.indexOf(selectedResult);
            const nextIndex = (currentIndex + 1) % results.length;
            setSelectedResult(results[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CheckCircle2 className="h-4 w-4 text-ink-400" />
          {selectedResult}
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

      {/* ===== TASKS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <CalendarClock className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No scheduled tasks found</h3>
          <p className="mt-1 text-sm text-ink-500">No tasks match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Task</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Schedule</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Last Run</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Enabled</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const resultStyle = item.lastResult ? RESULT_STYLES[item.lastResult] : null;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.SyncJob;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToTaskDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950 font-mono">{formatSchedule(item.schedule)}</div>
                        <div className="text-[9px] text-ink-400">{item.timezone}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950">{item.lastRun ? formatTime(item.lastRun) : "Never"}</div>
                        <div className="text-[9px] text-ink-400">Next: {formatTime(item.nextRun)}</div>
                      </td>
                      <td className="px-4 py-3">
                        {resultStyle ? (
                          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${resultStyle.bg} ${resultStyle.text}`}>
                            {resultStyle.icon}
                            {item.lastResult}
                          </span>
                        ) : (
                          <span className="text-xs text-ink-400">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleToggle(item.id);
                          }}
                          className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold transition ${
                            item.enabled
                              ? "bg-green-bg text-green hover:bg-green-bg/70"
                              : "bg-surface-2 text-ink-500 hover:bg-surface-3"
                          }`}
                        >
                          {item.enabled ? "Enabled" : "Disabled"}
                        </button>
                      </td>
                      <td className="px-4 py-3 text-right flex items-center justify-end gap-1">
                        {item.enabled && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleRunNow(item.id);
                            }}
                            className="text-primary hover:text-primary/80 transition-colors p-1"
                            title="Run Now"
                          >
                            <Play className="h-4 w-4" />
                          </button>
                        )}
                        <button className="text-ink-400 group-hover:text-primary transition-colors p-1">
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
          <CalendarClock className="h-4 w-4" />
          SYS-07 · Scheduled Tasks
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} tasks shown
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}