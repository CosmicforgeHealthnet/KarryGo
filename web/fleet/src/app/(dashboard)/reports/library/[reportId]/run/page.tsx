// src/app/(dashboard)/reports/library/[reportId]/run/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  RefreshCw,
  ArrowLeft,
  Play,
  Clock,
} from "lucide-react";

// ==================== TYPES ====================

type ReportConfig = {
  reportId: string;
  reportName: string;
  reportDescription: string;
  format: "PDF" | "Excel" | "CSV" | "JSON" | "HTML";
  period: string;
  branch: string;
  customer: string;
  provider: string;
  customFilters: string;
  outputName: string;
};

// ==================== MOCK DATA ====================

const MOCK_REPORT_CONFIGS: Record<string, { name: string; description: string; defaultFormat: string }> = {
  "RPT-001": {
    name: "Operations Summary",
    description: "Daily operations overview including active work, exceptions, and branch performance.",
    defaultFormat: "PDF",
  },
  "RPT-002": {
    name: "Fleet Utilization",
    description: "Asset utilization, maintenance status, and fleet readiness metrics.",
    defaultFormat: "Excel",
  },
  "RPT-003": {
    name: "Workforce Performance",
    description: "Provider utilization, attendance, acceptance/decline categories, and training status.",
    defaultFormat: "PDF",
  },
  "RPT-004": {
    name: "SLA Compliance Report",
    description: "SLA results by customer, metric, and period. Includes MET/BREACHED/UNKNOWN status.",
    defaultFormat: "Excel",
  },
  "RPT-005": {
    name: "Financial Summary",
    description: "Revenue, receivables, expenses, and profitability metrics by branch.",
    defaultFormat: "Excel",
  },
};

const PERIODS = ["Today", "This Week", "This Month", "This Quarter", "Custom Range"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const FORMATS = ["PDF", "Excel", "CSV", "JSON", "HTML"];

export default function RunReportPage() {
  const params = useParams();
  const router = useRouter();
  const reportId = params.reportId as string;

  const [loading, setLoading] = useState<boolean>(true);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [config, setConfig] = useState<ReportConfig>({
    reportId: reportId,
    reportName: "",
    reportDescription: "",
    format: "PDF",
    period: "This Month",
    branch: "All branches",
    customer: "",
    provider: "",
    customFilters: "",
    outputName: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_REPORT_CONFIGS[reportId] || {
        name: "Report",
        description: "Report description",
        defaultFormat: "PDF",
      };
      setConfig((prev) => ({
        ...prev,
        reportName: mockData.name,
        reportDescription: mockData.description,
        format: mockData.defaultFormat as any,
        outputName: `${mockData.name}_${new Date().toISOString().slice(0, 10)}`,
      }));
      setLoading(false);
    };
    loadData();
  }, [reportId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading report configuration...</p>
      </div>
    );
  }

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!config.outputName.trim()) newErrors.outputName = "Output name is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleRunReport = async () => {
    if (!validateForm()) return;

    setIsRunning(true);
    setLoading(true);

    // Simulate report generation
    await new Promise((resolve) => setTimeout(resolve, 2000));

    setIsRunning(false);
    setLoading(false);

    // Navigate to results
    router.push(`/reports/library/${reportId}/results/latest`);
  };

  const handleCancel = () => {
    router.push("/reports/library");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/reports/library")}
              className="hover:text-primary transition-colors"
            >
              Reports Library
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{config.reportName}</span>
            <span>/</span>
            <span className="text-ink-700 font-medium">Run</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Run Report
          </h1>
          <p className="mt-1 text-sm text-ink-600">{config.reportDescription}</p>
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
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {/* ===== FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Report Name (read-only) */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Report
              </label>
              <div className="mt-1 rounded-xl border border-line bg-surface-2 px-4 py-2.5 text-sm text-ink-950">
                {config.reportName}
              </div>
            </div>

            {/* Output Name */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Output Name <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={config.outputName}
                onChange={(e) => setConfig((prev) => ({ ...prev, outputName: e.target.value }))}
                placeholder="Enter output file name..."
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.outputName ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.outputName && <p className="mt-1 text-sm text-red">{errors.outputName}</p>}
            </div>

            {/* Format */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Output Format
              </label>
              <select
                value={config.format}
                onChange={(e) => setConfig((prev) => ({ ...prev, format: e.target.value as any }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {FORMATS.map((f) => (
                  <option key={f} value={f}>{f}</option>
                ))}
              </select>
            </div>

            {/* Period */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Period
              </label>
              <select
                value={config.period}
                onChange={(e) => setConfig((prev) => ({ ...prev, period: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {PERIODS.map((p) => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </select>
            </div>

            {/* Branch */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Branch Filter
              </label>
              <select
                value={config.branch}
                onChange={(e) => setConfig((prev) => ({ ...prev, branch: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {BRANCHES.map((b) => (
                  <option key={b} value={b}>{b}</option>
                ))}
              </select>
            </div>

            {/* Customer Filter */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Customer Filter
              </label>
              <input
                type="text"
                value={config.customer}
                onChange={(e) => setConfig((prev) => ({ ...prev, customer: e.target.value }))}
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
                value={config.provider}
                onChange={(e) => setConfig((prev) => ({ ...prev, provider: e.target.value }))}
                placeholder="Filter by provider..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>

            {/* Custom Filters */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Custom Filters
              </label>
              <input
                type="text"
                value={config.customFilters}
                onChange={(e) => setConfig((prev) => ({ ...prev, customFilters: e.target.value }))}
                placeholder="Add custom filter criteria..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>
          </div>

          {/* Submit Buttons */}
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={handleRunReport}
              disabled={isRunning}
              className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                isRunning
                  ? "bg-ink-400 cursor-not-allowed"
                  : "bg-primary shadow-primary/20 hover:bg-primary/90"
              }`}
            >
              {isRunning ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Generating...
                </>
              ) : (
                <>
                  <Play className="h-4 w-4" />
                  Run Report
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
          <Play className="h-4 w-4" />
          RPT-06 · Run Report · {config.reportName}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}