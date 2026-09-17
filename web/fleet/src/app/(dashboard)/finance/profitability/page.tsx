"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import {
  ChartNoAxesCombined,
  RefreshCw,
  AlertCircle,
  TrendingUp,
  TrendingDown,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type {
  InvoiceView,
  ExpenseRow,
  ReceivableRow,
} from "@/lib/types/finance";
import { formatMinor } from "@/lib/utils/financeLabels";

type MonthBucket = {
  label: string;
  revenue: number;
  costs: number;
  profit: number;
};

type CategoryBucket = {
  label: string;
  amount: number;
};

export default function ProfitabilityPage() {
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [invoices, setInvoices] = useState<InvoiceView[]>([]);
  const [expenses, setExpenses] = useState<ExpenseRow[]>([]);
  const [receivables, setReceivables] = useState<ReceivableRow[]>([]);

  const fetchAll = useCallback(async (opts: { silent?: boolean } = {}) => {
    if (opts.silent) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const [invoicesRes, expensesRes, receivablesRes] = await Promise.all([
        financeService.invoices.list(),
        financeService.expenses.list(),
        financeService.receivables.list(),
      ]);
      setInvoices(invoicesRes.data);
      setExpenses(expensesRes.data);
      setReceivables(receivablesRes.data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load profitability data.";
      setError(message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const currency =
    invoices[0]?.currency ??
    expenses[0]?.currency ??
    receivables[0]?.currency ??
    "NGN";

  const summary = useMemo(() => {
    const revenue = invoices
      .filter((i) => i.status !== "CANCELLED" && i.status !== "WRITTEN_OFF")
      .reduce((sum, i) => sum + i.total_minor, 0);
    const costs = expenses
      .filter((e) => e.status !== "CANCELLED" && e.status !== "REJECTED")
      .reduce((sum, e) => sum + e.amount_minor, 0);
    const grossProfit = revenue - costs;
    const grossMargin = revenue > 0 ? (grossProfit / revenue) * 100 : 0;

    return {
      revenue,
      costs,
      grossProfit,
      grossMargin,
    };
  }, [invoices, expenses]);

  const monthlyTrend: MonthBucket[] = useMemo(() => {
    const buckets = new Map<string, MonthBucket>();
    const getBucket = (iso: string): MonthBucket => {
      const d = new Date(iso);
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
      const label = d.toLocaleDateString("en-NG", {
        month: "short",
        year: "2-digit",
      });
      let existing = buckets.get(key);
      if (!existing) {
        existing = { label, revenue: 0, costs: 0, profit: 0 };
        buckets.set(key, existing);
      }
      return existing;
    };

    for (const inv of invoices) {
      if (inv.status === "CANCELLED" || inv.status === "WRITTEN_OFF") continue;
      getBucket(inv.created_at).revenue += inv.total_minor;
    }
    for (const exp of expenses) {
      if (exp.status === "CANCELLED" || exp.status === "REJECTED") continue;
      getBucket(exp.created_at).costs += exp.amount_minor;
    }

    const sorted = Array.from(buckets.entries())
      .sort((a, b) => a[0].localeCompare(b[0]))
      .slice(-8)
      .map(([, bucket]) => ({
        ...bucket,
        profit: bucket.revenue - bucket.costs,
      }));
    return sorted;
  }, [invoices, expenses]);

  const costBreakdown: CategoryBucket[] = useMemo(() => {
    const byCategory = new Map<string, number>();
    for (const exp of expenses) {
      if (exp.status === "CANCELLED" || exp.status === "REJECTED") continue;
      const current = byCategory.get(exp.category) ?? 0;
      byCategory.set(exp.category, current + exp.amount_minor);
    }
    const total = Array.from(byCategory.values()).reduce((s, v) => s + v, 0);
    return Array.from(byCategory.entries())
      .map(([label, amount]) => ({ label, amount }))
      .sort((a, b) => b.amount - a.amount)
      .map((b) => ({
        ...b,
        amount: b.amount,
      }))
      .slice(0, 6)
      .map((b) => ({
        label: b.label,
        amount: b.amount,
      })) as CategoryBucket[];
  }, [expenses]);

  const costTotal = costBreakdown.reduce((s, b) => s + b.amount, 0);
  const revenueByCurrency = currency;

  if (loading && invoices.length === 0 && expenses.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading profitability…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Profitability
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Revenue, cost, and margin computed from invoices and expenses.
          </p>
        </div>
        <button
          onClick={() => fetchAll({ silent: true })}
          disabled={refreshing}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
          aria-label="Refresh"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchAll()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-green-bg text-green">
              <TrendingUp className="h-3.5 w-3.5" />
            </div>
            <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
              Revenue
            </span>
          </div>
          <div className="mt-2 text-lg font-bold text-ink-950">
            {formatMinor(summary.revenue, revenueByCurrency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-red-bg text-red">
              <TrendingDown className="h-3.5 w-3.5" />
            </div>
            <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
              Costs
            </span>
          </div>
          <div className="mt-2 text-lg font-bold text-ink-950">
            {formatMinor(summary.costs, revenueByCurrency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary-bg text-primary">
              <ChartNoAxesCombined className="h-3.5 w-3.5" />
            </div>
            <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
              Gross profit
            </span>
          </div>
          <div
            className={`mt-2 text-lg font-bold ${
              summary.grossProfit >= 0 ? "text-green" : "text-red"
            }`}
          >
            {formatMinor(summary.grossProfit, revenueByCurrency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary-bg text-primary">
              <ChartNoAxesCombined className="h-3.5 w-3.5" />
            </div>
            <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
              Gross margin
            </span>
          </div>
          <div className="mt-2 text-lg font-bold text-ink-950">
            {summary.grossMargin.toFixed(1)}%
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Trend
                </div>
                <div className="text-sm font-semibold text-ink-950">
                  Revenue and costs by month
                </div>
              </div>
              <span className="text-xs text-ink-400">
                Last {monthlyTrend.length} months
              </span>
            </div>
            {monthlyTrend.length === 0 ? (
              <div className="py-8 text-center text-sm text-ink-500">
                No data yet.
              </div>
            ) : (
              <>
                <div className="flex h-[200px] items-end gap-2">
                  {monthlyTrend.map((bucket, i) => {
                    const max = Math.max(
                      ...monthlyTrend.map((b) =>
                        Math.max(b.revenue, b.costs, Math.abs(b.profit))
                      ),
                      1
                    );
                    const revHeight = (bucket.revenue / max) * 100;
                    const costHeight = (bucket.costs / max) * 100;
                    const profitHeight = (Math.abs(bucket.profit) / max) * 100;
                    return (
                      <div
                        key={i}
                        className="flex flex-1 flex-col items-center gap-1"
                      >
                        <div className="flex w-full gap-0.5">
                          <div
                            className="flex-1 rounded-t-sm bg-green"
                            style={{ height: `${revHeight}%`, minHeight: "2px" }}
                          />
                          <div
                            className="flex-1 rounded-t-sm bg-red"
                            style={{ height: `${costHeight}%`, minHeight: "2px" }}
                          />
                          <div
                            className={`flex-1 rounded-t-sm ${
                              bucket.profit >= 0 ? "bg-primary" : "bg-amber"
                            }`}
                            style={{ height: `${profitHeight}%`, minHeight: "2px" }}
                          />
                        </div>
                        <span className="text-[9px] font-medium text-ink-400">
                          {bucket.label}
                        </span>
                      </div>
                    );
                  })}
                </div>
                <div className="mt-3 flex justify-center gap-4 text-xs text-ink-500">
                  <span className="flex items-center gap-1.5">
                    <span className="h-2.5 w-2.5 rounded-sm bg-green" />
                    Revenue
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="h-2.5 w-2.5 rounded-sm bg-red" />
                    Costs
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="h-2.5 w-2.5 rounded-sm bg-primary" />
                    Profit
                  </span>
                </div>
              </>
            )}
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-3">
              <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Breakdown
              </div>
              <div className="text-sm font-semibold text-ink-950">
                Costs by category
              </div>
            </div>
            {costBreakdown.length === 0 ? (
              <div className="py-8 text-center text-sm text-ink-500">
                No expenses recorded.
              </div>
            ) : (
              <div className="space-y-3">
                {costBreakdown.map((bucket) => {
                  const pct =
                    costTotal > 0 ? (bucket.amount / costTotal) * 100 : 0;
                  return (
                    <div key={bucket.label}>
                      <div className="flex items-center justify-between text-sm">
                        <span className="text-ink-600">{bucket.label}</span>
                        <span className="font-medium text-ink-950">
                          {formatMinor(bucket.amount, revenueByCurrency)}{" "}
                          <span className="ml-1 text-[11px] text-ink-400">
                            ({pct.toFixed(1)}%)
                          </span>
                        </span>
                      </div>
                      <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-surface-3">
                        <div
                          className="h-full rounded-full bg-red"
                          style={{ width: `${Math.min(pct, 100)}%` }}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Source counts
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Invoices</span>
                <span className="font-medium text-ink-950">
                  {invoices.length}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Expenses</span>
                <span className="font-medium text-ink-950">
                  {expenses.length}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Receivables</span>
                <span className="font-medium text-ink-950">
                  {receivables.length}
                </span>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="flex items-start gap-2.5">
              <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0 text-ink-400" />
              <div className="text-xs text-ink-500">
                Profitability is computed in the browser from invoices and
                expenses. The backend does not store precomputed margin.
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}