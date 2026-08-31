// src/app/(dashboard)/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Route,
  Users,
  Truck,
  AlertTriangle,
  SlidersHorizontal,
  ChevronRight,
  TrendingUp,
  TrendingDown,
  Plus,
  Send,
  MapPin,
  UserPlus,
  AlertCircle,
  Bell,
  BarChart3,
  ArrowRight,
  Calendar,
  Activity,
  Rocket,
  X,
} from "lucide-react";
import {
  BarChart as RBarChart,
  Bar,
  LineChart as RLineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

// ===== SIMPLE VISUAL MAP =====
function MapComponent() {
  return (
    <div className="w-full h-[280px] sm:h-[320px] lg:h-[380px] rounded-lg overflow-hidden relative bg-[#eef0f4]">
      <div className="absolute inset-0" style={{
        backgroundImage: `
          linear-gradient(rgba(200,200,210,0.3) 1px, transparent 1px),
          linear-gradient(90deg, rgba(200,200,210,0.3) 1px, transparent 1px)
        `,
        backgroundSize: '40px 40px'
      }} />
      <div className="absolute left-[20%] top-[30%] w-[55%] h-[2px] bg-ink-300/40 rounded-full" />
      <div className="absolute left-[10%] top-[50%] w-[75%] h-[2px] bg-ink-300/40 rounded-full" />
      <div className="absolute left-[40%] top-[15%] w-[2px] h-[65%] bg-ink-300/40 rounded-full" />
      <div className="absolute left-[25%] top-[65%] w-[35%] h-[2px] bg-ink-300/40 rounded-full" />
      <div className="absolute left-[60%] top-[20%] w-[2px] h-[55%] bg-ink-300/40 rounded-full" />
      <div className="absolute left-[22%] top-[32%] w-[45%] h-[3px] bg-primary/60 rounded-full" />
      <div className="absolute left-[22%] top-[25%]">
        <div className="h-4 w-4 rounded-full bg-primary border-2 border-white" />
        <div className="text-[10px] font-semibold text-ink-700 mt-1 ml-[-12px] whitespace-nowrap">Ikeja</div>
      </div>
      <div className="absolute left-[55%] top-[35%]">
        <div className="h-4 w-4 rounded-full bg-green border-2 border-white" />
        <div className="text-[10px] font-semibold text-ink-700 mt-1 ml-[-12px] whitespace-nowrap">Lekki</div>
      </div>
      <div className="absolute left-[38%] top-[58%]">
        <div className="h-4 w-4 rounded-full bg-amber border-2 border-white" />
        <div className="text-[10px] font-semibold text-ink-700 mt-1 ml-[-12px] whitespace-nowrap">Yaba</div>
      </div>
      <div className="absolute left-[12%] top-[65%]">
        <div className="h-4 w-4 rounded-full bg-red border-2 border-white" />
        <div className="text-[10px] font-semibold text-ink-700 mt-1 ml-[-12px] whitespace-nowrap">Apapa</div>
      </div>
      <div className="absolute left-[68%] top-[55%]">
        <div className="h-4 w-4 rounded-full bg-blue-500 border-2 border-white" />
        <div className="text-[10px] font-semibold text-ink-700 mt-1 ml-[-12px] whitespace-nowrap">VI</div>
      </div>
      <div className="absolute left-[45%] top-[40%]">
        <div className="h-3 w-3 rounded-full bg-primary" />
      </div>
    </div>
  );
}

type Tone = "blue" | "green" | "amber" | "red" | "neutral";
type DateRange = "Today" | "Yesterday" | "This Week" | "This Month";
type Branch = "All Branches" | "Ikeja" | "Lekki" | "Yaba" | "Apapa";

const TONE_ICON_BG: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
  neutral: "bg-surface-2 text-ink-600",
};

const DATE_OPTIONS: DateRange[] = ["Today", "Yesterday", "This Week", "This Month"];
const BRANCH_OPTIONS: Branch[] = ["All Branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

const BASE_METRICS = [
  {
    label: "Active work",
    value: "128",
    note: "Across 4 branches",
    tone: "blue" as Tone,
    trend: "+10%",
    trendUp: true,
    icon: <Route className="h-[18px] w-[18px]" />,
    href: "/operations/control-tower",
  },
  {
    label: "Available providers",
    value: "84",
    note: "12 currently busy",
    tone: "green" as Tone,
    trend: "+6%",
    trendUp: true,
    icon: <Users className="h-[18px] w-[18px]" />,
    href: "/providers",
  },
  {
    label: "Fleet ready",
    value: "96",
    note: "7 safety / maintenance blocks",
    tone: "amber" as Tone,
    trend: "+3%",
    trendUp: true,
    icon: <Truck className="h-[18px] w-[18px]" />,
    href: "/fleet/assets",
  },
  {
    label: "Open exceptions",
    value: "14",
    note: "Needs attention",
    tone: "red" as Tone,
    trend: "-27%",
    trendUp: false,
    icon: <AlertTriangle className="h-[18px] w-[18px]" />,
    href: "/operations/control-tower/exceptions",
  },
];

const WEEKLY_VOLUME = [
  { day: "Mon", value: 62 },
  { day: "Tue", value: 78 },
  { day: "Wed", value: 48 },
  { day: "Thu", value: 91 },
  { day: "Fri", value: 70 },
  { day: "Sat", value: 88 },
  { day: "Sun", value: 96 },
];

const LINE_DATA = [
  { day: "Mon", value: 45 },
  { day: "Tue", value: 62 },
  { day: "Wed", value: 38 },
  { day: "Thu", value: 78 },
  { day: "Fri", value: 55 },
  { day: "Sat", value: 82 },
  { day: "Sun", value: 94 },
];

const EXCEPTIONS = [
  {
    id: "exc_1",
    badge: "Critical",
    title: "Vehicle BR-204 safety block",
    detail: "Inspection failed · Ikeja",
    severity: "critical" as const,
    href: "/compliance/restrictions",
  },
  {
    id: "exc_2",
    badge: "High",
    title: "Trip KG-8491 behind SLA",
    detail: "12 min beyond target",
    severity: "high" as const,
    href: "/operations/trips/TRP-8491",
  },
  {
    id: "exc_3",
    badge: "Medium",
    title: "Provider documents expiring",
    detail: "3 documents expire in 2 days",
    severity: "medium" as const,
    href: "/providers",
  },
  {
    id: "exc_4",
    badge: "Low",
    title: "Maintenance due",
    detail: "7 vehicles within next 48 hours",
    severity: "low" as const,
    href: "/fleet/maintenance/schedules",
  },
];

const EXCEPTION_STYLE: Record<string, string> = {
  critical: "bg-red-bg text-red",
  high: "bg-amber-bg text-amber",
  medium: "bg-primary-bg text-primary",
  low: "bg-surface-2 text-ink-600",
};

const ACTIVE_WORK = [
  { id: "KG-8491", route: "Ikeja → Lekki", provider: "Tunde A.", branch: "Lagos", status: "Delayed" as const, eta: "18 min", href: "/operations/trips/TRP-8491" },
  { id: "KG-8502", route: "Wuse → Garki", provider: "Amina B.", branch: "Abuja", status: "In transit" as const, eta: "9 min", href: "/operations/trips/TRP-8502" },
  { id: "KG-8514", route: "GRA → Trans Amadi", provider: "Chuka O.", branch: "Port Harcourt", status: "In transit" as const, eta: "22 min", href: "/operations/trips/TRP-8514" },
  { id: "KG-8519", route: "Sabon Gari → Fagge", provider: "Musa I.", branch: "Kano", status: "Assigned" as const, eta: "Pending", href: "/operations/assignments/ASN-8519" },
  { id: "KG-8523", route: "Bodija → Dugbe", provider: "Ngozi E.", branch: "Ibadan", status: "Completed" as const, eta: "—", href: "/operations/trips/TRP-8523" },
];

const WORK_STATUS_STYLE: Record<string, string> = {
  "In transit": "bg-primary-bg text-primary",
  Assigned: "bg-surface-2 text-ink-600",
  Delayed: "bg-amber-bg text-amber",
  Completed: "bg-green-bg text-green",
};

const QUICK_ACTIONS = [
  { label: "View Trips", icon: <Calendar className="h-4 w-4" />, tone: "blue" as Tone, href: "/operations/trips" },
  { label: "Dispatch Now", icon: <Send className="h-4 w-4" />, tone: "blue" as Tone, href: "/operations/dispatch" },
  { label: "View Routes", icon: <MapPin className="h-4 w-4" />, tone: "amber" as Tone, href: "/operations/routes" },
  { label: "Add Provider", icon: <UserPlus className="h-4 w-4" />, tone: "green" as Tone, href: "/providers/add" },
  { label: "Report Issue", icon: <AlertCircle className="h-4 w-4" />, tone: "red" as Tone, href: "/compliance/safety-reports" },
  { label: "View Alerts", icon: <Bell className="h-4 w-4" />, tone: "neutral" as Tone, href: "/operations/alerts" },
];

const CHART_AXIS_STYLE = { fontSize: 11, fill: "#8A8FA3" };
const CHART_TOOLTIP_STYLE = {
  borderRadius: 8,
  border: "1px solid #E5E7EB",
  fontSize: 12,
  boxShadow: "none",
};

function SectionHeader({ title, subtitle, action, actionLabel }: { title: string; subtitle?: string; action?: string; actionLabel?: string }) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 border-b border-line px-4 sm:px-5 py-3.5">
      <div>
        <h3 className="text-sm font-semibold text-ink-950">{title}</h3>
        {subtitle && <div className="mt-0.5 text-xs text-ink-500">{subtitle}</div>}
      </div>
      {action && (
        <Link href={action} className="text-xs font-semibold text-primary hover:text-primary-hover transition-colors flex items-center gap-1">
          {actionLabel || "View all"} <ArrowRight className="h-3.5 w-3.5" />
        </Link>
      )}
    </div>
  );
}

export default function DashboardHomePage() {
  const [liveTime, setLiveTime] = useState("12 sec ago");
  const [selectedDate, setSelectedDate] = useState<DateRange>("Today");
  const [selectedBranch, setSelectedBranch] = useState<Branch>("All Branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => {
      const seconds = Math.floor(Math.random() * 30) + 5;
      setLiveTime(`${seconds} sec ago`);
    }, 5000);
    return () => clearInterval(interval);
  }, []);

  const hasActiveFilters = selectedDate !== "Today" || selectedBranch !== "All Branches";

  const clearFilters = () => {
    setSelectedDate("Today");
    setSelectedBranch("All Branches");
  };

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-4">
        <div>
          <h1>Operations Overview</h1>
          <p className="mt-1.5">Live operational health across your organization.</p>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          <div className="flex items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 py-2 text-sm font-medium text-ink-700">
            <Calendar className="h-4 w-4 text-ink-400" />
            <span>{selectedDate}</span>
            <span className="text-ink-300">·</span>
            <MapPin className="h-4 w-4 text-ink-400" />
            <span>{selectedBranch}</span>
          </div>

          <button
            onClick={() => setShowFilters(true)}
            className={`flex h-10 items-center gap-2 rounded-lg border px-3.5 text-sm font-semibold transition-colors ${
              hasActiveFilters
                ? "border-primary bg-primary-bg text-primary"
                : "border-line bg-white text-ink-700 hover:bg-surface-2"
            }`}
          >
            <SlidersHorizontal className="h-4 w-4" />
            Filters
            {hasActiveFilters && (
              <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
                {(selectedDate !== "Today" ? 1 : 0) + (selectedBranch !== "All Branches" ? 1 : 0)}
              </span>
            )}
          </button>

          {hasActiveFilters && (
            <button
              onClick={clearFilters}
              className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors"
            >
              <X className="h-4 w-4" />
              Clear
            </button>
          )}
        </div>
      </div>

      {/* ── Filter Drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-semibold text-ink-950">Filters</h2>
            <button
              onClick={() => setShowFilters(false)}
              className="rounded-lg p-1.5 text-ink-400 hover:bg-surface-2 transition-colors"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
            <div>
              <div className="mb-2 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Date Range</div>
              <div className="flex flex-wrap gap-1.5">
                {DATE_OPTIONS.map((date) => (
                  <button
                    key={date}
                    onClick={() => setSelectedDate(date)}
                    className={`rounded-md px-3.5 py-1.5 text-xs font-semibold transition-colors ${
                      selectedDate === date
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {date}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-2 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex flex-wrap gap-1.5">
                {BRANCH_OPTIONS.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-md px-3.5 py-1.5 text-xs font-semibold transition-colors ${
                      selectedBranch === branch
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branch}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="mt-5 flex gap-2.5 pt-4 border-t border-line">
            <button
              onClick={() => {
                clearFilters();
                setShowFilters(false);
              }}
              className="rounded-md border border-line px-4 py-2 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
            >
              Clear all
            </button>
            <button
              onClick={() => setShowFilters(false)}
              className="rounded-md bg-primary px-4 py-2 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
            >
              Apply filters
            </button>
          </div>
        </div>
      )}

      {/* ── Metrics ── */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        {BASE_METRICS.map((m) => (
          <Link
            key={m.label}
            href={m.href || "#"}
            className="rounded-lg border border-line bg-white p-4 sm:p-5 hover:border-ink-300 transition-colors"
          >
            <div className="flex items-center gap-2.5">
              <div className={`flex h-8 w-8 items-center justify-center rounded-md ${TONE_ICON_BG[m.tone]}`}>
                {m.icon}
              </div>
              <span className="text-xs font-semibold text-ink-600">{m.label}</span>
            </div>
            <div className="mt-3.5 font-display text-[28px] sm:text-[30px] font-bold tracking-[-0.02em] text-ink-950">
              {m.value}
            </div>
            <div className="mt-1 flex items-center gap-1.5">
              <span className={`flex items-center gap-0.5 text-[11.5px] font-semibold ${m.trendUp ? 'text-green' : 'text-red'}`}>
                {m.trendUp ? <TrendingUp className="h-3.5 w-3.5" /> : <TrendingDown className="h-3.5 w-3.5" />}
                {m.trend}
              </span>
              <span className="text-xs text-ink-500">· {m.note}</span>
            </div>
          </Link>
        ))}
      </div>

      {/* ── Map + Exceptions ── */}
      <div className="grid grid-cols-1 xl:grid-cols-[1.5fr_1fr] gap-5">
        <section className="overflow-hidden rounded-lg border border-line bg-white">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 border-b border-line px-4 sm:px-5 py-3.5">
            <div>
              <h3 className="text-sm font-semibold text-ink-950">Live Operations</h3>
              <div className="mt-0.5 text-xs text-ink-500">Real-time view of active work and assets</div>
            </div>
            <div className="flex items-center gap-3">
              <span className="flex items-center gap-1.5 rounded-full bg-green-bg px-3 py-1.5 text-xs font-semibold text-green">
                <span className="h-1.5 w-1.5 rounded-full bg-green" /> Live · {liveTime}
              </span>
              <Link href="/operations/control-tower" className="text-xs font-semibold text-primary hover:text-primary-hover transition-colors">
                View Full Map →
              </Link>
            </div>
          </div>

          <MapComponent />

          <div className="flex flex-wrap items-center gap-3 px-4 sm:px-5 py-2.5 border-t border-line bg-surface-2/50">
            <span className="text-[10px] font-semibold text-ink-600">Legend:</span>
            <span className="flex items-center gap-1.5 text-[10px] text-ink-600">
              <span className="h-2.5 w-2.5 rounded-full bg-primary" /> Active Work (126)
            </span>
            <span className="flex items-center gap-1.5 text-[10px] text-ink-600">
              <span className="h-2.5 w-2.5 rounded-full bg-green" /> Trip/Job (1)
            </span>
            <span className="flex items-center gap-1.5 text-[10px] text-ink-600">
              <span className="h-2.5 w-2.5 rounded-full bg-amber" /> Provider (7429)
            </span>
            <span className="flex items-center gap-1.5 text-[10px] text-ink-600">
              <span className="h-2.5 w-2.5 rounded-full bg-red" /> En Route
            </span>
            <span className="text-[10px] text-ink-500 ml-auto">
              <span className="font-semibold">ETA:</span> 11:13 AM
            </span>
          </div>
        </section>

        <section className="rounded-lg border border-line bg-white flex flex-col">
          <SectionHeader title="Priority Exceptions" subtitle="Requires dispatcher attention" action="/operations/control-tower/exceptions" />

          <div className="divide-y divide-line flex-1 overflow-y-auto max-h-[420px]">
            {EXCEPTIONS.map((exc) => (
              <Link
                key={exc.id}
                href={exc.href}
                className="flex w-full items-start gap-3 px-4 sm:px-5 py-3.5 text-left transition-colors hover:bg-surface-2 group"
              >
                <span className={`mt-0.5 flex-shrink-0 rounded-full px-2.5 py-0.5 text-[10px] font-bold ${EXCEPTION_STYLE[exc.severity]}`}>
                  {exc.badge}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold text-ink-950 group-hover:text-primary transition-colors">
                    {exc.title}
                  </div>
                  <div className="mt-0.5 text-xs text-ink-500">{exc.detail}</div>
                </div>
                <ChevronRight className="h-4 w-4 text-ink-300 group-hover:text-primary transition-colors flex-shrink-0 mt-1" />
              </Link>
            ))}
          </div>
        </section>
      </div>

      {/* ── Charts ── */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        <section className="rounded-lg border border-line bg-white p-4 sm:p-5">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-sm font-semibold text-ink-950">Weekly Volume</h3>
              <div className="text-xs text-ink-500">Completed jobs by day</div>
            </div>
            <BarChart3 className="h-4 w-4 text-ink-400" />
          </div>
          <ResponsiveContainer width="100%" height={200}>
            <RBarChart data={WEEKLY_VOLUME} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#E5E7EB" />
              <XAxis dataKey="day" tickLine={false} axisLine={false} tick={CHART_AXIS_STYLE} />
              <YAxis tickLine={false} axisLine={false} tick={CHART_AXIS_STYLE} />
              <Tooltip cursor={{ fill: "#F4F5FA" }} contentStyle={CHART_TOOLTIP_STYLE} />
              <Bar dataKey="value" fill="#2B2CAA" radius={[4, 4, 0, 0]} maxBarSize={32} />
            </RBarChart>
          </ResponsiveContainer>
        </section>

        <section className="rounded-lg border border-line bg-white p-4 sm:p-5">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-sm font-semibold text-ink-950">Weekly Trend</h3>
              <div className="text-xs text-ink-500">Provider availability trend</div>
            </div>
            <Activity className="h-4 w-4 text-ink-400" />
          </div>
          <ResponsiveContainer width="100%" height={200}>
            <RLineChart data={LINE_DATA} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#E5E7EB" />
              <XAxis dataKey="day" tickLine={false} axisLine={false} tick={CHART_AXIS_STYLE} />
              <YAxis tickLine={false} axisLine={false} tick={CHART_AXIS_STYLE} />
              <Tooltip contentStyle={CHART_TOOLTIP_STYLE} />
              <Line
                type="monotone"
                dataKey="value"
                stroke="#2B2CAA"
                strokeWidth={2.5}
                dot={{ r: 3, fill: "#2B2CAA", strokeWidth: 2, stroke: "#fff" }}
                activeDot={{ r: 5 }}
              />
            </RLineChart>
          </ResponsiveContainer>
        </section>
      </div>

      {/* ── Active Work Table ── */}
      <section className="overflow-hidden rounded-lg border border-line bg-white">
        <SectionHeader title="Active Work" subtitle="Most recent organization trips and assignments" action="/operations/control-tower" />

        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm min-w-[600px]">
            <thead>
              <tr className="bg-surface-1">
                <th className="border-b border-line px-3 sm:px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Reference</th>
                <th className="border-b border-line px-3 sm:px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Route</th>
                <th className="border-b border-line px-3 sm:px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Provider</th>
                <th className="border-b border-line px-3 sm:px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Status</th>
                <th className="border-b border-line px-3 sm:px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">ETA</th>
              </tr>
            </thead>
            <tbody>
              {ACTIVE_WORK.map((row) => (
                <tr
                  key={row.id}
                  onClick={() => window.location.href = row.href}
                  className="cursor-pointer hover:bg-surface-2 transition-colors group"
                >
                  <td className="border-b border-line px-3 sm:px-4 py-3 font-semibold text-ink-950 text-sm">{row.id}</td>
                  <td className="border-b border-line px-3 sm:px-4 py-3 text-ink-700 text-sm">{row.route}</td>
                  <td className="border-b border-line px-3 sm:px-4 py-3 text-ink-700 text-sm">{row.provider}</td>
                  <td className="border-b border-line px-3 sm:px-4 py-3">
                    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${WORK_STATUS_STYLE[row.status]} whitespace-nowrap`}>
                      {row.status}
                    </span>
                  </td>
                  <td className="border-b border-line px-3 sm:px-4 py-3 text-ink-600 text-sm">{row.eta}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* ── Quick Actions ── */}
      <section className="rounded-lg border border-line bg-white">
        <SectionHeader title="Quick Actions" subtitle="Common tasks for dispatchers" />
        <div className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-6 gap-2.5 p-4 sm:p-5">
          {QUICK_ACTIONS.map((action) => (
            <Link
              key={action.label}
              href={action.href}
              className="flex flex-col items-start gap-2.5 rounded-md border border-line px-3.5 py-3 text-xs font-semibold text-ink-800 hover:bg-surface-2 hover:border-ink-300 transition-colors"
            >
              <span className={`flex h-7 w-7 items-center justify-center rounded-md ${TONE_ICON_BG[action.tone]}`}>
                {action.icon}
              </span>
              {action.label}
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}