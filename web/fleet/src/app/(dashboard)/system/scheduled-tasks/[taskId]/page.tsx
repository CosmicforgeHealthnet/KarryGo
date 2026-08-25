// src/app/(dashboard)/system/scheduled-tasks/[taskId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  CalendarClock,
  RefreshCw,
  ArrowLeft,
  Edit3,
  Pause,
  Play,
  CheckCircle2,
  XCircle,
  History,
  ChevronUp,
  ChevronDown,
  Clock,
} from "lucide-react";

// ==================== TYPES ====================

type ScheduledTaskDetail = {
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
  version: number;
  tags: string[];
  runHistory: {
    id: string;
    startedAt: string;
    completedAt: string | null;
    duration: string | null;
    status: "Success" | "Failed" | "Running";
    error: string | null;
    triggeredBy: "Schedule" | "Manual";
  }[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_TASK_DATA: Record<string, ScheduledTaskDetail> = {
  "TASK-001": {
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
    version: 3,
    tags: ["Sync", "Provider", "Hourly"],
    runHistory: [
      {
        id: "RH-001",
        startedAt: "2026-08-22T14:00:00Z",
        completedAt: "2026-08-22T14:05:00Z",
        duration: "5m 12s",
        status: "Success",
        error: null,
        triggeredBy: "Schedule",
      },
      {
        id: "RH-002",
        startedAt: "2026-08-22T13:00:00Z",
        completedAt: "2026-08-22T13:04:00Z",
        duration: "4m 45s",
        status: "Success",
        error: null,
        triggeredBy: "Schedule",
      },
      {
        id: "RH-003",
        startedAt: "2026-08-22T12:00:00Z",
        completedAt: "2026-08-22T12:08:00Z",
        duration: "8m 22s",
        status: "Failed",
        error: "Connection timeout",
        triggeredBy: "Schedule",
      },
      {
        id: "RH-004",
        startedAt: "2026-08-22T11:00:00Z",
        completedAt: "2026-08-22T11:03:00Z",
        duration: "3m 18s",
        status: "Success",
        error: null,
        triggeredBy: "Schedule",
      },
    ],
    sourceFreshness: "2026-08-22T14:05:00Z",
  },
  "TASK-004": {
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
    version: 2,
    tags: ["Report", "Daily", "Disabled"],
    runHistory: [
      {
        id: "RH-005",
        startedAt: "2026-08-21T06:00:00Z",
        completedAt: null,
        duration: null,
        status: "Failed",
        error: "Report generation timeout after 60 seconds",
        triggeredBy: "Schedule",
      },
      {
        id: "RH-006",
        startedAt: "2026-08-20T06:00:00Z",
        completedAt: "2026-08-20T06:10:00Z",
        duration: "10m 02s",
        status: "Success",
        error: null,
        triggeredBy: "Schedule",
      },
      {
        id: "RH-007",
        startedAt: "2026-08-19T06:00:00Z",
        completedAt: "2026-08-19T06:08:00Z",
        duration: "8m 15s",
        status: "Success",
        error: null,
        triggeredBy: "Schedule",
      },
    ],
    sourceFreshness: "2026-08-21T06:00:00Z",
  },
};

type ResultStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const RESULT_STYLES: Record<string, ResultStyle> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
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

type RunStatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const RUN_STATUS_STYLES: Record<string, RunStatusStyle> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
};

export default function ScheduledTaskDetailPage() {
  const params = useParams();
  const router = useRouter();
  const taskId = params.taskId as string;

  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<ScheduledTaskDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [showHistory, setShowHistory] = useState<boolean>(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_TASK_DATA[taskId] || MOCK_TASK_DATA["TASK-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [taskId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading scheduled task details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <CalendarClock className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Task not found</h3>
        <p className="mt-1 text-sm text-ink-500">No scheduled task found with this ID.</p>
        <button
          onClick={() => router.push("/system/scheduled-tasks")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Tasks
        </button>
      </div>
    );
  }

  const typeStyle = TYPE_STYLES[data.type] || TYPE_STYLES.SyncJob;
  const lastResultStyle = data.lastResult ? RESULT_STYLES[data.lastResult] : null;

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

  const formatDateTime = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const formatSchedule = (schedule: string): string => {
    const parts = schedule.split(" ");
    if (parts.length === 5) {
      return `${parts[0]} ${parts[1]} ${parts[2]} ${parts[3]} ${parts[4]}`;
    }
    return schedule;
  };

  const handleToggle = () => {
    toast(`Toggled task ${data.id} (mock)`);
  };

  const handleRunNow = () => {
    toast(`Running task ${data.id} now (mock)`);
  };

  const handleEdit = () => {
    toast(`Editing task ${data.id} (mock)`);
  };

  const handleBack = () => {
    router.push("/system/scheduled-tasks");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/system/scheduled-tasks")}
              className="hover:text-primary transition-colors"
            >
              Scheduled Tasks
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
              {data.type}
            </span>
            {data.enabled ? (
              <span className="rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">Enabled</span>
            ) : (
              <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-semibold text-ink-500">Disabled</span>
            )}
            {lastResultStyle && (
              <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${lastResultStyle.bg} ${lastResultStyle.text}`}>
                {lastResultStyle.icon}
                Last: {data.lastResult}
              </span>
            )}
            <span className="text-[10px] text-ink-500">v{data.version}</span>
            <span className="text-[10px] text-ink-500">{data.branch}</span>
            {data.tags.map((tag) => (
              <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                {tag}
              </span>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button
            onClick={handleBack}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
          <button
            onClick={handleEdit}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          <button
            onClick={handleToggle}
            className={`flex h-9 items-center gap-1.5 rounded-xl px-3.5 text-[11.5px] font-semibold transition ${
              data.enabled
                ? "border border-red/20 bg-red-bg text-red hover:bg-red/10"
                : "bg-primary text-white shadow-sm shadow-primary/20 hover:bg-primary/90"
            }`}
          >
            {data.enabled ? (
              <>
                <Pause className="h-3.5 w-3.5" />
                Disable
              </>
            ) : (
              <>
                <Play className="h-3.5 w-3.5" />
                Enable
              </>
            )}
          </button>
          {data.enabled && (
            <button
              onClick={handleRunNow}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3.5 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10"
            >
              <Play className="h-3.5 w-3.5" />
              Run Now
            </button>
          )}
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Description</div>
          <div className="mt-1 text-sm text-ink-950">{data.description}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Schedule</div>
          <div className="mt-1 text-sm font-mono font-medium text-ink-950">{formatSchedule(data.schedule)}</div>
          <div className="mt-0.5 text-xs text-ink-500">Timezone: {data.timezone}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Next Run</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{formatDateTime(data.nextRun)}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.runCount} total runs</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Last Run</div>
          <div className="mt-1 text-sm text-ink-950">
            {data.lastRun ? formatDateTime(data.lastRun) : "Never"}
          </div>
          {data.lastResult && (
            <div className="mt-0.5 text-xs text-ink-500">Result: {data.lastResult}</div>
          )}
        </div>
      </div>

      {/* ===== ERROR ===== */}
      {data.lastError && (
        <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red-bg p-2 flex-shrink-0">
              <XCircle className="h-5 w-5 text-red" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-red">Last Error</h4>
              <p className="mt-1 text-sm text-red/80">{data.lastError}</p>
            </div>
          </div>
        </div>
      )}

      {/* ===== RUN HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowHistory(!showHistory)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <History className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Run History</span>
              <span className="ml-2 text-sm text-ink-500">{data.runHistory.length} runs</span>
            </div>
          </div>
          {showHistory ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showHistory && (
          <div className="border-t border-line p-4">
            <div className="relative">
              <div className="absolute left-[11px] top-0 bottom-0 w-0.5 bg-line" />
              <div className="space-y-4">
                {data.runHistory.map((run) => {
                  const runStatusStyle = RUN_STATUS_STYLES[run.status] || RUN_STATUS_STYLES.Failed;
                  return (
                    <div key={run.id} className="flex gap-3">
                      <div className="relative flex-shrink-0">
                        <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                          {runStatusStyle.icon}
                        </div>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-medium ${runStatusStyle.text}`}>
                            {run.status}
                          </span>
                          {run.duration && (
                            <span className="text-xs text-ink-500">· {run.duration}</span>
                          )}
                          <span className="text-xs text-ink-400">
                            ({run.triggeredBy})
                          </span>
                        </div>
                        <div className="text-xs text-ink-500">
                          Started: {formatDateTime(run.startedAt)}
                          {run.completedAt && ` · Completed: ${formatDateTime(run.completedAt)}`}
                        </div>
                        {run.error && (
                          <div className="text-xs text-red-500">{run.error}</div>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <CalendarClock className="h-4 w-4" />
          SYS-08 · Scheduled Task Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}