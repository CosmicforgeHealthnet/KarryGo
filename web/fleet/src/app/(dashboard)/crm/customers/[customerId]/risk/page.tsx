// src/app/(dashboard)/crm/customers/[customerId]/risk/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  X,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Edit3,
  DollarSign,
  CreditCard,
  AlertTriangle,
  Lock,
  Unlock,
  Save,
  Plus,
} from "lucide-react";

type RiskControl = {
  id: string;
  riskStatus: "Low" | "Medium" | "High" | "Critical";
  riskReason: string | null;
  creditPolicy: "Standard" | "Restricted" | "Hold" | "Custom";
  creditLimit: number;
  creditUsed: number;
  creditAvailable: number;
  holdState: "Active" | "Inactive" | "Pending";
  holdReason: string | null;
  holdAppliedAt: string | null;
  holdAppliedBy: string | null;
  updatedAt: string;
  updatedBy: string;
  notes: string | null;
  history: {
    id: string;
    action: string;
    previousValue: string;
    newValue: string;
    timestamp: string;
    user: string;
  }[];
};

// Mock data
const MOCK_RISK: RiskControl = {
  id: "RISK-001",
  riskStatus: "Low",
  riskReason: "Consistent payment history. No late payments in 6 months.",
  creditPolicy: "Standard",
  creditLimit: 5000000,
  creditUsed: 1200000,
  creditAvailable: 3800000,
  holdState: "Inactive",
  holdReason: null,
  holdAppliedAt: null,
  holdAppliedBy: null,
  updatedAt: "2026-08-20",
  updatedBy: "F. Daniel",
  notes: "Customer has good credit standing. Recommend increasing credit limit to 7,000,000.",
  history: [
    { id: "HIS-001", action: "Risk Status Changed", previousValue: "Medium", newValue: "Low", timestamp: "2026-08-20 14:30", user: "F. Daniel" },
    { id: "HIS-002", action: "Credit Limit Increased", previousValue: "₦3,500,000", newValue: "₦5,000,000", timestamp: "2026-07-15 10:00", user: "O. Adebayo" },
    { id: "HIS-003", action: "Hold Released", previousValue: "Active", newValue: "Inactive", timestamp: "2026-06-01 09:00", user: "F. Daniel" },
  ],
};

const RISK_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Low: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Medium: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  High: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertTriangle className="h-4 w-4" />,
  },
  Critical: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertTriangle className="h-4 w-4" />,
  },
};

const POLICY_STYLES: Record<string, { bg: string; text: string }> = {
  Standard: { bg: "bg-green-bg", text: "text-green" },
  Restricted: { bg: "bg-amber-bg", text: "text-amber" },
  Hold: { bg: "bg-red-bg", text: "text-red" },
  Custom: { bg: "bg-blue-bg", text: "text-blue" },
};

const HOLD_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Inactive: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
};

export default function RiskAndCreditPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<RiskControl | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [editing, setEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [showHoldModal, setShowHoldModal] = useState(false);
  const [holdReason, setHoldReason] = useState("");
  const [showLimitModal, setShowLimitModal] = useState(false);
  const [newLimit, setNewLimit] = useState("");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_RISK, id: `RISK-${customerId}` });
      setLoading(false);
    };
    loadData();
  }, [customerId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading risk and credit data...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Shield className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Risk data not found</h3>
        <p className="mt-1 text-sm text-ink-500">No risk and credit data exists for this customer.</p>
        <button className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
          <Plus className="inline h-3.5 w-3.5 mr-1" />
          Create Risk Profile
        </button>
      </div>
    );
  }

  const riskStyle = RISK_STYLES[data.riskStatus] || RISK_STYLES.Low;
  const policyStyle = POLICY_STYLES[data.creditPolicy] || POLICY_STYLES.Standard;
  const holdStyle = HOLD_STYLES[data.holdState] || HOLD_STYLES.Inactive;

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat("en-NG", {
      style: "currency",
      currency: "NGN",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  };

  const handleSave = () => {
    setIsSaving(true);
    setTimeout(() => {
      setIsSaving(false);
      setEditing(false);
      toast("Risk profile updated successfully");
    }, 1500);
  };

  const handleApplyHold = () => {
    if (!holdReason.trim()) {
      toast("Please provide a hold reason");
      return;
    }
    setIsSaving(true);
    setTimeout(() => {
      setIsSaving(false);
      setShowHoldModal(false);
      setHoldReason("");
      toast("Hold applied successfully");
    }, 1500);
  };

  const handleReleaseHold = () => {
    setIsSaving(true);
    setTimeout(() => {
      setIsSaving(false);
      toast("Hold released successfully");
    }, 1500);
  };

  const handleUpdateLimit = () => {
    const limit = parseInt(newLimit);
    if (!limit || limit <= 0) {
      toast("Please enter a valid credit limit");
      return;
    }
    setIsSaving(true);
    setTimeout(() => {
      setIsSaving(false);
      setShowLimitModal(false);
      setNewLimit("");
      toast("Credit limit updated successfully");
    }, 1500);
  };

  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  const creditUtilization = Math.round((data.creditUsed / data.creditLimit) * 100);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Risk & Credit Controls
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Risk status, credit policy, hold state, and commitment blockers.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {editing ? (
            <>
              <button
                onClick={() => setEditing(false)}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
              >
                <X className="h-3.5 w-3.5" />
                Cancel
              </button>
              <button
                onClick={handleSave}
                disabled={isSaving}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                {isSaving ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                Save
              </button>
            </>
          ) : (
            <button
              onClick={() => setEditing(true)}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit
            </button>
          )}
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Risk Status Card */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <Shield className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Risk Status</h3>
            </div>
            <div className="flex flex-wrap items-center gap-4">
              <span className={`flex items-center gap-2 rounded-full px-4 py-2 text-sm font-semibold ${riskStyle.bg} ${riskStyle.text}`}>
                {riskStyle.icon}
                {data.riskStatus}
              </span>
              <span className="text-sm text-ink-500">
                {data.riskReason || "No specific risk reason provided."}
              </span>
            </div>
            <div className="mt-3 flex items-center gap-4 text-sm text-ink-500">
              <span>Updated: {data.updatedAt}</span>
              <span className="text-ink-300">·</span>
              <span>By: {data.updatedBy}</span>
            </div>
          </div>

          {/* Credit Information */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <CreditCard className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Credit Information</h3>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Credit Policy</div>
                <div className={`inline-block rounded-full px-3 py-1 text-sm font-semibold ${policyStyle.bg} ${policyStyle.text}`}>
                  {data.creditPolicy}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Credit Limit</div>
                <div className="text-lg font-bold text-ink-950">{formatCurrency(data.creditLimit)}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Credit Used</div>
                <div className="text-lg font-bold text-amber">{formatCurrency(data.creditUsed)}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Credit Available</div>
                <div className="text-lg font-bold text-green">{formatCurrency(data.creditAvailable)}</div>
              </div>
            </div>
            <div className="mt-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-ink-500">Utilization</span>
                <span className={`font-semibold ${creditUtilization > 80 ? "text-red" : creditUtilization > 50 ? "text-amber" : "text-green"}`}>
                  {creditUtilization}%
                </span>
              </div>
              <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                <div
                  className={`h-full rounded-full ${creditUtilization > 80 ? "bg-red" : creditUtilization > 50 ? "bg-amber" : "bg-green"}`}
                  style={{ width: `${creditUtilization}%` }}
                />
              </div>
            </div>
          </div>

          {/* Hold Status */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <Lock className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Hold Status</h3>
            </div>
            <div className="flex flex-wrap items-center gap-4">
              <span className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-semibold ${holdStyle.bg} ${holdStyle.text}`}>
                <span className={`h-1.5 w-1.5 rounded-full ${holdStyle.dot}`} />
                {data.holdState}
              </span>
              {data.holdState === "Active" && (
                <span className="text-sm text-ink-500">
                  Reason: {data.holdReason || "No reason provided"}
                </span>
              )}
            </div>
            {data.holdState === "Active" && data.holdAppliedAt && (
              <div className="mt-2 text-sm text-ink-500">
                Applied: {data.holdAppliedAt} by {data.holdAppliedBy || "Unknown"}
              </div>
            )}
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Notes */}
          {data.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
              <p className="text-sm text-ink-600">{data.notes}</p>
            </div>
          )}

          {/* Quick Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => setShowLimitModal(true)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <DollarSign className="h-4 w-4" />
                  Update Credit Limit
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => setShowHoldModal(true)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Lock className="h-4 w-4" />
                  Apply Hold
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              {data.holdState === "Active" && (
                <button
                  onClick={handleReleaseHold}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <Unlock className="h-4 w-4" />
                    Release Hold
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
            </div>
          </div>

          {/* History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Recent History</h3>
            <div className="space-y-3">
              {data.history.map((event) => (
                <div key={event.id} className="border-b border-line pb-2 last:border-0 last:pb-0">
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="text-sm font-medium text-ink-950">{event.action}</div>
                      <div className="text-xs text-ink-500">
                        {event.previousValue} → {event.newValue}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="text-xs text-ink-400">{event.timestamp}</div>
                      <div className="text-xs text-ink-400">{event.user}</div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* ===== CREDIT LIMIT MODAL ===== */}
      {showLimitModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <DollarSign className="h-5 w-5 text-primary" />
              <h3 className="text-lg font-semibold text-ink-950">Update Credit Limit</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Current limit: {formatCurrency(data.creditLimit)}
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                New Credit Limit
              </label>
              <input
                type="number"
                value={newLimit}
                onChange={(e) => setNewLimit(e.target.value)}
                placeholder="Enter new credit limit..."
                className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowLimitModal(false);
                  setNewLimit("");
                }}
                className="flex-1 rounded-lg border border-line bg-white/70 px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-white"
              >
                Cancel
              </button>
              <button
                onClick={handleUpdateLimit}
                disabled={isSaving || !newLimit}
                className={`flex-1 rounded-lg px-4 py-2 text-sm font-medium text-white transition ${
                  isSaving || !newLimit
                    ? "bg-ink-300 cursor-not-allowed"
                    : "bg-primary hover:bg-primary/90"
                }`}
              >
                {isSaving ? <RefreshCw className="h-4 w-4 animate-spin inline mr-1" /> : null}
                Update
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== HOLD MODAL ===== */}
      {showHoldModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <Lock className="h-5 w-5 text-amber" />
              <h3 className="text-lg font-semibold text-ink-950">Apply Hold</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              This will block new commitments for this customer.
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                Hold Reason
              </label>
              <textarea
                value={holdReason}
                onChange={(e) => setHoldReason(e.target.value)}
                placeholder="Explain why this hold is being applied..."
                rows={3}
                className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none"
              />
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowHoldModal(false);
                  setHoldReason("");
                }}
                className="flex-1 rounded-lg border border-line bg-white/70 px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-white"
              >
                Cancel
              </button>
              <button
                onClick={handleApplyHold}
                disabled={isSaving || !holdReason.trim()}
                className={`flex-1 rounded-lg px-4 py-2 text-sm font-medium text-white transition ${
                  isSaving || !holdReason.trim()
                    ? "bg-ink-300 cursor-not-allowed"
                    : "bg-amber hover:bg-amber/90"
                }`}
              >
                {isSaving ? <RefreshCw className="h-4 w-4 animate-spin inline mr-1" /> : null}
                Apply Hold
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CRM-06 · Risk & Credit Controls
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}