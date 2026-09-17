'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { AlertTriangle, ArrowLeft, Link2, UserPlus } from 'lucide-react';
import {
  newProviderIdempotencyKey,
  providerService,
} from '@/lib/services/providerService';
import type { CreateProviderResult, ServiceCategory } from '@/lib/types/provider';

type Step = 'form' | 'resolving' | 'success' | 'conflict' | 'already-managed' | 'error';

const SERVICE_CATEGORIES: ServiceCategory[] = ['DISPATCH', 'TAXI', 'HAULING'];

const SERVICE_CATEGORY_LABEL: Record<ServiceCategory, string> = {
  DISPATCH: 'Dispatch',
  TAXI: 'Taxi',
  HAULING: 'Hauling',
};

export default function AddProviderPage() {
  const router = useRouter();

  const [step, setStep] = useState<Step>('form');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [result, setResult] = useState<CreateProviderResult | null>(null);

  const [form, setForm] = useState({
    legalName: '',
    email: '',
    phone: '',
    serviceCategory: 'DISPATCH' as ServiceCategory,
    homeBranchId: '',
    relationshipType: '',
    employmentType: '',
    employeeNumber: '',
  });

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setStep('resolving');
    setErrorMessage(null);

    try {
      const idempotencyKey = newProviderIdempotencyKey();

      const created = await providerService.create(
        {
          legalName: form.legalName.trim(),
          email: form.email.trim(),
          phone: form.phone.trim(),
          serviceCategory: form.serviceCategory,
          homeBranchId: form.homeBranchId.trim() || undefined,
          relationshipType: form.relationshipType
            ? (form.relationshipType as any)
            : undefined,
          employmentType: form.employmentType.trim() || undefined,
          employeeNumber: form.employeeNumber.trim() || undefined,
        },
        idempotencyKey,
      );

      setResult(created);
      setStep('success');
    } catch (err: any) {
      const code = err?.response?.data?.error?.code;
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        'Failed to create provider.';

      if (code === 'provider_identity_conflict') {
        setStep('conflict');
        setErrorMessage(message);
      } else if (code === 'provider_already_managed') {
        setStep('already-managed');
        setErrorMessage(message);
      } else {
        setStep('error');
        setErrorMessage(message);
      }
    }
  }

  function reset() {
    setStep('form');
    setResult(null);
    setErrorMessage(null);
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-2">
        <Link
          href="/providers"
          className="flex h-8 w-8 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          aria-label="Back to providers"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Add or link provider</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            We'll resolve an existing platform identity before creating a new one.
          </p>
        </div>
      </div>

      {step === 'form' && (
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field label="Legal name" required>
            <input
              required
              value={form.legalName}
              onChange={(e) => setForm({ ...form, legalName: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Email" required>
              <input
                required
                type="email"
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
            <Field label="Phone" required>
              <input
                required
                value={form.phone}
                onChange={(e) => setForm({ ...form, phone: e.target.value })}
                placeholder="+234..."
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Service category" required>
              <select
                value={form.serviceCategory}
                onChange={(e) =>
                  setForm({ ...form, serviceCategory: e.target.value as ServiceCategory })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                {SERVICE_CATEGORIES.map((c) => (
                  <option key={c} value={c}>
                    {SERVICE_CATEGORY_LABEL[c]}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Home branch ID">
              <input
                value={form.homeBranchId}
                onChange={(e) => setForm({ ...form, homeBranchId: e.target.value })}
                placeholder="brn_..."
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Relationship">
              <select
                value={form.relationshipType}
                onChange={(e) => setForm({ ...form, relationshipType: e.target.value })}
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                <option value="">Default (employee)</option>
                <option value="EMPLOYEE">Employee</option>
                <option value="CONTRACTOR">Contractor</option>
                <option value="PARTNER">Partner</option>
              </select>
            </Field>
            <Field label="Employment type">
              <input
                value={form.employmentType}
                onChange={(e) => setForm({ ...form, employmentType: e.target.value })}
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
            <Field label="Employee number">
              <input
                value={form.employeeNumber}
                onChange={(e) => setForm({ ...form, employeeNumber: e.target.value })}
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
          </div>

          <button
            type="submit"
            className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <UserPlus className="h-4 w-4" />
            Send invitation
          </button>
        </form>
      )}

      {step === 'resolving' && (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-line bg-surface py-12 text-center">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-line border-t-primary" />
          <p className="text-sm text-ink-600">
            Resolving provider identity…
          </p>
        </div>
      )}

      {step === 'success' && result && (
        <Panel
          tone="green"
          icon={<UserPlus className="h-5 w-5 text-green" />}
          title={
            result.invitationSent
              ? 'Invitation sent'
              : 'Provider created — invitation pending delivery'
          }
          body={
            <>
              <p className="text-sm text-ink-700">
                Membership <span className="font-mono">{result.membershipId}</span> was
                created with status{' '}
                <span className="font-semibold">{result.status}</span>.
                {result.invitationSent
                  ? ' The provider has been emailed an invitation to consent to the association.'
                  : ' The invitation could not be delivered — it will be retried automatically.'}
              </p>
              <p className="mt-2 text-xs text-ink-500">
                The company cannot accept on the provider's behalf; the provider
                must consent themselves.
              </p>
            </>
          }
          actions={
            <>
              <button
                onClick={() => router.push(`/providers/${result.membershipId}`)}
                className="rounded-md bg-primary px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
              >
                View provider
              </button>
              <button
                onClick={() => router.push('/providers')}
                className="rounded-md border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                Back to list
              </button>
            </>
          }
        />
      )}

      {step === 'conflict' && (
        <Panel
          tone="red"
          icon={<AlertTriangle className="h-5 w-5 text-red" />}
          title="Conflicting identity found"
          body={
            <>
              <p className="text-sm text-ink-700">
                {errorMessage ??
                  'The email and phone number belong to different provider accounts.'}
              </p>
              <p className="mt-2 text-xs text-ink-500">
                This cannot be auto-merged. Contact the platform team to resolve
                the identity conflict before proceeding.
              </p>
            </>
          }
          actions={
            <button
              onClick={reset}
              className="rounded-md border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Back
            </button>
          }
        />
      )}

      {step === 'already-managed' && (
        <Panel
          tone="amber"
          icon={<Link2 className="h-5 w-5 text-amber" />}
          title="Already managed elsewhere"
          body={
            <>
              <p className="text-sm text-ink-700">
                {errorMessage ??
                  'This provider already has an active organization for this service.'}
              </p>
              <p className="mt-2 text-xs text-ink-500">
                A provider can only be managed by one organization per service
                category at a time. The existing relationship must end before a
                new one can begin.
              </p>
            </>
          }
          actions={
            <button
              onClick={reset}
              className="rounded-md border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Back
            </button>
          }
        />
      )}

      {step === 'error' && (
        <Panel
          tone="red"
          icon={<AlertTriangle className="h-5 w-5 text-red" />}
          title="Could not create provider"
          body={<p className="text-sm text-ink-700">{errorMessage}</p>}
          actions={
            <button
              onClick={reset}
              className="rounded-md border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Try again
            </button>
          }
        />
      )}
    </div>
  );
}

function Field({
  label,
  required,
  children,
}: {
  label: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
        {required && <span className="ml-1 text-red">*</span>}
      </label>
      {children}
    </div>
  );
}

function Panel({
  tone,
  icon,
  title,
  body,
  actions,
}: {
  tone: 'green' | 'red' | 'amber';
  icon: React.ReactNode;
  title: string;
  body: React.ReactNode;
  actions: React.ReactNode;
}) {
  const toneClasses = {
    green: 'border-green/20 bg-green-bg/30',
    amber: 'border-amber/20 bg-amber-bg/30',
    red: 'border-red/20 bg-red-bg/30',
  }[tone];

  return (
    <div className={`rounded-lg border p-5 ${toneClasses}`}>
      <div className="flex items-start gap-3">
        <div className="rounded-full bg-surface p-2">{icon}</div>
        <div className="flex-1 min-w-0">
          <h3 className="text-sm font-semibold text-ink-950">{title}</h3>
          <div className="mt-2">{body}</div>
          <div className="mt-4 flex flex-wrap gap-2.5">{actions}</div>
        </div>
      </div>
    </div>
  );
}