// src/app/(dashboard)/warehouse/[warehouseId]/outbound/picking/page.tsx

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
  User,
  Boxes,
  MoreHorizontal,
  Plus,
  ListChecks,
} from "lucide-react";

type PickTask = {
  id: string;
  waveId: string;
  orderRef: string;
  itemName: string;
  sku: string;
  location: string;
  quantity: number;
  picked: number;
  picker: string | null;
  status: "Pending" | "In Progress" | "Completed" | "Skipped";
  priority: "High" | "Medium" | "Low";
};

// Mock data
const PICK_TASKS: PickTask[] = [
  { id: "PT-001", waveId: "WAVE-001", orderRef: "OR-129", itemName: "Tyre 120/80 R17", sku: "SKU-2918", location: "A-01-04", quantity: 80, picked: 50, picker: "O. Adebayo", status: "In Progress", priority: "High" },
  { id: "PT-002", waveId: "WAVE-001", orderRef: "OR-129", itemName: "Brake Pads BR110", sku: "SKU-1440", location: "A-02-11", quantity: 40, picked: 0, picker: null, status: "Pending", priority: "High" },
  { id: "PT-003", waveId: "WAVE-001", orderRef: "OR-129", itemName: "Medical Grade Gloves", sku: "SKU-3312", location: "B-01-09", quantity: 100, picked: 0, picker: null, status: "Pending", priority: "High" },
  { id: "PT-004", waveId: "WAVE-002", orderRef: "OR-128", itemName: "Engine Oil 10W40", sku: "SKU-2207", location: "C-02-02", quantity: 120, picked: 0, picker: null, status: "Pending", priority: "Medium" },
  { id: "PT-005", waveId: "WAVE-002", orderRef: "OR-128", itemName: "Truck Seal Set", sku: "SKU-1009", location: "D-03-08", quantity: 60, picked: 0, picker: null, status: "Pending", priority: "Medium" },
  { id: "PT-006", waveId: "WAVE-003", orderRef: "OR-127", itemName: "Safety Vests", sku: "LOT-7721", location: "Q-01-01", quantity: 100, picked: 100, picker: "A. Yusuf", status: "Completed", priority: "High" },
  { id: "PT-007", waveId: "WAVE-003", orderRef: "OR-127", itemName: "Forklift Battery", sku: "SKU-5517", location: "C-03-01", quantity: 4, picked: 4, picker: "A. Yusuf", status: "Completed", priority: "High" },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <User className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Skipped: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  High: { bg: "bg-red-bg", text: "text-red" },
  Medium: { bg: "bg-amber-bg", text: "text-amber" },
  Low: { bg: "bg-green-bg", text: "text-green" },
};

const STATUS_FILTERS = ["All", "Pending", "In Progress", "Completed", "Skipped"];

export default function PickWavesPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<PickTask[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PICK_TASKS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.orderRef.toLowerCase().includes(search.toLowerCase()) ||
      item.itemName.toLowerCase().includes(search.toLowerCase()) ||
      item.sku.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading pick tasks...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const goToTaskDetail = (taskId: string) => {
    router.push(`/warehouse/${warehouseId}/outbound/picking/${taskId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Pick Waves
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Manage pick waves, tasks, and order fulfilment.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />
          Create Wave
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by order, item, or SKU..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "In Progress", "Completed", "Skipped"];
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
            <ListChecks className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No pick tasks found</h3>
          <p className="mt-1 text-sm text-ink-500">No pick tasks match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Task</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Item</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Location</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Qty</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Picker</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                  const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Medium;
                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToTaskDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.id}</div>
                        <div className="text-[11px] text-ink-500">Wave: {item.waveId} · Order: {item.orderRef}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">{item.itemName}</div>
                        <div className="text-[11px] text-ink-500 font-mono">{item.sku}</div>
                      </td>
                      <td className="px-4 py-3 font-mono text-[12px] text-ink-600">{item.location}</td>
                      <td className="px-4 py-3">
                        <span className="text-ink-950 font-medium">{item.picked}/{item.quantity}</span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.picker || "—"}</td>
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