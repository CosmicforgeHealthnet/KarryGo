"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Calculator,
  CheckCircle2,
  FileText,
  Users,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { PayrollPeriodView } from "@/lib/types/finance";
import {
  compensationItemSourceLabel,
  compensationItemStatusLabel,
  compensationItemStatusTone,
  formatDate,
  formatMinor,
  payrollStatusLabel,
  payrollStatusTone,
} from "@/lib/utils/financeLabels";

export default function PayrollReviewPage() {
  const router = useRouter();
  const params = useParams();
  const periodId = params.periodId as string;

  const [period, setPeriod] = useState<PayrollPeriodView | null>(null);
  const [loading, setLoading] = useState(true);
  const [calculating, setCalculating] = useState(false);
  const [approving, setApproving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const fetchPeriod = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.payrollPeriods.get(periodId);
      setPeriod(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load payroll period.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [periodId]);

  useEffect(() => {
    if (periodId) fetchPeriod();
  }, [periodId, fetchPeriod]);

  const handleCalculate = async () => {
    if (!period || calculating) return;
    setCalculating(true);
    setError(null);
    setSuccess(null);
    try {
      const updated = await financeService.payrollPeriods.calculate(
        period.id,
        period.version
      );
      setPeriod(updated);
      setSuccess("Payroll calculated.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to calculate payroll.";
      setError(message);
    } finally {
      setCalculating(false);
    }
  };

  const handleApprove = async () => {
    if (!period || approving) return;
    setApproving(true);
    setError(null);
    setSuccess(null);
    try {
      const updated = await financeService.payrollPeriods.approve(
        period.id,
        period.version
      );
      setPeriod(updated);
      setSuccess("Payroll approved.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to approve payroll.";
      setError(message);
    } finally {
      setApproving(false);
    }
  };

  const summary = useMemo(() => {
    if (!period) {
      return {
        totalBase: 0,
        totalBonus: 0,
        totalDeduction: 0,
        totalNet: 0,
        count: 0,
      };
    }
    let totalBase = 0;
    let totalBonus = 0;
    let totalDeduction = 0;
    for (const item of period.items) {
      if (item.amount_minor >= 0) {
        if (item.reason_code === "BONUS" || item.reason_code === "COMMISSION") {
          totalBonus += item.amount_minor;
        } else {
          totalBase += item.amount_minor;
        }
      } else {
        totalDeduction += Math.abs(item.amount_minor);
      }
    }
    return {
      totalBase,
      totalBonus,
      totalDeduction,
      totalNet: totalBase + totalBonus - totalDeduction,
      count: period.items.length,
    };
  }, [period]);

  if (loading && !period) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading payroll period…</p>
      </div>
    );
  }

  if (error && !period) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load period
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <button
          onClick={fetchPeriod}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!period) return null;

  const statusTone = payrollStatusTone(period.status);
  const canCalculate =
    period.status === "DRAFT" || period.status === "CALCULATED";
  const canApprove =
    period.status === "CALCULATED" || period.status === "PENDING_APPROVAL";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/finance/payroll/${period.id}`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to period"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Payroll Review
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {payrollStatusLabel(period.status)}
              </span>
              <span className="text-sm text-ink-400">v{period.version}</span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">
              {formatDate(period.period_start)} –{" "}
              {formatDate(period.period_end)} ·{" "}
              {period.branch_id ?? "All branches"}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchPeriod}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Reload"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {canCalculate && (
            <button
              onClick={handleCalculate}
              disabled={calculating}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2 disabled:opacity-50"
            >
              {calculating ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Calculator className="h-3.5 w-3.5" />
              )}
              Recalculate
            </button>
          )}
          {canApprove && (
            <button
              onClick={handleApprove}
              disabled={approving}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {approving ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCircle2 className="h-3.5 w-3.5" />
              )}
              Approve
            </button>
          )}
        </div>
      </div>

      {success && (
        <div className="rounded-lg border border-green/20 bg-green-bg px-4 py-2.5 text-sm text-green">
          {success}
        </div>
      )}

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => setError(null)}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Dismiss
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Items
          </div>
          <div className="text-lg font-bold text-ink-950">{summary.count}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Base
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(summary.totalBase, period.currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Bonus
          </div>
          <div className="text-lg font-bold text-green">
            {formatMinor(summary.totalBonus, period.currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Net
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(summary.totalNet, period.currency)}
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-surface p-5">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-ink-950">
            Computed items
          </h3>
          <span className="text-xs text-ink-400">
            Calculation v{period.calculation_version}
          </span>
        </div>
        {period.items.length === 0 ? (
          <div className="py-8 text-center text-sm text-ink-500">
            <Users className="mx-auto h-8 w-8 text-ink-400" />
            <p className="mt-2">No items yet — run calculation first.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Membership
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Source
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Rule
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Amount
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Status
                  </th>
                </tr>
              </thead>
              <tbody>
                {period.items.map((item) => {
                  const itemTone = compensationItemStatusTone(item.status);
                  const negative = item.amount_minor < 0;
                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line transition-colors hover:bg-surface-2"
                    >
                      <td className="px-4 py-3 font-mono text-[12px] text-ink-600">
                        {item.membership_id}
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">
                          {compensationItemSourceLabel(item.source_type)}
                        </div>
                        <div className="font-mono text-[11px] text-ink-500">
                          {item.source_reference}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-mono text-[12px] text-ink-600">
                          {item.rule_id}
                        </div>
                        <div className="text-[11px] text-ink-500">
                          v{item.rule_version}
                        </div>
                      </td>
                      <td
                        className={`px-4 py-3 font-semibold ${
                          negative ? "text-red" : "text-ink-950"
                        }`}
                      >
                        {negative ? "− " : ""}
                        {formatMinor(Math.abs(item.amount_minor), item.currency)}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${itemTone.bg} ${itemTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${itemTone.dot}`}
                          />
                          {compensationItemStatusLabel(item.status)}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}