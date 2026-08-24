// src/app/(dashboard)/documents/exports/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Download,
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
  DownloadCloud,
  FileSpreadsheet as FileSpreadsheetIcon,
  FileJson as FileJsonIcon,
  FileCsv,
  FileXml,
  FileText as FileTextIcon2,
  AlertCircle as AlertCircleIcon,
  CheckCircle as CheckCircleIcon,
  XCircle as XCircleIcon,
  AlertTriangle as AlertTriangleIcon,
  RefreshCw as RefreshCwIcon,
  Info as InfoIcon,
  Table,
  Columns,
  Split,
  Merge,
  AlignLeft,
  AlignCenter,
  AlignRight,
  Type,
  Hash,
  Calendar as CalendarIcon,
  DollarSign,
  Percent,
  Link,
  Unlink,
  Play,
  Save,
  FileSpreadsheet as FileSpreadsheetIcon2,
  FileCog as FileCogIcon2,
  RotateCw,
  Undo2,
  Redo2,
  FileCheck as FileCheckIcon2,
  FileX as FileXIcon2,
  FileMinus as FileMinusIcon2,
  FilePlus as FilePlusIcon2,
  FileCog as FileCogIcon3,
  FileCheck2 as FileCheck2Icon2,
  AlertCircle as AlertCircleIcon2,
  CheckCircle as CheckCircleIcon2,
  XCircle as XCircleIcon2,
  AlertTriangle as AlertTriangleIcon2,
  Download as DownloadIcon,
  FileSpreadsheet as FileSpreadsheetIcon3,
  FileJson as FileJsonIcon2,
  FileCsv as FileCsvIcon,
  FileXml as FileXmlIcon,
} from "lucide-react";

// ==================== TYPES ====================

type ExportJob = {
  id: string;
  reference: string;
  dataset: "Orders" | "Shipments" | "Packages" | "Providers" | "Assets" | "Customers" | "Inventory" | "Trips" | "Invoices" | "Reports" | "Other";
  filters: string;
  requester: string;
  status: "Queued" | "Processing" | "Generating" | "Ready" | "Completed" | "Failed" | "Expired" | "Cancelled";
  fileSize: string | null;
  fileName: string | null;
  rowCount: number | null;
  createdAt: string;
  expiresAt: string | null;
  completedAt: string | null;
  downloadedAt: string | null;
  downloadedBy: string | null;
  errorSummary: string | null;
  branch: string;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_EXPORTS: ExportJob[] = [
  {
    id: "EXP-001",
    reference: "EXP-2026-001",
    dataset: "Orders",
    filters: "Date range: Aug 1-22, 2026 · Branch: Ikeja",
    requester: "F. Daniel",
    status: "Ready",
    fileSize: "12.4 MB",
    fileName: "orders_export_20260822.csv",
    rowCount: 248,
    createdAt: "2026-08-22T10:00:00Z",
    expiresAt: "2026-08-29T10:00:00Z",
    completedAt: "2026-08-22T10:15:00Z",
    downloadedAt: null,
    downloadedBy: null,
    errorSummary: null,
    branch: "Ikeja",
    tags: ["Orders", "Export"],
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  {
    id: "EXP-002",
    reference: "EXP-2026-002",
    dataset: "Inventory",
    filters: "Location: Lekki · Category: All",
    requester: "M. Bello",
    status: "Completed",
    fileSize: "18.7 MB",
    fileName: "inventory_export_20260821.xlsx",
    rowCount: 1180,
    createdAt: "2026-08-22T09:00:00Z",
    expiresAt: "2026-08-29T09:00:00Z",
    completedAt: "2026-08-22T09:20:00Z",
    downloadedAt: "2026-08-22T10:00:00Z",
    downloadedBy: "M. Bello",
    errorSummary: null,
    branch: "Lekki",
    tags: ["Inventory", "Export", "Downloaded"],
    sourceFreshness: "2026-08-22T10:00:00Z",
  },
  {
    id: "EXP-003",
    reference: "EXP-2026-003",
    dataset: "Providers",
    filters: "Status: Active · All branches",
    requester: "O. Adebayo",
    status: "Processing",
    fileSize: null,
    fileName: null,
    rowCount: null,
    createdAt: "2026-08-22T11:00:00Z",
    expiresAt: null,
    completedAt: null,
    downloadedAt: null,
    downloadedBy: null,
    errorSummary: null,
    branch: "All branches",
    tags: ["Providers", "Processing"],
    sourceFreshness: "2026-08-22T11:00:00Z",
  },
  {
    id: "EXP-004",
    reference: "EXP-2026-004",
    dataset: "Trips",
    filters: "Date: Aug 2026 · Branch: Apapa",
    requester: "K. Ahmed",
    status: "Failed",
    fileSize: null,
    fileName: null,
    rowCount: null,
    createdAt: "2026-08-22T08:00:00Z",
    expiresAt: null,
    completedAt: null,
    downloadedAt: null,
    downloadedBy: null,
    errorSummary: "Data source timeout. Please retry.",
    branch: "Apapa",
    tags: ["Trips", "Failed"],
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  {
    id: "EXP-005",
    reference: "EXP-2026-005",
    dataset: "Invoices",
    filters: "Status: Outstanding · Customer: Charitos Hospital",
    requester: "F. Daniel",
    status: "Queued",
    fileSize: null,
    fileName: null,
    rowCount: null,
    createdAt: "2026-08-22T12:00:00Z",
    expiresAt: null,
    completedAt: null,
    downloadedAt: null,
    downloadedBy: null,
    errorSummary: null,
    branch: "Ikeja",
    tags: ["Invoices", "Queued"],
    sourceFreshness: "2026-08-22T12:00:00Z",
  },
  {
    id: "EXP-006",
    reference: "EXP-2026-006",
    dataset: "Reports",
    filters: "Type: SLA Report · Period: Q3 2026",
    requester: "O. Adebayo",
    status: "Expired",
    fileSize: "8.2 MB",
    fileName: "sla_report_q3_2026.pdf",
    rowCount: 42,
    createdAt: "2026-08-15T14:00:00Z",
    expiresAt: "2026-08-22T14:00:00Z",
    completedAt: "2026-08-15T14:30:00Z",
    downloadedAt: null,
    downloadedBy: null,
    errorSummary: "Export expired. Please regenerate.",
    branch: "All branches",
    tags: ["Reports", "SLA", "Expired"],
    sourceFreshness: "2026-08-15T14:30:00Z",
  },
  {
    id: "EXP-007",
    reference: "EXP-2026-007",
    dataset: "Customers",
    filters: "Segment: Corporate · Branch: Lekki",
    requester: "O. Adebayo",
    status: "Cancelled",
    fileSize: null,
    fileName: null,
    rowCount: null,
    createdAt: "2026-08-22T07:00:00Z",
    expiresAt: null,
    completedAt: null,
    downloadedAt: null,
    downloadedBy: null,
    errorSummary: "Cancelled by user",
    branch: "Lekki",
    tags: ["Customers", "Cancelled"],
    sourceFreshness: "2026-08-22T07:00:00Z",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Queued: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-3.5 w-3.5" /> },
  Processing: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCwIcon className="h-3.5 w-3.5" /> },
  Generating: { bg: "bg-amber-bg", text: "text-amber", icon: <FileTextIcon className="h-3.5 w-3.5" /> },
  Ready: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-3.5 w-3.5" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
};

const DATASET_ICONS: Record<string, React.ReactNode> = {
  Orders: <Briefcase className="h-3.5 w-3.5" />,
  Shipments: <Package className="h-3.5 w-3.5" />,
  Packages: <Package className="h-3.5 w-3.5" />,
  Providers: <UserIcon className="h-3.5 w-3.5" />,
  Assets: <FileIcon className="h-3.5 w-3.5" />,
  Customers: <UsersIcon className="h-3.5 w-3.5" />,
  Inventory: <Database className="h-3.5 w-3.5" />,
  Trips: <MapPinIcon className="h-3.5 w-3.5" />,
  Invoices: <FileTextIcon className="h-3.5 w-3.5" />,
  Reports: <Chart className="h-3.5 w-3.5" />,
  Other: <FileTextIcon className="h-3.5 w-3.5" />,
};

const STATUS_FILTERS = ["All", "Queued", "Processing", "Generating", "Ready", "Completed", "Failed", "Expired", "Cancelled"];
const DATASET_FILTERS = ["All", "Orders", "Shipments", "Packages", "Providers", "Assets", "Customers", "Inventory", "Trips", "Invoices", "Reports", "Other"];

export default function ExportsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ExportJob[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedDataset, setSelectedDataset] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_EXPORTS);
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
      item.dataset.toLowerCase().includes(search.toLowerCase()) ||
      item.requester.toLowerCase().includes(search.toLowerCase()) ||
      (item.fileName && item.fileName.toLowerCase().includes(search.toLowerCase()));

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesDataset = selectedDataset === "All" || item.dataset === selectedDataset;

    return matchesSearch && matchesStatus && matchesDataset;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading exports...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedDataset !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedDataset("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedDataset !== "All" ? 1 : 0);

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
  const totalReady = data.filter((d) => d.status === "Ready" || d.status === "Completed").length;
  const totalProcessing = data.filter((d) => d.status === "Queued" || d.status === "Processing" || d.status === "Generating").length;
  const totalFailed = data.filter((d) => d.status === "Failed").length;

  const goToExportDetail = (id: string) => {
    router.push(`/documents/exports/${id}`);
  };

  const goToCreateExport = () => {
    router.push("/documents/exports/create");
  };

  const handleDownload = (id: string) => {
    toast("Download started (mock)");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Exports
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Dataset type, filters, requester, state, created/expiry time and file readiness.
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
            onClick={goToCreateExport}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Export
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Ready / Completed</div>
          <div className="text-lg font-bold text-green">{totalReady}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Processing</div>
          <div className="text-lg font-bold text-blue">{totalProcessing}</div>
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
            placeholder="Search by reference, dataset, requester..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Queued", "Processing", "Generating", "Ready", "Completed", "Failed", "Expired", "Cancelled"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <ClockIcon className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const datasets = ["All", "Orders", "Shipments", "Packages", "Providers", "Assets", "Customers", "Inventory", "Trips", "Invoices", "Reports", "Other"];
            const currentIndex = datasets.indexOf(selectedDataset);
            const nextIndex = (currentIndex + 1) % datasets.length;
            setSelectedDataset(datasets[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileTextIcon className="h-4 w-4 text-ink-400" />
          {selectedDataset}
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

      {/* ===== EXPORTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <DownloadCloud className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No exports found</h3>
          <p className="mt-1 text-sm text-ink-500">No exports match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Export</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Dataset</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">File</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Requester</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Created</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Queued;
                  const datasetIcon = DATASET_ICONS[item.dataset] || DATASET_ICONS.Other;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToExportDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="text-ink-400">{datasetIcon}</span>
                          <div>
                            <div className="font-medium text-ink-950 text-xs">{item.dataset}</div>
                            <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="rounded-full px-2.5 py-0.5 text-[10px] font-semibold bg-surface-2 text-ink-600">
                          {item.dataset}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        {item.fileName ? (
                          <div>
                            <div className="text-xs text-ink-950 truncate max-w-[150px]">{item.fileName}</div>
                            <div className="text-[9px] text-ink-400">{item.fileSize} · {item.rowCount} rows</div>
                          </div>
                        ) : (
                          <span className="text-xs text-ink-400">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.requester}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                      <td className="px-4 py-3 text-right flex items-center justify-end gap-1">
                        {item.status === "Ready" && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDownload(item.id);
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
          <DownloadCloud className="h-4 w-4" />
          DAT-11 · Exports
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} exports shown
        </span>
      </div>
    </div>
  );
}