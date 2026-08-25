// src/app/(dashboard)/crm/customers/[customerId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  User,
  MapPin,
  Mail,
  Phone,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Users,
  MoreHorizontal,
  Download,
  FileText,
  Printer,
  DollarSign,
  CreditCard,
  Receipt,
  Package,
  Truck,
  Star,
  Shield,
  Edit3,
  Send,
} from "lucide-react";

type CustomerDetail = {
  id: string;
  name: string;
  initials: string;
  type: "Corporate" | "SME" | "Enterprise" | "Individual";
  status: "Active" | "Follow-up" | "At Risk" | "Inactive";
  risk: "Low" | "Medium" | "High";
  email: string;
  phone: string;
  location: string;
  branch: string;
  accountOwner: string;
  lifetimeValue: number;
  orders: number;
  lastActivity: string;
  createdAt: string;
  notes: string | null;
  recentOrders: {
    id: string;
    reference: string;
    amount: number;
    status: "Completed" | "Pending" | "Cancelled";
    date: string;
    service: string;
  }[];
  recentQuotations: {
    id: string;
    reference: string;
    amount: number;
    status: "Draft" | "Issued" | "Accepted" | "Expired";
    date: string;
  }[];
  contacts: {
    id: string;
    name: string;
    role: string;
    email: string;
    phone: string;
    primary: boolean;
  }[];
};

// Mock data
const MOCK_CUSTOMER: CustomerDetail = {
  id: "CUST-001",
  name: "Charitos Hospital",
  initials: "CH",
  type: "Corporate",
  status: "Active",
  risk: "Low",
  email: "accounts@charitoshospital.com",
  phone: "+234 800 123 4567",
  location: "42 Medical Way, Ikeja, Lagos",
  branch: "Ikeja",
  accountOwner: "F. Daniel",
  lifetimeValue: 4200000,
  orders: 46,
  lastActivity: "2 days ago",
  createdAt: "2026-01-15",
  notes: "Key healthcare client. Regular medical supply deliveries. Annual contract.",
  recentOrders: [
    { id: "ORD-001", reference: "OR-129", amount: 1280000, status: "Completed", date: "2026-08-21", service: "Dispatch" },
    { id: "ORD-002", reference: "OR-125", amount: 840000, status: "Pending", date: "2026-08-20", service: "Hauling" },
    { id: "ORD-003", reference: "OR-121", amount: 560000, status: "Completed", date: "2026-08-18", service: "Dispatch" },
  ],
  recentQuotations: [
    { id: "QT-001", reference: "QT-2026-012", amount: 1200000, status: "Issued", date: "2026-08-19" },
    { id: "QT-002", reference: "QT-2026-008", amount: 950000, status: "Accepted", date: "2026-08-10" },
    { id: "QT-003", reference: "QT-2026-005", amount: 720000, status: "Expired", date: "2026-07-25" },
  ],
  contacts: [
    { id: "CON-001", name: "Dr. Adeola Ogunlesi", role: "Procurement Director", email: "adeola.ogunlesi@charitoshospital.com", phone: "+234 800 111 2222", primary: true },
    { id: "CON-002", name: "Mr. Femi Adeyemi", role: "Logistics Manager", email: "femi.adeyemi@charitoshospital.com", phone: "+234 800 333 4444", primary: false },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  "Follow-up": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
  },
  "At Risk": {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Inactive: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-4 w-4" />,
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Corporate: { bg: "bg-blue-bg", text: "text-blue" },
  SME: { bg: "bg-amber-bg", text: "text-amber" },
  Enterprise: { bg: "bg-purple-bg", text: "text-purple" },
  Individual: { bg: "bg-green-bg", text: "text-green" },
};

const RISK_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Low: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  High: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const ORDER_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Cancelled: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const QUOTATION_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Issued: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Accepted: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

export default function CustomerDetailPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CustomerDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_CUSTOMER, id: customerId });
      setLoading(false);
    };
    loadData();
  }, [customerId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading customer details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <User className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Customer not found</h3>
        <p className="mt-1 text-sm text-ink-500">The customer you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/crm/customers")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to customers
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Active;
  const typeStyle = TYPE_STYLES[data.type] || TYPE_STYLES.Corporate;
  const riskStyle = RISK_STYLES[data.risk] || RISK_STYLES.Low;
  const formattedLTV = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.lifetimeValue);

  return (
    <div className="space-y-5">
      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Company Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Company Information</h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Type</div>
                <div className="text-sm font-medium text-ink-950">
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                    {data.type}
                  </span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Branch</div>
                <div className="text-sm font-medium text-ink-950">{data.branch}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Account Owner</div>
                <div className="text-sm font-medium text-ink-950">{data.accountOwner}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Customer Since</div>
                <div className="text-sm font-medium text-ink-950">{data.createdAt}</div>
              </div>
              <div className="col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Location</div>
                <div className="flex items-center gap-1.5 text-sm font-medium text-ink-950">
                  <MapPin className="h-4 w-4 text-ink-400" />
                  {data.location}
                </div>
              </div>
              <div className="col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Contact</div>
                <div className="flex flex-wrap items-center gap-3 text-sm font-medium text-ink-950">
                  <span className="flex items-center gap-1.5">
                    <Mail className="h-4 w-4 text-ink-400" />
                    {data.email}
                  </span>
                  <span className="flex items-center gap-1.5">
                    <Phone className="h-4 w-4 text-ink-400" />
                    {data.phone}
                  </span>
                </div>
              </div>
            </div>
          </div>

          {/* Recent Orders */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Recent Orders</h3>
              <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1">
                View all <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {data.recentOrders.map((order) => {
                const statusStyle2 = ORDER_STATUS_STYLES[order.status] || ORDER_STATUS_STYLES.Completed;
                const formattedAmount = new Intl.NumberFormat("en-NG", {
                  style: "currency",
                  currency: "NGN",
                  minimumFractionDigits: 0,
                  maximumFractionDigits: 0,
                }).format(order.amount);

                return (
                  <div
                    key={order.id}
                    className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2"
                    onClick={() => router.push(`/operations/orders/${order.id}`)}
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-[12px] text-ink-950">{order.reference}</span>
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle2.bg} ${statusStyle2.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle2.dot}`} />
                          {order.status}
                        </span>
                      </div>
                      <div className="text-[12px] text-ink-500">{order.service} · {order.date}</div>
                    </div>
                    <div className="text-right">
                      <div className="text-sm font-semibold text-ink-950">{formattedAmount}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Quick Stats */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Stats</h3>
            <div className="space-y-3">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-sm text-ink-500">Lifetime Value</span>
                <span className="text-sm font-semibold text-ink-950">{formattedLTV}</span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-sm text-ink-500">Total Orders</span>
                <span className="text-sm font-semibold text-ink-950">{data.orders}</span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-sm text-ink-500">Last Activity</span>
                <span className="text-sm font-semibold text-ink-950">{data.lastActivity}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-ink-500">Risk Level</span>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${riskStyle.bg} ${riskStyle.text}`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${riskStyle.dot}`} />
                  {data.risk}
                </span>
              </div>
            </div>
          </div>

          {/* Recent Quotations */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Recent Quotations</h3>
              <button
                onClick={() => router.push("/crm/quotations")}
                className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1"
              >
                View all <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {data.recentQuotations.map((quote) => {
                const statusStyle2 = QUOTATION_STATUS_STYLES[quote.status] || QUOTATION_STATUS_STYLES.Draft;
                const formattedAmount = new Intl.NumberFormat("en-NG", {
                  style: "currency",
                  currency: "NGN",
                  minimumFractionDigits: 0,
                  maximumFractionDigits: 0,
                }).format(quote.amount);

                return (
                  <div
                    key={quote.id}
                    className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2"
                    onClick={() => router.push(`/crm/quotations/${quote.id}`)}
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-[12px] text-ink-950">{quote.reference}</span>
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle2.bg} ${statusStyle2.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle2.dot}`} />
                          {quote.status}
                        </span>
                      </div>
                      <div className="text-[12px] text-ink-500">{quote.date}</div>
                    </div>
                    <div className="text-right">
                      <div className="text-sm font-semibold text-ink-950">{formattedAmount}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Primary Contact */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Primary Contact</h3>
            {data.contacts.filter(c => c.primary).map((contact) => (
              <div key={contact.id} className="space-y-2">
                <div className="flex items-center gap-3">
                  <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-primary text-xs font-bold text-white">
                    {contact.name.split(" ").map(n => n[0]).join("")}
                  </div>
                  <div>
                    <div className="text-sm font-medium text-ink-950">{contact.name}</div>
                    <div className="text-xs text-ink-500">{contact.role}</div>
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-3 text-xs text-ink-500">
                  <span className="flex items-center gap-1.5">
                    <Mail className="h-3.5 w-3.5" />
                    {contact.email}
                  </span>
                  <span className="flex items-center gap-1.5">
                    <Phone className="h-3.5 w-3.5" />
                    {contact.phone}
                  </span>
                </div>
                <button
                  onClick={() => router.push(`/crm/customers/${customerId}/contacts`)}
                  className="mt-2 text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1"
                >
                  View all contacts <ArrowRight className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>

          {/* Notes */}
          {data.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
              <p className="text-sm text-ink-600">{data.notes}</p>
            </div>
          )}

          {/* Quick Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => router.push(`/crm/customers/${customerId}/edit`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="h-4 w-4" />
                  Edit Customer
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push(`/crm/customers/${customerId}/contacts`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Users className="h-4 w-4" />
                  Manage Contacts
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push(`/crm/quotations/create?customer=${customerId}`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <FileText className="h-4 w-4" />
                  Create Quotation
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push(`/private-ops/orders/create?customer=${customerId}`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Package className="h-4 w-4" />
                  Create Order
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
          <User className="h-4 w-4" />
          CRM-03 · Customer Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.orders} orders
        </span>
      </div>
    </div>
  );
}