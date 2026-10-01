"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { usePathname, useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  User,
  Users,
  MapPin,
  Shield,
  Clock,
  Mail,
  Briefcase,
  Building2,
  RefreshCw,
} from "lucide-react";
import { crmService } from "@/lib/services/crmService";
import type { CustomerView } from "@/lib/types/crm";
import {
  customerStatusLabel,
  customerStatusTone,
  customerTypeLabel,
  riskStatusLabel,
  riskStatusTone,
} from "@/lib/utils/crmLabels";

const TABS = [
  { label: "Overview", segment: "", icon: User },
  { label: "Contacts", segment: "contacts", icon: Users },
  { label: "Locations", segment: "locations", icon: MapPin },
  { label: "Risk & Credit", segment: "risk", icon: Shield },
  { label: "History", segment: "history", icon: Clock },
];

export default function CustomerDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const router = useRouter();
  const customerId = params.customerId as string;

  const [customer, setCustomer] = useState<CustomerView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchCustomer = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await crmService.getCustomer(customerId);
      setCustomer(data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load customer.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [customerId]);

  useEffect(() => {
    if (customerId) fetchCustomer();
  }, [customerId, fetchCustomer]);

  const basePath = `/crm/customers/${customerId}`;

  if (loading && !customer) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading customer…</p>
      </div>
    );
  }

  if (error && !customer) {
    return (
      <div className="mx-auto max-w-md py-12 text-center space-y-4">
        <div className="rounded-xl border border-red/30 bg-red-bg p-4 text-sm text-red">
          {error}
        </div>
        <div className="flex items-center justify-center gap-2">
          <button
            onClick={fetchCustomer}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Retry
          </button>
          <button
            onClick={() => router.push("/crm/customers")}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-sm font-semibold text-white transition hover:bg-primary/90"
          >
            Back to customers
          </button>
        </div>
      </div>
    );
  }

  if (!customer) return null;

  const statusTone = customerStatusTone(customer.status);
  const riskTone = riskStatusTone(customer.riskStatus);

  return (
    <div className="mx-auto max-w-[1400px] space-y-5 pb-8">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <Link
            href="/crm/customers"
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {customer.displayName || customer.legalName}
              </h1>
              <span className="font-mono text-xs text-ink-400 sm:text-sm">
                {customer.id}
              </span>
              <span
                className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium sm:px-3 ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {customerStatusLabel(customer.status)}
              </span>
              <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-xs font-medium text-ink-600">
                {customerTypeLabel(customer.customerType)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-medium ${riskTone.bg} ${riskTone.text}`}
              >
                <span className={`h-1 w-1 rounded-full ${riskTone.dot}`} />
                {riskStatusLabel(customer.riskStatus)}
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500 sm:text-sm">
              <span className="flex items-center gap-1.5">
                <Building2 className="h-3.5 w-3.5" />
                {customer.branchId || "No branch"}
              </span>
              <span className="flex items-center gap-1.5">
                <Briefcase className="h-3.5 w-3.5" />
                {customer.accountOwnerUserId || "Unassigned"}
              </span>
              <span className="flex items-center gap-1.5">
                <Mail className="h-3.5 w-3.5" />
                <span className="font-mono">{customer.defaultCurrency}</span>
              </span>
            </div>
          </div>
        </div>
      </div>

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

      <div>{children}</div>
    </div>
  );
}