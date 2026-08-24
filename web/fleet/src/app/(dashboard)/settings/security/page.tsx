// src/app/(dashboard)/settings/security/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
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

type SecuritySetting = {
  id: string;
  name: string;
  description: string;
  category: "Authentication" | "Session" | "Access" | "Documents" | "Audit" | "Retention";
  value: string | boolean;
  type: "toggle" | "select" | "number";
  options?: string[];
  defaultValue: string | boolean;
  platformMin?: string;
  platformMax?: string;
  isOverride: boolean;
  canOverride: boolean;
};

// ==================== MOCK DATA ====================

const MOCK_SECURITY_SETTINGS: SecuritySetting[] = [
  {
    id: "SS-001",
    name: "Sensitive Action Verification",
    description: "Require fresh verification for high-risk actions.",
    category: "Authentication",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-002",
    name: "Verification Freshness Window",
    description: "How long a verification remains valid.",
    category: "Authentication",
    value: "15 minutes",
    type: "select",
    options: ["5 minutes", "15 minutes", "30 minutes", "1 hour", "2 hours"],
    defaultValue: "15 minutes",
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-003",
    name: "Session Timeout",
    description: "Maximum session idle time before automatic logout.",
    category: "Session",
    value: "8 hours",
    type: "select",
    options: ["1 hour", "4 hours", "8 hours", "12 hours", "24 hours"],
    defaultValue: "8 hours",
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-004",
    name: "Provider Access Policy",
    description: "Allow providers to access organization-managed features.",
    category: "Access",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-005",
    name: "Restricted Document Controls",
    description: "Require additional verification for restricted documents.",
    category: "Documents",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-006",
    name: "Audit Capture",
    description: "Enable detailed audit logging.",
    category: "Audit",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: false,
  },
  {
    id: "SS-007",
    name: "Audit Retention Period",
    description: "How long audit logs are retained.",
    category: "Audit",
    value: "90 days",
    type: "select",
    options: ["30 days", "90 days", "180 days", "365 days", "3 years", "7 years"],
    defaultValue: "90 days",
    isOverride: false,
    canOverride: false,
    platformMin: "90 days",
  },
];

const CATEGORY_COLORS: Record<string, { bg: string; text: string }> = {
  Authentication: { bg: "bg-blue-bg", text: "text-blue" },
  Session: { bg: "bg-purple-bg", text: "text-purple" },
  Access: { bg: "bg-green-bg", text: "text-green" },
  Documents: { bg: "bg-amber-bg", text: "text-amber" },
  Audit: { bg: "bg-red-bg", text: "text-red" },
  Retention: { bg: "bg-surface-2", text: "text-ink-500" },
};

export default function SecuritySettingsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SecuritySetting[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_SECURITY_SETTINGS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleToggle = (id: string) => {
    setData((prev) =>
      prev.map((setting) =>
        setting.id === id && setting.type === "toggle"
          ? { ...setting, value: !setting.value }
          : setting
      )
    );
  };

  const handleSelect = (id: string, value: string) => {
    setData((prev) =>
      prev.map((setting) =>
        setting.id === id && setting.type === "select"
          ? { ...setting, value }
          : setting
      )
    );
  };

  const handleSave = async () => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setIsSaving(false);
    toast("Security settings updated successfully! (mock)");
  };

  const handleReset = () => {
    setData(MOCK_SECURITY_SETTINGS);
    toast("Settings reset to default (mock)");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading security settings...</p>
      </div>
    );
  }

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Security Settings
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Sensitive-action verification requirement, freshness window, session policy, provider access policy, restricted-document controls, audit capture and retention preference.
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
        </div>
      </div>

      {/* ===== SETTINGS LIST ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="divide-y divide-line">
          {data.map((setting) => {
            const categoryStyle = CATEGORY_COLORS[setting.category] || CATEGORY_COLORS.Retention;

            return (
              <div key={setting.id} className="p-4 hover:bg-surface-2 transition-colors">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-ink-950">{setting.name}</span>
                      <span className={`rounded-full px-2 py-0.5 text-[9px] font-medium ${categoryStyle.bg} ${categoryStyle.text}`}>
                        {setting.category}
                      </span>
                      {!setting.canOverride && (
                        <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                          Platform Default
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 text-xs text-ink-500">{setting.description}</div>
                    {setting.platformMin && (
                      <div className="mt-0.5 text-[10px] text-ink-400">
                        Platform minimum: {setting.platformMin}
                      </div>
                    )}
                  </div>
                  <div className="flex-shrink-0">
                    {setting.type === "toggle" ? (
                      <button
                        onClick={() => handleToggle(setting.id)}
                        disabled={!setting.canOverride}
                        className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full transition-colors ${
                          setting.value ? "bg-primary" : "bg-ink-300"
                        } ${!setting.canOverride ? "opacity-50 cursor-not-allowed" : ""}`}
                      >
                        <span
                          className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
                            setting.value ? "translate-x-5" : "translate-x-0.5"
                          } mt-0.5`}
                        />
                      </button>
                    ) : setting.type === "select" ? (
                      <select
                        value={setting.value as string}
                        onChange={(e) => handleSelect(setting.id, e.target.value)}
                        disabled={!setting.canOverride}
                        className={`rounded-xl border border-line px-3 py-1.5 text-sm text-ink-950 outline-none focus:border-primary ${
                          !setting.canOverride ? "opacity-50 cursor-not-allowed" : ""
                        }`}
                      >
                        {setting.options?.map((opt) => (
                          <option key={opt} value={opt}>
                            {opt}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type="number"
                        value={setting.value as number}
                        disabled={!setting.canOverride}
                        className={`rounded-xl border border-line px-3 py-1.5 text-sm text-ink-950 outline-none focus:border-primary ${
                          !setting.canOverride ? "opacity-50 cursor-not-allowed" : ""
                        }`}
                      />
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="flex flex-wrap gap-3">
        <button
          onClick={handleSave}
          disabled={isSaving}
          className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
            isSaving
              ? "bg-ink-400 cursor-not-allowed"
              : "bg-primary shadow-primary/20 hover:bg-primary/90"
          }`}
        >
          {isSaving ? (
            <>
              <RefreshCw className="h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : (
            <>
              <Save className="h-4 w-4" />
              Save Settings
            </>
          )}
        </button>
        <button
          onClick={handleReset}
          className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
        >
          Reset to Defaults
        </button>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          SEC-01 · Security Settings
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}