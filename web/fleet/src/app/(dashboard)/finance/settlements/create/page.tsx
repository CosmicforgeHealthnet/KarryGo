"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  HandCoins,
  Plus,
  X,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import { majorToMinor } from "@/lib/utils/financeLabels";

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

export default function CreateSettlementPage() {
  const router = useRouter();

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [supplierReference, setSupplierReference] = useState("");
  const [providerReference, setProviderReference] = useState("");
  const [currency, setCurrency] = useState("NGN");
  const [periodStart, setPeriodStart] = useState("");
  const [periodEnd, setPeriodEnd] = useState("");
  const [amountMajor, setAmountMajor] = useState("");
  const [jobs, setJobs] = useState<string[]>([]);
  const [jobInput, setJobInput] = useState("");

  const amountMinor = useMemo(() => {
    const trimmed = amountMajor.trim();
    if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null;
    const n = Number(trimmed);
    if (!Number.isFinite(n) || n < 0) return null;
    return majorToMinor(n);
  }, [amountMajor]);

  const periodValid =
    periodStart !== "" && periodEnd !== "" && periodEnd >= periodStart;

  const canSubmit = useMemo(
    () =>
      supplierReference.trim() !== "" &&
      providerReference.trim() !== "" &&
      periodValid &&
      amountMinor !== null &&
      jobs.length > 0,
    [
      supplierReference,
      providerReference,
      periodValid,
      amountMinor,
      jobs.length,
    ]
  );

  const addJob = () => {
    const v = jobInput.trim();
    if (!v || jobs.includes(v)) return;
    setJobs((prev) => [...prev, v]);
    setJobInput("");
  };

  const removeJob = (j: string) =>
    setJobs((prev) => prev.filter((x) => x !== j));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    try {
      const created = await financeService.settlements.create({
        supplierReference: supplierReference.trim(),
        providerOrCarrierReference: providerReference.trim(),
        currency: currency.toUpperCase(),
        periodStart,
        periodEnd,
        jobReferences: jobs,
        amountMinor: amountMinor!,
      });
      setSuccess(`Settlement ${created.id} created as draft.`);
      setTimeout(() => {
        router.push(`/finance/settlements/${created.id}`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create settlement.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <button
          onClick={() => router.push("/finance/settlements")}
          className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          aria-label="Back to settlements"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            New Subcontractor Settlement
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Group completed jobs into a settlement for a supplier or provider.
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
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Settlement details
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Supplier reference *
                  </label>
                  <input
                    value={supplierReference}
                    onChange={(e) => setSupplierReference(e.target.value)}
                    placeholder="Supplier ID or reference"
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Provider / carrier reference *
                  </label>
                  <input
                    value={providerReference}
                    onChange={(e) => setProviderReference(e.target.value)}
                    placeholder="Provider or carrier ID"
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
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
                    Period start *
                  </label>
                  <input
                    type="date"
                    value={periodStart}
                    onChange={(e) => setPeriodStart(e.target.value)}
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Period end *
                  </label>
                  <input
                    type="date"
                    value={periodEnd}
                    onChange={(e) => setPeriodEnd(e.target.value)}
                    required
                    className={`w-full rounded-xl border bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 ${
                      periodEnd !== "" &&
                      periodStart !== "" &&
                      periodEnd < periodStart
                        ? "border-red"
                        : "border-line"
                    }`}
                  />
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Job references
              </h3>
              <p className="mb-3 text-xs text-ink-500">
                Each job can only be settled once. Backend rejects duplicates.
              </p>
              <div className="flex gap-2">
                <input
                  value={jobInput}
                  onChange={(e) => setJobInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      addJob();
                    }
                  }}
                  placeholder="Job/assignment reference (asm_…)"
                  className="flex-1 rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
                <button
                  type="button"
                  onClick={addJob}
                  className="flex h-10 items-center gap-1 rounded-xl border border-line bg-surface px-3.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add
                </button>
              </div>
              {jobs.length > 0 && (
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {jobs.map((j) => (
                    <span
                      key={j}
                      className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 font-mono text-[11px] text-ink-700"
                    >
                      {j}
                      <button
                        type="button"
                        onClick={() => removeJob(j)}
                        className="text-ink-400 hover:text-red"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Supplier</span>
                  <span className="truncate font-mono font-medium text-ink-950 max-w-[180px]">
                    {supplierReference || "—"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Provider</span>
                  <span className="truncate font-mono font-medium text-ink-950 max-w-[180px]">
                    {providerReference || "—"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Jobs</span>
                  <span className="font-medium text-ink-950">
                    {jobs.length}
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
                  <HandCoins className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Settlement"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/finance/settlements")}
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