// src/app/(dashboard)/warehouse/[warehouseId]/transfers/reconciliation/page.tsx

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
  FileText,
} from "lucide-react";

type TransferReconciliation = {
  id: string;
  transferRef: string;
  origin: string;
  destination: string;
  expected: number;
  arrived: number;
  discrepancy: number;
  status: "Pending" | "Investigating" | "Resolved" | "Closed";
  notes: string | null;
  timestamp: string;
  resolvedBy: string | null;
};

// Mock data
const RECONCILIATIONS: TransferReconciliation[] = [
  { id: "REC-001", transferRef: "TF-031", origin: "Ikeja Central", destination: "Lekki Distribution", expected: 180, arrived: 170, discrepancy: -10, status: "Investigating", notes: "10 units of SKU-2918 missing. Checking loading records.", timestamp: "2 hours ago", resolvedBy: null },
  { id: "REC-002", transferRef: "TF-027", origin: "Ikeja Central", destination: "Lekki Distribution", expected: 89, arrived: 89, discrepancy: 0, status: "Closed", notes: "All items accounted for.", timestamp: "1 day ago", resolvedBy: "O. Adebayo" },
  { id: "REC-003", transferRef: "TF-030", origin: "Apapa Port Storage", destination: "Ikeja Central", expected: 220, arrived: 218, discrepancy: -2, status: "Pending", notes: "2 damaged items. Awaiting inspection report.", timestamp: "3 hours ago", resolvedBy: null },
  { id: "REC-004", transferRef: "TF-029", origin: "Lekki Distribution", destination: "Ikeja Cold Storage", expected: 45, arrived: 48, discrepancy: 3, status: "Resolved", notes: "3 extra items found. Documented as overage.", timestamp: "5 hours ago", resolvedBy: "A. Yusuf" },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Pending", "Investigating", "Resolved", "Closed"];

export default function TransferReconciliationPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<TransferReconciliation[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(RECONCILIATIONS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.transferRef.toLowerCase().includes(search.toLowerCase()) ||
      item.origin.toLowerCase().includes(search.toLowerCase()) ||
      item.destination.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading reconciliation cases...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const goToCaseDetail = (caseId: string) => {
    router.push(`/warehouse/${warehouseId}/transfers/reconciliation/${caseId}`);
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Transfer Reconciliation
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Resolve discrepancies between expected and arrived transfer items.
          </p>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by transfer reference..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Investigating", "Resolved", "Closed"];
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

      {/* Cards */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileText className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No reconciliation cases found</h3>
          <p className="mt-1 text-sm text-ink-500">No cases match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {filteredData.map((item) => {
            const style = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
            const isDiscrepancy = item.discrepancy !== 0;
            return (
              <div
                key={item.id}
                onClick={() => goToCaseDetail(item.id)}
                className="group cursor-pointer rounded-xl border border-line bg-white p-4 transition hover:shadow-md hover:-translate-y-0.5"
              >
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <div className="flex items-center gap-3">
                      <span className="text-[15px] font-semibold text-ink-950">{item.transferRef}</span>
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                        {style.icon}
                        {item.status}
                      </span>
                    </div>
                    <div className="mt-1 text-sm text-ink-500">
                      {item.origin} → {item.destination}
                    </div>
                    <div className="mt-1 flex items-center gap-4 text-sm">
                      <span className="text-ink-600">Expected: <span className="font-medium">{item.expected}</span></span>
                      <span className="text-ink-600">Arrived: <span className="font-medium">{item.arrived}</span></span>
                      <span className={`font-medium ${isDiscrepancy ? "text-red" : "text-green"}`}>
                        {item.discrepancy > 0 ? `+${item.discrepancy}` : item.discrepancy}
                      </span>
                    </div>
                    {item.notes && (
                      <div className="mt-1 text-sm text-ink-400">{item.notes}</div>
                    )}
                  </div>
                  <div className="flex items-center gap-2 text-sm text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    {item.timestamp}
                    <ArrowRight className="h-4 w-4 text-ink-400 group-hover:text-primary transition-colors ml-2" />
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