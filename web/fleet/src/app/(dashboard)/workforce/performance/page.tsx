// src/app/(dashboard)/workforce/performance/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Search,
  TrendingUp,
  TrendingDown,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Eye,
  Filter,
  ArrowRight,
  Calendar,
  User,
  Briefcase,
  FileText,
  Sparkles,
  Award,
  Zap,
  Gauge,
  AlertTriangle,
  MoreHorizontal,
  Users,
  Shield,
  Activity,
  Star,
  BarChart3,
  PieChart,
  LineChart,
} from "lucide-react";

type PerformanceRecord = {
  id: string;
  providerId: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  period: string;
  utilization: number;
  acceptanceRate: number;
  onTimeRate: number;
  completedJobs: number;
  rating: number;
  attendanceRate: number;
  trend: "up" | "down" | "stable";
  categories: {
    safety: number;
    operational: number;
    technical: number;
    customer: number;
  };
};

// Mock data
const PERFORMANCE_DATA: PerformanceRecord[] = [
  {
    id: "PERF-001",
    providerId: "P-044",
    name: "Peter Okafor",
    initials: "PO",
    role: "Dispatch Rider",
    branch: "Ikeja",
    period: "Aug 2026",
    utilization: 78,
    acceptanceRate: 94,
    onTimeRate: 97,
    completedJobs: 342,
    rating: 4.8,
    attendanceRate: 97,
    trend: "up",
    categories: {
      safety: 92,
      operational: 88,
      technical: 85,
      customer: 90,
    },
  },
  {
    id: "PERF-002",
    providerId: "P-038",
    name: "Musa Bello",
    initials: "MB",
    role: "Hauling Driver",
    branch: "Apapa",
    period: "Aug 2026",
    utilization: 82,
    acceptanceRate: 86,
    onTimeRate: 92,
    completedJobs: 189,
    rating: 4.2,
    attendanceRate: 94,
    trend: "up",
    categories: {
      safety: 78,
      operational: 85,
      technical: 80,
      customer: 82,
    },
  },
  {
    id: "PERF-003",
    providerId: "P-052",
    name: "Amina Yusuf",
    initials: "AY",
    role: "Dispatch Supervisor",
    branch: "Yaba",
    period: "Aug 2026",
    utilization: 72,
    acceptanceRate: 88,
    onTimeRate: 94,
    completedJobs: 178,
    rating: 4.6,
    attendanceRate: 96,
    trend: "down",
    categories: {
      safety: 90,
      operational: 82,
      technical: 88,
      customer: 92,
    },
  },
  {
    id: "PERF-004",
    providerId: "P-061",
    name: "Chinedu Uche",
    initials: "CU",
    role: "Dispatch Rider",
    branch: "Lekki",
    period: "Aug 2026",
    utilization: 85,
    acceptanceRate: 97,
    onTimeRate: 99,
    completedJobs: 512,
    rating: 4.9,
    attendanceRate: 98,
    trend: "up",
    categories: {
      safety: 95,
      operational: 92,
      technical: 90,
      customer: 94,
    },
  },
  {
    id: "PERF-005",
    providerId: "P-029",
    name: "Kabiru Ahmed",
    initials: "KA",
    role: "Hauling Driver",
    branch: "Lekki",
    period: "Aug 2026",
    utilization: 60,
    acceptanceRate: 78,
    onTimeRate: 85,
    completedJobs: 234,
    rating: 4.4,
    attendanceRate: 88,
    trend: "stable",
    categories: {
      safety: 65,
      operational: 70,
      technical: 75,
      customer: 68,
    },
  },
  {
    id: "PERF-006",
    providerId: "P-073",
    name: "Esther Obi",
    initials: "EO",
    role: "Fleet Manager",
    branch: "Ikeja",
    period: "Aug 2026",
    utilization: 80,
    acceptanceRate: 92,
    onTimeRate: 96,
    completedJobs: 412,
    rating: 4.7,
    attendanceRate: 95,
    trend: "up",
    categories: {
      safety: 88,
      operational: 90,
      technical: 92,
      customer: 85,
    },
  },
];

const TREND_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  up: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <TrendingUp className="h-3.5 w-3.5" />,
  },
  down: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <TrendingDown className="h-3.5 w-3.5" />,
  },
  stable: {
    bg: "bg-blue-bg",
    text: "text-blue",
    icon: <Activity className="h-3.5 w-3.5" />,
  },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function PerformanceOverviewPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<PerformanceRecord[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PERFORMANCE_DATA);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesBranch;
  });

  // Stats
  const stats = {
    total: data.length,
    avgUtilization: Math.round(data.reduce((acc, d) => acc + d.utilization, 0) / data.length),
    avgAcceptance: Math.round(data.reduce((acc, d) => acc + d.acceptanceRate, 0) / data.length),
    avgOnTime: Math.round(data.reduce((acc, d) => acc + d.onTimeRate, 0) / data.length),
  };

  // Navigation
  const goToPerformanceDetail = (providerId: string) => {
    router.push(`/workforce/performance/${providerId}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading performance data...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
  };

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="absolute -top-1 -right-1">
                <Sparkles className="h-4 w-4 text-amber animate-pulse" />
              </div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                Performance Overview
              </h1>
            </div>
            <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-[11px] font-semibold text-primary">
              {data.length}
            </span>
            <div className="flex items-center gap-1 rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
              </span>
              {stats.avgUtilization}% utilization
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Provider performance metrics including utilization, acceptance, on-time delivery, and category scores.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <BarChart3 className="h-3.5 w-3.5" />
            Reports
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Users className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total Providers</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.total}</div>
            </div>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Gauge className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Utilization</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.avgUtilization}%</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-blue">
            <TrendingUp className="h-3 w-3" />
            <span>+4% vs last month</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Acceptance Rate</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.avgAcceptance}%</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+2% vs last month</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <Clock className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">On-Time Rate</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.avgOnTime}%</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+1% vs last month</span>
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[240px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or role..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          {selectedBranch}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-[12px] font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {selectedBranch !== "All branches" ? 1 : 0}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-[12px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== PERFORMANCE TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <BarChart3 className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No performance data found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No performance records match your current filters.
          </p>
          <button
            onClick={clearFilters}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-[13px] min-w-[1000px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Provider</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Utilization</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Acceptance</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">On-Time</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Rating</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Trend</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const trendStyle = TREND_STYLES[item.trend] || TREND_STYLES.stable;

                  return (
                    <tr
                      key={item.id}
                      className="cursor-pointer hover:bg-surface-2 transition-colors group"
                    >
                      <td
                        className="border-b border-line px-4 py-3"
                        onClick={() => goToPerformanceDetail(item.providerId)}
                      >
                        <div className="flex items-center gap-3">
                          <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-white">
                            {item.initials}
                          </div>
                          <div>
                            <div className="font-medium text-ink-950">{item.name}</div>
                            <div className="text-[11px] text-ink-500">{item.providerId}</div>
                          </div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{item.role}</td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{item.branch}</td>
                      <td className="border-b border-line px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-ink-950">{item.utilization}%</span>
                          <div className="w-16 h-1.5 rounded-full bg-surface-3 overflow-hidden">
                            <div
                              className={`h-full rounded-full ${
                                item.utilization >= 75 ? "bg-green" :
                                item.utilization >= 60 ? "bg-amber" :
                                "bg-red"
                              }`}
                              style={{ width: `${item.utilization}%` }}
                            />
                          </div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`font-semibold ${
                          item.acceptanceRate >= 90 ? "text-green" :
                          item.acceptanceRate >= 75 ? "text-amber" :
                          "text-red"
                        }`}>
                          {item.acceptanceRate}%
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`font-semibold ${
                          item.onTimeRate >= 95 ? "text-green" :
                          item.onTimeRate >= 85 ? "text-amber" :
                          "text-red"
                        }`}>
                          {item.onTimeRate}%
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className="flex items-center gap-0.5 text-amber">
                          <Star className="h-3.5 w-3.5 fill-amber text-amber" />
                          {item.rating}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${trendStyle.bg} ${trendStyle.text}`}>
                          {trendStyle.icon}
                          {item.trend}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-right">
                        <button
                          onClick={() => goToPerformanceDetail(item.providerId)}
                          className="text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors"
                        >
                          <Eye className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}