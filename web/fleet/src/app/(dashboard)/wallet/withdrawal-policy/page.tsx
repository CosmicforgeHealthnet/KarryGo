// src/app/(dashboard)/wallet/withdrawal-policy/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Settings,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  Wallet,
  Users,
  MoreHorizontal,
  CalendarDays,
  Save,
  Edit3,
  Lock,
  Unlock,
  DollarSign,
  Shield,
  AlertTriangle,
  Info,
  ChevronDown,
  X,
} from "lucide-react";

type WithdrawalPolicy = {
  id: string;
  companyId: string;
  mode: "Auto" | "ApprovalRequired" | "ManagerApproval" | "FinanceApproval";
  approvalThresholds: {
    tier: string;
    amount: number;
    requiredApprovals: number;
    approverRoles: string[];
  }[];
  maxAmountPerWithdrawal: number;
  maxAmountPerDay: number;
  maxAmountPerMonth: number;
  minAmountPerWithdrawal: number;
  requireReason: boolean;
  requireBusinessReference: boolean;
  allowedBankTypes: string[];
  allowedChannels: string[];
  updatedAt: string;
  updatedBy: string;
  version: number;
};

// Mock data
const MOCK_POLICY: WithdrawalPolicy = {
  id: "WP-001",
  companyId: "COMP-001",
  mode: "ApprovalRequired",
  approvalThresholds: [
    { tier: "Tier 1", amount: 500000, requiredApprovals: 1, approverRoles: ["Finance User", "Organization Admin"] },
    { tier: "Tier 2", amount: 1000000, requiredApprovals: 2, approverRoles: ["Finance Manager", "Organization Admin"] },
    { tier: "Tier 3", amount: 5000000, requiredApprovals: 3, approverRoles: ["Finance Manager", "Organization Admin", "Finance Director"] },
  ],
  maxAmountPerWithdrawal: 5000000,
  maxAmountPerDay: 10000000,
  maxAmountPerMonth: 50000000,
  minAmountPerWithdrawal: 10000,
  requireReason: true,
  requireBusinessReference: true,
  allowedBankTypes: ["Commercial Bank", "Microfinance Bank", "Payment Service Provider"],
  allowedChannels: ["Bank Transfer", "Mobile Money", "USSD"],
  updatedAt: "2026-08-20 14:30",
  updatedBy: "F. Daniel",
  version: 3,
};

const MODE_STYLES: Record<string, { bg: string; text: string; description: string }> = {
  Auto: {
    bg: "bg-green-bg",
    text: "text-green",
    description: "Withdrawals are automatically processed after verification.",
  },
  ApprovalRequired: {
    bg: "bg-blue-bg",
    text: "text-blue",
    description: "All withdrawals require approval before execution.",
  },
  ManagerApproval: {
    bg: "bg-amber-bg",
    text: "text-amber",
    description: "Manager approval required for all withdrawals.",
  },
  FinanceApproval: {
    bg: "bg-purple-bg",
    text: "text-purple",
    description: "Finance team approval required for all withdrawals.",
  },
};

export default function WithdrawalPolicyPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WithdrawalPolicy | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [editing, setEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [showConfirmModal, setShowConfirmModal] = useState(false);

  // Form state
  const [formData, setFormData] = useState<WithdrawalPolicy | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_POLICY);
      setFormData(MOCK_POLICY);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading withdrawal policy...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Settings className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Policy not found</h3>
        <p className="mt-1 text-sm text-ink-500">No withdrawal policy exists for this organization.</p>
        <button className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
          Create Policy
        </button>
      </div>
    );
  }

  const modeStyle = MODE_STYLES[data.mode] || MODE_STYLES.ApprovalRequired;

  const handleSave = () => {
    setShowConfirmModal(true);
  };

  const confirmSave = () => {
    setShowConfirmModal(false);
    setIsSaving(true);
    setTimeout(() => {
      setIsSaving(false);
      setEditing(false);
      setData(formData);
      toast("Withdrawal policy updated successfully");
    }, 1500);
  };

  // Simple toast
  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat("en-NG", {
      style: "currency",
      currency: "NGN",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
              Withdrawal Policy
            </h1>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${modeStyle.bg} ${modeStyle.text}`}>
              {data.mode}
            </span>
          </div>
          <p className="mt-1 text-sm text-ink-600">
            Company withdrawal mode and approval threshold/control summary.
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
                onClick={() => {
                  setEditing(false);
                  setFormData(data);
                }}
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
                {isSaving ? (
                  <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Save className="h-3.5 w-3.5" />
                )}
                Save Changes
              </button>
            </>
          ) : (
            <button
              onClick={() => setEditing(true)}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit Policy
            </button>
          )}
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Max Withdrawal</div>
          <div className="text-lg font-bold text-ink-950">{formatCurrency(data.maxAmountPerWithdrawal)}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Max Per Day</div>
          <div className="text-lg font-bold text-ink-950">{formatCurrency(data.maxAmountPerDay)}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Min Withdrawal</div>
          <div className="text-lg font-bold text-ink-950">{formatCurrency(data.minAmountPerWithdrawal)}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Version</div>
          <div className="text-lg font-bold text-ink-950">v{data.version}</div>
        </div>
      </div>

      {/* ===== MODE ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Withdrawal Mode</h3>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Object.entries(MODE_STYLES).map(([key, style]) => {
            const isSelected = key === data.mode;
            return (
              <button
                key={key}
                onClick={() => {
                  if (editing && formData) {
                    setFormData({ ...formData, mode: key as any });
                  }
                }}
                disabled={!editing}
                className={`rounded-xl border p-3.5 text-left transition ${
                  isSelected
                    ? `border-primary bg-primary-bg ring-2 ring-primary/30`
                    : `border-line hover:bg-surface-2 ${!editing ? "cursor-default" : ""}`
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className={`text-sm font-semibold ${style.text}`}>{key}</span>
                  {isSelected && <CheckCircle2 className="h-4 w-4 text-primary" />}
                </div>
                <div className="mt-1 text-xs text-ink-500">{style.description}</div>
              </button>
            );
          })}
        </div>
        {editing && (
          <div className="mt-2 text-xs text-amber">
            <AlertCircle className="inline h-3.5 w-3.5 mr-1" />
            Changing mode may affect existing withdrawal requests.
          </div>
        )}
      </div>

      {/* ===== APPROVAL THRESHOLDS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-[15px] font-semibold text-ink-950">Approval Thresholds</h3>
          {editing && (
            <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors">
              + Add Tier
            </button>
          )}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/70">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Tier</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Required Approvals</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Approver Roles</th>
              </tr>
            </thead>
            <tbody>
              {data.approvalThresholds.map((tier) => (
                <tr key={tier.tier} className="border-b border-line hover:bg-surface-2 transition-colors">
                  <td className="px-4 py-3 font-medium text-ink-950">{tier.tier}</td>
                  <td className="px-4 py-3 text-ink-600">{formatCurrency(tier.amount)}</td>
                  <td className="px-4 py-3 text-ink-600">{tier.requiredApprovals}</td>
                  <td className="px-4 py-3 text-ink-600">{tier.approverRoles.join(", ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== LIMITS & REQUIREMENTS ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Limits</h3>
          <div className="space-y-3">
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-sm text-ink-500">Max Per Withdrawal</span>
              <span className="font-medium text-ink-950">
                {editing ? (
                  <input
                    type="number"
                    value={formData?.maxAmountPerWithdrawal || 0}
                    onChange={(e) => setFormData(prev => prev ? { ...prev, maxAmountPerWithdrawal: parseInt(e.target.value) || 0 } : null)}
                    className="w-40 rounded-lg border border-line px-3 py-1 text-right text-sm text-ink-950 outline-none focus:border-primary"
                  />
                ) : (
                  formatCurrency(data.maxAmountPerWithdrawal)
                )}
              </span>
            </div>
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-sm text-ink-500">Max Per Day</span>
              <span className="font-medium text-ink-950">
                {editing ? (
                  <input
                    type="number"
                    value={formData?.maxAmountPerDay || 0}
                    onChange={(e) => setFormData(prev => prev ? { ...prev, maxAmountPerDay: parseInt(e.target.value) || 0 } : null)}
                    className="w-40 rounded-lg border border-line px-3 py-1 text-right text-sm text-ink-950 outline-none focus:border-primary"
                  />
                ) : (
                  formatCurrency(data.maxAmountPerDay)
                )}
              </span>
            </div>
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-sm text-ink-500">Max Per Month</span>
              <span className="font-medium text-ink-950">
                {editing ? (
                  <input
                    type="number"
                    value={formData?.maxAmountPerMonth || 0}
                    onChange={(e) => setFormData(prev => prev ? { ...prev, maxAmountPerMonth: parseInt(e.target.value) || 0 } : null)}
                    className="w-40 rounded-lg border border-line px-3 py-1 text-right text-sm text-ink-950 outline-none focus:border-primary"
                  />
                ) : (
                  formatCurrency(data.maxAmountPerMonth)
                )}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-ink-500">Min Per Withdrawal</span>
              <span className="font-medium text-ink-950">
                {editing ? (
                  <input
                    type="number"
                    value={formData?.minAmountPerWithdrawal || 0}
                    onChange={(e) => setFormData(prev => prev ? { ...prev, minAmountPerWithdrawal: parseInt(e.target.value) || 0 } : null)}
                    className="w-40 rounded-lg border border-line px-3 py-1 text-right text-sm text-ink-950 outline-none focus:border-primary"
                  />
                ) : (
                  formatCurrency(data.minAmountPerWithdrawal)
                )}
              </span>
            </div>
          </div>
        </div>

        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Requirements</h3>
          <div className="space-y-3">
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-sm text-ink-500">Require Reason</span>
              <span className="font-medium text-ink-950">
                {editing ? (
                  <button
                    onClick={() => setFormData(prev => prev ? { ...prev, requireReason: !prev.requireReason } : null)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold ${
                      formData?.requireReason ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                    }`}
                  >
                    {formData?.requireReason ? "Yes" : "No"}
                  </button>
                ) : (
                  data.requireReason ? "Yes" : "No"
                )}
              </span>
            </div>
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-sm text-ink-500">Require Business Reference</span>
              <span className="font-medium text-ink-950">
                {editing ? (
                  <button
                    onClick={() => setFormData(prev => prev ? { ...prev, requireBusinessReference: !prev.requireBusinessReference } : null)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold ${
                      formData?.requireBusinessReference ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                    }`}
                  >
                    {formData?.requireBusinessReference ? "Yes" : "No"}
                  </button>
                ) : (
                  data.requireBusinessReference ? "Yes" : "No"
                )}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-ink-500">Allowed Bank Types</span>
              <span className="font-medium text-ink-950 text-right text-sm">
                {data.allowedBankTypes.join(", ")}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* ===== INFO ===== */}
      <div className="rounded-xl border border-line bg-blue-bg/20 p-4">
        <div className="flex items-start gap-2.5">
          <Info className="h-4 w-4 flex-shrink-0 text-blue" />
          <div className="text-sm text-blue/80">
            <span className="font-medium">Policy controls:</span> This policy determines withdrawal approval requirements and limits. Changes apply to new withdrawal requests only.
            <br />
            <span className="text-xs">Last updated: {data.updatedAt} by {data.updatedBy} · Version {data.version}</span>
          </div>
        </div>
      </div>

      {/* ===== CONFIRM MODAL ===== */}
      {showConfirmModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <h3 className="text-lg font-semibold text-ink-950">Save Changes?</h3>
            <p className="mt-2 text-sm text-ink-600">
              Are you sure you want to save these policy changes? This will affect all future withdrawal requests.
            </p>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => setShowConfirmModal(false)}
                className="flex-1 rounded-lg border border-line bg-white/70 px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-white"
              >
                Cancel
              </button>
              <button
                onClick={confirmSave}
                className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white transition hover:bg-primary/90"
              >
                Save Changes
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Settings className="h-4 w-4" />
          WAL-09 · Withdrawal Policy
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · v{data.version}
        </span>
      </div>
    </div>
  );
}