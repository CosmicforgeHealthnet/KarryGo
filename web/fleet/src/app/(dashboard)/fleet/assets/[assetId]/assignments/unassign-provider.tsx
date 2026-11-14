'use client';

import { useState, type FormEvent } from 'react';
import { UserMinus } from 'lucide-react';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from '@/components/features/FormField';
import { DangerButton, Notice, Panel, humanize } from '@/components/features/OpsUi';
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from '@/lib/api/errors';
import { assetCareService } from '@/lib/services/assetCareService';
import { assetService } from '@/lib/services/assetService';

// Ends the asset's current (or a named) provider allocation. The command
// carries the asset version read just before it, and is refused while the
// provider still has active company work on the asset. A safety block is
// never released by unassigning.
export function UnassignProvider({ assetId }: { assetId: string }) {
  const [assignmentId, setAssignmentId] = useState('');
  const [reason, setReason] = useState('');
  const [key, setKey] = useState(newIdempotencyKey);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const asset = await assetService.get(assetId);
      const ended = await assetCareService.unassignProvider(
        assetId,
        { assignmentId: assignmentId.trim() || undefined, reason: reason.trim() },
        asset.version,
        key,
      );
      setKey(newIdempotencyKey());
      setReason('');
      setAssignmentId('');
      setNotice(`Assignment ${ended.id} for provider ${ended.providerMembershipId} is now ${humanize(ended.status).toLowerCase()}.`);
    } catch (err) {
      setError(describeApiError(err, 'The provider could not be unassigned.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Panel title="End provider assignment" description="Leave the assignment blank to end the current one.">
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      <form onSubmit={submit} className="space-y-3">
        <Field label="Assignment ID">
          <input value={assignmentId} onChange={(e) => setAssignmentId(e.target.value)} placeholder="Current assignment" className={INPUT} />
        </Field>
        <Field label="Reason" required>
          <input required value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Provider moved to another asset" className={INPUT} />
        </Field>
        <DangerButton type="submit" disabled={busy || reason.trim() === ''}>
          <UserMinus className="h-3.5 w-3.5" /> {busy ? 'Ending…' : 'Unassign provider'}
        </DangerButton>
      </form>
    </Panel>
  );
}
