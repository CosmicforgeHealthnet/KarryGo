'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  ArrowLeft,
  Bike,
  Car,
  Truck,
  Package,
  FileText,
  UserPlus,
  Archive,
  Trash2,
  AlertCircle,
} from 'lucide-react';
import { assetService, newAssetIdempotencyKey } from '@/lib/services/assetService';
import {
  assetDisplayName,
  assetTypeLabel,
  formatCapacity,
  formatDate,
  formatDateTime,
  formatOdometer,
  formatOperatingHours,
  lifecycleStatusLabel,
  lifecycleStatusTone,
  operationalStatusLabel,
  operationalStatusTone,
  ownershipTypeLabel,
} from '@/lib/utils/assetLabels';
import type { Asset } from '@/lib/types/asset';

const TYPE_ICON: Record<string, typeof Truck> = {
  MOTORCYCLE: Bike,
  BIKE: Bike,
  VAN: Car,
  CAR: Car,
  TRUCK: Truck,
  OTHER: Package,
};

export default function AssetDetailPage() {
  const params = useParams();
  const router = useRouter();
  const assetId = params.assetId as string;

  const [asset, setAsset] = useState<Asset | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [action, setAction] = useState<'archive' | 'retire' | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!assetId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    assetService
      .get(assetId)
      .then((data) => {
        if (!cancelled) setAsset(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load asset.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [assetId]);

  async function runAction() {
    if (!asset || !action) return;
    setBusy(true);
    setError(null);
    try {
      if (action === 'archive') {
        await assetService.archive(asset.id, asset.version);
      } else {
        await assetService.retire(asset.id, asset.version);
      }
      const refreshed = await assetService.get(asset.id);
      setAsset(refreshed);
      setAction(null);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Action failed.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading asset…
      </div>
    );
  }

  if (error && !asset) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/assets"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to assets
        </Link>
      </div>
    );
  }

  if (!asset) return null;

  const opTone = operationalStatusTone(asset.operationalStatus);
  const lifeTone = lifecycleStatusTone(asset.lifecycleStatus);
  const Icon = TYPE_ICON[asset.assetType] ?? Truck;
  const name = assetDisplayName(asset);
  const canArchive =
    asset.lifecycleStatus === 'ACTIVE' &&
    asset.operationalStatus !== 'ASSIGNED';
  const canRetire =
    asset.lifecycleStatus === 'ACTIVE' &&
    asset.operationalStatus !== 'ASSIGNED';

  const fields: { label: string; value: React.ReactNode }[] = [
    { label: 'Asset code', value: asset.assetCode },
    { label: 'Type', value: assetTypeLabel(asset.assetType) },
    { label: 'Make / model', value: name },
    { label: 'Year', value: asset.year ?? '—' },
    { label: 'Registration', value: asset.registrationReference || '—' },
    { label: 'Ownership', value: ownershipTypeLabel(asset.ownershipType) },
    { label: 'Capacity', value: formatCapacity(asset.capacity, asset.capacityUnit) },
    { label: 'Odometer', value: formatOdometer(asset.odometer) },
    { label: 'Operating hours', value: formatOperatingHours(asset.operatingHours) },
    {
      label: 'Branch',
      value: <span className="font-mono text-xs">{asset.branchId}</span>,
    },
    {
      label: 'Platform vehicle',
      value: asset.platformVehicleId ? (
        <span className="font-mono text-xs">{asset.platformVehicleId}</span>
      ) : (
        <span className="text-ink-400">Not linked</span>
      ),
    },
    { label: 'Created', value: formatDateTime(asset.createdAt) },
    { label: 'Updated', value: formatDateTime(asset.updatedAt) },
    {
      label: 'Archived',
      value: asset.archivedAt ? formatDateTime(asset.archivedAt) : '—',
    },
    {
      label: 'Retired',
      value: asset.retiredAt ? formatDateTime(asset.retiredAt) : '—',
    },
    {
      label: 'Version',
      value: <span className="font-mono text-xs">{asset.version}</span>,
    },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            <Link
              href="/fleet/assets"
              className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
            >
              <ArrowLeft className="h-4 w-4" />
            </Link>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2.5">
                <h1 className="truncate">{asset.assetCode}</h1>
                <span
                  className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${opTone.bg} ${opTone.text}`}
                >
                  <span className={`h-1.5 w-1.5 rounded-full ${opTone.dot}`} />
                  {operationalStatusLabel(asset.operationalStatus)}
                </span>
                <span
                  className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-medium ${lifeTone.bg} ${lifeTone.text}`}
                >
                  <span className={`h-1 w-1 rounded-full ${lifeTone.dot}`} />
                  {lifecycleStatusLabel(asset.lifecycleStatus)}
                </span>
              </div>
              <p className="mt-0.5 text-sm text-ink-500 truncate">
                <Icon className="mr-1 inline h-3.5 w-3.5" />
                {name}
              </p>
            </div>
          </div>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <TabLink
          href={`/fleet/assets/${asset.id}/documents`}
          icon={<FileText className="h-4 w-4" />}
          label="Documents"
          description="Attach and view asset documents."
        />
        <TabLink
          href={`/fleet/assets/${asset.id}/assignments`}
          icon={<UserPlus className="h-4 w-4" />}
          label="Assignments"
          description="Assign this asset to a provider."
        />
        <TabLink
          href={`/fleet/assets/${asset.id}/compliance`}
          icon={<AlertCircle className="h-4 w-4" />}
          label="Compliance"
          description="Compliance status and restrictions."
        />
      </div>

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 text-sm font-semibold text-ink-950">Details</h3>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
          {fields.map((f) => (
            <div
              key={f.label}
              className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0"
            >
              <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {f.label}
              </dt>
              <dd className="text-sm text-ink-950 text-right truncate">
                {f.value}
              </dd>
            </div>
          ))}
        </dl>
      </div>

      {(canArchive || canRetire) && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 text-sm font-semibold text-ink-950">Actions</h3>
          <div className="flex flex-wrap gap-2.5">
            {canArchive && (
              <button
                onClick={() => setAction('archive')}
                className="flex items-center gap-2 rounded-md border border-amber/30 bg-amber-bg px-3.5 py-1.5 text-xs font-semibold text-amber transition-colors hover:bg-amber-bg/70"
              >
                <Archive className="h-3.5 w-3.5" />
                Archive
              </button>
            )}
            {canRetire && (
              <button
                onClick={() => setAction('retire')}
                className="flex items-center gap-2 rounded-md border border-red/30 bg-red-bg px-3.5 py-1.5 text-xs font-semibold text-red transition-colors hover:bg-red-bg/70"
              >
                <Trash2 className="h-3.5 w-3.5" />
                Retire
              </button>
            )}
          </div>
        </div>
      )}

      {action && (
        <div className="rounded-lg border border-amber/30 bg-amber-bg/20 p-4">
          <div className="flex items-start gap-3">
            <AlertCircle className="h-4 w-4 flex-shrink-0 text-amber mt-0.5" />
            <div className="flex-1">
              <p className="text-sm font-semibold text-ink-950">
                {action === 'archive'
                  ? 'Archive this asset?'
                  : 'Retire this asset?'}
              </p>
              <p className="mt-0.5 text-xs text-ink-600">
                {action === 'archive'
                  ? 'The asset will be hidden from active views. It cannot be assigned while archived.'
                  : 'The asset will be permanently retired. This is the final lifecycle state.'}
              </p>
              <div className="mt-3 flex gap-2">
                <button
                  onClick={runAction}
                  disabled={busy}
                  className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
                >
                  {busy ? 'Working…' : 'Confirm'}
                </button>
                <button
                  onClick={() => setAction(null)}
                  disabled={busy}
                  className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function TabLink({
  href,
  icon,
  label,
  description,
}: {
  href: string;
  icon: React.ReactNode;
  label: string;
  description: string;
}) {
  return (
    <Link
      href={href}
      className="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400 hover:bg-surface-2"
    >
      <div className="flex items-center gap-2.5">
        <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
          {icon}
        </div>
        <span className="text-sm font-semibold text-ink-950">{label}</span>
      </div>
      <p className="mt-2 text-xs text-ink-500">{description}</p>
    </Link>
  );
}