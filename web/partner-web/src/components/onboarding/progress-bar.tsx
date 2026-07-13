import { cn } from "@/lib/utils";

interface ProgressBarProps {
  currentStep: number; // 1-indexed
  totalSteps?: number;
}

export function ProgressBar({ currentStep, totalSteps = 3 }: ProgressBarProps) {
  return (
    <div className="flex gap-1.5">
      {Array.from({ length: totalSteps }).map((_, i) => (
        <div
          key={i}
          className={cn(
            "h-1.5 flex-1 rounded-full",
            i < currentStep ? "bg-brand-500" : "bg-brand-100"
          )}
        />
      ))}
    </div>
  );
}