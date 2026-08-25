// src/app/(dashboard)/finance/payroll/[periodId]/review/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Users,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  FileText,
  Send,
  XCircle,
  Save,
  FileCheck,
} from "lucide-react";

type PayrollItem = {
  id: string;
  providerId: string;
  providerName: string;
  providerInitials: string;
  role: string;
  branch: string;
  baseAmount: number;
  adjustments: number;
  bonuses: number;
  deductions: number;
  netAmount: number;
  currency: string;
  status: "Pending" | "Approved" | "Posted" | "Reconciling" | "Failed";
  ruleVersion: string;
  source: string;
  explanation: string;
};

type PayrollPeriod = {
  id: string;
  reference: string;
  period: string;
  startDate: string;
  endDate: string;
  totalAmount: number;
  currency: string;
  providerCount: number;
  status: "Draft" | "Calculated" | "Approved" | "Posted" | "Reconciling" | "Failed";
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  postedAt: string | null;
  branch: string;
  notes: string | null;
  items: PayrollItem[];
  summary: {
    totalBase: number;
    totalAdjustments: number;
    totalBonuses: number;
    totalDeductions: number;
    totalNet: number;
    averageAmount: number;
  };
};

// Mock data
const MOCK_PERIOD: PayrollPeriod = {
  id: "PP-001",
  reference: "PP-2026-008",
  period: "August 2026",
  startDate: "2026-08-01",
  endDate: "2026-08-31",
  totalAmount: 2800000,
  currency: "NGN",
  providerCount: 12,
  status: "Calculated",
  createdAt: "2026-08-20",
  updatedAt: "2026-08-20",
  createdBy: "F. Daniel",
  approvedBy: null,
  approvedAt: null,
  postedAt: null,
  branch: "All branches",
  notes: "August 2026 payroll. Includes all active providers.",
  items: [
    {
      id: "PI-001",
      providerId: "P-044",
      providerName: "Peter Okafor",
      providerInitials: "PO",
      role: "Dispatch Rider",
      branch: "Ikeja",
      baseAmount: 240000,
      adjustments: 0,
      bonuses: 44000,
      deductions: 0,
      netAmount: 284000,
      currency: "NGN",
      status: "Pending",
      ruleVersion: "v3.2",
      source: "Provider Response",
      explanation: "Base: 240,000 + Bonus: 44,000",
    },
    {
      id: "PI-002",
      providerId: "P-038",
      providerName: "Musa Bello",
      providerInitials: "MB",
      role: "Hauling Driver",
      branch: "Apapa",
      baseAmount: 140000,
      adjustments: 0,
      bonuses: 16000,
      deductions: 0,
      netAmount: 156000,
      currency: "NGN",
      status: "Pending",
      ruleVersion: "v2.1",
      source: "Provider Response",
      explanation: "Base: 140,000 + Bonus: 16,000",
    },
    {
      id: "PI-003",
      providerId: "P-052",
      providerName: "Amina Yusuf",
      providerInitials: "AY",
      role: "Dispatch Supervisor",
      branch: "Yaba",
      baseAmount: 35000,
      adjustments: 0,
      bonuses: 7000,
      deductions: 0,
      netAmount: 42000,
      currency: "NGN",
      status: "Pending",
      ruleVersion: "v3.2",
      source: "Provider Response",
      explanation: "Base: 35,000 + Bonus: 7,000",
    },
    {
      id: "PI-004",
      providerId: "P-061",
      providerName: "Chinedu Uche",
      providerInitials: "CU",
      role: "Dispatch Rider",
      branch: "Lekki",
      baseAmount: 200000,
      adjustments: 0,
      bonuses: 38000,
      deductions: 0,
      netAmount: 238000,
      currency: "NGN",
      status: "Pending",
      ruleVersion: "v3.2",
      source: "Provider Response",
      explanation: "Base: 200,000 + Bonus: 38,000",
    },
    {
      id: "PI-005",
      providerId: "P-029",
      providerName: "Kabiru Ahmed",
      providerInitials: "KA",
      role: "Hauling Driver",
      branch: "Lekki",
      baseAmount: 80000,
      adjustments: 0,
      bonuses: 9000,
      deductions: 0,
      netAmount: 89000,
      currency: "NGN",
      status: "Pending",
      ruleVersion: "v2.1",
      source: "Provider Response",
      explanation: "Base: 80,000 + Bonus: 9,000",
    },
    {
      id: "PI-006",
      providerId: "P-073",
      providerName: "Esther Obi",
      providerInitials: "EO",
      role: "Fleet Manager",
      branch: "Ikeja",
      baseAmount: 200000,
      adjustments: 0,
      bonuses: 10000,
      deductions: 0,
      netAmount: 210000,
      currency: "NGN",
      status: "Pending",
      ruleVersion: "v1.5",
      source: "Workforce Service",
      explanation: "Base: 200,000 + Bonus: 10,000",
    },
  ],
  summary: {
    totalBase: 895000,
    totalAdjustments: 0,
    totalBonuses: 124000,
    totalDeductions: 0,
    totalNet: 1019000,
    averageAmount: 169833,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Approved: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Posted: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Reconciling: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const PERIOD_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Calculated: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Approved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Posted: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Reconciling: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple" },
  Failed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function PayrollReviewPage() {
  const router = useRouter();
  const params = useParams();
  const periodId = params.periodId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<PayrollPeriod | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [saving, setSaving] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_PERIOD, id: periodId });
      setLoading(false);
    };
    loadData();
  }, [periodId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading payroll review...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileText className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Payroll period not found</h3>
        <p className="mt-1 text-sm text-ink-500">The payroll period you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/finance/payroll")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to payroll
        </button>
      </div>
    );
  }

  const periodStatusStyle = PERIOD_STATUS_STYLES[data.status] || PERIOD_STATUS_STYLES.Draft;
  const formattedTotal = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.totalAmount);

  const isCalculated = data.status === "Calculated";
  const isDraft = data.status === "Draft";

  const handleRecalculate = () => {
    toast("Recalculation requested. Please wait...");
  };

  const handleSave = () => {
    setSaving(true);
    setTimeout(() => {
      setSaving(false);
      toast("Review saved successfully");
    }, 1500);
  };

  const handleSubmitForApproval = () => {
    setShowConfirm(true);
  };

  const confirmSubmitForApproval = () => {
    setShowConfirm(false);
    setSaving(true);
    setTimeout(() => {
      setSaving(false);
      toast("Payroll submitted for approval");
      router.push("/finance/payroll");
    }, 1500);
  };

  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/payroll")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Payroll Review
              </h1>
              <span className="text-sm text-ink-400">{data.reference}</span>
              <span
                className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${periodStatusStyle.bg} ${periodStatusStyle.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${periodStatusStyle.dot}`} />
                {data.status}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.period} · {data.branch}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {isCalculated && (
            <>
              <button
                onClick={handleSave}
                disabled={saving}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
              >
                <Save className="h-3.5 w-3.5" />
                Save Review
              </button>
              <button
                onClick={handleSubmitForApproval}
                disabled={saving}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                {saving ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
                Submit for Approval
              </button>
            </>
          )}
          {isDraft && (
            <button
              onClick={handleRecalculate}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Calculate
            </button>
          )}
        </div>
      </div>

      {/* ===== CONFIRM MODAL ===== */}
      {showConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <FileCheck className="h-5 w-5 text-primary" />
              <h3 className="text-lg font-semibold text-ink-950">Submit for Approval?</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Are you sure you want to submit this payroll period for approval? This will freeze the calculated values.
            </p>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => setShowConfirm(false)}
                className="flex-1 rounded-lg border border-line bg-white/70 px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-white"
              >
                Cancel
              </button>
              <button
                onClick={confirmSubmitForApproval}
                className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white transition hover:bg-primary/90"
              >
                Submit
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Providers</div>
          <div className="text-lg font-bold text-ink-950">{data.providerCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Amount</div>
          <div className="text-lg font-bold text-ink-950">{formattedTotal}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Average</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.averageAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Bonus Total</div>
          <div className="text-lg font-bold text-green">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalBonuses)}
          </div>
        </div>
      </div>

      {/* ===== SUMMARY ROW ===== */}
      <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
        <div className="flex flex-wrap items-center gap-6 text-sm">
          <div>
            <span className="text-ink-500">Base Amount:</span>
            <span className="ml-1 font-semibold text-ink-950">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalBase)}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Adjustments:</span>
            <span className="ml-1 font-semibold text-amber">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalAdjustments)}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Bonuses:</span>
            <span className="ml-1 font-semibold text-green">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalBonuses)}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Deductions:</span>
            <span className="ml-1 font-semibold text-red">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalDeductions)}
            </span>
          </div>
          <div>
            <span className="text-ink-500">Total Net:</span>
            <span className="ml-1 font-semibold text-ink-950">
              {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(data.summary.totalNet)}
            </span>
          </div>
        </div>
      </div>

      {/* ===== PAYROLL ITEMS TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm min-w-[900px]">
            <thead>
              <tr className="bg-surface-2/70">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Provider</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Base</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Bonus</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Net</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Rule</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((item) => {
                const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                const formattedBase = new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.baseAmount);
                const formattedBonus = new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.bonuses);
                const formattedNet = new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.netAmount);

                return (
                  <tr key={item.id} className="border-b border-line hover:bg-surface-2 transition-colors">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-white">
                          {item.providerInitials}
                        </div>
                        <div>
                          <div className="font-medium text-ink-950">{item.providerName}</div>
                          <div className="text-[11px] text-ink-500 font-mono">{item.providerId}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-ink-600">{item.role}</td>
                    <td className="px-4 py-3 font-medium text-ink-950">{formattedBase}</td>
                    <td className="px-4 py-3 text-green font-medium">{formattedBonus}</td>
                    <td className="px-4 py-3 font-semibold text-ink-950">{formattedNet}</td>
                    <td className="px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        {statusStyle.icon}
                        {item.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-ink-500 text-[11px]">{item.ruleVersion}</td>
                    <td className="px-4 py-3 text-right">
                      <button className="text-ink-400 hover:text-primary transition-colors">
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

      {/* ===== NOTES ===== */}
      {data.notes && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
          <p className="text-sm text-ink-600">{data.notes}</p>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileText className="h-4 w-4" />
          FIN-11 · Payroll Calculation Review
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.items.length} items
        </span>
      </div>
    </div>
  );
}