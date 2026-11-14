'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ArrowLeft, MapPin } from 'lucide-react';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { describeApiError, type ApiErrorInfo } from '@/lib/api/errors';
import { controlTowerService } from '@/lib/services/controlTowerService';
import { tripService } from '@/lib/services/operationsService';
import type { TrackingProjection } from '@/lib/types/controlTower';
import type { CorporateTripDetail } from '@/lib/types/operations';

const humanize = (value?: string) =>
  value ? value.replace(/_/g, ' ').toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : '—';
const formatWhen = (iso?: string) =>
  iso ? new Date(iso).toLocaleString(undefined, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—';

// One trip: lifecycle (status, history, assignment and canonical references)
// from the marketplace trip projection, plus the last live position from
// tracking. Older links that carry a canonical trip reference still resolve
// through tracking alone.
export default function TripDetailPage() {
  const params = useParams();
  const tripRef = decodeURIComponent(params.tripRef as string);
  const [detail, setDetail] = useState<CorporateTripDetail | null>(null);
  const [position, setPosition] = useState<TrackingProjection | null>(null);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      let lifecycle: CorporateTripDetail | null = null;
      try {
        lifecycle = await tripService.get(tripRef);
      } catch (err) {
        const status = (err as { response?: { status?: number } })?.response?.status;
        if (status !== 404 && !cancelled) setError(describeApiError(err, 'Failed to load the trip.'));
      }
      const trackingRef = lifecycle?.trip.platformTripId || tripRef;
      const live = await controlTowerService.getTrip(trackingRef).catch(() => null);
      if (cancelled) return;
      setDetail(lifecycle);
      setPosition(live);
      if (!lifecycle && !live) setError((e) => e ?? { message: 'This trip was not found.' });
      setLoading(false);
    })();
    return () => {
      cancelled = true;
    };
  }, [tripRef]);

  const trip = detail?.trip;
  const fields: [string, string][] = [];
  if (trip) {
    fields.push(
      ['Status', humanize(trip.tripStatus)],
      ['Service', humanize(trip.serviceType || trip.sourceService.replace(/^driver-|-service$/g, ''))],
      ['Platform trip', trip.platformTripId || '—'],
      ['Booking', trip.platformBookingId || '—'],
      ['Request', trip.platformRequestId || '—'],
      ['Assignment', trip.assignmentId || '—'],
      ['Provider', trip.executingProviderId || '—'],
      ['Membership', trip.organizationMembershipId || '—'],
      ['Data', humanize(trip.freshnessState)],
      ['Last event', formatWhen(trip.lastEventAt)],
      ['Completed', formatWhen(trip.completedAt)],
      ['Cancelled', formatWhen(trip.cancelledAt)],
    );
  } else if (position) {
    fields.push(
      ['Status', humanize(position.operationalStatus)],
      ['Service', humanize(position.sourceService.replace(/^driver-|-service$/g, ''))],
      ['Provider', position.providerId || '—'],
      ['Assignment', position.assignmentId || '—'],
    );
  }
  if (position) {
    fields.push(
      ['Vehicle', position.vehicleOrTruckReference || '—'],
      ['Signal', humanize(position.freshnessStatus)],
      ['Last reported', formatWhen(position.sourceTimestamp)],
      ['Speed', position.speed != null ? `${Math.round(position.speed)} km/h` : '—'],
    );
  }

  const title = trip?.platformTripId || tripRef;

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <Link href="/operations/trips" className="hover:text-primary">Trips</Link>
            <span>/</span>
            <span className="font-mono text-ink-700">{title}</span>
          </div>
          <h1 className="mt-1">Trip {title}</h1>
        </div>
        <Link
          href="/operations/trips"
          className="flex h-9 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-xs font-medium text-ink-600 hover:bg-surface-2"
        >
          <ArrowLeft className="h-3.5 w-3.5" /> Back
        </Link>
      </div>

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">Loading trip…</div>
      ) : (
        <>
          <ApiErrorNotice error={error} />
          {fields.length > 0 && (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              {fields.map(([label, value]) => (
                <div key={label} className="rounded-lg border border-line bg-surface p-4">
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">{label}</div>
                  <div className="mt-1 break-all text-sm font-medium text-ink-950">{value}</div>
                </div>
              ))}
            </div>
          )}
          {position?.latitude != null && position?.longitude != null ? (
            <div className="flex items-center gap-2 rounded-lg border border-line bg-surface p-4 text-sm text-ink-700">
              <MapPin className="h-4 w-4 text-primary" />
              Last position {position.latitude.toFixed(5)}, {position.longitude.toFixed(5)}
            </div>
          ) : trip ? (
            <div className="rounded-lg border border-line bg-surface p-4 text-sm text-ink-500">No live position reported for this trip.</div>
          ) : null}

          {detail?.deliveryOutcome && (
            <div className="rounded-lg border border-line bg-surface p-4 text-sm text-ink-700">
              <div className="mb-2 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Delivery outcome</div>
              {detail.deliveryOutcome.recipientConfirmedAt
                ? `Recipient confirmed ${formatWhen(detail.deliveryOutcome.recipientConfirmedAt)} (${humanize(detail.deliveryOutcome.confirmationMethod)})`
                : detail.deliveryOutcome.failedAt
                  ? `Failed ${formatWhen(detail.deliveryOutcome.failedAt)}: ${humanize(detail.deliveryOutcome.failureReasonCode)}${detail.deliveryOutcome.retryEligible ? ' · retry possible' : ''}`
                  : 'Not yet delivered.'}
            </div>
          )}

          {detail && detail.history.length > 0 && (
            <div className="rounded-lg border border-line bg-surface p-4">
              <div className="mb-3 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Status history</div>
              <ol className="space-y-2">
                {detail.history.map((h) => (
                  <li key={h.id} className="flex items-center justify-between gap-3 text-sm">
                    <span className="text-ink-950">
                      {humanize(h.fromStatus)} → {humanize(h.toStatus)}
                    </span>
                    <span className="text-xs text-ink-500">{formatWhen(h.occurredAt)}</span>
                  </li>
                ))}
              </ol>
            </div>
          )}
        </>
      )}
    </div>
  );
}
