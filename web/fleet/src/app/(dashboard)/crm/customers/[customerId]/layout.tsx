// src/app/(dashboard)/crm/customers/[customerId]/layout.tsx

"use client";

import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import {
  ArrowLeft,
  User,
  Users,
  MapPin,
  Shield,
  Clock,
  CheckCircle2,
  AlertCircle,
  MoreHorizontal,
  Edit3,
  Mail,
  Phone,
  Briefcase,
  Building2,
  Star,
  TrendingUp,
  CalendarDays,
  CreditCard,
  FileText,
  Receipt,
  Package,
  Truck,
} from "lucide-react";

const TABS = [
  { label: "Overview", segment: "", icon: User },
  { label: "Contacts", segment: "contacts", icon: Users },
  { label: "Locations", segment: "locations", icon: MapPin },
  { label: "Risk & Credit", segment: "risk", icon: Shield },
  { label: "History", segment: "history", icon: Clock },
];

// Mock data - in real app, fetch from API
const MOCK_CUSTOMER = {
  id: "CUST-001",
  name: "Charitos Hospital",
  initials: "CH",
  type: "Corporate",
  status: "Active",
  risk: "Low",
  email: "accounts@charitoshospital.com",
  phone: "+234 800 123 4567",
  location: "Ikeja, Lagos",
  branch: "Ikeja",
  accountOwner: "F. Daniel",
  lifetimeValue: 4200000,
  orders: 46,
  lastActivity: "2 days ago",
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "Follow-up": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "At Risk": { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
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

export default function CustomerDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const customerId = params.customerId as string;
  const customer = MOCK_CUSTOMER;

  const basePath = `/crm/customers/${customerId}`;
  const statusStyle = STATUS_STYLES[customer.status] || STATUS_STYLES.Active;
  const typeStyle = TYPE_STYLES[customer.type] || TYPE_STYLES.Corporate;
  const riskStyle = RISK_STYLES[customer.risk] || RISK_STYLES.Low;

  const formattedLTV = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(customer.lifetimeValue);

  return (
    <div className="mx-auto max-w-[1400px] space-y-5 pb-8">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <Link
            href="/crm/customers"
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {customer.name}
              </h1>
              <span className="font-mono text-xs text-ink-400 sm:text-sm">
                {customer.id}
              </span>
              <span
                className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium sm:px-3 ${statusStyle.bg} ${statusStyle.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                {customer.status}
              </span>
              <span
                className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${typeStyle.bg} ${typeStyle.text}`}
              >
                {customer.type}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-medium ${riskStyle.bg} ${riskStyle.text}`}
              >
                <span className={`h-1 w-1 rounded-full ${riskStyle.dot}`} />
                {customer.risk} risk
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500 sm:text-sm">
              <span className="flex items-center gap-1.5">
                <Briefcase className="h-3.5 w-3.5" />
                Account Owner: {customer.accountOwner}
              </span>
              <span className="flex items-center gap-1.5">
                <Building2 className="h-3.5 w-3.5" />
                {customer.branch}
              </span>
              <span className="flex items-center gap-1.5">
                <Mail className="h-3.5 w-3.5" />
                {customer.email}
              </span>
              <span className="flex items-center gap-1.5">
                <Phone className="h-3.5 w-3.5" />
                {customer.phone}
              </span>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <CreditCard className="h-3.5 w-3.5" />
            New Order
          </button>
        </div>
      </div>

      {/* ===== QUICK STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Lifetime Value</div>
          <div className="text-lg font-bold text-ink-950">{formattedLTV}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Orders</div>
          <div className="text-lg font-bold text-ink-950">{customer.orders}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Last Activity</div>
          <div className="text-lg font-bold text-ink-950">{customer.lastActivity}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm cursor-default">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Location</div>
          <div className="text-lg font-bold text-ink-950">{customer.location}</div>
        </div>
      </div>

      {/* ===== TABS ===== */}
      <div className="flex gap-1 border-b border-line overflow-x-auto scrollbar-hide">
        {TABS.map((tab) => {
          const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
          const active = pathname === href;
          const Icon = tab.icon;

          return (
            <Link
              key={tab.label}
              href={href}
              className={`flex items-center gap-1.5 px-3.5 py-2.5 text-[12.5px] font-medium transition whitespace-nowrap border-b-2 ${
                active
                  ? "text-primary border-primary"
                  : "text-ink-600 hover:text-ink-950 border-transparent hover:border-ink-300"
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </Link>
          );
        })}
      </div>

      {/* ===== CHILDREN ===== */}
      <div>{children}</div>
    </div>
  );
}