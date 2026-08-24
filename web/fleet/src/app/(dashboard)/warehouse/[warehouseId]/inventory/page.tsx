// src/app/(dashboard)/warehouse/inventory/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Package,
  RefreshCw,
  SlidersHorizontal,
  XCircle,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Boxes,
  Truck,
  CalendarDays,
  MoreHorizontal,
  Building2,
  Filter,
  Layers,
  Tag,
  Hash,
  ClipboardList,
  Warehouse,
  User,
  Edit3,
  Trash2,
} from "lucide-react";

type InventoryItem = {
  id: string;
  name: string;
  sku: string;
  owner: string;
  customer: string | null;
  location: string;
  warehouse: string;
  branch: string;
  lot: string | null;
  batch: string | null;
  serial: string | null;
  physical: number;
  reserved: number;
  available: number;
  status: "Ready" | "Low" | "Quarantine" | "Expired" | "Blocked";
  unit: string;
  lastUpdated: string;
  expiryDate: string | null;
};

// Mock data
const INVENTORY_ITEMS: InventoryItem[] = [
  {
    id: "INV-001",
    name: "Tyre 120/80 R17",
    sku: "SKU-2918",
    owner: "Fleet Stock",
    customer: null,
    location: "A-01-04",
    warehouse: "Ikeja Central",
    branch: "Ikeja",
    lot: "LOT-2024-001",
    batch: null,
    serial: null,
    physical: 24,
    reserved: 4,
    available: 20,
    status: "Ready",
    unit: "pcs",
    lastUpdated: "12 sec ago",
    expiryDate: null,
  },
  {
    id: "INV-002",
    name: "Brake Pads BR110",
    sku: "SKU-1440",
    owner: "Fleet Stock",
    customer: null,
    location: "A-02-11",
    warehouse: "Ikeja Central",
    branch: "Ikeja",
    lot: "LOT-2024-002",
    batch: null,
    serial: null,
    physical: 12,
    reserved: 2,
    available: 10,
    status: "Low",
    unit: "set",
    lastUpdated: "4 min ago",
    expiryDate: null,
  },
  {
    id: "INV-003",
    name: "Large Parcel Box",
    sku: "PKG-0802",
    owner: "Operations",
    customer: "Charitos Hospital",
    location: "B-04-03",
    warehouse: "Ikeja Central",
    branch: "Ikeja",
    lot: null,
    batch: "BATCH-2024-008",
    serial: null,
    physical: 184,
    reserved: 38,
    available: 146,
    status: "Ready",
    unit: "pcs",
    lastUpdated: "18 min ago",
    expiryDate: null,
  },
  {
    id: "INV-004",
    name: "Engine Oil 10W40",
    sku: "SKU-2207",
    owner: "Fleet Stock",
    customer: null,
    location: "C-02-02",
    warehouse: "Lekki Distribution",
    branch: "Lekki",
    lot: "LOT-2024-003",
    batch: null,
    serial: null,
    physical: 42,
    reserved: 0,
    available: 42,
    status: "Ready",
    unit: "L",
    lastUpdated: "1 hour ago",
    expiryDate: "2026-12-31",
  },
  {
    id: "INV-005",
    name: "Safety Vests",
    sku: "LOT-7721",
    owner: "Operations",
    customer: null,
    location: "Q-01-01",
    warehouse: "Ikeja Central",
    branch: "Ikeja",
    lot: "LOT-2024-004",
    batch: null,
    serial: null,
    physical: 18,
    reserved: 0,
    available: 18,
    status: "Quarantine",
    unit: "pcs",
    lastUpdated: "2 hours ago",
    expiryDate: null,
  },
  {
    id: "INV-006",
    name: "Truck Seal Set",
    sku: "SKU-1009",
    owner: "Hauling",
    customer: null,
    location: "D-03-08",
    warehouse: "Apapa Port Storage",
    branch: "Apapa",
    lot: null,
    batch: "BATCH-2024-012",
    serial: null,
    physical: 61,
    reserved: 15,
    available: 46,
    status: "Ready",
    unit: "set",
    lastUpdated: "3 hours ago",
    expiryDate: null,
  },
  {
    id: "INV-007",
    name: "Medical Grade Gloves",
    sku: "SKU-3312",
    owner: "Operations",
    customer: "Charitos Hospital",
    location: "B-01-09",
    warehouse: "Ikeja Central",
    branch: "Ikeja",
    lot: "LOT-2024-005",
    batch: "BATCH-2024-015",
    serial: null,
    physical: 850,
    reserved: 120,
    available: 730,
    status: "Ready",
    unit: "box",
    lastUpdated: "4 hours ago",
    expiryDate: "2025-06-30",
  },
  {
    id: "INV-008",
    name: "Forklift Battery",
    sku: "SKU-5517",
    owner: "Fleet Stock",
    customer: null,
    location: "C-03-01",
    warehouse: "Ikeja Central",
    branch: "Ikeja",
    lot: "LOT-2024-006",
    batch: null,
    serial: "SN-2024-088",
    physical: 2,
    reserved: 1,
    available: 1,
    status: "Blocked",
    unit: "pcs",
    lastUpdated: "1 day ago",
    expiryDate: null,
  },
  {
    id: "INV-009",
    name: "Pallet Wrap 500m",
    sku: "SKU-7720",
    owner: "Warehouse",
    customer: null,
    location: "E-01-12",
    warehouse: "Lekki Distribution",
    branch: "Lekki",
    lot: null,
    batch: "BATCH-2024-020",
    serial: null,
    physical: 32,
    reserved: 8,
    available: 24,
    status: "Low",
    unit: "roll",
    lastUpdated: "2 days ago",
    expiryDate: null,
  },
  {
    id: "INV-010",
    name: "Cold Chain Container",
    sku: "SKU-4410",
    owner: "Operations",
    customer: "Greenline Foods",
    location: "F-02-03",
    warehouse: "Ikeja Cold Storage",
    branch: "Ikeja",
    lot: "LOT-2024-007",
    batch: null,
    serial: "SN-2024-102",
    physical: 8,
    reserved: 3,
    available: 5,
    status: "Ready",
    unit: "pcs",
    lastUpdated: "3 days ago",
    expiryDate: null,
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Ready: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Low: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Quarantine: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Expired: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Blocked: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const WAREHOUSES = ["All warehouses", "Ikeja Central", "Lekki Distribution", "Apapa Port Storage", "Ikeja Cold Storage"];
const STATUS_FILTERS = ["All", "Ready", "Low", "Quarantine", "Expired", "Blocked"];
const OWNERS = ["All", "Fleet Stock", "Operations", "Hauling", "Warehouse"];

export default function InventorySearchPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedWarehouse, setSelectedWarehouse] = useState("All warehouses");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedOwner, setSelectedOwner] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<InventoryItem[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(INVENTORY_ITEMS);
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
      item.sku.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.location.toLowerCase().includes(search.toLowerCase()) ||
      (item.customer && item.customer.toLowerCase().includes(search.toLowerCase()));

    const matchesWarehouse =
      selectedWarehouse === "All warehouses" || item.warehouse === selectedWarehouse;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesOwner = selectedOwner === "All" || item.owner === selectedOwner;

    return matchesSearch && matchesWarehouse && matchesStatus && matchesOwner;
  });

  const stats = {
    total: data.length,
    totalPhysical: data.reduce((acc, d) => acc + d.physical, 0),
    totalAvailable: data.reduce((acc, d) => acc + d.available, 0),
    lowStock: data.filter((d) => d.status === "Low").length,
    quarantine: data.filter((d) => d.status === "Quarantine" || d.status === "Blocked").length,
  };

  const goToItemDetail = (itemId: string) => {
    router.push(`/warehouse/inventory/${itemId}`);
  };

  const goToAddItem = () => {
    router.push("/warehouse/inventory/create");
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-sm text-ink-600">Loading inventory...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedWarehouse !== "All warehouses" ||
    selectedStatus !== "All" ||
    selectedOwner !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedWarehouse("All warehouses");
    setSelectedStatus("All");
    setSelectedOwner("All");
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
                Inventory Search
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
              {stats.totalAvailable.toLocaleString()} available
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Stock by warehouse/location with owner, lot/batch, status, quantity, reservations and availability.
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
          <button
            onClick={goToAddItem}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Item
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Package className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total SKUs</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.total}</div>
            </div>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Boxes className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Physical Units</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {stats.totalPhysical.toLocaleString()}
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
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Available</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {stats.totalAvailable.toLocaleString()}
              </div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>93% of physical</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5">
              <AlertCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Low / Quarantine</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {stats.lowStock + stats.quarantine}
              </div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>{stats.lowStock} low, {stats.quarantine} blocked</span>
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
            placeholder="Search SKU, item, location, customer..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>

        <button
          onClick={() => {
            const warehouses = ["All warehouses", "Ikeja Central", "Lekki Distribution", "Apapa Port Storage", "Ikeja Cold Storage"];
            const currentIndex = warehouses.indexOf(selectedWarehouse);
            const nextIndex = (currentIndex + 1) % warehouses.length;
            setSelectedWarehouse(warehouses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Warehouse className="h-4 w-4 text-ink-400" />
          {selectedWarehouse === "All warehouses" ? "All" : selectedWarehouse}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Ready", "Low", "Quarantine", "Expired", "Blocked"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedStatus}
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
              {(selectedWarehouse !== "All warehouses" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0) +
                (selectedOwner !== "All" ? 1 : 0)}
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

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Owner</div>
              <div className="flex gap-1.5">
                {OWNERS.map((owner) => (
                  <button
                    key={owner}
                    onClick={() => setSelectedOwner(owner)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedOwner === owner
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {owner}
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

      {/* ===== INVENTORY TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Package className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No items found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No inventory items match your current filters.
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Item</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Owner</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Location</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Physical</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reserved</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Available</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Ready;
                  const availabilityRatio = item.physical > 0 ? Math.round((item.available / item.physical) * 100) : 0;

                  return (
                    <tr
                      key={item.id}
                      className="cursor-pointer hover:bg-surface-2 transition-colors group"
                      onClick={() => goToItemDetail(item.id)}
                    >
                      <td className="border-b border-line px-4 py-3">
                        <div>
                          <div className="font-medium text-ink-950">{item.name}</div>
                          <div className="flex flex-wrap items-center gap-2 text-[11px] text-ink-500">
                            <span className="font-mono">{item.sku}</span>
                            <span className="text-ink-300">·</span>
                            <span>{item.id}</span>
                            {item.lot && (
                              <>
                                <span className="text-ink-300">·</span>
                                <span className="font-mono text-[10px]">Lot: {item.lot}</span>
                              </>
                            )}
                            {item.customer && (
                              <>
                                <span className="text-ink-300">·</span>
                                <span className="text-primary">Customer: {item.customer}</span>
                              </>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{item.owner}</td>
                      <td className="border-b border-line px-4 py-3">
                        <div>
                          <div className="text-ink-600 font-mono text-[12px]">{item.location}</div>
                          <div className="text-[10px] text-ink-400">{item.warehouse}</div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3 font-semibold text-ink-950">{item.physical}</td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{item.reserved}</td>
                      <td className="border-b border-line px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-ink-950">{item.available}</span>
                          <span className="text-[10px] text-ink-400">{item.unit}</span>
                          <div className="w-12 h-1.5 rounded-full bg-surface-3 overflow-hidden">
                            <div
                              className="h-full rounded-full bg-primary"
                              style={{ width: `${availabilityRatio}%` }}
                            />
                          </div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-right">
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            goToItemDetail(item.id);
                          }}
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

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Package className="h-4 w-4" />
          WH-04 · Inventory Search
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} items shown
        </span>
      </div>
    </div>
  );
}