// src/app/(dashboard)/operations/trips/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  MapPin,
  Truck,
  Package,
  Clock,
  CheckCircle2,
  AlertCircle,
  SlidersHorizontal,
  X,
  ChevronUp,
  ChevronDown,
} from "lucide-react";

type Trip = {
  id: string;
  assignmentId: string;
  provider: string;
  providerId: string;
  asset: string;
  service: "Dispatch" | "Hauling";
  pickup: string;
  dropoff: string;
  branch: string;
  status: "Scheduled" | "En Route" | "At Pickup" | "In Transit" | "At Dropoff" | "Completed" | "Exception" | "Cancelled";
  stopCount: number;
  completedStops: number;
  eta: string;
  lastUpdated: string;
  progress: number;
};

const MOCK_TRIPS: Trip[] = [
  {
    id: "TRP-8491",
    assignmentId: "ASN-8491",
    provider: "Peter Okafor",
    providerId: "P-044",
    asset: "Bike BR-88",
    service: "Dispatch",
    pickup: "Ikeja",
    dropoff: "Lekki",
    branch: "Ikeja",
    status: "In Transit",
    stopCount: 3,
    completedStops: 1,
    eta: "18 min",
    lastUpdated: "12 sec ago",
    progress: 33,
  },
  {
    id: "TRP-8487",
    assignmentId: "ASN-8487",
    provider: "Musa Bello",
    providerId: "P-038",
    asset: "Van VN-04",
    service: "Hauling",
    pickup: "Apapa",
    dropoff: "Ikeja",
    branch: "Apapa",
    status: "At Pickup",
    stopCount: 2,
    completedStops: 0,
    eta: "25 min",
    lastUpdated: "45 sec ago",
    progress: 0,
  },
  {
    id: "TRP-8479",
    assignmentId: "ASN-8479",
    provider: "Amina Yusuf",
    providerId: "P-052",
    asset: "Bike BR-71",
    service: "Dispatch",
    pickup: "Yaba",
    dropoff: "Victoria Island",
    branch: "Yaba",
    status: "Scheduled",
    stopCount: 4,
    completedStops: 0,
    eta: "45 min",
    lastUpdated: "2 min ago",
    progress: 0,
  },
  {
    id: "TRP-8472",
    assignmentId: "ASN-8472",
    provider: "Kabiru Ahmed",
    providerId: "P-029",
    asset: "Truck TR-12",
    service: "Hauling",
    pickup: "Lekki",
    dropoff: "Marina",
    branch: "Lekki",
    status: "Exception",
    stopCount: 3,
    completedStops: 1,
    eta: "N/A",
    lastUpdated: "5 min ago",
    progress: 33,
  },
  {
    id: "TRP-8465",
    assignmentId: "ASN-8465",
    provider: "Chinedu Uche",
    providerId: "P-061",
    asset: "Bike BR-20",
    service: "Dispatch",
    pickup: "Surulere",
    dropoff: "Yaba",
    branch: "Yaba",
    status: "Completed",
    stopCount: 2,
    completedStops: 2,
    eta: "Completed",
    lastUpdated: "12 min ago",
    progress: 100,
  },
  {
    id: "TRP-8458",
    assignmentId: "ASN-8458",
    provider: "Tunde Adeyemi",
    providerId: "P-073",
    asset: "Bike BR-12",
    service: "Dispatch",
    pickup: "Ikeja",
    dropoff: "Ikorodu",
    branch: "Ikeja",
    status: "Cancelled",
    stopCount: 3,
    completedStops: 0,
    eta: "N/A",
    lastUpdated: "18 min ago",
    progress: 0,
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Scheduled: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  "En Route": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  "At Pickup": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "In Transit": { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "At Dropoff": { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Exception: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const SERVICE_ICONS = {
  Dispatch: <Package className="h-3.5 w-3.5" />,
  Hauling: <Truck className="h-3.5 w-3.5" />,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Active", "Completed", "Exception", "Cancelled"];

type SortField = "id" | "status" | "provider" | "eta" | "progress";
type SortDirection = "asc" | "desc";

const STATUS_ORDER = {
  Scheduled: 0,
  "En Route": 1,
  "At Pickup": 2,
  "In Transit": 3,
  "At Dropoff": 4,
  Completed: 5,
  Exception: 6,
  Cancelled: 7,
};

export default function TripsPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [statusFilter, setStatusFilter] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [trips, setTrips] = useState<Trip[]>([]);
  const [sortField, setSortField] = useState<SortField>("status");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setTrips(MOCK_TRIPS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredTrips = trips.filter((trip) => {
    const matchesSearch =
      search.trim() === "" ||
      trip.id.toLowerCase().includes(search.toLowerCase()) ||
      trip.provider.toLowerCase().includes(search.toLowerCase()) ||
      trip.asset.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || trip.branch === selectedBranch;

    const matchesStatus =
      statusFilter === "All" ||
      (statusFilter === "Active" &&
        ["Scheduled", "En Route", "At Pickup", "In Transit", "At Dropoff"].includes(trip.status)) ||
      (statusFilter === "Completed" && trip.status === "Completed") ||
      (statusFilter === "Exception" && trip.status === "Exception") ||
      (statusFilter === "Cancelled" && trip.status === "Cancelled");

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const sorted = [...filteredTrips].sort((a, b) => {
    let aVal: string | number = a[sortField as keyof Trip] as string | number;
    let bVal: string | number = b[sortField as keyof Trip] as string | number;

    if (sortField === "status") {
      aVal = STATUS_ORDER[a.status];
      bVal = STATUS_ORDER[b.status];
    } else if (sortField === "eta") {
      if (a.eta === "N/A" || a.eta === "Completed") aVal = "999";
      if (b.eta === "N/A" || b.eta === "Completed") bVal = "999";
    }

    if (aVal < bVal) return sortDirection === "asc" ? -1 : 1;
    if (aVal > bVal) return sortDirection === "asc" ? 1 : -1;
    return 0;
  });

  const activeCount = trips.filter((t) =>
    ["Scheduled", "En Route", "At Pickup", "In Transit", "At Dropoff"].includes(t.status)
  ).length;
  const completedCount = trips.filter((t) => t.status === "Completed").length;
  const exceptionCount = trips.filter((t) => t.status === "Exception").length;

  const hasActiveFilters = statusFilter !== "All" || selectedBranch !== "All branches" || search !== "";

  const clearFilters = () => {
    setStatusFilter("All");
    setSelectedBranch("All branches");
    setSearch("");
  };

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("asc");
    }
  };

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ChevronUp className="h-3 w-3 text-ink-300" />;
    return sortDirection === "asc" ? (
      <ChevronUp className="h-3 w-3 text-primary" />
    ) : (
      <ChevronDown className="h-3 w-3 text-primary" />
    );
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading trips...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Trips</h1>
        <p className="mt-1.5">Live operational trip projections from Dispatch and Hauling services.</p>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{activeCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Completed</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{completedCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Exceptions</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{exceptionCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search trips..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {statusFilter}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {(statusFilter !== "All" || selectedBranch !== "All branches") && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(statusFilter !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {sorted.length} trips
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setStatusFilter(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      statusFilter === status
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</span>
              <div className="flex flex-wrap gap-1.5">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
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
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-white">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-line bg-surface-1">
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("id")} className="flex items-center gap-1 hover:text-ink-700">
                  Trip <SortIcon field="id" />
                </button>
              </th>
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Service</th>
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("provider")} className="flex items-center gap-1 hover:text-ink-700">
                  Provider <SortIcon field="provider" />
                </button>
              </th>
              <th className="hidden md:table-cell px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Asset</th>
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("status")} className="flex items-center gap-1 hover:text-ink-700">
                  Status <SortIcon field="status" />
                </button>
              </th>
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("eta")} className="flex items-center gap-1 hover:text-ink-700">
                  ETA <SortIcon field="eta" />
                </button>
              </th>
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("progress")} className="flex items-center gap-1 hover:text-ink-700">
                  Progress <SortIcon field="progress" />
                </button>
              </th>
              <th className="px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">Action</th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-4 py-12 text-center text-ink-500">
                  <div className="flex flex-col items-center gap-2">
                    <Truck className="h-8 w-8 text-ink-300" />
                    <p className="text-sm font-medium">No trips match your filters</p>
                    <button onClick={clearFilters} className="text-sm text-primary hover:underline">
                      Clear filters
                    </button>
                  </div>
                </td>
              </tr>
            ) : (
              sorted.map((trip) => {
                const statusStyle = STATUS_STYLES[trip.status];
                const ServiceIcon = SERVICE_ICONS[trip.service];
                const isException = trip.status === "Exception";
                const isActive = ["Scheduled", "En Route", "At Pickup", "In Transit", "At Dropoff"].includes(trip.status);

                return (
                  <tr key={trip.id} className="border-b border-line transition-colors hover:bg-surface-1/50">
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <span className="font-mono font-semibold text-ink-950 text-sm">{trip.id}</span>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <span className="flex items-center gap-1 text-sm text-ink-700">
                        {ServiceIcon}
                        {trip.service}
                      </span>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary-bg text-[9px] font-bold text-primary flex-shrink-0">
                          {trip.provider.split(" ").map((n) => n[0]).join("")}
                        </div>
                        <span className="text-sm font-medium text-ink-950 truncate max-w-[80px] sm:max-w-none">
                          {trip.provider}
                        </span>
                      </div>
                    </td>
                    <td className="hidden md:table-cell px-2 py-2.5 whitespace-nowrap text-sm text-ink-700">{trip.asset}</td>
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <div className="flex items-center gap-1">
                        <span className={`flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1 w-1 rounded-full ${statusStyle.dot}`} />
                          {trip.status}
                        </span>
                        {isActive && (
                          <span className="flex items-center gap-0.5 rounded-full bg-green-bg px-1 py-0.5 text-[7px] font-semibold text-green">
                            <span className="relative flex h-0.5 w-0.5">
                              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                              <span className="relative inline-flex h-0.5 w-0.5 rounded-full bg-green" />
                            </span>
                            Live
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <span className={`text-sm font-medium ${isException ? "text-red" : "text-ink-700"}`}>
                        {trip.eta}
                      </span>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <div className="h-1 w-10 rounded-full bg-surface-3 overflow-hidden">
                          <div
                            className={`h-full rounded-full ${trip.progress === 100 ? "bg-green" : isException ? "bg-red" : "bg-primary"}`}
                            style={{ width: `${trip.progress}%` }}
                          />
                        </div>
                        <span className="text-[10px] font-medium text-ink-500">{trip.progress}%</span>
                      </div>
                    </td>
                    <td className="px-2 py-2.5 text-right whitespace-nowrap">
                      <Link
                        href={`/operations/trips/${trip.id}`}
                        className="inline-flex items-center gap-1 rounded border border-line px-2 py-0.5 text-[10px] font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                      >
                        View
                      </Link>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}