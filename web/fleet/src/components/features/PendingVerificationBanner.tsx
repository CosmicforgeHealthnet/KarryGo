// src/components/features/PendingVerificationBanner.tsx

"use client";

import Link from "next/link";
import { Clock, AlertCircle } from "lucide-react";
import type { OrganizationAccess } from "@/lib/auth/organizationDestination";

interface PendingVerificationBannerProps {
  organizationName?: string;
  // The company's lifecycle access; the banner is not shown for operational
  // companies.
  access?: OrganizationAccess;
}

const COPY: Partial<Record<OrganizationAccess, { title: string; body: string; badge: string }>> = {
  pending: {
    title: "Your company is under review.",
    body: "You can explore Karryam Fleet while we verify your company. Operational features will be available once your company is approved.",
    badge: "Pending Review",
  },
  restricted: {
    title: "Your company is not active.",
    body: "Operational features are unavailable for this company. Contact support if you need help.",
    badge: "Restricted",
  },
  unknown: {
    title: "We could not confirm your company's status.",
    body: "Operational features stay unavailable until your company's status is confirmed.",
    badge: "Restricted",
  },
};

export function PendingVerificationBanner({ organizationName, access = "pending" }: PendingVerificationBannerProps) {
  const copy = COPY[access] ?? COPY.pending!;
  return (
    <div
      role="status"
      aria-label={organizationName ? `${organizationName}: ${copy.badge}` : copy.badge}
      className="rounded-lg border border-amber/30 bg-amber-bg/40 px-4 py-4 sm:px-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3 sm:items-center">
          <div className="rounded-full bg-amber/20 p-2">
            <Clock className="h-5 w-5 text-amber" />
          </div>
          <div>
            <h4 className="text-sm font-semibold text-amber">
              {copy.title}
            </h4>
            <p className="mt-0.5 text-sm text-amber/80">
              {copy.body}
            </p>
          </div>
        </div>
        <div className="flex flex-shrink-0 items-center gap-3">
          {access === "pending" && (
            <Link href="/company" className="text-xs font-semibold text-amber hover:underline">
              View submission
            </Link>
          )}
          <span className="inline-flex items-center gap-1.5 rounded-full bg-amber/20 px-3 py-1 text-xs font-medium text-amber">
            <AlertCircle className="h-3.5 w-3.5" />
            {copy.badge}
          </span>
        </div>
      </div>
    </div>
  );
}
