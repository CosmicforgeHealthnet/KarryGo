// src/app/(dashboard)/documents/exports/create/page.tsx

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
  Download as DownloadIcon,
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
  FileSpreadsheet as FileSpreadsheetIcon3,
  FileJson as FileJsonIcon2,
  FileCsv as FileCsvIcon,
  FileXml as FileXmlIcon,
  Filter as FilterIcon,
  SlidersHorizontal as SlidersHorizontalIcon,
  X as XIcon,
  ChevronDown as ChevronDownIcon,
  Plus as PlusIcon,
  Users as UsersIcon,
  Building2 as Building2Icon,
  Tag as TagIcon,
  Calendar as CalendarIcon2,
  Info as InfoIcon2,
  ListChecks as ListChecksIcon,
  Download as DownloadIcon2,
  Printer as PrinterIcon,
  Edit3 as Edit3Icon,
  Trash2 as Trash2Icon,
  Check as CheckIcon,
  MoreHorizontal as MoreHorizontalIcon,
  User as UserIcon2,
  Mail as MailIcon,
  Phone as PhoneIcon,
  MapPin as MapPinIcon,
  Briefcase as BriefcaseIcon,
  Send as SendIcon,
  Paperclip as PaperclipIcon,
  Link2 as Link2Icon,
  ExternalLink as ExternalLinkIcon,
  FileText as FileTextIcon3,
  File as FileIcon,
  FileArchive as FileArchiveIcon,
  FileImage as FileImageIcon,
  FileJson as FileJsonIcon,
  FilePdf as FilePdfIcon,
  FileSpreadsheet as FileSpreadsheetIcon4,
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
  FileSpreadsheet as FileSpreadsheetIcon5,
  FileJson as FileJsonIcon3,
  FileCsv as FileCsvIcon2,
  FileXml as FileXmlIcon2,
  Sparkles,
  Globe,
  FilterX,
  LayoutGrid,
  LayoutList,
  Sliders,
  XCircle as XCircleIcon,
  CheckCircle as CheckCircleIcon,
  AlertTriangle as AlertTriangleIcon,
  ArrowRight as ArrowRightIcon,
  ArrowLeft as ArrowLeftIcon,
  Send as SendIcon2,
  Paperclip as PaperclipIcon2,
  Link2 as Link2Icon2,
  ExternalLink as ExternalLinkIcon2,
  FileText as FileTextIcon4,
  File as FileIcon2,
  FileArchive as FileArchiveIcon2,
  FileImage as FileImageIcon2,
  FileJson as FileJsonIcon4,
  FilePdf as FilePdfIcon2,
  FileSpreadsheet as FileSpreadsheetIcon6,
  FolderOpen as FolderOpenIcon2,
  Shield as ShieldIcon2,
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
  FileX as FileXIcon3,
  FileMinus as FileMinusIcon3,
  FilePlus as FilePlusIcon3,
  FileCog as FileCogIcon4,
  FileCheck2 as FileCheck2Icon3,
  FileSpreadsheet as FileSpreadsheetIcon7,
  FileJson as FileJsonIcon5,
  FileCsv as FileCsvIcon3,
  FileXml as FileXmlIcon3,
  Sparkles as SparklesIcon,
  Globe as GlobeIcon,
  FilterX as FilterXIcon,
  LayoutGrid as LayoutGridIcon,
  LayoutList as LayoutListIcon,
  Sliders as SlidersIcon,
  XCircle as XCircleIcon3,
  CheckCircle as CheckCircleIcon3,
  AlertTriangle as AlertTriangleIcon3,
} from "lucide-react";

// ==================== TYPES ====================

type ExportConfig = {
  dataset: string;
  format: "CSV" | "Excel" | "JSON" | "PDF" | "XML";
  filters: {
    dateRange: { from: string; to: string } | null;
    branch: string;
    status: string;
    customer: string;
    provider: string;
    custom: string;
  };
  columns: string[];
  scope: "All" | "Branch" | "Department" | "User";
  scopeValue: string;
  requireVerification: boolean;
};

const DATASETS = [
  "Orders", "Shipments", "Packages", "Providers", "Assets",
  "Customers", "Inventory", "Trips", "Invoices", "Reports", "Other"
];

const FORMATS = [
  { value: "CSV", icon: <FileCsvIcon className="h-4 w-4" /> },
  { value: "Excel", icon: <FileSpreadsheetIcon className="h-4 w-4" /> },
  { value: "JSON", icon: <FileJsonIcon className="h-4 w-4" /> },
  { value: "PDF", icon: <FilePdfIcon className="h-4 w-4" /> },
  { value: "XML", icon: <FileXmlIcon className="h-4 w-4" /> },
];

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const SCOPES = ["All", "Branch", "Department", "User"];

export default function CreateExportPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [config, setConfig] = useState<ExportConfig>({
    dataset: "Orders",
    format: "CSV",
    filters: {
      dateRange: null,
      branch: "All branches",
      status: "All",
      customer: "",
      provider: "",
      custom: "",
    },
    columns: [],
    scope: "All",
    scopeValue: "",
    requireVerification: false,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!config.dataset) newErrors.dataset = "Dataset is required";
    if (!config.format) newErrors.format = "Format is required";
    if (config.scope !== "All" && !config.scopeValue) {
      newErrors.scopeValue = "Scope value is required";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setIsSubmitting(true);
    setLoading(true);

    // Simulate export creation
    await new Promise((resolve) => setTimeout(resolve, 1500));

    setLoading(false);
    setIsSubmitting(false);

    toast("Export created successfully (mock)");
    router.push("/documents/exports");
  };

  const handleCancel = () => {
    router.push("/documents/exports");
  };

  const toggleColumn = (column: string) => {
    setConfig((prev) => ({
      ...prev,
      columns: prev.columns.includes(column)
        ? prev.columns.filter((c) => c !== column)
        : [...prev.columns, column],
    }));
  };

  const getAvailableColumns = () => {
    const baseColumns = [
      "ID", "Reference", "Date", "Status", "Branch", "Created By", "Updated By"
    ];
    switch (config.dataset) {
      case "Orders":
        return [...baseColumns, "Customer", "Service Type", "Pickup", "Delivery", "Weight", "Priority"];
      case "Providers":
        return [...baseColumns, "Name", "Phone", "Email", "Service", "Availability", "Eligibility"];
      case "Customers":
        return [...baseColumns, "Name", "Email", "Phone", "Segment", "Risk", "Account Owner"];
      case "Inventory":
        return [...baseColumns, "SKU", "Item", "Location", "Quantity", "Reserved", "Available"];
      default:
        return baseColumns;
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/documents/exports")}
              className="hover:text-primary transition-colors"
            >
              Exports
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">Create Export</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Create Export
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Select dataset, filters, scope and output format.
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
            onClick={handleCancel}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <X className="h-3.5 w-3.5" />
            Cancel
          </button>
        </div>
      </div>

      {/* ===== FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Dataset */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Dataset <span className="text-red">*</span>
              </label>
              <select
                value={config.dataset}
                onChange={(e) => setConfig((prev) => ({ ...prev, dataset: e.target.value }))}
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.dataset ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              >
                {DATASETS.map((ds) => (
                  <option key={ds} value={ds}>{ds}</option>
                ))}
              </select>
              {errors.dataset && <p className="mt-1 text-sm text-red">{errors.dataset}</p>}
            </div>

            {/* Format */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Output Format <span className="text-red">*</span>
              </label>
              <div className="mt-1 flex flex-wrap gap-2">
                {FORMATS.map((fmt) => (
                  <button
                    key={fmt.value}
                    onClick={() => setConfig((prev) => ({ ...prev, format: fmt.value as ExportConfig["format"] }))}
                    className={`flex items-center gap-1.5 rounded-xl border px-3.5 py-2 text-sm font-medium transition ${
                      config.format === fmt.value
                        ? "border-primary bg-primary-bg text-primary"
                        : "border-line bg-white/70 text-ink-600 hover:bg-white"
                    }`}
                  >
                    {fmt.icon}
                    {fmt.value}
                  </button>
                ))}
              </div>
              {errors.format && <p className="mt-1 text-sm text-red">{errors.format}</p>}
            </div>

            {/* Scope */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Scope
              </label>
              <select
                value={config.scope}
                onChange={(e) => setConfig((prev) => ({ ...prev, scope: e.target.value as ExportConfig["scope"] }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {SCOPES.map((s) => (
                  <option key={s} value={s}>{s}</option>
                ))}
              </select>
            </div>

            {/* Scope Value */}
            {config.scope !== "All" && (
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Scope Value <span className="text-red">*</span>
                </label>
                <input                  type="text"
                  value={config.scopeValue}
                  onChange={(e) => setConfig((prev) => ({ ...prev, scopeValue: e.target.value }))}
                  placeholder="Enter scope value..."
                  className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                    errors.scopeValue ? "border-red focus:border-red" : "border-line focus:border-primary"
                  }`}
                />
                {errors.scopeValue && <p className="mt-1 text-sm text-red">{errors.scopeValue}</p>}
              </div>
            )}

            {/* Branch Filter */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Branch Filter
              </label>
              <select
                value={config.filters.branch}
                onChange={(e) => setConfig((prev) => ({
                  ...prev,
                  filters: { ...prev.filters, branch: e.target.value }
                }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {BRANCHES.map((b) => (
                  <option key={b} value={b}>{b}</option>
                ))}
              </select>
            </div>

            {/* Status Filter */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Status Filter
              </label>
              <input
                type="text"
                value={config.filters.status}
                onChange={(e) => setConfig((prev) => ({
                  ...prev,
                  filters: { ...prev.filters, status: e.target.value }
                }))}
                placeholder="e.g. Active, Completed, Pending..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>

            {/* Customer Filter */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Customer Filter
              </label>
              <input
                type="text"
                value={config.filters.customer}
                onChange={(e) => setConfig((prev) => ({
                  ...prev,
                  filters: { ...prev.filters, customer: e.target.value }
                }))}
                placeholder="Filter by customer..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>

            {/* Provider Filter */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Provider Filter
              </label>
              <input
                type="text"
                value={config.filters.provider}
                onChange={(e) => setConfig((prev) => ({
                  ...prev,
                  filters: { ...prev.filters, provider: e.target.value }
                }))}
                placeholder="Filter by provider..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>

            {/* Custom Filter */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Custom Filters
              </label>
              <input
                type="text"
                value={config.filters.custom}
                onChange={(e) => setConfig((prev) => ({
                  ...prev,
                  filters: { ...prev.filters, custom: e.target.value }
                }))}
                placeholder="Add custom filter criteria..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>

            {/* Columns Selection */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Columns to Include
              </label>
              <div className="mt-1 flex flex-wrap gap-1.5">
                {getAvailableColumns().map((col) => (
                  <button
                    key={col}
                    onClick={() => toggleColumn(col)}
                    className={`rounded-full px-3 py-1 text-[11px] font-medium transition ${
                      config.columns.includes(col)
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {col}
                  </button>
                ))}
              </div>
              {config.columns.length === 0 && (
                <p className="mt-1 text-xs text-ink-400">Click columns to select. All columns included if none selected.</p>
              )}
            </div>

            {/* Sensitive Action Verification */}
            <div className="sm:col-span-2">
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={config.requireVerification}
                  onChange={(e) => setConfig((prev) => ({ ...prev, requireVerification: e.target.checked }))}
                  className="h-4 w-4 rounded border-line text-primary focus:ring-primary"
                />
                <span className="text-sm text-ink-700">
                  Require sensitive-action verification for this export
                </span>
                <span className="text-xs text-ink-400">(High-risk datasets)</span>
              </label>
            </div>
          </div>

          {/* Submit Buttons */}
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={handleSubmit}
              disabled={isSubmitting}
              className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                isSubmitting
                  ? "bg-ink-400 cursor-not-allowed"
                  : "bg-primary shadow-primary/20 hover:bg-primary/90"
              }`}
            >
              {isSubmitting ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Creating...
                </>
              ) : (
                <>
                  <Download className="h-4 w-4" />
                  Create Export
                </>
              )}
            </button>
            <button
              onClick={handleCancel}
              className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <DownloadCloud className="h-4 w-4" />
          DAT-12 · Create Export
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}