// src/app/(dashboard)/settings/audit/[auditId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ClipboardList,
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
  Home,
  UserPlus,
  UserMinus,
  UserCheck as UserCheckIcon2,
  UserX as UserXIcon2,
  Calendar as CalendarIcon,
  Clock as ClockIcon2,
  Save,
  Undo,
  Redo,
  HelpCircle,
  Wallet,
  ShieldCheck as ShieldCheckIcon,
  FileCheck as FileCheckIcon2,
  Briefcase as BriefcaseIcon,
  User as UserIcon,
  Building2 as Building2Icon,
  FileText as FileTextIcon4,
  ListChecks as ListChecksIcon,
  CheckCircle2 as CheckCircle2Icon,
  AlertCircle as AlertCircleIcon,
  AlertTriangle as AlertTriangleIcon,
  RefreshCw as RefreshCwIcon,
  ArrowRight as ArrowRightIcon,
  X as XIcon,
  Settings as SettingsIcon,
  Users as UsersIcon,
  Tag as TagIcon,
  Info as InfoIcon,
  Download as DownloadIcon,
  Printer as PrinterIcon,
  Edit3 as Edit3Icon,
  Trash2 as Trash2Icon,
  MoreHorizontal as MoreHorizontalIcon,
  Mail as MailIcon,
  Phone as PhoneIcon,
  MapPin as MapPinIcon,
  Send as SendIcon,
  Paperclip as PaperclipIcon,
  Link2 as Link2Icon,
  ExternalLink as ExternalLinkIcon,
  FileText as FileTextIcon5,
  File as FileIcon2,
  FileArchive as FileArchiveIcon2,
  FileImage as FileImageIcon2,
  FileJson as FileJsonIcon6,
  FilePdf as FilePdfIcon6,
  FileSpreadsheet as FileSpreadsheetIcon6,
  FolderOpen as FolderOpenIcon2,
  Lock as LockIcon2,
  EyeOff as EyeOffIcon2,
  AlertOctagon as AlertOctagonIcon2,
  FileCheck as FileCheckIcon3,
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
  FileText as FileTextIcon6,
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
  Save as SaveIcon,
  SkipForward,
  SkipBack,
  FastForward,
  Rewind,
  PlayCircle,
  PauseCircle,
  StopCircle,
  Inbox,
  MailOpen,
  MailX,
  MailCheck,
  MailWarning,
  Archive,
  ArchiveRestore,
  Trash,
  Trash2 as TrashIcon,
  Server,
  Cpu,
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
  FileSpreadsheet as FileSpreadsheetIcon9,
  FilePdf as FilePdfIcon9,
  FileJson as FileJsonIcon9,
  FileCsv as FileCsvIcon6,
  FileXml as FileXmlIcon6,
  DownloadCloud as DownloadCloudIcon2,
  FileSpreadsheet as FileSpreadsheetIcon10,
  FileJson as FileJsonIcon10,
  FileCsv as FileCsvIcon7,
  FileXml as FileXmlIcon7,
  FilePdf as FilePdfIcon10,
  Cpu as CpuIcon,
  HardDrive as HardDriveIcon3,
} from "lucide-react";

// ==================== TYPES ====================

type AuditDetail = {
  id: string;
  reference: string;
  actor: {
    id: string;
    name: string;
    email: string;
    role: string;
  };
  action: string;
  resource: string;
  resourceId: string | null;
  branch: string;
  riskFlag: "Low" | "Medium" | "High" | "Critical";
  requestReference: string | null;
  timestamp: string;
  ip: string;
  userAgent: string | null;
  before: Record<string, any> | null;
  after: Record<string, any> | null;
  approvalReference: string | null;
  correlationId: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_AUDIT_DETAIL: Record<string, AuditDetail> = {
  "AUD-002": {
    id: "AUD-002",
    reference: "AUD-2026-002",
    actor: {
      id: "S-001",
      name: "Favour Daniel",
      email: "favour@apexlogistics.com",
      role: "Organization Admin",
    },
    action: "Role Update",
    resource: "Staff: S-005",
    resourceId: "S-005",
    branch: "Ikeja",
    riskFlag: "High",
    requestReference: "APR-2026-001",
    timestamp: "2026-08-22T16:00:00Z",
    ip: "192.168.1.100",
    userAgent: "Chrome/120.0.0.0",
    before: {
      role: "Warehouse Manager",
      scope: "Branch",
      status: "Pending",
    },
    after: {
      role: "Warehouse Manager",
      scope: "Branch",
      status: "Active",
    },
    approvalReference: "AR-001",
    correlationId: "CORR-2026-002",
  },
  "AUD-004": {
    id: "AUD-004",
    reference: "AUD-2026-004",
    actor: {
      id: "S-001",
      name: "Favour Daniel",
      email: "favour@apexlogistics.com",
      role: "Organization Admin",
    },
    action: "Staff Suspension",
    resource: "Staff: S-007",
    resourceId: "S-007",
    branch: "Apapa",
    riskFlag: "Critical",
    requestReference: "AR-004",
    timestamp: "2026-08-22T16:30:00Z",
    ip: "192.168.1.100",
    userAgent: "Chrome/120.0.0.0",
    before: {
      status: "Active",
      lastActive: "2026-08-20T08:00:00Z",
    },
    after: {
      status: "Suspended",
      lastActive: "2026-08-20T08:00:00Z",
    },
    approvalReference: "AR-004",
    correlationId: "CORR-2026-004",
  },
  "AUD-005": {
    id: "AUD-005",
    reference: "AUD-2026-005",
    actor: {
      id: "S-002",
      name: "Oluwaseun Adebayo",
      email: "seun@apexlogistics.com",
      role: "Operations Manager",
    },
    action: "Wallet Withdrawal",
    resource: "₦1,200,000 · UBA ••3901",
    resourceId: null,
    branch: "Ikeja",
    riskFlag: "High",
    requestReference: "APR-2026-002",
    timestamp: "2026-08-23T08:30:00Z",
    ip: "192.168.1.102",
    userAgent: "Chrome/120.0.0.0",
    before: {
      balance: "₦18,420,000",
      status: "Pending Approval",
    },
    after: {
      balance: "₦17,220,000",
      status: "Approved",
    },
    approvalReference: "AR-002",
    correlationId: "CORR-2026-005",
  },
};

const RISK_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Low: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  High: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function AuditDetailPage() {
  const params = useParams();
  const router = useRouter();
  const auditId = params.auditId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<AuditDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showBeforeAfter, setShowBeforeAfter] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_AUDIT_DETAIL[auditId] || MOCK_AUDIT_DETAIL["AUD-002"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [auditId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading audit details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <ClipboardList className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Audit event not found</h3>
        <p className="mt-1 text-sm text-ink-500">No audit event found with this ID.</p>
        <button
          onClick={() => router.push("/settings/audit")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Audit Log
        </button>
      </div>
    );
  }

  const riskStyle = RISK_STYLES[data.riskFlag] || RISK_STYLES.Low;

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

  const handleBack = () => {
    router.push("/settings/audit");
  };

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/audit")}
              className="hover:text-primary transition-colors"
            >
              Audit Log
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Audit Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${riskStyle.bg} ${riskStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${riskStyle.dot}`} />
              {data.riskFlag}
            </span>
            <span className="text-[10px] text-ink-500">{data.action}</span>
            <span className="text-[10px] text-ink-500">{data.branch}</span>
            {data.approvalReference && (
              <span className="text-[10px] text-blue font-medium">Approval: {data.approvalReference}</span>
            )}
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
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Actor</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.actor.name}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.actor.email}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.actor.role}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Action & Resource</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.action}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.resource}</div>
          {data.resourceId && (
            <div className="mt-0.5 text-xs text-ink-400 font-mono">{data.resourceId}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Request Reference</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.requestReference || "—"}
          </div>
          {data.correlationId && (
            <div className="mt-0.5 text-xs text-ink-400 font-mono">Correlation: {data.correlationId}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Timestamp & IP</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{formatDateTime(data.timestamp)}</div>
          <div className="mt-0.5 text-xs text-ink-500">IP: {data.ip}</div>
          {data.userAgent && (
            <div className="mt-0.5 text-xs text-ink-400 truncate">{data.userAgent}</div>
          )}
        </div>
      </div>

      {/* ===== BEFORE / AFTER ===== */}
      {data.before && data.after && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <button
            onClick={() => setShowBeforeAfter(!showBeforeAfter)}
            className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="rounded-full bg-blue-bg p-2">
                <GitBranch className="h-4 w-4 text-blue" />
              </div>
              <div>
                <span className="font-semibold text-ink-950">Before / After</span>
                <span className="ml-2 text-sm text-ink-500">State changes</span>
              </div>
            </div>
            {showBeforeAfter ? (
              <ChevronUp className="h-4 w-4 text-ink-400" />
            ) : (
              <ChevronDown className="h-4 w-4 text-ink-400" />
            )}
          </button>

          {showBeforeAfter && (
            <div className="border-t border-line p-4">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="rounded-xl border border-line bg-red-bg/10 p-4">
                  <div className="text-xs font-medium text-red uppercase tracking-[0.06em]">Before</div>
                  <div className="mt-2 space-y-1.5">
                    {Object.entries(data.before).map(([key, value]) => (
                      <div key={key} className="flex justify-between text-sm">
                        <span className="text-ink-500">{key}:</span>
                        <span className="font-medium text-ink-950">{String(value)}</span>
                      </div>
                    ))}
                  </div>
                </div>
                <div className="rounded-xl border border-line bg-green-bg/10 p-4">
                  <div className="text-xs font-medium text-green uppercase tracking-[0.06em]">After</div>
                  <div className="mt-2 space-y-1.5">
                    {Object.entries(data.after).map(([key, value]) => (
                      <div key={key} className="flex justify-between text-sm">
                        <span className="text-ink-500">{key}:</span>
                        <span className="font-medium text-ink-950">{String(value)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <ClipboardList className="h-4 w-4" />
          SEC-03 · Audit Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}