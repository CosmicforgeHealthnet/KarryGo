// src/app/(dashboard)/documents/imports/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Upload,
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
  UploadCloud,
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
} from "lucide-react";

// ==================== TYPES ====================

type ImportJob = {
  id: string;
  reference: string;
  importType: "Orders" | "Shipments" | "Packages" | "Providers" | "Assets" | "Customers" | "Inventory" | "Pricing" | "Other";
  sourceFile: string;
  sourceFileSize: string;
  status: "Queued" | "Running" | "Validating" | "Mapping" | "Processing" | "Partial" | "Completed" | "Failed" | "Cancelled";
  totalRows: number;
  validRows: number;
  invalidRows: number;
  committedRows: number;
  failedRows: number;
  progress: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  errorSummary: string | null;
  branch: string;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_IMPORTS: ImportJob[] = [
  {
    id: "IMP-001",
    reference: "IMP-2026-001",
    importType: "Orders",
    sourceFile: "orders_bulk_20260822.csv",
    sourceFileSize: "4.2 MB",
    status: "Completed",
    totalRows: 250,
    validRows: 248,
    invalidRows: 2,
    committedRows: 248,
    failedRows: 0,
    progress: 100,
    createdBy: "F. Daniel",
    createdAt: "2026-08-22T10:00:00Z",
    updatedAt: "2026-08-22T10:15:00Z",
    completedAt: "2026-08-22T10:15:00Z",
    errorSummary: "2 rows skipped due to validation errors",
    branch: "Ikeja",
    tags: ["Bulk", "Orders", "Completed"],
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  {
    id: "IMP-002",
    reference: "IMP-2026-002",
    importType: "Inventory",
    sourceFile: "inventory_update_20260821.xlsx",
    sourceFileSize: "8.7 MB",
    status: "Processing",
    totalRows: 1250,
    validRows: 1180,
    invalidRows: 70,
    committedRows: 0,
    failedRows: 0,
    progress: 65,
    createdBy: "M. Bello",
    createdAt: "2026-08-22T09:00:00Z",
    updatedAt: "2026-08-22T09:45:00Z",
    completedAt: null,
    errorSummary: null,
    branch: "Lekki",
    tags: ["Inventory", "Bulk", "Processing"],
    sourceFreshness: "2026-08-22T09:45:00Z",
  },
  {
    id: "IMP-003",
    reference: "IMP-2026-003",
    importType: "Providers",
    sourceFile: "provider_onboarding_20260820.csv",
    sourceFileSize: "1.8 MB",
    status: "Partial",
    totalRows: 85,
    validRows: 72,
    invalidRows: 13,
    committedRows: 68,
    failedRows: 4,
    progress: 85,
    createdBy: "O. Adebayo",
    createdAt: "2026-08-20T14:00:00Z",
    updatedAt: "2026-08-20T14:30:00Z",
    completedAt: null,
    errorSummary: "4 rows failed due to duplicate provider IDs. 8 rows skipped.",
    branch: "All branches",
    tags: ["Providers", "Partial", "Errors"],
    sourceFreshness: "2026-08-20T14:30:00Z",
  },
  {
    id: "IMP-004",
    reference: "IMP-2026-004",
    importType: "Customers",
    sourceFile: "customer_import_20260819.json",
    sourceFileSize: "3.1 MB",
    status: "Completed",
    totalRows: 180,
    validRows: 178,
    invalidRows: 2,
    committedRows: 178,
    failedRows: 0,
    progress: 100,
    createdBy: "O. Adebayo",
    createdAt: "2026-08-19T11:00:00Z",
    updatedAt: "2026-08-19T11:20:00Z",
    completedAt: "2026-08-19T11:20:00Z",
    errorSummary: "2 rows skipped due to missing required fields",
    branch: "Ikeja",
    tags: ["Customers", "JSON", "Completed"],
    sourceFreshness: "2026-08-19T11:20:00Z",
  },
  {
    id: "IMP-005",
    reference: "IMP-2026-005",
    importType: "Packages",
    sourceFile: "package_data_20260818.xlsx",
    sourceFileSize: "2.3 MB",
    status: "Failed",
    totalRows: 320,
    validRows: 0,
    invalidRows: 320,
    committedRows: 0,
    failedRows: 0,
    progress: 0,
    createdBy: "F. Daniel",
    createdAt: "2026-08-18T13:00:00Z",
    updatedAt: "2026-08-18T13:15:00Z",
    completedAt: null,
    errorSummary: "File format error. Expected column 'tracking_code' not found.",
    branch: "Lekki",
    tags: ["Packages", "Failed", "Format Error"],
    sourceFreshness: "2026-08-18T13:15:00Z",
  },
  {
    id: "IMP-006",
    reference: "IMP-2026-006",
    importType: "Assets",
    sourceFile: "asset_register_20260817.csv",
    sourceFileSize: "5.6 MB",
    status: "Cancelled",
    totalRows: 450,
    validRows: 420,
    invalidRows: 30,
    committedRows: 0,
    failedRows: 0,
    progress: 30,
    createdBy: "M. Bello",
    createdAt: "2026-08-17T09:00:00Z",
    updatedAt: "2026-08-17T09:30:00Z",
    completedAt: null,
    errorSummary: "Cancelled by user",
    branch: "Apapa",
    tags: ["Assets", "Cancelled", "Bulk"],
    sourceFreshness: "2026-08-17T09:30:00Z",
  },
  {
    id: "IMP-007",
    reference: "IMP-2026-007",
    importType: "Orders",
    sourceFile: "orders_express_20260816.csv",
    sourceFileSize: "1.2 MB",
    status: "Queued",
    totalRows: 75,
    validRows: 0,
    invalidRows: 0,
    committedRows: 0,
    failedRows: 0,
    progress: 0,
    createdBy: "F. Daniel",
    createdAt: "2026-08-22T12:00:00Z",
    updatedAt: "2026-08-22T12:00:00Z",
    completedAt: null,
    errorSummary: null,
    branch: "Ikeja",
    tags: ["Orders", "Queued", "Express"],
    sourceFreshness: "2026-08-22T12:00:00Z",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Queued: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-3.5 w-3.5" /> },
  Running: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCwIcon className="h-3.5 w-3.5" /> },
  Validating: { bg: "bg-amber-bg", text: "text-amber", icon: <CheckCircle className="h-3.5 w-3.5" /> },
  Mapping: { bg: "bg-purple-bg", text: "text-purple", icon: <Link2 className="h-3.5 w-3.5" /> },
  Processing: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCwIcon className="h-3.5 w-3.5" /> },
  Partial: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangleIcon className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
};

const IMPORT_TYPE_ICONS: Record<string, React.ReactNode> = {
  Orders: <Briefcase className="h-3.5 w-3.5" />,
  Shipments: <Package className="h-3.5 w-3.5" />,
  Packages: <Package className="h-3.5 w-3.5" />,
  Providers: <UserIcon className="h-3.5 w-3.5" />,
  Assets: <FileIcon className="h-3.5 w-3.5" />,
  Customers: <UsersIcon className="h-3.5 w-3.5" />,
  Inventory: <Database className="h-3.5 w-3.5" />,
  Pricing: <Tag className="h-3.5 w-3.5" />,
  Other: <FileTextIcon className="h-3.5 w-3.5" />,
};

const STATUS_FILTERS = ["All", "Queued", "Running", "Validating", "Mapping", "Processing", "Partial", "Completed", "Failed", "Cancelled"];
const IMPORT_TYPE_FILTERS = ["All", "Orders", "Shipments", "Packages", "Providers", "Assets", "Customers", "Inventory", "Pricing", "Other"];

export default function GenericImportsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ImportJob[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_IMPORTS);
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
      item.sourceFile.toLowerCase().includes(search.toLowerCase()) ||
      item.importType.toLowerCase().includes(search.toLowerCase()) ||
      item.createdBy.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesType = selectedType === "All" || item.importType === selectedType;

    return matchesSearch && matchesStatus && matchesType;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading imports...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedType !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedType("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0);

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
  const totalActive = data.filter((d) => d.status === "Queued" || d.status === "Running" || d.status === "Validating" || d.status === "Mapping" || d.status === "Processing").length;
  const totalCompleted = data.filter((d) => d.status === "Completed").length;
  const totalFailed = data.filter((d) => d.status === "Failed" || d.status === "Partial").length;

  const goToImportDetail = (id: string) => {
    router.push(`/documents/imports/${id}`);
  };

  const goToCreateImport = () => {
    router.push("/documents/imports/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Generic Imports
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Import type, source file, status, total/valid/invalid/committed rows and created time.
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
          <button
            onClick={goToCreateImport}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Import
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-blue">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Completed</div>
          <div className="text-lg font-bold text-green">{totalCompleted}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Failed / Partial</div>
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
            placeholder="Search by reference, file, type..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Queued", "Running", "Validating", "Mapping", "Processing", "Partial", "Completed", "Failed", "Cancelled"];
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
            const types = ["All", "Orders", "Shipments", "Packages", "Providers", "Assets", "Customers", "Inventory", "Pricing", "Other"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileTextIcon className="h-4 w-4 text-ink-400" />
          {selectedType}
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

      {/* ===== IMPORTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <UploadCloud className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No imports found</h3>
          <p className="mt-1 text-sm text-ink-500">No imports match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Import</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">File</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Rows</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Progress</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Created</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Queued;
                  const typeIcon = IMPORT_TYPE_ICONS[item.importType] || IMPORT_TYPE_ICONS.Other;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToImportDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="text-ink-400">{typeIcon}</span>
                          <div>
                            <div className="font-medium text-ink-950 text-xs">{item.importType}</div>
                            <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950">{item.sourceFile}</div>
                        <div className="text-[9px] text-ink-400">{item.sourceFileSize}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950">
                          <span className="text-green">{item.validRows}</span>
                          <span className="text-ink-400 mx-1">/</span>
                          <span className="text-ink-500">{item.totalRows}</span>
                        </div>
                        <div className="text-[9px] text-ink-400">
                          <span className="text-red">{item.invalidRows} invalid</span>
                          {item.failedRows > 0 && <span className="mx-1">·</span>}
                          {item.failedRows > 0 && <span className="text-red">{item.failedRows} failed</span>}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div className="h-1.5 w-16 rounded-full bg-surface-2 overflow-hidden">
                            <div
                              className="h-full rounded-full bg-primary transition-all"
                              style={{ width: `${item.progress}%` }}
                            />
                          </div>
                          <span className="text-xs font-medium text-ink-950">{item.progress}%</span>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                      <td className="px-4 py-3 text-right">
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
          <UploadCloud className="h-4 w-4" />
          DAT-08 · Generic Imports
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} imports shown
        </span>
      </div>
    </div>
  );
}