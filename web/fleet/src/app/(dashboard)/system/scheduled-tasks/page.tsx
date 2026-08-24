// src/app/(dashboard)/system/scheduled-tasks/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  CalendarClock,
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
  Pause,
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
  CalendarClock as CalendarClockIcon,
  Play as PlayIcon,
  Pause as PauseIcon,
  Calendar as CalendarIcon2,
  Clock as ClockIcon3,
  CheckCircle2 as CheckCircle2Icon2,
  AlertCircle as AlertCircleIcon2,
  AlertTriangle as AlertTriangleIcon2,
  Eye as EyeIcon2,
  ArrowRight as ArrowRightIcon2,
  ArrowLeft as ArrowLeftIcon2,
  Users as UsersIcon2,
  Building2 as Building2Icon2,
  Tag as TagIcon2,
  Calendar as CalendarIcon3,
  Info as InfoIcon2,
  ListChecks as ListChecksIcon2,
  Download as DownloadIcon2,
  Printer as PrinterIcon2,
  Edit3 as Edit3Icon2,
  Trash2 as Trash2Icon2,
  Check as CheckIcon2,
  MoreHorizontal as MoreHorizontalIcon2,
  User as UserIcon2,
  Mail as MailIcon2,
  Phone as PhoneIcon2,
  MapPin as MapPinIcon2,
  Briefcase as BriefcaseIcon2,
  Send as SendIcon2,
  Paperclip as PaperclipIcon2,
  Link2 as Link2Icon2,
  ExternalLink as ExternalLinkIcon2,
  FileText as FileTextIcon6,
  File as FileIcon3,
  FileArchive as FileArchiveIcon3,
  FileImage as FileImageIcon3,
  FileJson as FileJsonIcon9,
  FilePdf as FilePdfIcon9,
  FileSpreadsheet as FileSpreadsheetIcon9,
  FolderOpen as FolderOpenIcon3,
  Shield as ShieldIcon3,
  Lock as LockIcon3,
  EyeOff as EyeOffIcon3,
  AlertOctagon as AlertOctagonIcon3,
  FileCheck as FileCheckIcon3,
  FileSignature as FileSignatureIcon3,
  Folder as FolderIcon3,
  HardDrive as HardDriveIcon5,
  Database as DatabaseIcon3,
  Cloud as CloudIcon3,
  History as HistoryIcon3,
  GitBranch as GitBranchIcon3,
  FileUser as FileUserIcon3,
  FileKey as FileKeyIcon3,
  FileClock as FileClockIcon3,
  FileX as FileXIcon3,
  FileMinus as FileMinusIcon3,
  FilePlus as FilePlusIcon3,
  FileCog as FileCogIcon3,
  FileCheck2 as FileCheck2Icon3,
  TrendingUp as TrendingUpIcon3,
  TrendingDown as TrendingDownIcon3,
  Gauge as GaugeIcon3,
  Target as TargetIcon3,
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
  BookOpen as BookOpenIcon3,
  Book as BookIcon3,
  FileText as FileTextIcon7,
  FileSpreadsheet as FileSpreadsheetIcon10,
  FilePdf as FilePdfIcon10,
  FileJson as FileJsonIcon10,
  FileCsv as FileCsvIcon6,
  FileXml as FileXmlIcon6,
  DownloadCloud as DownloadCloudIcon2,
  FileSpreadsheet as FileSpreadsheetIcon11,
  FileJson as FileJsonIcon11,
  FileCsv as FileCsvIcon7,
  FileXml as FileXmlIcon7,
  FilePdf as FilePdfIcon11,
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

const RESULT_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
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
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ScheduledTask[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedResult, setSelectedResult] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

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

  const formatTime = (date: string | null) => {
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

  const formatSchedule = (schedule: string) => {
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