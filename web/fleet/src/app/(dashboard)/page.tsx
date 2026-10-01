"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import Image from "next/image";
import { useRouter } from "next/navigation";
import {
  ChevronRight,
  Check,
  AlertTriangle,
  User,
  UserPlus,
  Truck,
  Calendar,
  MapPin,
  ShieldAlert,
  Wrench,
  CheckCircle2,
  FileText,
  Send,
} from "lucide-react";
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
  PieChart,
  Pie,
} from "recharts";
import { Map, Marker, Popup, Source, Layer } from "react-map-gl/maplibre";
import { setWorkerUrl } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useDashboardStore } from "@/lib/stores/dashboardStore";
import { dashboardService } from "@/lib/services/dashboardService";
import { useTheme } from "@/components/ThemeProvider";

if (typeof window !== "undefined") {
  setWorkerUrl("https://unpkg.com/maplibre-gl@6.8.0/dist/maplibre-gl-worker.mjs");
}

// ============================================
// CONSTANTS
// ============================================

const RANGE_OPTIONS = ["7D", "30D", "90D"] as const;

// Light/dark tint for base map layers.
const MAP_BRAND_COLORS_LIGHT = {
  background: "#f6f7f4",
  water: "#cfe3f5",
  park: "#dcebd9",
  landcoverWood: "#d7e8d3",
  landuseResidential: "#eef1ec",
};

const MAP_BRAND_COLORS_DARK = {
  background: "#0A0F1E",
  water: "#1a2a4a",
  park: "#14201a",
  landcoverWood: "#0f1a14",
  landuseResidential: "#111827",
};

// ============================================
// SHARED EMPTY STATE
// ============================================

function EmptyState({
  image,
  title,
  description,
  actionHref,
  actionLabel,
}: {
  image: string;
  title: string;
  description: string;
  actionHref?: string;
  actionLabel?: string;
}) {
  return (
    <div className="flex flex-col items-center justify-center text-center py-8 px-4">
      <div className="relative w-50 h-50">
        <Image src={image} alt={title} fill className="object-contain" />
      </div>
      <h4 className="text-sm font-semibold text-ink-950">{title}</h4>
      <p className="mt-1 text-xs text-ink-500 max-w-[220px]">{description}</p>
      {actionHref && actionLabel && (
        <Link
          href={actionHref}
          className="mt-4 inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary hover:bg-primary/10"
        >
          {actionLabel}
        </Link>
      )}
    </div>
  );
}

// ============================================
// SPARKLINE
// ============================================

function Sparkline({ data, color, id }: { data: number[]; color: string; id: string }) {
  const points = data.map((v, i) => ({ i, v }));
  return (
    <div className="h-8 w-16 flex-shrink-0">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={points} margin={{ top: 2, right: 2, left: 2, bottom: 2 }}>
          <defs>
            <linearGradient id={`spark-${id}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.35} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <Area
            type="monotone"
            dataKey="v"
            stroke={color}
            strokeWidth={2}
            fill={`url(#spark-${id})`}
            dot={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

// ============================================
// LIVE OPERATIONS MAP
// ============================================

type MapVehicle = {
  id: string;
  status: string;
  speed?: number;
  route?: string;
  latitude: number;
  longitude: number;
};

function LiveOperationsMap({
  vehicles,
  onRoute,
  idle,
}: {
  vehicles: MapVehicle[];
  onRoute: number;
  idle: number;
}) {
  const { theme } = useTheme();
  const isDark = theme === "dark";

  if (vehicles.length === 0) {
    return (
      <div className="w-full h-[220px] sm:h-[280px] lg:h-[340px] rounded-lg border border-line bg-surface-2 flex items-center justify-center">
        <EmptyState
          image="/images/live-operations-empty.png"
          title="No live vehicles"
          description="Vehicles will appear here in real time once they go on route."
        />
      </div>
    );
  }

  // Fit view around the returned markers.
  const lats = vehicles.map((v) => v.latitude);
  const lngs = vehicles.map((v) => v.longitude);
  const centerLng = (Math.min(...lngs) + Math.max(...lngs)) / 2;
  const centerLat = (Math.min(...lats) + Math.max(...lats)) / 2;

  const mapStyle = isDark
    ? "https://tiles.openfreemap.org/styles/liberty"
    : "https://tiles.openfreemap.org/styles/positron";

  const mapColors = isDark ? MAP_BRAND_COLORS_DARK : MAP_BRAND_COLORS_LIGHT;

  return (
    <div className="w-full h-[220px] sm:h-[280px] lg:h-[340px] rounded-lg overflow-hidden relative">
      <Map
        key={isDark ? "dark" : "light"}
        initialViewState={{ longitude: centerLng, latitude: centerLat, zoom: 7 }}
        mapStyle={mapStyle}
        style={{ width: "100%", height: "100%" }}
        attributionControl={false}
        onLoad={(e) => {
          const map = e.target;
          try {
            map.setPaintProperty("background", "background-color", mapColors.background);
            map.setPaintProperty("water", "fill-color", mapColors.water);
            map.setPaintProperty(
              "landuse_residential",
              "fill-color",
              mapColors.landuseResidential
            );
          } catch (_) {
            // Layer not present in this style.
          }
        }}
      >
        {vehicles.map((v) => (
          <Marker key={v.id} longitude={v.longitude} latitude={v.latitude}>
            <div className="relative flex h-6 w-6 items-center justify-center">
              <span className="absolute h-6 w-6 rounded-full bg-aurora-teal/40 animate-ping" />
              <span className="relative flex h-4 w-4 items-center justify-center rounded-full bg-aurora-teal border-2 border-white dark:border-surface shadow">
                <Truck className="h-2.5 w-2.5 text-white" />
              </span>
            </div>
          </Marker>
        ))}

        <Popup
          longitude={centerLng}
          latitude={centerLat}
          closeButton={false}
          closeOnClick={false}
          anchor="left"
          offset={14}
        >
          <div className="min-w-[110px] text-xs bg-white dark:bg-surface border border-line rounded-lg p-2 shadow-lg">
            <div className="font-semibold text-ink-950 flex items-center gap-1.5">
              <span className="h-1.5 w-1.5 rounded-full bg-aurora-teal animate-pulse" />
              Live
            </div>
            <div className="mt-1 space-y-0.5 text-ink-500">
              <div className="flex justify-between gap-3">
                <span>On route</span>
                <span className="font-semibold text-ink-950">{onRoute}</span>
              </div>
              <div className="flex justify-between gap-3">
                <span>Idle</span>
                <span className="font-semibold text-ink-950">{idle}</span>
              </div>
            </div>
          </div>
        </Popup>
      </Map>
    </div>
  );
}

// ============================================
// INSIGHTS RAIL
// ============================================

type ActivityRow = {
  id: string;
  type: string;
  title: string;
  time: string;
};

type ActivityIcon = {
  Icon: typeof Check;
  bg: string;
  color: string;
};

const ACTIVITY_ICON_PALETTE: ActivityIcon[] = [
  { Icon: CheckCircle2,  bg: "bg-emerald-500/10", color: "text-emerald-500" },
  { Icon: Send,          bg: "bg-blue-500/10",    color: "text-blue-500" },
  { Icon: FileText,      bg: "bg-amber-500/10",   color: "text-amber-500" },
  { Icon: Wrench,        bg: "bg-violet-500/10",  color: "text-violet-500" },
  { Icon: MapPin,        bg: "bg-cyan-500/10",    color: "text-cyan-500" },
  { Icon: AlertTriangle, bg: "bg-rose-500/10",    color: "text-rose-500" },
  { Icon: UserPlus,      bg: "bg-indigo-500/10",  color: "text-indigo-500" },
];

function hashString(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) | 0;
  }
  return Math.abs(h);
}

function iconForActivityType(type: string): ActivityIcon {
  const t = type.toLowerCase();

  // Keyword matches first — higher semantic fidelity.
  if (t.includes("complete")) {
    return ACTIVITY_ICON_PALETTE[0];
  }
  if (t.includes("fail") || t.includes("error") || t.includes("block") || t.includes("reject")) {
    return ACTIVITY_ICON_PALETTE[5];
  }
  if (t.includes("assign") || t.includes("driver") || t.includes("representative")) {
    return ACTIVITY_ICON_PALETTE[6];
  }
  if (t.includes("document") || t.includes("attach") || t.includes("file")) {
    return ACTIVITY_ICON_PALETTE[2];
  }
  if (t.includes("submit") || t.includes("sent") || t.includes("publish")) {
    return ACTIVITY_ICON_PALETTE[1];
  }
  if (t.includes("maint") || t.includes("service") || t.includes("inspect") || t.includes("update")) {
    return ACTIVITY_ICON_PALETTE[3];
  }
  if (
    t.includes("geofence") ||
    t.includes("location") ||
    t.includes("trip") ||
    t.includes("route")
  ) {
    return ACTIVITY_ICON_PALETTE[4];
  }
  if (t.includes("created") || t.includes("capabilit")) {
    return ACTIVITY_ICON_PALETTE[0];
  }

  // Deterministic fallback — same type always gets the same color.
  return ACTIVITY_ICON_PALETTE[hashString(t) % ACTIVITY_ICON_PALETTE.length];
}

function timeAgo(iso: string): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "";
  const sec = Math.max(1, Math.floor((Date.now() - then) / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  return `${day}d ago`;
}

function InsightsRail({
  status,
  activity,
  exceptions,
}: {
  status: { label: string; status: string }[];
  activity: ActivityRow[];
  exceptions: number;
}) {
  return (
    <>
      {/* System Status — hidden entirely when no status data. */}
      {status.length > 0 && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2 mb-3">
            <Check className="h-4 w-4 text-green" />
            <h3 className="text-sm font-semibold text-ink-950">All Systems Operational</h3>
          </div>
          <div className="space-y-2">
            {status.map((item) => (
              <div
                key={item.label}
                className="flex items-center justify-between py-1.5 border-b border-line last:border-0"
              >
                <span className="text-sm text-ink-700">{item.label}</span>
                <span className="text-xs font-semibold text-green flex items-center gap-1.5">
                  <span className="h-1.5 w-1.5 rounded-full bg-green" />
                  {item.status}
                </span>
              </div>
            ))}
            <div className="flex items-center justify-between py-1.5 pt-2 border-t border-line">
              <span className="text-sm text-ink-700">Open Exceptions</span>
              <span className="text-xs font-semibold text-amber">{exceptions}</span>
            </div>
          </div>
        </div>
      )}

      {/* Recent Activity */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-ink-950">Recent Activity</h3>
          <Link
            href="/operations/trips"
            className="text-xs font-semibold text-primary hover:text-primary-hover"
          >
            View all →
          </Link>
        </div>
        {activity.length === 0 ? (
          <EmptyState
            image="/images/alerts-empty.png"
            title="No recent activity"
            description="Fleet events will show up here as they happen."
          />
        ) : (
          <div className="space-y-3.5">
            {activity.map((a) => {
              const { Icon, bg, color } = iconForActivityType(a.type);
              return (
                <div key={a.id} className="flex items-start gap-3">
                  <div
                    className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full ${bg}`}
                  >
                    <Icon className={`h-4 w-4 ${color}`} />
                  </div>
                  <div className="flex-1 min-w-0 flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <p className="text-[13px] font-semibold text-ink-950 leading-tight">
                        {a.type}
                      </p>
                      <p className="text-[12px] text-ink-500 truncate leading-tight mt-0.5">
                        {a.title}
                      </p>
                    </div>
                    <span className="text-[11px] text-ink-400 flex-shrink-0 whitespace-nowrap pt-0.5">
                      {a.time}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* No Active Alerts — illustrated empty state + action button */}
      <div className="rounded-lg border border-line bg-surface p-6 text-center">
        <div className="flex flex-col items-center">
          <div className="relative w-50 h-50">
            <Image
              src="/images/alerts-empty.png"
              alt="No active alerts"
              fill
              className="object-contain"
            />
          </div>
          <h4 className="text-sm font-semibold text-ink-950">No active alerts</h4>
          <p className="text-xs text-ink-500 mt-1">
            You're all caught up! No new alerts at the moment.
          </p>
          <Link
            href="/operations/trips"
            className="mt-4 inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary hover:bg-primary/10"
          >
            View all notifications
          </Link>
        </div>
      </div>
    </>
  );
}

// ============================================
// MAIN PAGE
// ============================================

export default function DashboardHomePage() {
  const router = useRouter();
  const { _hasHydrated, accessToken } = useAuthStore();
  const { isLoading: orgLoading } = useOrganizationStore();
  const [range, setRange] = useState<(typeof RANGE_OPTIONS)[number]>("7D");

  const {
    stats,
    alerts,
    activeTrips,
    topVehicles,
    activity,
    systemStatus,
    weeklyVolume,
    weeklyTrend,
    isLoading: dashboardLoading,
    error: dashboardError,
    lastFetched,
    setDashboard,
    setLoading,
    setError,
  } = useDashboardStore();

  // Auth redirect.
  useEffect(() => {
    if (_hasHydrated && !accessToken) {
      router.replace("/sign-in");
    }
  }, [_hasHydrated, accessToken, router]);

  // Fetch dashboard once per session (cached for CACHE_TTL in store).
  useEffect(() => {
    if (!_hasHydrated || !accessToken) return;
    if (lastFetched && Date.now() - lastFetched < 30_000) return;

    let cancelled = false;
    setLoading(true);
    dashboardService
      .getDashboard()
      .then((data) => {
        if (!cancelled) setDashboard(data);
      })
      .catch((err) => {
        if (!cancelled) {
          const message =
            err?.response?.data?.error?.message ?? err?.message ?? "Failed to load dashboard";
          setError(message);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [_hasHydrated, accessToken, lastFetched, setDashboard, setLoading, setError]);

  if (!_hasHydrated || orgLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    );
  }

  if (!accessToken) return null;

  // ============================
  // Derived values (real only)
  // ============================

  const s = stats ?? null;

  const kpiRow = [
    {
      label: "Total Vehicles",
      value: s?.fleetReady ?? 0,
      trendPct: s?.fleetReadyTrend ?? 0,
      trendLabel: "vs. last 7 days",
      spark: s?.fleetReadySpark ?? [],
      icon: Truck,
      iconBg: "bg-blue-500/10",
      iconColor: "text-blue-500",
      sparkColor: "#2563EB",
    },
    {
      label: "Active Drivers",
      value: s?.activeDrivers ?? 0,
      trendPct: s?.activeDriversTrend ?? 0,
      trendLabel: "vs. last 7 days",
      spark: s?.activeDriversSpark ?? [],
      icon: User,
      iconBg: "bg-emerald-500/10",
      iconColor: "text-emerald-500",
      sparkColor: "#14B885",
    },
    {
      label: "Trips Today",
      value: s?.tripsToday ?? 0,
      trendPct: s?.tripsTodayTrend ?? 0,
      trendLabel: "vs. last 7 days",
      spark: s?.tripsTodaySpark ?? [],
      icon: MapPin,
      iconBg: "bg-violet-500/10",
      iconColor: "text-violet-500",
      sparkColor: "#7C5CFC",
    },
    {
      label: "Open Exceptions",
      value: s?.exceptions ?? 0,
      trendPct: s?.exceptionsTrend ?? 0,
      trendLabel: "vs. last 7 days",
      spark: s?.exceptionsSpark ?? [],
      icon: ShieldAlert,
      iconBg: "bg-rose-500/10",
      iconColor: "text-rose-500",
      sparkColor: "#F0447A",
    },
  ];

  // Fleet Performance — merge weeklyVolume + weeklyTrend into chart rows.
  const weeklyData = (() => {
    const len = Math.max(weeklyVolume.length, weeklyTrend.length);
    const rows: { day: string; trips: number; active: number }[] = [];
    for (let i = 0; i < len; i++) {
      const day = weeklyVolume[i]?.day ?? weeklyTrend[i]?.day ?? "";
      rows.push({
        day,
        trips: weeklyVolume[i]?.value ?? 0,
        active: weeklyTrend[i]?.value ?? 0,
      });
    }
    return rows;
  })();

  const hasPerformanceData = weeklyData.some(
    (d) => d.trips > 0 || d.active > 0
  );

  // Fleet Utilization donut — active vs idle.
  const activeCount = s?.onRoute ?? 0;
  const idleCount = s?.idle ?? 0;
  const utilizationTotal = activeCount + idleCount;
  const utilizationPct =
    utilizationTotal > 0 ? Math.round((activeCount / utilizationTotal) * 100) : 0;
  const utilizationData = [
    { name: "Active", value: activeCount },
    { name: "Idle", value: idleCount },
  ];

  // Live map — only vehicles with coordinates.
  const mapVehicles: MapVehicle[] = activeTrips
    .filter(
      (t): t is typeof t & { latitude: number; longitude: number } =>
        typeof t.latitude === "number" && typeof t.longitude === "number"
    )
    .map((t) => ({
      id: t.vehicleOrTruckReference || t.id,
      status: t.status,
      speed: t.speed,
      route: t.routeText || t.route,
      latitude: t.latitude,
      longitude: t.longitude,
    }));

  // Live Fleet list — same data, no coordinates needed.
  const liveFleet = activeTrips.map((t) => ({
    id: t.vehicleOrTruckReference || t.id,
    status: t.status,
    speedText:
      typeof t.speed === "number" ? `${Math.round(t.speed)} km/h` : "—",
    route: t.routeText || t.route || "—",
  }));

  // Activity rows for the rail.
  const activityRows: ActivityRow[] = activity.map((a) => ({
    id: a.id,
    type: a.type,
    title: a.title,
    time: timeAgo(a.createdAt),
  }));

  // ============================
  // RENDER
  // ============================

  return (
    <div className="flex flex-col 2xl:flex-row gap-6">
      {/* MAIN CONTENT */}
      <div className="flex-1 min-w-0 space-y-5 @container">
        {dashboardError && (
          <div className="rounded-lg border border-rose-300/40 bg-rose-500/10 px-4 py-2 text-xs text-rose-600 dark:text-rose-400">
            Could not load dashboard data: {dashboardError}
          </div>
        )}

        {/* ---- HERO BANNER ---- */}
        <section className="relative overflow-hidden rounded-xl p-5 sm:p-7 lg:p-8 shadow-sm border border-hero-border">
          <div
            aria-hidden
            className="absolute inset-0 -z-10 bg-gradient-to-r from-hero-gradient-from-light via-hero-gradient-via-light/70 to-transparent opacity-100 dark:opacity-0"
          />
          <div
            aria-hidden
            className="absolute inset-0 -z-10 bg-gradient-to-r from-hero-gradient-from-dark via-hero-gradient-via-dark/70 to-transparent opacity-0 dark:opacity-100"
          />
          <div className="flex flex-col sm:flex-row items-center gap-4 sm:gap-6">
            <div className="flex-1 w-full">
              <p className="text-xs sm:text-sm font-medium text-ink-500 flex items-center gap-2">
                Good morning, David
              </p>
              <h2 className="mt-1.5 text-2xl sm:text-3xl lg:text-[34px] font-bold tracking-[-0.01em] text-ink-950 leading-tight">
                Fleet operations, smarter and simpler.
              </h2>
              <p className="mt-2 text-sm sm:text-[15px] text-ink-500 max-w-lg">
                Monitor, manage and keep your fleet moving
              </p>
            </div>

            <div className="flex-shrink-0 w-full sm:w-[300px] lg:w-[480px] h-[110px] sm:h-[130px] lg:h-[150px] relative">
              <div className="absolute inset-y-0 left-0 w-1/3 z-10 pointer-events-none" />
              <Image
                src="/images/karryam-truck-light.png"
                alt="KarryGo Truck"
                fill
                className="object-contain opacity-100 dark:opacity-0"
                priority
              />
              <Image
                src="/images/karryam-truck-dark.png"
                alt="KarryGo Truck"
                fill
                className="object-contain opacity-0 dark:opacity-100"
                priority
              />
            </div>
          </div>
        </section>

        {/* ---- KPI ROW ---- */}
        <section className="grid grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
          {kpiRow.map((kpi) => {
            const Icon = kpi.icon;
            const trendSign = kpi.trendPct > 0 ? "+" : "";
            const trendText =
              kpi.trendPct === 0
                ? "0%"
                : `${trendSign}${kpi.trendPct.toFixed(0)}%`;
            const trendColor =
              kpi.trendPct > 0
                ? "text-green"
                : kpi.trendPct < 0
                ? "text-red"
                : "text-ink-500";

            const sparkData =
              kpi.spark.length > 0 ? kpi.spark : new Array(7).fill(0);

            return (
              <div
                key={kpi.label}
                className="rounded-lg border border-line bg-surface px-3 sm:px-4 py-3 sm:py-4"
              >
                <div className="flex items-center gap-2 mb-2">
                  <div
                    className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg ${kpi.iconBg}`}
                  >
                    <Icon className={`h-4 w-4 ${kpi.iconColor}`} />
                  </div>
                  <span className="text-[11px] sm:text-xs font-medium text-ink-500 truncate">
                    {kpi.label}
                  </span>
                </div>
                <div className="flex items-end justify-between gap-2">
                  <div className="min-w-0">
                    <div className="text-xl sm:text-[26px] font-bold text-ink-950 leading-none">
                      {kpi.value}
                    </div>
                    <div className="mt-1.5 flex items-center gap-1.5 flex-wrap">
                      <span className={`text-[11px] font-semibold ${trendColor}`}>
                        {trendText}
                      </span>
                      <span className="text-[10px] text-ink-400 truncate">
                        {kpi.trendLabel}
                      </span>
                    </div>
                  </div>
                  <Sparkline
                    data={sparkData}
                    color={kpi.sparkColor}
                    id={kpi.label.replace(/\s+/g, "-").toLowerCase()}
                  />
                </div>
              </div>
            );
          })}
        </section>

        {/* ---- FLEET PERFORMANCE ---- */}
        <section className="rounded-lg border border-line bg-surface p-4 sm:p-5">
          <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
            <div className="min-w-0">
              <h3 className="text-sm font-semibold text-ink-950">Fleet Performance</h3>
              <p className="text-[11px] text-ink-500">
                Vehicle activity and trips over time.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <div className="flex items-center rounded-lg bg-surface-2 p-0.5">
                {RANGE_OPTIONS.map((opt) => (
                  <button
                    key={opt}
                    type="button"
                    onClick={() => setRange(opt)}
                    className={`px-3 py-1.5 rounded-md text-[11px] font-semibold ${
                      range === opt
                        ? "bg-primary text-white shadow-sm"
                        : "text-ink-500 hover:text-ink-700 dark:hover:text-ink-300"
                    }`}
                  >
                    {opt}
                  </button>
                ))}
              </div>
              <button
                type="button"
                className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg border border-line text-ink-500 hover:bg-surface-2"
                aria-label="Pick date range"
              >
                <Calendar className="h-4 w-4" />
              </button>
            </div>
          </div>
          {!hasPerformanceData ? (
            <EmptyState
              image="/images/reports-empty.png"
              title="No performance data yet"
              description="Trends will appear here once trips start logging."
              actionHref="/operations/trips"
              actionLabel="View trips"
            />
          ) : (
            <>
              <div className="h-[220px] sm:h-[260px] lg:h-[300px]">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart
                    data={weeklyData}
                    margin={{ top: 4, right: 8, left: -12, bottom: 0 }}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      vertical={false}
                      stroke="var(--line)"
                    />
                    <XAxis
                      dataKey="day"
                      tickLine={false}
                      axisLine={false}
                      tick={{ fontSize: 10, fill: "var(--ink-500)" }}
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      tick={{ fontSize: 10, fill: "var(--ink-500)" }}
                    />
                    <Tooltip
                      contentStyle={{
                        borderRadius: 8,
                        border: "1px solid var(--line)",
                        fontSize: 12,
                        backgroundColor: "var(--surface)",
                        color: "var(--ink-950)",
                      }}
                    />
                    <Line
                      type="monotone"
                      dataKey="trips"
                      stroke="#1c3daabd"
                      strokeWidth={2}
                      dot={{ r: 3, fill: "#00C2A8", strokeWidth: 2, stroke: "#fff" }}
                    />
                    <Line
                      type="monotone"
                      dataKey="active"
                      stroke="#D99A24"
                      strokeWidth={2}
                      dot={{ r: 3, fill: "#D99A24", strokeWidth: 2, stroke: "#fff" }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
              <div className="flex flex-wrap items-center gap-3 mt-2 pt-2 border-t border-line">
                <span className="flex items-center gap-1 text-[11px] text-ink-500">
                  <span className="h-1.5 w-1.5 rounded-full bg-aurora-teal" /> Trips
                </span>
                <span className="flex items-center gap-1 text-[11px] text-ink-500">
                  <span className="h-1.5 w-1.5 rounded-full bg-amber" /> Active
                </span>
              </div>
            </>
          )}
        </section>

        {/* ---- UTILIZATION + TOP VEHICLES ---- */}
        <section className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Fleet Utilization */}
          <div className="rounded-lg border border-line bg-surface p-4 sm:p-5 flex flex-col items-center">
            <div className="text-center self-start">
              <h3 className="text-sm font-semibold text-ink-950">Fleet Utilization</h3>
              <p className="text-[11px] text-ink-500">Active vs Idle</p>
            </div>
            {utilizationTotal === 0 ? (
              <EmptyState
                image="/images/vehicles-empty.png"
                title="No fleet utilization data"
                description="Utilization will appear here once vehicles start operating."
                actionHref="/fleet/assets"
                actionLabel="View assets"
              />
            ) : (
              <>
                <div className="relative h-[150px] sm:h-[170px] w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <defs>
                        <linearGradient
                          id="utilizationGradient"
                          x1="0"
                          y1="0"
                          x2="1"
                          y2="1"
                        >
                          <stop offset="0%" stopColor="#0E7C6B" />
                          <stop offset="100%" stopColor="#5CE0C6" />
                        </linearGradient>
                      </defs>
                      <Pie
                        data={utilizationData}
                        cx="50%"
                        cy="50%"
                        innerRadius={44}
                        outerRadius={62}
                        startAngle={90}
                        endAngle={450}
                        dataKey="value"
                      >
                        <Cell fill="url(#utilizationGradient)" stroke="none" />
                        <Cell fill="var(--line)" stroke="none" />
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="absolute inset-0 flex items-center justify-center">
                    <span className="text-2xl font-bold text-ink-950">
                      {utilizationPct}%
                    </span>
                  </div>
                </div>
                <div className="mt-1 text-[12px] text-ink-500 flex items-center gap-2">
                  <span>
                    Active: <span className="font-semibold text-ink-700">{activeCount}</span>
                  </span>
                  <span className="text-line">|</span>
                  <span>
                    Idle: <span className="font-semibold text-ink-700">{idleCount}</span>
                  </span>
                </div>
              </>
            )}
          </div>

          {/* Top Performing Vehicles */}
          <div className="rounded-lg border border-line bg-surface p-4 sm:p-5">
            <div className="flex items-center justify-between mb-3">
              <div className="min-w-0">
                <h3 className="text-sm font-semibold text-ink-950 truncate">
                  Top Performing Vehicles
                </h3>
                <p className="text-[11px] text-ink-500 truncate">By trip volume</p>
              </div>
              <Link
                href="/fleet/assets"
                className="text-xs font-semibold text-primary hover:text-primary-hover flex-shrink-0 ml-2"
              >
                View All →
              </Link>
            </div>
            {topVehicles.length === 0 ? (
              <EmptyState
                image="/images/vehicles-empty.png"
                title="No vehicles yet"
                description="Your top performing vehicles will show up here once trips start rolling in."
                actionHref="/fleet/assets"
                actionLabel="Add a vehicle"
              />
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="text-[10px] uppercase tracking-wide text-ink-400">
                    <th className="font-medium pb-2 w-6">#</th>
                    <th className="font-medium pb-2">Vehicle</th>
                    <th className="font-medium pb-2">Trips</th>
                    <th className="font-medium pb-2 text-right">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {topVehicles.map((v) => (
                    <tr key={v.rank} className="hover:bg-surface-2">
                      <td className="py-1.5 text-xs font-medium text-ink-400">{v.rank}</td>
                      <td className="py-1.5">
                        <span className="flex items-center gap-2">
                          <Truck className="h-3.5 w-3.5 text-ink-400 flex-shrink-0" />
                          <span className="text-sm font-semibold text-ink-950">
                            {v.vehicle}
                          </span>
                        </span>
                      </td>
                      <td className="py-1.5 text-sm text-ink-600">{v.trips}</td>
                      <td className="py-1.5 text-right">
                        <span className="inline-flex items-center gap-1.5 text-[11px] font-semibold text-green">
                          <span className="h-1.5 w-1.5 rounded-full bg-green" />
                          {v.status}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </section>

        {/* ---- LIVE OPERATIONS MAP ---- */}
        <section className="rounded-lg border border-line bg-surface overflow-hidden">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 border-b border-line px-4 sm:px-5 py-3">
            <div className="min-w-0">
              <h3 className="text-sm font-semibold text-ink-950">Live Operations</h3>
              <p className="text-[11px] text-ink-500 truncate">
                Real-time view of active vehicles and assets.
              </p>
            </div>
            <Link
              href="/operations/control-tower"
              className="text-xs font-semibold text-primary hover:text-primary-hover flex-shrink-0"
            >
              View Map →
            </Link>
          </div>
          <div className="p-3 sm:p-4">
            <LiveOperationsMap
              vehicles={mapVehicles}
              onRoute={activeCount}
              idle={idleCount}
            />
          </div>
        </section>

        {/* ---- LIVE FLEET LIST ---- */}
        <section className="rounded-lg border border-line bg-surface">
          <div className="border-b border-line px-4 sm:px-5 py-3">
            <h3 className="text-sm font-semibold text-ink-950">Live Fleet</h3>
            <p className="text-[11px] text-ink-500">Active vehicles nearby</p>
          </div>
          {liveFleet.length === 0 ? (
            <EmptyState
              image="/images/trips-empty.png"
              title="No active trips"
              description="Vehicles currently on a trip will show up here."
              actionHref="/operations/trips"
              actionLabel="View trips"
            />
          ) : (
            <>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 divide-y sm:divide-y-0 divide-line">
                {liveFleet.map((v) => {
                  const isOnRoute =
                    v.status === "EN_ROUTE" || v.status === "IN_PROGRESS";
                  return (
                    <div
                      key={v.id}
                      className="px-4 sm:px-5 py-3 sm:border-r sm:border-line last:border-r-0 hover:bg-surface-2"
                    >
                      <div className="flex items-center justify-between">
                        <div className="min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="text-sm font-semibold text-ink-950">
                              {v.id}
                            </span>
                            <span
                              className={`text-[9px] font-medium px-1.5 py-0.5 rounded ${
                                isOnRoute
                                  ? "bg-green-bg text-green"
                                  : "bg-surface-2 text-ink-500"
                              }`}
                            >
                              {v.status}
                            </span>
                          </div>
                          <div className="text-xs text-ink-500 truncate">
                            {v.speedText} • {v.route}
                          </div>
                        </div>
                        {isOnRoute && (
                          <span className="h-2 w-2 rounded-full bg-green animate-pulse flex-shrink-0 ml-2" />
                        )}
                        {!isOnRoute && (
                          <span className="h-2 w-2 rounded-full bg-ink-400 flex-shrink-0 ml-2" />
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
              <div className="px-4 sm:px-5 py-2 bg-surface-2 flex items-center justify-between text-xs rounded-b-lg">
                <span className="text-ink-500">Active: {activeCount}</span>
                <span className="text-ink-400">Idle: {idleCount}</span>
              </div>
            </>
          )}
        </section>

        {/* ---- INSIGHTS RAIL (inline below 2xl) ---- */}
        <section className="2xl:hidden grid grid-cols-1 sm:grid-cols-2 gap-4">
          <InsightsRail
            status={systemStatus}
            activity={activityRows}
            exceptions={s?.exceptions ?? 0}
          />
        </section>
      </div>

      {/* RIGHT SIDEBAR */}
      <aside className="hidden 2xl:flex 2xl:flex-col w-[300px] flex-shrink-0 gap-4 sticky top-[86px] self-start">
        <InsightsRail
          status={systemStatus}
          activity={activityRows}
          exceptions={s?.exceptions ?? 0}
        />
      </aside>
    </div>
  );
}