// src/app/(dashboard)/finance/profitability/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ChartNoAxesCombined,
  TrendingUp,
  TrendingDown,
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
  Download,
  FileText,
  Printer,
  DollarSign,
  CreditCard,
  Receipt,
  Briefcase,
  PieChart,
  BarChart3,
  Activity,
  AlertTriangle,
  Info,
  Building2,
  Landmark,
  HandCoins,
  Coins,
  Percent,
  Calculator,
  ChartBar,
  LineChart,
} from "lucide-react";

type ProfitabilityData = {
  period: string;
  currency: string;
  summary: {
    totalRevenue: number;
    totalCosts: number;
    grossProfit: number;
    grossMargin: number;
    operatingExpenses: number;
    operatingProfit: number;
    operatingMargin: number;
    netProfit: number;
    netMargin: number;
  };
  revenueBreakdown: {
    category: string;
    amount: number;
    percentage: number;
  }[];
  costBreakdown: {
    category: string;
    amount: number;
    percentage: number;
  }[];
  monthlyTrend: {
    month: string;
    revenue: number;
    costs: number;
    profit: number;
  }[];
  branchPerformance: {
    branch: string;
    revenue: number;
    costs: number;
    profit: number;
    margin: number;
  }[];
  servicePerformance: {
    service: string;
    revenue: number;
    costs: number;
    profit: number;
    margin: number;
  }[];
  topCustomers: {
    customer: string;
    revenue: number;
    share: number;
  }[];
  freshness: string;
};

// Mock data
const MOCK_DATA: ProfitabilityData = {
  period: "August 2026",
  currency: "NGN",
  summary: {
    totalRevenue: 18420000,
    totalCosts: 8400000,
    grossProfit: 10020000,
    grossMargin: 54.4,
    operatingExpenses: 2200000,
    operatingProfit: 7820000,
    operatingMargin: 42.5,
    netProfit: 7820000,
    netMargin: 42.5,
  },
  revenueBreakdown: [
    { category: "Dispatch Services", amount: 8200000, percentage: 44.5 },
    { category: "Hauling Services", amount: 5600000, percentage: 30.4 },
    { category: "Private Orders", amount: 2800000, percentage: 15.2 },
    { category: "Warehouse Services", amount: 1200000, percentage: 6.5 },
    { category: "Other Services", amount: 620000, percentage: 3.4 },
  ],
  costBreakdown: [
    { category: "Provider Compensation", amount: 3200000, percentage: 38.1 },
    { category: "Fuel", amount: 1800000, percentage: 21.4 },
    { category: "Maintenance", amount: 1200000, percentage: 14.3 },
    { category: "Payroll", amount: 1100000, percentage: 13.1 },
    { category: "Subcontractors", amount: 600000, percentage: 7.1 },
    { category: "Other", amount: 500000, percentage: 6.0 },
  ],
  monthlyTrend: [
    { month: "Jan", revenue: 1200000, costs: 800000, profit: 400000 },
    { month: "Feb", revenue: 1400000, costs: 850000, profit: 550000 },
    { month: "Mar", revenue: 1600000, costs: 900000, profit: 700000 },
    { month: "Apr", revenue: 1500000, costs: 880000, profit: 620000 },
    { month: "May", revenue: 1800000, costs: 950000, profit: 850000 },
    { month: "Jun", revenue: 2000000, costs: 1000000, profit: 1000000 },
    { month: "Jul", revenue: 2200000, costs: 1050000, profit: 1150000 },
    { month: "Aug", revenue: 2400000, costs: 1100000, profit: 1300000 },
  ],
  branchPerformance: [
    { branch: "Ikeja", revenue: 8200000, costs: 3800000, profit: 4400000, margin: 53.7 },
    { branch: "Lekki", revenue: 5600000, costs: 2600000, profit: 3000000, margin: 53.6 },
    { branch: "Apapa", revenue: 3400000, costs: 1600000, profit: 1800000, margin: 52.9 },
    { branch: "Yaba", revenue: 1200000, costs: 400000, profit: 800000, margin: 66.7 },
  ],
  servicePerformance: [
    { service: "Dispatch", revenue: 8200000, costs: 3600000, profit: 4600000, margin: 56.1 },
    { service: "Hauling", revenue: 5600000, costs: 2800000, profit: 2800000, margin: 50.0 },
    { service: "Private Ops", revenue: 2800000, costs: 1200000, profit: 1600000, margin: 57.1 },
    { service: "Warehouse", revenue: 1200000, costs: 500000, profit: 700000, margin: 58.3 },
  ],
  topCustomers: [
    { customer: "Charitos Hospital", revenue: 3400000, share: 18.5 },
    { customer: "Greenline Foods", revenue: 2800000, share: 15.2 },
    { customer: "Oakfield Stores", revenue: 1800000, share: 9.8 },
    { customer: "Solaris Construction", revenue: 1200000, share: 6.5 },
    { customer: "BlueMarket Retail", revenue: 800000, share: 4.3 },
  ],
  freshness: "12 sec ago",
};

const FORMAT_OPTIONS = ["This Month", "Last Month", "This Quarter", "Last Quarter", "This Year"];

export default function ProfitabilityPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProfitabilityData | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState("This Month");
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DATA);
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
        <p className="mt-3 text-sm text-ink-600">Loading profitability data...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <ChartNoAxesCombined className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Data unavailable</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load profitability data.</p>
        <button className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
          <RefreshCw className="inline h-3.5 w-3.5 mr-1" />
          Refresh
        </button>
      </div>
    );
  }

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat("en-NG", {
      style: "currency",
      currency: "NGN",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  };

  const formatPercentage = (value: number) => {
    return value.toFixed(1) + "%";
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Profitability & Commercial Reporting
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Company-private revenue/cost/reference metrics and source freshness.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ===== PERIOD SELECTOR ===== */}
      <div className="flex flex-wrap items-center gap-1 rounded-xl border border-line bg-white/70 p-1 shadow-sm">
        {FORMAT_OPTIONS.map((period) => (
          <button
            key={period}
            onClick={() => setSelectedPeriod(period)}
            className={`rounded-lg px-4 py-1.5 text-sm font-medium transition ${
              selectedPeriod === period
                ? "bg-primary text-white shadow-sm shadow-primary/20"
                : "text-ink-600 hover:bg-surface-2"
            }`}
          >
            {period}
          </button>
        ))}
        <div className="ml-auto flex items-center gap-2 text-sm text-ink-400">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </div>
      </div>

      {/* ===== SUMMARY CARDS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Revenue</div>
          <div className="text-lg font-bold text-ink-950">{formatCurrency(data.summary.totalRevenue)}</div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+12% vs last month</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Gross Profit</div>
          <div className="text-lg font-bold text-green">{formatCurrency(data.summary.grossProfit)}</div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <span className="font-semibold">{formatPercentage(data.summary.grossMargin)}</span>
            <span className="text-ink-400">margin</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Operating Profit</div>
          <div className="text-lg font-bold text-ink-950">{formatCurrency(data.summary.operatingProfit)}</div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-blue">
            <span className="font-semibold">{formatPercentage(data.summary.operatingMargin)}</span>
            <span className="text-ink-400">margin</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Net Profit</div>
          <div className="text-lg font-bold text-ink-950">{formatCurrency(data.summary.netProfit)}</div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <span className="font-semibold">{formatPercentage(data.summary.netMargin)}</span>
            <span className="text-ink-400">margin</span>
          </div>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Monthly Trend Chart */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-[15px] font-semibold text-ink-950">Monthly Performance Trend</h3>
              <span className="text-xs text-ink-400">Revenue · Costs · Profit</span>
            </div>
            <div className="h-[180px] w-full">
              <div className="flex h-full items-end gap-2">
                {data.monthlyTrend.map((item, i) => {
                  const maxValue = Math.max(
                    ...data.monthlyTrend.map(d => Math.max(d.revenue, d.costs, d.profit))
                  );
                  const revHeight = (item.revenue / maxValue) * 100;
                  const costHeight = (item.costs / maxValue) * 100;
                  const profitHeight = (item.profit / maxValue) * 100;
                  return (
                    <div key={i} className="flex flex-1 flex-col items-center gap-1">
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
                          className="flex-1 rounded-t-sm bg-blue"
                          style={{ height: `${profitHeight}%`, minHeight: "2px" }}
                        />
                      </div>
                      <span className="text-[9px] font-medium text-ink-400">{item.month}</span>
                    </div>
                  );
                })}
              </div>
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
                <span className="h-2.5 w-2.5 rounded-sm bg-blue" />
                Profit
              </span>
            </div>
          </div>

          {/* Branch Performance */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Branch Performance</h3>
              <span className="text-xs text-ink-400">Revenue · Profit · Margin</span>
            </div>
            <div className="space-y-3">
              {data.branchPerformance.map((branch) => (
                <div key={branch.branch} className="rounded-lg border border-line p-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="text-sm font-semibold text-ink-950">{branch.branch}</div>
                      <div className="text-[11px] text-ink-500">
                        Revenue: {formatCurrency(branch.revenue)}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="text-sm font-semibold text-ink-950">
                        {formatCurrency(branch.profit)}
                      </div>
                      <div className={`text-[11px] font-medium ${
                        branch.margin >= 50 ? "text-green" :
                        branch.margin >= 30 ? "text-amber" :
                        "text-red"
                      }`}>
                        {formatPercentage(branch.margin)} margin
                      </div>
                    </div>
                  </div>
                  <div className="mt-2 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className="h-full rounded-full bg-primary"
                      style={{ width: `${branch.margin}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Revenue Breakdown */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Revenue Breakdown</h3>
            <div className="space-y-3">
              {data.revenueBreakdown.map((item) => (
                <div key={item.category}>
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-600">{item.category}</span>
                    <span className="font-medium text-ink-950">
                      {formatCurrency(item.amount)}
                      <span className="ml-1 text-ink-400 text-[11px]">({formatPercentage(item.percentage)})</span>
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className="h-full rounded-full bg-green"
                      style={{ width: `${item.percentage}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Cost Breakdown */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Cost Breakdown</h3>
            <div className="space-y-3">
              {data.costBreakdown.map((item) => (
                <div key={item.category}>
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-ink-600">{item.category}</span>
                    <span className="font-medium text-ink-950">
                      {formatCurrency(item.amount)}
                      <span className="ml-1 text-ink-400 text-[11px]">({formatPercentage(item.percentage)})</span>
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className="h-full rounded-full bg-red"
                      style={{ width: `${item.percentage}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Top Customers */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Top Customers</h3>
              <span className="text-xs text-ink-400">By revenue</span>
            </div>
            <div className="space-y-2">
              {data.topCustomers.map((customer, index) => (
                <div key={customer.customer} className="flex items-center gap-3 rounded-lg border border-line p-2.5">
                  <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-primary-bg text-xs font-bold text-primary">
                    {index + 1}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-ink-950 truncate">{customer.customer}</div>
                    <div className="text-[11px] text-ink-500">{formatCurrency(customer.revenue)}</div>
                  </div>
                  <div className="text-sm font-semibold text-ink-950">
                    {formatPercentage(customer.share)}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Service Performance */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Service Performance</h3>
            <div className="space-y-2">
              {data.servicePerformance.map((service) => (
                <div key={service.service} className="flex items-center justify-between rounded-lg border border-line p-2.5">
                  <div>
                    <div className="text-sm font-medium text-ink-950">{service.service}</div>
                    <div className="text-[11px] text-ink-500">{formatCurrency(service.revenue)}</div>
                  </div>
                  <div className="text-right">
                    <div className="text-sm font-semibold text-ink-950">
                      {formatCurrency(service.profit)}
                    </div>
                    <div className={`text-[11px] font-medium ${
                      service.margin >= 50 ? "text-green" :
                      service.margin >= 30 ? "text-amber" :
                      "text-red"
                    }`}>
                      {formatPercentage(service.margin)}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <ChartNoAxesCombined className="h-4 w-4" />
          FIN-17 · Profitability & Commercial Reporting
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}