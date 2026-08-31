// src/app/(dashboard)/fleet/parts/[partId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Package,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Edit3,
  RefreshCw,
  User,
  Calendar,
  Activity,
  Shield,
  FileText,
  Briefcase,
  Users,
  ArrowRight,
  MoreVertical,
  Gauge,
  Fuel,
  Wrench,
  Zap,
  Award,
  TrendingUp,
  TrendingDown,
  Circle,
  Radar,
  Sparkles,
  Battery,
  Car,
  Bike,
  Navigation,
  Copy,
  Download,
  Printer,
  Clipboard,
  History,
  Box,
  AlertTriangle,
  ShoppingCart,
  Trash2,
  Plus,
  Send,
  BarChart3,
  Truck,
} from "lucide-react";

type PartDetail = {
  id: string;
  name: string;
  sku: string;
  category: string;
  location: string;
  available: number;
  reserved: number;
  reorderLevel: number;
  minStock: number;
  maxStock: number;
  status: "In Stock" | "Low Stock" | "Out of Stock" | "On Order";
  unit: string;
  supplier: string;
  cost: string;
  lastUpdated: string;
  description: string;
  createdAt: string;
  movements: {
    id: string;
    type: "Receive" | "Issue" | "Adjust" | "Return";
    quantity: number;
    fromLocation: string;
    toLocation: string;
    reason: string;
    performedBy: string;
    timestamp: string;
    status: "Completed" | "Pending" | "Reconciling";
  }[];
};

// Mock data
const PART_DETAIL: PartDetail = {
  id: "P-001",
  name: "Brake Pads - Front",
  sku: "BRK-110",
  category: "Brakes",
  location: "Ikeja",
  available: 24,
  reserved: 4,
  reorderLevel: 10,
  minStock: 5,
  maxStock: 50,
  status: "In Stock",
  unit: "Set",
  supplier: "AutoParts Ltd",
  cost: "₦12,500",
  lastUpdated: "2 hrs ago",
  description: "High-quality ceramic brake pads for Honda ACE 110. Provides excellent stopping power and reduced brake dust.",
  createdAt: "2026-01-15",
  movements: [
    {
      id: "mov-001",
      type: "Receive",
      quantity: 20,
      fromLocation: "Supplier",
      toLocation: "Ikeja",
      reason: "Purchase Order PO-2026-0800",
      performedBy: "F. Daniel",
      timestamp: "2026-08-15T10:30:00Z",
      status: "Completed",
    },
    {
      id: "mov-002",
      type: "Issue",
      quantity: 2,
      fromLocation: "Ikeja",
      toLocation: "Workshop",
      reason: "Work Order WO-1917",
      performedBy: "T. Adeyemi",
      timestamp: "2026-08-17T14:15:00Z",
      status: "Completed",
    },
    {
      id: "mov-003",
      type: "Issue",
      quantity: 1,
      fromLocation: "Ikeja",
      toLocation: "Workshop",
      reason: "Work Order WO-1910",
      performedBy: "M. Bello",
      timestamp: "2026-08-16T09:00:00Z",
      status: "Completed",
    },
    {
      id: "mov-004",
      type: "Receive",
      quantity: 10,
      fromLocation: "Supplier",
      toLocation: "Ikeja",
      reason: "Purchase Order PO-2026-0785",
      performedBy: "K. Ahmed",
      timestamp: "2026-08-12T14:20:00Z",
      status: "Completed",
    },
    {
      id: "mov-005",
      type: "Adjust",
      quantity: -2,
      fromLocation: "Ikeja",
      toLocation: "Ikeja",
      reason: "Stock count correction - damaged items",
      performedBy: "A. Yusuf",
      timestamp: "2026-08-10T11:00:00Z",
      status: "Reconciling",
    },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  "In Stock": {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  "Low Stock": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  "Out of Stock": {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
  "On Order": {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <ShoppingCart className="h-4 w-4" />,
  },
};

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

export default function PartDetailPage() {
  const params = useParams();
  const router = useRouter();
  const partId = params.partId as string;

  const [loading, setLoading] = useState(true);
  const [part, setPart] = useState<PartDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setPart(PART_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [partId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  // Navigation handlers
  const goBack = () => {
    router.push("/fleet/parts");
  };

  const goToStock = () => {
    router.push("/fleet/parts/stock");
  };

  const goToEdit = () => {
    router.push(`/fleet/parts/${partId}/edit`);
  };

  const goToAddMovement = () => {
    router.push(`/fleet/parts/${partId}/movement/add`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading part details...</span>
        </div>
      </div>
    );
  }

  if (!part) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Package className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Part not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The part you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to parts
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[part.status] || STATUS_STYLES["In Stock"];
  const stockPercentage = Math.min((part.available / part.reorderLevel) * 100, 100);
  const isLow = part.status === "Low Stock" || part.status === "Out of Stock";

  return (
    <div className="mx-auto max-w-5xl space-y-5">
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
              <div className="flex items-center gap-2.5">
                <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                  {part.name}
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {statusStyle.icon}
                  {part.status}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {part.sku} · {part.category} · {part.location}
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
            onClick={goToEdit}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          <button
            onClick={goToAddMovement}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Movement
          </button>
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-green-bg p-1.5">
              <Box className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Available</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {part.available} {part.unit}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-amber-bg p-1.5">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Reserved</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {part.reserved} {part.unit}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-blue-bg p-1.5">
              <AlertTriangle className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Reorder Level</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {part.reorderLevel} {part.unit}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Calendar className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Last Updated</div>
              <div className="text-[16px] font-semibold text-ink-950">{part.lastUpdated}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* LEFT COLUMN */}
        <div className="space-y-5">
          {/* Stock Level */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Stock Level</h3>
            <div className="space-y-3">
              <div className="flex items-center justify-between text-[13px]">
                <span className="text-ink-500">Current stock</span>
                <span className={`font-semibold ${isLow ? "text-amber" : "text-ink-950"}`}>
                  {part.available} / {part.reorderLevel} {part.unit}
                </span>
              </div>
              <div className="h-3 w-full rounded-full bg-surface-3 overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all ${
                    part.status === "Out of Stock"
                      ? "bg-red"
                      : part.status === "Low Stock"
                      ? "bg-amber"
                      : "bg-green"
                  }`}
                  style={{ width: `${Math.min(stockPercentage, 100)}%` }}
                />
              </div>
              <div className="flex justify-between text-[11px] text-ink-400">
                <span>Min: {part.minStock} {part.unit}</span>
                <span>Max: {part.maxStock} {part.unit}</span>
              </div>
            </div>
          </div>

          {/* Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Part Details</h3>
            </div>
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">SKU</div>
                  <div className="mt-0.5 font-mono text-[13px] text-ink-950">{part.sku}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Category</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{part.category}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Supplier</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{part.supplier}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Cost</div>
                  <div className="mt-0.5 text-[13px] font-semibold text-ink-950">{part.cost}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Location</div>
                  <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                    <MapPin className="h-4 w-4 text-ink-400" />
                    {part.location}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Created</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{part.createdAt}</div>
                </div>
              </div>
            </div>
          </div>

          {/* Description */}
          {part.description && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <p className="text-[13px] text-ink-700">{part.description}</p>
            </div>
          )}
        </div>

        {/* RIGHT COLUMN */}
        <div className="space-y-5">
          {/* Quick Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button
                onClick={goToAddMovement}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Plus className="h-4 w-4" />
                  Record Movement
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={goToStock}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <BarChart3 className="h-4 w-4" />
                  View Stock Summary
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={goToEdit}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="h-4 w-4" />
                  Edit Part
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== MOVEMENT HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2.5">
            <History className="h-5 w-5 text-ink-400" />
            <h3 className="text-[15px] font-semibold text-ink-950">Movement History</h3>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {part.movements.length} records
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

        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px] min-w-[700px]">
            <thead>
              <tr className="bg-surface-2/70">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Qty</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">From → To</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reason</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">By</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
              </tr>
            </thead>
            <tbody>
              {part.movements.map((movement) => {
                const typeStyle = MOVEMENT_TYPE_STYLES[movement.type] || MOVEMENT_TYPE_STYLES.Receive;

                return (
                  <tr key={movement.id} className="hover:bg-surface-2 transition-colors">
                    <td className="border-b border-line px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                        {typeStyle.icon}
                        {movement.type}
                      </span>
                    </td>
                    <td className="border-b border-line px-4 py-3">
                      <span className={`font-semibold ${
                        movement.type === "Receive" ? "text-green" :
                        movement.type === "Issue" ? "text-red" :
                        movement.type === "Adjust" ? "text-amber" :
                        "text-blue"
                      }`}>
                        {movement.quantity > 0 ? "+" : ""}{movement.quantity}
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
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== NAVIGATION FOOTER ===== */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-[12px] font-medium text-ink-500 hover:text-ink-900 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to parts
        </button>
        <button
          onClick={goToStock}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          View Stock Summary
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}