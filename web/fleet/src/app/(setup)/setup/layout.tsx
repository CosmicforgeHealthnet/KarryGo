"use client";
import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuthStore } from "@/lib/stores/authStore";
import { KarryamLogo } from "@/components/brand/KarryamLogo";
import { ThemeToggle } from "@/components/ThemeToggle";
import { OnboardingProvider, useOnboarding } from "./onboarding-context";
import { Loading } from "./onboarding-ui";
import { SETUP_STEPS, stepForPath, isStepComplete } from "./steps";
function Shell({ children }: { children: ReactNode }) {
  const { data, editable } = useOnboarding();
  const path = usePathname();
  const router = useRouter();
  const current = stepForPath(path);
  const stages = Object.fromEntries(
    (data.status?.stages ?? []).map((s) => [s.stage, s.status]),
  );
  const done = SETUP_STEPS.filter((s) => isStepComplete(s, stages)).length;
  useEffect(() => {
    if (!data.organization && path !== "/setup/create-company")
      router.replace("/setup/create-company");
  }, [data.organization, path, router]);
  if (!data.organization && path !== "/setup/create-company")
    return <Loading />;
  return (
    <div className="mx-auto max-w-6xl px-4 py-5 sm:px-6 sm:py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <Link href="/setup" className="text-sm font-semibold text-ink-950">
          Company setup
        </Link>
        <span className="text-sm text-ink-500" role="status">
          {done} of 5 sections complete
          {current ? ` · Step ${SETUP_STEPS.indexOf(current) + 1} of 6` : ""}
        </span>
      </div>
      <div className="grid min-w-0 grid-cols-1 gap-6 md:grid-cols-[230px_minmax(0,1fr)]">
        <nav aria-label="Onboarding steps" className="min-w-0">
          <ol className="flex gap-2 overflow-x-auto pb-2 md:block md:space-y-2">
            {SETUP_STEPS.map((step, i) => (
              <li key={step.id} className="shrink-0">
                <Link
                  href={
                    !data.organization ? "/setup/create-company" : step.path
                  }
                  aria-current={current?.id === step.id ? "step" : undefined}
                  className={`flex items-center gap-3 rounded-xl px-3 py-3 text-sm font-semibold ${current?.id === step.id ? "bg-surface text-ink-950 ring-1 ring-line" : "text-ink-500 hover:bg-surface"}`}
                >
                  <span
                    className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full ${isStepComplete(step, stages) ? "bg-label-accent text-surface" : "border border-line"}`}
                    aria-label={
                      isStepComplete(step, stages) ? "Complete" : undefined
                    }
                  >
                    {isStepComplete(step, stages) ? "✓" : i + 1}
                  </span>
                  {step.label}
                </Link>
              </li>
            ))}
          </ol>
        </nav>
        <main className="min-w-0">
          {!editable && (
            <p className="mb-5 rounded-xl border border-line bg-surface p-4 text-sm">
              Company status:{" "}
              {data.organization?.status.replaceAll("_", " ").toLowerCase()}.
              Saved information is available for review.
            </p>
          )}
          {children}
        </main>
      </div>
    </div>
  );
}
export default function Layout({ children }: { children: ReactNode }) {
  const hydrated = useAuthStore((s) => s._hasHydrated);
  const authenticated = useAuthStore((s) => s.isAuthenticated);
  const identity = useAuthStore((s) => s.identityId);
  const selected = useAuthStore((s) => s.selectedOrganization?.organizationId);
  const router = useRouter();
  useEffect(() => {
    if (hydrated && !authenticated)
      router.replace("/sign-in?redirect=%2Fsetup");
  }, [hydrated, authenticated, router]);
  return (
    <div className="min-h-dvh bg-surface-2 text-ink-950">
      <header className="border-b border-line bg-surface">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-3 px-4 py-4">
          <Link href="/setup" aria-label="Karryam Fleet setup">
            <KarryamLogo />
          </Link>
          <div className="flex items-center gap-3">
            <ThemeToggle />
            <button
              className="text-sm font-semibold"
              onClick={async () => {
                await useAuthStore.getState().logout();
                router.replace("/sign-in");
              }}
            >
              Sign out
            </button>
          </div>
        </div>
      </header>
      {!hydrated || !authenticated ? (
        <Loading />
      ) : (
        <OnboardingProvider key={`${identity}:${selected ?? "auto"}`}>
          <Shell>{children}</Shell>
        </OnboardingProvider>
      )}
    </div>
  );
}
