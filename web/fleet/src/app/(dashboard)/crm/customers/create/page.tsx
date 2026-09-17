"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  UserPlus,
  Save,
} from "lucide-react";
import { crmService, type CreateCustomerInput } from "@/lib/services/crmService";
import type { CustomerType } from "@/lib/types/crm";

const CUSTOMER_TYPES: Array<{ value: CustomerType; label: string }> = [
  { value: "BUSINESS", label: "Business" },
  { value: "INDIVIDUAL", label: "Individual" },
  { value: "GOVERNMENT", label: "Government" },
  { value: "NONPROFIT", label: "Non-profit" },
  { value: "INTERNAL_BUSINESS_UNIT", label: "Internal business unit" },
  { value: "OTHER", label: "Other" },
];

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

type FormState = {
  customerType: CustomerType;
  legalName: string;
  displayName: string;
  registrationNumber: string;
  taxReference: string;
  accountOwnerUserId: string;
  defaultCurrency: string;
  paymentTermsReference: string;
};

const INITIAL_FORM: FormState = {
  customerType: "BUSINESS",
  legalName: "",
  displayName: "",
  registrationNumber: "",
  taxReference: "",
  accountOwnerUserId: "",
  defaultCurrency: "NGN",
  paymentTermsReference: "",
};

export default function CreateCustomerPage() {
  const router = useRouter();

  const [form, setForm] = useState<FormState>(INITIAL_FORM);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const canSubmit = useMemo(
    () =>
      form.legalName.trim().length > 0 &&
      form.displayName.trim().length > 0 &&
      form.defaultCurrency.trim().length === 3,
    [form]
  );

  const setField = <K extends keyof FormState>(field: K, value: FormState[K]) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    const payload: CreateCustomerInput = {
      customerType: form.customerType,
      legalName: form.legalName.trim(),
      displayName: form.displayName.trim(),
      defaultCurrency: form.defaultCurrency.toUpperCase(),
    };
    if (form.registrationNumber.trim()) {
      payload.registrationNumber = form.registrationNumber.trim();
    }
    if (form.taxReference.trim()) {
      payload.taxReference = form.taxReference.trim();
    }
    if (form.accountOwnerUserId.trim()) {
      payload.accountOwnerUserId = form.accountOwnerUserId.trim();
    }
    if (form.paymentTermsReference.trim()) {
      payload.paymentTermsReference = form.paymentTermsReference.trim();
    }

    try {
      const created = await crmService.createCustomer(payload);
      setSuccess(`${created.displayName} has been added to the CRM.`);
      setTimeout(() => {
        router.push(`/crm/customers/${created.id}`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create customer.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/crm/customers")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to customers"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
              Create Customer
            </h1>
            <p className="mt-1 text-sm text-ink-600">
              Add a new customer to the CRM.
            </p>
          </div>
        </div>
      </div>

      {success && (
        <div className="rounded-xl border border-green/20 bg-green-bg p-4">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-green p-1.5">
              <CheckCircle2 className="h-4 w-4 text-white" />
            </div>
            <div className="text-sm font-semibold text-green">{success}</div>
          </div>
        </div>
      )}

      {error && (
        <div className="rounded-xl border border-red/30 bg-red-bg p-4">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-red p-1.5">
              <AlertCircle className="h-4 w-4 text-white" />
            </div>
            <div className="text-sm font-semibold text-red">{error}</div>
          </div>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-4">
                Basic Information
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Legal name *
                  </label>
                  <input
                    value={form.legalName}
                    onChange={(e) => setField("legalName", e.target.value)}
                    placeholder="Registered legal entity name"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    required
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Display name *
                  </label>
                  <input
                    value={form.displayName}
                    onChange={(e) => setField("displayName", e.target.value)}
                    placeholder="Name shown across the CRM"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    required
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Customer type
                  </label>
                  <select
                    value={form.customerType}
                    onChange={(e) =>
                      setField("customerType", e.target.value as CustomerType)
                    }
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CUSTOMER_TYPES.map((t) => (
                      <option key={t.value} value={t.value}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Default currency *
                  </label>
                  <select
                    value={form.defaultCurrency}
                    onChange={(e) => setField("defaultCurrency", e.target.value)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CURRENCIES.map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Registration number
                  </label>
                  <input
                    value={form.registrationNumber}
                    onChange={(e) => setField("registrationNumber", e.target.value)}
                    placeholder="Optional"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Tax reference
                  </label>
                  <input
                    value={form.taxReference}
                    onChange={(e) => setField("taxReference", e.target.value)}
                    placeholder="Optional"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Account owner user ID
                  </label>
                  <input
                    value={form.accountOwnerUserId}
                    onChange={(e) => setField("accountOwnerUserId", e.target.value)}
                    placeholder="Optional"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Payment terms reference
                  </label>
                  <input
                    value={form.paymentTermsReference}
                    onChange={(e) =>
                      setField("paymentTermsReference", e.target.value)
                    }
                    placeholder="Optional"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Type</span>
                  <span className="font-medium text-ink-950">
                    {CUSTOMER_TYPES.find((t) => t.value === form.customerType)?.label ?? "—"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Currency</span>
                  <span className="font-mono font-medium text-ink-950">
                    {form.defaultCurrency}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Legal name</span>
                  <span className="font-medium text-ink-950 truncate max-w-[160px]">
                    {form.legalName || "—"}
                  </span>
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <button
                type="submit"
                disabled={saving || !canSubmit}
                className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {saving ? (
                  <RefreshCw className="h-4 w-4 animate-spin" />
                ) : (
                  <UserPlus className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Customer"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/crm/customers")}
                className="mt-2 flex w-full items-center justify-center gap-2 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                <Save className="h-4 w-4" />
                Cancel
              </button>
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}