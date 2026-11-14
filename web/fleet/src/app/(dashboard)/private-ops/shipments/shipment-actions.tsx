"use client";

import { useState } from "react";
import { CheckCircle2, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT } from "@/components/features/FormField";
import { DangerButton, PrimaryButton } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { Shipment } from "@/lib/types/operations";

// Mark a draft shipment ready for planning, or cancel it. Both commands are
// idempotent and carry the shipment version the user saw.
export function ShipmentActions({ shipment, onChanged }: { shipment: Shipment; onChanged: (s: Shipment, message: string) => void }) {
  const [keys, setKeys] = useState({ ready: newIdempotencyKey(), cancel: newIdempotencyKey() });
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const run = async (command: "ready" | "cancel", message: string) => {
    setBusy(true);
    setError(null);
    try {
      const updated = await privateOpsService.shipmentCommand(shipment.id, command, shipment.version, keys[command], command === "cancel" ? reason.trim() : undefined);
      setKeys((k) => ({ ...k, [command]: newIdempotencyKey() }));
      onChanged(updated, message);
    } catch (e) {
      setError(describeApiError(e, "The shipment could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  const canCancel = !["COMPLETED", "CANCELLED", "FAILED", "PARTIALLY_COMPLETED", "IN_TRANSIT"].includes(shipment.status);
  if (shipment.status !== "DRAFT" && !canCancel) return null;

  return (
    <div className="space-y-2">
      <ApiErrorNotice error={error} />
      <div className="flex flex-wrap items-center gap-2">
        {shipment.status === "DRAFT" && (
          <PrimaryButton disabled={busy} onClick={() => run("ready", "Shipment marked ready.")}>
            <CheckCircle2 className="h-3.5 w-3.5" /> Mark ready
          </PrimaryButton>
        )}
        {canCancel && (
          <>
            <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Cancellation reason" aria-label="Cancellation reason" className={`${INPUT} max-w-[220px]`} />
            <DangerButton disabled={busy || reason.trim() === ""} onClick={() => run("cancel", "Shipment cancelled.")}>
              <XCircle className="h-3.5 w-3.5" /> Cancel shipment
            </DangerButton>
          </>
        )}
      </div>
    </div>
  );
}
