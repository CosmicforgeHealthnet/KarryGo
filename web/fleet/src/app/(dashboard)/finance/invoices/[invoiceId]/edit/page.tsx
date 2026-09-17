"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Plus,
  Trash2,
  Save,
  Send,
  CheckCircle2,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type {
  InvoiceLineInput,
  InvoiceView,
} from "@/lib/types/finance";
import {
  formatMinor,
  invoiceStatusLabel,
  invoiceStatusTone,
  majorToMinor,
  minorToMajor,
} from "@/lib/utils/financeLabels";

type DraftLine = {
  key: string;
  sourceType: string;
  sourceReference: string;
  description: string;
  quantity: string;
  unitAmountMajor: string;
  taxAmountMajor: string;
  taxReference: string;
  adjustmentReference: string;
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
  adjustmentReference: "",
});

const toDraft = (l: InvoiceLineInput & { id?: string }): DraftLine => ({
  key: l.id ?? `line-${Date.now()}-${Math.random().toString(36).slice(2)}`,
  sourceType: l.sourceType,
  sourceReference: l.sourceReference,
  description: l.description,
  quantity: String(l.quantity),
  unitAmountMajor: String(minorToMajor(l.unitAmountMinor)),
  taxAmountMajor: String(minorToMajor(l.taxAmountMinor)),
  taxReference: l.taxReference ?? "",
  adjustmentReference: l.adjustmentReference ?? "",
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

export default function InvoiceEditorPage() {
  const router = useRouter();
  const params = useParams();
  const invoiceId = params.invoiceId as string;

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [approving, setApproving] = useState(false);
  const [issuing, setIssuing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [invoice, setInvoice] = useState<InvoiceView | null>(null);
  const [currency, setCurrency] = useState("NGN");
  const [dueDate, setDueDate] = useState("");
  const [customerId, setCustomerId] = useState("");
  const [contractId, setContractId] = useState("");
  const [quotationId, setQuotationId] = useState("");
  const [branchId, setBranchId] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([newLine()]);

  const fetchInvoice = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.invoices.get(invoiceId);
      setInvoice(result);
      setCurrency(result.currency);
      setDueDate(result.due_date);
      setCustomerId(result.customer_id);
      setContractId(result.contract_id ?? "");
      setQuotationId(result.quotation_id ?? "");
      setBranchId(result.branch_id ?? "");
      setLines(
        result.lines.length > 0
          ? result.lines.map((l) =>
              toDraft({
                sourceType: l.source_type,
                sourceReference: l.source_reference,
                description: l.description,
                quantity: l.quantity,
                unitAmountMinor: l.unit_amount_minor,
                taxAmountMinor: l.tax_amount_minor,
                taxReference: l.tax_reference,
                adjustmentReference: l.adjustment_reference,
                id: l.id,
              })
            )
          : [newLine()]
      );
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load invoice.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [invoiceId]);

  useEffect(() => {
    if (invoiceId) fetchInvoice();
  }, [invoiceId, fetchInvoice]);

  const editable =
    invoice?.status === "DRAFT" || invoice?.status === "PENDING_APPROVAL";
  const isApprovable =
    invoice?.status === "DRAFT" || invoice?.status === "PENDING_APPROVAL";
  const isIssuable = invoice?.status === "APPROVED";

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

  const canSave = useMemo(() => {
    if (!editable) return false;
    if (!dueDate || !customerId.trim()) return false;
    return computedLines.every(
      (c) =>
        c.line.sourceType.trim() &&
        c.line.sourceReference.trim() &&
        c.line.description.trim() &&
        c.qty !== null &&
        c.unit !== null &&
        c.tax !== null
    );
  }, [computedLines, editable, dueDate, customerId]);

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

  const buildLines = (): InvoiceLineInput[] =>
    computedLines.map((c) => ({
      sourceType: c.line.sourceType.trim(),
      sourceReference: c.line.sourceReference.trim(),
      description: c.line.description.trim(),
      quantity: c.qty!,
      unitAmountMinor: c.unit!,
      taxAmountMinor: c.tax!,
      taxReference: c.line.taxReference.trim() || undefined,
      adjustmentReference: c.line.adjustmentReference.trim() || undefined,
    }));

  const handleSave = async () => {
    if (!invoice || !canSave || saving) return;
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const updated = await financeService.invoices.update(
        invoice.id,
        {
          branchId: branchId.trim() || undefined,
          customerId: customerId.trim(),
          contractId: contractId.trim() || undefined,
          quotationId: quotationId.trim() || undefined,
          currency: currency.toUpperCase(),
          dueDate,
          lines: buildLines(),
        },
        invoice.version
      );
      setInvoice(updated);
      setSuccess("Invoice saved.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to save invoice.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  const handleApprove = async () => {
    if (!invoice || approving) return;
    setApproving(true);
    setError(null);
    try {
      const updated = await financeService.invoices.approve(
        invoice.id,
        invoice.version
      );
      setInvoice(updated);
      setSuccess("Invoice approved.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to approve invoice.";
      setError(message);
    } finally {
      setApproving(false);
    }
  };

  const handleIssue = async () => {
    if (!invoice || issuing) return;
    setIssuing(true);
    setError(null);
    try {
      const updated = await financeService.invoices.issue(
        invoice.id,
        invoice.version
      );
      setInvoice(updated);
      setSuccess("Invoice issued.");
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to issue invoice.";
      setError(message);
    } finally {
      setIssuing(false);
    }
  };

  if (loading && !invoice) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading invoice…</p>
      </div>
    );
  }

  if (error && !invoice) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load invoice
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <button
          onClick={fetchInvoice}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!invoice) return null;

  const statusTone = invoiceStatusTone(invoice.status);

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/finance/invoices/${invoiceId}`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to invoice"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {editable ? "Edit Invoice" : "Invoice"}
              </h1>
              <span className="font-mono text-sm text-ink-400">{invoice.id}</span>
              <span
                className={`rounded-full px-2.5 py-0.5 text-[10px] font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                {invoiceStatusLabel(invoice.status)}
              </span>
              <span className="text-sm text-ink-400">v{invoice.version}</span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">
              {editable
                ? "Update invoice fields and line items."
                : "This invoice is no longer editable."}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchInvoice}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Reload"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {editable && (
            <button
              onClick={handleSave}
              disabled={!canSave || saving}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {saving ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              {saving ? "Saving…" : "Save draft"}
            </button>
          )}
          {isApprovable && (
            <button
              onClick={handleApprove}
              disabled={approving}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {approving ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCircle2 className="h-3.5 w-3.5" />
              )}
              Approve
            </button>
          )}
          {isIssuable && (
            <button
              onClick={handleIssue}
              disabled={issuing}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {issuing ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Send className="h-3.5 w-3.5" />
              )}
              Issue
            </button>
          )}
        </div>
      </div>

      {success && (
        <div className="rounded-lg border border-green/20 bg-green-bg px-4 py-2.5 text-sm text-green">
          {success}
        </div>
      )}

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => setError(null)}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Dismiss
          </button>
        </div>
      )}

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
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
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
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
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
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
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
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Currency *
                </label>
                <input
                  value={currency}
                  onChange={(e) => setCurrency(e.target.value.toUpperCase())}
                  maxLength={3}
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm uppercase text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Due date *
                </label>
                <input
                  type="date"
                  value={dueDate}
                  onChange={(e) => setDueDate(e.target.value)}
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
                />
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-[15px] font-semibold text-ink-950">
                Line items
              </h3>
              {editable && (
                <button
                  onClick={addLine}
                  className="flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-[11px] font-medium text-white transition hover:bg-primary/90"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add line
                </button>
              )}
            </div>
            <div className="space-y-3">
              {lines.map((line) => {
                const computed = computedLines.find(
                  (c) => c.line.key === line.key
                );
                const invalidQty =
                  computed && computed.qty === null && line.quantity.trim() !== "";
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
                        disabled={!editable}
                        className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60"
                      />
                      <input
                        value={line.sourceReference}
                        onChange={(e) =>
                          updateLine(line.key, {
                            sourceReference: e.target.value,
                          })
                        }
                        placeholder="Source reference"
                        disabled={!editable}
                        className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60"
                      />
                      <input
                        value={line.description}
                        onChange={(e) =>
                          updateLine(line.key, { description: e.target.value })
                        }
                        placeholder="Description"
                        disabled={!editable}
                        className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60 sm:col-span-2"
                      />
                      <input
                        value={line.quantity}
                        onChange={(e) =>
                          updateLine(line.key, { quantity: e.target.value })
                        }
                        placeholder="Qty"
                        disabled={!editable}
                        className={`rounded-lg border bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60 ${
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
                        disabled={!editable}
                        className={`rounded-lg border bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60 ${
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
                        disabled={!editable}
                        className={`rounded-lg border bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60 ${
                          invalidTax ? "border-red" : "border-line"
                        }`}
                      />
                      <input
                        value={line.taxReference}
                        onChange={(e) =>
                          updateLine(line.key, { taxReference: e.target.value })
                        }
                        placeholder="Tax reference (optional)"
                        disabled={!editable}
                        className="rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60"
                      />
                    </div>
                    <div className="mt-2 flex items-center justify-between border-t border-line pt-2">
                      <span className="text-xs text-ink-500">Line total</span>
                      <span className="text-sm font-semibold text-ink-950">
                        {formatMinor(computed?.lineTotal ?? 0, currency)}
                      </span>
                    </div>
                    {editable && lines.length > 1 && (
                      <button
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
                <span className="font-medium text-ink-950">{lines.length}</span>
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
        </div>
      </div>
    </div>
  );
}