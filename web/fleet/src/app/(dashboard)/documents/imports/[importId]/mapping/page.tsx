// src/app/(dashboard)/documents/imports/[importId]/mapping/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowRight,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowLeft,
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
} from "lucide-react";

// ==================== TYPES ====================

type ColumnMapping = {
  sourceColumn: string;
  sourceSample: string;
  targetField: string;
  targetType: string;
  required: boolean;
  validated: boolean;
  validationError: string | null;
};

type ImportMappingData = {
  importId: string;
  importReference: string;
  importType: string;
  sourceFile: string;
  sourceFileSize: string;
  totalRows: number;
  previewRows: Record<string, string>[];
  columns: ColumnMapping[];
  validationSummary: {
    totalRows: number;
    validRows: number;
    invalidRows: number;
    errorCount: number;
    warnings: number;
  };
  status: "Draft" | "Validating" | "Valid" | "Invalid" | "Ready";
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_MAPPING_DATA: Record<string, ImportMappingData> = {
  "IMP-001": {
    importId: "IMP-001",
    importReference: "IMP-2026-001",
    importType: "Orders",
    sourceFile: "orders_bulk_20260822.csv",
    sourceFileSize: "4.2 MB",
    totalRows: 250,
    previewRows: [
      { "order_id": "ORD-001", "customer": "Charitos Hospital", "service": "Dispatch", "pickup": "Ikeja", "delivery": "Lekki", "weight": "15.4", "priority": "High" },
      { "order_id": "ORD-002", "customer": "Oakfield Stores", "service": "Dispatch", "pickup": "Lekki", "delivery": "Yaba", "weight": "8.2", "priority": "Normal" },
      { "order_id": "ORD-003", "customer": "Greenline Foods", "service": "Hauling", "pickup": "Apapa", "delivery": "Ikeja", "weight": "220.0", "priority": "Urgent" },
      { "order_id": "ORD-004", "customer": "Solaris Construction", "service": "Hauling", "pickup": "Ikeja", "delivery": "Lekki", "weight": "550.0", "priority": "High" },
      { "order_id": "ORD-005", "customer": "Adeyemi Stores", "service": "Dispatch", "pickup": "Yaba", "delivery": "Surulere", "weight": "5.0", "priority": "Normal" },
    ],
    columns: [
      { sourceColumn: "order_id", sourceSample: "ORD-001", targetField: "reference", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "customer", sourceSample: "Charitos Hospital", targetField: "customer_name", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "service", sourceSample: "Dispatch", targetField: "service_type", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "pickup", sourceSample: "Ikeja", targetField: "pickup_location", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "delivery", sourceSample: "Lekki", targetField: "delivery_location", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "weight", sourceSample: "15.4", targetField: "weight_kg", targetType: "number", required: false, validated: true, validationError: null },
      { sourceColumn: "priority", sourceSample: "High", targetField: "priority", targetType: "string", required: false, validated: true, validationError: null },
    ],
    validationSummary: {
      totalRows: 250,
      validRows: 248,
      invalidRows: 2,
      errorCount: 2,
      warnings: 0,
    },
    status: "Ready",
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  "IMP-003": {
    importId: "IMP-003",
    importReference: "IMP-2026-003",
    importType: "Providers",
    sourceFile: "provider_onboarding_20260820.csv",
    sourceFileSize: "1.8 MB",
    totalRows: 85,
    previewRows: [
      { "provider_id": "P-101", "name": "John Smith", "phone": "08012345678", "email": "john@example.com", "service": "Dispatch", "branch": "Ikeja" },
      { "provider_id": "P-102", "name": "Mary Jones", "phone": "08023456789", "email": "mary@example.com", "service": "Hauling", "branch": "Apapa" },
      { "provider_id": "P-103", "name": "David Williams", "phone": "08034567890", "email": "", "service": "Dispatch", "branch": "Lekki" },
    ],
    columns: [
      { sourceColumn: "provider_id", sourceSample: "P-101", targetField: "provider_id", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "name", sourceSample: "John Smith", targetField: "provider_name", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "phone", sourceSample: "08012345678", targetField: "phone", targetType: "string", required: true, validated: true, validationError: null },
      { sourceColumn: "email", sourceSample: "john@example.com", targetField: "email", targetType: "string", required: false, validated: true, validationError: null },
      { sourceColumn: "service", sourceSample: "Dispatch", targetField: "service_type", targetType: "string", required: true, validated: false, validationError: "Invalid service type: 'Dispatch' not in allowed list" },
      { sourceColumn: "branch", sourceSample: "Ikeja", targetField: "branch", targetType: "string", required: true, validated: true, validationError: null },
    ],
    validationSummary: {
      totalRows: 85,
      validRows: 72,
      invalidRows: 13,
      errorCount: 18,
      warnings: 5,
    },
    status: "Invalid",
    sourceFreshness: "2026-08-20T14:30:00Z",
  },
};

export default function ImportMappingPage() {
  const params = useParams();
  const router = useRouter();
  const importId = params.importId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ImportMappingData | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [editingColumn, setEditingColumn] = useState<string | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_MAPPING_DATA[importId] || MOCK_MAPPING_DATA["IMP-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [importId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading mapping details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Columns className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Mapping not found</h3>
        <p className="mt-1 text-sm text-ink-500">No mapping data found for this import.</p>
        <button
          onClick={() => router.push("/documents/imports")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Imports
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

  const handleValidate = () => {
    toast("Validation started (mock)");
  };

  const handleCommit = () => {
    toast("Import committed (mock)");
    setTimeout(() => {
      router.push(`/documents/imports/${importId}/results`);
    }, 1000);
  };

  const handleCancel = () => {
    router.push("/documents/imports");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/documents/imports")}
              className="hover:text-primary transition-colors"
            >
              Imports
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.importReference}</span>
            <span>/</span>
            <span className="text-ink-700 font-medium">Mapping</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Import Mapping & Validation
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            {data.importType} import · {data.sourceFile} · {data.totalRows} rows
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

      {/* ===== VALIDATION SUMMARY ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-5">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Rows</div>
          <div className="text-lg font-bold text-ink-950">{data.validationSummary.totalRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Valid</div>
          <div className="text-lg font-bold text-green">{data.validationSummary.validRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Invalid</div>
          <div className="text-lg font-bold text-red">{data.validationSummary.invalidRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Errors</div>
          <div className="text-lg font-bold text-red">{data.validationSummary.errorCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Warnings</div>
          <div className="text-lg font-bold text-amber">{data.validationSummary.warnings}</div>
        </div>
      </div>

      {/* ===== STATUS INDICATOR ===== */}
      <div className={`rounded-2xl border p-4 shadow-sm ${
        data.status === "Ready" ? "border-green/20 bg-green-bg/30" :
        data.status === "Invalid" ? "border-red/20 bg-red-bg/30" :
        data.status === "Validating" ? "border-amber/20 bg-amber-bg/30" :
        "border-line bg-white/50"
      }`}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {data.status === "Ready" && <CheckCircle2 className="h-5 w-5 text-green" />}
            {data.status === "Invalid" && <XCircle className="h-5 w-5 text-red" />}
            {data.status === "Validating" && <RefreshCw className="h-5 w-5 text-amber animate-spin" />}
            {data.status === "Draft" && <ClockIcon className="h-5 w-5 text-ink-400" />}
            {data.status === "Valid" && <CheckCircle2 className="h-5 w-5 text-green" />}
            <div>
              <div className="text-sm font-semibold text-ink-950">
                Status: {data.status}
                {data.status === "Invalid" && " — Fix validation errors before committing"}
                {data.status === "Ready" && " — Ready to commit"}
              </div>
              <div className="text-xs text-ink-500">
                {data.validRows} valid rows, {data.invalidRows} invalid rows
              </div>
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleValidate}
              className="flex items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 py-1.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Re-validate
            </button>
            {data.status === "Ready" && (
              <button
                onClick={handleCommit}
                className="flex items-center gap-1.5 rounded-xl bg-primary px-3.5 py-1.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                <Play className="h-3.5 w-3.5" />
                Commit Import
              </button>
            )}
          </div>
        </div>
      </div>

      {/* ===== COLUMN MAPPING ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <Columns className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Column Mapping</span>
            <span className="text-xs text-ink-400">— Map source columns to target fields</span>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Source Column</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Sample Value</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Target Field</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Required</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Validated</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
              </tr>
            </thead>
            <tbody>
              {data.columns.map((col) => (
                <tr key={col.sourceColumn} className="border-b border-line hover:bg-surface-2 transition-colors">
                  <td className="px-4 py-3">
                    <div className="font-mono text-xs text-ink-950">{col.sourceColumn}</div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="text-xs text-ink-600">{col.sourceSample}</div>
                  </td>
                  <td className="px-4 py-3">
                    <select
                      value={col.targetField}
                      onChange={(e) => {
                        // Mock update
                        toast(`Field mapped to ${e.target.value}`);
                      }}
                      className="rounded-lg border border-line bg-white px-2 py-1 text-xs text-ink-950 outline-none focus:border-primary"
                    >
                      <option value={col.targetField}>{col.targetField}</option>
                      <option value="new_field">+ New field</option>
                    </select>
                  </td>
                  <td className="px-4 py-3">
                    <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-600">
                      {col.targetType}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    {col.required ? (
                      <span className="text-red font-semibold text-xs">Required</span>
                    ) : (
                      <span className="text-ink-400 text-xs">Optional</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    {col.validated ? (
                      <CheckCircle2 className="h-4 w-4 text-green" />
                    ) : (
                      <XCircle className="h-4 w-4 text-red" />
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {col.validationError && (
                      <span className="text-[10px] text-red">{col.validationError}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== DATA PREVIEW ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <Table className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Data Preview</span>
            <span className="text-xs text-ink-400">— First {data.previewRows.length} rows</span>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                {Object.keys(data.previewRows[0] || {}).map((col) => (
                  <th key={col} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.previewRows.map((row, idx) => (
                <tr key={idx} className="border-b border-line hover:bg-surface-2 transition-colors">
                  {Object.values(row).map((val, colIdx) => (
                    <td key={colIdx} className="px-4 py-3 text-xs text-ink-700">
                      {String(val)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="p-3 text-center text-xs text-ink-400 border-t border-line">
          Showing {data.previewRows.length} of {data.totalRows} rows
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Columns className="h-4 w-4" />
          DAT-09 · Import Mapping · {data.importReference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}


