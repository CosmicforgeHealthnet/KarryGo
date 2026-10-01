"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowRight,
  RefreshCw,
  Building2,
  Briefcase,
  MapPin,
  Mail,
  FileText,
  Shield,
} from "lucide-react";
import { crmService } from "@/lib/services/crmService";
import type { CustomerView } from "@/lib/types/crm";
import {
  customerStatusLabel,
  customerStatusTone,
  customerTypeLabel,
  riskStatusLabel,
  riskStatusTone,
  formatDateTime,
  timeAgo,
} from "@/lib/utils/crmLabels";

export default function CustomerOverviewPage() {
  const router = useRouter();
  const params = useParams();
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

  if (loading && !customer) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading customer details…</p>
      </div>
    );
  }

  if (error && !customer) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <RefreshCw className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load customer
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <button
          onClick={fetchCustomer}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!customer) return null;

  const statusTone = customerStatusTone(customer.status);
  const riskTone = riskStatusTone(customer.riskStatus);
  const isArchived = customer.status === "ARCHIVED";

  return (
    <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
      <div className="space-y-5">
        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
            Company Information
          </h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Legal name
              </div>
              <div className="text-sm font-medium text-ink-950">
                {customer.legalName}
              </div>
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Display name
              </div>
              <div className="text-sm font-medium text-ink-950">
                {customer.displayName}
              </div>
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Type
              </div>
              <div className="text-sm font-medium text-ink-950">
                {customerTypeLabel(customer.customerType)}
              </div>
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Currency
              </div>
              <div className="font-mono text-sm font-medium text-ink-950">
                {customer.defaultCurrency}
              </div>
            </div>
            {customer.branchId && (
              <div className="flex items-start gap-2">
                <Building2 className="mt-0.5 h-4 w-4 text-ink-400" />
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Branch
                  </div>
                  <div className="text-sm font-medium text-ink-950">
                    {customer.branchId}
                  </div>
                </div>
              </div>
            )}
            {customer.accountOwnerUserId && (
              <div className="flex items-start gap-2">
                <Briefcase className="mt-0.5 h-4 w-4 text-ink-400" />
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Account owner
                  </div>
                  <div className="text-sm font-medium text-ink-950">
                    {customer.accountOwnerUserId}
                  </div>
                </div>
              </div>
            )}
            {customer.registrationNumberMasked && (
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Registration
                </div>
                <div className="font-mono text-sm font-medium text-ink-950">
                  {customer.registrationNumberMasked}
                </div>
              </div>
            )}
            {customer.taxReferenceMasked && (
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Tax reference
                </div>
                <div className="font-mono text-sm font-medium text-ink-950">
                  {customer.taxReferenceMasked}
                </div>
              </div>
            )}
          </div>
        </div>

        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">Status</h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Customer status
              </div>
              <span
                className={`mt-1 inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {customerStatusLabel(customer.status)}
              </span>
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                Risk status
              </div>
              <span
                className={`mt-1 inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ${riskTone.bg} ${riskTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${riskTone.dot}`} />
                {riskStatusLabel(customer.riskStatus)}
              </span>
            </div>
          </div>
          {isArchived && customer.archivedAt && (
            <div className="mt-3 rounded-lg border border-red/20 bg-red-bg/30 p-3 text-xs text-red">
              Archived {formatDateTime(customer.archivedAt)}
            </div>
          )}
        </div>
      </div>

      <div className="space-y-5">
        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
            Metadata
          </h3>
          <div className="space-y-2 text-sm">
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Version</span>
              <span className="font-mono font-medium text-ink-950">
                {customer.version}
              </span>
            </div>
            <div className="flex items-center justify-between border-b border-line pb-2">
              <span className="text-ink-500">Created</span>
              <span className="text-ink-950">{timeAgo(customer.createdAt)}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-ink-500">Updated</span>
              <span className="text-ink-950">{timeAgo(customer.updatedAt)}</span>
            </div>
          </div>
        </div>

        <div className="rounded-2xl border border-line bg-surface p-5">
          <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
            Quick Actions
          </h3>
          <div className="space-y-2">
            <button
              onClick={() => router.push(`/crm/customers/${customerId}/contacts`)}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <Mail className="h-4 w-4" />
                Manage contacts
              </span>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </button>
            <button
              onClick={() => router.push(`/crm/customers/${customerId}/locations`)}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <MapPin className="h-4 w-4" />
                Manage locations
              </span>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </button>
            <button
              onClick={() => router.push(`/crm/customers/${customerId}/risk`)}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <Shield className="h-4 w-4" />
                Risk & credit
              </span>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </button>
            <button
              onClick={() => router.push(`/crm/customers/${customerId}/history`)}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <FileText className="h-4 w-4" />
                View history
              </span>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}