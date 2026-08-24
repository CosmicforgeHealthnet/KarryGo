// src/app/(dashboard)/finance/invoices/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Receipt,
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
  TrendingDown,
  Users,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
  Plus,
  DollarSign,
  Building2,
  User,
  Mail,
  Phone,
} from "lucide-react";

type Invoice = {
  id: string;
  reference: string;
  customer: string;
  customerType: "Corporate" | "SME" | "Individual";
  amount: number;
  currency: string;
  status: "Paid" | "Pending" | "Overdue" | "Draft" | "Cancelled";
  issueDate: string;
  dueDate: string;
  paidDate: string | null;
  items: number;
  orderRef: string | null;
  branch: string;
};

// Mock data
const INVOICES: Invoice[] = [
  {
    id: "INV-001",
    reference: "INV-2026-001",
    customer: "Charitos Hospital",
    customerType: "Corporate",
    amount: 1280000,
    currency: "NGN",
    status: "Paid",
    issueDate: "2026-08-01",
    dueDate: "2026-08-15",
    paidDate: "2026-08-14",
    items: 12,
    orderRef: "OR-129",
    branch: "Ikeja",
  },
  {
    id: "INV-002",
    reference: "INV-2026-002",
    customer: "Oakfield Stores",
    customerType: "SME",
    amount: 840000,
    currency: "NGN",
    status: "Pending",
    issueDate: "2026-08-10",
    dueDate: "2026-08-30",
    paidDate: null,
    items: 8,
    orderRef: "OR-128",
    branch: "Ikeja",
  },
  {
    id: "INV-003",
    reference: "INV-2026-003",
    customer: "Greenline Foods",
    customerType: "Corporate",
    amount: 2100000,
    currency: "NGN",
    status: "Overdue",
    issueDate: "2026-07-15",
    dueDate: "2026-07-31",
    paidDate: null,
    items: 24,
    orderRef: "OR-126",
    branch: "Ikeja",
  },
  {
    id: "INV-004",
    reference: "INV-2026-004",
    customer: "Solaris Construction",
    customerType: "Corporate",
    amount: 560000,
    currency: "NGN",
    status: "Draft",
    issueDate: "2026-08-20",
    dueDate: "2026-09-15",
    paidDate: null,
    items: 6,
    orderRef: "OR-127",
    branch: "Lekki",
  },
  {
    id: "INV-005",
    reference: "INV-2026-005",
    customer: "BlueMarket Retail",
    customerType: "SME",
    amount: 320000,
    currency: "NGN",
    status: "Paid",
    issueDate: "2026-07-20",
    dueDate: "2026-08-05",
    paidDate: "2026-08-04",
    items: 4,
    orderRef: "OR-125",
    branch: "Ikeja",
  },
  {
    id: "INV-006",
    reference: "INV-2026-006",
    customer: "Apex Logistics Internal",
    customerType: "Corporate",
    amount: 450000,
    currency: "NGN",
    status: "Cancelled",
    issueDate: "2026-08-18",
    dueDate: "2026-09-05",
    paidDate: null,
    items: 3,
    orderRef: null,
    branch: "Ikeja",
  },
  {
    id: "INV-007",
    reference: "INV-2026-007",
    customer: "Oakfield Stores",
    customerType: "SME",
    amount: 180000,
    currency: "NGN",
    status: "Pending",
    issueDate: "2026-08-22",
    dueDate: "2026-09-10",
    paidDate: null,
    items: 2,
    orderRef: "OR-130",
    branch: "Yaba",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Paid: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Overdue: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Cancelled: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <X className="h-3.5 w-3.5" />,
  },
};

const CUSTOMER_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Corporate: { bg: "bg-blue-bg", text: "text-blue" },
  SME: { bg: "bg-amber-bg", text: "text-amber" },
  Individual: { bg: "bg-green-bg", text: "text-green" },
};

const STATUS_FILTERS = ["All", "Paid", "Pending", "Overdue", "Draft", "Cancelled"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function InvoicesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Invoice[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(INVOICES);
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
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.customer.toLowerCase().includes(search.toLowerCase()) ||
      item.orderRef?.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading invoices...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const goToInvoiceDetail = (invoiceId: string) => {
    router.push(`/finance/invoices/${invoiceId}`);
  };

  const goToCreateInvoice = () => {
    router.push("/finance/invoices/create");
  };

  // Stats
  const totalAmount = data.reduce((acc, item) => acc + item.amount, 0);
  const paidAmount = data.filter((item) => item.status === "Paid").reduce((acc, item) => acc + item.amount, 0);
  const pendingAmount = data.filter((item) => item.status === "Pending" || item.status === "Overdue").reduce((acc, item) => acc + item.amount, 0);
  const paidCount = data.filter((item) => item.status === "Paid").length;
  const overdueCount = data.filter((item) => item.status === "Overdue").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Invoices
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Customer invoices, amounts, issue/due dates, states, and outstanding projections.
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
          <button
            onClick={goToCreateInvoice}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Invoice
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Invoices</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Amount</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Paid</div>
          <div className="text-lg font-bold text-green">{paidCount}</div>
          <div className="text-[10px] text-ink-400">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(paidAmount)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Overdue</div>
          <div className="text-lg font-bold text-red">{overdueCount}</div>
          <div className="text-[10px] text-ink-400">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(pendingAmount)}
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, customer, or order..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Paid", "Pending", "Overdue", "Draft", "Cancelled"];
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
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Building2 className="h-4 w-4 text-ink-400" />
          {selectedBranch}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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

      {/* ===== INVOICES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Receipt className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No invoices found</h3>
          <p className="mt-1 text-sm text-ink-500">No invoices match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Invoice</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Customer</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Issue Date</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Due Date</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const customerTypeStyle = CUSTOMER_TYPE_STYLES[item.customerType] || CUSTOMER_TYPE_STYLES.Corporate;
                  const formattedAmount = new Intl.NumberFormat("en-NG", {
                    style: "currency",
                    currency: "NGN",
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  }).format(item.amount);

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToInvoiceDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 font-mono text-[12px]">{item.reference}</div>
                        <div className="text-[11px] text-ink-500">{item.items} items · {item.branch}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">{item.customer}</div>
                        <div className="text-[11px] text-ink-500">
                          <span className={`rounded-full px-2 py-0.5 text-[9px] font-medium ${customerTypeStyle.bg} ${customerTypeStyle.text}`}>
                            {item.customerType}
                          </span>
                          {item.orderRef && (
                            <span className="ml-1 font-mono">{item.orderRef}</span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 font-semibold text-ink-950">{formattedAmount}</td>
                      <td className="px-4 py-3 text-ink-500">{item.issueDate}</td>
                      <td className="px-4 py-3 text-ink-500">{item.dueDate}</td>
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

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Receipt className="h-4 w-4" />
          FIN-02 · Invoices
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} invoices shown
        </span>
      </div>
    </div>
  );
}