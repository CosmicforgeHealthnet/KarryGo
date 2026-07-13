"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";

interface Invoice {
  id: string;
  period: string;
  amount: string;
  status: "Paid" | "Pending";
}

const MOCK_INVOICES: Invoice[] = [
  { id: "INV-001", period: "Jul 2026", amount: "\u20A625,000.00", status: "Paid" },
  { id: "INV-002", period: "Aug 2026", amount: "\u20A618,000.00", status: "Pending" },
];

const USAGE_ROWS = [
  { label: "Deliveries this month", value: "324" },
  { label: "Billable Deliveries", value: "324" },
  { label: "Rate per Delivery", value: "324" },
  { label: "Total Cost", value: "324" },
];

function InvoiceStatusPill({ status }: { status: Invoice["status"] }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold",
        status === "Paid" ? "bg-brand-100 text-brand-700" : "bg-amber-100 text-warning"
      )}
    >
      {status}
    </span>
  );
}

export default function BillingPage() {
  const [hasPaymentMethod, setHasPaymentMethod] = useState(false);

  return (
    <div className="space-y-6">
      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Billing &amp; Usage</h1>
        <p className="mt-1 text-sm text-ink-500">Monitor your billing information</p>
      </div>

      <div className="grid grid-cols-3 gap-3 rounded-2xl bg-white p-5 text-center">
        <div>
          <p className="text-xs font-semibold text-brand-600">Current Balance</p>
          <p className="mt-1 text-lg font-bold text-ink-900">0</p>
        </div>
        <div>
          <p className="text-xs font-semibold text-brand-600">This month&apos;s charges</p>
          <p className="mt-1 text-lg font-bold text-ink-900">0</p>
        </div>
        <div>
          <p className="text-xs font-semibold text-brand-600">Total Billed</p>
          <p className="mt-1 text-lg font-bold text-ink-900">0</p>
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Billing History</h2>

        <div className="hidden overflow-x-auto rounded-2xl bg-white md:block">
          <table className="w-full min-w-[560px] text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs font-semibold uppercase tracking-wide text-ink-500">
                <th className="px-5 py-4">Invoice</th>
                <th className="px-5 py-4">Period</th>
                <th className="px-5 py-4">Amount</th>
                <th className="px-5 py-4">Status</th>
                <th className="px-5 py-4 text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {MOCK_INVOICES.map((invoice) => (
                <tr key={invoice.id} className="border-b border-ink-100 last:border-0">
                  <td className="px-5 py-4 text-ink-500">{invoice.id}</td>
                  <td className="px-5 py-4 text-ink-500">{invoice.period}</td>
                  <td className="px-5 py-4 text-ink-500">{invoice.amount}</td>
                  <td className="px-5 py-4">
                    <InvoiceStatusPill status={invoice.status} />
                  </td>
                  <td className="px-5 py-4 text-right">
                    <button
                      type="button"
                      className="font-semibold text-brand-600 hover:text-brand-700"
                    >
                      View
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="space-y-3 md:hidden">
          {MOCK_INVOICES.map((invoice) => (
            <div key={invoice.id} className="rounded-2xl bg-white p-4">
              <div className="flex items-start justify-between">
                <div>
                  <p className="text-sm font-semibold text-ink-900">{invoice.id}</p>
                  <p className="mt-0.5 text-xs text-ink-500">{invoice.period}</p>
                </div>
                <InvoiceStatusPill status={invoice.status} />
              </div>
              <div className="mt-3 flex items-center justify-between">
                <p className="text-sm font-medium text-ink-900">{invoice.amount}</p>
                <button
                  type="button"
                  className="text-sm font-semibold text-brand-600"
                >
                  View
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Usage Summary</h2>
        <div className="space-y-3 rounded-2xl bg-ink-100/60 p-5">
          {USAGE_ROWS.map((row) => (
            <div key={row.label} className="flex items-center justify-between text-sm">
              <p className="text-ink-700">{row.label}</p>
              <p className="font-semibold text-ink-900">{row.value}</p>
            </div>
          ))}
        </div>
      </div>

      <div className="space-y-4 rounded-2xl bg-white p-5">
        <div>
          <p className="text-sm font-bold text-ink-900">Current Plan</p>
          <p className="mt-1 text-sm text-ink-500">Standard</p>
        </div>
        <div>
          <p className="text-sm font-bold text-ink-900">Billing Model</p>
          <p className="mt-1 text-sm text-ink-500">Per deliveries</p>
        </div>
        <div>
          <p className="text-sm font-bold text-ink-900">Status</p>
          <p className="mt-1 text-sm text-ink-500">Active</p>
        </div>

        <div>
          <p className="mb-2 text-sm font-bold text-ink-900">Payment Method</p>
          {hasPaymentMethod ? (
            <div className="space-y-3">
              <div className="rounded-xl bg-ink-100/60 p-4">
                <div className="flex items-center justify-between">
                  <p className="text-sm font-bold text-ink-900">VISA</p>
                  <p className="text-sm tracking-wider text-ink-700">****************4821</p>
                </div>
                <p className="mt-3 text-xs text-ink-500">Last Updated</p>
                <p className="text-sm text-ink-700">12 Aug 2026</p>
              </div>
              <div className="flex justify-end">
                <button
                  type="button"
                  onClick={() => setHasPaymentMethod(false)}
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
                  onClick={() => setHasPaymentMethod(true)}
                  className="rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
                >
                  Add payment Method
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}