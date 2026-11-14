"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ComplianceStatusList } from "@/components/features/ComplianceStatusList";

// Asset safety and compliance status (spec 19.3).
export default function AssetCompliancePage() {
  const params = useParams();
  const assetId = params.assetId as string;

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/fleet/assets/${assetId}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950">Safety and compliance</h1>
          <p className="mt-0.5 text-sm text-ink-600">This asset&apos;s status against each compliance requirement.</p>
        </div>
      </div>
      <ComplianceStatusList subject="ASSET" subjectId={assetId} />
    </div>
  );
}
