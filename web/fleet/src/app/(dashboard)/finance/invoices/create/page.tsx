"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  Plus,
  Trash2,
  Receipt,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { InvoiceLineInput } from "@/lib/types/finance";
import { formatMinor, majorToMinor } from "@/lib/utils/financeLabels";

type DraftLine = {
  key: string;
  sourceType: string;
  sourceReference: string;
  description: string;
  quantity: string;
  unitAmountMajor: string;
  taxAmountMajor: string;
  taxReference: string;
};

const newLine = (): DraftLine => ({
  key: `line-${Date.now()}-${Math.random().toString(36).slice(2)}`,
  sourceType: "",
  sourceReference: "",
  description: "",
  quantity: "1",
  unitAmountMajor: "0",
  taxAmountMajor: "0",
  taxReference: "",
});

const parseQuantity = (raw: string): number | null => {
  const trimmed = raw.trim();
  if (!/^\d+(\.\d{1,4})?$/.test(trimmed)) return null;
  const n = Number(trimmed);
  return Number.isFinite(n) && n > 0 ? n : null;
};

const parseMajorToMinor = (raw: string): number | null => {
  const trimmed = raw.trim();
  if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null;
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n < 0) return null;
  return majorToMinor(n);
};

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

export default function CreateInvoicePage() {
  const router = useRouter();

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [branchId, setBranchId] = useState("");
  const [customerId, setCustomerId] = useState("");
  const [contractId, setContractId] = useState("");
  const [quotationId, setQuotationId] = useState("");
  const [currency, setCurrency] = useState("NGN");
  const [dueDate, setDueDate] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([newLine()]);

  const computedLines = useMemo(() => {
    return lines.map((line) => {
      const qty = parseQuantity(line.quantity);
      const unit = parseMajorToMinor(line.unitAmountMajor);
      const tax = parseMajorToMinor(line.taxAmountMajor);
      const lineTotal =
        qty !== null && unit !== null ? Math.round(qty * unit) : 0;
      return { line, qty, unit, tax, lineTotal };
    });
  }, [lines]);

  const subtotalMinor = useMemo(
    () => computedLines.reduce((sum, c) => sum + c.lineTotal, 0),
    [computedLines]
  );
  const taxTotalMinor = useMemo(
    () => computedLines.reduce((sum, c) => sum + (c.tax ?? 0), 0),
    [computedLines]
  );

  const canSubmit = useMemo(() => {
    if (!customerId.trim() || !dueDate) return false;
    return computedLines.every(
      (c) =>
        c.line.sourceType.trim() &&
        c.line.sourceReference.trim() &&
        c.line.description.trim() &&
        c.qty !== null &&
        c.unit !== null &&
        c.tax !== null
    );
  }, [computedLines, customerId, dueDate]);

  const updateLine = (key: string, patch: Partial<DraftLine>) => {
    setLines((prev) =>
      prev.map((l) => (l.key === key ? { ...l, ...patch } : l))
    );
  };

  const addLine = () => setLines((prev) => [...prev, newLine()]);

  const removeLine = (key: string) =>
    setLines((prev) =>
      prev.length > 1 ? prev.filter((l) => l.key !== key) : prev
    );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    const payloadLines: InvoiceLineInput[] = computedLines.map((c) => ({
      sourceType: c.line.sourceType.trim(),
      sourceReference: c.line.sourceReference.trim(),
      description: c.line.description.trim(),
      quantity: c.qty!,
      unitAmountMinor: c.unit!,
      taxAmountMinor: c.tax!,
      taxReference: c.line.taxReference.trim() || undefined,
    }));

    try {
      const created = await financeService.invoices.create({
        branchId: branchId.trim() || undefined,
        customerId: customerId.trim(),
        contractId: contractId.trim() || undefined,
        quotationId: quotationId.trim() || undefined,
        currency: currency.toUpperCase(),
        dueDate,
        lines: payloadLines,
      });
      setSuccess(`Invoice ${created.id} created.`);
      setTimeout(() => {
        router.push(`/finance/invoices/${created.id}`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create invoice.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <button
          onClick={() => router.push("/finance/invoices")}
          className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          aria-label="Back to invoices"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            New Invoice
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Create a draft invoice with line items.
          </p>
        </div>
      </div>

      {success && (
        <div className="rounded-lg border border-green/20 bg-green-bg px-4 py-2.5 text-sm text-green">
          {success}
        </div>
      )}

      {error && (
        <div className="flex items-start gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0 mt-0.5" />
          <span>{error}</span>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Invoice details
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Customer ID *
                  </label>
                  <input
                    value={customerId}
                    onChange={(e) => setCustomerId(e.target.value)}
                    placeholder="cst_…"
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Contract ID
                  </label>
                  <input
                    value={contractId}
                    onChange={(e) => setContractId(e.target.value)}
                    placeholder="ctr_… (optional)"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Quotation ID
                  </label>
                  <input
                    value={quotationId}
                    onChange={(e) => setQuotationId(e.target.value)}
                    placeholder="qot_… (optional)"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Branch ID
                  </label>
                  <input
                    value={branchId}
                    onChange={(e) => setBranchId(e.target.value)}
                    placeholder="brn_… (optional)"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Currency *
                  </label>
                  <select
                    value={currency}
                    onChange={(e) => setCurrency(e.target.value)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
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
                    Due date *
                  </label>
                  <input
                    type="date"
                    value={dueDate}
                    onChange={(e) => setDueDate(e.target.value)}
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <div className="mb-4 flex items-center justify-between">
                <h3 className="text-[15px] font-semibold text-ink-950">
                  Line items
                </h3>
                <button
                  type="button"
                  onClick={addLine}
                  className="flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-[11px] font-medium text-white transition hover:bg-primary/90"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add line
                </button>
              </div>
              <div className="space-y-3">
                {lines.map((line) => {
                  const computed = computedLines.find(
                    (c) => c.line.key === line.key
                  );
                  const invalidQty =
                    computed &&
                    computed.qty === null &&
                    line.quantity.trim() !== "";
                  const invalidUnit =
                    computed &&
                    computed.unit === null &&
                    line.unitAmountMajor.trim() !== "";
                  const invalidTax =
                    computed &&
                    computed.tax === null &&
                    line.taxAmountMajor.trim() !== "";
                  return (
                    <div
                      key={line.key}
                      className="rounded-lg border border-line p-3"
                    >
                      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                        <input
                          value={line.sourceType}
                          onChange={(e) =>
                            updateLine(line.key, { sourceType: e.target.value })
                          }
                          placeholder="Source type (e.g. TRIP)"
                          className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                        />
                        <input
                          value={line.sourceReference}
                          onChange={(e) =>
                            updateLine(line.key, {
                              sourceReference: e.target.value,
                            })
                          }
                          placeholder="Source reference"
                          className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                        />
                        <input
                          value={line.description}
                          onChange={(e) =>
                            updateLine(line.key, {
                              description: e.target.value,
                            })
                          }
                          placeholder="Description"
                          className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary sm:col-span-2"
                        />
                        <input
                          value={line.quantity}
                          onChange={(e) =>
                            updateLine(line.key, { quantity: e.target.value })
                          }
                          placeholder="Qty"
                          className={`rounded-lg border bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary ${
                            invalidQty ? "border-red" : "border-line"
                          }`}
                        />
                        <input
                          value={line.unitAmountMajor}
                          onChange={(e) =>
                            updateLine(line.key, {
                              unitAmountMajor: e.target.value,
                            })
                          }
                          placeholder="Unit amount"
                          className={`rounded-lg border bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary ${
                            invalidUnit ? "border-red" : "border-line"
                          }`}
                        />
                        <input
                          value={line.taxAmountMajor}
                          onChange={(e) =>
                            updateLine(line.key, {
                              taxAmountMajor: e.target.value,
                            })
                          }
                          placeholder="Tax amount"
                          className={`rounded-lg border bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary ${
                            invalidTax ? "border-red" : "border-line"
                          }`}
                        />
                        <input
                          value={line.taxReference}
                          onChange={(e) =>
                            updateLine(line.key, {
                              taxReference: e.target.value,
                            })
                          }
                          placeholder="Tax reference (optional)"
                          className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                        />
                      </div>
                      <div className="mt-2 flex items-center justify-between border-t border-line pt-2">
                        <span className="text-xs text-ink-500">
                          Line total
                        </span>
                        <span className="text-sm font-semibold text-ink-950">
                          {formatMinor(computed?.lineTotal ?? 0, currency)}
                        </span>
                      </div>
                      {lines.length > 1 && (
                        <button
                          type="button"
                          onClick={() => removeLine(line.key)}
                          className="mt-2 flex items-center gap-1 text-[11px] font-medium text-red transition hover:text-red/80"
                        >
                          <Trash2 className="h-3 w-3" />
                          Remove line
                        </button>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Currency</span>
                  <span className="font-mono font-medium text-ink-950">
                    {currency}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Line items</span>
                  <span className="font-medium text-ink-950">
                    {lines.length}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Subtotal</span>
                  <span className="font-medium text-ink-950">
                    {formatMinor(subtotalMinor, currency)}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Tax</span>
                  <span className="font-medium text-ink-950">
                    {formatMinor(taxTotalMinor, currency)}
                  </span>
                </div>
                <div className="flex items-center justify-between pt-1">
                  <span className="font-semibold text-ink-950">Total</span>
                  <span className="text-lg font-bold text-ink-950">
                    {formatMinor(subtotalMinor + taxTotalMinor, currency)}
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
                  <Receipt className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Invoice"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/finance/invoices")}
                className="mt-2 flex w-full items-center justify-center gap-2 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}