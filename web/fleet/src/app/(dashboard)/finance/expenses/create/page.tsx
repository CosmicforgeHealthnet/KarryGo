"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Save,
  CheckCircle2,
  Wallet,
} from "lucide-react";
import { financeService, type ExpenseInput } from "@/lib/services/financeService";
import { majorToMinor } from "@/lib/utils/financeLabels";

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

const CATEGORIES = [
  "FUEL",
  "MAINTENANCE",
  "OFFICE_SUPPLIES",
  "SOFTWARE",
  "TRAVEL",
  "UTILITIES",
  "OTHER",
];

export default function CreateExpensePage() {
  const router = useRouter();

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [branchId, setBranchId] = useState("");
  const [category, setCategory] = useState("OTHER");
  const [costCentre, setCostCentre] = useState("");
  const [description, setDescription] = useState("");
  const [amountMajor, setAmountMajor] = useState("");
  const [currency, setCurrency] = useState("NGN");
  const [assetId, setAssetId] = useState("");
  const [orderId, setOrderId] = useState("");
  const [shipmentId, setShipmentId] = useState("");
  const [workOrderId, setWorkOrderId] = useState("");
  const [incidentId, setIncidentId] = useState("");
  const [providerMembershipId, setProviderMembershipId] = useState("");
  const [receiptMediaId, setReceiptMediaId] = useState("");

  const amountMinor = useMemo(() => {
    const trimmed = amountMajor.trim();
    if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null;
    const n = Number(trimmed);
    if (!Number.isFinite(n) || n <= 0) return null;
    return majorToMinor(n);
  }, [amountMajor]);

  const canSubmit = useMemo(
    () =>
      category.trim() !== "" &&
      costCentre.trim() !== "" &&
      description.trim() !== "" &&
      amountMinor !== null &&
      currency.trim().length === 3,
    [category, costCentre, description, amountMinor, currency]
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    const payload: ExpenseInput = {
      branchId: branchId.trim() || undefined,
      category: category.toUpperCase(),
      costCentre: costCentre.trim(),
      amountMinor: amountMinor!,
      currency: currency.toUpperCase(),
      description: description.trim(),
    };
    if (assetId.trim()) payload.assetId = assetId.trim();
    if (orderId.trim()) payload.orderId = orderId.trim();
    if (shipmentId.trim()) payload.shipmentId = shipmentId.trim();
    if (workOrderId.trim()) payload.workOrderId = workOrderId.trim();
    if (incidentId.trim()) payload.incidentId = incidentId.trim();
    if (providerMembershipId.trim())
      payload.providerMembershipId = providerMembershipId.trim();
    if (receiptMediaId.trim()) payload.receiptMediaId = receiptMediaId.trim();

    try {
      const created = await financeService.expenses.create(payload);
      setSuccess(`Expense ${created.id} created as draft.`);
      setTimeout(() => {
        router.push(`/finance/expenses/${created.id}`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create expense.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <button
          onClick={() => router.push("/finance/expenses")}
          className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          aria-label="Back to expenses"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            New Expense
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Log an operational expense. Draft is created first; submit and
            approve to post.
          </p>
        </div>
      </div>

      {success && (
        <div className="rounded-lg border border-green/20 bg-green-bg px-4 py-2.5 text-sm text-green">
          {success}
        </div>
      )}

      {error && (
        <div className="flex items-start gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0 mt-0.5" />
          <span>{error}</span>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Expense details
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Category *
                  </label>
                  <select
                    value={category}
                    onChange={(e) => setCategory(e.target.value)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CATEGORIES.map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Cost centre *
                  </label>
                  <input
                    value={costCentre}
                    onChange={(e) => setCostCentre(e.target.value)}
                    placeholder="e.g. Operations"
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Description *
                  </label>
                  <textarea
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    rows={3}
                    required
                    className="w-full resize-none rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Amount *
                  </label>
                  <input
                    value={amountMajor}
                    onChange={(e) => setAmountMajor(e.target.value)}
                    placeholder="0.00"
                    required
                    className={`w-full rounded-xl border bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 ${
                      amountMajor.trim() !== "" && amountMinor === null
                        ? "border-red"
                        : "border-line"
                    }`}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Currency *
                  </label>
                  <select
                    value={currency}
                    onChange={(e) => setCurrency(e.target.value)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CURRENCIES.map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Branch ID
                  </label>
                  <input
                    value={branchId}
                    onChange={(e) => setBranchId(e.target.value)}
                    placeholder="brn_… (optional)"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Receipt media ID
                  </label>
                  <input
                    value={receiptMediaId}
                    onChange={(e) => setReceiptMediaId(e.target.value)}
                    placeholder="media_… (optional)"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Linked records (optional)
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Asset ID
                  </label>
                  <input
                    value={assetId}
                    onChange={(e) => setAssetId(e.target.value)}
                    placeholder="ast_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Provider membership ID
                  </label>
                  <input
                    value={providerMembershipId}
                    onChange={(e) => setProviderMembershipId(e.target.value)}
                    placeholder="prv_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Order ID
                  </label>
                  <input
                    value={orderId}
                    onChange={(e) => setOrderId(e.target.value)}
                    placeholder="ord_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Shipment ID
                  </label>
                  <input
                    value={shipmentId}
                    onChange={(e) => setShipmentId(e.target.value)}
                    placeholder="shp_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Work order ID
                  </label>
                  <input
                    value={workOrderId}
                    onChange={(e) => setWorkOrderId(e.target.value)}
                    placeholder="wo_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Incident ID
                  </label>
                  <input
                    value={incidentId}
                    onChange={(e) => setIncidentId(e.target.value)}
                    placeholder="inc_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Category</span>
                  <span className="font-medium text-ink-950">{category}</span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Cost centre</span>
                  <span className="font-medium text-ink-950 truncate max-w-[180px]">
                    {costCentre || "—"}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Amount</span>
                  <span className="font-mono font-semibold text-ink-950">
                    {amountMinor !== null
                      ? `${(amountMinor / 100).toLocaleString()} ${currency}`
                      : "—"}
                  </span>
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <button
                type="submit"
                disabled={saving || !canSubmit}
                className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {saving ? (
                  <RefreshCw className="h-4 w-4 animate-spin" />
                ) : (
                  <Save className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Draft"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/finance/expenses")}
                className="mt-2 flex w-full items-center justify-center gap-2 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}