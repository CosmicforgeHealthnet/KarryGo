'use client';

import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { CheckCircle2, Plus, Save, X, XCircle } from 'lucide-react';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from '@/components/features/FormField';
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from '@/lib/api/errors';
import { providerService } from '@/lib/services/providerService';
import { staffService } from '@/lib/services/staffService';
import { workforceService } from '@/lib/services/workforceService';
import { serviceCategoryLabel } from '@/lib/utils/providerLabels';
import { formatDate } from '@/lib/utils/workforceLabels';
import type { Course, Enrollment } from '@/lib/types/workforce';

type Person = { id: string; label: string };

const STATUS_STYLE: Record<string, string> = {
  ASSIGNED: 'bg-primary-bg text-primary',
  PASSED: 'bg-green-bg text-green',
  COMPLETED: 'bg-green-bg text-green',
  FAILED: 'bg-red-bg text-red',
  CANCELLED: 'bg-surface-2 text-ink-500',
};

const statusLabel = (s: string) => s.charAt(0) + s.slice(1).toLowerCase();

const nowLocal = () => {
  const d = new Date();
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
};

// Course enrolments for staff and providers (/training/enrollments).
export function TrainingEnrollmentsSection({ courses }: { courses: Course[] }) {
  const [rows, setRows] = useState<Enrollment[]>([]);
  const [people, setPeople] = useState<Person[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [assigning, setAssigning] = useState(false);
  const [assignForm, setAssignForm] = useState({ courseId: '', membershipId: '', dueAt: '' });
  const [assignKey, setAssignKey] = useState(newIdempotencyKey);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [saving, setSaving] = useState(false);

  const [completing, setCompleting] = useState<Enrollment | null>(null);
  const [completion, setCompletion] = useState({ score: '', certificateMediaId: '', completedAt: nowLocal() });
  const [completeKey, setCompleteKey] = useState(newIdempotencyKey);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [enrollments, staff, providers] = await Promise.all([
        workforceService.listEnrollments({ limit: 100 }),
        staffService.list().catch(() => []),
        providerService.list().catch(() => []),
      ]);
      setRows(enrollments);
      setPeople([
        ...staff
          .filter((m) => m.status === 'ACTIVE')
          .map((m) => ({ id: m.id, label: `${m.fullName || m.email} (staff)` })),
        ...providers
          .filter((p) => p.status === 'ACTIVE')
          .map((p) => ({
            id: p.membershipId,
            label: `${serviceCategoryLabel(p.serviceCategory)} provider ${p.membershipId}`,
          })),
      ]);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, 'Enrolments could not be loaded.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const courseTitle = new Map(courses.map((c) => [c.id, c.title]));
  const personLabel = new Map(people.map((p) => [p.id, p.label]));

  async function assign(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      await workforceService.createEnrollment(
        {
          courseId: assignForm.courseId,
          membershipId: assignForm.membershipId,
          ...(assignForm.dueAt ? { dueAt: new Date(assignForm.dueAt).toISOString() } : {}),
        },
        assignKey,
      );
      setAssigning(false);
      setAssignForm({ courseId: '', membershipId: '', dueAt: '' });
      setAssignKey(newIdempotencyKey());
      setNotice('Course assigned.');
      await load();
    } catch (err) {
      setFormError(describeApiError(err, 'The course could not be assigned.'));
    } finally {
      setSaving(false);
    }
  }

  async function complete(e: FormEvent) {
    e.preventDefault();
    if (!completing) return;
    setSaving(true);
    setFormError(null);
    try {
      await workforceService.completeEnrollment(
        completing.id,
        {
          score: Number(completion.score),
          completedAt: new Date(completion.completedAt).toISOString(),
          ...(completion.certificateMediaId.trim() ? { certificateMediaId: completion.certificateMediaId.trim() } : {}),
        },
        completeKey,
      );
      setCompleting(null);
      setCompleteKey(newIdempotencyKey());
      setNotice('Completion recorded.');
      await load();
    } catch (err) {
      setFormError(describeApiError(err, 'The completion could not be recorded.'));
    } finally {
      setSaving(false);
    }
  }

  async function cancel(enrollment: Enrollment) {
    if (!window.confirm('Cancel this enrolment?')) return;
    setBusyId(enrollment.id);
    setError(null);
    setNotice(null);
    try {
      await workforceService.cancelEnrollment(enrollment.id, newIdempotencyKey());
      setNotice('Enrolment cancelled.');
      await load();
    } catch (err) {
      setError(describeApiError(err, 'The enrolment could not be cancelled.'));
    } finally {
      setBusyId(null);
    }
  }

  const activeCourses = courses.filter((c) => c.status === 'ACTIVE');

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-base font-semibold text-ink-950">Enrolments</h2>
          <p className="text-xs text-ink-500">Courses assigned to staff and providers, and their results.</p>
        </div>
        {!assigning && (
          <button
            onClick={() => {
              setAssigning(true);
              setCompleting(null);
              setFormError(null);
              setNotice(null);
            }}
            className="flex h-9 items-center gap-1.5 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <Plus className="h-3.5 w-3.5" />
            Assign course
          </button>
        )}
      </div>

      <ApiErrorNotice error={error} />
      {notice && <div className="rounded-lg border border-green/30 bg-green-bg px-4 py-2 text-sm text-green">{notice}</div>}

      {assigning && (
        <form onSubmit={assign} className="space-y-4 rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-ink-950">Assign a course</h3>
            <button type="button" onClick={() => setAssigning(false)} className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          </div>
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Course" required>
              <select required value={assignForm.courseId} onChange={(e) => setAssignForm({ ...assignForm, courseId: e.target.value })} className={INPUT}>
                <option value="">Choose a course</option>
                {activeCourses.map((c) => (
                  <option key={c.id} value={c.id}>{c.title}</option>
                ))}
              </select>
            </Field>
            <Field label="Person" required>
              <select required value={assignForm.membershipId} onChange={(e) => setAssignForm({ ...assignForm, membershipId: e.target.value })} className={INPUT}>
                <option value="">Choose a person</option>
                {people.map((p) => (
                  <option key={p.id} value={p.id}>{p.label}</option>
                ))}
              </select>
            </Field>
            <Field label="Due">
              <input type="datetime-local" value={assignForm.dueAt} onChange={(e) => setAssignForm({ ...assignForm, dueAt: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <button type="submit" disabled={saving} className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60">
            <Save className="h-4 w-4" />
            {saving ? 'Assigning…' : 'Assign'}
          </button>
        </form>
      )}

      {completing && (
        <form onSubmit={complete} className="space-y-4 rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-ink-950">
              Record result: {courseTitle.get(completing.courseId) ?? completing.courseId}
            </h3>
            <button type="button" onClick={() => setCompleting(null)} className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          </div>
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Score (%)" required>
              <input required type="number" min={0} max={100} step="0.01" value={completion.score} onChange={(e) => setCompletion({ ...completion, score: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Completed at" required>
              <input required type="datetime-local" value={completion.completedAt} onChange={(e) => setCompletion({ ...completion, completedAt: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Certificate media ID" hint="From an uploaded certificate, if any.">
              <input value={completion.certificateMediaId} onChange={(e) => setCompletion({ ...completion, certificateMediaId: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <button type="submit" disabled={saving} className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60">
            <Save className="h-4 w-4" />
            {saving ? 'Saving…' : 'Record result'}
          </button>
        </form>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-8 text-center text-sm text-ink-500">Loading enrolments…</div>
      ) : rows.length === 0 ? (
        <div className="rounded-lg border border-dashed border-line bg-surface px-4 py-8 text-center text-sm text-ink-500">
          No courses have been assigned yet.
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  {['Course', 'Person', 'Status', 'Due', 'Completed', 'Score', ''].map((h) => (
                    <th key={h} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.id} className="border-b border-line">
                    <td className="px-4 py-3 text-xs font-medium text-ink-950">{courseTitle.get(r.courseId) ?? r.courseId}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{personLabel.get(r.membershipId) ?? r.membershipId}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${STATUS_STYLE[r.status] ?? 'bg-surface-2 text-ink-500'}`}>
                        {statusLabel(r.status)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{r.dueAt ? formatDate(r.dueAt) : '—'}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{r.completedAt ? formatDate(r.completedAt) : '—'}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{r.score == null ? '—' : `${r.score}%`}</td>
                    <td className="px-4 py-3 text-right">
                      {r.status === 'ASSIGNED' && (
                        <div className="flex justify-end gap-3">
                          <button
                            onClick={() => {
                              setCompleting(r);
                              setAssigning(false);
                              setCompletion({ score: '', certificateMediaId: '', completedAt: nowLocal() });
                              setFormError(null);
                              setNotice(null);
                            }}
                            className="flex items-center gap-1 text-xs font-medium text-green transition-colors hover:text-green/80"
                          >
                            <CheckCircle2 className="h-3.5 w-3.5" />
                            Record result
                          </button>
                          <button
                            onClick={() => cancel(r)}
                            disabled={busyId === r.id}
                            className="flex items-center gap-1 text-xs font-medium text-ink-500 transition-colors hover:text-red disabled:opacity-50"
                          >
                            <XCircle className="h-3.5 w-3.5" />
                            Cancel
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}
