// src/app/(dashboard)/warehouse/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Warehouse as WarehouseIcon,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  TrendingUp,
  Package,
  Boxes,
  Truck,
  Building2,
} from "lucide-react";

type Warehouse = {
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
  lastUpdated: string;
};

const WAREHOUSES: Warehouse[] = [
  {
    id: "WH-001",
    name: "Ikeja Central Warehouse",
    branch: "Ikeja",
    timezone: "Africa/Lagos",
    status: "Active",
    utilization: 78,
    totalItems: 12450,
    activeWork: 18,
    dockDoors: 8,
    address: "42 Logistics Way, Ikeja, Lagos",
    lastUpdated: "12 sec ago",
  },
  {
    id: "WH-002",
    name: "Lekki Phase 1 Distribution",
    branch: "Lekki",
    timezone: "Africa/Lagos",
    status: "Active",
    utilization: 92,
    totalItems: 8400,
    activeWork: 24,
    dockDoors: 6,
    address: "15 Marine Road, Lekki Phase 1, Lagos",
    lastUpdated: "45 sec ago",
  },
  {
    id: "WH-003",
    name: "Apapa Port Storage",
    branch: "Apapa",
    timezone: "Africa/Lagos",
    status: "Maintenance",
    utilization: 45,
    totalItems: 3200,
    activeWork: 6,
    dockDoors: 10,
    address: "88 Port Access Road, Apapa, Lagos",
    lastUpdated: "3 min ago",
  },
  {
    id: "WH-004",
    name: "Yaba Fulfillment Center",
    branch: "Yaba",
    timezone: "Africa/Lagos",
    status: "Active",
    utilization: 65,
    totalItems: 6700,
    activeWork: 12,
    dockDoors: 4,
    address: "23 Herbert Macaulay Way, Yaba, Lagos",
    lastUpdated: "1 min ago",
  },
  {
    id: "WH-005",
    name: "Ikeja Cold Storage",
    branch: "Ikeja",
    timezone: "Africa/Lagos",
    status: "Active",
    utilization: 85,
    totalItems: 2800,
    activeWork: 9,
    dockDoors: 5,
    address: "7 Cold Chain Crescent, Ikeja, Lagos",
    lastUpdated: "2 min ago",
  },
  {
    id: "WH-006",
    name: "Lekki E-Commerce Hub",
    branch: "Lekki",
    timezone: "Africa/Lagos",
    status: "Inactive",
    utilization: 12,
    totalItems: 450,
    activeWork: 0,
    dockDoors: 3,
    address: "50 Admiralty Way, Lekki, Lagos",
    lastUpdated: "2 days ago",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Maintenance: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Clock className="h-3.5 w-3.5" /> },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Active", "Inactive", "Maintenance"];

export default function WarehousePage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Warehouse[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(WAREHOUSES);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.branch.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const stats = {
    total: data.length,
    active: data.filter((d) => d.status === "Active").length,
    totalItems: data.reduce((acc, d) => acc + d.totalItems, 0),
    utilization: Math.round(data.reduce((acc, d) => acc + d.utilization, 0) / data.length),
  };

  const hasActiveFilters = selectedBranch !== "All branches" || selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading warehouses...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Warehouses</h1>
          <p className="mt-1.5">Warehouse sites, utilization, inventory, and active operational work.</p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => router.push("/warehouse/create")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Warehouse
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <WarehouseIcon className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.total}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.active}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Package className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total Items</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {stats.totalItems.toLocaleString()}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Boxes className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Avg Utilization</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.utilization}%</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, ID, or branch..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
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
              {(selectedBranch !== "All branches" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0)}
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
          {filteredData.length} warehouses
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
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Warehouse Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <WarehouseIcon className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No warehouses found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No warehouses match your current filters. Add a new warehouse to get started.
          </p>
          <button
            onClick={() => router.push("/warehouse/create")}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add Warehouse
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
            const isActive = item.status === "Active";
            const utilizationColor =
              item.utilization >= 80
                ? "text-green"
                : item.utilization >= 50
                ? "text-amber"
                : "text-ink-500";

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/warehouse/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isActive
                    ? "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                    : item.status === "Maintenance"
                    ? "border-amber/20 bg-amber-bg/10 hover:border-amber/40 hover:bg-amber-bg/20"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-md ${
                        isActive ? "bg-green-bg text-green" :
                        item.status === "Maintenance" ? "bg-amber-bg text-amber" :
                        "bg-surface-2 text-ink-400"
                      }`}
                    >
                      <WarehouseIcon className="h-5 w-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{item.name}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.id}</span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          {statusStyle.icon}
                          {item.status}
                        </span>
                        <span className="flex items-center gap-1 text-[10px] text-ink-400">
                          <MapPin className="h-3 w-3" />
                          {item.branch}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Package className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.totalItems.toLocaleString()} items</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Truck className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.activeWork} active orders</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Building2 className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.dockDoors} dock doors</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.timezone.split("/")[1]}</span>
                  </div>
                </div>

                {/* Utilization bar */}
                <div className="mt-3 border-t border-line pt-2.5">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-ink-500">Utilization</span>
                    <span className={`font-semibold ${utilizationColor}`}>{item.utilization}%</span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className={`h-full rounded-full ${
                        item.utilization >= 80 ? "bg-green" :
                        item.utilization >= 50 ? "bg-amber" :
                        "bg-ink-300"
                      }`}
                      style={{ width: `${item.utilization}%` }}
                    />
                  </div>
                </div>

                {/* Address and actions */}
                <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400 truncate max-w-[65%]">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.address}</span>
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