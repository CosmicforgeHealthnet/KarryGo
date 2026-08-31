// src/app/(dashboard)/documents/exports/[exportId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Download,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  FileText,
  User,
  ArrowLeft,
  RotateCw,
  Printer,
  Share2,
  X,
  XCircle,
} from "lucide-react";

// ==================== TYPES ====================

type ExportDetail = {
  id: string;
  reference: string;
  dataset: string;
  format: "CSV" | "Excel" | "JSON" | "PDF" | "XML";
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
  downloadCount: number;
  errorSummary: string | null;
  retryCount: number;
  lastRetryAt: string | null;
  branch: string;
  scope: string;
  columns: string[];
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_EXPORT_DATA: Record<string, ExportDetail> = {
  "EXP-001": {
    id: "EXP-001",
    reference: "EXP-2026-001",
    dataset: "Orders",
    format: "CSV",
    filters: "Date range: Aug 1-22, 2026 · Branch: Ikeja · Status: All",
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
    downloadCount: 0,
    errorSummary: null,
    retryCount: 0,
    lastRetryAt: null,
    branch: "Ikeja",
    scope: "All",
    columns: ["ID", "Reference", "Date", "Status", "Branch", "Customer", "Service Type", "Pickup", "Delivery", "Weight", "Priority"],
    tags: ["Orders", "Export"],
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  "EXP-002": {
    id: "EXP-002",
    reference: "EXP-2026-002",
    dataset: "Inventory",
    format: "Excel",
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
    downloadCount: 3,
    errorSummary: null,
    retryCount: 0,
    lastRetryAt: null,
    branch: "Lekki",
    scope: "Branch",
    columns: ["SKU", "Item", "Location", "Quantity", "Reserved", "Available", "Status", "Last Updated"],
    tags: ["Inventory", "Export", "Downloaded"],
    sourceFreshness: "2026-08-22T10:00:00Z",
  },
  "EXP-004": {
    id: "EXP-004",
    reference: "EXP-2026-004",
    dataset: "Trips",
    format: "Excel",
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
    downloadCount: 0,
    errorSummary: "Data source timeout. Please retry.",
    retryCount: 2,
    lastRetryAt: "2026-08-22T08:30:00Z",
    branch: "Apapa",
    scope: "Branch",
    columns: ["ID", "Reference", "Provider", "Status", "Date", "Origin", "Destination"],
    tags: ["Trips", "Failed"],
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  "EXP-006": {
    id: "EXP-006",
    reference: "EXP-2026-006",
    dataset: "Reports",
    format: "PDF",
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
    downloadCount: 0,
    errorSummary: "Export expired. Please regenerate.",
    retryCount: 0,
    lastRetryAt: null,
    branch: "All branches",
    scope: "All",
    columns: ["Metric", "Target", "Actual", "Result", "Period", "Branch"],
    tags: ["Reports", "SLA", "Expired"],
    sourceFreshness: "2026-08-15T14:30:00Z",
  },
};

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Queued: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-4 w-4" /> },
  Processing: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Generating: { bg: "bg-amber-bg", text: "text-amber", icon: <FileText className="h-4 w-4" /> },
  Ready: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-4 w-4" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-4 w-4" /> },
};

const FORMAT_ICONS: Record<string, React.ReactNode> = {
  CSV: <FileText className="h-5 w-5 text-green" />,
  Excel: <FileText className="h-5 w-5 text-green" />,
  JSON: <FileText className="h-5 w-5 text-amber" />,
  PDF: <FileText className="h-5 w-5 text-red" />,
  XML: <FileText className="h-5 w-5 text-blue" />,
};

// Simple toast function
const toast = (message: string) => {
  if (typeof window !== "undefined") {
    const toastEl = document.createElement("div");
    toastEl.className = "fixed bottom-4 right-4 bg-ink-950 text-white px-4 py-2 rounded-xl text-sm shadow-lg z-50 animate-in slide-in-from-bottom-5";
    toastEl.textContent = message;
    document.body.appendChild(toastEl);
    setTimeout(() => {
      toastEl.classList.add("opacity-0", "transition-opacity", "duration-300");
      setTimeout(() => toastEl.remove(), 300);
    }, 3000);
  }
};

export default function ExportDetailPage() {
  const params = useParams();
  const router = useRouter();
  const exportId = params.exportId as string;

  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<ExportDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isDownloading, setIsDownloading] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_EXPORT_DATA[exportId] || MOCK_EXPORT_DATA["EXP-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [exportId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading export details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Download className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Export not found</h3>
        <p className="mt-1 text-sm text-ink-500">No export found with this ID.</p>
        <button
          onClick={() => router.push("/documents/exports")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Exports
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Queued;
  const formatIcon = FORMAT_ICONS[data.format] || <FileText className="h-5 w-5 text-ink-400" />;

  const formatTime = (date: string): string => {
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

  const formatDateTime = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const formatDate = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const handleDownload = async () => {
    if (data.status !== "Ready" && data.status !== "Completed") {
      toast("Export not ready for download");
      return;
    }

    setIsDownloading(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setIsDownloading(false);

    toast("Download started (mock)");
  };

  const handleRegenerate = async () => {
    if (data.status !== "Expired") {
      toast("Export is still valid. Use download instead.");
      return;
    }

    toast("Regenerating export (mock)");
    await new Promise((resolve) => setTimeout(resolve, 1000));
    toast("Export regenerated (mock)");
  };

  const handleRetry = async () => {
    if (data.status !== "Failed") {
      toast("Export is not in failed state");
      return;
    }

    toast("Retrying export (mock)");
    await new Promise((resolve) => setTimeout(resolve, 1000));
    toast("Export retried (mock)");
  };

  const handleBack = () => {
    router.push("/documents/exports");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/documents/exports")}
              className="hover:text-primary transition-colors"
            >
              Exports
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Export Detail
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.status}
            </span>
            <span className="text-[10px] text-ink-500">{data.dataset}</span>
            <span className="text-[10px] text-ink-500">{data.format}</span>
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">File</div>
          <div className="mt-1 flex items-center gap-2">
            {formatIcon}
            <div>
              <div className="text-sm font-medium text-ink-950">{data.fileName || "—"}</div>
              <div className="text-xs text-ink-500">{data.fileSize || "Generating..."}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Statistics</div>
          <div className="mt-1">
            <div className="text-sm font-medium text-ink-950">{data.rowCount?.toLocaleString() || "—"} rows</div>
            <div className="text-xs text-ink-500">{data.downloadCount} downloads</div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Requester</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <User className="h-4 w-4 text-ink-400" />
            {data.requester}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            Created {formatTime(data.createdAt)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expiry</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.expiresAt ? formatDate(data.expiresAt) : "No expiry"}
          </div>
          {data.completedAt && (
            <div className="mt-0.5 text-xs text-ink-500">
              Completed {formatTime(data.completedAt)}
            </div>
          )}
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Export Filters</h3>
        <p className="mt-2 text-sm text-ink-700">{data.filters}</p>
        {data.columns.length > 0 && (
          <div className="mt-3">
            <div className="text-xs font-medium text-ink-500">Columns:</div>
            <div className="mt-1 flex flex-wrap gap-1">
              {data.columns.map((col) => (
                <span key={col} className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-600">
                  {col}
                </span>
              ))}
            </div>
          </div>
        )}
        <div className="mt-2 text-xs text-ink-400">Scope: {data.scope}</div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Actions</h3>
        <div className="mt-3 flex flex-wrap gap-3">
          {(data.status === "Ready" || data.status === "Completed") && (
            <button
              onClick={handleDownload}
              disabled={isDownloading}
              className={`flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                isDownloading
                  ? "bg-ink-400 cursor-not-allowed"
                  : "bg-primary shadow-primary/20 hover:bg-primary/90"
              }`}
            >
              {isDownloading ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Downloading...
                </>
              ) : (
                <>
                  <Download className="h-4 w-4" />
                  Download
                </>
              )}
            </button>
          )}
          {data.status === "Expired" && (
            <button
              onClick={handleRegenerate}
              className="flex items-center gap-2 rounded-xl border border-amber/20 bg-amber-bg px-4 py-2.5 text-sm font-medium text-amber hover:bg-amber/10 transition-colors"
            >
              <RotateCw className="h-4 w-4" />
              Regenerate
            </button>
          )}
          {data.status === "Failed" && (
            <button
              onClick={handleRetry}
              className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg px-4 py-2.5 text-sm font-medium text-red hover:bg-red/10 transition-colors"
            >
              <RotateCw className="h-4 w-4" />
              Retry
            </button>
          )}
          <button className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors">
            <Printer className="h-4 w-4" />
            Print
          </button>
          <button className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors">
            <Share2 className="h-4 w-4" />
            Share
          </button>
        </div>
      </div>

      {/* ===== DOWNLOAD HISTORY ===== */}
      {data.downloadedAt && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Last Download</h3>
          <div className="mt-2 flex items-center gap-3 text-sm">
            <span className="text-ink-700">
              Downloaded {formatTime(data.downloadedAt)}
            </span>
            <span className="text-ink-400">by</span>
            <span className="font-medium text-ink-950">{data.downloadedBy}</span>
            <span className="text-ink-400">·</span>
            <span className="text-ink-500">{data.downloadCount} total downloads</span>
          </div>
        </div>
      )}

      {/* ===== ERROR SUMMARY ===== */}
      {data.errorSummary && (
        <div className={`rounded-2xl border p-5 shadow-sm ${
          data.status === "Failed" ? "border-red/20 bg-red-bg/30" : "border-amber/20 bg-amber-bg/30"
        }`}>
          <div className="flex items-start gap-3">
            <div className={`rounded-full p-2 flex-shrink-0 ${
              data.status === "Failed" ? "bg-red-bg" : "bg-amber-bg"
            }`}>
              {data.status === "Failed" ? (
                <AlertCircle className="h-5 w-5 text-red" />
              ) : (
                <AlertTriangle className="h-5 w-5 text-amber" />
              )}
            </div>
            <div>
              <h4 className={`text-sm font-semibold ${
                data.status === "Failed" ? "text-red" : "text-amber"
              }`}>
                {data.status === "Failed" ? "Export Failed" : "Warning"}
              </h4>
              <p className="mt-1 text-sm text-ink-700">{data.errorSummary}</p>
              {data.retryCount > 0 && (
                <div className="mt-1 text-xs text-ink-500">
                  Retry attempts: {data.retryCount} · Last retry: {formatTime(data.lastRetryAt || "")}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Download className="h-4 w-4" />
          DAT-13 · Export Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}