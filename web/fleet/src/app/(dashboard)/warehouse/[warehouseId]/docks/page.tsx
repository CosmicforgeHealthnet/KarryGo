// src/app/(dashboard)/warehouse/[warehouseId]/docks/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Truck,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  MoreHorizontal,
  Plus,
  CalendarDays,
  User,
  MapPin,
} from "lucide-react";

type DockAppointment = {
  id: string;
  door: number;
  vehicle: string;
  carrier: string;
  driver: string;
  arrival: string;
  departure: string | null;
  status: "Scheduled" | "Arrived" | "Loading" | "Unloading" | "Departed" | "No-Show";
  orderRef: string | null;
};

// Mock data
const APPOINTMENTS: DockAppointment[] = [
  { id: "DS-001", door: 1, vehicle: "Truck TR-112", carrier: "Apex Logistics", driver: "Kabiru Ahmed", arrival: "08:30", departure: "10:15", status: "Departed", orderRef: "OR-126" },
  { id: "DS-002", door: 2, vehicle: "Van VN-041", carrier: "Apex Logistics", driver: "Musa Bello", arrival: "09:00", departure: null, status: "Unloading", orderRef: "OR-129" },
  { id: "DS-003", door: 3, vehicle: "Truck TR-098", carrier: "Greenline Foods", driver: "Chinedu Uche", arrival: "10:30", departure: null, status: "Scheduled", orderRef: "PO-1024" },
  { id: "DS-004", door: 4, vehicle: "Van VN-029", carrier: "Apex Logistics", driver: "Amina Yusuf", arrival: "11:00", departure: null, status: "Scheduled", orderRef: "OR-128" },
  { id: "DS-005", door: 5, vehicle: "Truck TR-112", carrier: "Apex Logistics", driver: "Kabiru Ahmed", arrival: "13:00", departure: "14:30", status: "Scheduled", orderRef: "OR-127" },
  { id: "DS-006", door: 1, vehicle: "Truck TR-098", carrier: "Solaris Logistics", driver: "Tunde Fatoki", arrival: "06:30", departure: null, status: "No-Show", orderRef: null },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Scheduled: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Arrived: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Loading: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Truck className="h-3.5 w-3.5" /> },
  Unloading: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <ArrowRight className="h-3.5 w-3.5" /> },
  Departed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  "No-Show": { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Scheduled", "Arrived", "Loading", "Unloading", "Departed", "No-Show"];
const DOORS = ["All", "1", "2", "3", "4", "5"];

export default function DockSchedulePage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedDoor, setSelectedDoor] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DockAppointment[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(APPOINTMENTS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.vehicle.toLowerCase().includes(search.toLowerCase()) ||
      item.carrier.toLowerCase().includes(search.toLowerCase()) ||
      item.driver.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesDoor = selectedDoor === "All" || item.door.toString() === selectedDoor;

    return matchesSearch && matchesStatus && matchesDoor;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading dock schedule...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedDoor !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedDoor("All");
  };

  const goToAppointmentDetail = (appointmentId: string) => {
    router.push(`/warehouse/${warehouseId}/docks/${appointmentId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Dock Schedule
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Manage dock appointments, arrivals, and departures.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />
          Schedule Dock
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by vehicle, carrier, or driver..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Scheduled", "Arrived", "Loading", "Unloading", "Departed", "No-Show"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const doors = ["All", "1", "2", "3", "4", "5"];
            const currentIndex = doors.indexOf(selectedDoor);
            const nextIndex = (currentIndex + 1) % doors.length;
            setSelectedDoor(doors[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          Door {selectedDoor}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedStatus !== "All" ? 1 : 0) + (selectedDoor !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* Filter Drawer */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
              <div className="flex gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedStatus === status
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
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

      {/* Table */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Truck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No appointments found</h3>
          <p className="mt-1 text-sm text-ink-500">No dock appointments match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[800px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Door</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Vehicle</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Carrier</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Arrival</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Departure</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const style = STATUS_STYLES[item.status] || STATUS_STYLES.Scheduled;
                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToAppointmentDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3 font-medium text-ink-950">Door {item.door}</td>
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.vehicle}</div>
                        <div className="text-[11px] text-ink-500">{item.driver}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.carrier}</td>
                      <td className="px-4 py-3 text-ink-600">{item.arrival}</td>
                      <td className="px-4 py-3 text-ink-600">{item.departure || "—"}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                          {style.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
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