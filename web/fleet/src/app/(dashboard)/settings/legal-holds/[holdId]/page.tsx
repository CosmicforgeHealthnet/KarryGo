// src/app/(dashboard)/settings/legal-holds/[holdId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Scale,
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
  Scale as ScaleIcon,
  Gavel,
  BookOpen as BookOpenIcon2,
  FileCheck as FileCheckIcon4,
  AlertCircle as AlertCircleIcon2,
  CheckCircle2 as CheckCircle2Icon2,
  XCircle,
  X as XIcon2,
} from "lucide-react";

// ==================== TYPES ====================

type LegalHoldDetail = {
  id: string;
  reference: string;
  scopeType: "Document" | "Staff" | "Provider" | "Asset" | "Trip" | "Order" | "All";
  scopeId: string | null;
  scopeName: string | null;
  reason: string;
  description: string | null;
  status: "Active" | "Released" | "Expired";
  creator: {
    id: string;
    name: string;
    email: string;
  };
  approver: {
    id: string;
    name: string;
    email: string;
  } | null;
  createdAt: string;
  releasedAt: string | null;
  releasedBy: string | null;
  releaseReason: string | null;
  affectedRetention: string;
  version: number;
  history: {
    id: string;
    action: "Created" | "Approved" | "Released" | "Expired" | "Updated";
    performedBy: string;
    timestamp: string;
    notes: string | null;
  }[];
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_HOLD_DETAIL: Record<string, LegalHoldDetail> = {
  "LH-001": {
    id: "LH-001",
    reference: "LH-2026-001",
    scopeType: "Document",
    scopeId: "DOC-007",
    scopeName: "Financial Report Q3 2026",
    reason: "Under audit investigation",
    description: "Financial report requires preservation for ongoing audit. All related records must be retained.",
    status: "Active",
    creator: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    approver: { id: "S-006", name: "Esther Obi", email: "esther@apexlogistics.com" },
    createdAt: "2026-08-20T09:00:00Z",
    releasedAt: null,
    releasedBy: null,
    releaseReason: null,
    affectedRetention: "7 years",
    version: 1,
    history: [
      {
        id: "H-001",
        action: "Created",
        performedBy: "Favour Daniel",
        timestamp: "2026-08-20T09:00:00Z",
        notes: "Legal hold created for audit investigation.",
      },
      {
        id: "H-002",
        action: "Approved",
        performedBy: "Esther Obi",
        timestamp: "2026-08-20T10:00:00Z",
        notes: "Hold approved. Compliance team notified.",
      },
    ],
    branch: "Ikeja",
    tags: ["Audit", "Financial"],
  },
  "LH-002": {
    id: "LH-002",
    reference: "LH-2026-002",
    scopeType: "Staff",
    scopeId: "S-007",
    scopeName: "Kabiru Ahmed",
    reason: "Employment dispute investigation",
    description: "Staff records related to employment dispute must be preserved pending investigation outcome.",
    status: "Active",
    creator: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    approver: { id: "S-006", name: "Esther Obi", email: "esther@apexlogistics.com" },
    createdAt: "2026-08-22T10:00:00Z",
    releasedAt: null,
    releasedBy: null,
    releaseReason: null,
    affectedRetention: "5 years",
    version: 1,
    history: [
      {
        id: "H-003",
        action: "Created",
        performedBy: "Favour Daniel",
        timestamp: "2026-08-22T10:00:00Z",
        notes: "Hold created for employment dispute.",
      },
    ],
    branch: "Apapa",
    tags: ["Staff", "Dispute"],
  },
  "LH-004": {
    id: "LH-004",
    reference: "LH-2026-004",
    scopeType: "All",
    scopeId: null,
    scopeName: "All Organization Data",
    reason: "Regulatory compliance review",
    description: "Full data preservation for regulatory review. All records must be retained.",
    status: "Released",
    creator: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    approver: { id: "S-006", name: "Esther Obi", email: "esther@apexlogistics.com" },
    createdAt: "2026-07-01T09:00:00Z",
    releasedAt: "2026-08-15T10:00:00Z",
    releasedBy: "F. Daniel",
    releaseReason: "Review completed, no further holds required",
    affectedRetention: "Indefinite",
    version: 2,
    history: [
      {
        id: "H-004",
        action: "Created",
        performedBy: "Favour Daniel",
        timestamp: "2026-07-01T09:00:00Z",
        notes: "Full data hold for regulatory review.",
      },
      {
        id: "H-005",
        action: "Approved",
        performedBy: "Esther Obi",
        timestamp: "2026-07-01T10:00:00Z",
        notes: "Hold approved.",
      },
      {
        id: "H-006",
        action: "Released",
        performedBy: "F. Daniel",
        timestamp: "2026-08-15T10:00:00Z",
        notes: "Review completed. Hold released.",
      },
    ],
    branch: "All branches",
    tags: ["Regulatory", "Compliance"],
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Released: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const SCOPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Document: { bg: "bg-blue-bg", text: "text-blue", icon: <FileText className="h-4 w-4" /> },
  Staff: { bg: "bg-purple-bg", text: "text-purple", icon: <User className="h-4 w-4" /> },
  Provider: { bg: "bg-amber-bg", text: "text-amber", icon: <Users className="h-4 w-4" /> },
  Asset: { bg: "bg-green-bg", text: "text-green", icon: <Car className="h-4 w-4" /> },
  Trip: { bg: "bg-blue-bg", text: "text-blue", icon: <MapPin className="h-4 w-4" /> },
  Order: { bg: "bg-green-bg", text: "text-green", icon: <Briefcase className="h-4 w-4" /> },
  All: { bg: "bg-red-bg", text: "text-red", icon: <Globe className="h-4 w-4" /> },
};

const ACTION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Created: { bg: "bg-blue-bg", text: "text-blue", icon: <Plus className="h-4 w-4" /> },
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Released: { bg: "bg-amber-bg", text: "text-amber", icon: <Check className="h-4 w-4" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-4 w-4" /> },
  Updated: { bg: "bg-blue-bg", text: "text-blue", icon: <Edit3 className="h-4 w-4" /> },
};

export default function LegalHoldDetailPage() {
  const params = useParams();
  const router = useRouter();
  const holdId = params.holdId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<LegalHoldDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showHistory, setShowHistory] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_HOLD_DETAIL[holdId] || MOCK_HOLD_DETAIL["LH-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [holdId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading legal hold details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Scale className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Legal hold not found</h3>
        <p className="mt-1 text-sm text-ink-500">No legal hold found with this ID.</p>
        <button
          onClick={() => router.push("/settings/legal-holds")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Legal Holds
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Active;
  const scopeStyle = SCOPE_STYLES[data.scopeType] || SCOPE_STYLES.All;

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

  const handleRelease = () => {
    toast("Legal hold released (mock)");
  };

  const handleEdit = () => {
    toast("Edit legal hold (mock)");
  };

  const handleBack = () => {
    router.push("/settings/legal-holds");
  };

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/legal-holds")}
              className="hover:text-primary transition-colors"
            >
              Legal Holds
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Legal Hold Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {data.status}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${scopeStyle.bg} ${scopeStyle.text}`}>
              {scopeStyle.icon}
              {data.scopeType}
            </span>
            <span className="text-[10px] text-ink-500 font-mono">v{data.version}</span>
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
          {data.status === "Active" && (
            <>
              <button
                onClick={handleRelease}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
              >
                <CheckCircle2 className="h-3.5 w-3.5" />
                Release Hold
              </button>
              <button
                onClick={handleEdit}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3.5 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10"
              >
                <Edit3 className="h-3.5 w-3.5" />
                Edit
              </button>
            </>
          )}
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Scope</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.scopeName || "All Data"}</div>
          {data.scopeId && (
            <div className="mt-0.5 text-xs text-ink-500 font-mono">{data.scopeId}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reason</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.reason}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Creator / Approver</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.creator.name}</div>
          <div className="mt-0.5 text-xs text-ink-500">
            {data.approver ? `Approved by: ${data.approver.name}` : "Awaiting approval"}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Retention</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.affectedRetention}</div>
          <div className="mt-0.5 text-xs text-ink-500">Created {formatTime(data.createdAt)}</div>
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      {data.description && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
          <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
        </div>
      )}

      {/* ===== RELEASE INFO ===== */}
      {data.status === "Released" && data.releasedAt && (
        <div className="rounded-2xl border border-green/20 bg-green-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-green-bg p-2 flex-shrink-0">
              <CheckCircle2 className="h-5 w-5 text-green" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-green">Hold Released</h4>
              <p className="mt-1 text-sm text-green/80">
                Released {formatTime(data.releasedAt)} by {data.releasedBy}
              </p>
              {data.releaseReason && (
                <p className="mt-1 text-sm text-green/80">{data.releaseReason}</p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ===== HISTORY ===== */}
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
              <span className="font-semibold text-ink-950">History</span>
              <span className="ml-2 text-sm text-ink-500">{data.history.length} events</span>
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
                {data.history.map((event) => {
                  const actionStyle = ACTION_STYLES[event.action] || ACTION_STYLES.Created;

                  return (
                    <div key={event.id} className="flex gap-3">
                      <div className="relative flex-shrink-0">
                        <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                          {actionStyle.icon}
                        </div>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-medium ${actionStyle.text}`}>
                            {event.action}
                          </span>
                          <span className="text-xs text-ink-500">· {formatTime(event.timestamp)}</span>
                        </div>
                        <div className="text-xs text-ink-500">by {event.performedBy}</div>
                        {event.notes && (
                          <div className="text-xs text-ink-600 mt-0.5">{event.notes}</div>
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
          <Scale className="h-4 w-4" />
          SEC-06 · Legal Hold Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · v{data.version}
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}