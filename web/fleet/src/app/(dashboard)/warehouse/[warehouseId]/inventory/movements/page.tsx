// src/app/(dashboard)/warehouse/[warehouseId]/inventory/movements/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
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
  TrendingUp,
  ArrowDownRight,
  ArrowUpRight,
  Truck,
  MoreHorizontal,
} from "lucide-react";

type StockMovement = {
  id: string;
  type: "In" | "Out" | "Adjustment" | "Transfer";
  itemName: string;
  sku: string;
  quantity: number;
  from: string | null;
  to: string | null;
  reference: string;
  timestamp: string;
  user: string;
  status: "Completed" | "Pending" | "Cancelled";
};

// Mock data
const MOVEMENTS: StockMovement[] = [
  { id: "MOV-001", type: "In", itemName: "Tyre 120/80 R17", sku: "SKU-2918", quantity: 20, from: null, to: "A-01-04", reference: "IR-042", timestamp: "12 min ago", user: "O. Adebayo", status: "Completed" },
  { id: "MOV-002", type: "Out", itemName: "Brake Pads BR110", sku: "SKU-1440", quantity: 4, from: "A-02-11", to: null, reference: "OR-129", timestamp: "28 min ago", user: "M. Bello", status: "Completed" },
  { id: "MOV-003", type: "In", itemName: "Engine Oil 10W40", sku: "SKU-2207", quantity: 8, from: null, to: "C-02-02", reference: "IR-038", timestamp: "2 hours ago", user: "O. Adebayo", status: "Completed" },
  { id: "MOV-004", type: "Adjustment", itemName: "Safety Vests", sku: "LOT-7721", quantity: -2, from: "Q-01-01", to: null, reference: "AD-003", timestamp: "1 day ago", user: "F. Daniel", status: "Pending" },
  { id: "MOV-005", type: "Transfer", itemName: "Truck Seal Set", sku: "SKU-1009", quantity: 10, from: "D-03-08", to: "E-01-12", reference: "TF-027", timestamp: "2 days ago", user: "K. Ahmed", status: "Completed" },
  { id: "MOV-006", type: "Out", itemName: "Medical Grade Gloves", sku: "SKU-3312", quantity: 50, from: "B-01-09", to: null, reference: "OR-112", timestamp: "3 days ago", user: "A. Yusuf", status: "Completed" },
];

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  In: { bg: "bg-green-bg", text: "text-green", icon: <ArrowDownRight className="h-3.5 w-3.5" /> },
  Out: { bg: "bg-blue-bg", text: "text-blue", icon: <ArrowUpRight className="h-3.5 w-3.5" /> },
  Adjustment: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Transfer: { bg: "bg-purple-bg", text: "text-purple", icon: <Truck className="h-3.5 w-3.5" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Cancelled: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const TYPE_FILTERS = ["All", "In", "Out", "Adjustment", "Transfer"];

export default function StockMovementsPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<StockMovement[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOVEMENTS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.itemName.toLowerCase().includes(search.toLowerCase()) ||
      item.sku.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;

    return matchesSearch && matchesType;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading stock movements...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedType !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Stock Movements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Receive, issue, adjust, and transfer stock movements for this warehouse.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <ArrowRight className="h-3.5 w-3.5" />
          New Movement
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by item, SKU, or reference..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "In", "Out", "Adjustment", "Transfer"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedType}
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
              {selectedType !== "All" ? 1 : 0}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</div>
              <div className="flex gap-1.5">
                {TYPE_FILTERS.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedType(type)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedType === type
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {type}
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
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No movements found</h3>
          <p className="mt-1 text-sm text-ink-500">No stock movements match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[700px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Item</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Qty</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">From / To</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.In;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Completed;
                  return (
                    <tr key={item.id} className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer">
                      <td className="px-4 py-3">
                        <div>
                          <div className="font-medium text-ink-950">{item.itemName}</div>
                          <div className="text-[11px] text-ink-500 font-mono">{item.sku}</div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-medium text-ink-950">
                        {item.quantity > 0 ? "+" : ""}{item.quantity}
                      </td>
                      <td className="px-4 py-3 text-ink-600 font-mono text-[11px]">
                        {item.from || "—"} → {item.to || "—"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 font-mono text-[11px]">{item.reference}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
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