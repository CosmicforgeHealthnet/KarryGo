// src/app/(dashboard)/documents/imports/[importId]/results/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  Search,
  Filter,
  X,
  ChevronDown,
  Download,
  Edit3,
  Check,
  XCircle,
  RotateCw,
} from "lucide-react";

// ==================== TYPES ====================

type RowResult = {
  rowNumber: number;
  reference: string | null;
  status: "Valid" | "Invalid" | "Committed" | "Failed" | "Corrected";
  validationErrors: string[];
  correctedValues: Record<string, string> | null;
  commitResult: "Success" | "Failed" | "Pending" | null;
  commitError: string | null;
};

type ImportResultsData = {
  importId: string;
  importReference: string;
  importType: string;
  sourceFile: string;
  status: "Validating" | "Partial" | "Completed" | "Failed";
  totalRows: number;
  validRows: number;
  invalidRows: number;
  committedRows: number;
  failedRows: number;
  results: RowResult[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_RESULTS_DATA: Record<string, ImportResultsData> = {
  "IMP-001": {
    importId: "IMP-001",
    importReference: "IMP-2026-001",
    importType: "Orders",
    sourceFile: "orders_bulk_20260822.csv",
    status: "Completed",
    totalRows: 250,
    validRows: 248,
    invalidRows: 2,
    committedRows: 248,
    failedRows: 0,
    results: [
      {
        rowNumber: 1,
        reference: "ORD-001",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 2,
        reference: "ORD-002",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 3,
        reference: "ORD-003",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 4,
        reference: "ORD-004",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 5,
        reference: "ORD-005",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 6,
        reference: null,
        status: "Invalid",
        validationErrors: ["Missing required field: customer", "Invalid service type"],
        correctedValues: null,
        commitResult: null,
        commitError: null,
      },
      {
        rowNumber: 7,
        reference: null,
        status: "Invalid",
        validationErrors: ["Missing required field: pickup_location"],
        correctedValues: null,
        commitResult: null,
        commitError: null,
      },
    ],
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  "IMP-003": {
    importId: "IMP-003",
    importReference: "IMP-2026-003",
    importType: "Providers",
    sourceFile: "provider_onboarding_20260820.csv",
    status: "Partial",
    totalRows: 85,
    validRows: 72,
    invalidRows: 13,
    committedRows: 68,
    failedRows: 4,
    results: [
      {
        rowNumber: 1,
        reference: "P-101",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 2,
        reference: "P-102",
        status: "Committed",
        validationErrors: [],
        correctedValues: null,
        commitResult: "Success",
        commitError: null,
      },
      {
        rowNumber: 3,
        reference: "P-103",
        status: "Invalid",
        validationErrors: ["Invalid service type: 'Dispatch' not in allowed list"],
        correctedValues: null,
        commitResult: null,
        commitError: null,
      },
      {
        rowNumber: 4,
        reference: "P-104",
        status: "Corrected",
        validationErrors: ["Invalid branch: 'Lagos' - corrected to 'Lekki'"],
        correctedValues: { branch: "Lekki" },
        commitResult: "Pending",
        commitError: null,
      },
      {
        rowNumber: 5,
        reference: "P-105",
        status: "Failed",
        validationErrors: ["Duplicate provider ID"],
        correctedValues: null,
        commitResult: "Failed",
        commitError: "Duplicate provider ID: P-105 already exists",
      },
    ],
    sourceFreshness: "2026-08-20T14:30:00Z",
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Valid: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Invalid: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Committed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Corrected: { bg: "bg-amber-bg", text: "text-amber", icon: <Edit3 className="h-3.5 w-3.5" /> },
};

const COMMIT_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Success: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
};

// ===== SIMPLE TOAST FUNCTION =====
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

export default function ImportResultsPage() {
  const params = useParams();
  const router = useRouter();
  const importId = params.importId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ImportResultsData | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [search, setSearch] = useState("");
  const [editingRow, setEditingRow] = useState<number | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_RESULTS_DATA[importId] || MOCK_RESULTS_DATA["IMP-001"];
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
        <p className="mt-3 text-sm text-ink-600">Loading import results...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <CheckCircle2 className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Results not found</h3>
        <p className="mt-1 text-sm text-ink-500">No results found for this import.</p>
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

  const filteredResults = data.results.filter((row) => {
    const matchesStatus = selectedStatus === "All" || row.status === selectedStatus;
    const matchesSearch =
      search.trim() === "" ||
      (row.reference && row.reference.toLowerCase().includes(search.toLowerCase()));
    return matchesStatus && matchesSearch;
  });

  const hasActiveFilters = selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSelectedStatus("All");
    setSearch("");
  };

  const handleRetryFailed = () => {
    toast("Retrying failed rows (mock)");
  };

  const handleExportResults = () => {
    toast("Exporting results (mock)");
  };

  const handleBackToImports = () => {
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
            <span className="text-ink-700 font-medium">Results</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Import Results & Correction
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            {data.importType} import · {data.sourceFile} · {data.status}
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
            onClick={handleExportResults}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
          <button
            onClick={handleBackToImports}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {/* ===== SUMMARY STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-5">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.totalRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Committed</div>
          <div className="text-lg font-bold text-green">{data.committedRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Failed</div>
          <div className="text-lg font-bold text-red">{data.failedRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Invalid</div>
          <div className="text-lg font-bold text-red">{data.invalidRows}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Valid</div>
          <div className="text-lg font-bold text-green">{data.validRows}</div>
        </div>
      </div>

      {/* ===== STATUS INDICATOR ===== */}
      <div className={`rounded-2xl border p-4 shadow-sm ${
        data.status === "Completed" ? "border-green/20 bg-green-bg/30" :
        data.status === "Partial" ? "border-amber/20 bg-amber-bg/30" :
        data.status === "Failed" ? "border-red/20 bg-red-bg/30" :
        "border-line bg-white/50"
      }`}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {data.status === "Completed" && <CheckCircle2 className="h-5 w-5 text-green" />}
            {data.status === "Partial" && <AlertTriangle className="h-5 w-5 text-amber" />}
            {data.status === "Failed" && <XCircle className="h-5 w-5 text-red" />}
            {data.status === "Validating" && <RefreshCw className="h-5 w-5 text-amber animate-spin" />}
            <div>
              <div className="text-sm font-semibold text-ink-950">
                Status: {data.status}
                {data.status === "Partial" && " — Some rows failed. Review and retry."}
                {data.status === "Completed" && " — All valid rows committed successfully."}
                {data.status === "Failed" && " — Import failed. Check errors and retry."}
              </div>
              <div className="text-xs text-ink-500">
                {data.committedRows} committed, {data.failedRows} failed, {data.invalidRows} invalid
              </div>
            </div>
          </div>
          {data.failedRows > 0 && (
            <button
              onClick={handleRetryFailed}
              className="flex items-center gap-1.5 rounded-xl bg-primary px-3.5 py-1.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
            >
              <RotateCw className="h-3.5 w-3.5" />
              Retry Failed ({data.failedRows})
            </button>
          )}
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-10 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-3.5 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Valid", "Invalid", "Committed", "Failed", "Corrected"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-10 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== RESULTS TABLE ===== */}
      {filteredResults.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <CheckCircle2 className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No results match your filters</h3>
          <p className="mt-1 text-sm text-ink-500">Try adjusting your filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Row</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Validation Errors</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Commit</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredResults.map((row) => {
                  const statusStyle = STATUS_STYLES[row.status] || STATUS_STYLES.Valid;
                  const commitStyle = row.commitResult ? COMMIT_STATUS_STYLES[row.commitResult] : null;

                  return (
                    <tr
                      key={row.rowNumber}
                      className="border-b border-line hover:bg-surface-2 transition-colors"
                    >
                      <td className="px-4 py-3 font-mono text-xs text-ink-500">
                        {row.rowNumber}
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-mono text-xs text-ink-950">
                          {row.reference || "—"}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {row.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        {row.validationErrors.length > 0 ? (
                          <div className="space-y-0.5">
                            {row.validationErrors.map((err, idx) => (
                              <div key={idx} className="text-[10px] text-red">{err}</div>
                            ))}
                          </div>
                        ) : (
                          <span className="text-xs text-ink-400">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        {row.commitResult ? (
                          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${commitStyle?.bg} ${commitStyle?.text}`}>
                            {commitStyle?.icon}
                            {row.commitResult}
                          </span>
                        ) : (
                          <span className="text-xs text-ink-400">—</span>
                        )}
                        {row.commitError && (
                          <div className="text-[9px] text-red mt-0.5">{row.commitError}</div>
                        )}
                      </td>
                      <td className="px-4 py-3 text-right">
                        {row.status === "Invalid" || row.status === "Failed" ? (
                          <button
                            onClick={() => {
                              setEditingRow(row.rowNumber);
                              toast(`Editing row ${row.rowNumber} (mock)`);
                            }}
                            className="rounded-lg border border-line px-2.5 py-1 text-[10px] font-medium text-ink-700 hover:bg-white transition-colors"
                          >
                            <Edit3 className="h-3.5 w-3.5" />
                          </button>
                        ) : (
                          <button className="rounded-lg border border-line px-2.5 py-1 text-[10px] font-medium text-ink-400 opacity-50 cursor-not-allowed" disabled>
                            <Check className="h-3.5 w-3.5" />
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
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <CheckCircle2 className="h-4 w-4" />
          DAT-10 · Import Row Results · {data.importReference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredResults.length} rows shown
        </span>
      </div>
    </div>
  );
}