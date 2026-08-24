// src/app/(dashboard)/settings/retention/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Database,
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
  Edit3,
  Save,
  Users,
  Building2,
  Tag,
  Calendar,
  Info,
  ListChecks,
  Download,
  Printer,
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
  Save as SaveIcon,
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
  Save as SaveIcon2,
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

type RetentionClass = {
  id: string;
  name: string;
  description: string;
  platformMin: string;
  companyEffective: string;
  overrideStatus: "Default" | "Extended";
  canOverride: boolean;
  recordsAffected: number;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_RETENTION_CLASSES: RetentionClass[] = [
  {
    id: "RC-001",
    name: "Regulatory",
    description: "Records required for regulatory compliance.",
    platformMin: "5 years",
    companyEffective: "7 years",
    overrideStatus: "Extended",
    canOverride: true,
    recordsAffected: 1243,
    tags: ["Regulatory", "Compliance"],
  },
  {
    id: "RC-002",
    name: "Financial",
    description: "Financial and accounting records.",
    platformMin: "5 years",
    companyEffective: "5 years",
    overrideStatus: "Default",
    canOverride: true,
    recordsAffected: 892,
    tags: ["Financial", "Accounting"],
  },
  {
    id: "RC-003",
    name: "Operational",
    description: "Operational logs and trip records.",
    platformMin: "3 years",
    companyEffective: "3 years",
    overrideStatus: "Default",
    canOverride: true,
    recordsAffected: 5671,
    tags: ["Operational", "Logs"],
  },
  {
    id: "RC-004",
    name: "Staff",
    description: "Staff and employment records.",
    platformMin: "3 years",
    companyEffective: "3 years",
    overrideStatus: "Default",
    canOverride: true,
    recordsAffected: 234,
    tags: ["Staff", "HR"],
  },
  {
    id: "RC-005",
    name: "Provider",
    description: "Provider and contractor records.",
    platformMin: "3 years",
    companyEffective: "5 years",
    overrideStatus: "Extended",
    canOverride: true,
    recordsAffected: 156,
    tags: ["Provider", "Contractor"],
  },
  {
    id: "RC-006",
    name: "Audit",
    description: "Audit and compliance review records.",
    platformMin: "7 years",
    companyEffective: "7 years",
    overrideStatus: "Default",
    canOverride: false,
    recordsAffected: 456,
    tags: ["Audit", "Compliance"],
  },
];

const OVERRIDE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Default: { bg: "bg-surface-2", text: "text-ink-500", icon: <Check className="h-3.5 w-3.5" /> },
  Extended: { bg: "bg-amber-bg", text: "text-amber", icon: <ArrowUp className="h-3.5 w-3.5" /> },
};

export default function RetentionPoliciesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<RetentionClass[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editValue, setEditValue] = useState<string>("");
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_RETENTION_CLASSES);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleEdit = (id: string, currentValue: string) => {
    setEditingId(id);
    setEditValue(currentValue);
  };

  const handleSave = async (id: string) => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 800));
    setData((prev) =>
      prev.map((item) =>
        item.id === id
          ? { ...item, companyEffective: editValue, overrideStatus: editValue === item.platformMin ? "Default" : "Extended" }
          : item
      )
    );
    setEditingId(null);
    setEditValue("");
    setIsSaving(false);
    toast("Retention policy updated (mock)");
  };

  const handleCancel = () => {
    setEditingId(null);
    setEditValue("");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading retention policies...</p>
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

  const goToRunSetup = () => {
    router.push("/settings/retention/run");
  };

  // Stats
  const extendedCount = data.filter((d) => d.overrideStatus === "Extended").length;
  const totalRecords = data.reduce((acc, d) => acc + d.recordsAffected, 0);

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Retention Policies
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Retention classes, platform minimum, company effective duration and override status.
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
            onClick={goToRunSetup}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Play className="h-3.5 w-3.5" />
            Run Retention
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Classes</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Extended</div>
          <div className="text-lg font-bold text-amber">{extendedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Records Affected</div>
          <div className="text-lg font-bold text-ink-950">{totalRecords.toLocaleString()}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Platform Defaults</div>
          <div className="text-lg font-bold text-green">{data.length - extendedCount}</div>
        </div>
      </div>

      {/* ===== POLICIES TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Class</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Description</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Platform Min</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Company Effective</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Records</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
              </tr>
            </thead>
            <tbody>
              {data.map((item) => {
                const overrideStyle = OVERRIDE_STYLES[item.overrideStatus] || OVERRIDE_STYLES.Default;

                return (
                  <tr key={item.id} className="border-b border-line hover:bg-surface-2 transition-colors">
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950">{item.name}</div>
                      {item.tags.map((tag) => (
                        <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500 mr-1">
                          {tag}
                        </span>
                      ))}
                    </td>
                    <td className="px-4 py-3 text-ink-600 text-xs">{item.description}</td>
                    <td className="px-4 py-3 text-ink-600 text-xs font-mono">{item.platformMin}</td>
                    <td className="px-4 py-3">
                      {editingId === item.id ? (
                        <div className="flex items-center gap-2">
                          <input
                            type="text"
                            value={editValue}
                            onChange={(e) => setEditValue(e.target.value)}
                            className="rounded-lg border border-line px-2 py-1 text-sm text-ink-950 outline-none focus:border-primary w-24"
                          />
                          <button
                            onClick={() => handleSave(item.id)}
                            disabled={isSaving}
                            className="text-primary hover:text-primary/80 transition-colors"
                          >
                            <Check className="h-4 w-4" />
                          </button>
                          <button
                            onClick={handleCancel}
                            className="text-ink-400 hover:text-ink-600 transition-colors"
                          >
                            <X className="h-4 w-4" />
                          </button>
                        </div>
                      ) : (
                        <span className="font-medium text-ink-950">{item.companyEffective}</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${overrideStyle.bg} ${overrideStyle.text}`}>
                        {overrideStyle.icon}
                        {item.overrideStatus}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-ink-600 text-xs">{item.recordsAffected.toLocaleString()}</td>
                    <td className="px-4 py-3 text-right">
                      {item.canOverride && editingId !== item.id && (
                        <button
                          onClick={() => handleEdit(item.id, item.companyEffective)}
                          className="text-ink-400 hover:text-primary transition-colors"
                        >
                          <Edit3 className="h-4 w-4" />
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== NOTE ===== */}
      <div className="rounded-xl border border-amber/20 bg-amber-bg/30 p-4">
        <div className="flex items-start gap-2">
          <AlertCircle className="h-4 w-4 text-amber flex-shrink-0 mt-0.5" />
          <p className="text-xs text-amber/80">
            <span className="font-semibold">Note:</span> Company may extend but not shorten platform minimum retention periods. Legal holds override retention policies.
          </p>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Database className="h-4 w-4" />
          SEC-07 · Retention Policies
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.length} classes
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}