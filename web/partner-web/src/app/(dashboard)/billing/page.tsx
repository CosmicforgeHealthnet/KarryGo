"use client";

import { useState, useEffect } from "react";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";
import { billingService } from "@/services/billing-service";
import type { Invoice, BillingOverview, BillingUsageSummary, PaymentMethod } from "@/types";

function InvoiceStatusPill({ status }: { status: Invoice["status"] }) {
  const statusMap = {
    paid: { label: "Paid", className: "bg-brand-100 text-brand-700" },
    pending: { label: "Pending", className: "bg-amber-100 text-warning" },
    overdue: { label: "Overdue", className: "bg-red-100 text-danger" },
  };
  const s = statusMap[status] || statusMap.pending;

  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold",
        s.className
      )}
    >
      {s.label}
    </span>
  );
}

export default function BillingPage() {
  const token = useAuthStore((s) => s.token);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [overview, setOverview] = useState<BillingOverview | null>(null);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [usage, setUsage] = useState<BillingUsageSummary | null>(null);
  const [paymentMethods, setPaymentMethods] = useState<PaymentMethod[]>([]);
  const [showPaymentModal, setShowPaymentModal] = useState(false);

  useEffect(() => {
    if (!token) return;
    fetchBillingData();
  }, [token]);

  const fetchBillingData = async () => {
    setLoading(true);
    setError(null);
    try {
      const [overviewData, invoicesData, usageData] = await Promise.all([
        billingService.getOverview(token!),
        billingService.listInvoices(token!),
        billingService.getDashboardSummary(token!),
      ]);
      setOverview(overviewData);
      setInvoices(invoicesData || []);
      setUsage(usageData);
    } catch (err) {
      setError("Failed to load billing data");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const handleAddPaymentMethod = () => {
    setShowPaymentModal(true);
  };

  const handleCloseModal = () => {
    setShowPaymentModal(false);
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <div className="rounded-2xl bg-white p-5">
          <h1 className="text-lg font-bold text-ink-900">Billing &amp; Usage</h1>
          <p className="mt-1 text-sm text-ink-500">Loading your billing information...</p>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-ink-500">Loading...</div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <div className="rounded-2xl bg-white p-5">
          <h1 className="text-lg font-bold text-ink-900">Billing &amp; Usage</h1>
          <p className="mt-1 text-sm text-ink-500">Monitor your billing information</p>
        </div>
        <div className="rounded-xl bg-danger/10 p-4 text-sm text-danger">
          {error}
          <button onClick={fetchBillingData} className="ml-3 font-semibold underline">
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Payment Method Modal */}
      {showPaymentModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-bold text-ink-900">Add Payment Method</h2>
              <button
                type="button"
                onClick={handleCloseModal}
                className="rounded-lg p-2 text-ink-500 hover:bg-ink-100"
              >
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <div className="mt-6 flex flex-col items-center justify-center py-8 text-center">
              <div className="mb-4 rounded-full bg-brand-100 p-4">
                <svg className="h-12 w-12 text-brand-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <h3 className="text-base font-bold text-ink-900">Payment Integration Coming Soon</h3>
              <p className="mt-2 text-sm text-ink-500">
                We're working on integrating secure payment methods. You'll be able to add your card here soon.
              </p>
              <p className="mt-1 text-xs text-ink-400">
                For now, you can continue using the sandbox plan.
              </p>
            </div>

            <button
              type="button"
              onClick={handleCloseModal}
              className="mt-4 w-full rounded-xl bg-brand-600 px-6 py-3 text-sm font-semibold text-white hover:bg-brand-700"
            >
              Got it
            </button>
          </div>
        </div>
      )}

      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Billing &amp; Usage</h1>
        <p className="mt-1 text-sm text-ink-500">Monitor your billing information</p>
      </div>

      <div className="grid grid-cols-3 gap-3 rounded-2xl bg-white p-5 text-center">
        <div>
          <p className="text-xs font-semibold text-brand-600">Current Balance</p>
          <p className="mt-1 text-lg font-bold text-ink-900">
            {overview?.current_balance?.display || "₦0.00"}
          </p>
        </div>
        <div>
          <p className="text-xs font-semibold text-brand-600">This month&apos;s charges</p>
          <p className="mt-1 text-lg font-bold text-ink-900">
            {overview?.current_month_charges?.display || "₦0.00"}
          </p>
        </div>
        <div>
          <p className="text-xs font-semibold text-brand-600">Total Billed</p>
          <p className="mt-1 text-lg font-bold text-ink-900">
            {overview?.total_billed?.display || "₦0.00"}
          </p>
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Billing History</h2>

        {invoices.length === 0 ? (
          <div className="rounded-2xl bg-ink-100/60 px-6 py-10 text-center">
            <p className="text-sm text-ink-500">No invoices yet.</p>
          </div>
        ) : (
          <>
            <div className="hidden overflow-x-auto rounded-2xl bg-white md:block">
              <table className="w-full min-w-[560px] text-left text-sm">
                <thead>
                  <tr className="border-b border-ink-100 text-xs font-semibold uppercase tracking-wide text-ink-500">
                    <th className="px-5 py-4">Invoice</th>
                    <th className="px-5 py-4">Period</th>
                    <th className="px-5 py-4">Due Date</th>
                    <th className="px-5 py-4">Amount</th>
                    <th className="px-5 py-4">Status</th>
                    <th className="px-5 py-4 text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {invoices.map((invoice) => (
                    <tr key={invoice.id} className="border-b border-ink-100 last:border-0">
                      <td className="px-5 py-4 text-ink-500">{invoice.invoice_number}</td>
                      <td className="px-5 py-4 text-ink-500">
                        {new Date(invoice.period_start).toLocaleDateString()} - {new Date(invoice.period_end).toLocaleDateString()}
                      </td>
                      <td className="px-5 py-4 text-ink-500">
                        {new Date(invoice.due_date).toLocaleDateString()}
                      </td>
                      <td className="px-5 py-4 text-ink-500">{invoice.amount.display}</td>
                      <td className="px-5 py-4">
                        <InvoiceStatusPill status={invoice.status} />
                      </td>
                      <td className="px-5 py-4 text-right">
                        {invoice.pdf_url ? (
                          <a
                            href={invoice.pdf_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="font-semibold text-brand-600 hover:text-brand-700"
                          >
                            Download
                          </a>
                        ) : (
                          <span className="text-ink-400">—</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="space-y-3 md:hidden">
              {invoices.map((invoice) => (
                <div key={invoice.id} className="rounded-2xl bg-white p-4">
                  <div className="flex items-start justify-between">
                    <div>
                      <p className="text-sm font-semibold text-ink-900">{invoice.invoice_number}</p>
                      <p className="mt-0.5 text-xs text-ink-500">
                        {new Date(invoice.period_start).toLocaleDateString()} - {new Date(invoice.period_end).toLocaleDateString()}
                      </p>
                    </div>
                    <InvoiceStatusPill status={invoice.status} />
                  </div>
                  <div className="mt-3 flex items-center justify-between">
                    <p className="text-sm font-medium text-ink-900">{invoice.amount.display}</p>
                    {invoice.pdf_url ? (
                      <a
                        href={invoice.pdf_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-sm font-semibold text-brand-600"
                      >
                        Download
                      </a>
                    ) : (
                      <span className="text-sm text-ink-400">—</span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Usage Summary</h2>
        <div className="space-y-3 rounded-2xl bg-ink-100/60 p-5">
          <div className="flex items-center justify-between text-sm">
            <p className="text-ink-700">Deliveries this month</p>
            <p className="font-semibold text-ink-900">{usage?.deliveries_this_month || 0}</p>
          </div>
          <div className="flex items-center justify-between text-sm">
            <p className="text-ink-700">Billable Deliveries</p>
            <p className="font-semibold text-ink-900">{usage?.billable_deliveries || 0}</p>
          </div>
          <div className="flex items-center justify-between text-sm">
            <p className="text-ink-700">Rate per Delivery</p>
            <p className="font-semibold text-ink-900">{usage?.rate_per_delivery?.display || "₦0.00"}</p>
          </div>
          <div className="flex items-center justify-between text-sm">
            <p className="text-ink-700">Total Cost</p>
            <p className="font-semibold text-ink-900">{usage?.total_cost?.display || "₦0.00"}</p>
          </div>
        </div>
      </div>

      <div className="space-y-4 rounded-2xl bg-white p-5">
        <div>
          <p className="text-sm font-bold text-ink-900">Current Plan</p>
          <p className="mt-1 text-sm text-ink-500 capitalize">Sandbox</p>
        </div>
        <div>
          <p className="text-sm font-bold text-ink-900">Billing Model</p>
          <p className="mt-1 text-sm text-ink-500">Per delivery</p>
        </div>
        <div>
          <p className="text-sm font-bold text-ink-900">Status</p>
          <p className="mt-1 text-sm text-ink-500 capitalize">Active</p>
        </div>

        <div>
          <p className="mb-2 text-sm font-bold text-ink-900">Payment Method</p>
          {paymentMethods.length > 0 ? (
            <div className="space-y-3">
              <div className="rounded-xl bg-ink-100/60 p-4">
                {paymentMethods.map((method) => (
                  <div key={method.id} className="flex items-center justify-between">
                    <p className="text-sm font-bold capitalize text-ink-900">{method.type}</p>
                    <p className="text-sm tracking-wider text-ink-700">****{method.last_four}</p>
                  </div>
                ))}
                <p className="mt-3 text-xs text-ink-500">Last Updated</p>
                <p className="text-sm text-ink-700">
                  {paymentMethods.length > 0 
                    ? new Date(paymentMethods[0].created_at).toLocaleDateString()
                    : "Not set"}
                </p>
              </div>
              <div className="flex justify-end">
                <button
                  type="button"
                  className="rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
                >
                  Change Billing Method
                </button>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="rounded-xl bg-ink-100/60 p-4 text-center text-sm text-ink-500">
                No payment method added.
              </div>
              <div className="flex justify-end">
                <button
                  type="button"
                  onClick={handleAddPaymentMethod}
                  className="rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
                >
                  Add Payment Method
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}