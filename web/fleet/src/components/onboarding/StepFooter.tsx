"use client";

import { ArrowLeft, ArrowRight, Loader2 } from "lucide-react";

// The same two buttons at the bottom of every setup step: Back on the left,
// the step's main action on the right. On phones the main action sits on top.
export function StepFooter({
  onBack,
  backLabel = "Back",
  onPrimary,
  primaryLabel = "Save & continue",
  primaryDisabled = false,
  primaryLoading = false,
  loadingLabel = "Saving…",
  primaryType = "button",
}: {
  onBack?: () => void;
  backLabel?: string;
  onPrimary?: () => void;
  primaryLabel?: string;
  primaryDisabled?: boolean;
  primaryLoading?: boolean;
  loadingLabel?: string;
  primaryType?: "button" | "submit";
}) {
  return (
    <div className="flex flex-col-reverse gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
      {onBack ? (
        <button type="button" onClick={onBack} disabled={primaryLoading} className="btn btn-secondary btn-lg">
          <ArrowLeft />
          {backLabel}
        </button>
      ) : (
        <span />
      )}
      <button
        type={primaryType}
        onClick={onPrimary}
        disabled={primaryDisabled || primaryLoading}
        aria-busy={primaryLoading}
        className="btn btn-primary btn-lg sm:min-w-[200px]"
      >
        {primaryLoading ? (
          <>
            <Loader2 className="animate-spin" />
            {loadingLabel}
          </>
        ) : (
          <>
            {primaryLabel}
            <ArrowRight />
          </>
        )}
      </button>
    </div>
  );
}
