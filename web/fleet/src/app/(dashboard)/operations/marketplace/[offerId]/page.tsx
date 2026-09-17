// src/app/(dashboard)/operations/marketplace/[offerId]/page.tsx

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  ArrowRight,
  Package,
  Truck,
  Clock3,
  MapPin,
  CheckCircle2,
  XCircle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { marketplaceService } from "@/lib/services/marketplaceService";
import type { CorporateOfferDetail } from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Presentation helpers
// ---------------------------------------------------------------------------

function serviceLabel(sourceService: string): string {
  switch (sourceService) {
    case "DISPATCH":
      return "Dispatch";
    case "TAXI":
      return "Taxi";
    case "HAULING":
      return "Hauling";
    default:
      return sourceService || "—";
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "OFFERED":
      return "Open";
    case "RESERVATION_PENDING":
      return "Reserving";
    case "ORGANIZATION_ACCEPTED":
      return "Accepted";
    case "ASSIGNMENT_PENDING":
      return "Assigning";
    case "PROVIDER_ACCEPTED":
      return "Provider accepted";
    case "EXECUTION_REQUESTED":
    case "EXECUTION_CREATED":
      return "Starting";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "ORGANIZATION_DECLINED":
      return "Declined";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    default:
      return status || "—";
  }
}

function summaryLabel(summary: Record<string, unknown> | undefined): string {
  if (!summary) return "—";
  const candidateKeys = ["label", "name", "reference", "address"];
  for (const key of candidateKeys) {
    const value = summary[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }
  return "—";
}

function formatDeadline(expiresAtIso: string): {
  text: string;
  minutes: number;
} {
  const expires = new Date(expiresAtIso).getTime();
  if (!Number.isFinite(expires)) {
    return { text: "—", minutes: Number.MAX_SAFE_INTEGER };
  }
  const minutes = Math.max(0, Math.floor((expires - Date.now()) / 60_000));
  if (minutes <= 0) return { text: "Expired", minutes: 0 };
  if (minutes < 60) return { text: `${minutes} min left`, minutes };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { text: `${hours} h left`, minutes };
  const days = Math.floor(hours / 24);
  return { text: `${days} d left`, minutes };
}

function formatTimestamp(iso: string): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  return new Date(t).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function OfferDetailPage() {
  const params = useParams();
  const offerId = typeof params.offerId === "string" ? params.offerId : "";

  const [detail, setDetail] = useState<CorporateOfferDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!offerId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    marketplaceService
      .getOffer(offerId)
      .then((data) => {
        if (!cancelled) setDetail(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Offer not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load offer.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [offerId]);

  // Loading skeleton — keeps layout stable while data arrives.
  if (loading) {
    return (
      <div className="mx-auto max-w-3xl space-y-5">
        <div className="flex items-center gap-3">
          <div className="h-9 w-9 rounded-lg border border-line bg-surface" />
          <div className="flex-1">
            <div className="h-5 w-40 rounded bg-surface-2" />
            <div className="mt-1.5 h-3 w-24 rounded bg-surface-2" />
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-5">
          <div className="grid grid-cols-2 gap-y-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i}>
                <div className="h-3 w-20 rounded bg-surface-2" />
                <div className="mt-1.5 h-4 w-32 rounded bg-surface-2" />
              </div>
            ))}
          </div>
        </div>
      </div>
    );
  }

  if (error || !detail) {
    return (
      <div className="mx-auto max-w-3xl space-y-5">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/marketplace"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Offer</h1>
            <p className="font-mono text-sm text-ink-500">{offerId}</p>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title={error ?? "Offer not found"}
            description="This offer may have expired, been cancelled, or been accepted by another organization."
            actionHref="/operations/marketplace"
            actionLabel="Back to marketplace"
          />
        </div>
      </div>
    );
  }

  const { offer, acceptance } = detail;
  const deadline = formatDeadline(offer.expiresAt);
  const isOpen = offer.status === "OFFERED";
  const isAccepted = offer.status === "ORGANIZATION_ACCEPTED";
  const pickup = summaryLabel(offer.pickupSummary);
  const dropoff = summaryLabel(offer.dropoffSummary);

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/marketplace"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>{serviceLabel(offer.sourceService)} opportunity</h1>
            <p className="font-mono text-sm text-ink-500">{offer.offerId}</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <span
            className={`rounded-full px-3 py-1 text-xs font-semibold ${
              isOpen
                ? "bg-primary-bg text-primary"
                : isAccepted
                ? "bg-green-bg text-green"
                : "bg-surface-2 text-ink-600"
            }`}
          >
            {statusLabel(offer.status)}
          </span>
          {isOpen && (
            <span
              className={`rounded-full px-3 py-1 text-xs font-semibold ${
                deadline.minutes <= 15
                  ? "bg-red-bg text-red"
                  : deadline.minutes <= 30
                  ? "bg-amber-bg text-amber"
                  : "bg-surface-2 text-ink-600"
              }`}
            >
              <Clock3 className="mr-1 inline-block h-3 w-3 align-[-2px]" />
              {deadline.text}
            </span>
          )}
        </div>
      </div>

      {/* ── Details card ── */}
      <div className="rounded-lg border border-line bg-surface p-5">
        <dl className="grid grid-cols-2 gap-y-4">
          <Field label="Pickup area">{pickup}</Field>
          <Field label="Service area">{dropoff}</Field>
          <Field label="Vehicle required">{offer.serviceType || "—"}</Field>
          <Field label="Source service">{serviceLabel(offer.sourceService)}</Field>
          <Field label="Offered at">{formatTimestamp(offer.offeredAt)}</Field>
          <Field label="Response deadline">
            {isOpen ? (
              <span
                className={`rounded-full px-2.5 py-1 text-xs font-bold ${
                  deadline.minutes <= 15
                    ? "bg-red-bg text-red"
                    : deadline.minutes <= 30
                    ? "bg-amber-bg text-amber"
                    : "bg-surface-2 text-ink-700"
                }`}
              >
                {deadline.text}
              </span>
            ) : (
              formatTimestamp(offer.expiresAt)
            )}
          </Field>
        </dl>

        {/* Route diagram — only when we have both ends. */}
        {(pickup !== "—" || dropoff !== "—") && (
          <div className="mt-5 border-t border-line pt-4">
            <span className="text-xs font-medium uppercase tracking-wide text-ink-500">
              Route
            </span>
            <div className="mt-2 flex items-center gap-2 text-sm">
              <MapPin className="h-4 w-4 flex-shrink-0 text-primary" />
              <span className="font-medium text-ink-950">{pickup}</span>
              <ArrowRight className="h-3.5 w-3.5 flex-shrink-0 text-ink-400" />
              <MapPin className="h-4 w-4 flex-shrink-0 text-amber" />
              <span className="font-medium text-ink-950">{dropoff}</span>
            </div>
          </div>
        )}

        {/* Acceptance record — only when the org has accepted. */}
        {acceptance && (
          <div className="mt-5 border-t border-line pt-4">
            <span className="text-xs font-medium uppercase tracking-wide text-ink-500">
              Acceptance
            </span>
            <div className="mt-2 flex flex-wrap items-center gap-3 text-sm">
              <span className="inline-flex items-center gap-1.5 text-green">
                <CheckCircle2 className="h-4 w-4" />
                <span className="font-semibold">
                  Accepted by {acceptance.acceptanceSource === "PROVIDER_DIRECT" ? "provider" : "dispatcher"}
                </span>
              </span>
              <span className="text-ink-500">
                {formatTimestamp(acceptance.acceptedAt)}
              </span>
              {acceptance.sourceReservationId && (
                <span className="font-mono text-xs text-ink-400">
                Reservation {acceptance.sourceReservationId}
                </span>
              )}
            </div>
          </div>
        )}
      </div>

      {/* ── Actions ── */}
      <div className="flex flex-col gap-2.5 sm:flex-row">
        {isOpen && (
          <Link
            href={`/operations/marketplace/${offer.offerId}/candidates`}
            className="flex-1 rounded-lg bg-primary px-4 py-2.5 text-center text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            Compare eligible candidates
          </Link>
        )}
        {!isOpen && (
          <Link
            href={`/operations/assignments?offerId=${offer.offerId}`}
            className="flex-1 rounded-lg bg-primary px-4 py-2.5 text-center text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            View related assignments
          </Link>
        )}
        <Link
          href="/operations/marketplace"
          className="flex-1 rounded-lg border border-line bg-surface px-4 py-2.5 text-center text-sm font-semibold text-ink-800 transition-colors hover:bg-surface-2"
        >
          Back to marketplace
        </Link>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Small local component
// ---------------------------------------------------------------------------

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </dt>
      <dd className="mt-1 text-ink-950">{children}</dd>
    </div>
  );
}