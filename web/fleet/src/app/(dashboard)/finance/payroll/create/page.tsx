"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, RefreshCw, AlertCircle, Users } from "lucide-react";
import { financeService } from "@/lib/services/financeService";

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

export default function CreatePayrollPeriodPage() {
  const router = useRouter();

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [branchId, setBranchId] = useState("");
  const [currency, setCurrency] = useState("NGN");
  const [periodStart, setPeriodStart] = useState("");
  const [periodEnd, setPeriodEnd] = useState("");

  const periodValid = useMemo(
    () => periodStart !== "" && periodEnd !== "" && periodEnd >= periodStart,
    [periodStart, periodEnd]
  );

  const canSubmit = periodValid;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    try {
      const created = await financeService.payrollPeriods.create({
        branchId: branchId.trim() || undefined,
        currency: currency.toUpperCase(),
        periodStart,
        periodEnd,
      });
      setSuccess(`Payroll period ${created.id} created.`);
      setTimeout(() => {
        router.push(`/finance/payroll/${created.id}/review`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create payroll period.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <button
          onClick={() => router.push("/finance/payroll")}
          className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          aria-label="Back to payroll"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            New Payroll Period
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Create a period, then calculate items and post to provider sub-wallets.
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
                Period details
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Branch ID (optional)
                  </label>
                  <input
                    value={branchId}
                    onChange={(e) => setBranchId(e.target.value)}
                    placeholder="brn_… (blank = all branches)"
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
                <div />
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
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Branch</span>
                  <span className="font-mono font-medium text-ink-950">
                    {branchId || "All"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Currency</span>
                  <span className="font-mono font-medium text-ink-950">
                    {currency}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Duration</span>
                  <span className="text-ink-950">
                    {periodStart && periodEnd
                      ? `${periodStart} → ${periodEnd}`
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
                  <Users className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Period"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/finance/payroll")}
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