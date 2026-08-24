// src/app/(dashboard)/settings/roles/[roleId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
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

type RoleDetail = {
  id: string;
  reference: string;
  name: string;
  description: string;
  type: "System" | "Custom";
  scope: "Company-wide" | "Branch" | "Department";
  status: "Active" | "Inactive" | "Archived";
  version: number;
  permissions: {
    id: string;
    name: string;
    category: string;
    description: string;
    enabled: boolean;
  }[];
  assignedUsers: number;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_ROLE_DETAIL: Record<string, RoleDetail> = {
  "R-001": {
    id: "R-001",
    reference: "ROLE-2026-001",
    name: "Organization Admin",
    description: "Full access to all organization settings and data.",
    type: "System",
    scope: "Company-wide",
    status: "Active",
    version: 3,
    permissions: [
      { id: "P-001", name: "Manage Staff", category: "Staff", description: "Create, update, suspend staff members.", enabled: true },
      { id: "P-002", name: "Manage Roles", category: "Roles", description: "Create, update, delete roles.", enabled: true },
      { id: "P-003", name: "Manage Settings", category: "Settings", description: "Update organization settings.", enabled: true },
      { id: "P-004", name: "View All", category: "View", description: "View all data across the organization.", enabled: true },
      { id: "P-005", name: "Approve Requests", category: "Approvals", description: "Approve or reject approval requests.", enabled: true },
    ],
    assignedUsers: 1,
    createdAt: "2026-06-01T09:00:00Z",
    updatedAt: "2026-08-23T10:00:00Z",
    createdBy: "System",
    tags: ["Admin", "System"],
  },
  "R-002": {
    id: "R-002",
    reference: "ROLE-2026-002",
    name: "Operations Manager",
    description: "Manage operations, dispatch, and provider workflows.",
    type: "Custom",
    scope: "Branch",
    status: "Active",
    version: 2,
    permissions: [
      { id: "P-006", name: "Manage Dispatch", category: "Operations", description: "Manage dispatch board and assignments.", enabled: true },
      { id: "P-007", name: "Manage Providers", category: "Providers", description: "Manage provider directory and work setup.", enabled: true },
      { id: "P-008", name: "View Fleet", category: "View", description: "View fleet assets and status.", enabled: true },
      { id: "P-009", name: "View Reports", category: "Reports", description: "View operational reports.", enabled: true },
    ],
    assignedUsers: 2,
    createdAt: "2026-06-15T10:00:00Z",
    updatedAt: "2026-08-22T16:00:00Z",
    createdBy: "F. Daniel",
    tags: ["Operations", "Manager"],
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const SCOPE_LABELS: Record<string, string> = {
  "Company-wide": "Company-wide",
  Branch: "Branch",
  Department: "Department",
};

export default function RoleEditorPage() {
  const params = useParams();
  const router = useRouter();
  const roleId = params.roleId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<RoleDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [isEditing, setIsEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_ROLE_DETAIL[roleId] || MOCK_ROLE_DETAIL["R-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [roleId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading role details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Shield className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Role not found</h3>
        <p className="mt-1 text-sm text-ink-500">No role found with this ID.</p>
        <button
          onClick={() => router.push("/settings/roles")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Roles
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Active;

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

  const handleTogglePermission = (id: string) => {
    setData((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        permissions: prev.permissions.map((p) =>
          p.id === id ? { ...p, enabled: !p.enabled } : p
        ),
      };
    });
  };

  const handleSave = async () => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setIsSaving(false);
    setIsEditing(false);
    toast("Role updated successfully! (mock)");
  };

  const handleBack = () => {
    router.push("/settings/roles");
  };

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/roles")}
              className="hover:text-primary transition-colors"
            >
              Roles
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {data.status}
            </span>
            <span className="text-[10px] text-ink-500">{data.type}</span>
            <span className="text-[10px] text-ink-500">{SCOPE_LABELS[data.scope]}</span>
            <span className="text-[10px] text-ink-500 font-mono">v{data.version}</span>
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
          {data.type === "Custom" && !isEditing && (
            <button
              onClick={() => setIsEditing(true)}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3.5 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit Role
            </button>
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

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <p className="text-sm text-ink-700">{data.description}</p>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-ink-500">
          <span>Created: {formatDateTime(data.createdAt)}</span>
          <span>·</span>
          <span>Updated: {formatTime(data.updatedAt)}</span>
          <span>·</span>
          <span>By: {data.createdBy}</span>
          <span>·</span>
          <span>{data.assignedUsers} users assigned</span>
        </div>
      </div>

      {/* ===== PERMISSIONS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <Shield className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Permissions</span>
            <span className="text-xs text-ink-400">
              {data.permissions.filter((p) => p.enabled).length}/{data.permissions.length} enabled
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {data.permissions.map((perm) => (
            <div
              key={perm.id}
              className={`flex items-center gap-4 p-4 transition ${
                data.type === "System" ? "opacity-60" : "hover:bg-surface-2"
              }`}
            >
              <div className="flex-1 min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-ink-950 text-sm">{perm.name}</span>
                  <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                    {perm.category}
                  </span>
                </div>
                <div className="mt-0.5 text-xs text-ink-500">{perm.description}</div>
              </div>
              <div className="flex-shrink-0">
                {data.type === "System" ? (
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                    perm.enabled ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                  }`}>
                    {perm.enabled ? "Enabled" : "Disabled"}
                  </span>
                ) : isEditing ? (
                  <button
                    onClick={() => handleTogglePermission(perm.id)}
                    className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full transition-colors ${
                      perm.enabled ? "bg-primary" : "bg-ink-300"
                    }`}
                  >
                    <span
                      className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
                        perm.enabled ? "translate-x-5" : "translate-x-0.5"
                      } mt-0.5`}
                    />
                  </button>
                ) : (
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                    perm.enabled ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                  }`}>
                    {perm.enabled ? "Enabled" : "Disabled"}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* ===== SAVE ===== */}
      {isEditing && (
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
                Save Changes
              </>
            )}
          </button>
          <button
            onClick={() => setIsEditing(false)}
            className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
          >
            Cancel
          </button>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          IAM-05 · Role Editor · {data.reference}
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