'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  Briefcase,
  CalendarDays,
  Clock,
  FileText,
  User,
} from 'lucide-react';
import { workforceService } from '@/lib/services/workforceService';
import {
  formatDate,
  formatDateTime,
  profileStatusLabel,
  profileStatusTone,
  relationshipTypeLabel,
} from '@/lib/utils/workforceLabels';
import type {
  Leave,
  PerformanceSnapshot,
  Profile,
  ProviderQualification,
} from '@/lib/types/workforce';

export default function ProfileDetailPage() {
  const params = useParams();
  const profileId = params.profileId as string;

  const [profile, setProfile] = useState<Profile | null>(null);
  const [leave, setLeave] = useState<Leave[]>([]);
  const [qualifications, setQualifications] = useState<ProviderQualification[]>([]);
  const [performance, setPerformance] = useState<PerformanceSnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!profileId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    workforceService
      .getProfile(profileId)
      .then(async (p) => {
        if (cancelled) return;
        setProfile(p);

        const [allLeave, quals, perf] = await Promise.allSettled([
          workforceService.listLeave({ limit: 100 }),
          workforceService.listProviderQualifications(p.membershipId),
          workforceService.listPerformance(p.membershipId),
        ]);

        if (cancelled) return;

        if (quals.status === 'fulfilled') setQualifications(quals.value);
        if (perf.status === 'fulfilled') setPerformance(perf.value);

        if (allLeave.status === 'fulfilled') {
          setLeave(
            allLeave.value.filter((l) => l.membershipId === p.membershipId),
          );
        }
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load profile.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [profileId]);

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading profile…
      </div>
    );
  }

  if (error && !profile) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/workforce/shifts"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back
        </Link>
      </div>
    );
  }

  if (!profile) return null;

  const tone = profileStatusTone(profile.status);

  const rows: { label: string; value: React.ReactNode }[] = [
    {
      label: 'Profile ID',
      value: <span className="font-mono text-xs">{profile.id}</span>,
    },
    {
      label: 'Membership',
      value: (
        <Link
          href={`/providers/${profile.membershipId}`}
          className="font-mono text-xs text-primary hover:underline"
        >
          {profile.membershipId}
        </Link>
      ),
    },
    {
      label: 'Branch',
      value: <span className="font-mono text-xs">{profile.branchId}</span>,
    },
    { label: 'Role', value: profile.role || '—' },
    {
      label: 'Relationship',
      value: relationshipTypeLabel(profile.relationshipType),
    },
    { label: 'Employee number', value: profile.employeeNumber || '—' },
    {
      label: 'Supervisor',
      value: profile.supervisorMembershipId ? (
        <span className="font-mono text-xs">
          {profile.supervisorMembershipId}
        </span>
      ) : (
        '—'
      ),
    },
    { label: 'Start date', value: formatDate(profile.startDate) },
    { label: 'End date', value: formatDate(profile.endDate) },
    { label: 'Created', value: formatDateTime(profile.createdAt) },
    { label: 'Updated', value: formatDateTime(profile.updatedAt) },
    {
      label: 'Version',
      value: <span className="font-mono text-xs">{profile.version}</span>,
    },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/workforce/shifts"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">
                {profile.role || 'Workforce profile'}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                {profileStatusLabel(profile.status)}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {profile.id}
            </p>
          </div>
        </div>
        <Link
          href={`/workforce/${profile.id}/edit`}
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          Edit profile
        </Link>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
          <User className="h-4 w-4" />
          Profile
        </h3>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
          {rows.map((r) => (
            <div
              key={r.label}
              className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0"
            >
              <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {r.label}
              </dt>
              <dd className="text-sm text-ink-950 text-right truncate">
                {r.value}
              </dd>
            </div>
          ))}
        </dl>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <InfoCard
          href={`/workforce/leave?membershipId=${profile.membershipId}`}
          icon={<CalendarDays className="h-4 w-4" />}
          label="Leave"
          count={leave.length}
          description="Leave records for this membership."
        />
        <InfoCard
          href={`/workforce/qualifications/providers?membershipId=${profile.membershipId}`}
          icon={<FileText className="h-4 w-4" />}
          label="Qualifications"
          count={qualifications.length}
          description="Provider qualifications on file."
        />
        <InfoCard
          href={`/workforce/performance/${profile.membershipId}`}
          icon={<Briefcase className="h-4 w-4" />}
          label="Performance"
          count={performance.length}
          description="Recorded performance snapshots."
        />
      </div>

      {qualifications.length > 0 && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <FileText className="h-4 w-4" />
            Recent qualifications
          </h3>
          <div className="space-y-2">
            {qualifications.slice(0, 5).map((q) => (
              <div
                key={q.id}
                className="flex items-center justify-between rounded-lg border border-line p-3 text-sm"
              >
                <span className="font-mono text-xs text-ink-500">
                  {q.qualificationId}
                </span>
                <span className="text-xs text-ink-500">
                  {q.status} · expires {formatDate(q.expiresAt)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {performance.length > 0 && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <Clock className="h-4 w-4" />
            Recent performance
          </h3>
          <div className="space-y-2">
            {performance.slice(0, 5).map((p) => (
              <div
                key={p.id}
                className="flex items-center justify-between rounded-lg border border-line p-3 text-sm"
              >
                <span className="text-xs text-ink-500">
                  {formatDate(p.periodStart)} – {formatDate(p.periodEnd)}
                </span>
                <span className="text-xs text-ink-500">
                  {p.generationStatus}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function InfoCard({
  href,
  icon,
  label,
  count,
  description,
}: {
  href: string;
  icon: React.ReactNode;
  label: string;
  count: number;
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
        <span className="ml-auto text-xs font-medium text-ink-500">
          {count}
        </span>
      </div>
      <p className="mt-2 text-xs text-ink-500">{description}</p>
    </Link>
  );
}