// src/app/(dashboard)/warehouse/[warehouseId]/returns/page.tsx

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
  MoreHorizontal,
  Plus,
  Truck,
  User,
  MapPin,
  ArrowLeft,
} from "lucide-react";

type ReturnItem = {
  id: string;
  reference: string;
  source: string;
  orderRef: string | null;
  reason: "Damaged" | "Wrong Item" | "Customer Return" | "Quality Issue" | "Other";
  items: number;
  condition: "Good" | "Damaged" | "Unsellable";
  status: "Received" | "Inspecting" | "Quarantined" | "Disposed" | "Returned to Stock";
  receivedAt: string;
  disposedAt: string | null;
  notes: string | null;
};

// Mock data
const RETURNS: ReturnItem[] = [
  { id: "RET-001", reference: "RET-001", source: "Charitos Hospital", orderRef: "OR-129", reason: "Damaged", items: 5, condition: "Damaged", status: "Inspecting", receivedAt: "2026-08-21 09:30", disposedAt: null, notes: "5 damaged brake pads. Inspecting for warranty claim." },
  { id: "RET-002", reference: "RET-002", source: "Oakfield Stores", orderRef: "OR-128", reason: "Wrong Item", items: 12, condition: "Good", status: "Returned to Stock", receivedAt: "2026-08-20 14:00", disposedAt: null, notes: "Wrong SKU sent. Returned to stock." },
  { id: "RET-003", reference: "RET-003", source: "Greenline Foods", orderRef: "OR-126", reason: "Quality Issue", items: 8, condition: "Unsellable", status: "Quarantined", receivedAt: "2026-08-20 10:00", disposedAt: null, notes: "Quality issue with batch. Quarantined pending investigation." },
  { id: "RET-004", reference: "RET-004", source: "Solaris Construction", orderRef: "OR-127", reason: "Customer Return", items: 2, condition: "Damaged", status: "Disposed", receivedAt: "2026-08-19 16:30", disposedAt: "2026-08-20 08:00", notes: "Damaged beyond repair. Disposed." },
  { id: "RET-005", reference: "RET-005", source: "BlueMarket Retail", orderRef: "OR-125", reason: "Other", items: 3, condition: "Good", status: "Received", receivedAt: "2026-08-21 11:00", disposedAt: null, notes: "Overstock return awaiting inspection." },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Received: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Inspecting: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Eye className="h-3.5 w-3.5" /> },
  Quarantined: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Disposed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  "Returned to Stock": { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

const REASON_STYLES: Record<string, { bg: string; text: string }> = {
  Damaged: { bg: "bg-red-bg", text: "text-red" },
  "Wrong Item": { bg: "bg-amber-bg", text: "text-amber" },
  "Customer Return": { bg: "bg-blue-bg", text: "text-blue" },
  "Quality Issue": { bg: "bg-purple-bg", text: "text-purple" },
  Other: { bg: "bg-surface-2", text: "text-ink-500" },
};

const CONDITION_STYLES: Record<string, { bg: string; text: string }> = {
  Good: { bg: "bg-green-bg", text: "text-green" },
  Damaged: { bg: "bg-red-bg", text: "text-red" },
  Unsellable: { bg: "bg-red-bg", text: "text-red" },
};

const STATUS_FILTERS = ["All", "Received", "Inspecting", "Quarantined", "Disposed", "Returned to Stock"];
const REASON_FILTERS = ["All", "Damaged", "Wrong Item", "Customer Return", "Quality Issue", "Other"];
const CONDITION_FILTERS = ["All", "Good", "Damaged", "Unsellable"];

export default function ReturnsPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedReason, setSelectedReason] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ReturnItem[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(RETURNS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.source.toLowerCase().includes(search.toLowerCase()) ||
      item.orderRef?.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesReason = selectedReason === "All" || item.reason === selectedReason;

    return matchesSearch && matchesStatus && matchesReason;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading returns...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedReason !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedReason("All");
  };

  const goToReturnDetail = (returnId: string) => {
    router.push(`/warehouse/${warehouseId}/returns/${returnId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Returns
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Manage returns, inspections, disposition, and credit/refund processing.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />
          New Return
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, source, or order..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Received", "Inspecting", "Quarantined", "Disposed", "Returned to Stock"];
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
            const reasons = ["All", "Damaged", "Wrong Item", "Customer Return", "Quality Issue", "Other"];
            const currentIndex = reasons.indexOf(selectedReason);
            const nextIndex = (currentIndex + 1) % reasons.length;
            setSelectedReason(reasons[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertCircle className="h-4 w-4 text-ink-400" />
          {selectedReason}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedReason !== "All" ? 1 : 0)}
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
            <Package className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No returns found</h3>
          <p className="mt-1 text-sm text-ink-500">No returns match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Source</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reason</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Items</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Condition</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Received;
                  const reasonStyle = REASON_STYLES[item.reason] || REASON_STYLES.Other;
                  const condStyle = CONDITION_STYLES[item.condition] || CONDITION_STYLES.Good;
                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToReturnDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.orderRef || "—"}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.source}</td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${reasonStyle.bg} ${reasonStyle.text}`}>
                          {item.reason}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-medium text-ink-950">{item.items}</td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${condStyle.bg} ${condStyle.text}`}>
                          {item.condition}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
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