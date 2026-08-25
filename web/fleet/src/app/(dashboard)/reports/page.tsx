// src/app/(dashboard)/reports/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Download,
  Calendar,
  ArrowUp,
  ArrowDown,
  ArrowRight,
  Activity,
  AlertTriangle,
  Building2,
  Eye,
  Clock,
  LayoutDashboard,
  Users,
  Car,
} from "lucide-react";

// ==================== TYPES ====================

type Metric = {
  label: string;
  value: string;
  change: number;
  trend: "up" | "down" | "neutral";
  icon: React.ReactNode;
  color: "blue" | "green" | "amber" | "red" | "purple";
  subtitle: string;
};

type ActiveWork = {
  id: string;
  reference: string;
  type: string;
  provider: string;
  location: string;
  status: "InProgress" | "AtPickup" | "EnRoute" | "Delivered";
  progress: number;
  updatedAt: string;
};

type BranchPerformance = {
  name: string;
  completionRate: number;
  activeCount: number;
  trend: "up" | "down" | "neutral";
};

type Exception = {
  id: string;
  type: "Critical" | "High" | "Medium" | "Low";
  title: string;
  source: string;
  time: string;
};

// ==================== MOCK DATA ====================

const MOCK_METRICS: Metric[] = [
  {
    label: "Active Work",
    value: "128",
    change: 12,
    trend: "up",
    icon: <Activity className="h-5 w-5" />,
    color: "blue",
    subtitle: "Across 4 branches",
  },
  {
    label: "Available Providers",
    value: "84",
    change: 91,
    trend: "up",
    icon: <Users className="h-5 w-5" />,
    color: "green",
    subtitle: "12 currently busy",
  },
  {
    label: "Fleet Readiness",
    value: "96",
    change: 88,
    trend: "neutral",
    icon: <Car className="h-5 w-5" />,
    color: "amber",
    subtitle: "7 safety / maintenance blocks",
  },
  {
    label: "Open Exceptions",
    value: "14",
    change: -3,
    trend: "down",
    icon: <AlertTriangle className="h-5 w-5" />,
    color: "red",
    subtitle: "3 critical",
  },
];

const MOCK_ACTIVE_WORK: ActiveWork[] = [
  {
    id: "KG-8491",
    reference: "KG-8491",
    type: "Dispatch",
    provider: "P. Okafor",
    location: "Ikeja → Lekki",
    status: "InProgress",
    progress: 66,
    updatedAt: "2026-08-22T14:30:00Z",
  },
  {
    id: "KG-8487",
    reference: "KG-8487",
    type: "Hauling",
    provider: "M. Bello",
    location: "Apapa → Ikoyi",
    status: "AtPickup",
    progress: 42,
    updatedAt: "2026-08-22T14:15:00Z",
  },
  {
    id: "KG-8479",
    reference: "KG-8479",
    type: "Dispatch",
    provider: "A. Yusuf",
    location: "Yaba → VI",
    status: "EnRoute",
    progress: 18,
    updatedAt: "2026-08-22T14:00:00Z",
  },
  {
    id: "KG-8470",
    reference: "KG-8470",
    type: "Dispatch",
    provider: "C. Uche",
    location: "Ikeja → Surulere",
    status: "Delivered",
    progress: 100,
    updatedAt: "2026-08-22T13:45:00Z",
  },
];

const MOCK_BRANCHES: BranchPerformance[] = [
  { name: "Ikeja", completionRate: 96.4, activeCount: 38, trend: "up" },
  { name: "Lekki", completionRate: 94.9, activeCount: 32, trend: "up" },
  { name: "Yaba", completionRate: 91.7, activeCount: 28, trend: "neutral" },
  { name: "Apapa", completionRate: 89.2, activeCount: 30, trend: "down" },
];

const MOCK_EXCEPTIONS: Exception[] = [
  { id: "EX-001", type: "Critical", title: "Vehicle BR-204 safety block", source: "Inspection Failed", time: "1h ago" },
  { id: "EX-002", type: "High", title: "Provider P-044 decline rate 12%", source: "Performance", time: "3h ago" },
  { id: "EX-003", type: "Medium", title: "Trip KG-8491 SLA breach", source: "Dispatch", time: "4h ago" },
  { id: "EX-004", type: "Low", title: "Document expiry - P-052", source: "Compliance", time: "2d ago" },
];

const COLOR_CLASSES: Record<string, string> = {
  blue: "bg-blue-bg text-blue",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
  purple: "bg-purple-bg text-purple",
};

export default function ExecutiveDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [selectedBranch, setSelectedBranch] = useState<string>("All branches");
  const [timeRange, setTimeRange] = useState<string>("Today");

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

  const getStatusLabel = (status: string): string => {
    switch (status) {
      case "InProgress": return "In Transit";
      case "AtPickup": return "At Pickup";
      case "EnRoute": return "En Route";
      case "Delivered": return "Delivered";
      default: return status;
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Executive Dashboard
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            High-level company operations, active work, exceptions, service performance and branch filters.
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
            {timeRange} ▾
          </button>
        </div>
      </div>

      {/* ===== METRICS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {MOCK_METRICS.map((metric) => (
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

      {/* ===== ACTIVE WORK + EXCEPTIONS ===== */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[2fr_1fr]">
        {/* Active Work */}
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <div className="flex items-center justify-between p-4 border-b border-line">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-ink-400" />
              <span className="font-semibold text-ink-950">Active Work</span>
              <span className="text-xs text-ink-500">Most recent trips</span>
            </div>
            <button
              onClick={() => router.push("/operations/trips")}
              className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
            >
              View all <ArrowRight className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="divide-y divide-line">
            {MOCK_ACTIVE_WORK.map((work) => (
              <div
                key={work.id}
                onClick={() => router.push(`/operations/trips/${work.id}`)}
                className="flex items-center justify-between p-3 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-medium text-ink-950">{work.reference}</span>
                    <span className="text-[10px] text-ink-500">{work.type}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-medium ${
                      work.status === "Delivered" ? "bg-green-bg text-green" :
                      work.status === "InProgress" ? "bg-blue-bg text-blue" :
                      work.status === "AtPickup" ? "bg-amber-bg text-amber" :
                      "bg-surface-2 text-ink-500"
                    }`}>
                      {getStatusLabel(work.status)}
                    </span>
                  </div>
                  <div className="mt-0.5 text-xs text-ink-500">
                    {work.provider} · {work.location}
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <div className="w-16">
                    <div className="h-1.5 w-full rounded-full bg-surface-2 overflow-hidden">
                      <div
                        className={`h-full rounded-full ${
                          work.progress >= 80 ? "bg-green" :
                          work.progress >= 50 ? "bg-blue" :
                          "bg-amber"
                        }`}
                        style={{ width: `${work.progress}%` }}
                      />
                    </div>
                  </div>
                  <span className="text-xs font-medium text-ink-950">{work.progress}%</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Exceptions */}
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <div className="flex items-center justify-between p-4 border-b border-line">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-ink-400" />
              <span className="font-semibold text-ink-950">Exceptions</span>
              <span className="rounded-full bg-red-bg px-2 py-0.5 text-[9px] font-semibold text-red">
                {MOCK_EXCEPTIONS.filter(e => e.type === "Critical" || e.type === "High").length} urgent
              </span>
            </div>
            <button
              onClick={() => router.push("/operations/control-tower/exceptions")}
              className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
            >
              View all <ArrowRight className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="divide-y divide-line">
            {MOCK_EXCEPTIONS.map((exception) => {
              const severityColor = {
                Critical: "bg-red-bg text-red",
                High: "bg-amber-bg text-amber",
                Medium: "bg-blue-bg text-blue",
                Low: "bg-surface-2 text-ink-500",
              }[exception.type] || "bg-surface-2 text-ink-500";

              return (
                <div
                  key={exception.id}
                  className="flex items-center justify-between p-3 hover:bg-surface-2 transition-colors cursor-pointer"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${severityColor}`}>
                        {exception.type}
                      </span>
                      <span className="text-xs font-medium text-ink-950 truncate">{exception.title}</span>
                    </div>
                    <div className="mt-0.5 text-[10px] text-ink-500">
                      {exception.source} · {exception.time}
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
      </div>

      {/* ===== BRANCH PERFORMANCE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Building2 className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Branch Performance</span>
            <span className="text-xs text-ink-500">Current period</span>
          </div>
          <button className="text-xs font-medium text-primary hover:underline flex items-center gap-1">
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 divide-y sm:divide-y-0 sm:divide-x divide-line">
          {MOCK_BRANCHES.map((branch) => (
            <div key={branch.name} className="p-4 text-center">
              <div className="text-sm font-semibold text-ink-950">{branch.name}</div>
              <div className="mt-1 text-2xl font-bold text-ink-950">{branch.completionRate}%</div>
              <div className="text-xs text-ink-500">{branch.activeCount} active</div>
              <div className={`mt-1 text-xs font-medium flex items-center justify-center gap-0.5 ${
                branch.trend === "up" ? "text-green" :
                branch.trend === "down" ? "text-red" :
                "text-ink-500"
              }`}>
                {branch.trend === "up" && <ArrowUp className="h-3 w-3" />}
                {branch.trend === "down" && <ArrowDown className="h-3 w-3" />}
                {branch.trend === "up" ? "Improving" : branch.trend === "down" ? "Declining" : "Stable"}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <LayoutDashboard className="h-4 w-4" />
          RPT-01 · Executive Dashboard
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Branch: {selectedBranch}
        </span>
      </div>
    </div>
  );
}