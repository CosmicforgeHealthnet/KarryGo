"use client";

import { AlertCircle } from "lucide-react";
import type { ApiErrorInfo } from "@/lib/api/errors";

// The dashboard's inline error alert, with the reference support needs to
// trace the failure.
export function ApiErrorNotice({ error, className = "" }: { error: ApiErrorInfo | null; className?: string }) {
  if (!error) return null;
  const reference = [error.code, error.requestId].filter(Boolean).join(" · ");
  return (
    <div role="alert" className={`flex items-start gap-2 rounded-xl border border-red/30 bg-red-bg px-4 py-3 text-sm text-red ${className}`}>
      <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
      <div className="min-w-0">
        <p>{error.message}</p>
        {reference && <p className="mt-1 break-all text-xs">Reference: {reference}</p>}
      </div>
    </div>
  );
}
