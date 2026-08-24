// src/app/(dashboard)/finance/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  LayoutDashboard,
  TrendingUp,
  TrendingDown,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Wallet,
  Users,
  Download,
  CreditCard,
  Receipt,
  Briefcase,
  PieChart,
  AlertTriangle,
  Coins,
  Scale,
  ChartNoAxesCombined,
} from "lucide-react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

type FinanceDashboardData = {
  summary: {
    totalRevenue: number;
    totalExpenses: number;
    netProfit: number;
    receivables: number;
    overdue: number;
    payrollTotal: number;
    budgetUtilization: number;
  };
  recentInvoices: {
    id: string;
    reference: string;
    customer: string;
    amount: number;
    status: "Paid" | "Pending" | "Overdue" | "Draft";
    dueDate: string;
    date: string;
  }[];
  monthlyTrend: {
    month: string;
    revenue: number;
    expenses: number;
  }[];
  payrollSummary: {
    period: string;
    totalAmount: number;
    providerCount: number;
    status: "Calculated" | "Approved" | "Posted" | "Pending";
  };
  reconciliationExceptions: {
    id: string;
    type: string;
    description: string;
    severity: "Critical" | "High" | "Medium" | "Low";
    timestamp: string;
  }[];
};

const MOCK_DATA: FinanceDashboardData = {
  summary: {
    totalRevenue: 18420000,
    totalExpenses: 8400000,
    netProfit: 10020000,
    receivables: 6840000,
    overdue: 1200000,
    payrollTotal: 2800000,
    budgetUtilization: 78,
  },
  recentInvoices: [
    { id: "INV-001", reference: "INV-2026-001", customer: "Charitos Hospital", amount: 1280000, status: "Paid", dueDate: "2026-08-15", date: "2026-08-01" },
    { id: "INV-002", reference: "INV-2026-002", customer: "Oakfield Stores", amount: 840000, status: "Pending", dueDate: "2026-08-30", date: "2026-08-10" },
    { id: "INV-003", reference: "INV-2026-003", customer: "Greenline Foods", amount: 2100000, status: "Overdue", dueDate: "2026-07-31", date: "2026-07-15" },
    { id: "INV-004", reference: "INV-2026-004", customer: "Solaris Construction", amount: 560000, status: "Draft", dueDate: "2026-09-15", date: "2026-08-20" },
  ],
  monthlyTrend: [
    { month: "Jan", revenue: 1200000, expenses: 800000 },
    { month: "Feb", revenue: 1400000, expenses: 850000 },
    { month: "Mar", revenue: 1600000, expenses: 900000 },
    { month: "Apr", revenue: 1500000, expenses: 880000 },
    { month: "May", revenue: 1800000, expenses: 950000 },
    { month: "Jun", revenue: 2000000, expenses: 1000000 },
    { month: "Jul", revenue: 2200000, expenses: 1050000 },
    { month: "Aug", revenue: 2400000, expenses: 1100000 },
  ],
  payrollSummary: {
    period: "August 2026",
    totalAmount: 2800000,
    providerCount: 12,
    status: "Calculated",
  },
  reconciliationExceptions: [
    { id: "REC-001", type: "Payment Gateway", description: "Invoice INV-002 payment pending confirmation", severity: "High", timestamp: "2 hours ago" },
    { id: "REC-002", type: "Wallet Sync", description: "Provider wallet balance mismatch for P-029", severity: "Medium", timestamp: "4 hours ago" },
    { id: "REC-003", type: "Payroll", description: "Payroll period approval pending", severity: "Low", timestamp: "1 day ago" },
  ],
};

const INVOICE_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Paid: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Overdue: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Draft: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  High: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Medium: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Low: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

export default function FinanceDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<FinanceDashboardData | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DATA);
      setLoading(false);
    };
    loadData();
  }, []);

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat("en-NG", {
      style: "currency",
      currency: "NGN",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading finance dashboard...</span>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <LayoutDashboard className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-sm font-semibold text-ink-950">Finance data unavailable</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load finance dashboard data.</p>
        <button className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
          <RefreshCw className="inline h-3.5 w-3.5 mr-1" />
          Refresh
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Finance Dashboard</h1>
          <p className="mt-1.5">
            Invoices, receivables, expenses, payroll, budgets, and reconciliation.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ── Summary Cards ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <TrendingUp className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Revenue</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatCurrency(data.summary.totalRevenue)}
          </div>
          <div className="mt-1.5 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            +12% this month
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <TrendingDown className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Expenses</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatCurrency(data.summary.totalExpenses)}
          </div>
          <div className="mt-1.5 flex items-center gap-1 text-[10px] text-red">
            <TrendingDown className="h-3 w-3" />
            +5% this month
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Wallet className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Net Profit</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatCurrency(data.summary.netProfit)}
          </div>
          <div className="mt-1.5 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            +18% this month
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <PieChart className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Budget Utilization</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {data.summary.budgetUtilization}%
          </div>
          <div className="mt-1.5 h-1.5 w-24 rounded-full bg-surface-3 overflow-hidden">
            <div className="h-full rounded-full bg-primary" style={{ width: `${data.summary.budgetUtilization}%` }} />
          </div>
        </div>
      </div>

      {/* ── Second Row ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div
          onClick={() => router.push("/finance/receivables")}
          className="rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2 cursor-pointer"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <CreditCard className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Receivables</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatCurrency(data.summary.receivables)}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">42 open invoices</div>
        </div>
        <div
          onClick={() => router.push("/finance/receivables?status=Overdue")}
          className="rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2 cursor-pointer"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Overdue</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">
            {formatCurrency(data.summary.overdue)}
          </div>
          <div className="mt-1.5 text-[10px] text-red">7 invoices overdue</div>
        </div>
        <div
          onClick={() => router.push("/finance/payroll")}
          className="rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2 cursor-pointer"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Payroll</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {formatCurrency(data.summary.payrollTotal)}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">{data.payrollSummary.status}</div>
        </div>
        <div
          onClick={() => router.push("/finance/reconciliation")}
          className="rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2 cursor-pointer"
        >
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Exceptions</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-amber">
            {data.reconciliationExceptions.length}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">Need attention</div>
        </div>
      </div>

      {/* ── Main Grid ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ── Left Column ── */}
        <div className="space-y-5">
          {/* Monthly Trend Chart — FULLY RESPONSIVE */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-4">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Trend</div>
                <div className="text-sm font-semibold text-ink-950">Revenue & Expenses</div>
              </div>
              <span className="text-xs text-ink-400">Last 8 months</span>
            </div>
            <div className="w-full min-h-[220px] h-[220px]">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={data.monthlyTrend} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#E5E7EB" />
                  <XAxis dataKey="month" tick={{ fontSize: 11, fill: "#8A8FA3" }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 11, fill: "#8A8FA3" }} axisLine={false} tickLine={false} tickFormatter={(v) => `₦${(v / 1000000).toFixed(1)}M`} />
                  <Tooltip
                    contentStyle={{
                      borderRadius: 8,
                      border: "1px solid #E5E7EB",
                      fontSize: 12,
                      boxShadow: "none",
                    }}
                    formatter={(value: number) => [`₦${(value / 1000000).toFixed(1)}M`, ""]}
                  />
                  <Bar dataKey="revenue" fill="#139a5d" radius={[4, 4, 0, 0]} maxBarSize={32} />
                  <Bar dataKey="expenses" fill="#c81e33" radius={[4, 4, 0, 0]} maxBarSize={32} />
                </BarChart>
              </ResponsiveContainer>
            </div>
            <div className="mt-3 flex justify-center gap-4 text-xs text-ink-500">
              <span className="flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-sm bg-green" />
                Revenue
              </span>
              <span className="flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-sm bg-red" />
                Expenses
              </span>
            </div>
          </div>

          {/* Recent Invoices */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Recent</div>
                <div className="text-sm font-semibold text-ink-950">Invoices</div>
              </div>
              <button
                onClick={() => router.push("/finance/invoices")}
                className="text-xs font-medium text-primary transition-colors hover:text-primary-hover flex items-center gap-1"
              >
                View all <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {data.recentInvoices.map((invoice) => {
                const statusStyle = INVOICE_STATUS_STYLES[invoice.status] || INVOICE_STATUS_STYLES.Draft;
                return (
                  <div
                    key={invoice.id}
                    className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition-colors hover:bg-surface-2"
                    onClick={() => router.push(`/finance/invoices/${invoice.id}`)}
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-xs text-ink-950">{invoice.reference}</span>
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {invoice.status}
                        </span>
                      </div>
                      <div className="text-xs text-ink-500">{invoice.customer}</div>
                    </div>
                    <div className="text-right">
                      <div className="text-sm font-semibold text-ink-950">{formatCurrency(invoice.amount)}</div>
                      <div className="text-[10px] text-ink-400">Due: {invoice.dueDate}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* ── Right Column ── */}
        <div className="space-y-5">
          {/* Quick Navigation */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500 mb-3">Navigate</div>
            <div className="grid grid-cols-2 gap-2">
              {[
                { label: "Invoices", icon: Receipt, path: "/finance/invoices" },
                { label: "Receivables", icon: CreditCard, path: "/finance/receivables" },
                { label: "Expenses", icon: Wallet, path: "/finance/expenses" },
                { label: "Payroll", icon: Users, path: "/finance/payroll" },
                { label: "Budgets", icon: PieChart, path: "/finance/budgets" },
                { label: "Reconciliation", icon: RefreshCw, path: "/finance/reconciliation" },
                { label: "Compensation", icon: Scale, path: "/finance/compensation-rules" },
                { label: "Wallet", icon: Coins, path: "/wallet" },
              ].map((item) => (
                <button
                  key={item.label}
                  onClick={() => router.push(item.path)}
                  className="flex items-center gap-2 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  <item.icon className="h-4 w-4 text-ink-400" />
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          {/* Payroll Summary */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Summary</div>
                <div className="text-sm font-semibold text-ink-950">Payroll</div>
              </div>
              <button
                onClick={() => router.push("/finance/payroll")}
                className="text-xs font-medium text-primary transition-colors hover:text-primary-hover flex items-center gap-1"
              >
                View <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Period</span>
                <span className="font-medium text-ink-950">{data.payrollSummary.period}</span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Total</span>
                <span className="font-semibold text-ink-950">{formatCurrency(data.payrollSummary.totalAmount)}</span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Providers</span>
                <span className="font-medium text-ink-950">{data.payrollSummary.providerCount}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Status</span>
                <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                  data.payrollSummary.status === "Posted" ? "bg-green-bg text-green" :
                  data.payrollSummary.status === "Approved" ? "bg-primary-bg text-primary" :
                  data.payrollSummary.status === "Calculated" ? "bg-amber-bg text-amber" :
                  "bg-surface-2 text-ink-500"
                }`}>
                  {data.payrollSummary.status}
                </span>
              </div>
            </div>
          </div>

          {/* Exceptions */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Issues</div>
                <div className="text-sm font-semibold text-ink-950">Exceptions</div>
              </div>
              <button
                onClick={() => router.push("/finance/reconciliation")}
                className="text-xs font-medium text-primary transition-colors hover:text-primary-hover flex items-center gap-1"
              >
                View all <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {data.reconciliationExceptions.map((exc) => {
                const severityStyle = SEVERITY_STYLES[exc.severity] || SEVERITY_STYLES.Medium;
                return (
                  <div
                    key={exc.id}
                    className="flex items-start gap-2.5 rounded-lg border border-line p-2.5 transition-colors hover:bg-surface-2"
                  >
                    <AlertTriangle className={`h-4 w-4 flex-shrink-0 mt-0.5 ${
                      exc.severity === "Critical" ? "text-red" :
                      exc.severity === "High" ? "text-amber" :
                      "text-primary"
                    }`} />
                    <div className="flex-1 min-w-0">
                      <div className="text-xs font-medium text-ink-950">{exc.description}</div>
                      <div className="flex items-center gap-2 text-[10px] text-ink-400">
                        <span>{exc.type}</span>
                        <span className="text-ink-300">·</span>
                        <span>{exc.timestamp}</span>
                      </div>
                    </div>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${severityStyle.dot}`} />
                      {exc.severity}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}