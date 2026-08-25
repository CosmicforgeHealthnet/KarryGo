// src/app/(dashboard)/finance/invoices/[invoiceId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Receipt,
  TrendingUp,
  TrendingDown,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  Users,
  MapPin,
  Briefcase,
  CalendarDays,
  MoreHorizontal,
  Download,
  FileText,
  Printer,
  User,
  DollarSign,
  Send,
  Edit3,
  XCircle,
  CreditCard,
  Building2,
  Hash,
  Tag,
  Mail,
  Phone,
  Calendar,
  Package,
  Truck,
} from "lucide-react";

type InvoiceDetail = {
  id: string;
  reference: string;
  customer: string;
  customerType: "Corporate" | "SME" | "Individual";
  customerEmail: string;
  customerPhone: string;
  amount: number;
  currency: string;
  status: "Paid" | "Pending" | "Overdue" | "Draft" | "Cancelled";
  issueDate: string;
  dueDate: string;
  paidDate: string | null;
  items: {
    id: string;
    description: string;
    quantity: number;
    unitPrice: number;
    total: number;
  }[];
  subtotal: number;
  tax: number;
  discount: number;
  totalAmount: number;
  orderRef: string | null;
  branch: string;
  paymentReference: string | null;
  paymentMethod: string | null;
  notes: string | null;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  history: {
    id: string;
    action: string;
    status: string;
    timestamp: string;
    user: string;
    comment: string | null;
  }[];
};

// Mock data
const MOCK_INVOICE: InvoiceDetail = {
  id: "INV-001",
  reference: "INV-2026-001",
  customer: "Charitos Hospital",
  customerType: "Corporate",
  customerEmail: "accounts@charitoshospital.com",
  customerPhone: "+234 800 123 4567",
  amount: 1280000,
  currency: "NGN",
  status: "Paid",
  issueDate: "2026-08-01",
  dueDate: "2026-08-15",
  paidDate: "2026-08-14",
  items: [
    { id: "ITEM-001", description: "Medical supply delivery - Order OR-129", quantity: 12, unitPrice: 106667, total: 1280000 },
  ],
  subtotal: 1280000,
  tax: 0,
  discount: 0,
  totalAmount: 1280000,
  orderRef: "OR-129",
  branch: "Ikeja",
  paymentReference: "PAY-2026-001",
  paymentMethod: "Bank Transfer",
  notes: "Full payment received on 2026-08-14. Delivery completed and verified.",
  createdBy: "F. Daniel",
  approvedBy: "O. Adebayo",
  approvedAt: "2026-08-02",
  history: [
    { id: "HIS-001", action: "Invoice created", status: "Draft", timestamp: "2026-08-01 09:00", user: "F. Daniel", comment: "Initial invoice creation." },
    { id: "HIS-002", action: "Invoice issued", status: "Pending", timestamp: "2026-08-02 10:00", user: "O. Adebayo", comment: "Invoice issued to customer." },
    { id: "HIS-003", action: "Payment received", status: "Paid", timestamp: "2026-08-14 14:30", user: "System", comment: "Payment received via bank transfer." },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Paid: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
  },
  Overdue: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileText className="h-4 w-4" />,
  },
  Cancelled: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <XCircle className="h-4 w-4" />,
  },
};

const CUSTOMER_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Corporate: { bg: "bg-blue-bg", text: "text-blue" },
  SME: { bg: "bg-amber-bg", text: "text-amber" },
  Individual: { bg: "bg-green-bg", text: "text-green" },
};

export default function InvoiceDetailPage() {
  const router = useRouter();
  const params = useParams();
  const invoiceId = params.invoiceId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<InvoiceDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_INVOICE, id: invoiceId });
      setLoading(false);
    };
    loadData();
  }, [invoiceId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading invoice details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Receipt className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Invoice not found</h3>
        <p className="mt-1 text-sm text-ink-500">The invoice you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/finance/invoices")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to invoices
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Draft;
  const customerTypeStyle = CUSTOMER_TYPE_STYLES[data.customerType] || CUSTOMER_TYPE_STYLES.Corporate;
  const formattedAmount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.amount);
  const formattedSubtotal = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.subtotal);
  const formattedTax = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.tax);
  const formattedDiscount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.discount);
  const formattedTotal = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.totalAmount);

  const isEditable = data.status === "Draft" || data.status === "Pending";
  const isIssuable = data.status === "Draft";

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/invoices")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Invoice {data.reference}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}
              >
                {statusStyle.icon}
                {data.status}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.customer} · {data.branch}</p>
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            PDF
          </button>
          {isEditable && (
            <button
              onClick={() => router.push(`/finance/invoices/${data.id}/edit`)}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit
            </button>
          )}
          {isIssuable && (
            <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
              <Send className="h-3.5 w-3.5" />
              Issue
            </button>
          )}
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Amount Card */}
          <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${customerTypeStyle.bg} ${customerTypeStyle.text}`}>
                {data.customerType}
              </span>
              <span className="text-ink-300">·</span>
              <span>{data.branch}</span>
              {data.orderRef && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="font-mono">Order: {data.orderRef}</span>
                </>
              )}
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formattedAmount}
              </span>
              <span className="ml-2 text-sm text-ink-400">{data.currency}</span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <CalendarDays className="h-4 w-4" />
                Issued: {data.issueDate}
              </span>
              <span className="text-ink-300">·</span>
              <span className="flex items-center gap-1.5">
                <AlertCircle className="h-4 w-4" />
                Due: {data.dueDate}
              </span>
              {data.paidDate && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1.5 text-green">
                    <CheckCircle2 className="h-4 w-4" />
                    Paid: {data.paidDate}
                  </span>
                </>
              )}
            </div>
          </div>

          {/* Invoice Items */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Line Items</h3>
              <span className="text-xs text-ink-400">{data.items.length} items</span>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-sm">
                <thead>
                  <tr className="bg-surface-2/70">
                    <th className="border-b border-line px-3 py-2 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Description</th>
                    <th className="border-b border-line px-3 py-2 text-right text-[10px] font-bold uppercase tracking-wide text-ink-500">Qty</th>
                    <th className="border-b border-line px-3 py-2 text-right text-[10px] font-bold uppercase tracking-wide text-ink-500">Unit Price</th>
                    <th className="border-b border-line px-3 py-2 text-right text-[10px] font-bold uppercase tracking-wide text-ink-500">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((item) => (
                    <tr key={item.id} className="border-b border-line hover:bg-surface-2 transition-colors">
                      <td className="px-3 py-2 text-ink-950">{item.description}</td>
                      <td className="px-3 py-2 text-right text-ink-600">{item.quantity}</td>
                      <td className="px-3 py-2 text-right text-ink-600">
                        {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.unitPrice)}
                      </td>
                      <td className="px-3 py-2 text-right font-medium text-ink-950">
                        {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(item.total)}
                      </td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr className="border-t border-line">
                    <td colSpan={3} className="px-3 py-2 text-right text-sm text-ink-500">Subtotal</td>
                    <td className="px-3 py-2 text-right text-sm font-medium text-ink-950">{formattedSubtotal}</td>
                  </tr>
                  {data.tax > 0 && (
                    <tr>
                      <td colSpan={3} className="px-3 py-2 text-right text-sm text-ink-500">Tax</td>
                      <td className="px-3 py-2 text-right text-sm font-medium text-ink-950">{formattedTax}</td>
                    </tr>
                  )}
                  {data.discount > 0 && (
                    <tr>
                      <td colSpan={3} className="px-3 py-2 text-right text-sm text-ink-500">Discount</td>
                      <td className="px-3 py-2 text-right text-sm font-medium text-red">-{formattedDiscount}</td>
                    </tr>
                  )}
                  <tr className="border-t-2 border-line">
                    <td colSpan={3} className="px-3 py-2 text-right text-sm font-semibold text-ink-950">Total</td>
                    <td className="px-3 py-2 text-right text-sm font-bold text-ink-950">{formattedTotal}</td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </div>

          {/* History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">History</h3>
            <div className="space-y-3">
              {data.history.map((event, index) => (
                <div key={event.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-2.5 w-2.5 rounded-full ${
                      event.status === "Paid" ? "bg-green" :
                      event.status === "Pending" ? "bg-amber" :
                      event.status === "Draft" ? "bg-blue" :
                      "bg-red"
                    }`} />
                    {index < data.history.length - 1 && (
                      <div className="h-full w-0.5 bg-line" />
                    )}
                  </div>
                  <div className="flex-1 pb-3">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-ink-950">{event.action}</span>
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        event.status === "Paid" ? "bg-green-bg text-green" :
                        event.status === "Pending" ? "bg-amber-bg text-amber" :
                        event.status === "Draft" ? "bg-blue-bg text-blue" :
                        "bg-surface-2 text-ink-500"
                      }`}>
                        {event.status}
                      </span>
                    </div>
                    <div className="text-xs text-ink-500">
                      {event.user} · {event.timestamp}
                    </div>
                    {event.comment && (
                      <div className="mt-1 text-xs text-ink-600 bg-surface-2 rounded-lg px-2.5 py-1.5">
                        "{event.comment}"
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Customer Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Customer</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Name</div>
                <div className="text-sm font-medium text-ink-950">{data.customer}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Type</div>
                <div className="text-sm font-medium text-ink-950">
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${customerTypeStyle.bg} ${customerTypeStyle.text}`}>
                    {data.customerType}
                  </span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Email</div>
                <div className="text-sm font-medium text-ink-950 flex items-center gap-1.5">
                  <Mail className="h-3.5 w-3.5 text-ink-400" />
                  {data.customerEmail}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Phone</div>
                <div className="text-sm font-medium text-ink-950 flex items-center gap-1.5">
                  <Phone className="h-3.5 w-3.5 text-ink-400" />
                  {data.customerPhone}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Branch</div>
                <div className="text-sm font-medium text-ink-950 flex items-center gap-1.5">
                  <Building2 className="h-3.5 w-3.5 text-ink-400" />
                  {data.branch}
                </div>
              </div>
            </div>
          </div>

          {/* Payment Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Payment Information</h3>
            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Status</div>
                <div className="text-sm font-medium text-ink-950">
                  <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                    {data.status}
                  </span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Payment Reference</div>
                <div className="text-sm font-medium text-ink-950 font-mono">{data.paymentReference || "—"}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Payment Method</div>
                <div className="text-sm font-medium text-ink-950">{data.paymentMethod || "—"}</div>
              </div>
            </div>
          </div>

          {/* Notes */}
          {data.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
              <p className="text-sm text-ink-600">{data.notes}</p>
            </div>
          )}

          {/* Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => router.push(`/finance/invoices/${data.id}/edit`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="h-4 w-4" />
                  Edit Invoice
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                <span className="flex items-center gap-2">
                  <FileText className="h-4 w-4" />
                  Export PDF
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push(`/finance/receivables`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <CreditCard className="h-4 w-4" />
                  View Receivables
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Receipt className="h-4 w-4" />
          FIN-04 · Invoice Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.status}
        </span>
      </div>
    </div>
  );
}