"use client";

import { useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { PageHeader, Panel, PrimaryButton, humanize } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { crmService } from "@/lib/services/crmService";
import { privateOpsService } from "@/lib/services/operationsService";
import { structureService } from "@/lib/services/structureService";
import { ORDER_PRIORITIES, type ServiceCatalogItem } from "@/lib/types/operations";
import type { CustomerView } from "@/lib/types/crm";
import type { Branch } from "@/lib/types/structure";

const toIso = (local: string) => (local ? new Date(local).toISOString() : undefined);

export default function CreateOrderPage() {
  const router = useRouter();
  const [customers, setCustomers] = useState<CustomerView[]>([]);
  const [services, setServices] = useState<ServiceCatalogItem[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [loadError, setLoadError] = useState<ApiErrorInfo | null>(null);
  const [form, setForm] = useState({
    customerId: "",
    serviceCatalogItemId: "",
    branchId: "",
    customerReference: "",
    priority: "STANDARD",
    start: "",
    end: "",
    timezone: "",
    specialInstructions: "",
  });
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  useEffect(() => {
    Promise.all([
      crmService.listCustomers({ limit: 100 }).then((r) => setCustomers(r.data)),
      privateOpsService.listServiceCatalog().then((s) => setServices(s.filter((x) => x.status === "ACTIVE"))),
      structureService.listBranches().then(setBranches),
    ]).catch((e) => setLoadError(describeApiError(e, "Customers, services or branches could not be loaded.")));
  }, []);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const order = await privateOpsService.createOrder(
        {
          customerId: form.customerId,
          serviceCatalogItemId: form.serviceCatalogItemId,
          branchId: form.branchId || undefined,
          customerReference: form.customerReference.trim() || undefined,
          priority: form.priority,
          requestedScheduleStart: toIso(form.start),
          requestedScheduleEnd: toIso(form.end),
          timezone: form.timezone.trim() || undefined,
          specialInstructions: form.specialInstructions.trim() || undefined,
        },
        key,
      );
      setKey(newIdempotencyKey());
      router.push(`/private-ops/orders/${order.id}`);
    } catch (err) {
      setError(describeApiError(err, "The order could not be created."));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <PageHeader
        title="New order"
        description="The order starts as a draft. Submit it for approval when it is complete."
        actions={
          <Link href="/private-ops/orders" className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <ArrowLeft className="h-3.5 w-3.5" /> Back
          </Link>
        }
      />
      <ApiErrorNotice error={loadError} />
      <Panel title="Order details">
        <form onSubmit={submit} className="space-y-4">
          <ApiErrorNotice error={error} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Customer" required>
              <select required value={form.customerId} onChange={(e) => setForm({ ...form, customerId: e.target.value })} className={INPUT}>
                <option value="">Choose a customer</option>
                {customers.map((c) => <option key={c.id} value={c.id}>{c.displayName || c.legalName}</option>)}
              </select>
            </Field>
            <Field label="Service" required>
              <select required value={form.serviceCatalogItemId} onChange={(e) => setForm({ ...form, serviceCatalogItemId: e.target.value })} className={INPUT}>
                <option value="">Choose a service</option>
                {services.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </Field>
            <Field label="Branch">
              <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                <option value="">Organization-wide</option>
                {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
              </select>
            </Field>
            <Field label="Customer reference">
              <input value={form.customerReference} onChange={(e) => setForm({ ...form, customerReference: e.target.value })} placeholder="PO-1234" className={INPUT} />
            </Field>
            <Field label="Priority">
              <select value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })} className={INPUT}>
                {ORDER_PRIORITIES.map((p) => <option key={p} value={p}>{humanize(p)}</option>)}
              </select>
            </Field>
            <Field label="Timezone" hint="Company timezone if blank">
              <input value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} placeholder="Africa/Lagos" className={INPUT} />
            </Field>
            <Field label="Requested start">
              <input type="datetime-local" value={form.start} onChange={(e) => setForm({ ...form, start: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Requested end">
              <input type="datetime-local" value={form.end} onChange={(e) => setForm({ ...form, end: e.target.value })} className={INPUT} />
            </Field>
            <div className="sm:col-span-2">
              <Field label="Special instructions">
                <textarea rows={3} value={form.specialInstructions} onChange={(e) => setForm({ ...form, specialInstructions: e.target.value })} className={INPUT} />
              </Field>
            </div>
          </div>
          <PrimaryButton type="submit" disabled={saving}>
            <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Create order"}
          </PrimaryButton>
        </form>
      </Panel>
    </div>
  );
}
