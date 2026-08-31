// src/app/(dashboard)/crm/customers/[customerId]/history/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Clock,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  FileText,
  FileCheck,
  Package,
  CreditCard,
  User,
  Download,
} from "lucide-react";

type HistoryEvent = {
  id: string;
  type: "Quotation" | "Contract" | "Order" | "Payment" | "Hold" | "Status" | "Risk" | "Note";
  action: string;
  description: string;
  timestamp: string;
  user: string;
  reference: string | null;
  amount: number | null;
  status: string | null;
  before: string | null;
  after: string | null;
};

// Mock data
const MOCK_HISTORY: HistoryEvent[] = [
  {
    id: "HIS-001",
    type: "Quotation",
    action: "Quotation Issued",
    description: "New quotation issued for medical supplies",
    timestamp: "2026-08-21 09:30",
    user: "F. Daniel",
    reference: "QT-2026-012",
    amount: 1200000,
    status: "Issued",
    before: null,
    after: null,
  },
  {
    id: "HIS-002",
    type: "Order",
    action: "Order Created",
    description: "New order created for medical supplies",
    timestamp: "2026-08-21 08:15",
    user: "A. Yusuf",
    reference: "OR-129",
    amount: 1280000,
    status: "Pending",
    before: null,
    after: null,
  },
  {
    id: "HIS-003",
    type: "Payment",
    action: "Payment Received",
    description: "Invoice payment received",
    timestamp: "2026-08-20 14:30",
    user: "System",
    reference: "INV-2026-001",
    amount: 1280000,
    status: "Completed",
    before: null,
    after: null,
  },
  {
    id: "HIS-004",
    type: "Risk",
    action: "Risk Status Changed",
    description: "Risk status updated from Medium to Low",
    timestamp: "2026-08-20 10:00",
    user: "F. Daniel",
    reference: null,
    amount: null,
    status: "Low",
    before: "Medium",
    after: "Low",
  },
  {
    id: "HIS-005",
    type: "Contract",
    action: "Contract Activated",
    description: "New service contract activated",
    timestamp: "2026-08-19 16:00",
    user: "O. Adebayo",
    reference: "CT-2026-003",
    amount: 5000000,
    status: "Active",
    before: null,
    after: null,
  },
  {
    id: "HIS-006",
    type: "Status",
    action: "Customer Status Changed",
    description: "Status updated from Follow-up to Active",
    timestamp: "2026-08-19 11:30",
    user: "F. Daniel",
    reference: null,
    amount: null,
    status: "Active",
    before: "Follow-up",
    after: "Active",
  },
  {
    id: "HIS-007",
    type: "Hold",
    action: "Hold Released",
    description: "Credit hold released",
    timestamp: "2026-08-18 09:00",
    user: "F. Daniel",
    reference: null,
    amount: null,
    status: "Released",
    before: "Active",
    after: "Inactive",
  },
  {
    id: "HIS-008",
    type: "Order",
    action: "Order Completed",
    description: "Order OR-125 completed successfully",
    timestamp: "2026-08-17 15:00",
    user: "System",
    reference: "OR-125",
    amount: 840000,
    status: "Completed",
    before: null,
    after: null,
  },
  {
    id: "HIS-009",
    type: "Quotation",
    action: "Quotation Accepted",
    description: "Quotation accepted by customer",
    timestamp: "2026-08-16 12:00",
    user: "Customer",
    reference: "QT-2026-008",
    amount: 950000,
    status: "Accepted",
    before: null,
    after: null,
  },
  {
    id: "HIS-010",
    type: "Note",
    action: "Note Added",
    description: "Customer feedback: Excellent service, will renew contract",
    timestamp: "2026-08-15 08:30",
    user: "F. Daniel",
    reference: null,
    amount: null,
    status: null,
    before: null,
    after: null,
  },
];

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Quotation: { bg: "bg-blue-bg", text: "text-blue", icon: <FileText className="h-3.5 w-3.5" /> },
  Contract: { bg: "bg-purple-bg", text: "text-purple", icon: <FileCheck className="h-3.5 w-3.5" /> },
  Order: { bg: "bg-amber-bg", text: "text-amber", icon: <Package className="h-3.5 w-3.5" /> },
  Payment: { bg: "bg-green-bg", text: "text-green", icon: <CreditCard className="h-3.5 w-3.5" /> },
  Hold: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Status: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Risk: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Note: { bg: "bg-surface-2", text: "text-ink-500", icon: <FileText className="h-3.5 w-3.5" /> },
};

const TYPE_FILTERS = ["All", "Quotation", "Contract", "Order", "Payment", "Hold", "Status", "Risk", "Note"];

export default function CustomerHistoryPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<HistoryEvent[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_HISTORY);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.action.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.reference?.toLowerCase().includes(search.toLowerCase()) ||
      item.user.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;

    return matchesSearch && matchesType;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading history...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedType !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
  };

  const goToEventDetail = (eventId: string) => {
    router.push(`/crm/customers/${customerId}/history/${eventId}`);
  };

  // Stats
  const totalEvents = data.length;
  const quotationCount = data.filter((d) => d.type === "Quotation").length;
  const orderCount = data.filter((d) => d.type === "Order").length;
  const paymentCount = data.filter((d) => d.type === "Payment").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Commercial History
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Chronological quotations, contract versions, holds, service commitments and major account events.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Events</div>
          <div className="text-lg font-bold text-ink-950">{totalEvents}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Quotations</div>
          <div className="text-lg font-bold text-blue">{quotationCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Orders</div>
          <div className="text-lg font-bold text-amber">{orderCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Payments</div>
          <div className="text-lg font-bold text-green">{paymentCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by action, description, or user..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Quotation", "Contract", "Order", "Payment", "Hold", "Status", "Risk", "Note"];
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

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
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

      {/* ===== HISTORY TIMELINE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Clock className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No history found</h3>
          <p className="mt-1 text-sm text-ink-500">No history events match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {filteredData.map((event, index) => {
            const typeStyle = TYPE_STYLES[event.type] || TYPE_STYLES.Note;
            const formattedAmount = event.amount ? new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(event.amount) : null;

            return (
              <div
                key={event.id}
                onClick={() => goToEventDetail(event.id)}
                className="group relative cursor-pointer rounded-xl border border-line bg-white p-4 transition hover:shadow-md hover:-translate-y-0.5"
              >
                <div className="flex items-start gap-4">
                  {/* Timeline dot */}
                  <div className="flex flex-col items-center">
                    <div className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full ${typeStyle.bg} ${typeStyle.text}`}>
                      {typeStyle.icon}
                    </div>
                    {index < filteredData.length - 1 && (
                      <div className="h-full w-0.5 bg-line mt-1" />
                    )}
                  </div>

                  {/* Content */}
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-semibold text-ink-950">{event.action}</span>
                          <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                            {event.type}
                          </span>
                          {event.status && (
                            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                              {event.status}
                            </span>
                          )}
                        </div>
                        <p className="text-sm text-ink-600 mt-0.5">{event.description}</p>
                      </div>
                      <div className="text-right flex-shrink-0">
                        {formattedAmount && (
                          <div className="text-sm font-semibold text-ink-950">{formattedAmount}</div>
                        )}
                        <div className="text-xs text-ink-400">{event.timestamp}</div>
                      </div>
                    </div>

                    {/* Reference and User */}
                    <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                      {event.reference && (
                        <span className="flex items-center gap-1">
                          <FileText className="h-3 w-3" />
                          <span className="font-mono">{event.reference}</span>
                        </span>
                      )}
                      <span className="flex items-center gap-1">
                        <User className="h-3 w-3" />
                        {event.user}
                      </span>
                      {event.before && event.after && (
                        <span className="flex items-center gap-1 bg-surface-2 px-2 py-0.5 rounded-md">
                          <span className="text-ink-400">{event.before}</span>
                          <ArrowRight className="h-3 w-3 text-ink-400" />
                          <span className="text-ink-950">{event.after}</span>
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Clock className="h-4 w-4" />
          CRM-16 · Customer Commercial History
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} events shown
        </span>
      </div>
    </div>
  );
}