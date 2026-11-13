'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ArrowLeft, MapPin } from 'lucide-react';
import { controlTowerService } from '@/lib/services/controlTowerService';
import type { TrackingProjection } from '@/lib/types/controlTower';

const humanize = (value?: string) =>
  value ? value.replace(/_/g, ' ').toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : '—';
const formatWhen = (iso?: string) =>
  iso ? new Date(iso).toLocaleString(undefined, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—';

export default function TripDetailPage() {
  const params = useParams();
  const tripRef = decodeURIComponent(params.tripRef as string);
  const [trip, setTrip] = useState<TrackingProjection | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    controlTowerService
      .getTrip(tripRef)
      .then((data) => {
        if (!cancelled) setTrip(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err?.response?.status === 404 ? 'This trip was not found.' : (err?.response?.data?.error?.message ?? 'Failed to load the trip.'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tripRef]);

  const fields: [string, string][] = trip
    ? [
        ['Status', humanize(trip.operationalStatus)],
        ['Service', humanize(trip.sourceService.replace(/^driver-|-service$/g, ''))],
        ['Vehicle', trip.vehicleOrTruckReference || '—'],
        ['Provider', trip.providerId || '—'],
        ['Assignment', trip.assignmentId || '—'],
        ['Route plan', trip.routePlanId || '—'],
        ['Signal', humanize(trip.freshnessStatus)],
        ['Last reported', formatWhen(trip.sourceTimestamp)],
        ['Speed', trip.speed != null ? `${Math.round(trip.speed)} km/h` : '—'],
      ]
    : [];

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <Link href="/operations/trips" className="hover:text-primary">Trips</Link>
            <span>/</span>
            <span className="font-mono text-ink-700">{tripRef}</span>
          </div>
          <h1 className="mt-1">Trip {tripRef}</h1>
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
      ) : error ? (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-3 text-sm text-red">{error}</div>
      ) : trip ? (
        <>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {fields.map(([label, value]) => (
              <div key={label} className="rounded-lg border border-line bg-surface p-4">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">{label}</div>
                <div className="mt-1 break-all text-sm font-medium text-ink-950">{value}</div>
              </div>
            ))}
          </div>
          {trip.latitude != null && trip.longitude != null && (
            <div className="flex items-center gap-2 rounded-lg border border-line bg-surface p-4 text-sm text-ink-700">
              <MapPin className="h-4 w-4 text-primary" />
              Last position {trip.latitude.toFixed(5)}, {trip.longitude.toFixed(5)}
            </div>
          )}
        </>
      ) : null}
    </div>
  );
}
