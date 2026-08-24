// src/app/(dashboard)/system/sync/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
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
  Sync,
  RefreshCw as RefreshCwIcon,
  Clock as ClockIcon2,
  CheckCircle2 as CheckCircle2Icon,
  AlertCircle as AlertCircleIcon,
  AlertTriangle as AlertTriangleIcon,
  Eye as EyeIcon,
  ArrowRight as ArrowRightIcon,
  ArrowLeft as ArrowLeftIcon,
  Users as UsersIcon,
  Building2 as Building2Icon,
  Tag as TagIcon,
  Calendar as CalendarIcon,
  Info as InfoIcon,
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
} from "lucide-react";

// ==================== TYPES ====================

type SyncStatus = {
  type: string;
  label: string;
  status: "Current" | "Gapped" | "Reconciling" | "Stale" | "Error";
  lastSync: string;
  totalRecords: number;
  currentRecords: number;
  gappedRecords: number;
  reconcilingRecords: number;
  nextSync: string | null;
  icon: React.ReactNode;
};

// ==================== MOCK DATA ====================

const MOCK_SYNC_STATUS: SyncStatus[] = [
  {
    type: "Providers",
    label: "Provider Sync",
    status: "Current",
    lastSync: "2026-08-22T14:30:00Z",
    totalRecords: 146,
    currentRecords: 144,
    gappedRecords: 2,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T15:00:00Z",
    icon: <Users className="h-5 w-5" />,
  },
  {
    type: "Trips",
    label: "Trip Sync",
    status: "Gapped",
    lastSync: "2026-08-22T13:00:00Z",
    totalRecords: 128,
    currentRecords: 120,
    gappedRecords: 8,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T14:00:00Z",
    icon: <MapPin className="h-5 w-5" />,
  },
  {
    type: "Assets",
    label: "Asset Sync",
    status: "Current",
    lastSync: "2026-08-22T14:15:00Z",
    totalRecords: 121,
    currentRecords: 119,
    gappedRecords: 2,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T15:15:00Z",
    icon: <Car className="h-5 w-5" />,
  },
  {
    type: "Documents",
    label: "Document Sync",
    status: "Reconciling",
    lastSync: "2026-08-22T12:00:00Z",
    totalRecords: 342,
    currentRecords: 0,
    gappedRecords: 0,
    reconcilingRecords: 12,
    nextSync: null,
    icon: <FileText className="h-5 w-5" />,
  },
  {
    type: "Wallet",
    label: "Wallet Sync",
    status: "Stale",
    lastSync: "2026-08-22T08:00:00Z",
    totalRecords: 0,
    currentRecords: 0,
    gappedRecords: 0,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T16:00:00Z",
    icon: <Wallet className="h-5 w-5" />,
  },
  {
    type: "Finance",
    label: "Finance Sync",
    status: "Error",
    lastSync: "2026-08-22T09:00:00Z",
    totalRecords: 0,
    currentRecords: 0,
    gappedRecords: 0,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T15:00:00Z",
    icon: <Wallet className="h-5 w-5" />,
  },
  {
    type: "Workforce",
    label: "Workforce Sync",
    status: "Current",
    lastSync: "2026-08-22T14:00:00Z",
    totalRecords: 84,
    currentRecords: 82,
    gappedRecords: 2,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T15:00:00Z",
    icon: <Briefcase className="h-5 w-5" />,
  },
  {
    type: "Compliance",
    label: "Compliance Sync",
    status: "Gapped",
    lastSync: "2026-08-22T11:00:00Z",
    totalRecords: 56,
    currentRecords: 50,
    gappedRecords: 6,
    reconcilingRecords: 0,
    nextSync: "2026-08-22T16:00:00Z",
    icon: <Shield className="h-5 w-5" />,
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Current: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Gapped: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  Reconciling: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Stale: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-4 w-4" /> },
  Error: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
};

export default function SynchronizationOverviewPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SyncStatus[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_SYNC_STATUS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading synchronization status...</p>
      </div>
    );
  }

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

  const formatDateTime = (date: string) => {
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  // Stats
  const totalCurrent = data.filter((d) => d.status === "Current").length;
  const totalGapped = data.filter((d) => d.status === "Gapped").length;
  const totalReconciling = data.filter((d) => d.status === "Reconciling").length;
  const totalError = data.filter((d) => d.status === "Error").length;

  const goToReconciliation = () => {
    router.push("/system/reconciliation");
  };

  const goToSyncDetail = (type: string) => {
    router.push(`/system/sync/${type.toLowerCase()}`);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Synchronization Overview
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Projection types, freshness, current/gapped/reconciling counts and latest sync status.
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
          <button
            onClick={goToReconciliation}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <FileCheck2 className="h-3.5 w-3.5" />
            View Reconciliation
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Syncs</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Current</div>
          <div className="text-lg font-bold text-green">{totalCurrent}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Gapped / Reconciling</div>
          <div className="text-lg font-bold text-amber">{totalGapped + totalReconciling}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Error</div>
          <div className="text-lg font-bold text-red">{totalError}</div>
        </div>
      </div>

      {/* ===== SYNC STATUS CARDS ===== */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {data.map((item) => {
          const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Stale;

          return (
            <div
              key={item.type}
              onClick={() => goToSyncDetail(item.type)}
              className="rounded-2xl border border-line bg-white p-4 shadow-sm hover:shadow-md transition-all cursor-pointer group"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <div className={`rounded-full p-2 ${statusStyle.bg}`}>
                    {statusStyle.icon}
                  </div>
                  <div>
                    <div className="font-medium text-ink-950 text-sm">{item.label}</div>
                    <div className="text-[10px] text-ink-500">{item.type}</div>
                  </div>
                </div>
                <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {item.status}
                </span>
              </div>

              <div className="mt-3 grid grid-cols-3 gap-2 border-t border-line pt-3">
                <div className="text-center">
                  <div className="text-xs font-bold text-ink-950">{item.totalRecords}</div>
                  <div className="text-[8px] text-ink-400">Total</div>
                </div>
                <div className="text-center">
                  <div className="text-xs font-bold text-green">{item.currentRecords}</div>
                  <div className="text-[8px] text-ink-400">Current</div>
                </div>
                <div className="text-center">
                  <div className="text-xs font-bold text-amber">{item.gappedRecords}</div>
                  <div className="text-[8px] text-ink-400">Gapped</div>
                </div>
              </div>

              <div className="mt-2 flex items-center justify-between text-[10px] text-ink-400 border-t border-line pt-2">
                <span className="flex items-center gap-1">
                  <Clock className="h-3 w-3" />
                  {formatTime(item.lastSync)}
                </span>
                {item.nextSync && (
                  <span className="flex items-center gap-1">
                    <RefreshCw className="h-3 w-3" />
                    Next: {formatTime(item.nextSync)}
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Sync className="h-4 w-4" />
          SYS-01 · Synchronization Overview
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}