"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { DangerButton, KeyValueGrid, Loading, Notice, PageHeader, Panel, PrimaryButton, SecondaryButton, StatusPill, formatWhen } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { assetCareService } from "@/lib/services/assetCareService";
import type { Tyre } from "@/lib/types/assetCare";

type Action = "install" | "remove" | "rotate" | "tread-readings";
const num = (v: string) => (v.trim() === "" ? undefined : Number(v));

export default function TyreDetailPage() {
  const params = useParams();
  const router = useRouter();
  const tyreId = params.tyreId as string;
  const [tyre, setTyre] = useState<Tyre | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState({ assetId: "", position: "", toPosition: "", reason: "", scrap: false, treadDepthMm: "", odometer: "" });
  const [keys, setKeys] = useState<Record<Action, string>>({
    install: newIdempotencyKey(),
    remove: newIdempotencyKey(),
    rotate: newIdempotencyKey(),
    "tread-readings": newIdempotencyKey(),
  });

  const load = useCallback(async () => {
    try {
      setTyre(await assetCareService.getTyre(tyreId));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The tyre could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [tyreId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const act = async (action: Action, message: string) => {
    if (!tyre) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const updated = await assetCareService.tyreAction(
        tyre.id,
        action,
        {
          assetId: action === "install" ? form.assetId.trim() : undefined,
          position: action === "install" ? form.position.trim() : undefined,
          toPosition: action === "rotate" ? form.toPosition.trim() : undefined,
          reason: form.reason.trim() || undefined,
          scrap: action === "remove" ? form.scrap : undefined,
          treadDepthMm: action === "tread-readings" ? num(form.treadDepthMm) : undefined,
          odometer: num(form.odometer),
        },
        tyre.version,
        keys[action],
      );
      setKeys((k) => ({ ...k, [action]: newIdempotencyKey() }));
      setTyre(updated);
      setNotice(updated.lowTreadDefectId ? `${message} Tread is below the minimum; defect ${updated.lowTreadDefectId} was opened.` : message);
    } catch (e) {
      setError(describeApiError(e, "The tyre could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Loading label="Loading tyre..." />;

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      <PageHeader
        title={tyre ? `Tyre ${tyre.serialNumber}` : "Tyre"}
        description={tyre ? [tyre.brand, tyre.size].filter(Boolean).join(" · ") : undefined}
        actions={
          <SecondaryButton onClick={() => router.push("/fleet/tyres")}>
            <ArrowLeft className="h-3.5 w-3.5" /> Back
          </SecondaryButton>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      {tyre && (
        <>
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={tyre.status} />],
              ["Asset", tyre.assetId ? <Link key="a" className="text-primary" href={`/fleet/assets/${tyre.assetId}`}>{tyre.assetId}</Link> : "—"],
              ["Position", tyre.position || "—"],
              ["Installed", formatWhen(tyre.installedAt)],
              ["Last tread", tyre.lastTreadDepthMm != null ? `${tyre.lastTreadDepthMm} mm` : "—"],
              ["Minimum tread", `${tyre.minTreadDepthMm} mm`],
            ]}
          />
          <Panel title="Record a change">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              {tyre.status !== "INSTALLED" ? (
                <>
                  <Field label="Asset ID">
                    <input value={form.assetId} onChange={(e) => setForm({ ...form, assetId: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label="Position">
                    <input value={form.position} onChange={(e) => setForm({ ...form, position: e.target.value })} placeholder="FRONT_LEFT" className={INPUT} />
                  </Field>
                </>
              ) : (
                <>
                  <Field label="Rotate to position">
                    <input value={form.toPosition} onChange={(e) => setForm({ ...form, toPosition: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label="Tread depth (mm)">
                    <input inputMode="decimal" value={form.treadDepthMm} onChange={(e) => setForm({ ...form, treadDepthMm: e.target.value })} className={INPUT} />
                  </Field>
                </>
              )}
              <Field label="Odometer">
                <input inputMode="decimal" value={form.odometer} onChange={(e) => setForm({ ...form, odometer: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Reason">
                <input value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} className={INPUT} />
              </Field>
              {tyre.status === "INSTALLED" && (
                <label className="flex items-center gap-2 self-end pb-2 text-sm text-ink-700">
                  <input type="checkbox" checked={form.scrap} onChange={(e) => setForm({ ...form, scrap: e.target.checked })} /> Scrap on removal
                </label>
              )}
            </div>
            <div className="flex flex-wrap gap-2">
              {tyre.status !== "INSTALLED" && tyre.status !== "SCRAPPED" && (
                <PrimaryButton disabled={busy || !form.assetId.trim() || !form.position.trim()} onClick={() => act("install", "Tyre installed.")}>Install</PrimaryButton>
              )}
              {tyre.status === "INSTALLED" && (
                <>
                  <PrimaryButton disabled={busy || form.treadDepthMm.trim() === ""} onClick={() => act("tread-readings", "Tread reading recorded.")}>Record tread</PrimaryButton>
                  <SecondaryButton disabled={busy || !form.toPosition.trim()} onClick={() => act("rotate", "Tyre rotated.")}>Rotate</SecondaryButton>
                  <DangerButton disabled={busy} onClick={() => act("remove", form.scrap ? "Tyre removed and scrapped." : "Tyre removed.")}>Remove</DangerButton>
                </>
              )}
            </div>
          </Panel>
        </>
      )}
    </div>
  );
}
