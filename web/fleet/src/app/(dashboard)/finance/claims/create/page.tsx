"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  FileCheck,
  Plus,
  X,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { ClaimType } from "@/lib/types/finance";
import { majorToMinor } from "@/lib/utils/financeLabels";

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

const CLAIM_TYPES: Array<{ value: ClaimType; label: string }> = [
  { value: "INSURANCE", label: "Insurance" },
  { value: "CARGO", label: "Cargo" },
  { value: "VEHICLE", label: "Vehicle" },
  { value: "PROPERTY", label: "Property" },
  { value: "PERSONAL_INJURY", label: "Personal injury" },
  { value: "SECURITY", label: "Security" },
  { value: "OTHER", label: "Other" },
];

export default function CreateClaimPage() {
  const router = useRouter();

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [claimType, setClaimType] = useState<ClaimType>("CARGO");
  const [currency, setCurrency] = useState("NGN");
  const [policyReference, setPolicyReference] = useState("");
  const [amountMajor, setAmountMajor] = useState("");
  const [incidentId, setIncidentId] = useState("");
  const [assetId, setAssetId] = useState("");
  const [shipmentId, setShipmentId] = useState("");
  const [packageId, setPackageId] = useState("");
  const [externalRef, setExternalRef] = useState("");
  const [evidenceIds, setEvidenceIds] = useState<string[]>([]);
  const [evidenceInput, setEvidenceInput] = useState("");

  const amountMinor = useMemo(() => {
    if (amountMajor.trim() === "") return null;
    const trimmed = amountMajor.trim();
    if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return NaN;
    const n = Number(trimmed);
    if (!Number.isFinite(n) || n < 0) return NaN;
    return majorToMinor(n);
  }, [amountMajor]);

  const canSubmit = useMemo(
    () => claimType !== undefined && currency.trim().length === 3,
    [claimType, currency]
  );

  const addEvidence = () => {
    const v = evidenceInput.trim();
    if (!v || evidenceIds.includes(v)) return;
    setEvidenceIds((prev) => [...prev, v]);
    setEvidenceInput("");
  };

  const removeEvidence = (id: string) =>
    setEvidenceIds((prev) => prev.filter((x) => x !== id));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    const amountValue =
      amountMinor === null
        ? null
        : Number.isFinite(amountMinor)
          ? amountMinor
          : null;

    try {
      const created = await financeService.claims.create({
        claimType,
        currency: currency.toUpperCase(),
        policyReference: policyReference.trim() || undefined,
        amountReferenceMinor: amountValue,
        incidentId: incidentId.trim() || undefined,
        assetId: assetId.trim() || undefined,
        shipmentId: shipmentId.trim() || undefined,
        packageId: packageId.trim() || undefined,
        externalClaimReference: externalRef.trim() || undefined,
        evidenceMediaIds: evidenceIds,
      });
      setSuccess(`Claim ${created.id} created.`);
      setTimeout(() => {
        router.push(`/finance/claims/${created.id}`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create claim.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <button
          onClick={() => router.push("/finance/claims")}
          className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          aria-label="Back to claims"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            New Claim
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Log a claim record. Claims track insurance, cargo, or incident
            references with evidence.
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
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Claim details
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Claim type *
                  </label>
                  <select
                    value={claimType}
                    onChange={(e) => setClaimType(e.target.value as ClaimType)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CLAIM_TYPES.map((t) => (
                      <option key={t.value} value={t.value}>
                        {t.label}
                      </option>
                    ))}
                  </select>
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
                    Policy reference
                  </label>
                  <input
                    value={policyReference}
                    onChange={(e) => setPolicyReference(e.target.value)}
                    placeholder="POL-…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Reference amount
                  </label>
                  <input
                    value={amountMajor}
                    onChange={(e) => setAmountMajor(e.target.value)}
                    placeholder="0.00"
                    className={`w-full rounded-xl border bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 ${
                      amountMajor.trim() !== "" &&
                      !Number.isFinite(amountMinor as number)
                        ? "border-red"
                        : "border-line"
                    }`}
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    External claim reference
                  </label>
                  <input
                    value={externalRef}
                    onChange={(e) => setExternalRef(e.target.value)}
                    placeholder="External reference from insurer or counterparty"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Linked records (optional)
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Incident ID
                  </label>
                  <input
                    value={incidentId}
                    onChange={(e) => setIncidentId(e.target.value)}
                    placeholder="inc_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Asset ID
                  </label>
                  <input
                    value={assetId}
                    onChange={(e) => setAssetId(e.target.value)}
                    placeholder="ast_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Shipment ID
                  </label>
                  <input
                    value={shipmentId}
                    onChange={(e) => setShipmentId(e.target.value)}
                    placeholder="shp_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Package ID
                  </label>
                  <input
                    value={packageId}
                    onChange={(e) => setPackageId(e.target.value)}
                    placeholder="pkg_…"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Evidence
              </h3>
              <p className="mb-3 text-xs text-ink-500">
                Add media object IDs from media-file-service. Each is validated
                on submit.
              </p>
              <div className="flex gap-2">
                <input
                  value={evidenceInput}
                  onChange={(e) => setEvidenceInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      addEvidence();
                    }
                  }}
                  placeholder="Media object ID (med_…)"
                  className="flex-1 rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
                <button
                  type="button"
                  onClick={addEvidence}
                  className="flex h-10 items-center gap-1 rounded-xl border border-line bg-surface px-3.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add
                </button>
              </div>
              {evidenceIds.length > 0 && (
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {evidenceIds.map((id) => (
                    <span
                      key={id}
                      className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 font-mono text-[11px] text-ink-700"
                    >
                      {id}
                      <button
                        type="button"
                        onClick={() => removeEvidence(id)}
                        className="text-ink-400 hover:text-red"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Type</span>
                  <span className="font-medium text-ink-950">
                    {CLAIM_TYPES.find((t) => t.value === claimType)?.label}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Currency</span>
                  <span className="font-mono font-medium text-ink-950">
                    {currency}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Evidence</span>
                  <span className="font-medium text-ink-950">
                    {evidenceIds.length}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Reference amount</span>
                  <span className="font-mono font-semibold text-ink-950">
                    {amountMinor !== null &&
                    Number.isFinite(amountMinor as number)
                      ? `${((amountMinor as number) / 100).toLocaleString()} ${currency}`
                      : "—"}
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
                  <FileCheck className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Claim"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/finance/claims")}
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