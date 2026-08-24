// src/app/(dashboard)/warehouse/[warehouseId]/inbound/page.tsx

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
  Truck,
  CalendarDays,
  MoreHorizontal,
  Plus,
} from "lucide-react";

type InboundReceipt = {
  id: string;
  reference: string;
  source: string;
  orderRef: string | null;
  vehicle: string;
  carrier: string;
  appointment: string;
  status: "Scheduled" | "Arrived" | "Receiving" | "Completed" | "Discrepancy";
  items: number;
  received: number;
  warehouse: string;
  branch: string;
  timestamp: string;
};

// Mock data
const RECEIPTS: InboundReceipt[] = [
  { id: "IR-042", reference: "IR-042", source: "Greenline Foods", orderRef: "PO-1024", vehicle: "Truck TR-098", carrier: "Apex Logistics", appointment: "2026-08-21 09:00", status: "Receiving", items: 284, received: 180, warehouse: "Ikeja Central", branch: "Ikeja", timestamp: "12 min ago" },
  { id: "IR-041", reference: "IR-041", source: "Charitos Hospital", orderRef: "PO-1019", vehicle: "Van VN-041", carrier: "Apex Logistics", appointment: "2026-08-21 08:30", status: "Arrived", items: 156, received: 0, warehouse: "Ikeja Central", branch: "Ikeja", timestamp: "45 min ago" },
  { id: "IR-040", reference: "IR-040", source: "Oakfield Stores", orderRef: "PO-1015", vehicle: "Truck TR-112", carrier: "Apex Logistics", appointment: "2026-08-20 14:00", status: "Completed", items: 320, received: 320, warehouse: "Ikeja Central", branch: "Ikeja", timestamp: "2 hours ago" },
  { id: "IR-039", reference: "IR-039", source: "Solaris Construction", orderRef: "PO-1012", vehicle: "Van VN-029", carrier: "Greenline Logistics", appointment: "2026-08-20 10:30", status: "Discrepancy", items: 120, received: 110, warehouse: "Lekki Distribution", branch: "Lekki", timestamp: "4 hours ago" },
  { id: "IR-038", reference: "IR-038", source: "BlueMarket Retail", orderRef: "PO-1008", vehicle: "Truck TR-098", carrier: "Apex Logistics", appointment: "2026-08-20 09:00", status: "Completed", items: 240, received: 240, warehouse: "Ikeja Central", branch: "Ikeja", timestamp: "1 day ago" },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Scheduled: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Arrived: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <Truck className="h-3.5 w-3.5" /> },
  Receiving: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <ArrowDownRight className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Discrepancy: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Scheduled", "Arrived", "Receiving", "Completed", "Discrepancy"];

export default function InboundReceiptsPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<InboundReceipt[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(RECEIPTS);
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

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading inbound receipts...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const goToReceiptDetail = (receiptId: string) => {
    router.push(`/warehouse/${warehouseId}/inbound/${receiptId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Inbound Receipts
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Manage incoming shipments, receiving, and discrepancy resolution.
          </p>
        </div>
        <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />
          Create Receipt
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
            const statuses = ["All", "Scheduled", "Arrived", "Receiving", "Completed", "Discrepancy"];
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
            <Package className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No receipts found</h3>
          <p className="mt-1 text-sm text-ink-500">No inbound receipts match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Vehicle</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Items</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Appointment</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const style = STATUS_STYLES[item.status] || STATUS_STYLES.Scheduled;
                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToReceiptDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.orderRef || "—"}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.source}</td>
                      <td className="px-4 py-3 text-ink-600 font-mono text-[12px]">{item.vehicle}</td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950 font-medium">{item.received}/{item.items}</div>
                        <div className="text-[10px] text-ink-400">
                          {Math.round((item.received / item.items) * 100)}%
                        </div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-[12px]">{item.appointment}</td>
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