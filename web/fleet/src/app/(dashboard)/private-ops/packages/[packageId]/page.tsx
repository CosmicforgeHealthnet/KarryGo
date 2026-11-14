"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { KeyValueGrid, Loading, PageHeader, SecondaryButton, StatusPill, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { PackageView } from "@/lib/types/operations";
import { CustodyPanel } from "../../custody-panel";

export default function PackageDetailPage() {
  const params = useParams();
  const router = useRouter();
  const packageId = params.packageId as string;
  const [pkg, setPkg] = useState<PackageView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  useEffect(() => {
    privateOpsService
      .getPackage(packageId)
      .then(setPkg)
      .catch((e) => setError(describeApiError(e, "The package could not be loaded.")))
      .finally(() => setLoading(false));
  }, [packageId]);

  if (loading) return <Loading label="Loading package..." />;

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={pkg ? `Package ${pkg.packageCode}` : "Package"}
        actions={
          <SecondaryButton onClick={() => (pkg ? router.push(`/private-ops/shipments/${pkg.shipmentId}`) : router.push("/private-ops/packages"))}>
            <ArrowLeft className="h-3.5 w-3.5" /> Back
          </SecondaryButton>
        }
      />
      <ApiErrorNotice error={error} />
      {pkg && (
        <>
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={pkg.status} />],
              ["Shipment", pkg.shipmentId],
              ["Type", humanize(pkg.packageType)],
              ["Condition", humanize(pkg.condition)],
              ["Barcode", pkg.barcodeValue],
              ["Created", formatWhen(pkg.createdAt)],
            ]}
          />
          <CustodyPanel subjectType="PACKAGE" subjectReference={pkg.id} />
        </>
      )}
    </div>
  );
}
