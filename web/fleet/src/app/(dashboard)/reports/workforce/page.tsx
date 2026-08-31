// src/app/(dashboard)/reports/workforce/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Download,
  Building2,
  Users,
  Briefcase,
  ArrowUp,
  ArrowDown,
  ArrowRight,
  Clock,
  UserCheck,
  UserX,
  BookOpen,
  Eye,
} from "lucide-react";

// ==================== TYPES ====================

type WorkforceMetric = {
  label: string;
  value: string;
  change: number;
  trend: "up" | "down" | "neutral";
  icon: React.ReactNode;
  color: "blue" | "green" | "amber" | "red" | "purple";
  subtitle: string;
};

type ProviderStatus = {
  id: string;
  name: string;
  role: string;
  branch: string;
  availability: "Online" | "Busy" | "Offline" | "Leave";
  eligibility: "Eligible" | "Blocked" | "Training";
  shift: string;
  utilization: number;
};

type TrainingItem = {
  id: string;
  provider: string;
  course: string;
  status: "Pending" | "InProgress" | "Completed" | "Expired";
  dueDate: string;
  completionDate: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_WORKFORCE_METRICS: WorkforceMetric[] = [
  {
    label: "Total Providers",
    value: "146",
    change: 8,
    trend: "up",
    icon: <Users className="h-5 w-5" />,
    color: "blue",
    subtitle: "Across 4 branches",
  },
  {
    label: "Available Now",
    value: "84",
    change: 58,
    trend: "neutral",
    icon: <UserCheck className="h-5 w-5" />,
    color: "green",
    subtitle: "58% of active providers",
  },
  {
    label: "On Leave",
    value: "12",
    change: 12,
    trend: "up",
    icon: <UserX className="h-5 w-5" />,
    color: "amber",
    subtitle: "6 approved, 6 pending",
  },
  {
    label: "Training Due",
    value: "9",
    change: 9,
    trend: "down",
    icon: <BookOpen className="h-5 w-5" />,
    color: "red",
    subtitle: "Expiring within 30 days",
  },
];

const MOCK_PROVIDERS: ProviderStatus[] = [
  {
    id: "P-044",
    name: "Peter Okafor",
    role: "Dispatch Rider",
    branch: "Ikeja",
    availability: "Online",
    eligibility: "Eligible",
    shift: "06:00-14:00",
    utilization: 82,
  },
  {
    id: "P-052",
    name: "Amina Yusuf",
    role: "Dispatch Rider",
    branch: "Yaba",
    availability: "Busy",
    eligibility: "Blocked",
    shift: "08:00-16:00",
    utilization: 94,
  },
  {
    id: "P-071",
    name: "Kabiru Ahmed",
    role: "Hauling Driver",
    branch: "Apapa",
    availability: "Online",
    eligibility: "Eligible",
    shift: "06:00-14:00",
    utilization: 76,
  },
  {
    id: "P-031",
    name: "Esther Obi",
    role: "Dispatch Rider",
    branch: "Lekki",
    availability: "Leave",
    eligibility: "Blocked",
    shift: "10:00-18:00",
    utilization: 0,
  },
  {
    id: "P-088",
    name: "Chinedu Uche",
    role: "Dispatch Rider",
    branch: "Ikeja",
    availability: "Online",
    eligibility: "Eligible",
    shift: "14:00-22:00",
    utilization: 68,
  },
];

const MOCK_TRAINING: TrainingItem[] = [
  {
    id: "TR-001",
    provider: "Amina Yusuf",
    course: "Safety Training Q3 2026",
    status: "Expired",
    dueDate: "2026-07-01",
    completionDate: null,
  },
  {
    id: "TR-002",
    provider: "Peter Okafor",
    course: "Vehicle Inspection",
    status: "Completed",
    dueDate: "2026-08-15",
    completionDate: "2026-08-10",
  },
  {
    id: "TR-003",
    provider: "Kabiru Ahmed",
    course: "Hauling Safety",
    status: "InProgress",
    dueDate: "2026-09-05",
    completionDate: null,
  },
  {
    id: "TR-004",
    provider: "Esther Obi",
    course: "Dispatch Protocol",
    status: "Pending",
    dueDate: "2026-10-01",
    completionDate: null,
  },
  {
    id: "TR-005",
    provider: "Chinedu Uche",
    course: "First Aid Training",
    status: "Completed",
    dueDate: "2026-07-30",
    completionDate: "2026-07-25",
  },
];

type AvailabilityStyle = {
  bg: string;
  text: string;
};

const getAvailabilityStyle = (status: string): AvailabilityStyle => {
  switch (status) {
    case "Online":
      return { bg: "bg-green-bg", text: "text-green" };
    case "Busy":
      return { bg: "bg-amber-bg", text: "text-amber" };
    case "Offline":
      return { bg: "bg-surface-2", text: "text-ink-500" };
    case "Leave":
      return { bg: "bg-blue-bg", text: "text-blue" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500" };
  }
};

const getEligibilityStyle = (status: string): AvailabilityStyle => {
  switch (status) {
    case "Eligible":
      return { bg: "bg-green-bg", text: "text-green" };
    case "Blocked":
      return { bg: "bg-red-bg", text: "text-red" };
    case "Training":
      return { bg: "bg-amber-bg", text: "text-amber" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500" };
  }
};

const getTrainingStatusStyle = (status: string): AvailabilityStyle => {
  switch (status) {
    case "Completed":
      return { bg: "bg-green-bg", text: "text-green" };
    case "InProgress":
      return { bg: "bg-blue-bg", text: "text-blue" };
    case "Pending":
      return { bg: "bg-amber-bg", text: "text-amber" };
    case "Expired":
      return { bg: "bg-red-bg", text: "text-red" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500" };
  }
};

const COLOR_CLASSES: Record<string, string> = {
  blue: "bg-blue-bg text-blue",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
  purple: "bg-purple-bg text-purple",
};

export default function WorkforceDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [selectedBranch, setSelectedBranch] = useState<string>("All branches");

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
        <p className="mt-3 text-sm text-ink-600">Loading workforce dashboard...</p>
      </div>
    );
  }

  const formatDate = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Workforce Dashboard
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Shift coverage, leave, qualification/training expiries, utilization and categorized acceptance/decline measures.
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
            <Building2 className="h-3.5 w-3.5" />
            {selectedBranch} ▾
          </button>
        </div>
      </div>

      {/* ===== METRICS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {MOCK_WORKFORCE_METRICS.map((metric) => (
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

      {/* ===== PROVIDER STATUS TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Users className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Provider Status</span>
            <span className="text-xs text-ink-500">Real-time availability and eligibility</span>
          </div>
          <button
            onClick={() => router.push("/providers")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Provider</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Availability</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Eligibility</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Shift</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Utilization</th>
              </tr>
            </thead>
            <tbody>
              {MOCK_PROVIDERS.map((provider) => {
                const availabilityStyle = getAvailabilityStyle(provider.availability);
                const eligibilityStyle = getEligibilityStyle(provider.eligibility);

                return (
                  <tr
                    key={provider.id}
                    onClick={() => router.push(`/workforce/${provider.id}`)}
                    className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950 text-xs">{provider.name}</div>
                      <div className="font-mono text-[10px] text-ink-500">{provider.id}</div>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{provider.role}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{provider.branch}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${availabilityStyle.bg} ${availabilityStyle.text}`}>
                        {provider.availability}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${eligibilityStyle.bg} ${eligibilityStyle.text}`}>
                        {provider.eligibility}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{provider.shift}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 w-12 rounded-full bg-surface-2 overflow-hidden">
                          <div
                            className="h-full rounded-full bg-primary"
                            style={{ width: `${provider.utilization}%` }}
                          />
                        </div>
                        <span className="text-xs font-medium text-ink-950">{provider.utilization}%</span>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== TRAINING STATUS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <BookOpen className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Training Status</span>
            <span className="text-xs text-ink-500">Course completion and expiry</span>
          </div>
          <button
            onClick={() => router.push("/workforce/training")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="divide-y divide-line">
          {MOCK_TRAINING.map((item) => {
            const statusStyle = getTrainingStatusStyle(item.status);

            return (
              <div
                key={item.id}
                className="flex items-center justify-between p-3 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-ink-950">{item.provider}</span>
                    <span className="text-[10px] text-ink-500">{item.course}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      {item.status}
                    </span>
                  </div>
                  <div className="mt-0.5 text-[10px] text-ink-500">
                    Due: {formatDate(item.dueDate)}
                    {item.completionDate && ` · Completed: ${formatDate(item.completionDate)}`}
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
          <Briefcase className="h-4 w-4" />
          RPT-03 · Workforce Dashboard
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Branch: {selectedBranch}
        </span>
      </div>
    </div>
  );
}