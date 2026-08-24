// src/app/(dashboard)/system/jobs/[jobId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Server,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  ArrowLeft,
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
  FileText,
  File,
  FileArchive,
  FileImage,
  FileJson,
  FilePdf,
  FileSpreadsheet,
  FileText as FileTextIcon,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  Clock as ClockIcon,
  CheckCircle,
  XCircle,
  AlertOctagon,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
  History,
  GitBranch,
  FileUser,
  FileKey,
  FileClock,
  FileX,
  FileMinus,
  FilePlus,
  FileCog,
  FileCheck2,
  TrendingUp,
  TrendingDown,
  Gauge,
  Target,
  Activity,
  BarChart,
  LineChart,
  PieChart,
  Percent,
  Timer,
  Award,
  Sparkles,
  Globe,
  FilterX,
  LayoutGrid,
  LayoutList,
  Sliders,
  ArrowUp,
  ArrowDown,
  Zap,
  Flame,
  Medal,
  Star,
  BookOpen,
  Book,
  FileSpreadsheet as FileSpreadsheetIcon,
  FilePdf as FilePdfIcon,
  FileJson as FileJsonIcon,
  FileCsv,
  FileXml,
  FileSpreadsheet as FileSpreadsheetIcon2,
  FileJson as FileJsonIcon2,
  FilePdf as FilePdfIcon2,
  FileCsv as FileCsvIcon,
  FileXml as FileXmlIcon,
  FileText as FileTextIcon2,
  File as FileIcon,
  FileArchive as FileArchiveIcon,
  FileImage as FileImageIcon,
  FileJson as FileJsonIcon3,
  FilePdf as FilePdfIcon3,
  FileSpreadsheet as FileSpreadsheetIcon3,
  FolderOpen as FolderOpenIcon,
  Shield as ShieldIcon,
  Lock as LockIcon,
  EyeOff as EyeOffIcon,
  AlertOctagon as AlertOctagonIcon,
  FileCheck as FileCheckIcon,
  FileSignature as FileSignatureIcon,
  Folder as FolderIcon,
  HardDrive as HardDriveIcon,
  Database as DatabaseIcon,
  Cloud as CloudIcon,
  History as HistoryIcon,
  GitBranch as GitBranchIcon,
  FileUser as FileUserIcon,
  FileKey as FileKeyIcon,
  FileClock as FileClockIcon,
  FileX as FileXIcon,
  FileMinus as FileMinusIcon,
  FilePlus as FilePlusIcon,
  FileCog as FileCogIcon,
  FileCheck2 as FileCheck2Icon,
  TrendingUp as TrendingUpIcon,
  TrendingDown as TrendingDownIcon,
  Gauge as GaugeIcon,
  Target as TargetIcon,
  Activity as ActivityIcon,
  BarChart as BarChartIcon,
  LineChart as LineChartIcon,
  PieChart as PieChartIcon,
  Percent as PercentIcon,
  Timer as TimerIcon,
  Award as AwardIcon,
  Sparkles as SparklesIcon,
  Globe as GlobeIcon,
  FilterX as FilterXIcon,
  LayoutGrid as LayoutGridIcon,
  LayoutList as LayoutListIcon,
  Sliders as SlidersIcon,
  ArrowUp as ArrowUpIcon,
  ArrowDown as ArrowDownIcon,
  Zap as ZapIcon,
  Flame as FlameIcon,
  Medal as MedalIcon,
  Star as StarIcon,
  BookOpen as BookOpenIcon,
  Book as BookIcon,
  FileText as FileTextIcon3,
  FileSpreadsheet as FileSpreadsheetIcon4,
  FilePdf as FilePdfIcon4,
  FileJson as FileJsonIcon4,
  FileCsv as FileCsvIcon2,
  FileXml as FileXmlIcon2,
  DownloadCloud,
  FileSpreadsheet as FileSpreadsheetIcon5,
  FileJson as FileJsonIcon5,
  FileCsv as FileCsvIcon3,
  FileXml as FileXmlIcon3,
  FilePdf as FilePdfIcon5,
  GitMerge,
  GitPullRequest,
  GitBranch as GitBranchIcon2,
  GitCommit,
  GitPullRequestClosed,
  AlertCircle as AlertCircleIcon,
  CheckCircle as CheckCircleIcon,
  XCircle as XCircleIcon,
  Info as InfoIcon,
  RefreshCw as RefreshCwIcon,
  Clock as ClockIcon2,
  CheckCircle2 as CheckCircle2Icon,
  AlertTriangle as AlertTriangleIcon,
  Eye as EyeIcon,
  ArrowRight as ArrowRightIcon,
  ArrowLeft as ArrowLeftIcon,
  Users as UsersIcon,
  Building2 as Building2Icon,
  Tag as TagIcon,
  Calendar as CalendarIcon,
  ListChecks as ListChecksIcon,
  Download as DownloadIcon,
  Printer as PrinterIcon,
  Edit3 as Edit3Icon,
  Trash2 as Trash2Icon,
  Check as CheckIcon,
  MoreHorizontal as MoreHorizontalIcon,
  User as UserIcon,
  Mail as MailIcon,
  Phone as PhoneIcon,
  MapPin as MapPinIcon,
  Briefcase as BriefcaseIcon,
  Send as SendIcon,
  Paperclip as PaperclipIcon,
  Link2 as Link2Icon,
  ExternalLink as ExternalLinkIcon,
  FileText as FileTextIcon4,
  File as FileIcon2,
  FileArchive as FileArchiveIcon2,
  FileImage as FileImageIcon2,
  FileJson as FileJsonIcon6,
  FilePdf as FilePdfIcon6,
  FileSpreadsheet as FileSpreadsheetIcon6,
  FolderOpen as FolderOpenIcon2,
  Shield as ShieldIcon2,
  Lock as LockIcon2,
  EyeOff as EyeOffIcon2,
  AlertOctagon as AlertOctagonIcon2,
  FileCheck as FileCheckIcon2,
  FileSignature as FileSignatureIcon2,
  Folder as FolderIcon2,
  HardDrive as HardDriveIcon2,
  Database as DatabaseIcon2,
  Cloud as CloudIcon2,
  History as HistoryIcon2,
  GitBranch as GitBranchIcon2,
  FileUser as FileUserIcon2,
  FileKey as FileKeyIcon2,
  FileClock as FileClockIcon2,
  FileX as FileXIcon2,
  FileMinus as FileMinusIcon2,
  FilePlus as FilePlusIcon2,
  FileCog as FileCogIcon2,
  FileCheck2 as FileCheck2Icon2,
  TrendingUp as TrendingUpIcon2,
  TrendingDown as TrendingDownIcon2,
  Gauge as GaugeIcon2,
  Target as TargetIcon2,
  Activity as ActivityIcon2,
  BarChart as BarChartIcon2,
  LineChart as LineChartIcon2,
  PieChart as PieChartIcon2,
  Percent as PercentIcon2,
  Timer as TimerIcon2,
  Award as AwardIcon2,
  Sparkles as SparklesIcon2,
  Globe as GlobeIcon2,
  FilterX as FilterXIcon2,
  LayoutGrid as LayoutGridIcon2,
  LayoutList as LayoutListIcon2,
  Sliders as SlidersIcon2,
  ArrowUp as ArrowUpIcon2,
  ArrowDown as ArrowDownIcon2,
  Zap as ZapIcon2,
  Flame as FlameIcon2,
  Medal as MedalIcon2,
  Star as StarIcon2,
  BookOpen as BookOpenIcon2,
  Book as BookIcon2,
  FileText as FileTextIcon5,
  FileSpreadsheet as FileSpreadsheetIcon7,
  FilePdf as FilePdfIcon7,
  FileJson as FileJsonIcon7,
  FileCsv as FileCsvIcon4,
  FileXml as FileXmlIcon4,
  DownloadCloud as DownloadCloudIcon,
  FileSpreadsheet as FileSpreadsheetIcon8,
  FileJson as FileJsonIcon8,
  FileCsv as FileCsvIcon5,
  FileXml as FileXmlIcon5,
  FilePdf as FilePdfIcon8,
  RotateCw,
  Undo2,
  Redo2,
  Save,
  SkipForward,
  SkipBack,
  FastForward,
  Rewind,
  PlayCircle,
  PauseCircle,
  StopCircle,
  AlertCircle as AlertCircleIcon2,
  CheckCircle as CheckCircleIcon2,
  XCircle as XCircleIcon2,
  Info as InfoIcon2,
  Inbox as InboxIcon,
  MailOpen,
  MailX,
  MailCheck,
  MailWarning,
  Archive,
  ArchiveRestore,
  Trash,
  Trash2 as TrashIcon,
  RotateCw as RotateCwIcon,
  Undo2 as Undo2Icon,
  Redo2 as Redo2Icon,
  Save as SaveIcon,
  SkipForward as SkipForwardIcon,
  SkipBack as SkipBackIcon,
  FastForward as FastForwardIcon,
  Rewind as RewindIcon,
  PlayCircle as PlayCircleIcon,
  PauseCircle as PauseCircleIcon,
  StopCircle as StopCircleIcon,
  Server as ServerIcon,
  Cpu,
  HardDrive as HardDriveIcon2,
  Activity as ActivityIcon2,
  BarChart as BarChartIcon2,
  LineChart as LineChartIcon2,
  PieChart as PieChartIcon2,
  Percent as PercentIcon2,
  Timer as TimerIcon2,
  Award as AwardIcon2,
  Sparkles as SparklesIcon2,
  Globe as GlobeIcon2,
  FilterX as FilterXIcon2,
  LayoutGrid as LayoutGridIcon2,
  LayoutList as LayoutListIcon2,
  Sliders as SlidersIcon2,
  ArrowUp as ArrowUpIcon2,
  ArrowDown as ArrowDownIcon2,
  Zap as ZapIcon2,
  Flame as FlameIcon2,
  Medal as MedalIcon2,
  Star as StarIcon2,
  BookOpen as BookOpenIcon2,
  Book as BookIcon2,
  FileText as FileTextIcon5,
  FileSpreadsheet as FileSpreadsheetIcon7,
  FilePdf as FilePdfIcon7,
  FileJson as FileJsonIcon7,
  FileCsv as FileCsvIcon4,
  FileXml as FileXmlIcon4,
  DownloadCloud as DownloadCloudIcon,
  FileSpreadsheet as FileSpreadsheetIcon8,
  FileJson as FileJsonIcon8,
  FileCsv as FileCsvIcon5,
  FileXml as FileXmlIcon5,
  FilePdf as FilePdfIcon8,
  Cpu as CpuIcon,
  HardDrive as HardDriveIcon3,
  Activity as ActivityIcon3,
  BarChart as BarChartIcon3,
  LineChart as LineChartIcon3,
  PieChart as PieChartIcon3,
  Percent as PercentIcon3,
  Timer as TimerIcon3,
  Award as AwardIcon3,
  Sparkles as SparklesIcon3,
  Globe as GlobeIcon3,
  FilterX as FilterXIcon3,
  LayoutGrid as LayoutGridIcon3,
  LayoutList as LayoutListIcon3,
  Sliders as SlidersIcon3,
  ArrowUp as ArrowUpIcon3,
  ArrowDown as ArrowDownIcon3,
  Zap as ZapIcon3,
  Flame as FlameIcon3,
  Medal as MedalIcon3,
  Star as StarIcon3,
  Server as ServerIcon2,
  Cpu as CpuIcon2,
  HardDrive as HardDriveIcon4,
} from "lucide-react";

// ==================== TYPES ====================

type JobDetail = {
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
  errorClass: string | null;
  stackTrace: string | null;
  branch: string;
  tags: string[];
  attemptHistory: {
    id: string;
    attemptNumber: number;
    startedAt: string;
    completedAt: string | null;
    duration: string | null;
    status: "Running" | "Success" | "Failed" | "Retrying";
    error: string | null;
    performedBy: string;
  }[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_JOB_DATA: Record<string, JobDetail> = {
  "JOB-001": {
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
    errorClass: null,
    stackTrace: null,
    branch: "Ikeja",
    tags: ["Sync", "Provider", "Running"],
    attemptHistory: [
      {
        id: "AH-001",
        attemptNumber: 1,
        startedAt: "2026-08-22T14:28:00Z",
        completedAt: null,
        duration: "2m 14s",
        status: "Running",
        error: null,
        performedBy: "system",
      },
    ],
    sourceFreshness: "2026-08-22T14:30:00Z",
  },
  "JOB-005": {
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
    errorClass: "DeliveryException",
    stackTrace: "at NotificationService.SendEmail(...)\n at JobProcessor.Execute(...)",
    branch: "Ikeja",
    tags: ["Notification", "Failed", "Email"],
    attemptHistory: [
      {
        id: "AH-002",
        attemptNumber: 1,
        startedAt: "2026-08-22T13:30:00Z",
        completedAt: "2026-08-22T13:35:00Z",
        duration: "5m 2s",
        status: "Failed",
        error: "Connection timeout",
        performedBy: "system",
      },
      {
        id: "AH-003",
        attemptNumber: 2,
        startedAt: "2026-08-22T14:00:00Z",
        completedAt: "2026-08-22T14:05:00Z",
        duration: "5m 1s",
        status: "Failed",
        error: "Email server error: 500 Internal Server Error",
        performedBy: "system",
      },
      {
        id: "AH-004",
        attemptNumber: 3,
        startedAt: "2026-08-22T14:25:00Z",
        completedAt: "2026-08-22T14:30:00Z",
        duration: "5m 3s",
        status: "Failed",
        error: "Email server error: 500 Internal Server Error",
        performedBy: "system",
      },
    ],
    sourceFreshness: "2026-08-22T14:30:00Z",
  },
  "JOB-004": {
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
    errorClass: null,
    stackTrace: null,
    branch: "All branches",
    tags: ["Reconciliation", "Wallet", "Completed"],
    attemptHistory: [
      {
        id: "AH-005",
        attemptNumber: 1,
        startedAt: "2026-08-22T11:00:00Z",
        completedAt: "2026-08-22T11:30:00Z",
        duration: "30m 12s",
        status: "Success",
        error: null,
        performedBy: "system",
      },
    ],
    sourceFreshness: "2026-08-22T11:30:00Z",
  },
};

const STATE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Queued: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Retrying: { bg: "bg-amber-bg", text: "text-amber", icon: <RotateCw className="h-4 w-4" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-4 w-4" /> },
  DeadLettered: { bg: "bg-purple-bg", text: "text-purple", icon: <Inbox className="h-4 w-4" /> },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  Critical: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const ATTEMPT_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Retrying: { bg: "bg-amber-bg", text: "text-amber", icon: <RotateCw className="h-4 w-4" /> },
};

export default function JobDetailPage() {
  const params = useParams();
  const router = useRouter();
  const jobId = params.jobId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<JobDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showAttempts, setShowAttempts] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_JOB_DATA[jobId] || MOCK_JOB_DATA["JOB-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [jobId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading job details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Server className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Job not found</h3>
        <p className="mt-1 text-sm text-ink-500">No job found with this ID.</p>
        <button
          onClick={() => router.push("/system/jobs")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Jobs
        </button>
      </div>
    );
  }

  const stateStyle = STATE_STYLES[data.state] || STATE_STYLES.Queued;
  const priorityStyle = PRIORITY_STYLES[data.priority] || PRIORITY_STYLES.Normal;

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

  const formatDateTime = (date: string | null) => {
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

  const handleRetry = () => {
    if (data.state === "Failed" || data.state === "DeadLettered") {
      toast("Job retry requested (mock)");
    } else {
      toast("Job cannot be retried in current state");
    }
  };

  const handleCancel = () => {
    if (data.state === "Queued" || data.state === "Running" || data.state === "Retrying") {
      toast("Job cancellation requested (mock)");
    } else {
      toast("Job cannot be cancelled in current state");
    }
  };

  const handleBack = () => {
    router.push("/system/jobs");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/system/jobs")}
              className="hover:text-primary transition-colors"
            >
              Jobs
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Job Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
              {stateStyle.icon}
              {data.state}
            </span>
            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
              {data.priority}
            </span>
            <span className="text-[10px] text-ink-500">{data.type}</span>
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
          {(data.state === "Failed" || data.state === "DeadLettered") && (
            <button
              onClick={handleRetry}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3.5 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Retry
            </button>
          )}
          {(data.state === "Queued" || data.state === "Running" || data.state === "Retrying") && (
            <button
              onClick={handleCancel}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/20 bg-red-bg px-3.5 text-[11.5px] font-medium text-red shadow-sm transition hover:bg-red/10"
            >
              <X className="h-3.5 w-3.5" />
              Cancel
            </button>
          )}
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Subject</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.subject}</div>
          <div className="mt-0.5 text-xs text-ink-500 font-mono">{data.subjectId}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Attempts</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.attemptCount}/{data.maxAttempts}</div>
          <div className="mt-0.5 text-xs text-ink-500">Next: {data.nextAttempt ? formatTime(data.nextAttempt) : "None"}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Created</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{formatDateTime(data.createdAt)}</div>
          <div className="mt-0.5 text-xs text-ink-500">Age: {data.age}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Completed</div>
          <div className="mt-1 text-sm text-ink-950">
            {data.completedAt ? formatDateTime(data.completedAt) : "—"}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            Updated {formatTime(data.updatedAt)}
          </div>
        </div>
      </div>

      {/* ===== ERROR ===== */}
      {data.error && (
        <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red-bg p-2 flex-shrink-0">
              <XCircle className="h-5 w-5 text-red" />
            </div>
            <div className="flex-1">
              <h4 className="text-sm font-semibold text-red">Error</h4>
              <p className="mt-1 text-sm text-red/80">{data.error}</p>
              {data.errorClass && (
                <div className="mt-1 text-xs text-red/60">Error Class: {data.errorClass}</div>
              )}
              {data.stackTrace && (
                <div className="mt-2 rounded-lg bg-white/50 p-3">
                  <div className="text-xs font-mono text-ink-700 whitespace-pre-wrap">{data.stackTrace}</div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ===== ATTEMPT HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowAttempts(!showAttempts)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <History className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Attempt History</span>
              <span className="ml-2 text-sm text-ink-500">{data.attemptHistory.length} attempts</span>
            </div>
          </div>
          {showAttempts ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showAttempts && (
          <div className="border-t border-line p-4">
            <div className="relative">
              <div className="absolute left-[11px] top-0 bottom-0 w-0.5 bg-line" />
              <div className="space-y-4">
                {data.attemptHistory.map((attempt) => {
                  const attemptStyle = ATTEMPT_STATUS_STYLES[attempt.status] || ATTEMPT_STATUS_STYLES.Failed;
                  return (
                    <div key={attempt.id} className="flex gap-3">
                      <div className="relative flex-shrink-0">
                        <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                          {attemptStyle.icon}
                        </div>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-medium ${attemptStyle.text}`}>
                            Attempt {attempt.attemptNumber} — {attempt.status}
                          </span>
                          {attempt.duration && (
                            <span className="text-xs text-ink-500">· {attempt.duration}</span>
                          )}
                        </div>
                        <div className="text-xs text-ink-500">
                          Started: {formatDateTime(attempt.startedAt)}
                          {attempt.completedAt && ` · Completed: ${formatDateTime(attempt.completedAt)}`}
                        </div>
                        {attempt.error && (
                          <div className="text-xs text-red-500">{attempt.error}</div>
                        )}
                        <div className="text-xs text-ink-400">by {attempt.performedBy}</div>
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
          <Server className="h-4 w-4" />
          SYS-06 · Job Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}