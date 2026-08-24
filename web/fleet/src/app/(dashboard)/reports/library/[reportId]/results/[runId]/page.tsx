// src/app/(dashboard)/reports/library/[reportId]/results/[runId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  FileText,
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
  FileText as FileTextIcon2,
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
  FileText as FileTextIcon3,
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
  FileText as FileTextIcon4,
  FileSpreadsheet as FileSpreadsheetIcon4,
  FilePdf as FilePdfIcon4,
  FileJson as FileJsonIcon4,
  FileCsv as FileCsvIcon2,
  FileXml as FileXmlIcon2,
  Play as PlayIcon,
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
  FileText as FileTextIcon5,
  File as FileIcon2,
  FileArchive as FileArchiveIcon2,
  FileImage as FileImageIcon2,
  FileJson as FileJsonIcon5,
  FilePdf as FilePdfIcon5,
  FileSpreadsheet as FileSpreadsheetIcon5,
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
  DownloadCloud,
  FileSpreadsheet as FileSpreadsheetIcon6,
  FileJson as FileJsonIcon6,
  FileCsv as FileCsvIcon3,
  FileXml as FileXmlIcon3,
  FilePdf as FilePdfIcon6,
} from "lucide-react";

// ==================== TYPES ====================

type ReportResult = {
  runId: string;
  reportId: string;
  reportName: string;
  format: string;
  generatedAt: string;
  generatedBy: string;
  rowCount: number;
  fileSize: string;
  fileName: string;
  filters: string;
  summary: {
    label: string;
    value: string;
    change?: number;
    trend?: "up" | "down" | "neutral";
  }[];
  data: Record<string, string | number>[];
  columns: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_RESULT_DATA: Record<string, ReportResult> = {
  "latest": {
    runId: "RUN-2026-001",
    reportId: "RPT-001",
    reportName: "Operations Summary",
    format: "PDF",
    generatedAt: "2026-08-22T10:30:00Z",
    generatedBy: "F. Daniel",
    rowCount: 42,
    fileSize: "3.2 MB",
    fileName: "operations_summary_20260822.pdf",
    filters: "Period: This Month · Branch: All branches",
    summary: [
      { label: "Active Work", value: "128", change: 12, trend: "up" },
      { label: "Available Providers", value: "84", change: 91, trend: "up" },
      { label: "Fleet Readiness", value: "96%", change: 88, trend: "neutral" },
      { label: "Open Exceptions", value: "14", change: -3, trend: "down" },
    ],
    columns: ["Metric", "Value", "Change", "Trend", "Branch", "Updated"],
    data: [
      { Metric: "Active Work", Value: "128", Change: "+12%", Trend: "↑", Branch: "Ikeja", Updated: "2 min ago" },
      { Metric: "Active Work", Value: "96", Change: "+8%", Trend: "↑", Branch: "Lekki", Updated: "2 min ago" },
      { Metric: "Available Providers", Value: "84", Change: "+91%", Trend: "↑", Branch: "All", Updated: "2 min ago" },
      { Metric: "Fleet Readiness", Value: "96%", Change: "88%", Trend: "→", Branch: "All", Updated: "2 min ago" },
      { Metric: "Open Exceptions", Value: "14", Change: "-3%", Trend: "↓", Branch: "All", Updated: "2 min ago" },
    ],
    sourceFreshness: "2026-08-22T10:30:00Z",
  },
  "RUN-2026-002": {
    runId: "RUN-2026-002",
    reportId: "RPT-004",
    reportName: "SLA Compliance Report",
    format: "Excel",
    generatedAt: "2026-08-22T09:00:00Z",
    generatedBy: "F. Daniel",
    rowCount: 18,
    fileSize: "1.8 MB",
    fileName: "sla_compliance_20260822.xlsx",
    filters: "Period: Q3 2026 · Customer: All",
    summary: [
      { label: "Total SLAs", value: "18" },
      { label: "MET", value: "9", change: 50, trend: "up" },
      { label: "BREACHED", value: "6", change: 33, trend: "down" },
      { label: "UNKNOWN", value: "3", change: 17, trend: "neutral" },
    ],
    columns: ["Customer", "Metric", "Target", "Actual", "Result", "Period"],
    data: [
      { Customer: "Charitos Hospital", Metric: "On-Time Delivery", Target: "98%", Actual: "96.4%", Result: "BREACHED", Period: "Q3 2026" },
      { Customer: "Charitos Hospital", Metric: "Damage Rate", Target: "<1%", Actual: "0.8%", Result: "MET", Period: "Q3 2026" },
      { Customer: "Oakfield Stores", Metric: "On-Time Delivery", Target: "97%", Actual: "91.2%", Result: "BREACHED", Period: "Q3 2026" },
      { Customer: "Oakfield Stores", Metric: "Customer Satisfaction", Target: "4.5/5", Actual: "4.2/5", Result: "BREACHED", Period: "Q3 2026" },
      { Customer: "Greenline Foods", Metric: "On-Time Delivery", Target: "99%", Actual: "99.1%", Result: "MET", Period: "Q3 2026" },
    ],
    sourceFreshness: "2026-08-22T09:00:00Z",
  },
};

export default function ReportResultPage() {
  const params = useParams();
  const router = useRouter();
  const reportId = params.reportId as string;
  const runId = params.runId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ReportResult | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_RESULT_DATA[runId] || MOCK_RESULT_DATA["latest"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [runId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading report results...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileText className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No results found</h3>
        <p className="mt-1 text-sm text-ink-500">No results available for this report run.</p>
        <button
          onClick={() => router.push(`/reports/library/${reportId}/run`)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Run Report Again
        </button>
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

  const handleDownload = () => {
    toast("Download started (mock)");
  };

  const handleExport = () => {
    toast("Export started (mock)");
  };

  const handleBack = () => {
    router.push(`/reports/library/${reportId}/run`);
  };

  const handleBackToLibrary = () => {
    router.push("/reports/library");
  };

  const getTrendIcon = (trend: string) => {
    if (trend === "up") return <ArrowUp className="h-3.5 w-3.5 text-green" />;
    if (trend === "down") return <ArrowDown className="h-3.5 w-3.5 text-red" />;
    return <Minus className="h-3.5 w-3.5 text-ink-400" />;
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={handleBackToLibrary}
              className="hover:text-primary transition-colors"
            >
              Reports
            </button>
            <span>/</span>
            <button
              onClick={() => router.push(`/reports/library/${reportId}/run`)}
              className="hover:text-primary transition-colors"
            >
              {data.reportName}
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">Results</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.reportName}
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Run ID: {data.runId} · Generated {formatTime(data.generatedAt)} by {data.generatedBy}
          </p>
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
            onClick={handleDownload}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </button>
          <button
            onClick={handleExport}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Share2 className="h-3.5 w-3.5" />
            Export
          </button>
          <button
            onClick={handleBack}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {/* ===== SUMMARY ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {data.summary.map((item, idx) => (
          <div key={idx} className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
            <div className="text-[10px] font-medium text-ink-500">{item.label}</div>
            <div className="mt-1 flex items-center justify-between">
              <div className="text-lg font-bold text-ink-950">{item.value}</div>
              {item.change !== undefined && (
                <div className={`flex items-center gap-0.5 text-xs font-semibold ${
                  item.trend === "up" ? "text-green" :
                  item.trend === "down" ? "text-red" :
                  "text-ink-500"
                }`}>
                  {getTrendIcon(item.trend || "neutral")}
                  {item.change}%
                </div>
              )}
            </div>
          </div>
        ))}
      </div>

      {/* ===== REPORT INFO ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">File</div>
            <div className="mt-1 text-sm font-medium text-ink-950">{data.fileName}</div>
            <div className="text-xs text-ink-500">{data.fileSize} · {data.rowCount} rows</div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Format</div>
            <div className="mt-1 text-sm font-medium text-ink-950">{data.format}</div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Generated</div>
            <div className="mt-1 text-sm font-medium text-ink-950">{formatDateTime(data.generatedAt)}</div>
            <div className="text-xs text-ink-500">by {data.generatedBy}</div>
          </div>
          <div>
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Filters</div>
            <div className="mt-1 text-sm text-ink-950">{data.filters}</div>
          </div>
        </div>
      </div>

      {/* ===== DATA TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Table className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Data</span>
            <span className="text-xs text-ink-500">{data.rowCount} rows · {data.columns.length} columns</span>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                {data.columns.map((col) => (
                  <th key={col} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.data.map((row, idx) => (
                <tr key={idx} className="border-b border-line hover:bg-surface-2 transition-colors">
                  {data.columns.map((col) => (
                    <td key={col} className="px-4 py-3 text-xs text-ink-700">
                      {String(row[col] || "—")}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="p-3 text-center text-xs text-ink-400 border-t border-line">
          Showing {data.data.length} rows · Generated from source data
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileText className="h-4 w-4" />
          RPT-07 · Report Result · {data.runId}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}