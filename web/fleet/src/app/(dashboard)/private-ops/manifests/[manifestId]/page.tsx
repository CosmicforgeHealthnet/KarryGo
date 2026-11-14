"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Edit3, Lock, Plus, Save, ShieldCheck, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DangerButton,
  DataTable,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { manifestService } from "@/lib/services/operationsService";
import { MANIFEST_ITEM_TYPES, type Manifest, type ManifestItem } from "@/lib/types/operations";

type Mode = "none" | "edit" | "seal" | "cancel";

export default function ManifestDetailPage() {
  const params = useParams();
  const router = useRouter();
  const manifestId = params.manifestId as string;

  const [manifest, setManifest] = useState<Manifest | null>(null);
  const [items, setItems] = useState<ManifestItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [mode, setMode] = useState<Mode>("none");
  const [edit, setEdit] = useState({ routeReference: "", vehicleReference: "", providerMembershipId: "", sealNumber: "" });
  const [seal, setSeal] = useState({ sealNumber: "", reasonCode: "" });
  const [cancelReason, setCancelReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);

  const [item, setItem] = useState({ itemType: "SHIPMENT", itemId: "" });
  const [addingItem, setAddingItem] = useState(false);
  const [itemError, setItemError] = useState<ApiErrorInfo | null>(null);

  // Keys are kept for retries of one attempt and renewed after success.
  const [keys, setKeys] = useState(() => ({
    item: newIdempotencyKey(),
    close: newIdempotencyKey(),
    seal: newIdempotencyKey(),
    cancel: newIdempotencyKey(),
  }));
  const renew = (k: keyof typeof keys) => setKeys((prev) => ({ ...prev, [k]: newIdempotencyKey() }));

  const load = useCallback(async () => {
    try {
      const detail = await manifestService.get(manifestId);
      setManifest(detail.manifest);
      setItems(detail.items ?? []);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The manifest could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [manifestId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const openMode = (next: Mode) => {
    if (!manifest) return;
    setActionError(null);
    if (next === "edit") {
      setEdit({
        routeReference: manifest.routeReference,
        vehicleReference: manifest.vehicleReference,
        providerMembershipId: manifest.providerMembershipId,
        sealNumber: manifest.sealNumber,
      });
    }
    if (next === "seal") setSeal({ sealNumber: "", reasonCode: "" });
    if (next === "cancel") setCancelReason("");
    setMode(next);
  };

  const act = async (fn: () => Promise<Manifest>, message: string, key?: keyof typeof keys) => {
    setBusy(true);
    setActionError(null);
    try {
      setManifest(await fn());
      if (key) renew(key);
      setMode("none");
      setNotice(message);
    } catch (err) {
      setActionError(describeApiError(err, "The manifest could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = (e: FormEvent) => {
    e.preventDefault();
    if (!manifest) return;
    act(
      () =>
        manifestService.update(
          manifest.id,
          {
            routeReference: edit.routeReference.trim(),
            vehicleReference: edit.vehicleReference.trim(),
            providerMembershipId: edit.providerMembershipId.trim(),
            sealNumber: edit.sealNumber.trim(),
          },
          manifest.version,
        ),
      "Manifest updated.",
    );
  };

  const amendSeal = (e: FormEvent) => {
    e.preventDefault();
    if (!manifest) return;
    act(() => manifestService.amendSeal(manifest.id, seal.sealNumber.trim(), seal.reasonCode.trim(), manifest.version, keys.seal), "Seal amended.", "seal");
  };

  const cancel = (e: FormEvent) => {
    e.preventDefault();
    if (!manifest) return;
    act(() => manifestService.cancel(manifest.id, cancelReason.trim(), manifest.version, keys.cancel), "Manifest cancelled.", "cancel");
  };

  const close = () => {
    if (!manifest) return;
    act(() => manifestService.close(manifest.id, manifest.version, keys.close), "Manifest closed. Its contents are now locked.", "close");
  };

  const addItem = async (e: FormEvent) => {
    e.preventDefault();
    if (!manifest) return;
    setAddingItem(true);
    setItemError(null);
    try {
      await manifestService.addItem(manifest.id, item.itemType, item.itemId.trim(), keys.item);
      renew("item");
      setItem({ ...item, itemId: "" });
      setNotice("Item added to the manifest.");
      await load();
    } catch (err) {
      setItemError(describeApiError(err, "The item could not be added."));
    } finally {
      setAddingItem(false);
    }
  };

  if (loading) return <Loading label="Loading manifest..." />;
  if (!manifest) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <Link href="/private-ops/manifests" className="text-sm text-primary">Back to manifests</Link>
      </div>
    );
  }

  const editable = manifest.status === "DRAFT" || manifest.status === "OPEN";
  const cancellable = editable || manifest.status === "CLOSED";

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={`Manifest ${manifest.manifestReference}`}
        description={`Created ${formatWhen(manifest.createdAt)}`}
        actions={
          <>
            {editable && mode !== "edit" && (
              <SecondaryButton onClick={() => openMode("edit")}>
                <Edit3 className="h-3.5 w-3.5" /> Edit manifest
              </SecondaryButton>
            )}
            <SecondaryButton onClick={() => router.push("/private-ops/manifests")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </SecondaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <KeyValueGrid
        items={[
          ["Status", <StatusPill key="s" status={manifest.status} />],
          ["Branch", manifest.branchId],
          ["Route", manifest.routeReference || "—"],
          ["Vehicle", manifest.vehicleReference || "—"],
          ["Provider membership", manifest.providerMembershipId || "—"],
          ["Seal number", manifest.sealNumber || "—"],
          ["Closed", formatWhen(manifest.closedAt)],
          ["Dispatched", formatWhen(manifest.dispatchedAt)],
          ["Version", manifest.version],
        ]}
      />

      {(editable || cancellable) && (
        <Panel title="Actions" description="Closing locks the contents and records a snapshot. A closed manifest's seal can be amended with a reason.">
          <ApiErrorNotice error={actionError} />
          <div className="flex flex-wrap gap-2">
            {editable && (
              <PrimaryButton onClick={close} disabled={busy || items.length === 0} title={items.length === 0 ? "Add at least one item first." : undefined}>
                <Lock className="h-3.5 w-3.5" /> {busy && mode === "none" ? "Closing..." : "Close manifest"}
              </PrimaryButton>
            )}
            {manifest.status === "CLOSED" && (
              <SecondaryButton onClick={() => openMode("seal")} disabled={busy}>
                <ShieldCheck className="h-3.5 w-3.5" /> Amend seal
              </SecondaryButton>
            )}
            {cancellable && (
              <DangerButton onClick={() => openMode("cancel")} disabled={busy}>
                <XCircle className="h-3.5 w-3.5" /> Cancel manifest
              </DangerButton>
            )}
          </div>
        </Panel>
      )}

      {mode === "edit" && (
        <Panel title="Edit manifest" onClose={() => setMode("none")}>
          <form onSubmit={saveEdit} className="space-y-4">
            <ApiErrorNotice error={actionError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Route reference">
                <input value={edit.routeReference} onChange={(e) => setEdit({ ...edit, routeReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Vehicle reference">
                <input value={edit.vehicleReference} onChange={(e) => setEdit({ ...edit, vehicleReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Provider membership ID">
                <input value={edit.providerMembershipId} onChange={(e) => setEdit({ ...edit, providerMembershipId: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Seal number">
                <input value={edit.sealNumber} onChange={(e) => setEdit({ ...edit, sealNumber: e.target.value })} className={INPUT} />
              </Field>
            </div>
            <PrimaryButton type="submit" disabled={busy}>
              <Save className="h-3.5 w-3.5" /> {busy ? "Saving..." : "Save manifest"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {mode === "seal" && (
        <Panel title="Amend seal" onClose={() => setMode("none")}>
          <form onSubmit={amendSeal} className="space-y-4">
            <ApiErrorNotice error={actionError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="New seal number" required>
                <input required value={seal.sealNumber} onChange={(e) => setSeal({ ...seal, sealNumber: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Reason code" required>
                <input required value={seal.reasonCode} onChange={(e) => setSeal({ ...seal, reasonCode: e.target.value })} className={INPUT} />
              </Field>
            </div>
            <PrimaryButton type="submit" disabled={busy}>
              <ShieldCheck className="h-3.5 w-3.5" /> {busy ? "Saving..." : "Amend seal"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {mode === "cancel" && (
        <Panel title="Cancel manifest" description="Cancelling releases its items." onClose={() => setMode("none")}>
          <form onSubmit={cancel} className="space-y-4">
            <ApiErrorNotice error={actionError} />
            <Field label="Reason code" required>
              <input required value={cancelReason} onChange={(e) => setCancelReason(e.target.value)} className={INPUT} />
            </Field>
            <DangerButton type="submit" disabled={busy}>
              <XCircle className="h-3.5 w-3.5" /> {busy ? "Cancelling..." : "Cancel manifest"}
            </DangerButton>
          </form>
        </Panel>
      )}

      <Panel title="Items" description="Shipments, packages and order stops on this manifest.">
        {editable && (
          <form onSubmit={addItem} className="space-y-3 rounded-xl border border-line p-4">
            <ApiErrorNotice error={itemError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label="Item type" required>
                <select value={item.itemType} onChange={(e) => setItem({ ...item, itemType: e.target.value })} className={INPUT}>
                  {MANIFEST_ITEM_TYPES.map((t) => <option key={t} value={t}>{humanize(t)}</option>)}
                </select>
              </Field>
              <div className="sm:col-span-2">
                <Field label="Item ID" required>
                  <input required value={item.itemId} onChange={(e) => setItem({ ...item, itemId: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={addingItem}>
              <Plus className="h-3.5 w-3.5" /> {addingItem ? "Adding..." : "Add item"}
            </PrimaryButton>
          </form>
        )}
        {items.length === 0 ? (
          <p className="text-sm text-ink-500">No items on this manifest yet.</p>
        ) : (
          <DataTable
            minWidth={640}
            rows={items}
            rowKey={(i) => i.id}
            onRowClick={(i) => {
              if (i.itemType === "SHIPMENT") router.push(`/private-ops/shipments/${i.itemId}`);
              else if (i.itemType === "PACKAGE") router.push(`/private-ops/packages/${i.itemId}`);
            }}
            columns={[
              { header: "Type", cell: (i) => humanize(i.itemType) },
              { header: "Item", cell: (i) => <span className="font-mono">{i.itemId}</span> },
              { header: "Shipment", cell: (i) => <span className="font-mono">{i.shipmentId || "—"}</span> },
              { header: "Added", cell: (i) => formatWhen(i.createdAt) },
            ]}
          />
        )}
      </Panel>
    </div>
  );
}
