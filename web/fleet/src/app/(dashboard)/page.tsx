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
  Send,
  MapPin,
  UserPlus,
  AlertCircle,
  Bell,
  BarChart3,
  ArrowRight,
  Calendar,
  Activity,
  Check,
  X,
} from "lucide-react";
import {
  BarChart as RBarChart,
  Bar,
  LineChart as RLineChart,  // ✅ Fixed: correctly import LineChart
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

// ===== GLOWING MAP NODE =====
function GlowNode({ tone = "indigo" }: { tone?: "indigo" | "green" }) {
  const glow =
    tone === "green"
      ? "shadow-[0_0_16px_5px_rgba(34,197,94,0.55)] bg-green-500"  // ✅ Fixed: bg-green-500
      : "shadow-[0_0_16px_5px_rgba(129,140,248,0.6)] bg-indigo-300";
  const ring = tone === "green" ? "ring-green-500/20" : "ring-indigo-400/20";  // ✅ Fixed
  const pingColor = tone === "green" ? "bg-green-500" : "bg-indigo-400";  // ✅ Fixed
  return (
    <span className="relative flex h-3 w-3">
      <span className={`absolute inline-flex h-full w-full rounded-full opacity-60 animate-ping ${pingColor}`} />
      <span className={`relative flex h-3 w-3 rounded-full ring-[7px] ${ring} ${glow}`} />
    </span>
  );
}

// ===== DARK LIVE OPERATIONS MAP =====
function DarkMapComponent() {
  return (
    <div className="w-full h-[260px] sm:h-[300px] rounded-lg overflow-hidden relative bg-gradient-to-br from-[#0A0C24] to-[#111536]">
      {/* local keyframes for the flowing route lines */}
      <style>{`
        @keyframes dashFlow { to { stroke-dashoffset: -24; } }
      `}</style>

      {/* depth vignette */}
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_30%_20%,rgba(129,140,248,0.14),transparent_55%)]" />

      {/* dot-grid texture */}
      <svg className="absolute inset-0 h-full w-full opacity-[0.12]" viewBox="0 0 400 260">
        <defs>
          <pattern id="dotGrid" width="16" height="16" patternUnits="userSpaceOnUse">
            <circle cx="1" cy="1" r="1" fill="white" />
          </pattern>
        </defs>
        <rect width="400" height="260" fill="url(#dotGrid)" />
      </svg>

      {/* faint terrain lines */}
      <svg className="absolute inset-0 h-full w-full opacity-[0.15]" viewBox="0 0 400 260" preserveAspectRatio="none">
        <path d="M0,40 C60,60 90,10 150,30 C210,50 240,10 400,35" stroke="white" strokeWidth="1" fill="none" />
        <path d="M0,120 C80,100 140,150 220,110 C280,85 340,130 400,100" stroke="white" strokeWidth="1" fill="none" />
        <path d="M20,260 C60,200 40,150 90,120" stroke="white" strokeWidth="1" fill="none" />
        <path d="M300,260 C310,210 280,180 320,140" stroke="white" strokeWidth="1" fill="none" />
      </svg>

      {/* route connections — glow trail + animated flowing line on top */}
      <svg className="absolute inset-0 h-full w-full" viewBox="0 0 400 260" preserveAspectRatio="none">
        <defs>
          <linearGradient id="routeGrad" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#818CF8" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#818CF8" stopOpacity="0.7" />
          </linearGradient>
        </defs>

        <polyline points="40,150 95,175 150,95 205,110 260,70 300,85 345,60" fill="none" stroke="#818CF8" strokeOpacity="0.25" strokeWidth="6" style={{ filter: "blur(3px)" }} />
        <polyline points="150,95 175,190 230,215" fill="none" stroke="#2DD4BF" strokeOpacity="0.2" strokeWidth="6" style={{ filter: "blur(3px)" }} />

        <polyline
          points="40,150 95,175 150,95 205,110 260,70 300,85 345,60"
          fill="none"
          stroke="url(#routeGrad)"
          strokeWidth="2"
          strokeDasharray="8 6"
          style={{ animation: "dashFlow 1.6s linear infinite" }}
        />
        <polyline
          points="150,95 175,190 230,215"
          fill="none"
          stroke="#2DD4BF"
          strokeOpacity="0.45"
          strokeWidth="2"
          strokeDasharray="8 6"
          style={{ animation: "dashFlow 1.6s linear infinite" }}
        />
      </svg>

      {/* junction dots */}
      <span className="absolute h-1.5 w-1.5 rounded-full bg-teal-300/80" style={{ left: "44%", top: "37%" }} />
      <span className="absolute h-1.5 w-1.5 rounded-full bg-teal-300/80" style={{ left: "65%", top: "27%" }} />

      {/* markers */}
      <div className="absolute" style={{ left: "8%", top: "56%" }}><GlowNode tone="indigo" /></div>
      <div className="absolute" style={{ left: "35%", top: "35%" }}><GlowNode tone="indigo" /></div>
      <div className="absolute" style={{ left: "63%", top: "25%" }}><GlowNode tone="indigo" /></div>
      <div className="absolute" style={{ left: "85%", top: "21%" }}><GlowNode tone="indigo" /></div>
      <div className="absolute" style={{ left: "42%", top: "72%" }}><GlowNode tone="green" /></div>
      <div className="absolute" style={{ left: "88%", top: "58%" }}><GlowNode tone="green" /></div>

      {/* floating stat card */}
      <div className="absolute left-4 top-1/2 -translate-y-1/2 w-[128px] rounded-lg bg-[#080A1E]/90 backdrop-blur-sm border border-white/10 shadow-lg shadow-black/40 px-3.5 py-3 text-white overflow-hidden">
        <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-white/40 to-transparent" />
        <div className="flex items-center justify-between text-[11px] font-semibold text-white/85">
          <span className="flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse" />
            Ikeja Hub
          </span>
          <ChevronRight className="h-3 w-3 text-white/50" />
        </div>
        <div className="mt-2.5 space-y-1.5 text-[11px] text-white/50">
          <div className="flex items-center justify-between">
            <span>Active Work</span>
            <span className="text-white font-bold text-sm">12</span>
          </div>
          <div className="flex items-center justify-between">
            <span>On Route</span>
            <span className="text-white font-semibold">8</span>
          </div>
          <div className="flex items-center justify-between">
            <span>Idle</span>
            <span className="text-white font-semibold">4</span>
          </div>
        </div>
      </div>

      {/* legend */}
      <div className="absolute bottom-3 right-3 flex items-center gap-3 rounded-full bg-[#080A1E]/80 backdrop-blur-sm border border-white/10 px-3 py-1.5 text-[10px] text-white/70">
        <span className="flex items-center gap-1.5"><span className="h-1.5 w-1.5 rounded-full bg-indigo-300" /> En route</span>
        <span className="flex items-center gap-1.5"><span className="h-1.5 w-1.5 rounded-full bg-green-500" /> Delivered</span>
      </div>
    </div>
  );
}

type Tone = "blue" | "green" | "amber" | "red" | "neutral";
type DateRange = "Today" | "Yesterday" | "This Week" | "This Month";
type Branch = "All Branches" | "Ikeja" | "Lekki" | "Yaba" | "Apapa";

const TONE_ICON_BG: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green-500",
  amber: "bg-amber-bg text-amber-500",
  red: "bg-red-bg text-red-500",
  neutral: "bg-surface-2 text-ink-600",
};

const DATE_OPTIONS: DateRange[] = ["Today", "Yesterday", "This Week", "This Month"];
const BRANCH_OPTIONS: Branch[] = ["All Branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

const HERO_STATS = [
  { label: "Active Work", value: "128" },
  { label: "Fleet Ready", value: "96" },
  { label: "Open Exceptions", value: "14" },
];

const PERFORMANCE_STRIP = [
  { label: "Total Trips", value: "320", trend: "+8%", trendUp: true },
  { label: "On-time Performance", value: "91.2%", trend: "+4.3%", trendUp: true },
  { label: "Completed Trips", value: "286", trend: "+12%", trendUp: true },
  { label: "Incidents Reported", value: "7", trend: "-22%", trendUp: false },
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
  critical: "bg-red-bg text-red-500",
  high: "bg-amber-bg text-amber-500",
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
  Delayed: "bg-amber-bg text-amber-500",
  Completed: "bg-green-bg text-green-500",
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

      {/* ── Hero Status Banner ── */}
      <section className="relative overflow-hidden rounded-xl bg-gradient-to-br from-[#2D2A8C] via-[#3B2FA0] to-[#4C3CC4] px-6 sm:px-8 py-7 text-white">
        {/* decorative glow blobs + waves */}
        <div className="pointer-events-none absolute inset-0">
          <div className="absolute -right-8 -top-16 h-56 w-56 rounded-full bg-white/10 blur-3xl" />
          <div className="absolute right-16 -bottom-16 h-44 w-72 rounded-full bg-indigo-300/10 blur-3xl" />
          <svg className="absolute inset-0 h-full w-full opacity-70" preserveAspectRatio="none" viewBox="0 0 800 200">
            <path d="M0,140 C120,180 260,60 400,110 C540,160 660,70 800,120 L800,200 L0,200 Z" fill="white" fillOpacity="0.08" />
            <path d="M0,165 C160,105 320,195 480,135 C600,95 700,155 800,135 L800,200 L0,200 Z" fill="white" fillOpacity="0.12" />
          </svg>
        </div>

        <div className="relative flex flex-col lg:flex-row lg:items-start lg:justify-between gap-6">
          <div>
            <div className="text-lg sm:text-xl font-bold tracking-[-0.01em]">All Systems Running</div>
            <div className="mt-1 text-xs text-white/60">Your fleet operations are on track for today.</div>
            <span className="mt-4 flex h-10 w-10 items-center justify-center rounded-full bg-white/10 ring-4 ring-green-500/30 shadow-[inset_0_1px_1px_rgba(255,255,255,0.4),0_2px_6px_rgba(0,0,0,0.25)]">
              <Check className="h-[18px] w-[18px] text-green-500" strokeWidth={3} />
            </span>
          </div>

          <div className="flex flex-wrap gap-8 sm:gap-12">
            {HERO_STATS.map((s) => (
              <div key={s.label}>
                <div className="text-[11px] font-medium text-white/60">{s.label}</div>
                <div className="mt-1.5 text-2xl sm:text-[28px] font-bold tracking-[-0.02em]">{s.value}</div>
              </div>
            ))}
          </div>
        </div>

        <div className="relative mt-5 flex justify-end">
          <span className="flex items-center gap-1.5 text-[11px] font-medium text-white/70">
            <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse" /> Live · {liveTime}
          </span>
        </div>
      </section>

      {/* ── Map + Exceptions ── */}
      <div className="grid grid-cols-1 xl:grid-cols-[1.5fr_1fr] gap-5">
        <section className="overflow-hidden rounded-lg border border-line bg-white">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 border-b border-line px-4 sm:px-5 py-3.5">
            <div>
              <h3 className="text-sm font-semibold text-ink-950">Live Operations</h3>
              <div className="mt-0.5 text-xs text-ink-500">Real-time view of active work and assets</div>
            </div>
            <Link href="/operations/control-tower" className="text-xs font-semibold text-primary hover:text-primary-hover transition-colors">
              View Full Map →
            </Link>
          </div>

          <div className="p-4 sm:p-5">
            <DarkMapComponent />
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

      {/* ── Today's Performance ── */}
      <section>
        <h3 className="text-sm font-semibold text-ink-950 mb-3">Today's Performance</h3>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          {PERFORMANCE_STRIP.map((p) => (
            <div key={p.label} className="rounded-lg border border-line bg-white px-4 sm:px-5 py-4">
              <div className="text-xs font-medium text-ink-500">{p.label}</div>
              <div className="mt-1.5 text-xl sm:text-2xl font-bold tracking-[-0.02em] text-ink-950">{p.value}</div>
              <div className={`mt-1 text-[11px] font-semibold ${p.trendUp ? "text-green-500" : "text-red-500"}`}>{p.trend}</div>
            </div>
          ))}
        </div>
      </section>

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