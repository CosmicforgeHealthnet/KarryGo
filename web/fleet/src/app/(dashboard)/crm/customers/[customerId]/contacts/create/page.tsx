"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { AlertCircle, RefreshCw, Save } from "lucide-react";
import { crmService } from "@/lib/services/crmService";
import type { ContactChannel, ContactPurpose } from "@/lib/types/crm";

const CHANNELS: ContactChannel[] = ["EMAIL", "PHONE", "SMS", "WHATSAPP", "IN_APP", "NONE"];
const PURPOSES: ContactPurpose[] = ["GENERAL", "OPERATIONS", "BILLING", "LEGAL", "SALES", "EMERGENCY", "OTHER"];
const label = (v: string) => v.replace(/_/g, " ").toLowerCase().replace(/^\w/, (c) => c.toUpperCase());

export default function CreateContactPage() {
  const router = useRouter();
  const customerId = useParams().customerId as string;
  const back = `/crm/customers/${customerId}/contacts`;
  const [form, setForm] = useState({ fullName: "", roleTitle: "", email: "", phone: "", preferredChannel: "EMAIL" as ContactChannel, contactPurpose: "GENERAL" as ContactPurpose, isPrimary: false });
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const submit = async () => {
    if (!form.fullName.trim()) {
      setError("Enter the contact's full name.");
      return;
    }
    if (!form.email.trim() && !form.phone.trim()) {
      setError("Enter an email address or a phone number.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await crmService.createContact(customerId, {
        fullName: form.fullName.trim(),
        roleTitle: form.roleTitle.trim() || undefined,
        email: form.email.trim() || undefined,
        phone: form.phone.trim() || undefined,
        preferredChannel: form.preferredChannel,
        contactPurpose: form.contactPurpose,
        isPrimary: form.isPrimary,
      });
      router.push(back);
    } catch (e) {
      const err = e as { response?: { data?: { error?: { message?: string } } } };
      setError(err?.response?.data?.error?.message ?? "The contact could not be saved. Please try again.");
      setSaving(false);
    }
  };

  const input = "mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary";
  const lbl = "text-xs font-medium uppercase tracking-[0.06em] text-ink-500";

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div>
        <h1>Add Contact</h1>
        <p className="mt-1.5">A person at this customer you work with.</p>
      </div>
      <div className="rounded-2xl border border-line bg-surface p-6 shadow-sm">
        {error && (
          <div role="alert" className="mb-4 flex items-start gap-2 rounded-xl border border-red/30 bg-red-bg px-4 py-3 text-sm text-red">
            <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
            {error}
          </div>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label htmlFor="c-name" className={lbl}>Full name <span className="text-red">*</span></label>
            <input id="c-name" className={input} value={form.fullName} onChange={(e) => setForm({ ...form, fullName: e.target.value })} />
          </div>
          <div>
            <label htmlFor="c-email" className={lbl}>Email</label>
            <input id="c-email" type="email" className={input} value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          </div>
          <div>
            <label htmlFor="c-phone" className={lbl}>Phone</label>
            <input id="c-phone" type="tel" className={input} value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
          </div>
          <div>
            <label htmlFor="c-role" className={lbl}>Role / title</label>
            <input id="c-role" className={input} value={form.roleTitle} onChange={(e) => setForm({ ...form, roleTitle: e.target.value })} />
          </div>
          <div>
            <label htmlFor="c-purpose" className={lbl}>Purpose</label>
            <select id="c-purpose" className={input} value={form.contactPurpose} onChange={(e) => setForm({ ...form, contactPurpose: e.target.value as ContactPurpose })}>
              {PURPOSES.map((p) => <option key={p} value={p}>{label(p)}</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="c-channel" className={lbl}>Preferred channel</label>
            <select id="c-channel" className={input} value={form.preferredChannel} onChange={(e) => setForm({ ...form, preferredChannel: e.target.value as ContactChannel })}>
              {CHANNELS.map((c) => <option key={c} value={c}>{label(c)}</option>)}
            </select>
          </div>
          <label className="flex cursor-pointer items-center gap-3 self-end pb-2">
            <input type="checkbox" checked={form.isPrimary} onChange={(e) => setForm({ ...form, isPrimary: e.target.checked })} className="h-4 w-4" />
            <span className="text-sm text-ink-700">Primary contact</span>
          </label>
        </div>
        <div className="mt-6 flex flex-wrap gap-3">
          <button onClick={submit} disabled={saving} className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white hover:bg-primary/90 disabled:bg-ink-400">
            {saving ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {saving ? "Saving..." : "Save contact"}
          </button>
          <button onClick={() => router.push(back)} className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-surface-2">Cancel</button>
        </div>
      </div>
    </div>
  );
}
