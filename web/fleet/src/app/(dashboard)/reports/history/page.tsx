// src/app/(dashboard)/reports/history/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  History,
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
  History as HistoryIcon,
} from "lucide-react";

// ==================== TYPES ====================

type HistoryItem = {
  id: string;
  reference: string;
  type: "Report" | "Export";
  name: string;
  format: string;
  status: "Completed" | "Failed" | "Expired" | "Generating";
  requester: string;
  createdAt: string;
  completedAt: string | null;
  expiresAt: string | null;
  fileSize: string | null;
  fileName: string | null;
  rowCount: number | null;
  downloadCount: number;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_HISTORY: HistoryItem[] = [
  {
    id: "H-001",
    reference: "RPT-2026-001",
    type: "Report",
    name: "Operations Summary",
    format: "PDF",
    status: "Completed",
    requester: "F. Daniel",
    createdAt: "2026-08-22T10:30:00Z",
    completedAt: "2026-08-22T10:35:00Z",
    expiresAt: "2026-08-29T10:35:00Z",
    fileSize: "3.2 MB",
    fileName: "operations_summary_20260822.pdf",
    rowCount: 42,
    downloadCount: 0,
    branch: "All branches",
    tags: ["Operations", "Daily"],
  },
  {
    id: "H-002",
    reference: "EXP-2026-001",
    type: "Export",
    name: "Orders Export",
    format: "CSV",
    status: "Completed",
    requester: "F. Daniel",
    createdAt: "2026-08-22T10:00:00Z",
    completedAt: "2026-08-22T10:15:00Z",
    expiresAt: "2026-08-29T10:15:00Z",
    fileSize: "12.4 MB",
    fileName: "orders_export_20260822.csv",
    rowCount: 248,
    downloadCount: 0,
    branch: "Ikeja",
    tags: ["Orders", "Export"],
  },
  {
    id: "H-003",
    reference: "RPT-2026-004",
    type: "Report",
    name: "SLA Compliance Report",
    format: "Excel",
    status: "Completed",
    requester: "F. Daniel",
    createdAt: "2026-08-22T09:00:00Z",
    completedAt: "2026-08-22T09:20:00Z",
    expiresAt: "2026-08-29T09:20:00Z",
    fileSize: "1.8 MB",
    fileName: "sla_compliance_20260822.xlsx",
    rowCount: 18,
    downloadCount: 0,
    branch: "All branches",
    tags: ["SLA", "Compliance"],
  },
  {
    id: "H-004",
    reference: "EXP-2026-002",
    type: "Export",
    name: "Inventory Export",
    format: "Excel",
    status: "Completed",
    requester: "M. Bello",
    createdAt: "2026-08-22T09:00:00Z",
    completedAt: "2026-08-22T09:20:00Z",
    expiresAt: "2026-08-29T09:20:00Z",
    fileSize: "18.7 MB",
    fileName: "inventory_export_20260821.xlsx",
    rowCount: 1180,
    downloadCount: 3,
    branch: "Lekki",
    tags: ["Inventory", "Export"],
  },
  {
    id: "H-005",
    reference: "RPT-2026-002",
    type: "Report",
    name: "Fleet Utilization",
    format: "Excel",
    status: "Completed",
    requester: "M. Bello",
    createdAt: "2026-08-21T14:00:00Z",
    completedAt: "2026-08-21T14:15:00Z",
    expiresAt: "2026-08-28T14:15:00Z",
    fileSize: "4.6 MB",
    fileName: "fleet_utilization_20260821.xlsx",
    rowCount: 87,
    downloadCount: 2,
    branch: "All branches",
    tags: ["Fleet", "Utilization"],
  },
  {
    id: "H-006",
    reference: "EXP-2026-004",
    type: "Export",
    name: "Trips Export",
    format: "Excel",
    status: "Failed",
    requester: "K. Ahmed",
    createdAt: "2026-08-22T08:00:00Z",
    completedAt: null,
    expiresAt: null,
    fileSize: null,
    fileName: null,
    rowCount: null,
    downloadCount: 0,
    branch: "Apapa",
    tags: ["Trips", "Failed"],
  },
  {
    id: "H-007",
    reference: "RPT-2026-003",
    type: "Report",
    name: "Workforce Performance",
    format: "PDF",
    status: "Completed",
    requester: "O. Adebayo",
    createdAt: "2026-08-20T10:00:00Z",
    completedAt: "2026-08-20T10:10:00Z",
    expiresAt: "2026-08-27T10:10:00Z",
    fileSize: "2.1 MB",
    fileName: "workforce_performance_20260820.pdf",
    rowCount: 56,
    downloadCount: 1,
    branch: "All branches",
    tags: ["Workforce", "Performance"],
  },
  {
    id: "H-008",
    reference: "EXP-2026-006",
    type: "Export",
    name: "SLA Report Export",
    format: "PDF",
    status: "Expired",
    requester: "O. Adebayo",
    createdAt: "2026-08-15T14:00:00Z",
    completedAt: "2026-08-15T14:30:00Z",
    expiresAt: "2026-08-22T14:30:00Z",
    fileSize: "8.2 MB",
    fileName: "sla_report_q3_2026.pdf",
    rowCount: 42,
    downloadCount: 0,
    branch: "All branches",
    tags: ["Reports", "SLA", "Expired"],
  },
  {
    id: "H-009",
    reference: "RPT-2026-005",
    type: "Report",
    name: "Financial Summary",
    format: "Excel",
    status: "Generating",
    requester: "F. Daniel",
    createdAt: "2026-08-22T11:00:00Z",
    completedAt: null,
    expiresAt: null,
    fileSize: null,
    fileName: null,
    rowCount: null,
    downloadCount: 0,
    branch: "All branches",
    tags: ["Finance", "Generating"],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-3.5 w-3.5" /> },
  Generating: { bg: "bg-amber-bg", text: "text-amber", icon: <RefreshCw className="h-3.5 w-3.5" /> },
};

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Report: { bg: "bg-blue-bg", text: "text-blue", icon: <FileText className="h-3.5 w-3.5" /> },
  Export: { bg: "bg-purple-bg", text: "text-purple", icon: <DownloadCloud className="h-3.5 w-3.5" /> },
};

const FORMAT_ICONS: Record<string, React.ReactNode> = {
  PDF: <FilePdfIcon className="h-3.5 w-3.5 text-red" />,
  Excel: <FileSpreadsheetIcon className="h-3.5 w-3.5 text-green" />,
  CSV: <FileCsvIcon className="h-3.5 w-3.5 text-blue" />,
  JSON: <FileJsonIcon className="h-3.5 w-3.5 text-amber" />,
  HTML: <FileTextIcon className="h-3.5 w-3.5 text-purple" />,
};

const TYPE_FILTERS = ["All", "Report", "Export"];
const STATUS_FILTERS = ["All", "Completed", "Failed", "Expired", "Generating"];
const BRANCH_FILTERS = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ReportHistoryPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<HistoryItem[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_HISTORY);
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
      item.requester.toLowerCase().includes(search.toLowerCase()) ||
      item.tags.some(t => t.toLowerCase().includes(search.toLowerCase()));

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesType && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading history...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedType !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedType !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

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

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  // Stats
  const totalCompleted = data.filter((d) => d.status === "Completed").length;
  const totalFailed = data.filter((d) => d.status === "Failed").length;
  const totalGenerating = data.filter((d) => d.status === "Generating").length;

  const goToDetail = (item: HistoryItem) => {
    if (item.type === "Report") {
      router.push(`/reports/library/${item.id}/results/latest`);
    } else {
      router.push(`/documents/exports/${item.id}`);
    }
  };

  const handleDownload = (item: HistoryItem) => {
    if (item.status === "Completed") {
      toast(`Downloading ${item.fileName} (mock)`);
    } else {
      toast("Item not available for download");
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Report & Export History
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Report/export runs, requester, state, created time, expiry and download availability.
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Completed</div>
          <div className="text-lg font-bold text-green">{totalCompleted}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Failed</div>
          <div className="text-lg font-bold text-red">{totalFailed}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Generating</div>
          <div className="text-lg font-bold text-amber">{totalGenerating}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, name, requester..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Report", "Export"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileText className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Completed", "Failed", "Expired", "Generating"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CheckCircle2 className="h-4 w-4 text-ink-400" />
          {selectedStatus}
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

      {/* ===== HISTORY TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <HistoryIcon className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No history found</h3>
          <p className="mt-1 text-sm text-ink-500">No history items match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Item</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Format</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Requester</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Created</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Completed;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Report;
                  const formatIcon = FORMAT_ICONS[item.format] || FORMAT_ICONS.PDF;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToDetail(item)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className="flex items-center gap-1 text-xs text-ink-600">
                          {formatIcon}
                          {item.format}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.requester}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                      <td className="px-4 py-3 text-right flex items-center justify-end gap-1">
                        {item.status === "Completed" && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDownload(item);
                            }}
                            className="text-primary hover:text-primary/80 transition-colors"
                            title="Download"
                          >
                            <Download className="h-4 w-4" />
                          </button>
                        )}
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
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
          <HistoryIcon className="h-4 w-4" />
          RPT-08 · Report/Export History
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} items shown
        </span>
      </div>
    </div>
  );
}