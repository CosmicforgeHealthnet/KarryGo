// src/app/(dashboard)/warehouse/[warehouseId]/inventory/[itemId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Package,
  Clock,
  MapPin,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Edit3,
  MoreHorizontal,
  CalendarDays,
  Boxes,
  Tag,
  Hash,
  User,
  Building2,
  TrendingUp,
  TrendingDown,
  Eye,
  Download,
  FileText,
  Printer,
  XCircle,
  Layers,
  Truck,
  ArrowRight,
} from "lucide-react";

type InventoryItemDetail = {
  id: string;
  name: string;
  sku: string;
  description: string;
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
  createdAt: string;
  createdBy: string;
  minStock: number;
  maxStock: number;
  reorderPoint: number;
  stockMovements: {
    id: string;
    type: "In" | "Out" | "Adjustment" | "Transfer";
    quantity: number;
    from: string | null;
    to: string | null;
    reference: string;
    timestamp: string;
    user: string;
  }[];
};

// Mock data
const MOCK_ITEM: InventoryItemDetail = {
  id: "INV-001",
  name: "Tyre 120/80 R17",
  sku: "SKU-2918",
  description: "Heavy-duty motorcycle tyre for dispatch bikes. Tubeless, all-weather tread pattern.",
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
  createdAt: "2026-01-15",
  createdBy: "F. Daniel",
  minStock: 10,
  maxStock: 50,
  reorderPoint: 15,
  stockMovements: [
    { id: "MOV-001", type: "In", quantity: 20, from: null, to: "A-01-04", reference: "IR-042", timestamp: "12 min ago", user: "O. Adebayo" },
    { id: "MOV-002", type: "Out", quantity: 4, from: "A-01-04", to: null, reference: "OR-129", timestamp: "28 min ago", user: "M. Bello" },
    { id: "MOV-003", type: "In", quantity: 8, from: null, to: "A-01-04", reference: "IR-038", timestamp: "2 hours ago", user: "O. Adebayo" },
    { id: "MOV-004", type: "Adjustment", quantity: -2, from: "A-01-04", to: null, reference: "AD-003", timestamp: "1 day ago", user: "F. Daniel" },
  ],
};

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
    icon: <XCircle className="h-3.5 w-3.5" />,
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

const MOVEMENT_TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  In: { bg: "bg-green-bg", text: "text-green", icon: <ArrowRight className="h-3.5 w-3.5 rotate-180" /> },
  Out: { bg: "bg-blue-bg", text: "text-blue", icon: <ArrowRight className="h-3.5 w-3.5" /> },
  Adjustment: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Transfer: { bg: "bg-purple-bg", text: "text-purple", icon: <Truck className="h-3.5 w-3.5" /> },
};

export default function InventoryItemDetailPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;
  const itemId = params.itemId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<InventoryItemDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_ITEM, id: itemId });
      setLoading(false);
    };
    loadData();
  }, [itemId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading item details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Package className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Item not found</h3>
        <p className="mt-1 text-sm text-ink-500">The inventory item you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push(`/warehouse/${warehouseId}/inventory`)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to inventory
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Ready;

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/warehouse/${warehouseId}/inventory`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {data.name}
              </h1>
              <span className="font-mono text-xs text-ink-400 sm:text-sm">{data.sku}</span>
              <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}>
                {statusStyle.icon}
                {data.status}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.id} · {data.warehouse} · {data.location}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white">
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Physical</div>
          <div className="text-xl font-bold text-ink-950">{data.physical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reserved</div>
          <div className="text-xl font-bold text-ink-950">{data.reserved}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Available</div>
          <div className="text-xl font-bold text-green">{data.available}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Unit</div>
          <div className="text-xl font-bold text-ink-950">{data.unit}</div>
        </div>
      </div>

      {/* Details Grid */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* Left: Details */}
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Item Details</h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">SKU</div>
                <div className="font-mono text-sm font-medium text-ink-950">{data.sku}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Owner</div>
                <div className="text-sm font-medium text-ink-950">{data.owner}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Location</div>
                <div className="font-mono text-sm font-medium text-ink-950">{data.location}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Warehouse</div>
                <div className="text-sm font-medium text-ink-950">{data.warehouse}</div>
              </div>
              {data.lot && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Lot</div>
                  <div className="font-mono text-sm font-medium text-ink-950">{data.lot}</div>
                </div>
              )}
              {data.batch && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Batch</div>
                  <div className="font-mono text-sm font-medium text-ink-950">{data.batch}</div>
                </div>
              )}
              {data.serial && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Serial</div>
                  <div className="font-mono text-sm font-medium text-ink-950">{data.serial}</div>
                </div>
              )}
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Customer</div>
                <div className="text-sm font-medium text-ink-950">{data.customer || "—"}</div>
              </div>
              {data.expiryDate && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expiry</div>
                  <div className="text-sm font-medium text-red">{data.expiryDate}</div>
                </div>
              )}
            </div>
          </div>

          {/* Stock Movement History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Movement History</h3>
              <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors">
                View all
              </button>
            </div>
            <div className="space-y-2">
              {data.stockMovements.map((movement) => {
                const style = MOVEMENT_TYPE_STYLES[movement.type] || MOVEMENT_TYPE_STYLES.In;
                return (
                  <div key={movement.id} className="flex items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2">
                    <div className="flex items-center gap-3">
                      <span className={style.text}>{style.icon}</span>
                      <div>
                        <div className="text-[13px] font-medium text-ink-950">
                          {movement.type} · {movement.reference}
                        </div>
                        <div className="text-[11px] text-ink-500">
                          {movement.quantity > 0 ? "+" : ""}{movement.quantity} {MOCK_ITEM.unit} · {movement.user}
                        </div>
                      </div>
                    </div>
                    <div className="text-right">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                        {movement.type}
                      </span>
                      <div className="text-[11px] text-ink-400">{movement.timestamp}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Right: Quick Actions */}
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Stock Levels</h3>
            <div className="space-y-3">
              <div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-500">Min Stock</span>
                  <span className="font-medium text-ink-950">{data.minStock}</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-500">Max Stock</span>
                  <span className="font-medium text-ink-950">{data.maxStock}</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-500">Reorder Point</span>
                  <span className="font-medium text-amber">{data.reorderPoint}</span>
                </div>
              </div>
              <div className="mt-2 h-2 w-full rounded-full bg-surface-3 overflow-hidden">
                <div
                  className="h-full rounded-full bg-primary"
                  style={{ width: `${(data.physical / data.maxStock) * 100}%` }}
                />
              </div>
              <div className="text-center text-[11px] text-ink-400">
                {data.physical} / {data.maxStock} units
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <ArrowRight className="h-4 w-4 text-green rotate-180" />
                  Receive Stock
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <ArrowRight className="h-4 w-4 text-blue" />
                  Issue Stock
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <AlertCircle className="h-4 w-4 text-amber" />
                  Adjust Stock
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}