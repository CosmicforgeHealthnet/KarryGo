'use client';

import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Archive, CalendarRange, Plus, RotateCcw, Save, X } from 'lucide-react';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from '@/components/features/FormField';
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from '@/lib/api/errors';
import { structureService } from '@/lib/services/structureService';
import { workforceService } from '@/lib/services/workforceService';
import type { Branch } from '@/lib/types/structure';
import type { GeneratedShifts, ShiftTemplate } from '@/lib/types/workforce';

const WEEKDAYS = [
  { value: 1, label: 'Mon' },
  { value: 2, label: 'Tue' },
  { value: 3, label: 'Wed' },
  { value: 4, label: 'Thu' },
  { value: 5, label: 'Fri' },
  { value: 6, label: 'Sat' },
  { value: 7, label: 'Sun' },
];

const dayLabel = (days: number[]) =>
  days.map((d) => WEEKDAYS.find((w) => w.value === d)?.label ?? String(d)).join(', ');

const today = () => new Date().toISOString().slice(0, 10);

type TemplateForm = {
  branchId: string;
  name: string;
  role: string;
  timezone: string;
  weekdays: number[];
  startLocalTime: string;
  endLocalTime: string;
  requiredCapacity: string;
};

const EMPTY_FORM: TemplateForm = {
  branchId: '',
  name: '',
  role: '',
  timezone: 'Africa/Lagos',
  weekdays: [1, 2, 3, 4, 5],
  startLocalTime: '08:00',
  endLocalTime: '17:00',
  requiredCapacity: '1',
};

// Weekly shift patterns and generating draft shifts from them
// (/shift-templates). Shifts are created as drafts in the template's
// timezone; days that fall in a daylight-saving gap are skipped and listed.
export function ShiftTemplatesSection({ onShiftsGenerated }: { onShiftsGenerated: () => void }) {
  const [templates, setTemplates] = useState<ShiftTemplate[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState<TemplateForm>(EMPTY_FORM);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [saving, setSaving] = useState(false);
  // One key per create attempt, so a retried submit cannot create twice.
  const [createKey, setCreateKey] = useState(newIdempotencyKey);

  const [generatingFor, setGeneratingFor] = useState<string | null>(null);
  const [range, setRange] = useState({ from: today(), to: today() });
  const [generateKey, setGenerateKey] = useState(newIdempotencyKey);
  const [generating, setGenerating] = useState(false);
  const [result, setResult] = useState<GeneratedShifts | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [rows, allBranches] = await Promise.all([
        workforceService.listShiftTemplates({ limit: 100 }),
        structureService.listBranches().catch(() => [] as Branch[]),
      ]);
      setTemplates(rows);
      setBranches(allBranches);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, 'Shift templates could not be loaded.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const branchName = new Map(branches.map((b) => [b.id, b.name]));

  async function submit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      await workforceService.createShiftTemplate(
        {
          branchId: form.branchId,
          name: form.name.trim(),
          role: form.role.trim(),
          timezone: form.timezone.trim(),
          weekdays: [...form.weekdays].sort(),
          startLocalTime: form.startLocalTime,
          endLocalTime: form.endLocalTime,
          requiredCapacity: Number(form.requiredCapacity),
        },
        createKey,
      );
      setCreating(false);
      setForm(EMPTY_FORM);
      setCreateKey(newIdempotencyKey());
      await load();
    } catch (err) {
      setFormError(describeApiError(err, 'The shift template could not be created.'));
    } finally {
      setSaving(false);
    }
  }

  async function generate(template: ShiftTemplate) {
    setGenerating(true);
    setError(null);
    setResult(null);
    try {
      const generated = await workforceService.generateShiftsFromTemplate(template.id, range, generateKey);
      setResult(generated);
      setGenerateKey(newIdempotencyKey());
      if (generated.created.length > 0) onShiftsGenerated();
    } catch (err) {
      setError(describeApiError(err, 'Shifts could not be generated.'));
    } finally {
      setGenerating(false);
    }
  }

  async function setStatus(template: ShiftTemplate, status: 'ACTIVE' | 'ARCHIVED') {
    setBusyId(template.id);
    setError(null);
    try {
      await workforceService.updateShiftTemplate(template.id, { expectedVersion: template.version, status });
      await load();
    } catch (err) {
      setError(describeApiError(err, 'The shift template could not be updated.'));
    } finally {
      setBusyId(null);
    }
  }

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-base font-semibold text-ink-950">Shift templates</h2>
          <p className="text-xs text-ink-500">Weekly patterns that generate draft shifts.</p>
        </div>
        {!creating && (
          <button
            onClick={() => {
              setCreating(true);
              setFormError(null);
            }}
            className="flex h-9 items-center gap-1.5 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <Plus className="h-3.5 w-3.5" />
            New template
          </button>
        )}
      </div>

      <ApiErrorNotice error={error} />

      {creating && (
        <form onSubmit={submit} className="space-y-4 rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-ink-950">New shift template</h3>
            <button type="button" onClick={() => setCreating(false)} className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          </div>
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Branch" required>
              <select
                required
                value={form.branchId}
                onChange={(e) => {
                  const branch = branches.find((b) => b.id === e.target.value);
                  setForm({ ...form, branchId: e.target.value, timezone: branch?.timezone || form.timezone });
                }}
                className={INPUT}
              >
                <option value="">Choose a branch</option>
                {branches
                  .filter((b) => b.status === 'ACTIVE')
                  .map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.name}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label="Name" required>
              <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Morning dispatch" className={INPUT} />
            </Field>
            <Field label="Role">
              <input value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })} placeholder="Dispatch Rider" className={INPUT} />
            </Field>
            <Field label="Timezone" required hint="An IANA timezone such as Africa/Lagos.">
              <input required value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Starts" required>
              <input required type="time" value={form.startLocalTime} onChange={(e) => setForm({ ...form, startLocalTime: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Ends" required hint="An end before the start runs overnight.">
              <input required type="time" value={form.endLocalTime} onChange={(e) => setForm({ ...form, endLocalTime: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Required capacity" required>
              <input required type="number" min={1} value={form.requiredCapacity} onChange={(e) => setForm({ ...form, requiredCapacity: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Days" required>
              <div className="flex flex-wrap gap-1.5">
                {WEEKDAYS.map((d) => {
                  const on = form.weekdays.includes(d.value);
                  return (
                    <button
                      key={d.value}
                      type="button"
                      aria-pressed={on}
                      onClick={() =>
                        setForm({
                          ...form,
                          weekdays: on ? form.weekdays.filter((w) => w !== d.value) : [...form.weekdays, d.value],
                        })
                      }
                      className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
                        on ? 'bg-primary text-white' : 'bg-surface-2 text-ink-600 hover:bg-surface-3'
                      }`}
                    >
                      {d.label}
                    </button>
                  );
                })}
              </div>
            </Field>
          </div>
          <button
            type="submit"
            disabled={saving}
            className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Save className="h-4 w-4" />
            {saving ? 'Creating…' : 'Create template'}
          </button>
        </form>
      )}

      {result && (
        <div className="rounded-lg border border-line bg-surface p-4 text-sm text-ink-700">
          <div className="flex items-start justify-between gap-3">
            <p>
              Created {result.created.length} draft shift{result.created.length === 1 ? '' : 's'}.
              {result.alreadyExist.length > 0 && ` ${result.alreadyExist.length} day(s) already had a shift from this template.`}
            </p>
            <button onClick={() => setResult(null)} className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          </div>
          {result.skippedDays.length > 0 && (
            <p className="mt-2 rounded-md bg-amber-bg px-3 py-2 text-xs text-amber">
              Skipped because the shift time does not exist on these days (daylight-saving change):{' '}
              {result.skippedDays.join(', ')}
            </p>
          )}
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-8 text-center text-sm text-ink-500">
          Loading shift templates…
        </div>
      ) : templates.length === 0 ? (
        <div className="rounded-lg border border-dashed border-line bg-surface px-4 py-8 text-center text-sm text-ink-500">
          No shift templates yet.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {templates.map((t) => (
            <div key={t.id} className="rounded-lg border border-line bg-surface p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold text-ink-950">{t.name}</div>
                  <div className="truncate text-xs text-ink-500">
                    {branchName.get(t.branchId) ?? t.branchId}
                    {t.role ? ` · ${t.role}` : ''}
                  </div>
                </div>
                <span
                  className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                    t.status === 'ACTIVE' ? 'bg-green-bg text-green' : 'bg-surface-2 text-ink-500'
                  }`}
                >
                  {t.status === 'ACTIVE' ? 'Active' : 'Archived'}
                </span>
              </div>
              <div className="mt-3 space-y-1 text-xs text-ink-500">
                <div>{dayLabel(t.weekdays)}</div>
                <div>
                  {t.startLocalTime}-{t.endLocalTime} · {t.timezone} · {t.requiredCapacity} slots
                </div>
              </div>

              {generatingFor === t.id ? (
                <div className="mt-3 space-y-2 border-t border-line pt-3">
                  <div className="grid grid-cols-2 gap-2">
                    <Field label="From">
                      <input type="date" value={range.from} onChange={(e) => setRange({ ...range, from: e.target.value })} className={INPUT} />
                    </Field>
                    <Field label="To">
                      <input type="date" value={range.to} onChange={(e) => setRange({ ...range, to: e.target.value })} className={INPUT} />
                    </Field>
                  </div>
                  <p className="text-[11px] text-ink-400">Up to 62 days. Days already generated are not duplicated.</p>
                  <div className="flex gap-2">
                    <button
                      onClick={() => generate(t)}
                      disabled={generating}
                      className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
                    >
                      {generating ? 'Generating…' : 'Generate shifts'}
                    </button>
                    <button
                      onClick={() => setGeneratingFor(null)}
                      className="rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
                    >
                      Close
                    </button>
                  </div>
                </div>
              ) : (
                <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-2.5">
                  {t.status === 'ACTIVE' && (
                    <button
                      onClick={() => {
                        setGeneratingFor(t.id);
                        setResult(null);
                      }}
                      className="flex items-center gap-1 text-xs font-medium text-primary transition-colors hover:text-primary-hover"
                    >
                      <CalendarRange className="h-3.5 w-3.5" />
                      Generate shifts
                    </button>
                  )}
                  <button
                    onClick={() => setStatus(t, t.status === 'ACTIVE' ? 'ARCHIVED' : 'ACTIVE')}
                    disabled={busyId === t.id}
                    className="ml-auto flex items-center gap-1 text-xs font-medium text-ink-500 transition-colors hover:text-ink-800 disabled:opacity-50"
                  >
                    {t.status === 'ACTIVE' ? <Archive className="h-3.5 w-3.5" /> : <RotateCcw className="h-3.5 w-3.5" />}
                    {t.status === 'ACTIVE' ? 'Archive' : 'Restore'}
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
