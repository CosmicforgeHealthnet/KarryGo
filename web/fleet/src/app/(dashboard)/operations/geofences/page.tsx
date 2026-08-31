// src/app/(dashboard)/operations/geofences/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  MapPin,
  SlidersHorizontal,
  X,
  ChevronDown,
  Trash2,
  Eye,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Layers,
  Grid3x3,
  Activity,
  Clock,
  ArrowRight,
  Globe,
  Navigation,
  Pencil,
  MoreVertical,
} from "lucide-react";

type Geofence = {
  id: string;
  name: string;
  type: "Pickup" | "Delivery" | "Restricted" | "Parking" | "Depot";
  branch: string;
  status: "Active" | "Inactive" | "Draft";
  geometry: string;
  area: string;
  createdAt: string;
  lastUpdated: string;
  createdBy: string;
  purpose: string;
};

// Mock data
const GEOFENCES: Geofence[] = [
  {
    id: "GEOF-001",
    name: "Ikeja Mall Pickup Zone",
    type: "Pickup",
    branch: "Ikeja",
    status: "Active",
    geometry: "Polygon · 12 vertices",
    area: "0.8 km²",
    createdAt: "2026-08-15",
    lastUpdated: "2 min ago",
    createdBy: "T. Adeyemi",
    purpose: "Designated pickup area for mall orders",
  },
  {
    id: "GEOF-002",
    name: "Lekki Phase 1 Restriction",
    type: "Restricted",
    branch: "Lekki",
    status: "Active",
    geometry: "Circle · 500m radius",
    area: "0.3 km²",
    createdAt: "2026-08-14",
    lastUpdated: "15 min ago",
    createdBy: "F. Daniel",
    purpose: "No entry during peak hours 7-9am",
  },
  {
    id: "GEOF-003",
    name: "Apapa Port Parking",
    type: "Parking",
    branch: "Apapa",
    status: "Inactive",
    geometry: "Polygon · 8 vertices",
    area: "1.2 km²",
    createdAt: "2026-08-12",
    lastUpdated: "1 day ago",
    createdBy: "M. Bello",
    purpose: "Truck parking and loading zone",
  },
  {
    id: "GEOF-004",
    name: "Yaba Tech Delivery Zone",
    type: "Delivery",
    branch: "Yaba",
    status: "Active",
    geometry: "Polygon · 10 vertices",
    area: "0.5 km²",
    createdAt: "2026-08-10",
    lastUpdated: "3 min ago",
    createdBy: "A. Yusuf",
    purpose: "Student delivery collection point",
  },
  {
    id: "GEOF-005",
    name: "Victoria Island Restricted Area",
    type: "Restricted",
    branch: "Lekki",
    status: "Draft",
    geometry: "Polygon · 14 vertices",
    area: "0.9 km²",
    createdAt: "2026-08-08",
    lastUpdated: "2 days ago",
    createdBy: "T. Adeyemi",
    purpose: "Diplomatic zone - special permit required",
  },
];

const TYPE_BADGES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pickup: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <MapPin className="h-3 w-3" />,
  },
  Delivery: {
    bg: "bg-primary-bg",
    text: "text-primary",
    icon: <Navigation className="h-3 w-3" />,
  },
  Restricted: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <AlertCircle className="h-3 w-3" />,
  },
  Parking: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <Layers className="h-3 w-3" />,
  },
  Depot: {
    bg: "bg-purple-bg",
    text: "text-purple",
    icon: <Globe className="h-3 w-3" />,
  },
};

const STATUS_BADGES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Draft: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const TYPES = ["All", "Pickup", "Delivery", "Restricted", "Parking", "Depot"];
const STATUSES = ["All", "Active", "Inactive", "Draft"];

export default function GeofencesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Geofence[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(GEOFENCES);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.purpose.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus =
      selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesBranch && matchesType && matchesStatus;
  });

  const activeCount = data.filter((d) => d.status === "Active").length;
  const totalCount = data.length;

  const handleViewGeofence = (id: string) => {
    router.push(`/operations/geofences/${id}`);
  };

  const handleCreateGeofence = () => {
    router.push("/operations/geofences/create");
  };

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedType !== "All" ||
    selectedStatus !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedType("All");
    setSelectedStatus("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading geofences...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Geofences</h1>
          <p className="mt-1.5">
            Manage geofences for branches, pickup zones, restricted areas, and parking locations.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={handleCreateGeofence}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Create geofence
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{activeCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Inactive / Draft</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {totalCount - activeCount}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Grid3x3 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total Area</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">3.7 km²</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search geofences..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Type:</span>
          {selectedType}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus}
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
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedType !== "All" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0)}
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
          {filteredData.length} geofences
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</span>
              <div className="flex flex-wrap gap-1.5">
                {TYPES.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedType(type)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedType === type
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {type}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUSES.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === status
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

      {/* ── Geofence Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <MapPin className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No geofences found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No geofences match your current filters. Create a new geofence to get started.
          </p>
          <button
            onClick={handleCreateGeofence}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Create geofence
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const typeBadge = TYPE_BADGES[item.type] || TYPE_BADGES.Pickup;
            const statusBadge = STATUS_BADGES[item.status] || STATUS_BADGES.Draft;
            const isActive = item.status === "Active";

            return (
              <div
                key={item.id}
                onClick={() => handleViewGeofence(item.id)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isActive
                    ? "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                    : item.status === "Draft"
                    ? "border-amber/20 bg-white hover:border-amber/40 hover:bg-surface-2"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={`flex h-9 w-9 items-center justify-center rounded-md ${typeBadge.bg} ${typeBadge.text}`}
                    >
                      {typeBadge.icon}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">
                          {item.name}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.id}
                        </span>
                      </div>
                      <div className="flex items-center gap-1.5 mt-0.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${typeBadge.bg} ${typeBadge.text}`}
                        >
                          {typeBadge.icon}
                          {item.type}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusBadge.bg} ${statusBadge.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${statusBadge.dot}`} />
                          {item.status}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Grid3x3 className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.area}</span>
                  </div>
                </div>

                <div className="mt-2 text-xs text-ink-600 line-clamp-2">
                  {item.purpose}
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    {item.lastUpdated}
                  </div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}