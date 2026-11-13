"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { AlertCircle, RefreshCw, Save } from "lucide-react";
import { crmService } from "@/lib/services/crmService";
import type { LocationType } from "@/lib/types/crm";

const TYPES: LocationType[] = ["PICKUP", "DELIVERY", "BILLING", "WAREHOUSE", "OFFICE", "DEPOT", "SITE", "PORT", "TERMINAL", "OTHER"];
const label = (v: string) => v.replace(/_/g, " ").toLowerCase().replace(/^\w/, (c) => c.toUpperCase());

export default function CreateLocationPage() {
  const router = useRouter();
  const customerId = useParams().customerId as string;
  const back = `/crm/customers/${customerId}/locations`;
  const [form, setForm] = useState({
    locationType: "DELIVERY" as LocationType, label: "", line1: "", line2: "", city: "", stateOrRegion: "",
    postalCode: "", countryCode: "NG", timezone: "Africa/Lagos", accessInstructions: "",
  });
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) =>
    setForm({ ...form, [key]: e.target.value });

  const submit = async () => {
    if (!form.label.trim() || !form.line1.trim() || !form.city.trim() || !form.stateOrRegion.trim()) {
      setError("Enter a name, street address, city and state.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await crmService.createLocation(customerId, {
        locationType: form.locationType,
        label: form.label.trim(),
        address: {
          line1: form.line1.trim(),
          line2: form.line2.trim() || undefined,
          city: form.city.trim(),
          stateOrRegion: form.stateOrRegion.trim(),
          postalCode: form.postalCode.trim() || undefined,
          countryCode: form.countryCode.trim().toUpperCase(),
        },
        accessInstructions: form.accessInstructions.trim() || undefined,
        timezone: form.timezone.trim(),
      });
      router.push(back);
    } catch (e) {
      const err = e as { response?: { data?: { error?: { message?: string } } } };
      setError(err?.response?.data?.error?.message ?? "The location could not be saved. Please try again.");
      setSaving(false);
    }
  };

  const input = "mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary";
  const lbl = "text-xs font-medium uppercase tracking-[0.06em] text-ink-500";

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div>
        <h1>Add Location</h1>
        <p className="mt-1.5">A pickup, delivery or other site for this customer.</p>
      </div>
      <div className="rounded-2xl border border-line bg-surface p-6 shadow-sm">
        {error && (
          <div role="alert" className="mb-4 flex items-start gap-2 rounded-xl border border-red/30 bg-red-bg px-4 py-3 text-sm text-red">
            <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
            {error}
          </div>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="l-label" className={lbl}>Name <span className="text-red">*</span></label>
            <input id="l-label" className={input} placeholder="e.g. Ikeja warehouse" value={form.label} onChange={set("label")} />
          </div>
          <div>
            <label htmlFor="l-type" className={lbl}>Type</label>
            <select id="l-type" className={input} value={form.locationType} onChange={set("locationType")}>
              {TYPES.map((t) => <option key={t} value={t}>{label(t)}</option>)}
            </select>
          </div>
          <div className="sm:col-span-2">
            <label htmlFor="l-line1" className={lbl}>Street address <span className="text-red">*</span></label>
            <input id="l-line1" className={input} value={form.line1} onChange={set("line1")} />
          </div>
          <div className="sm:col-span-2">
            <label htmlFor="l-line2" className={lbl}>Address line 2</label>
            <input id="l-line2" className={input} value={form.line2} onChange={set("line2")} />
          </div>
          <div>
            <label htmlFor="l-city" className={lbl}>City <span className="text-red">*</span></label>
            <input id="l-city" className={input} value={form.city} onChange={set("city")} />
          </div>
          <div>
            <label htmlFor="l-state" className={lbl}>State / region <span className="text-red">*</span></label>
            <input id="l-state" className={input} value={form.stateOrRegion} onChange={set("stateOrRegion")} />
          </div>
          <div>
            <label htmlFor="l-postal" className={lbl}>Postal code</label>
            <input id="l-postal" className={input} value={form.postalCode} onChange={set("postalCode")} />
          </div>
          <div>
            <label htmlFor="l-country" className={lbl}>Country code</label>
            <input id="l-country" className={input} maxLength={2} value={form.countryCode} onChange={set("countryCode")} />
          </div>
          <div className="sm:col-span-2">
            <label htmlFor="l-access" className={lbl}>Access instructions</label>
            <textarea id="l-access" rows={3} className={`${input} resize-none`} value={form.accessInstructions} onChange={set("accessInstructions")} />
          </div>
        </div>
        <div className="mt-6 flex flex-wrap gap-3">
          <button onClick={submit} disabled={saving} className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white hover:bg-primary/90 disabled:bg-ink-400">
            {saving ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {saving ? "Saving..." : "Save location"}
          </button>
          <button onClick={() => router.push(back)} className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-surface-2">Cancel</button>
        </div>
      </div>
    </div>
  );
}
