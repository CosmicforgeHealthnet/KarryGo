// src/app/(dashboard)/fleet/parts/stock/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Search,
  Plus,
  Package,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Eye,
  Filter,
  ArrowRight,
  Calendar,
  User,
  Truck,
  FileText,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Award,
  Zap,
  Gauge,
  Box,
  AlertTriangle,
  ShoppingCart,
  MoreHorizontal,
  Edit3,
  Trash2,
  BarChart3,
  Circle,
  Send,
  Download,
  Printer,
  History,
  Activity,
} from "lucide-react";

type StockMovement = {
  id: string;
  partId: string;
  partName: string;
  sku: string;
  type: "Receive" | "Issue" | "Adjust" | "Return";
  quantity: number;
  unit: string;
  fromLocation: string;
  toLocation: string;
  reason: string;
  reference: string;
  performedBy: string;
  timestamp: string;
  status: "Completed" | "Pending" | "Reconciling";
};

type StockSummary = {
  partId: string;
  partName: string;
  sku: string;
  category: string;
  location: string;
  available: number;
  reserved: number;
  reorderLevel: number;
  unit: string;
  status: "In Stock" | "Low Stock" | "Out of Stock" | "On Order";
  lastMovement: string;
};

// Mock stock summary data
const STOCK_SUMMARY: StockSummary[] = [
  {
    partId: "P-001",
    partName: "Brake Pads - Front",
    sku: "BRK-110",
    category: "Brakes",
    location: "Ikeja",
    available: 24,
    reserved: 4,
    reorderLevel: 10,
    unit: "Set",
    status: "In Stock",
    lastMovement: "2 hrs ago",
  },
  {
    partId: "P-002",
    partName: "Engine Oil 10W-40",
    sku: "OIL-220",
    category: "Lubricants",
    location: "Lekki",
    available: 42,
    reserved: 0,
    reorderLevel: 15,
    unit: "Liter",
    status: "In Stock",
    lastMovement: "5 hrs ago",
  },
  {
    partId: "P-003",
    partName: "Air Filter",
    sku: "FIL-330",
    category: "Filters",
    location: "Ikeja",
    available: 8,
    reserved: 5,
    reorderLevel: 12,
    unit: "Unit",
    status: "Low Stock",
    lastMovement: "1 day ago",
  },
  {
    partId: "P-004",
    partName: "Tyre - 120/80 R17",
    sku: "TYR-440",
    category: "Tyres",
    location: "Apapa",
    available: 3,
    reserved: 1,
    reorderLevel: 8,
    unit: "Unit",
    status: "Low Stock",
    lastMovement: "2 days ago",
  },
  {
    partId: "P-005",
    partName: "Spark Plug",
    sku: "SPK-550",
    category: "Engine",
    location: "Yaba",
    available: 0,
    reserved: 0,
    reorderLevel: 20,
    unit: "Unit",
    status: "Out of Stock",
    lastMovement: "3 days ago",
  },
  {
    partId: "P-006",
    partName: "Coolant Fluid",
    sku: "COL-660",
    category: "Fluids",
    location: "Ikeja",
    available: 15,
    reserved: 3,
    reorderLevel: 10,
    unit: "Liter",
    status: "In Stock",
    lastMovement: "4 hrs ago",
  },
];

// Mock movement history
const MOVEMENT_HISTORY: StockMovement[] = [
  {
    id: "mov-001",
    partId: "P-001",
    partName: "Brake Pads - Front",
    sku: "BRK-110",
    type: "Issue",
    quantity: 2,
    unit: "Set",
    fromLocation: "Ikeja",
    toLocation: "Workshop",
    reason: "Work Order WO-1917",
    reference: "WO-1917",
    performedBy: "T. Adeyemi",
    timestamp: "2026-08-17T14:15:00Z",
    status: "Completed",
  },
  {
    id: "mov-002",
    partId: "P-003",
    partName: "Air Filter",
    sku: "FIL-330",
    type: "Receive",
    quantity: 10,
    unit: "Unit",
    fromLocation: "Supplier",
    toLocation: "Ikeja",
    reason: "Purchase Order PO-2026-0842",
    reference: "PO-2026-0842",
    performedBy: "F. Daniel",
    timestamp: "2026-08-17T11:30:00Z",
    status: "Completed",
  },
  {
    id: "mov-003",
    partId: "P-005",
    partName: "Spark Plug",
    sku: "SPK-550",
    type: "Adjust",
    quantity: -5,
    unit: "Unit",
    fromLocation: "Yaba",
    toLocation: "Yaba",
    reason: "Stock count correction",
    reference: "ADJ-2026-007",
    performedBy: "K. Ahmed",
    timestamp: "2026-08-16T09:45:00Z",
    status: "Reconciling",
  },
  {
    id: "mov-004",
    partId: "P-002",
    partName: "Engine Oil 10W-40",
    sku: "OIL-220",
    type: "Receive",
    quantity: 20,
    unit: "Liter",
    fromLocation: "Supplier",
    toLocation: "Lekki",
    reason: "Purchase Order PO-2026-0835",
    reference: "PO-2026-0835",
    performedBy: "M. Bello",
    timestamp: "2026-08-15T16:20:00Z",
    status: "Completed",
  },
  {
    id: "mov-005",
    partId: "P-004",
    partName: "Tyre - 120/80 R17",
    sku: "TYR-440",
    type: "Issue",
    quantity: 1,
    unit: "Unit",
    fromLocation: "Apapa",
    toLocation: "Workshop",
    reason: "Work Order WO-1914",
    reference: "WO-1914",
    performedBy: "A. Yusuf",
    timestamp: "2026-08-14T10:00:00Z",
    status: "Completed",
  },
  {
    id: "mov-006",
    partId: "P-006",
    partName: "Coolant Fluid",
    sku: "COL-660",
    type: "Return",
    quantity: 2,
    unit: "Liter",
    fromLocation: "Workshop",
    toLocation: "Ikeja",
    reason: "Unused - Return to stock",
    reference: "RET-2026-003",
    performedBy: "C. Uche",
    timestamp: "2026-08-13T13:10:00Z",
    status: "Completed",
  },
];

const MOVEMENT_TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Receive: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <ArrowRight className="h-3.5 w-3.5 rotate-180" />,
  },
  Issue: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <ArrowRight className="h-3.5 w-3.5" />,
  },
  Adjust: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <Edit3 className="h-3.5 w-3.5" />,
  },
  Return: {
    bg: "bg-blue-bg",
    text: "text-blue",
    icon: <ArrowRight className="h-3.5 w-3.5 rotate-180" />,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  "In Stock": { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "Low Stock": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "Out of Stock": { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  "On Order": { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const MOVEMENT_TYPES = ["All", "Receive", "Issue", "Adjust", "Return"];

export default function PartsStockPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [summary, setSummary] = useState<StockSummary[]>([]);
  const [movements, setMovements] = useState<StockMovement[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setSummary(STOCK_SUMMARY);
      setMovements(MOVEMENT_HISTORY);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredMovements = movements.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.partName.toLowerCase().includes(search.toLowerCase()) ||
      item.sku.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesBranch =
      selectedBranch === "All branches" ||
      item.fromLocation === selectedBranch ||
      item.toLocation === selectedBranch;

    return matchesSearch && matchesType && matchesBranch;
  });

  const totalAvailable = summary.reduce((acc, d) => acc + d.available, 0);
  const lowStockCount = summary.filter((d) => d.status === "Low Stock" || d.status === "Out of Stock").length;

  // Navigation handlers
  const goBack = () => {
    router.push("/fleet/parts");
  };

  const goToPartDetail = (partId: string) => {
    router.push(`/fleet/parts/${partId}`);
  };

  const goToAddMovement = () => {
    router.push("/fleet/parts/stock/add");
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading stock data...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedType !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedType("All");
  };

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <button
              onClick={goBack}
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 text-ink-600 transition hover:bg-white hover:text-ink-900"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                Parts Stock
              </h1>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                Stock levels, movements, and inventory adjustments
              </p>
            </div>
          </div>
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
            onClick={goToAddMovement}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            New Movement
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <Box className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total Available</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{totalAvailable}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5">
              <AlertTriangle className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Low / Out</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{lowStockCount}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Activity className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Movements</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{movements.length}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== STOCK SUMMARY TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="border-b border-line px-5 py-3.5">
          <div className="flex items-center gap-2">
            <Package className="h-4 w-4 text-ink-400" />
            <span className="text-[14px] font-semibold text-ink-950">Current Stock Levels</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {summary.length} items
            </span>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px] min-w-[700px]">
            <thead>
              <tr className="bg-surface-2/70">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Part</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">SKU</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Location</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Available</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reserved</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Last Movement</th>
              </tr>
            </thead>
            <tbody>
              {summary.map((item) => {
                const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES["In Stock"];
                const isLow = item.status === "Low Stock" || item.status === "Out of Stock";
                const stockPercentage = Math.min((item.available / item.reorderLevel) * 100, 100);

                return (
                  <tr
                    key={item.partId}
                    onClick={() => goToPartDetail(item.partId)}
                    className="cursor-pointer hover:bg-surface-2 transition-colors group"
                  >
                    <td className="border-b border-line px-4 py-3">
                      <div className="font-semibold text-ink-950">{item.partName}</div>
                      <div className="text-[11px] text-ink-500">{item.category}</div>
                    </td>
                    <td className="border-b border-line px-4 py-3 font-mono text-[12px] text-ink-500">{item.sku}</td>
                    <td className="border-b border-line px-4 py-3 text-ink-600">{item.location}</td>
                    <td className="border-b border-line px-4 py-3">
                      <div className="flex items-center gap-2">
                        <span className={`font-semibold ${isLow ? "text-amber" : "text-ink-950"}`}>
                          {item.available}
                        </span>
                        <span className="text-[11px] text-ink-400">{item.unit}</span>
                        <div className="w-12 h-1.5 rounded-full bg-surface-3 overflow-hidden">
                          <div
                            className={`h-full rounded-full ${
                              item.status === "Out of Stock" ? "bg-red" :
                              item.status === "Low Stock" ? "bg-amber" : "bg-green"
                            }`}
                            style={{ width: `${Math.min(stockPercentage, 100)}%` }}
                          />
                        </div>
                      </div>
                    </td>
                    <td className="border-b border-line px-4 py-3 text-ink-500">
                      {item.reserved > 0 ? item.reserved : "—"}
                    </td>
                    <td className="border-b border-line px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                        {item.status}
                      </span>
                    </td>
                    <td className="border-b border-line px-4 py-3 text-[11px] text-ink-500">{item.lastMovement}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== MOVEMENT HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="border-b border-line px-5 py-3.5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <History className="h-4 w-4 text-ink-400" />
              <span className="text-[14px] font-semibold text-ink-950">Movement History</span>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {movements.length} records
              </span>
            </div>
            <button
              onClick={goToAddMovement}
              className="flex items-center gap-1 text-[11px] font-medium text-primary hover:text-primary-hover transition-colors"
            >
              <Plus className="h-3.5 w-3.5" />
              Record Movement
            </button>
          </div>
        </div>

        {/* Filters for movement history */}
        <div className="flex flex-wrap items-center gap-2 px-5 py-3 border-b border-line">
          <div className="flex h-9 flex-1 min-w-[200px] items-center gap-2 rounded-lg border border-line bg-white/70 px-3 shadow-sm">
            <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search movements..."
              className="w-full bg-transparent text-[12px] text-ink-950 outline-none placeholder:text-ink-400"
            />
          </div>

          <button
            onClick={() => {
              const types = ["All", "Receive", "Issue", "Adjust", "Return"];
              const currentIndex = types.indexOf(selectedType);
              const nextIndex = (currentIndex + 1) % types.length;
              setSelectedType(types[nextIndex]);
            }}
            className="flex h-9 items-center gap-1.5 rounded-lg border border-line bg-white/70 px-3 text-[11px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Filter className="h-3.5 w-3.5 text-ink-400" />
            {selectedType}
            <ChevronDown className="h-3.5 w-3.5 text-ink-400" />
          </button>

          {hasActiveFilters && (
            <button
              onClick={clearFilters}
              className="flex h-9 items-center gap-1 rounded-lg border border-line bg-white/70 px-2.5 text-[11px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
            >
              <X className="h-3.5 w-3.5" />
              Clear
            </button>
          )}
        </div>

        {/* Movement table */}
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px] min-w-[800px]">
            <thead>
              <tr className="bg-surface-2/70">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Part</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Qty</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">From → To</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reason</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">By</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
              </tr>
            </thead>
            <tbody>
              {filteredMovements.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-ink-500">
                    No movements match your filters
                  </td>
                </tr>
              ) : (
                filteredMovements.map((movement) => {
                  const typeStyle = MOVEMENT_TYPE_STYLES[movement.type] || MOVEMENT_TYPE_STYLES.Receive;
                  const isReceive = movement.type === "Receive";
                  const isIssue = movement.type === "Issue";
                  const isAdjust = movement.type === "Adjust";
                  const isReturn = movement.type === "Return";

                  return (
                    <tr key={movement.id} className="hover:bg-surface-2 transition-colors group">
                      <td className="border-b border-line px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {movement.type}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <div className="font-medium text-ink-950">{movement.partName}</div>
                        <div className="font-mono text-[11px] text-ink-400">{movement.sku}</div>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`font-semibold ${
                          isReceive ? "text-green" :
                          isIssue ? "text-red" :
                          isAdjust ? "text-amber" :
                          isReturn ? "text-blue" : "text-ink-950"
                        }`}>
                          {movement.quantity > 0 ? "+" : ""}{movement.quantity} {movement.unit}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-[12px] text-ink-600">
                        {movement.fromLocation} → {movement.toLocation}
                      </td>
                      <td className="border-b border-line px-4 py-3 text-[12px] text-ink-600 max-w-[150px] truncate">
                        {movement.reason}
                      </td>
                      <td className="border-b border-line px-4 py-3 text-[12px] text-ink-600">
                        {movement.performedBy}
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                          movement.status === "Completed" ? "bg-green-bg text-green" :
                          movement.status === "Pending" ? "bg-amber-bg text-amber" :
                          "bg-blue-bg text-blue"
                        }`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${
                            movement.status === "Completed" ? "bg-green" :
                            movement.status === "Pending" ? "bg-amber" :
                            "bg-blue"
                          }`} />
                          {movement.status}
                        </span>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}