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
  FileText,
  XCircle,
} from "lucide-react";
import { crmService, type CreateQuotationLineItemInput } from "@/lib/services/crmService";
import type { QuotationView, LineItemView } from "@/lib/types/crm";
import {
  formatMoney,
  quotationStatusLabel,
  quotationStatusTone,
} from "@/lib/utils/crmLabels";

type DraftLine = {
  key: string;
  description: string;
  quantity: string;
  unitAmountMajor: string;
};

const newLine = (): DraftLine => ({
  key: `line-${Date.now()}-${Math.random().toString(36).slice(2)}`,
  description: "",
  quantity: "1",
  unitAmountMajor: "0",
});

const toDraftLine = (item: LineItemView): DraftLine => ({
  key: item.id,
  description: item.description,
  quantity: item.quantity,
  unitAmountMajor: (item.unitAmountMinor / 100).toString(),
});

const parseQuantity = (raw: string): number | null => {
  const trimmed = raw.trim();
  if (!/^\d+(\.\d{1,6})?$/.test(trimmed)) return null;
  const n = Number(trimmed);
  return Number.isFinite(n) && n > 0 ? n : null;
};

const parseMajorToMinor = (raw: string): number | null => {
  const trimmed = raw.trim();
  if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null;
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.round(n * 100);
};

export default function QuotationEditorPage() {
  const router = useRouter();
  const params = useParams();
  const quotationId = params.quotationId as string;

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [quotation, setQuotation] = useState<QuotationView | null>(null);
  const [validFrom, setValidFrom] = useState("");
  const [validUntil, setValidUntil] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([]);

  const fetchQuotation = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await crmService.getQuotation(quotationId);
      setQuotation(result);
      setValidFrom(result.validFrom.slice(0, 10));
      setValidUntil(result.validUntil.slice(0, 10));
      setLines(
        (result.lineItems ?? []).length > 0
          ? (result.lineItems ?? []).map(toDraftLine)
          : [newLine()]
      );
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load quotation.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [quotationId]);

  useEffect(() => {
    if (quotationId) fetchQuotation();
  }, [quotationId, fetchQuotation]);

  const editable = quotation?.status === "DRAFT";

  const computedLines = useMemo(() => {
    return lines.map((line) => {
      const qty = parseQuantity(line.quantity);
      const unit = parseMajorToMinor(line.unitAmountMajor);
      const lineAmountMinor =
        qty !== null && unit !== null ? Math.round(qty * unit) : 0;
      return { line, qty, unit, lineAmountMinor };
    });
  }, [lines]);

  const subtotalMinor = useMemo(
    () => computedLines.reduce((sum, c) => sum + c.lineAmountMinor, 0),
    [computedLines]
  );

  const canSave = useMemo(() => {
    if (!editable) return false;
    return computedLines.every(
      (c) =>
        c.line.description.trim().length > 0 &&
        c.qty !== null &&
        c.unit !== null
    );
  }, [computedLines, editable]);

  const updateLine = (key: string, patch: Partial<DraftLine>) => {
    setLines((prev) =>
      prev.map((l) => (l.key === key ? { ...l, ...patch } : l))
    );
  };

  const addLine = () => setLines((prev) => [...prev, newLine()]);

  const removeLine = (key: string) =>
    setLines((prev) => (prev.length > 1 ? prev.filter((l) => l.key !== key) : prev));

  const buildLineItems = (): CreateQuotationLineItemInput[] =>
    computedLines.map((c) => ({
      description: c.line.description.trim(),
      quantity: c.line.quantity,
      unitAmountMinor: c.unit!,
      lineAmountMinor: c.lineAmountMinor,
    }));

  const handleSave = async () => {
    if (!quotation || !canSave || saving) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await crmService.updateQuotation(
        quotationId,
        {
          validFrom: new Date(validFrom).toISOString(),
          validUntil: new Date(validUntil).toISOString(),
          lineItems: buildLineItems(),
          replaceLineItems: true,
        },
        quotation.version
      );
      setQuotation(updated);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to save quotation.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  const handleSubmit = async () => {
    if (!quotation || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const updated = await crmService.submitQuotation(
        quotationId,
        quotation.version
      );
      setQuotation(updated);
      router.push(`/crm/quotations/${quotationId}`);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to submit quotation.";
      setError(message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = async () => {
    if (!quotation || cancelling) return;
    setCancelling(true);
    setError(null);
    try {
      const updated = await crmService.cancelQuotation(
        quotationId,
        quotation.version,
        "cancelled_by_staff"
      );
      setQuotation(updated);
      router.push(`/crm/quotations/${quotationId}`);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to cancel quotation.";
      setError(message);
    } finally {
      setCancelling(false);
    }
  };

  if (loading && !quotation) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading quotation…</p>
      </div>
    );
  }

  if (error && !quotation) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load quotation
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <button
          onClick={fetchQuotation}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!quotation) return null;

  const statusTone = quotationStatusTone(quotation.status);

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/crm/quotations/${quotationId}`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to quotation"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {editable ? "Edit Quotation" : "Quotation"}
              </h1>
              <span className="font-mono text-sm text-ink-400">
                {quotation.quotationNumber}
              </span>
              <span
                className={`rounded-full px-2.5 py-0.5 text-[10px] font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                {quotationStatusLabel(quotation.status)}
              </span>
              <span className="text-sm text-ink-400">
                v{quotation.currentVersion}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">
              {editable
                ? "Update validity window and line items."
                : "This quotation is no longer editable."}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchQuotation}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Reload"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {editable && (
            <>
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
              <button
                onClick={handleSubmit}
                disabled={submitting}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
              >
                {submitting ? (
                  <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Send className="h-3.5 w-3.5" />
                )}
                Submit
              </button>
            </>
          )}
        </div>
      </div>

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
              Validity Window
            </h3>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Valid from
                </label>
                <input
                  type="date"
                  value={validFrom}
                  onChange={(e) => setValidFrom(e.target.value)}
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Valid until
                </label>
                <input
                  type="date"
                  value={validUntil}
                  onChange={(e) => setValidUntil(e.target.value)}
                  disabled={!editable}
                  className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
                />
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-[15px] font-semibold text-ink-950">
                Line Items
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
            <div className="space-y-2">
              {lines.map((line) => {
                const computed = computedLines.find((c) => c.line.key === line.key);
                const invalidQty =
                  computed && computed.qty === null && line.quantity.trim() !== "";
                const invalidUnit =
                  computed &&
                  computed.unit === null &&
                  line.unitAmountMajor.trim() !== "";
                return (
                  <div
                    key={line.key}
                    className="flex flex-wrap items-start gap-2 rounded-lg border border-line p-3"
                  >
                    <div className="min-w-[180px] flex-1">
                      <input
                        value={line.description}
                        onChange={(e) =>
                          updateLine(line.key, { description: e.target.value })
                        }
                        placeholder="Item description"
                        disabled={!editable}
                        className="w-full rounded-lg border border-line bg-surface px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60"
                      />
                    </div>
                    <div className="w-24">
                      <input
                        value={line.quantity}
                        onChange={(e) =>
                          updateLine(line.key, { quantity: e.target.value })
                        }
                        placeholder="Qty"
                        disabled={!editable}
                        className={`w-full rounded-lg border bg-surface px-2 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60 ${
                          invalidQty ? "border-red" : "border-line"
                        }`}
                      />
                    </div>
                    <div className="w-32">
                      <input
                        value={line.unitAmountMajor}
                        onChange={(e) =>
                          updateLine(line.key, {
                            unitAmountMajor: e.target.value,
                          })
                        }
                        placeholder="Unit amount"
                        disabled={!editable}
                        className={`w-full rounded-lg border bg-surface px-2 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:opacity-60 ${
                          invalidUnit ? "border-red" : "border-line"
                        }`}
                      />
                    </div>
                    <div className="w-28 pt-1.5 text-right text-sm font-medium text-ink-950">
                      {formatMoney({
                        amountMinor: computed?.lineAmountMinor ?? 0,
                        currency: quotation.total.currency,
                      })}
                    </div>
                    {editable && lines.length > 1 && (
                      <button
                        onClick={() => removeLine(line.key)}
                        className="rounded-lg p-1.5 text-red transition hover:bg-red-bg"
                        aria-label="Remove line"
                      >
                        <Trash2 className="h-4 w-4" />
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
                  {quotation.total.currency}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Line items</span>
                <span className="font-medium text-ink-950">{lines.length}</span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Subtotal</span>
                <span className="font-medium text-ink-950">
                  {formatMoney({
                    amountMinor: subtotalMinor,
                    currency: quotation.total.currency,
                  })}
                </span>
              </div>
              <div className="flex items-center justify-between pt-1">
                <span className="text-ink-950 font-semibold">Total</span>
                <span className="text-lg font-bold text-ink-950">
                  {formatMoney({
                    amountMinor: subtotalMinor,
                    currency: quotation.total.currency,
                  })}
                </span>
              </div>
            </div>
          </div>

          {editable && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Danger Zone
              </h3>
              <button
                onClick={handleCancel}
                disabled={cancelling}
                className="flex w-full items-center justify-between rounded-xl border border-red/30 bg-red-bg/50 px-4 py-2.5 text-[12.5px] font-medium text-red transition hover:bg-red-bg disabled:opacity-50"
              >
                <span className="flex items-center gap-2">
                  {cancelling ? (
                    <RefreshCw className="h-4 w-4 animate-spin" />
                  ) : (
                    <XCircle className="h-4 w-4" />
                  )}
                  Cancel quotation
                </span>
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}