// src/app/(dashboard)/fleet/parts/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Package,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  ArrowRight,
  Box,
  AlertTriangle,
  ShoppingCart,
  BarChart3,
} from "lucide-react";

type Part = {
  id: string;
  name: string;
  sku: string;
  category: string;
  location: string;
  available: number;
  reserved: number;
  reorderLevel: number;
  status: "In Stock" | "Low Stock" | "Out of Stock" | "On Order";
  unit: string;
  lastUpdated: string;
  minStock?: number;
  maxStock?: number;
  supplier?: string;
  cost?: string;
};

const PARTS: Part[] = [
  {
    id: "P-001",
    name: "Brake Pads - Front",
    sku: "BRK-110",
    category: "Brakes",
    location: "Ikeja",
    available: 24,
    reserved: 4,
    reorderLevel: 10,
    status: "In Stock",
    unit: "Set",
    lastUpdated: "2 hrs ago",
    minStock: 5,
    maxStock: 50,
    supplier: "AutoParts Ltd",
    cost: "₦12,500",
  },
  {
    id: "P-002",
    name: "Engine Oil 10W-40",
    sku: "OIL-220",
    category: "Lubricants",
    location: "Lekki",
    available: 42,
    reserved: 0,
    reorderLevel: 15,
    status: "In Stock",
    unit: "Liter",
    lastUpdated: "5 hrs ago",
    minStock: 10,
    maxStock: 100,
    supplier: "PetroChem",
    cost: "₦8,200",
  },
  {
    id: "P-003",
    name: "Air Filter",
    sku: "FIL-330",
    category: "Filters",
    location: "Ikeja",
    available: 8,
    reserved: 5,
    reorderLevel: 12,
    status: "Low Stock",
    unit: "Unit",
    lastUpdated: "1 day ago",
    minStock: 5,
    maxStock: 30,
    supplier: "FilterPro",
    cost: "₦4,800",
  },
  {
    id: "P-004",
    name: "Tyre - 120/80 R17",
    sku: "TYR-440",
    category: "Tyres",
    location: "Apapa",
    available: 3,
    reserved: 1,
    reorderLevel: 8,
    status: "Low Stock",
    unit: "Unit",
    lastUpdated: "2 days ago",
    minStock: 4,
    maxStock: 20,
    supplier: "TyreWorld",
    cost: "₦45,000",
  },
  {
    id: "P-005",
    name: "Spark Plug",
    sku: "SPK-550",
    category: "Engine",
    location: "Yaba",
    available: 0,
    reserved: 0,
    reorderLevel: 20,
    status: "Out of Stock",
    unit: "Unit",
    lastUpdated: "3 days ago",
    minStock: 10,
    maxStock: 50,
    supplier: "EngineMart",
    cost: "₦2,300",
  },
  {
    id: "P-006",
    name: "Coolant Fluid",
    sku: "COL-660",
    category: "Fluids",
    location: "Ikeja",
    available: 15,
    reserved: 3,
    reorderLevel: 10,
    status: "In Stock",
    unit: "Liter",
    lastUpdated: "4 hrs ago",
    minStock: 5,
    maxStock: 40,
    supplier: "CoolTech",
    cost: "₦6,700",
  },
  {
    id: "P-007",
    name: "Brake Rotor",
    sku: "BRK-120",
    category: "Brakes",
    location: "Ikeja",
    available: 6,
    reserved: 2,
    reorderLevel: 6,
    status: "Low Stock",
    unit: "Unit",
    lastUpdated: "1 day ago",
    minStock: 4,
    maxStock: 20,
    supplier: "AutoParts Ltd",
    cost: "₦28,500",
  },
  {
    id: "P-008",
    name: "Headlight Bulb",
    sku: "LGT-770",
    category: "Lighting",
    location: "Lekki",
    available: 18,
    reserved: 0,
    reorderLevel: 8,
    status: "In Stock",
    unit: "Unit",
    lastUpdated: "6 hrs ago",
    minStock: 4,
    maxStock: 30,
    supplier: "LightPro",
    cost: "₦3,200",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  "In Stock": { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  "Low Stock": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  "Out of Stock": { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  "On Order": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: <ShoppingCart className="h-3.5 w-3.5" /> },
};

const CATEGORIES = ["All Categories", "Brakes", "Lubricants", "Filters", "Tyres", "Engine", "Fluids", "Lighting"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "In Stock", "Low Stock", "Out of Stock", "On Order"];

export default function PartsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All Categories");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Part[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PARTS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.sku.toLowerCase().includes(search.toLowerCase()) ||
      item.category.toLowerCase().includes(search.toLowerCase());

    const matchesCategory = selectedCategory === "All Categories" || item.category === selectedCategory;
    const matchesBranch = selectedBranch === "All branches" || item.location === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesCategory && matchesBranch && matchesStatus;
  });

  const lowStockCount = data.filter((d) => d.status === "Low Stock" || d.status === "Out of Stock").length;
  const totalStock = data.reduce((acc, d) => acc + d.available, 0);
  const totalCategories = [...new Set(data.map((d) => d.category))].length;

  const hasActiveFilters =
    selectedCategory !== "All Categories" ||
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All Categories");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading parts...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Parts Catalogue</h1>
          <p className="mt-1.5">Parts inventory with stock levels, locations, and reorder status.</p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => router.push("/fleet/parts/stock")}
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <BarChart3 className="h-3.5 w-3.5" />
            Stock
          </button>
          <button
            onClick={() => router.push("/fleet/parts/create")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Part
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Box className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total Stock</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalStock}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Low / Out</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{lowStockCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Package className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Categories</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalCategories}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, SKU, or category..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <Package className="h-3.5 w-3.5 text-ink-400" />
          {selectedCategory}
          <span className="text-ink-300">·</span>
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
              {(selectedCategory !== "All Categories" ? 1 : 0) +
                (selectedBranch !== "All branches" ? 1 : 0) +
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

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">{filteredData.length} parts</span>
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
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Category</span>
              <div className="flex flex-wrap gap-1.5">
                {CATEGORIES.map((category) => (
                  <button
                    key={category}
                    onClick={() => setSelectedCategory(category)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedCategory === category
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {category}
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

      {/* ── Parts Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Package className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No parts found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No parts match your current filters. Add a new part to get started.
          </p>
          <button
            onClick={() => router.push("/fleet/parts/create")}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add Part
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES["In Stock"];
            const isLow = item.status === "Low Stock" || item.status === "Out of Stock";
            const stockPercentage = Math.min((item.available / item.reorderLevel) * 100, 100);

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/fleet/parts/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isLow
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-md ${
                        isLow ? "bg-amber-bg text-amber" : "bg-primary-bg text-primary"
                      }`}
                    >
                      <Package className="h-4 w-4" />
                    </div>
                    <div>
                      <div className="text-sm font-semibold text-ink-950">{item.name}</div>
                      <div className="flex items-center gap-2 text-xs text-ink-500">
                        <span className="font-mono">{item.sku}</span>
                        <span className="text-ink-300">·</span>
                        <span>{item.category}</span>
                      </div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                  >
                    {statusStyle.icon}
                    {item.status}
                  </span>
                </div>

                <div className="mt-3">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-ink-500">Available</span>
                    <span className="font-semibold text-ink-950">{item.available} {item.unit}</span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className={`h-full rounded-full ${
                        item.status === "Out of Stock" ? "bg-red" : item.status === "Low Stock" ? "bg-amber" : "bg-green"
                      }`}
                      style={{ width: `${Math.min(stockPercentage, 100)}%` }}
                    />
                  </div>
                  <div className="mt-1 flex justify-between text-[10px] text-ink-400">
                    <span>Reorder: {item.reorderLevel}</span>
                    <span>{item.reserved > 0 ? `${item.reserved} reserved` : "No reservations"}</span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-500">
                    <MapPin className="h-3.5 w-3.5" />
                    {item.location}
                    {item.supplier && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span className="truncate max-w-[80px]">{item.supplier}</span>
                      </>
                    )}
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