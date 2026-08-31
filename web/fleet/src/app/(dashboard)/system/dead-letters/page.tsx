// src/app/(dashboard)/system/dead-letters/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Download,
  Search,
  Inbox,
  ChevronDown,
  AlertCircle,
  SlidersHorizontal,
  X,
  Clock,
  CheckCircle2,
  EyeOff,
  XCircle,
  AlertTriangle,
  Mail,
  FileX,
  Link2,
  Database,
  Eye,
} from "lucide-react";

// ==================== TYPES ====================

type DeadLetter = {
  id: string;
  reference: string;
  type: string;
  subject: string;
  subjectId: string;
  errorClass: string;
  errorMessage: string;
  stackTrace: string | null;
  age: string;
  createdAt: string;
  attempts: number;
  lastAttemptAt: string | null;
  status: "Pending" | "Retrying" | "Failed" | "Resolved" | "Ignored";
  source: string;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_DEAD_LETTERS: DeadLetter[] = [
  {
    id: "DL-001",
    reference: "DL-2026-001",
    type: "SyncJob",
    subject: "Provider Sync",
    subjectId: "P-044",
    errorClass: "TimeoutException",
    errorMessage: "Connection timeout while syncing provider P-044. Authority service unavailable.",
    stackTrace: null,
    age: "2h",
    createdAt: "2026-08-22T12:00:00Z",
    attempts: 3,
    lastAttemptAt: "2026-08-22T13:30:00Z",
    status: "Pending",
    source: "system/sync",
    branch: "Ikeja",
    tags: ["Sync", "Timeout", "Provider"],
  },
  {
    id: "DL-002",
    reference: "DL-2026-002",
    type: "VerificationJob",
    subject: "Document Verification",
    subjectId: "DOC-005",
    errorClass: "ValidationException",
    errorMessage: "Document validation failed: Checksum mismatch. Local file differs from authority.",
    stackTrace: null,
    age: "1h",
    createdAt: "2026-08-22T13:00:00Z",
    attempts: 2,
    lastAttemptAt: "2026-08-22T14:00:00Z",
    status: "Retrying",
    source: "system/verification",
    branch: "Ikeja",
    tags: ["Verification", "Validation", "Document"],
  },
  {
    id: "DL-003",
    reference: "DL-2026-003",
    type: "ExportJob",
    subject: "Order Export",
    subjectId: "EXP-2026-001",
    errorClass: "DataAccessException",
    errorMessage: "Failed to fetch order data for export. Database connection pool exhausted.",
    stackTrace: null,
    age: "4h",
    createdAt: "2026-08-22T10:00:00Z",
    attempts: 5,
    lastAttemptAt: "2026-08-22T12:00:00Z",
    status: "Failed",
    source: "system/exports",
    branch: "All branches",
    tags: ["Export", "DataAccess", "Database"],
  },
  {
    id: "DL-004",
    reference: "DL-2026-004",
    type: "ReconciliationJob",
    subject: "Wallet Reconciliation",
    subjectId: "WAL-001",
    errorClass: "InvalidStateException",
    errorMessage: "Cannot reconcile wallet: Transaction state mismatch. Local and authority states conflict.",
    stackTrace: null,
    age: "6h",
    createdAt: "2026-08-22T08:00:00Z",
    attempts: 1,
    lastAttemptAt: "2026-08-22T08:30:00Z",
    status: "Pending",
    source: "system/reconciliation",
    branch: "All branches",
    tags: ["Reconciliation", "Wallet", "State"],
  },
  {
    id: "DL-005",
    reference: "DL-2026-005",
    type: "NotificationJob",
    subject: "SLA Breach Notification",
    subjectId: "SLA-001",
    errorClass: "DeliveryException",
    errorMessage: "Failed to deliver notification to recipient. Email server returned 500 error.",
    stackTrace: null,
    age: "30m",
    createdAt: "2026-08-22T14:30:00Z",
    attempts: 2,
    lastAttemptAt: "2026-08-22T14:45:00Z",
    status: "Resolved",
    source: "system/notifications",
    branch: "Ikeja",
    tags: ["Notification", "Email", "Delivery"],
  },
  {
    id: "DL-006",
    reference: "DL-2026-006",
    type: "ImportJob",
    subject: "Bulk Order Import",
    subjectId: "IMP-003",
    errorClass: "FormatException",
    errorMessage: "File format error: Expected column 'order_id' not found in source file.",
    stackTrace: null,
    age: "1d",
    createdAt: "2026-08-21T09:00:00Z",
    attempts: 1,
    lastAttemptAt: "2026-08-21T09:15:00Z",
    status: "Ignored",
    source: "system/imports",
    branch: "Lekki",
    tags: ["Import", "Format", "File"],
  },
  {
    id: "DL-007",
    reference: "DL-2026-007",
    type: "WebhookJob",
    subject: "Order Webhook",
    subjectId: "WH-001",
    errorClass: "HttpException",
    errorMessage: "Webhook delivery failed: Destination endpoint returned 404.",
    stackTrace: null,
    age: "45m",
    createdAt: "2026-08-22T14:15:00Z",
    attempts: 3,
    lastAttemptAt: "2026-08-22T14:45:00Z",
    status: "Pending",
    source: "system/webhooks",
    branch: "All branches",
    tags: ["Webhook", "HTTP", "Delivery"],
  },
  {
    id: "DL-008",
    reference: "DL-2026-008",
    type: "Job",
    subject: "Report Generation",
    subjectId: "RPT-004",
    errorClass: "TimeoutException",
    errorMessage: "Report generation timeout after 60 seconds. Data volume exceeded limits.",
    stackTrace: null,
    age: "3h",
    createdAt: "2026-08-22T11:00:00Z",
    attempts: 2,
    lastAttemptAt: "2026-08-22T11:30:00Z",
    status: "Failed",
    source: "system/jobs",
    branch: "All branches",
    tags: ["Report", "Timeout", "Generation"],
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Retrying: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Ignored: { bg: "bg-surface-2", text: "text-ink-500", icon: <EyeOff className="h-3.5 w-3.5" /> },
};

type ErrorStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const ERROR_CLASS_STYLES: Record<string, ErrorStyle> = {
  TimeoutException: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  ValidationException: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  DataAccessException: { bg: "bg-purple-bg", text: "text-purple", icon: <Database className="h-3.5 w-3.5" /> },
  InvalidStateException: { bg: "bg-red-bg", text: "text-red", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  DeliveryException: { bg: "bg-blue-bg", text: "text-blue", icon: <Mail className="h-3.5 w-3.5" /> },
  FormatException: { bg: "bg-amber-bg", text: "text-amber", icon: <FileX className="h-3.5 w-3.5" /> },
  HttpException: { bg: "bg-red-bg", text: "text-red", icon: <Link2 className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Pending", "Retrying", "Failed", "Resolved", "Ignored"];
const ERROR_CLASS_FILTERS = ["All", "TimeoutException", "ValidationException", "DataAccessException", "InvalidStateException", "DeliveryException", "FormatException", "HttpException"];

export default function DeadLettersPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<DeadLetter[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedErrorClass, setSelectedErrorClass] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DEAD_LETTERS);
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
      item.errorMessage.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesErrorClass = selectedErrorClass === "All" || item.errorClass === selectedErrorClass;

    return matchesSearch && matchesStatus && matchesErrorClass;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading dead letters...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedErrorClass !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedErrorClass("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedErrorClass !== "All" ? 1 : 0);

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
  const totalPending = data.filter((d) => d.status === "Pending" || d.status === "Retrying").length;
  const totalFailed = data.filter((d) => d.status === "Failed").length;
  const totalResolved = data.filter((d) => d.status === "Resolved" || d.status === "Ignored").length;

  const handleRetry = (id: string) => {
    toast(`Retrying dead letter ${id} (mock)`);
  };

  const handleIgnore = (id: string) => {
    toast(`Ignoring dead letter ${id} (mock)`);
  };

  const goToDetail = (id: string) => {
    router.push(`/system/dead-letters/${id}`);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Dead Letters
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Failed asynchronous records with safe type/reference, final error class and age.
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Failed</div>
          <div className="text-lg font-bold text-red">{totalFailed}</div>
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
            placeholder="Search by reference, subject, error..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Retrying", "Failed", "Resolved", "Ignored"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Inbox className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const classes = ["All", "TimeoutException", "ValidationException", "DataAccessException", "InvalidStateException", "DeliveryException", "FormatException", "HttpException"];
            const currentIndex = classes.indexOf(selectedErrorClass);
            const nextIndex = (currentIndex + 1) % classes.length;
            setSelectedErrorClass(classes[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertCircle className="h-4 w-4 text-ink-400" />
          {selectedErrorClass}
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

      {/* ===== DEAD LETTERS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Inbox className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No dead letters found</h3>
          <p className="mt-1 text-sm text-ink-500">No dead letters match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Subject</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Error</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Attempts</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                  const errorStyle = ERROR_CLASS_STYLES[item.errorClass] || ERROR_CLASS_STYLES.TimeoutException;

                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line hover:bg-surface-2 transition-colors group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-mono text-[11px] font-medium text-ink-950">{item.reference}</div>
                        <div className="text-[10px] text-ink-500">{item.type}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs font-medium text-ink-950">{item.subject}</div>
                        <div className="text-[10px] text-ink-400 font-mono">{item.subjectId}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5">
                          <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${errorStyle.bg} ${errorStyle.text}`}>
                            {errorStyle.icon}
                            {item.errorClass}
                          </span>
                        </div>
                        <div className="text-[10px] text-ink-500 truncate max-w-[180px]">{item.errorMessage}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{item.age}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{item.attempts}</td>
                      <td className="px-4 py-3 text-right flex items-center justify-end gap-1">
                        {(item.status === "Pending" || item.status === "Retrying" || item.status === "Failed") && (
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
                        {item.status === "Pending" && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleIgnore(item.id);
                            }}
                            className="text-ink-400 hover:text-ink-600 transition-colors p-1"
                            title="Ignore"
                          >
                            <EyeOff className="h-4 w-4" />
                          </button>
                        )}
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            goToDetail(item.id);
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
          <Inbox className="h-4 w-4" />
          SYS-04 · Dead Letters
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} records shown
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}