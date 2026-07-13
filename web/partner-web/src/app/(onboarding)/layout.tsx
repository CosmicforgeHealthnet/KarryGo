"use client";

import { usePathname, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ProgressBar } from "@/components/onboarding/progress-bar";

const STEP_ORDER = ["/business-info", "/business-logo", "/profile-photo"];
const BACK_TARGET: Record<string, string> = {
  "/business-info": "/otp-confirmation",
  "/business-logo": "/business-info",
  "/profile-photo": "/business-logo",
};

export default function OnboardingLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const stepIndex = STEP_ORDER.indexOf(pathname);
  const currentStep = stepIndex === -1 ? 1 : stepIndex + 1;

  return (
    <div className="flex min-h-screen justify-center bg-surface px-6 py-12">
      <div className="w-full max-w-md">
        <ProgressBar currentStep={currentStep} />

        <button
          type="button"
          onClick={() => router.push(BACK_TARGET[pathname] ?? "/business-info")}
          aria-label="Back"
          className="mt-6 flex h-8 w-8 items-center justify-center text-ink-900"
        >
          <ArrowLeft className="h-5 w-5" />
        </button>

        <div className="mt-2">{children}</div>
      </div>
    </div>
  );
}