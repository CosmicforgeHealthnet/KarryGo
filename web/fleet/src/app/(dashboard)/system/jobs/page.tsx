// src/app/(dashboard)/system/jobs/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Server,
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
  Play,
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

const STATE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Queued: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Retrying: { bg: "bg-amber-bg", text: "text-amber", icon: <RotateCw className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
  DeadLettered: { bg: "bg-purple-bg", text: "text-purple", icon: <Inbox className="h-3.5 w-3.5" /> },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  Critical: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATE_FILTERS = ["All", "Queued", "Running", "Retrying", "Completed", "Failed", "Cancelled", "DeadLettered"];
const PRIORITY_FILTERS = ["All", "Critical", "High", "Normal", "Low"];

export default function BackgroundJobsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<BackgroundJob[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedState, setSelectedState] = useState("All");
  const [selectedPriority, setSelectedPriority] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

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