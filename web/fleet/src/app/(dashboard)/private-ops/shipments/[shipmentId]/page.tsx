"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import {
  DataTable,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  SecondaryButton,
  StatusPill,
  formatMinor,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { PackageView, Shipment } from "@/lib/types/operations";
import { CustodyPanel } from "../../custody-panel";
import { ShipmentActions } from "../shipment-actions";

export default function ShipmentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const shipmentId = params.shipmentId as string;
  const [shipment, setShipment] = useState<Shipment | null>(null);
  const [packages, setPackages] = useState<PackageView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setShipment(await privateOpsService.getShipment(shipmentId));
      privateOpsService.shipmentPackages(shipmentId).then(setPackages).catch(() => setPackages([]));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The shipment could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [shipmentId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  if (loading) return <Loading label="Loading shipment..." />;

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={`Shipment ${shipmentId}`}
        description={shipment ? `Updated ${formatWhen(shipment.updatedAt)}` : undefined}
        actions={
          <SecondaryButton onClick={() => router.push("/private-ops/shipments")}>
            <ArrowLeft className="h-3.5 w-3.5" /> Back
          </SecondaryButton>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      {shipment && (
        <>
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={shipment.status} />],
              ["Order", shipment.orderId],
              ["Packages", shipment.packageCount],
              ["Vehicle class", shipment.requiredVehicleClass || "—"],
              ["Capability", shipment.requiredServiceCapability || "—"],
              ["Declared value", formatMinor(shipment.declaredValueAmountMinor, shipment.declaredValueCurrency)],
              ["Origin stop", shipment.originStopId],
              ["Destination stop", shipment.destinationStopId],
              ["Version", shipment.version],
            ]}
          />
          <ShipmentActions
            shipment={shipment}
            onChanged={(s, message) => {
              setShipment(s);
              setNotice(message);
            }}
          />
          <Panel title="Packages">
            {packages.length === 0 ? (
              <p className="text-sm text-ink-500">No packages on this shipment.</p>
            ) : (
              <DataTable
                minWidth={640}
                rows={packages}
                rowKey={(p) => p.id}
                onRowClick={(p) => router.push(`/private-ops/packages/${p.id}`)}
                columns={[
                  { header: "Package", cell: (p) => <span className="font-mono">{p.packageCode}</span> },
                  { header: "Type", cell: (p) => humanize(p.packageType) },
                  { header: "Condition", cell: (p) => humanize(p.condition) },
                  { header: "Status", cell: (p) => <StatusPill status={p.status} /> },
                ]}
              />
            )}
          </Panel>
          <CustodyPanel subjectType="SHIPMENT" subjectReference={shipment.id} />
        </>
      )}
    </div>
  );
}
