"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  FileCheck,
  Save,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { ClaimRow, ClaimStatus } from "@/lib/types/finance";
import {
  claimStatusLabel,
  claimStatusTone,
  claimTypeLabel,
  claimTypeTone,
  formatDateTime,
  formatMinor,
  timeAgo,
} from "@/lib/utils/financeLabels";

const STATUS_OPTIONS: ClaimStatus[] = [
  "DRAFT",
  "SUBMITTED",
  "UNDER_REVIEW",
  "MORE_INFORMATION_REQUIRED",
  "APPROVED",
  "PARTIALLY_APPROVED",
  "REJECTED",
  "SETTLED",
  "CLOSED",
  "CANCELLED",
];

export default function ClaimDetailPage() {
  const router = useRouter();
  const params = useParams();
  const claimId = params.claimId as string;

  const [claim, setClaim] = useState<ClaimRow | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);

  const [status, setStatus] = useState<ClaimStatus>("DRAFT");
  const [externalRef, setExternalRef] = useState("");
  const [settlementRef, setSettlementRef] = useState("");
  const [approvedAmountMajor, setApprovedAmountMajor] = useState("");
  const [ownerUserId, setOwnerUserId] = useState("");

  const fetchClaim = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.claims.get(claimId);
      setClaim(result);
      setStatus(result.status);
      setExternalRef(result.external_claim_reference);
      setSettlementRef(result.settlement_reference);
      setApprovedAmountMajor(
        result.approved_amount_minor !== null
          ? String(result.approved_amount_minor / 100)
          : ""
      );
      setOwnerUserId(result.owner_user_id);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load claim.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [claimId]);

  useEffect(() => {
    if (claimId) fetchClaim();
  }, [claimId, fetchClaim]);

  const handleSave = async () => {
    if (!claim || saving) return;
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const approvedAmount =
        approvedAmountMajor.trim() === ""
          ? null
          : Math.round(Number(approvedAmountMajor) * 100);
      const updated = await financeService.claims.update(
        claim.id,
        {
          claimType: claim.claim_type,
          incidentId: claim.incident_id ?? undefined,
          assetId: claim.asset_id ?? undefined,
          shipmentId: claim.shipment_id ?? undefined,
          packageId: claim.package_id ?? undefined,
          policyReference: claim.policy_reference,
          amountReferenceMinor: claim.amount_reference_minor,
          currency: claim.currency,
          evidenceMediaIds: claim.evidence_media_ids,
          externalClaimReference: externalRef.trim() || undefined,
          settlementReference: settlementRef.trim() || undefined,
          approvedAmountMinor: approvedAmount,
          ownerUserId: ownerUserId.trim() || undefined,
          status,
        },
        claim.version
      );
      setClaim(updated);
      setSuccess("Claim updated.");
      setEditing(false);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to save claim.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  if (loading && !claim) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading claim…</p>
      </div>
    );
  }

  if (error && !claim) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load claim
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchClaim}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/claims")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to claims
          </button>
        </div>
      </div>
    );
  }

  if (!claim) return null;

  const statusTone = claimStatusTone(claim.status);
  const typeTone = claimTypeTone(claim.claim_type);
  const isFinal =
    claim.status === "SETTLED" ||
    claim.status === "CLOSED" ||
    claim.status === "CANCELLED";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/claims")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to claims"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Claim {claim.id}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {claimStatusLabel(claim.status)}
              </span>
              <span
                className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${typeTone.bg} ${typeTone.text}`}
              >
                {claimTypeLabel(claim.claim_type)}
              </span>
              <span className="text-sm text-ink-400">v{claim.version}</span>
            </div>
            {claim.policy_reference && (
              <div className="mt-1 font-mono text-sm text-ink-500">
                {claim.policy_reference}
              </div>
            )}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchClaim}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {!isFinal && (
            <>
              {editing ? (
                <>
                  <button
                    onClick={() => {
                      setEditing(false);
                      fetchClaim();
                    }}
                    className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleSave}
                    disabled={saving}
                    className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
                  >
                    {saving ? (
                      <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Save className="h-3.5 w-3.5" />
                    )}
                    Save
                  </button>
                </>
              ) : (
                <button
                  onClick={() => setEditing(true)}
                  className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-surface px-3.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
                >
                  Edit
                </button>
              )}
            </>
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
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Claim information
            </h3>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Claim type
                </div>
                <div className="text-sm font-medium text-ink-950">
                  {claimTypeLabel(claim.claim_type)}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Currency
                </div>
                <div className="font-mono text-sm font-medium text-ink-950">
                  {claim.currency}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Reference amount
                </div>
                <div className="text-sm font-medium text-ink-950">
                  {claim.amount_reference_minor !== null
                    ? formatMinor(claim.amount_reference_minor, claim.currency)
                    : "—"}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Approved amount
                </div>
                <div className="text-sm font-medium text-ink-950">
                  {claim.approved_amount_minor !== null
                    ? formatMinor(claim.approved_amount_minor, claim.currency)
                    : "—"}
                </div>
              </div>
              <div className="col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Policy reference
                </div>
                <div className="font-mono text-sm font-medium text-ink-950">
                  {claim.policy_reference || "—"}
                </div>
              </div>
            </div>
          </div>

          {(claim.incident_id ||
            claim.asset_id ||
            claim.shipment_id ||
            claim.package_id) && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Linked records
              </h3>
              <div className="grid grid-cols-2 gap-3">
                {claim.incident_id && (
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                      Incident
                    </div>
                    <div className="font-mono text-sm text-ink-950">
                      {claim.incident_id}
                    </div>
                  </div>
                )}
                {claim.asset_id && (
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                      Asset
                    </div>
                    <div className="font-mono text-sm text-ink-950">
                      {claim.asset_id}
                    </div>
                  </div>
                )}
                {claim.shipment_id && (
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                      Shipment
                    </div>
                    <div className="font-mono text-sm text-ink-950">
                      {claim.shipment_id}
                    </div>
                  </div>
                )}
                {claim.package_id && (
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                      Package
                    </div>
                    <div className="font-mono text-sm text-ink-950">
                      {claim.package_id}
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {claim.evidence_media_ids.length > 0 && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-[15px] font-semibold text-ink-950">
                  Evidence
                </h3>
                <span className="text-xs text-ink-400">
                  {claim.evidence_media_ids.length} file
                  {claim.evidence_media_ids.length === 1 ? "" : "s"}
                </span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {claim.evidence_media_ids.map((id) => (
                  <span
                    key={id}
                    className="rounded-full bg-surface-2 px-2.5 py-0.5 font-mono text-[11px] text-ink-700"
                  >
                    {id}
                  </span>
                ))}
              </div>
            </div>
          )}

          {editing && (
            <div className="rounded-2xl border border-primary/30 bg-primary-bg/20 p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Update claim
              </h3>
              <div className="grid grid-cols-2 gap-4">
                <div className="col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Status
                  </label>
                  <select
                    value={status}
                    onChange={(e) => setStatus(e.target.value as ClaimStatus)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {STATUS_OPTIONS.map((s) => (
                      <option key={s} value={s}>
                        {claimStatusLabel(s)}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    External reference
                  </label>
                  <input
                    value={externalRef}
                    onChange={(e) => setExternalRef(e.target.value)}
                    placeholder="External claim reference"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Settlement reference
                  </label>
                  <input
                    value={settlementRef}
                    onChange={(e) => setSettlementRef(e.target.value)}
                    placeholder="Settlement reference"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Approved amount
                  </label>
                  <input
                    value={approvedAmountMajor}
                    onChange={(e) => setApprovedAmountMajor(e.target.value)}
                    placeholder="0.00"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Owner user ID
                  </label>
                  <input
                    value={ownerUserId}
                    onChange={(e) => setOwnerUserId(e.target.value)}
                    placeholder="owner user id"
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Timeline
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">
                  {formatDateTime(claim.created_at)}
                </span>
              </div>
              {claim.submitted_at && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Submitted</span>
                  <span className="text-amber">
                    {formatDateTime(claim.submitted_at)}
                  </span>
                </div>
              )}
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">
                  {timeAgo(claim.updated_at)}
                </span>
              </div>
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Metadata
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Version</span>
                <span className="font-mono font-medium text-ink-950">
                  {claim.version}
                </span>
              </div>
              {claim.owner_user_id && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Owner</span>
                  <span className="font-mono text-ink-950">
                    {claim.owner_user_id}
                  </span>
                </div>
              )}
              {claim.external_claim_reference && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">External ref</span>
                  <span className="font-mono text-ink-950">
                    {claim.external_claim_reference}
                  </span>
                </div>
              )}
              {claim.settlement_reference && (
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Settlement ref</span>
                  <span className="font-mono text-ink-950">
                    {claim.settlement_reference}
                  </span>
                </div>
              )}
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Quick links
            </h3>
            <button
              onClick={() => router.push("/finance/claims")}
              className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2"
            >
              <span className="flex items-center gap-2">
                <FileCheck className="h-4 w-4" />
                All claims
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}