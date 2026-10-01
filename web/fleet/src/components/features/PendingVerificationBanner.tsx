// src/components/features/PendingVerificationBanner.tsx

"use client";

import { Clock, AlertCircle } from "lucide-react";

interface PendingVerificationBannerProps {
  organizationName?: string;
}

export function PendingVerificationBanner({ organizationName }: PendingVerificationBannerProps) {
  return (
    <div className="rounded-lg border border-amber/30 bg-amber-bg/40 px-4 py-4 sm:px-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3 sm:items-center">
          <div className="rounded-full bg-amber/20 p-2">
            <Clock className="h-5 w-5 text-amber" />
          </div>
          <div>
            <h4 className="text-sm font-semibold text-amber">
              {organizationName ? `${organizationName} is being verified` : 'Verification in progress'}
            </h4>
            <p className="mt-0.5 text-sm text-amber/80">
              Your organization is currently under review. You can view your dashboard, but actions are limited until verification is complete.
            </p>
          </div>
        </div>
        <div className="flex-shrink-0">
          <span className="inline-flex items-center gap-1.5 rounded-full bg-amber/20 px-3 py-1 text-xs font-medium text-amber">
            <AlertCircle className="h-3.5 w-3.5" />
            Pending Review
          </span>
        </div>
      </div>
    </div>
  );
}