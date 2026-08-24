// src/app/(dashboard)/warehouse/[warehouseId]/outbound/packing/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Package,
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
  Box,
  MoreHorizontal,
  Plus,
  Truck,
  User,
} from "lucide-react";

type PackUnit = {
  id: string;
  orderRef: string;
  packageType: string;
  items: number;
  weight: string;
  dimensions: string;
  status: "Pending" | "Packed" | "Ready" | "Dispatched";
  packedBy: string | null;
  dispatchedAt: string | null;
  tracking: string | null;
};

// Mock data
const PACK_UNITS: PackUnit[] = [
  { id: "PK-001", orderRef: "OR-129", packageType: "Box", items: 80, weight: "45 kg", dimensions: "60x40x30 cm", status: "Packed", packedBy: "O. Adebayo", dispatchedAt: null, tracking: null },
  { id: "PK-002", orderRef: "OR-129", packageType: "Box", items: 76, weight: "42 kg", dimensions: "60x40x30 cm", status: "Pending", packedBy: null, dispatchedAt: null, tracking: null },
  { id: "PK-003", orderRef: "OR-127", packageType: "Pallet", items: 120, weight: "180 kg", dimensions: "120x100x80 cm", status: "Ready", packedBy: "A. Yusuf", dispatchedAt: null, tracking: null },
  { id: "PK-004", orderRef: "OR-127", packageType: "Box", items: 60, weight: "35 kg", dimensions: "50x40x30 cm", status: "Dispatched", packedBy: "A. Yusuf", dispatchedAt: "2026-08-21 10:00", tracking: "TRK-001" },
  { id: "PK-005", orderRef: "OR-126", packageType: "Pallet", items: 340, weight: "420 kg", dimensions: "120x100x120 cm", status: "Ready", packedBy: "M. Bello", dispatchedAt: null, tracking: null },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Packed: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Box className="h-3.5 w-3.5" /> },
  Ready: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Dispatched: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <Truck className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Pending", "Packed", "Ready", "Dispatched"];

export default function PackingUnitsPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<PackUnit[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PACK_UNITS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.orderRef.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading packing units...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const goToPackDetail = (packId: string) => {
    router.push(`/warehouse/${warehouseId}/outbound/packing/${packId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Packing Units
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Manage packages, packing, and dispatch readiness.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />
          Create Pack Unit
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by pack unit or order..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Packed", "Ready", "Dispatched"];
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
              {selectedStatus !== "All" ? 1 : 0}
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

      {/* Cards instead of table for packing units */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Package className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No packing units found</h3>
          <p className="mt-1 text-sm text-ink-500">No packing units match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
            return (
              <div
                key={item.id}
                onClick={() => goToPackDetail(item.id)}
                className="group cursor-pointer rounded-xl border border-line bg-white p-4 transition hover:shadow-md hover:-translate-y-0.5"
              >
                <div className="flex items-start justify-between">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-[15px] font-semibold text-ink-950">{item.id}</span>
                      <span className="font-mono text-[11px] text-ink-400">{item.orderRef}</span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-ink-500">
                      <span>{item.packageType}</span>
                      <span className="text-ink-300">·</span>
                      <span>{item.items} items</span>
                      <span className="text-ink-300">·</span>
                      <span>{item.weight}</span>
                    </div>
                  </div>
                  <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                    {item.status}
                  </span>
                </div>
                <div className="mt-3 flex items-center justify-between text-sm text-ink-400">
                  <div className="flex items-center gap-2">
                    <User className="h-3.5 w-3.5" />
                    <span>{item.packedBy || "Not packed"}</span>
                  </div>
                  <div className="flex items-center gap-1 text-ink-400 group-hover:text-primary transition-colors">
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