"use client";

import { useState } from "react";
import { CheckCircle2, Send, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { DangerButton, Panel, PrimaryButton } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { Order } from "@/lib/types/operations";

// Submit a draft for approval, or approve / send back an order awaiting
// approval. Each command carries the order version the user saw and its own
// idempotency key, renewed only after it succeeds.
export function OrderActions({ order, onChanged }: { order: Order; onChanged: (order: Order, message: string) => void }) {
  const [keys, setKeys] = useState({ submit: newIdempotencyKey(), approve: newIdempotencyKey(), reject: newIdempotencyKey() });
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const run = async (command: "submit" | "approve" | "reject", message: string) => {
    setBusy(true);
    setError(null);
    try {
      const updated = await privateOpsService.orderCommand(order.id, command, order.version, keys[command], command === "reject" ? reason.trim() : undefined);
      setKeys((k) => ({ ...k, [command]: newIdempotencyKey() }));
      setReason("");
      onChanged(updated, message);
    } catch (e) {
      setError(describeApiError(e, "The order could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  if (order.status === "DRAFT") {
    return (
      <Panel title="Submit for approval" description="A submitted order is locked for review until it is approved or sent back.">
        <ApiErrorNotice error={error} />
        <PrimaryButton disabled={busy} onClick={() => run("submit", "Order submitted for approval.")}>
          <Send className="h-3.5 w-3.5" /> Submit order
        </PrimaryButton>
      </Panel>
    );
  }

  if (order.status === "AWAITING_APPROVAL") {
    return (
      <Panel title="Approval" description="Approving confirms the order; sending it back returns it to draft.">
        <ApiErrorNotice error={error} />
        <div className="flex flex-wrap items-end gap-2">
          <PrimaryButton disabled={busy} onClick={() => run("approve", "Order approved.")}>
            <CheckCircle2 className="h-3.5 w-3.5" /> Approve order
          </PrimaryButton>
          <div className="min-w-[220px] flex-1">
            <Field label="Reason for sending back" required>
              <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="missing_stop_details" className={INPUT} />
            </Field>
          </div>
          <DangerButton disabled={busy || reason.trim() === ""} onClick={() => run("reject", "Order sent back to draft.")}>
            <XCircle className="h-3.5 w-3.5" /> Reject
          </DangerButton>
        </div>
      </Panel>
    );
  }

  return null;
}
