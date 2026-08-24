// src/app/(dashboard)/operations/marketplace/[offerId]/page.tsx

"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Package, Truck, MapPin, Clock, ArrowLeft } from "lucide-react";
import { OFFER_DETAIL } from "@/lib/mock-marketplace-data";

export default function OfferDetailPage() {
  const params = useParams();
  const offerId = params.offerId as string;
  const offer = OFFER_DETAIL;

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/marketplace"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-white text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>{offer.service} opportunity</h1>
            <p className="font-mono text-sm text-ink-500">{offerId}</p>
          </div>
        </div>
      </div>

      {/* ── Details Card ── */}
      <div className="rounded-lg border border-line bg-white p-5">
        <dl className="grid grid-cols-2 gap-y-4">
          <Field label="Pickup area">{offer.pickup}</Field>
          <Field label="Service area">{offer.dropoff}</Field>
          <Field label="Vehicle required">{offer.vehicle}</Field>
          <Field label="Distance">{offer.distance}</Field>
          <Field label="Load">{offer.load}</Field>
          <Field label="Branch">{offer.branch}</Field>
          <Field label="Service window">{offer.window}</Field>
          <Field label="Response deadline">
            <span className="rounded-full bg-amber-bg px-2.5 py-1 text-xs font-bold text-amber">
              {offer.deadline} left
            </span>
          </Field>
        </dl>

        {/* ── Instructions ── */}
        <div className="mt-5 border-t border-line pt-4">
          <span className="text-xs font-medium uppercase tracking-wide text-ink-500">
            Instructions
          </span>
          <p className="mt-1.5">{offer.instructions}</p>
        </div>
      </div>

      {/* ── Actions ── */}
      <div className="flex flex-col gap-2.5 sm:flex-row">
        <Link
          href={`/operations/marketplace/${offerId}/candidates`}
          className="flex-1 rounded-lg border border-line bg-white px-4 py-2.5 text-center text-sm font-semibold text-ink-800 transition-colors hover:bg-surface-2"
        >
          Compare eligible candidates
        </Link>
        <Link
          href={`/operations/marketplace/${offerId}/assign`}
          className="flex-1 rounded-lg bg-primary px-4 py-2.5 text-center text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          Create assignment
        </Link>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </dt>
      <dd className="mt-1">{children}</dd>
    </div>
  );
}