// src/app/(dashboard)/warehouse/[warehouseId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Warehouse,
  ArrowRight,
  Package,
  Boxes,
  Truck,
  Clock,
  MapPin,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Eye,
  TrendingUp,
  TrendingDown,
  MoreHorizontal,
  CalendarDays,
  Users,
  Building2,
  Dock,
  ClipboardList,
  ArrowUpRight,
  ArrowDownRight,
  Layers,
  Search,
  Filter,
  X,
  ChevronDown,
  Plus,
  Sparkles,
  Award,
  Activity,
  BarChart3,
  PieChart,
  Download,
  FileText,
  Printer,
} from "lucide-react";

type WarehouseDetail = {
  id: string;
  name: string;
  branch: string;
  timezone: string;
  status: "Active" | "Inactive" | "Maintenance";
  utilization: number;
  totalItems: number;
  activeWork: number;
  dockDoors: number;
  address: string;
  city: string;
  state: string;
  country: string;
  phone: string;
  email: string;
  manager: string;
  lastUpdated: string;
  storageZones: {
    id: string;
    name: string;
    type: string;
    capacity: number;
    used: number;
    status: "Available" | "Full" | "Maintenance";
  }[];
  recentActivity: {
    id: string;
    type: "Inbound" | "Outbound" | "Transfer" | "Adjustment";
    reference: string;
    items: number;
    timestamp: string;
    status: "Completed" | "In Progress" | "Pending";
  }[];
  dockSchedule: {
    id: string;
    door: number;
    vehicle: string;
    carrier: string;
    arrival: string;
    departure: string | null;
    status: "Arrived" | "Loading" | "Unloading" | "Departed" | "Scheduled";
  }[];
  yardVisits: {
    id: string;
    vehicle: string;
    driver: string;
    bay: string;
    checkIn: string;
    checkOut: string | null;
    status: "Checked In" | "In Progress" | "Checked Out";
  }[];
  monthlyStats: {
    inbound: number;
    outbound: number;
    transfers: number;
    adjustments: number;
  };
};

// Mock data
const MOCK_WAREHOUSE: WarehouseDetail = {
  id: "WH-001",
  name: "Ikeja Central Warehouse",
  branch: "Ikeja",
  timezone: "Africa/Lagos",
  status: "Active",
  utilization: 78,
  totalItems: 12450,
  activeWork: 18,
  dockDoors: 8,
  address: "42 Logistics Way",
  city: "Ikeja",
  state: "Lagos",
  country: "Nigeria",
  phone: "+234 800 123 4567",
  email: "ikeja.warehouse@apexlogistics.com",
  manager: "Oluwaseun Adebayo",
  lastUpdated: "12 sec ago",

  storageZones: [
    { id: "Z-001", name: "Aisle A - General", type: "General", capacity: 5000, used: 4200, status: "Available" },
    { id: "Z-002", name: "Aisle B - Heavy", type: "Heavy", capacity: 3000, used: 2800, status: "Full" },
    { id: "Z-003", name: "Cold Storage", type: "Cold Chain", capacity: 1500, used: 800, status: "Available" },
    { id: "Z-004", name: "Aisle C - Fragile", type: "Fragile", capacity: 2000, used: 1200, status: "Available" },
    { id: "Z-005", name: "Dock Zone", type: "Dock", capacity: 800, used: 600, status: "Available" },
  ],

  recentActivity: [
    { id: "ACT-001", type: "Inbound", reference: "IR-042", items: 284, timestamp: "12 min ago", status: "Completed" },
    { id: "ACT-002", type: "Outbound", reference: "OR-129", items: 156, timestamp: "28 min ago", status: "In Progress" },
    { id: "ACT-003", type: "Transfer", reference: "TF-027", items: 89, timestamp: "1 hour ago", status: "Completed" },
    { id: "ACT-004", type: "Adjustment", reference: "AD-003", items: 12, timestamp: "2 hours ago", status: "Pending" },
    { id: "ACT-005", type: "Inbound", reference: "IR-043", items: 320, timestamp: "3 hours ago", status: "In Progress" },
  ],

  dockSchedule: [
    { id: "DS-001", door: 1, vehicle: "Truck TR-112", carrier: "Apex Logistics", arrival: "08:30", departure: "10:15", status: "Departed" },
    { id: "DS-002", door: 2, vehicle: "Van VN-041", carrier: "Apex Logistics", arrival: "09:00", departure: null, status: "Unloading" },
    { id: "DS-003", door: 3, vehicle: "Truck TR-098", carrier: "Greenline Foods", arrival: "10:30", departure: null, status: "Scheduled" },
    { id: "DS-004", door: 4, vehicle: "Van VN-029", carrier: "Apex Logistics", arrival: "11:00", departure: null, status: "Scheduled" },
    { id: "DS-005", door: 5, vehicle: "Truck TR-112", carrier: "Apex Logistics", arrival: "13:00", departure: "14:30", status: "Scheduled" },
  ],

  yardVisits: [
    { id: "YV-001", vehicle: "Truck TR-112", driver: "Kabiru Ahmed", bay: "A-01", checkIn: "08:15", checkOut: "10:15", status: "Checked Out" },
    { id: "YV-002", vehicle: "Van VN-041", driver: "Musa Bello", bay: "B-02", checkIn: "08:45", checkOut: null, status: "In Progress" },
    { id: "YV-003", vehicle: "Truck TR-098", driver: "Chinedu Uche", bay: "A-03", checkIn: "10:20", checkOut: null, status: "Checked In" },
  ],

  monthlyStats: {
    inbound: 3420,
    outbound: 2890,
    transfers: 420,
    adjustments: 85,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Maintenance: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
};

const ACTIVITY_STATUS_STYLES: Record<string, { bg: string; text: string }> = {
  Completed: { bg: "bg-green-bg", text: "text-green" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber" },
  Pending: { bg: "bg-blue-bg", text: "text-blue" },
};

const DOCK_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Scheduled: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Arrived: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Loading: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Unloading: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Departed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const YARD_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  "Checked In": { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "Checked Out": { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const ACTIVITY_TYPES: Record<string, { icon: React.ReactNode; color: string }> = {
  Inbound: { icon: <ArrowDownRight className="h-3.5 w-3.5" />, color: "text-green" },
  Outbound: { icon: <ArrowUpRight className="h-3.5 w-3.5" />, color: "text-blue" },
  Transfer: { icon: <ArrowRight className="h-3.5 w-3.5" />, color: "text-amber" },
  Adjustment: { icon: <Activity className="h-3.5 w-3.5" />, color: "text-purple" },
};

export default function WarehouseDetailPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WarehouseDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_WAREHOUSE, id: warehouseId });
      setLoading(false);
    };
    loadData();
  }, [warehouseId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-sm text-ink-600">Loading warehouse details...</span>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Warehouse className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Warehouse not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The warehouse you're looking for doesn't exist.
          </p>
          <button
            onClick={() => router.push("/warehouse")}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            ← Back to warehouses
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Active;
  const isActive = data.status === "Active";

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/warehouse")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {data.name}
              </h1>
              <span className="font-mono text-xs text-ink-400 sm:text-sm">
                {data.id}
              </span>
              <span
                className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium sm:px-3 ${statusStyle.bg} ${statusStyle.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                {data.status}
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500 sm:text-sm">
              <span className="flex items-center gap-1.5">
                <MapPin className="h-3.5 w-3.5" />
                {data.address}, {data.city}, {data.state}
              </span>
              <span className="flex items-center gap-1.5">
                <Clock className="h-3.5 w-3.5" />
                {data.timezone}
              </span>
              <span className="flex items-center gap-1.5">
                <Users className="h-3.5 w-3.5" />
                Manager: {data.manager}
              </span>
            </div>
          </div>
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
            <Plus className="h-3.5 w-3.5" />
            New Work
          </button>
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Package className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total Items</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {data.totalItems.toLocaleString()}
              </div>
            </div>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Utilization</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{data.utilization}%</div>
            </div>
          </div>
          <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
            <div
              className={`h-full rounded-full ${
                data.utilization >= 80 ? "bg-green" :
                data.utilization >= 50 ? "bg-amber" :
                "bg-ink-300"
              }`}
              style={{ width: `${data.utilization}%` }}
            />
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5">
              <ClipboardList className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Active Work</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{data.activeWork}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <Clock className="h-3 w-3" />
            <span>12 in progress</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Building2 className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Dock Doors</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{data.dockDoors}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-blue">
            <Truck className="h-3 w-3" />
            <span>3 active</span>
          </div>
        </div>
      </div>

      {/* ===== OVERVIEW CONTENT ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* Left: Monthly Stats + Activity */}
        <div className="space-y-5">
          {/* Monthly Stats */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Monthly Activity</h3>
              <span className="text-xs text-ink-400">Current period</span>
            </div>
            <div className="grid grid-cols-4 gap-3">
              <div className="rounded-lg border border-line p-3 text-center">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Inbound</div>
                <div className="text-lg font-semibold text-green">{data.monthlyStats.inbound}</div>
              </div>
              <div className="rounded-lg border border-line p-3 text-center">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Outbound</div>
                <div className="text-lg font-semibold text-blue">{data.monthlyStats.outbound}</div>
              </div>
              <div className="rounded-lg border border-line p-3 text-center">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Transfers</div>
                <div className="text-lg font-semibold text-amber">{data.monthlyStats.transfers}</div>
              </div>
              <div className="rounded-lg border border-line p-3 text-center">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Adjustments</div>
                <div className="text-lg font-semibold text-purple">{data.monthlyStats.adjustments}</div>
              </div>
            </div>
          </div>

          {/* Recent Activity */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Recent Activity</h3>
              <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors">
                View all
              </button>
            </div>
            <div className="space-y-2">
              {data.recentActivity.map((activity) => {
                const typeStyle = ACTIVITY_TYPES[activity.type] || ACTIVITY_TYPES.Inbound;
                const statusStyle2 = ACTIVITY_STATUS_STYLES[activity.status] || ACTIVITY_STATUS_STYLES.Completed;
                return (
                  <div key={activity.id} className="flex items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2">
                    <div className="flex items-center gap-3">
                      <span className={typeStyle.color}>{typeStyle.icon}</span>
                      <div>
                        <div className="text-[13px] font-medium text-ink-950">
                          {activity.type} · {activity.reference}
                        </div>
                        <div className="text-[11px] text-ink-500">{activity.items} items · {activity.timestamp}</div>
                      </div>
                    </div>
                    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle2.bg} ${statusStyle2.text}`}>
                      {activity.status}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Right: Storage Zones + Quick Actions */}
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Storage Zones</h3>
              <button 
                onClick={() => router.push(`/warehouse/${warehouseId}/storage`)}
                className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1"
              >
                Manage <ArrowRight className="h-3 w-3" />
              </button>
            </div>
            <div className="space-y-2.5">
              {data.storageZones.map((zone) => {
                const percentage = Math.round((zone.used / zone.capacity) * 100);
                const isFull = zone.status === "Full";
                return (
                  <div key={zone.id} className="rounded-lg border border-line p-3">
                    <div className="flex items-center justify-between">
                      <div>
                        <div className="text-[13px] font-medium text-ink-950">{zone.name}</div>
                        <div className="text-[11px] text-ink-500">{zone.type} · {zone.used}/{zone.capacity} units</div>
                      </div>
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        isFull ? "bg-red-bg text-red" : "bg-green-bg text-green"
                      }`}>
                        {zone.status}
                      </span>
                    </div>
                    <div className="mt-1.5 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                      <div
                        className={`h-full rounded-full ${isFull ? "bg-red" : "bg-green"}`}
                        style={{ width: `${percentage}%` }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Quick Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button 
                onClick={() => router.push(`/warehouse/${warehouseId}/inbound`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <ArrowDownRight className="h-4 w-4 text-green" />
                  Receive Inbound
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button 
                onClick={() => router.push(`/warehouse/${warehouseId}/outbound`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <ArrowUpRight className="h-4 w-4 text-blue" />
                  Create Outbound
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button 
                onClick={() => router.push(`/warehouse/${warehouseId}/storage`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Layers className="h-4 w-4 text-amber" />
                  View Storage
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button 
                onClick={() => router.push(`/warehouse/${warehouseId}/docks`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Truck className="h-4 w-4 text-primary" />
                  Dock Schedule
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Warehouse className="h-4 w-4" />
          WH-02 · Warehouse Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}