'use client';

import { useState, type FormEvent } from 'react';
import { AlertCircle, Save, UserMinus } from 'lucide-react';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from '@/components/features/FormField';
import { describeApiError, type ApiErrorInfo } from '@/lib/api/errors';
import { providerService } from '@/lib/services/providerService';
import type {
  EmploymentType,
  ProviderMembership,
  RelationshipType,
  UpdateProviderInput,
} from '@/lib/types/provider';
import type { Branch } from '@/lib/types/structure';

// Organization-side membership edits (PATCH /providers/:membershipId). The
// provider's platform identity is owned by its provider service and is not
// editable here.
export function MembershipEditor({
  membership,
  branches,
  onCancel,
  onSaved,
}: {
  membership: ProviderMembership;
  branches: Branch[];
  onCancel: () => void;
  onSaved: (updated: ProviderMembership) => void;
}) {
  const [homeBranchId, setHomeBranchId] = useState(membership.homeBranchId ?? '');
  const [relationshipType, setRelationshipType] = useState<RelationshipType>(membership.relationshipType);
  const [employmentType, setEmploymentType] = useState<EmploymentType | ''>(
    (membership.employmentType as EmploymentType | undefined) ?? '',
  );
  const [employeeNumber, setEmployeeNumber] = useState(membership.employeeNumber ?? '');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    const input: UpdateProviderInput = { relationshipType };
    if (homeBranchId && homeBranchId !== membership.homeBranchId) input.homeBranchId = homeBranchId;
    if (employmentType !== (membership.employmentType ?? '')) input.employmentType = employmentType;
    if (employeeNumber.trim() !== (membership.employeeNumber ?? '')) input.employeeNumber = employeeNumber.trim();
    try {
      onSaved(await providerService.update(membership.membershipId, input, membership.version));
    } catch (err) {
      setError(describeApiError(err, 'The membership could not be saved.'));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4 rounded-lg border border-line bg-surface p-4">
      <h3 className="text-sm font-semibold text-ink-950">Edit membership</h3>
      <ApiErrorNotice error={error} />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field label="Home branch">
          <select value={homeBranchId} onChange={(e) => setHomeBranchId(e.target.value)} className={INPUT}>
            <option value="">{membership.homeBranchId ? 'Keep current branch' : 'None'}</option>
            {branches
              .filter((b) => b.status === 'ACTIVE')
              .map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}
                </option>
              ))}
          </select>
        </Field>
        <Field label="Relationship" required>
          <select
            value={relationshipType}
            onChange={(e) => setRelationshipType(e.target.value as RelationshipType)}
            className={INPUT}
          >
            <option value="EMPLOYEE">Employee</option>
            <option value="CONTRACTOR">Contractor</option>
            <option value="PARTNER">Partner</option>
          </select>
        </Field>
        <Field label="Employment type">
          <select
            value={employmentType}
            onChange={(e) => setEmploymentType(e.target.value as EmploymentType | '')}
            className={INPUT}
          >
            <option value="">Not set</option>
            <option value="FULL_TIME">Full time</option>
            <option value="PART_TIME">Part time</option>
            <option value="CASUAL">Casual</option>
          </select>
        </Field>
        <Field label="Employee number" hint="Leave blank to keep the current number.">
          <input value={employeeNumber} onChange={(e) => setEmployeeNumber(e.target.value)} className={INPUT} />
        </Field>
      </div>
      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          disabled={saving}
          className="flex items-center gap-2 rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
        >
          <Save className="h-3.5 w-3.5" />
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

const REMOVAL_REASONS = [
  { value: 'removed_by_organization', label: 'Removed by the organization' },
  { value: 'contract_ended', label: 'Contract ended' },
  { value: 'resigned', label: 'Provider resigned' },
  { value: 'performance', label: 'Performance' },
  { value: 'compliance', label: 'Compliance' },
];

// Ends the membership (POST /providers/:membershipId/remove). The server
// refuses while the provider still holds company work.
export function RemoveMembershipCard({
  membership,
  onRemoved,
}: {
  membership: ProviderMembership;
  onRemoved: (status: string) => Promise<void> | void;
}) {
  const [confirming, setConfirming] = useState(false);
  const [reasonCode, setReasonCode] = useState(REMOVAL_REASONS[0].value);
  const [removing, setRemoving] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const notWorking = ['INVITED', 'PENDING_PROVIDER_ACCEPTANCE', 'PENDING_SETUP'].includes(membership.status);

  async function remove() {
    setRemoving(true);
    setError(null);
    try {
      const result = await providerService.remove(membership.membershipId, membership.version, reasonCode);
      setConfirming(false);
      await onRemoved(result.status);
    } catch (err) {
      setError(describeApiError(err, 'The provider could not be removed.'));
    } finally {
      setRemoving(false);
    }
  }

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="flex items-start gap-3">
        <div className="rounded-full bg-red-bg p-2">
          <UserMinus className="h-4 w-4 text-red" />
        </div>
        <div className="flex-1">
          <h3 className="text-sm font-semibold text-ink-950">
            {notWorking ? 'Withdraw invitation' : 'Remove from organization'}
          </h3>
          <p className="mt-0.5 text-xs text-ink-500">
            Ends this provider&apos;s membership. Work they still hold must be finished or reassigned first.
            Their provider account is not deleted.
          </p>
          <ApiErrorNotice error={error} className="mt-3" />
          {!confirming ? (
            <button
              onClick={() => setConfirming(true)}
              className="mt-3 flex items-center gap-2 rounded-md border border-red/30 bg-red-bg px-3.5 py-1.5 text-xs font-semibold text-red transition-colors hover:bg-red-bg/70"
            >
              <UserMinus className="h-3.5 w-3.5" />
              {notWorking ? 'Withdraw' : 'Remove'}
            </button>
          ) : (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <select
                value={reasonCode}
                onChange={(e) => setReasonCode(e.target.value)}
                className="rounded-md border border-line bg-surface px-2.5 py-1.5 text-xs text-ink-950 outline-none focus:border-primary"
              >
                {REMOVAL_REASONS.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
              <span className="flex items-center gap-1 text-xs text-ink-600">
                <AlertCircle className="h-3.5 w-3.5 text-red" />
                Are you sure?
              </span>
              <button
                onClick={remove}
                disabled={removing}
                className="rounded-md bg-red px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-red/90 disabled:opacity-50"
              >
                {removing ? 'Removing…' : 'Confirm'}
              </button>
              <button
                onClick={() => setConfirming(false)}
                disabled={removing}
                className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
