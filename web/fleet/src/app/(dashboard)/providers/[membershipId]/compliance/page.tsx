// src/app/(dashboard)/providers/[membershipId]/compliance/page.tsx

"use client";

import { useParams } from "next/navigation";
import { ComplianceStatusList } from "@/components/features/ComplianceStatusList";

// Compliance tab: the provider's status against each requirement, as
// GET /providers/:membershipId/compliance reports it.
export default function ComplianceTab() {
  const params = useParams();
  const membershipId = params.membershipId as string;
  return <ComplianceStatusList subject="PROVIDER" subjectId={membershipId} />;
}
