// src/app/(dashboard)/workforce/performance/[providerId]/page.tsx

"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter, useParams } from "next/navigation";
import {
  TrendingUp,
  TrendingDown,
  RefreshCw,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  Eye,
  Download,
  FileText,
  Printer,
  ChevronDown,
  MoreHorizontal,
  User,
  Briefcase,
  Star,
  Award,
  Shield,
  Truck,
  Users,
  Phone,
  Mail,
  Sparkles,
  Activity,
  BarChart3,
  Info,
  Search,
  ChevronRight,
  CalendarDays,
  SlidersHorizontal,
  X,
} from "lucide-react";
import {
  LineChart,
  Line,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  ReferenceLine,
} from "recharts";

type DeclineHistory = {
  id: string;
  date: string;
  assignment: string;
  reasonCategory:
    | "Safety"
    | "Availability"
    | "Operational"
    | "Technical"
    | "Personal"
    | "Other";
  note?: string;
  evidence?: boolean;
};

type WeeklyTrend = {
  week: string;
  value: number;
};

type CategoryScore = {
  label: string;
  value: number;
  color: string;
  description: string;
  icon: React.ReactNode;
};

type MetricInput = {
  label: string;
  value: number | string;
  source: string;
};

// Mock data
const MOCK_DATA = {
  providerId: "P-044",
  name: "Peter Okafor",
  initials: "PO",
  role: "Dispatch Rider",
  branch: "Ikeja",
  period: "Aug 2026",
  utilization: 78,
  acceptanceRate: 94,
  onTimeRate: 97,
  attendanceRate: 97,
  rating: 4.8,
  trend: "up" as const,
  totalJobs: 342,
  activeSince: "2024-03-15",
  phone: "+234 801 234 5678",
  email: "peter.okafor@apexlogistics.com",

  weeklyTrend: [
    { week: "W1", value: 72 },
    { week: "W2", value: 78 },
    { week: "W3", value: 82 },
    { week: "W4", value: 76 },
    { week: "W5", value: 84 },
    { week: "W6", value: 88 },
    { week: "W7", value: 92 },
    { week: "W8", value: 94 },
  ],

  categories: [
    {
      label: "Safety",
      value: 92,
      color: "#10b981",
      description: "Incident-free driving",
      icon: <Shield className="h-4 w-4" />,
    },
    {
      label: "Operational",
      value: 88,
      color: "#3b82f6",
      description: "Route efficiency",
      icon: <Activity className="h-4 w-4" />,
    },
    {
      label: "Technical",
      value: 85,
      color: "#8b5cf6",
      description: "Vehicle handling",
      icon: <Truck className="h-4 w-4" />,
    },
    {
      label: "Customer",
      value: 90,
      color: "#f59e0b",
      description: "Professionalism",
      icon: <Users className="h-4 w-4" />,
    },
  ],

  declineHistory: [
    {
      id: "DEC-001",
      date: "2026-08-18",
      assignment: "KG-8491",
      reasonCategory: "Safety",
      note: "Vehicle brake issue reported during pre-trip inspection.",
      evidence: true,
    },
    {
      id: "DEC-002",
      date: "2026-08-14",
      assignment: "KG-8479",
      reasonCategory: "Availability",
      note: "Already assigned to another urgent delivery.",
    },
    {
      id: "DEC-003",
      date: "2026-08-09",
      assignment: "KG-8452",
      reasonCategory: "Operational",
      note: "Route would exceed shift hours.",
    },
    {
      id: "DEC-004",
      date: "2026-08-02",
      assignment: "KG-8431",
      reasonCategory: "Personal",
      note: "Personal emergency.",
    },
    {
      id: "DEC-005",
      date: "2026-07-28",
      assignment: "KG-8417",
      reasonCategory: "Safety",
      note: "Severe weather conditions on route.",
      evidence: true,
    },
  ],

  metricInputs: [
    { label: "Total Assignments", value: 412, source: "Assignment" },
    { label: "Accepted", value: 388, source: "Provider Response" },
    { label: "Declined", value: 24, source: "Provider Response" },
    { label: "Completed On-Time", value: 376, source: "Trip Service" },
    { label: "Attendance", value: "97%", source: "Workforce" },
    { label: "Feedback Score", value: "4.8 ★", source: "CRM" },
  ],
};

const CATEGORY_STYLES: Record<
  DeclineHistory["reasonCategory"],
  { bg: string; text: string; dot: string }
> = {
  Safety: { bg: "bg-red-50", text: "text-red-600", dot: "bg-red-500" },
  Availability: { bg: "bg-blue-50", text: "text-blue-600", dot: "bg-blue-500" },
  Operational: { bg: "bg-amber-50", text: "text-amber-600", dot: "bg-amber-500" },
  Technical: { bg: "bg-purple-50", text: "text-purple-600", dot: "bg-purple-500" },
  Personal: { bg: "bg-emerald-50", text: "text-emerald-600", dot: "bg-emerald-500" },
  Other: { bg: "bg-gray-50", text: "text-gray-600", dot: "bg-gray-400" },
};

const PERIODS = ["This Month", "Last Month", "This Quarter", "Last Quarter", "This Year"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ProviderPerformanceDetailPage() {
  const router = useRouter();
  const params = useParams();
  const providerId = params.providerId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<typeof MOCK_DATA | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState("This Month");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [showExportDropdown, setShowExportDropdown] = useState(false);
  const [searchDeclines, setSearchDeclines] = useState("");
  const [showAllDeclines, setShowAllDeclines] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [expandedDecline, setExpandedDecline] = useState<string | null>(null);

  const exportRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DATA);
      setLoading(false);
    };
    loadData();
  }, [providerId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (exportRef.current && !exportRef.current.contains(e.target as Node)) {
        setShowExportDropdown(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <span className="ml-3 text-sm text-ink-600">Loading performance data...</span>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-16">
        <div className="rounded-full bg-surface-2 p-4">
          <User className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-4 text-lg font-semibold text-ink-950">Provider not found</h3>
        <p className="mt-1 text-sm text-ink-500">No performance data exists for {providerId}</p>
        <button
          onClick={() => router.push("/workforce/performance")}
          className="mt-4 rounded-lg bg-primary px-6 py-2.5 text-sm font-medium text-white transition hover:bg-primary-hover"
        >
          ← Back to Performance
        </button>
      </div>
    );
  }

  const trendStyle =
    data.trend === "up"
      ? { bg: "bg-green-bg", text: "text-green", icon: <TrendingUp className="h-4 w-4" /> }
      : data.trend === "down"
      ? { bg: "bg-red-bg", text: "text-red", icon: <TrendingDown className="h-4 w-4" /> }
      : { bg: "bg-primary-bg", text: "text-primary", icon: <Activity className="h-4 w-4" /> };

  const filteredDeclines = data.declineHistory.filter(
    (d) =>
      d.assignment.toLowerCase().includes(searchDeclines.toLowerCase()) ||
      d.reasonCategory.toLowerCase().includes(searchDeclines.toLowerCase()) ||
      (d.note && d.note.toLowerCase().includes(searchDeclines.toLowerCase()))
  );

  const displayedDeclines = showAllDeclines
    ? filteredDeclines
    : filteredDeclines.slice(0, 3);

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    searchDeclines !== "";

  const clearFilters = () => {
    setSelectedBranch("All branches");
    setSearchDeclines("");
  };

  // Chart data for Recharts
  const chartData = data.weeklyTrend.map((item) => ({
    week: item.week,
    value: item.value,
  }));

  return (
    <div className="space-y-6 pb-8">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
              {data.name}
            </h1>
            <span className="font-mono text-xs text-ink-400 sm:text-sm">
              {data.providerId}
            </span>
            <span
              className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium sm:px-3 ${trendStyle.bg} ${trendStyle.text}`}
            >
              {trendStyle.icon}
              {data.trend === "up" ? "Improving" : data.trend === "down" ? "Declining" : "Stable"}
            </span>
            <div className="flex items-center gap-1.5 rounded-full bg-green-bg px-2.5 py-0.5 text-xs font-medium text-green sm:px-3">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
              </span>
              Active
            </div>
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500 sm:text-sm">
            <span className="flex items-center gap-1.5">
              <Briefcase className="h-3.5 w-3.5" />
              {data.role}
            </span>
            <span className="flex items-center gap-1.5">
              <MapPin className="h-3.5 w-3.5" />
              {data.branch}
            </span>
            <span className="flex items-center gap-1.5">
              <CalendarDays className="h-3.5 w-3.5" />
              Since {data.activeSince}
            </span>
            <span className="flex items-center gap-1.5">
              <Clock className="h-3.5 w-3.5" />
              {data.totalJobs} jobs
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-2 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-500 transition hover:bg-white"
          >
            <RefreshCw className="h-4 w-4" />
            {secondsAgo}s
          </button>
          <div className="relative" ref={exportRef}>
            <button
              onClick={() => setShowExportDropdown(!showExportDropdown)}
              className="flex h-9 items-center gap-2 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 transition hover:bg-white"
            >
              <Download className="h-4 w-4" />
              Export
              <ChevronDown className="h-4 w-4" />
            </button>
            {showExportDropdown && (
              <div className="absolute right-0 top-full mt-1.5 min-w-[180px] rounded-xl border border-line bg-white p-1 shadow-xl">
                <button className="flex w-full items-center gap-3 rounded-lg px-4 py-2.5 text-sm text-ink-700 transition hover:bg-surface-2">
                  <FileText className="h-4 w-4" />
                  PDF Report
                </button>
                <button className="flex w-full items-center gap-3 rounded-lg px-4 py-2.5 text-sm text-ink-700 transition hover:bg-surface-2">
                  <FileText className="h-4 w-4" />
                  CSV Data
                </button>
                <button className="flex w-full items-center gap-3 rounded-lg px-4 py-2.5 text-sm text-ink-700 transition hover:bg-surface-2">
                  <Printer className="h-4 w-4" />
                  Print
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ===== PERIOD SELECTOR ===== */}
      <div className="flex flex-wrap items-center gap-1 rounded-xl border border-line bg-white/70 p-1">
        {PERIODS.map((period) => (
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

      {/* ===== FILTERS - SAME STYLE AS WORKFORCE PAGE ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-10 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          {selectedBranch}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
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
            className="flex h-10 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== FILTER DRAWER - SAME STYLE AS WORKFORCE PAGE ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex gap-1.5">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedBranch === branch
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branch}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
        {/* ===== CHART - Using Recharts ===== */}
        <div className="lg:col-span-3">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm sm:p-6">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Sparkles className="h-4 w-4 text-ink-400" />
                <h3 className="text-sm font-semibold text-ink-950">
                  Performance Trend
                </h3>
              </div>
              <span className="text-xs text-ink-400">Weekly acceptance rate</span>
            </div>

            <div className="mt-4 h-[200px] w-full sm:h-[220px]">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart
                  data={chartData}
                  margin={{ top: 10, right: 10, left: -10, bottom: 0 }}
                >
                  <defs>
                    <linearGradient id="chartGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#2B2CAA" stopOpacity={0.25} />
                      <stop offset="100%" stopColor="#2B2CAA" stopOpacity={0.02} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#f0f0f0" />
                  <XAxis
                    dataKey="week"
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 10, fill: "#9CA3AF" }}
                    dy={6}
                  />
                  <YAxis
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 10, fill: "#9CA3AF" }}
                    domain={[65, 100]}
                    ticks={[65, 70, 75, 80, 85, 90, 95, 100]}
                    dx={-4}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "#1F2937",
                      border: "none",
                      borderRadius: "8px",
                      color: "#fff",
                      fontSize: "12px",
                      padding: "8px 12px",
                    }}
                    labelStyle={{ color: "#9CA3AF", fontSize: "10px" }}
                    formatter={(value: number) => [`${value}%`, "Acceptance Rate"]}
                    cursor={{ stroke: "#D1D5DB", strokeWidth: 1, strokeDasharray: "4 4" }}
                  />
                  <Area
                    type="monotone"
                    dataKey="value"
                    stroke="none"
                    fill="url(#chartGradient)"
                  />
                  <Line
                    type="monotone"
                    dataKey="value"
                    stroke="#2B2CAA"
                    strokeWidth={2.5}
                    dot={{
                      fill: "#2B2CAA",
                      stroke: "#fff",
                      strokeWidth: 2,
                      r: 4,
                    }}
                    activeDot={{
                      fill: "#2B2CAA",
                      stroke: "#fff",
                      strokeWidth: 2,
                      r: 6,
                    }}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>

            <div className="mt-3 flex flex-wrap justify-between gap-2 border-t border-line pt-3 text-xs text-ink-400">
              <span>Week 1</span>
              <span>Week 8</span>
              <span className="flex items-center gap-1 text-green">
                <TrendingUp className="h-3.5 w-3.5" />
                +{data.weeklyTrend[data.weeklyTrend.length - 1].value - data.weeklyTrend[0].value}% improvement
              </span>
            </div>
          </div>
        </div>

        {/* ===== CATEGORY SCORES ===== */}
        <div className="lg:col-span-2">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm sm:p-6">
            <h3 className="text-sm font-semibold text-ink-950">
              Category Scores
            </h3>
            <div className="mt-4 space-y-3.5">
              {data.categories.map((cat) => (
                <div key={cat.label}>
                  <div className="flex items-center justify-between text-sm">
                    <span className="flex items-center gap-1.5 text-ink-600">
                      <span style={{ color: cat.color }}>{cat.icon}</span>
                      {cat.label}
                    </span>
                    <span className="font-semibold text-ink-950">
                      {cat.value}%
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className="h-full rounded-full transition-all duration-1000"
                      style={{
                        width: `${cat.value}%`,
                        backgroundColor: cat.color,
                      }}
                    />
                  </div>
                  <div className="mt-0.5 text-xs text-ink-400">
                    {cat.description}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* ===== METRIC INPUTS + DECLINE HISTORY ===== */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
        {/* ===== METRIC INPUTS ===== */}
        <div className="lg:col-span-2">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm sm:p-6">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-ink-950">
                Key Metrics
              </h3>
              <span className="text-xs text-ink-400">Formula v3.2</span>
            </div>
            <div className="mt-4 space-y-3">
              {data.metricInputs.map((metric) => {
                const isPercentage = typeof metric.value === "string" && metric.value.includes("%");
                const isRating = typeof metric.value === "string" && metric.value.includes("★");
                const numValue = typeof metric.value === "number" ? metric.value : 0;

                return (
                  <div
                    key={metric.label}
                    className="flex items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2"
                  >
                    <div>
                      <div className="text-xs font-medium uppercase tracking-wider text-ink-400">
                        {metric.label}
                      </div>
                      <div className="mt-0.5 text-base font-semibold text-ink-950 sm:text-lg">
                        {metric.value}
                      </div>
                      <div className="text-[10px] text-ink-400">{metric.source}</div>
                    </div>
                    {typeof metric.value === "number" && (
                      <div className="w-16 h-1.5 rounded-full bg-surface-3 overflow-hidden">
                        <div
                          className="h-full rounded-full bg-primary"
                          style={{
                            width: `${Math.min((metric.value / 500) * 100, 100)}%`,
                          }}
                        />
                      </div>
                    )}
                    {isPercentage && (
                      <div className="w-16 h-1.5 rounded-full bg-surface-3 overflow-hidden">
                        <div
                          className="h-full rounded-full bg-green"
                          style={{
                            width: `${parseInt(metric.value)}%`,
                          }}
                        />
                      </div>
                    )}
                    {isRating && (
                      <div className="flex items-center gap-0.5">
                        <Star className="h-3.5 w-3.5 fill-amber text-amber" />
                        <span className="text-sm font-medium text-ink-950">
                          {metric.value}
                        </span>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* ===== DECLINE HISTORY ===== */}
        <div className="lg:col-span-3">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm sm:p-6">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <div className="flex items-center gap-2">
                  <XCircle className="h-5 w-5 text-ink-400" />
                  <h3 className="text-sm font-semibold text-ink-950">
                    Decline History
                  </h3>
                  <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-xs font-medium text-ink-500">
                    {data.declineHistory.length}
                  </span>
                </div>
                <p className="mt-0.5 text-xs text-ink-500 sm:text-sm">
                  Categorized reasons — no automatic disciplinary action
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex h-9 min-w-[140px] items-center gap-2 rounded-lg border border-line bg-white/70 px-2.5 text-xs sm:h-9 sm:min-w-[180px] sm:px-3 sm:text-sm">
                  <Search className="h-3.5 w-3.5 text-ink-400" />
                  <input
                    value={searchDeclines}
                    onChange={(e) => setSearchDeclines(e.target.value)}
                    placeholder="Search declines..."
                    className="w-full bg-transparent text-xs text-ink-950 outline-none placeholder:text-ink-400 sm:text-sm"
                  />
                </div>
                <button
                  onClick={() => setShowAllDeclines(!showAllDeclines)}
                  className="flex h-9 items-center gap-1.5 rounded-lg border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 transition hover:bg-white"
                >
                  {showAllDeclines ? "Show Less" : "Show All"}
                  <ChevronDown className={`h-4 w-4 transition-transform ${showAllDeclines ? "rotate-180" : ""}`} />
                </button>
              </div>
            </div>

            {filteredDeclines.length === 0 ? (
              <div className="mt-5 flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-line py-8 sm:py-10">
                <div className="rounded-full bg-surface-2 p-3">
                  <CheckCircle2 className="h-6 w-6 text-green" />
                </div>
                <p className="mt-2 text-sm font-medium text-ink-600">No declines found</p>
                <p className="text-xs text-ink-400 sm:text-sm">
                  {searchDeclines ? "Try adjusting your search" : "No recorded declines for this provider"}
                </p>
              </div>
            ) : (
              <div className="mt-4 space-y-2">
                {displayedDeclines.map((decline) => {
                  const style = CATEGORY_STYLES[decline.reasonCategory];
                  const isExpanded = expandedDecline === decline.id;
                  return (
                    <div
                      key={decline.id}
                      className="rounded-xl border border-line p-3 transition hover:border-ink-300 hover:bg-surface-2 sm:p-4"
                    >
                      <div className="flex flex-wrap items-center gap-2 sm:gap-3">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium sm:px-2.5 sm:text-xs ${style.bg} ${style.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${style.dot}`} />
                          {decline.reasonCategory}
                        </span>
                        <span className="font-mono text-xs text-ink-500 sm:text-sm">
                          {decline.assignment}
                        </span>
                        <span className="text-xs text-ink-400 sm:text-sm">
                          {decline.date}
                        </span>
                        {decline.evidence && (
                          <span className="flex items-center gap-1 rounded-full bg-accent-bg px-2 py-0.5 text-[10px] font-medium text-accent sm:text-xs">
                            <Eye className="h-3 w-3" />
                            Evidence
                          </span>
                        )}
                        <button
                          onClick={() =>
                            setExpandedDecline(isExpanded ? null : decline.id)
                          }
                          className="ml-auto text-ink-400 hover:text-ink-600"
                        >
                          <ChevronRight
                            className={`h-4 w-4 transition-transform sm:h-5 sm:w-5 ${
                              isExpanded ? "rotate-90" : ""
                            }`}
                          />
                        </button>
                      </div>
                      {decline.note && (
                        <div className="mt-1 text-xs text-ink-500 sm:text-sm">
                          {decline.note}
                        </div>
                      )}
                      {isExpanded && (
                        <div className="mt-2 rounded-lg bg-surface-2 p-2.5 text-xs text-ink-600 sm:p-3 sm:text-sm">
                          <span className="font-medium">Full note:</span> {decline.note}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}

            <div className="mt-4 flex items-start gap-2.5 rounded-xl bg-primary-bg p-3 sm:p-4">
              <Info className="h-4 w-4 flex-shrink-0 text-primary sm:h-5 sm:w-5" />
              <div className="text-xs text-primary/80 sm:text-sm">
                <span className="font-medium">Safety-first:</span> Decline reasons
                are recorded for visibility. No automatic disciplinary action,
                suspension, or deduction is applied.
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Info className="h-4 w-4" />
          Read-only performance review — advisory metrics only
        </span>
        <span className="flex items-center gap-1.5">
          <Award className="h-4 w-4 text-amber" />
          WRK-12 · Provider Performance Detail
        </span>
      </div>
    </div>
  );
}