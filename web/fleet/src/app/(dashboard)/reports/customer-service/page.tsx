// src/app/(dashboard)/reports/customer-service/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Download,
  Calendar,
  Users,
  FileText,
  AlertTriangle,
  ArrowUp,
  ArrowDown,
  ArrowRight,
  Target,
  Eye,
  Clock,
} from "lucide-react";

// ==================== TYPES ====================

type CustomerMetric = {
  label: string;
  value: string;
  change: number;
  trend: "up" | "down" | "neutral";
  icon: React.ReactNode;
  color: "blue" | "green" | "amber" | "red" | "purple";
  subtitle: string;
};

type CustomerActivity = {
  id: string;
  name: string;
  segment: string;
  orders: number;
  revenue: string;
  lastActivity: string;
  slaStatus: "MET" | "BREACHED" | "UNKNOWN";
};

type SLAStatus = {
  metric: string;
  target: string;
  actual: string;
  result: "MET" | "BREACHED" | "UNKNOWN";
  customer: string;
  period: string;
};

// ==================== MOCK DATA ====================

const MOCK_CUSTOMER_METRICS: CustomerMetric[] = [
  {
    label: "Active Customers",
    value: "428",
    change: 19,
    trend: "up",
    icon: <Users className="h-5 w-5" />,
    color: "blue",
    subtitle: "Company commercial relationships",
  },
  {
    label: "Open Quotations",
    value: "31",
    change: 31,
    trend: "neutral",
    icon: <FileText className="h-5 w-5" />,
    color: "green",
    subtitle: "₦12.4m quoted value",
  },
  {
    label: "Renewals Due",
    value: "14",
    change: 14,
    trend: "up",
    icon: <Calendar className="h-5 w-5" />,
    color: "amber",
    subtitle: "Next 30 days",
  },
  {
    label: "SLA Risks",
    value: "7",
    change: 7,
    trend: "down",
    icon: <AlertTriangle className="h-5 w-5" />,
    color: "red",
    subtitle: "Commercial attention",
  },
];

const MOCK_CUSTOMER_ACTIVITY: CustomerActivity[] = [
  {
    id: "C-003",
    name: "Charitos Hospital",
    segment: "Corporate",
    orders: 46,
    revenue: "₦4.2m",
    lastActivity: "2026-08-22T11:00:00Z",
    slaStatus: "BREACHED",
  },
  {
    id: "C-004",
    name: "Greenline Foods",
    segment: "Enterprise",
    orders: 74,
    revenue: "₦6.9m",
    lastActivity: "2026-08-22T10:00:00Z",
    slaStatus: "MET",
  },
  {
    id: "C-002",
    name: "Oakfield Stores",
    segment: "SME",
    orders: 27,
    revenue: "₦1.8m",
    lastActivity: "2026-08-21T16:00:00Z",
    slaStatus: "BREACHED",
  },
  {
    id: "C-005",
    name: "Solaris Construction",
    segment: "Corporate",
    orders: 18,
    revenue: "₦3.1m",
    lastActivity: "2026-08-20T14:00:00Z",
    slaStatus: "UNKNOWN",
  },
];

const MOCK_SLA_STATUS: SLAStatus[] = [
  {
    metric: "On-Time Delivery",
    target: "98%",
    actual: "96.4%",
    result: "BREACHED",
    customer: "Charitos Hospital",
    period: "Q3 2026",
  },
  {
    metric: "Damage Rate",
    target: "<1%",
    actual: "0.8%",
    result: "MET",
    customer: "Charitos Hospital",
    period: "Q3 2026",
  },
  {
    metric: "On-Time Delivery",
    target: "97%",
    actual: "91.2%",
    result: "BREACHED",
    customer: "Oakfield Stores",
    period: "Q3 2026",
  },
  {
    metric: "Customer Satisfaction",
    target: "4.5/5",
    actual: "4.2/5",
    result: "BREACHED",
    customer: "Oakfield Stores",
    period: "Q3 2026",
  },
  {
    metric: "On-Time Delivery",
    target: "99%",
    actual: "99.1%",
    result: "MET",
    customer: "Greenline Foods",
    period: "Q3 2026",
  },
];

type SlaStyle = {
  bg: string;
  text: string;
  label: string;
};

const getSlaStatusStyle = (status: string): SlaStyle => {
  switch (status) {
    case "MET":
      return { bg: "bg-green-bg", text: "text-green", label: "MET" };
    case "BREACHED":
      return { bg: "bg-red-bg", text: "text-red", label: "BREACHED" };
    case "UNKNOWN":
      return { bg: "bg-amber-bg", text: "text-amber", label: "UNKNOWN" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500", label: status };
  }
};

const COLOR_CLASSES: Record<string, string> = {
  blue: "bg-blue-bg text-blue",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
  purple: "bg-purple-bg text-purple",
};

export default function CustomerServiceDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [selectedPeriod, setSelectedPeriod] = useState<string>("Q3 2026");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
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
        <p className="mt-3 text-sm text-ink-600">Loading dashboard...</p>
      </div>
    );
  }

  const formatTime = (date: string): string => {
    const d = new Date(date);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Customer & Service Dashboard
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Volume, SLA results, complaints/quality, receivable references and service activity.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Calendar className="h-3.5 w-3.5" />
            {selectedPeriod} ▾
          </button>
        </div>
      </div>

      {/* ===== METRICS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {MOCK_CUSTOMER_METRICS.map((metric) => (
          <div key={metric.label} className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
            <div className="flex items-center justify-between">
              <div className={`rounded-full p-2 ${COLOR_CLASSES[metric.color]}`}>
                {metric.icon}
              </div>
              <div className={`flex items-center gap-0.5 text-xs font-semibold ${
                metric.trend === "up" ? "text-green" :
                metric.trend === "down" ? "text-red" :
                "text-ink-500"
              }`}>
                {metric.trend === "up" && <ArrowUp className="h-3 w-3" />}
                {metric.trend === "down" && <ArrowDown className="h-3 w-3" />}
                {metric.change}%
              </div>
            </div>
            <div className="mt-2 text-lg font-bold text-ink-950">{metric.value}</div>
            <div className="text-[10px] font-medium text-ink-500">{metric.label}</div>
            <div className="text-[9px] text-ink-400">{metric.subtitle}</div>
          </div>
        ))}
      </div>

      {/* ===== CUSTOMER ACTIVITY TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Users className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Customer Activity</span>
            <span className="text-xs text-ink-500">Recent commercial activity</span>
          </div>
          <button
            onClick={() => router.push("/crm/customers")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Customer</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Segment</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Orders</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Revenue</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Last Activity</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">SLA</th>
              </tr>
            </thead>
            <tbody>
              {MOCK_CUSTOMER_ACTIVITY.map((customer) => {
                const slaStyle = getSlaStatusStyle(customer.slaStatus);
                return (
                  <tr
                    key={customer.id}
                    onClick={() => router.push(`/crm/customers/${customer.id}`)}
                    className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950 text-xs">{customer.name}</div>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{customer.segment}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{customer.orders}</td>
                    <td className="px-4 py-3 text-xs font-medium text-ink-950">{customer.revenue}</td>
                    <td className="px-4 py-3 text-xs text-ink-500">{formatTime(customer.lastActivity)}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${slaStyle.bg} ${slaStyle.text}`}>
                        {slaStyle.label}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== SLA STATUS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Target className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">SLA Status</span>
            <span className="text-xs text-ink-500">Service Level Agreement results</span>
          </div>
          <button
            onClick={() => router.push("/comms/sla")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="divide-y divide-line">
          {MOCK_SLA_STATUS.map((item, index) => {
            const resultStyle = getSlaStatusStyle(item.result);
            return (
              <div
                key={index}
                className="flex items-center justify-between p-3 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-ink-950">{item.customer}</span>
                    <span className="text-[10px] text-ink-500">{item.metric}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${resultStyle.bg} ${resultStyle.text}`}>
                      {resultStyle.label}
                    </span>
                  </div>
                  <div className="mt-0.5 text-[10px] text-ink-500">
                    Target: {item.target} · Actual: {item.actual} · {item.period}
                  </div>
                </div>
                <button className="text-ink-400 hover:text-primary transition-colors">
                  <Eye className="h-4 w-4" />
                </button>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Users className="h-4 w-4" />
          RPT-04 · Customer & Service Dashboard
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Period: {selectedPeriod}
        </span>
      </div>
    </div>
  );
}