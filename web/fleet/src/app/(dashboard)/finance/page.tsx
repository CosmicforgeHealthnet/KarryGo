"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  TrendingUp,
  TrendingDown,
  Wallet,
  Users,
  Download,
  CreditCard,
  Receipt,
  PieChart,
  AlertTriangle,
  Coins,
  Scale,
  RefreshCw,
  ChartNoAxesCombined,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type {
  InvoiceView,
  ReceivableRow,
  ExpenseRow,
  PayrollPeriodRow,
  BudgetRow,
  Projection,
} from "@/lib/types/finance";
import {
  formatMinor,
  formatMoney,
  invoiceStatusLabel,
  invoiceStatusTone,
  timeAgo,
} from "@/lib/utils/financeLabels";

interface Summary {
  totalRevenue: number;
  totalExpenses: number;
  netProfit: number;
  receivables: number;
  overdue: number;
  payrollTotal: number;
  budgetUtilization: number;
  currency: string;
}

const CURRENCY_FALLBACK = "NGN";

export default function FinanceDashboardPage() {
  const router = useRouter();

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const [invoices, setInvoices] = useState<InvoiceView[]>([]);
  const [receivables, setReceivables] = useState<ReceivableRow[]>([]);
  const [expenses, setExpenses] = useState<ExpenseRow[]>([]);
  const [payrolls, setPayrolls] = useState<PayrollPeriodRow[]>([]);
  const [budgets, setBudgets] = useState<BudgetRow[]>([]);
  const [exceptions, setExceptions] = useState<Projection[]>([]);

  const fetchAll = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const [
          invoicesRes,
          receivablesRes,
          expensesRes,
          payrollsRes,
          budgetsRes,
          exceptionsRes,
        ] = await Promise.all([
          financeService.invoices.list(),
          financeService.receivables.list(),
          financeService.expenses.list(),
          financeService.payrollPeriods.list(),
          financeService.budgets.list(),
          financeService.reconciliation
            .listCases({ limit: 5 })
            .catch(() => ({ data: [] as Projection[] })),
        ]);
        setInvoices(invoicesRes.data);
        setReceivables(receivablesRes.data);
        setExpenses(expensesRes.data);
        setPayrolls(payrollsRes.data);
        setBudgets(budgetsRes.data);
        setExceptions(exceptionsRes.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load finance data.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    []
  );

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const summary: Summary = (() => {
    const currency =
      invoices[0]?.currency ??
      receivables[0]?.currency ??
      expenses[0]?.currency ??
      CURRENCY_FALLBACK;

    const totalRevenue = invoices
      .filter((i) => i.status !== "CANCELLED" && i.status !== "WRITTEN_OFF")
      .reduce((sum, i) => sum + i.total_minor, 0);

    const totalExpenses = expenses
      .filter((e) => e.status !== "CANCELLED" && e.status !== "REJECTED")
      .reduce((sum, e) => sum + e.amount_minor, 0);

    const receivablesTotal = receivables.reduce(
      (sum, r) => sum + r.outstanding_amount_minor,
      0
    );

    const overdue = receivables
      .filter((r) => r.status === "OVERDUE")
      .reduce((sum, r) => sum + r.outstanding_amount_minor, 0);

    const payrollTotal = payrolls
      .filter(
        (p) => p.status === "APPROVED" || p.status === "POSTED"
      )
      .reduce((sum, p) => sum + 0, 0);

    const approvedBudget = budgets
      .filter((b) => b.status === "APPROVED" || b.status === "ACTIVE")
      .reduce((sum, b) => sum + b.approved_amount_minor, 0);
    const actualBudget = budgets
      .filter((b) => b.status === "APPROVED" || b.status === "ACTIVE")
      .reduce((sum, b) => sum + b.actual_amount_projection, 0);
    const budgetUtilization =
      approvedBudget > 0 ? Math.round((actualBudget / approvedBudget) * 100) : 0;

    return {
      totalRevenue,
      totalExpenses,
      netProfit: totalRevenue - totalExpenses,
      receivables: receivablesTotal,
      overdue,
      payrollTotal,
      budgetUtilization,
      currency,
    };
  })();

  const recentInvoices = [...invoices]
    .sort((a, b) => b.created_at.localeCompare(a.created_at))
    .slice(0, 4);

  const currentPayroll = payrolls.find(
    (p) => p.status === "CALCULATED" || p.status === "APPROVED"
  );

  if (loading && invoices.length === 0) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">
            Loading finance dashboard…
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Finance Dashboard
          </h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Invoices, receivables, expenses, payroll, budgets, and reconciliation.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => fetchAll({ silent: true })}
            disabled={refreshing}
            className="flex h-10 w-10 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
        </div>
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

      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <TrendingUp className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Revenue</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatMinor(summary.totalRevenue, summary.currency)}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <TrendingDown className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Expenses</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatMinor(summary.totalExpenses, summary.currency)}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Wallet className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Net Profit</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatMinor(summary.netProfit, summary.currency)}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <PieChart className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">
              Budget utilization
            </span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {summary.budgetUtilization}%
          </div>
          <div className="mt-1.5 h-1.5 w-24 rounded-full bg-surface-3 overflow-hidden">
            <div
              className="h-full rounded-full bg-primary"
              style={{ width: `${summary.budgetUtilization}%` }}
            />
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <button
          onClick={() => router.push("/finance/receivables")}
          className="rounded-lg border border-line bg-surface p-4 text-left transition hover:bg-surface-2"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <CreditCard className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Receivables</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatMinor(summary.receivables, summary.currency)}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">
            {receivables.filter((r) => r.status !== "PAID").length} open
          </div>
        </button>
        <button
          onClick={() => router.push("/finance/receivables")}
          className="rounded-lg border border-line bg-surface p-4 text-left transition hover:bg-surface-2"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Overdue</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">
            {formatMinor(summary.overdue, summary.currency)}
          </div>
          <div className="mt-1.5 text-[10px] text-red">
            {receivables.filter((r) => r.status === "OVERDUE").length} invoices
          </div>
        </button>
        <button
          onClick={() => router.push("/finance/payroll")}
          className="rounded-lg border border-line bg-surface p-4 text-left transition hover:bg-surface-2"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Payroll</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {currentPayroll ? currentPayroll.currency : "—"}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">
            {currentPayroll ? currentPayroll.status : "No open period"}
          </div>
        </button>
        <button
          onClick={() => router.push("/finance/reconciliation")}
          className="rounded-lg border border-line bg-surface p-4 text-left transition hover:bg-surface-2"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Exceptions</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-amber">
            {exceptions.length}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">Need attention</div>
        </button>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-lg border border-line bg-surface p-5">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Recent
                </div>
                <div className="text-sm font-semibold text-ink-950">Invoices</div>
              </div>
              <button
                onClick={() => router.push("/finance/invoices")}
                className="flex items-center gap-1 text-xs font-medium text-primary transition hover:text-primary-hover"
              >
                View all →
              </button>
            </div>
            {recentInvoices.length === 0 ? (
              <div className="py-8 text-center text-sm text-ink-500">
                No invoices yet.
              </div>
            ) : (
              <div className="space-y-2">
                {recentInvoices.map((invoice) => {
                  const statusTone = invoiceStatusTone(invoice.status);
                  return (
                    <div
                      key={invoice.id}
                      onClick={() =>
                        router.push(`/finance/invoices/${invoice.id}`)
                      }
                      className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2"
                    >
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-ink-950">
                            {invoice.id}
                          </span>
                          <span
                            className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                          >
                            <span
                              className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`}
                            />
                            {invoiceStatusLabel(invoice.status)}
                          </span>
                        </div>
                        <div className="truncate text-xs text-ink-500">
                          {invoice.customer_id}
                        </div>
                      </div>
                      <div className="text-right">
                        <div className="text-sm font-semibold text-ink-950">
                          {formatMinor(invoice.total_minor, invoice.currency)}
                        </div>
                        <div className="text-[10px] text-ink-400">
                          Due {invoice.due_date}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          <div className="rounded-lg border border-line bg-surface p-5">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Recent
                </div>
                <div className="text-sm font-semibold text-ink-950">Expenses</div>
              </div>
              <button
                onClick={() => router.push("/finance/expenses")}
                className="flex items-center gap-1 text-xs font-medium text-primary transition hover:text-primary-hover"
              >
                View all →
              </button>
            </div>
            {expenses.length === 0 ? (
              <div className="py-8 text-center text-sm text-ink-500">
                No expenses yet.
              </div>
            ) : (
              <div className="space-y-2">
                {expenses.slice(0, 4).map((expense) => (
                  <div
                    key={expense.id}
                    onClick={() =>
                      router.push(`/finance/expenses/${expense.id}`)
                    }
                    className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2"
                  >
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-ink-950 truncate">
                        {expense.description}
                      </div>
                      <div className="text-[11px] text-ink-500">
                        {expense.cost_centre} · {expense.category}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="text-sm font-semibold text-ink-950">
                        {formatMinor(expense.amount_minor, expense.currency)}
                      </div>
                      <div className="text-[10px] text-ink-400">
                        {timeAgo(expense.created_at)}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="space-y-5">
          <div className="rounded-lg border border-line bg-surface p-5">
            <div className="mb-3 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              Navigate
            </div>
            <div className="grid grid-cols-2 gap-2">
              {[
                { label: "Invoices", icon: Receipt, path: "/finance/invoices" },
                { label: "Receivables", icon: CreditCard, path: "/finance/receivables" },
                { label: "Expenses", icon: Wallet, path: "/finance/expenses" },
                { label: "Payroll", icon: Users, path: "/finance/payroll" },
                { label: "Budgets", icon: PieChart, path: "/finance/budgets" },
                { label: "Reconciliation", icon: RefreshCw, path: "/finance/reconciliation" },
                { label: "Compensation", icon: Scale, path: "/finance/compensation-rules" },
                { label: "Settlements", icon: Coins, path: "/finance/settlements" },
              ].map((item) => (
                <button
                  key={item.label}
                  onClick={() => router.push(item.path)}
                  className="flex items-center gap-2 rounded-md border border-line bg-surface px-3 py-2 text-xs font-medium text-ink-700 transition hover:bg-surface-2"
                >
                  <item.icon className="h-4 w-4 text-ink-400" />
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          <div className="rounded-lg border border-line bg-surface p-5">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Issues
                </div>
                <div className="text-sm font-semibold text-ink-950">
                  Reconciliation
                </div>
              </div>
              <button
                onClick={() => router.push("/finance/reconciliation")}
                className="text-xs font-medium text-primary transition hover:text-primary-hover"
              >
                View all →
              </button>
            </div>
            {exceptions.length === 0 ? (
              <div className="py-6 text-center text-sm text-ink-500">
                No open cases.
              </div>
            ) : (
              <div className="space-y-2">
                {exceptions.slice(0, 4).map((proj) => (
                  <div
                    key={proj.id}
                    className="flex items-start gap-2.5 rounded-lg border border-line p-2.5"
                  >
                    <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber" />
                    <div className="min-w-0 flex-1">
                      <div className="text-xs font-medium text-ink-950">
                        {proj.projection_type} · {proj.safe_status}
                      </div>
                      <div className="flex items-center gap-2 text-[10px] text-ink-400">
                        <span>{proj.wallet_reference}</span>
                        <span>·</span>
                        <span>{timeAgo(proj.last_synced_at)}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}