'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { AlertCircle, ArrowLeft, Save } from 'lucide-react';
import { workforceService } from '@/lib/services/workforceService';
import type { Profile } from '@/lib/types/workforce';

const PROFILE_STATUSES = ['ACTIVE', 'SUSPENDED', 'ENDED', 'ARCHIVED'] as const;

export default function EditProfilePage() {
  const params = useParams();
  const router = useRouter();
  const profileId = params.profileId as string;

  const [profile, setProfile] = useState<Profile | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] = useState({
    role: '',
    supervisorMembershipId: '',
    status: '',
  });

  useEffect(() => {
    if (!profileId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    workforceService
      .getProfile(profileId)
      .then((p) => {
        if (cancelled) return;
        setProfile(p);
        setForm({
          role: p.role ?? '',
          supervisorMembershipId: p.supervisorMembershipId ?? '',
          status: p.status ?? '',
        });
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

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!profile) return;
    setSubmitting(true);
    setError(null);
    try {
      await workforceService.updateProfile(profile.id, {
        expectedVersion: profile.version,
        role: form.role.trim() || undefined,
        supervisorMembershipId: form.supervisorMembershipId.trim() || undefined,
        status: form.status || undefined,
      });
      router.push(`/workforce/${profile.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to update profile.',
      );
    } finally {
      setSubmitting(false);
    }
  }

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

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/workforce/${profile.id}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Edit profile</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">
            {profile.id}
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label="Role">
          <input
            value={form.role}
            onChange={(e) => setForm({ ...form, role: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Supervisor membership ID">
          <input
            value={form.supervisorMembershipId}
            onChange={(e) =>
              setForm({ ...form, supervisorMembershipId: e.target.value })
            }
            placeholder="pmb_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Status">
          <select
            value={form.status}
            onChange={(e) => setForm({ ...form, status: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          >
            <option value="">Unchanged</option>
            {PROFILE_STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </Field>

        <div className="flex items-start gap-2 rounded-lg border border-amber/30 bg-amber-bg/20 px-3 py-2 text-xs text-amber">
          <AlertCircle className="h-3.5 w-3.5 flex-shrink-0 mt-0.5" />
          <p>
            Status changes follow the workforce state machine. Some
            transitions may be refused by the backend.
          </p>
        </div>

        <div className="flex gap-2">
          <button
            type="submit"
            disabled={submitting}
            className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Save className="h-4 w-4" />
            {submitting ? 'Saving…' : 'Save changes'}
          </button>
          <Link
            href={`/workforce/${profile.id}`}
            className="flex h-10 items-center gap-2 rounded-lg border border-line bg-surface px-4 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Cancel
          </Link>
        </div>
      </form>
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </label>
      {children}
    </div>
  );
}