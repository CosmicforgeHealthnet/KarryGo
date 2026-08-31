// src/app/(dashboard)/warehouse/[warehouseId]/transfers/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Warehouse,
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
  Truck,
  MapPin,
} from "lucide-react";

type TerminalTransfer = {
  id: string;
  reference: string;
  origin: string;
  destination: string;
  vehicle: string;
  driver: string;
  items: number;
  status: "Pending" | "Loading" | "In Transit" | "Arrived" | "Completed" | "Discrepancy";
  departure: string | null;
  arrival: string | null;
  manifestRef: string;
};

// Mock data
const TRANSFERS: TerminalTransfer[] = [
  { id: "TF-027", reference: "TF-027", origin: "Ikeja Central", destination: "Lekki Distribution", vehicle: "Truck TR-112", driver: "Kabiru Ahmed", items: 89, status: "Completed", departure: "2026-08-21 08:30", arrival: "2026-08-21 10:15", manifestRef: "MF-004" },
  { id: "TF-028", reference: "TF-028", origin: "Ikeja Central", destination: "Apapa Port Storage", vehicle: "Truck TR-098", driver: "Chinedu Uche", items: 120, status: "In Transit", departure: "2026-08-21 09:00", arrival: null, manifestRef: "MF-005" },
  { id: "TF-029", reference: "TF-029", origin: "Lekki Distribution", destination: "Ikeja Cold Storage", vehicle: "Van VN-041", driver: "Musa Bello", items: 45, status: "Pending", departure: null, arrival: null, manifestRef: "MF-006" },
  { id: "TF-030", reference: "TF-030", origin: "Apapa Port Storage", destination: "Ikeja Central", vehicle: "Truck TR-112", driver: "Kabiru Ahmed", items: 220, status: "Loading", departure: null, arrival: null, manifestRef: "MF-007" },
  { id: "TF-031", reference: "TF-031", origin: "Ikeja Central", destination: "Lekki Distribution", vehicle: "Truck TR-098", driver: "Chinedu Uche", items: 180, status: "Discrepancy", departure: "2026-08-20 14:00", arrival: "2026-08-20 16:30", manifestRef: "MF-008" },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Loading: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Truck className="h-3.5 w-3.5" /> },
  "In Transit": { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple", icon: <ArrowRight className="h-3.5 w-3.5" /> },
  Arrived: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Discrepancy: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Pending", "Loading", "In Transit", "Arrived", "Completed", "Discrepancy"];

export default function TerminalTransfersPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<TerminalTransfer[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(TRANSFERS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.origin.toLowerCase().includes(search.toLowerCase()) ||
      item.destination.toLowerCase().includes(search.toLowerCase()) ||
      item.vehicle.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading terminal transfers...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const goToTransferDetail = (transferId: string) => {
    router.push(`/warehouse/${warehouseId}/transfers/${transferId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Terminal Transfers
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Manage inter-warehouse transfers, manifests, and discrepancies.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />
          New Transfer
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, origin, destination..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Loading", "In Transit", "Arrived", "Completed", "Discrepancy"];
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

      {/* Table */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Warehouse className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No transfers found</h3>
          <p className="mt-1 text-sm text-ink-500">No terminal transfers match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Origin → Destination</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Vehicle</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Items</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const style = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToTransferDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.manifestRef}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-600">{item.origin}</div>
                        <div className="text-[11px] text-ink-400">→ {item.destination}</div>
                      </td>
                      <td className="px-4 py-3 font-mono text-[12px] text-ink-600">{item.vehicle}</td>
                      <td className="px-4 py-3 font-medium text-ink-950">{item.items}</td>
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