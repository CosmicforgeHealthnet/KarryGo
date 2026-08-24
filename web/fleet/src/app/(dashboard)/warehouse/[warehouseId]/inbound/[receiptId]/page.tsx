// src/app/(dashboard)/warehouse/[warehouseId]/inbound/[receiptId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Package,
  Clock,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Edit3,
  MoreHorizontal,
  Truck,
  User,
  MapPin,
  CalendarDays,
  Boxes,
  ArrowDownRight,
  Eye,
  Download,
  FileText,
  Printer,
  XCircle,
  Plus,
  ArrowRight,
} from "lucide-react";

type InboundReceiptDetail = {
  id: string;
  reference: string;
  source: string;
  orderRef: string | null;
  vehicle: string;
  carrier: string;
  driver: string;
  appointment: string;
  arrived: string | null;
  completed: string | null;
  status: "Scheduled" | "Arrived" | "Receiving" | "Completed" | "Discrepancy";
  warehouse: string;
  branch: string;
  items: {
    sku: string;
    name: string;
    expected: number;
    received: number;
    condition: "Good" | "Damaged" | "Missing";
    location: string;
  }[];
  notes: string | null;
  createdBy: string;
  timestamp: string;
};

// Mock data
const MOCK_RECEIPT: InboundReceiptDetail = {
  id: "IR-042",
  reference: "IR-042",
  source: "Greenline Foods",
  orderRef: "PO-1024",
  vehicle: "Truck TR-098",
  carrier: "Apex Logistics",
  driver: "Kabiru Ahmed",
  appointment: "2026-08-21 09:00",
  arrived: "2026-08-21 08:55",
  completed: null,
  status: "Receiving",
  warehouse: "Ikeja Central",
  branch: "Ikeja",
  items: [
    { sku: "SKU-2918", name: "Tyre 120/80 R17", expected: 40, received: 38, condition: "Good", location: "A-01-04" },
    { sku: "SKU-1440", name: "Brake Pads BR110", expected: 20, received: 18, condition: "Good", location: "A-02-11" },
    { sku: "SKU-3312", name: "Medical Grade Gloves", expected: 100, received: 95, condition: "Good", location: "B-01-09" },
    { sku: "SKU-5517", name: "Forklift Battery", expected: 4, received: 4, condition: "Good", location: "C-03-01" },
    { sku: "SKU-1009", name: "Truck Seal Set", expected: 60, received: 55, condition: "Damaged", location: "D-03-08" },
  ],
  notes: "Partial shipment. 5 damaged seal sets quarantined.",
  createdBy: "O. Adebayo",
  timestamp: "12 min ago",
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Scheduled: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-4 w-4" /> },
  Arrived: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <Truck className="h-4 w-4" /> },
  Receiving: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <ArrowDownRight className="h-4 w-4" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Discrepancy: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-4 w-4" /> },
};

const CONDITION_STYLES: Record<string, { bg: string; text: string }> = {
  Good: { bg: "bg-green-bg", text: "text-green" },
  Damaged: { bg: "bg-red-bg", text: "text-red" },
  Missing: { bg: "bg-amber-bg", text: "text-amber" },
};

export default function ReceiptDetailPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;
  const receiptId = params.receiptId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<InboundReceiptDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_RECEIPT, id: receiptId });
      setLoading(false);
    };
    loadData();
  }, [receiptId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading receipt details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Package className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Receipt not found</h3>
        <p className="mt-1 text-sm text-ink-500">The inbound receipt you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push(`/warehouse/${warehouseId}/inbound`)}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to inbound
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Scheduled;
  const totalExpected = data.items.reduce((acc, i) => acc + i.expected, 0);
  const totalReceived = data.items.reduce((acc, i) => acc + i.received, 0);
  const completionPercent = Math.round((totalReceived / totalExpected) * 100);

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/warehouse/${warehouseId}/inbound`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Receipt {data.reference}
              </h1>
              <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}>
                {statusStyle.icon}
                {data.status}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.source} · {data.orderRef || "No order reference"}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white">
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Edit3 className="h-3.5 w-3.5" />
            Update
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expected</div>
          <div className="text-xl font-bold text-ink-950">{totalExpected}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Received</div>
          <div className="text-xl font-bold text-green">{totalReceived}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Completion</div>
          <div className="text-xl font-bold text-ink-950">{completionPercent}%</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Vehicle</div>
          <div className="text-xl font-bold text-ink-950 font-mono text-[14px]">{data.vehicle}</div>
        </div>
      </div>

      {/* Details Grid */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* Left: Items */}
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Received Items</h3>
              <span className="text-xs text-ink-400">{data.items.length} items</span>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-sm">
                <thead>
                  <tr className="bg-surface-2/70">
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">SKU</th>
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Item</th>
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Expected</th>
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Received</th>
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Condition</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((item, index) => {
                    const condStyle = CONDITION_STYLES[item.condition] || CONDITION_STYLES.Good;
                    return (
                      <tr key={index} className="border-b border-line hover:bg-surface-2 transition-colors">
                        <td className="px-3 py-2 font-mono text-[12px] text-ink-600">{item.sku}</td>
                        <td className="px-3 py-2 text-ink-950">{item.name}</td>
                        <td className="px-3 py-2 text-ink-600">{item.expected}</td>
                        <td className="px-3 py-2 font-medium text-ink-950">{item.received}</td>
                        <td className="px-3 py-2">
                          <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${condStyle.bg} ${condStyle.text}`}>
                            {item.condition}
                          </span>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* Notes */}
          {data.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
              <p className="text-sm text-ink-600">{data.notes}</p>
            </div>
          )}
        </div>

        {/* Right: Details */}
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Receipt Details</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Source</div>
                <div className="text-sm font-medium text-ink-950">{data.source}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Order</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.orderRef || "—"}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Carrier</div>
                <div className="text-sm font-medium text-ink-950">{data.carrier}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Driver</div>
                <div className="text-sm font-medium text-ink-950">{data.driver}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Appointment</div>
                <div className="text-sm font-medium text-ink-950">{data.appointment}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Arrived</div>
                <div className="text-sm font-medium text-ink-950">{data.arrived || "—"}</div>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <CheckCircle2 className="h-4 w-4 text-green" />
                  Complete Receiving
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <Boxes className="h-4 w-4 text-amber" />
                  Put Away Items
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <AlertCircle className="h-4 w-4 text-red" />
                  Report Discrepancy
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