"use client";

// Original onboarding visual shell retained; current onboarding provider remains authoritative.
import { useEffect, useRef, useState, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import Link from "next/link";
import { Check, ChevronRight, LayoutGrid, LogOut, X, Loader2, AlertTriangle } from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { KarryamLogo } from "@/components/brand/KarryamLogo";
import { ThemeToggle } from "@/components/ThemeToggle";
import { OnboardingProvider, useOnboarding } from "./onboarding-context";
import { Loading } from "./onboarding-ui";
import { belongsInSetup, organizationDestination } from "@/lib/auth/organizationDestination";
import {
  SETUP_STEPS, SETUP_HOME, CREATE_COMPANY_PATH, stepForPath,
  isStepComplete, type SetupStep,
} from "./steps";

const HEADER_HEIGHT_PX = 70;
const SUPPORT_EMAIL = "support@karryam.com";
type StepIconStatus = "completed" | "current" | "pending";
interface StepRowProps {
  step: SetupStep;
  index: number;
  iconStatus: StepIconStatus;
  isActivePage: boolean;
  isClickable: boolean;
  onSelect: () => void;
  variant: "mobile" | "desktop";
}

function StepRow({ step, index, iconStatus, isActivePage, isClickable, onSelect, variant }: StepRowProps) {
  const isDisabled = !isClickable;
  const done = iconStatus === "completed";

  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={isDisabled}
      aria-current={isActivePage ? "step" : undefined}
      className={`relative flex w-full items-center gap-3.5 rounded-xl px-3 py-2.5 text-left transition ${
        isActivePage ? "bg-surface shadow-[0_1px_3px_rgba(11,31,68,0.08)] ring-1 ring-line" : isDisabled ? "" : "hover:bg-surface"
      } ${isDisabled ? "cursor-not-allowed" : "cursor-pointer"}`}
    >
      <span
        className={`relative z-10 flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-[13px] font-bold ${
          done
            ? "bg-label-accent text-surface"
            : isActivePage
            ? "bg-ink-950 text-surface"
            : "border border-field-border bg-surface text-ink-500"
        }`}
      >
        {done ? <Check className="h-4 w-4" strokeWidth={3} /> : index + 1}
      </span>
      <span className={`min-w-0 flex-1 text-[14px] font-bold ${isDisabled && !done ? "text-ink-400" : "text-ink-950"}`}>
        {step.label}
      </span>
      {variant === "mobile" && isClickable && <ChevronRight className="h-4 w-4 flex-shrink-0 text-ink-400" />}
    </button>
  );
}


function OnboardingShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { data, reload } = useOnboarding();
  const organization = data.organization;
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const mobileDrawerRef = useRef<HTMLDivElement>(null);
  const stageStatuses: Record<string, string> = Object.fromEntries(
    (data.status?.stages ?? []).map((stage) => [stage.stage, stage.status])
  );
  const stageStatusError = !!organization && !data.status;
  const loadStageStatuses = () => { void reload(); };
  const hasCompany = !!organization?.organizationId;
  const isOnboardingHome = pathname === SETUP_HOME;
  const activeStep = stepForPath(pathname);
  const workSteps = SETUP_STEPS.filter((s) => s.stages.length > 0);
  const completedCount = workSteps.filter((s) => isStepComplete(s, stageStatuses)).length;
  const activeIndex = activeStep ? SETUP_STEPS.indexOf(activeStep) : -1;
  const iconStatusFor = (step: SetupStep): StepIconStatus =>
    isStepComplete(step, stageStatuses) ? "completed" : step === activeStep ? "current" : "pending";
  const isStepClickable = (step: SetupStep) =>
    step === activeStep || hasCompany || step === SETUP_STEPS[0];
  const pathFor = (step: SetupStep) =>
    step === SETUP_STEPS[0] && !hasCompany ? CREATE_COMPANY_PATH : step.path;
  const openStep = (step: SetupStep) => {
    if (!isStepClickable(step) || step === activeStep) return;
    router.push(pathFor(step));
  };
  useEffect(() => {
    if (!hasCompany && pathname !== CREATE_COMPANY_PATH && pathname !== SETUP_HOME)
      router.replace(CREATE_COMPANY_PATH);
  }, [hasCompany, pathname, router]);
  // Load the next step's code once the browser is idle, so continuing does not
  // wait on a network round trip. (The previous step is already prefetched by
  // the Back link; prefetching it again would only compete with that request.)
  // Rendering is unchanged.
  useEffect(() => {
    const next = SETUP_STEPS[activeIndex + 1];
    if (activeIndex < 0 || !next) return;
    const idle = window.requestIdleCallback ?? ((fn: () => void) => window.setTimeout(fn, 200));
    const cancel = window.cancelIdleCallback ?? window.clearTimeout;
    const handle = idle(() => router.prefetch(next.path));
    return () => cancel(handle);
  }, [activeIndex, router]);
  // Close the step drawer when the page changes (adjusted during render, as
  // React recommends, rather than in an effect).
  const [menuPath, setMenuPath] = useState(pathname);
  if (menuPath !== pathname) {
    setMenuPath(pathname);
    setIsMobileMenuOpen(false);
  }
  useEffect(() => {
    if (!isMobileMenuOpen) return;
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape") setIsMobileMenuOpen(false);
    };
    document.addEventListener("keydown", handleEscape);
    const frame = requestAnimationFrame(() => {
      if (mobileDrawerRef.current && !mobileDrawerRef.current.contains(document.activeElement))
        mobileDrawerRef.current.querySelector<HTMLElement>("button")?.focus();
    });
    return () => {
      document.removeEventListener("keydown", handleEscape);
      cancelAnimationFrame(frame);
    };
  }, [isMobileMenuOpen]);
  const signOut = async () => {
    if (signingOut) return;
    setSigningOut(true);
    try {
      await useAuthStore.getState().logout();
      useOrganizationStore.getState().clearOrganization();
      router.replace("/sign-in");
    } finally { setSigningOut(false); }
  };
  // A submitted, active or otherwise post-onboarding company never stays in
  // setup, whichever setup URL was opened: it goes where the shared lifecycle
  // policy says, from the status the provider just read from the server.
  if (organization && !belongsInSetup(organization.status))
    return <LeaveSetup to={organizationDestination(organization.status)} />;

  const header = (
    <header
      className="sticky top-0 z-20 flex items-center justify-between border-b border-line bg-surface/80 px-4 backdrop-blur-xl sm:px-6"
      style={{ height: HEADER_HEIGHT_PX }}
    >
      <Link href={SETUP_HOME} aria-label="Setup overview" className="flex items-center">
        <KarryamLogo height={26} />
      </Link>
      <div className="flex items-center gap-2">
        <ThemeToggle />
        <button type="button" onClick={signOut} disabled={signingOut} className="btn btn-secondary btn-sm" aria-label="Sign out">
          {signingOut ? <Loader2 className="animate-spin" /> : <LogOut />}
          <span className="hidden sm:inline">Sign out</span>
        </button>
      </div>
    </header>
  );

  const stepList = (variant: "mobile" | "desktop") => (
    <>
      <Link
        href={SETUP_HOME}
        className={`flex items-center gap-3.5 rounded-xl px-3 py-2.5 text-[14px] font-bold transition ${
          isOnboardingHome ? "bg-surface text-ink-950 ring-1 ring-line" : "text-ink-700 hover:bg-surface"
        }`}
      >
        <span className="flex h-8 w-8 items-center justify-center rounded-full bg-surface-3 text-ink-700">
          <LayoutGrid className="h-4 w-4" />
        </span>
        Overview
      </Link>
      <div className="relative mt-2">
        <span className="absolute bottom-6 left-[27px] top-6 w-px bg-line" aria-hidden="true" />
        <div className="relative space-y-1">
          {SETUP_STEPS.map((step, index) => (
            <StepRow
              key={step.id}
              step={step}
              index={index}
              iconStatus={iconStatusFor(step)}
              isActivePage={step === activeStep}
              isClickable={isStepClickable(step)}
              onSelect={() => openStep(step)}
              variant={variant}
            />
          ))}
        </div>
      </div>
    </>
  );

  if (isOnboardingHome) {
    return (
      <div className="onboarding min-h-screen bg-surface-2">
        {header}
        <main className="mx-auto max-w-3xl px-4 py-8 sm:px-6 lg:py-12">{children}</main>
      </div>
    );
  }

  return (
    <div className="onboarding min-h-screen bg-surface-2">
      {header}

      {activeStep && (
        <button
          type="button"
          onClick={() => setIsMobileMenuOpen(true)}
          className="sticky z-10 flex w-full items-center gap-3 border-b border-line bg-surface px-4 py-3 text-left lg:hidden"
          style={{ top: HEADER_HEIGHT_PX }}
          aria-label="Show all setup steps"
        >
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[12px] font-bold text-label-accent">
              Step {activeIndex + 1} of {SETUP_STEPS.length} · {activeStep.label}
            </span>
            <span className="mt-1.5 flex gap-1" aria-hidden="true">
              {SETUP_STEPS.map((step, index) => (
                <span
                  key={step.id}
                  className={`h-1 flex-1 rounded-full ${
                    isStepComplete(step, stageStatuses) || index === activeIndex ? "bg-primary" : "bg-surface-3"
                  }`}
                />
              ))}
            </span>
          </span>
          <span className="text-[12px] font-bold text-ink-700">All steps</span>
          <ChevronRight className="h-4 w-4 text-ink-400" />
        </button>
      )}

      {isMobileMenuOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm lg:hidden"
          onClick={() => setIsMobileMenuOpen(false)}
          aria-hidden="true"
        />
      )}

      <div
        ref={mobileDrawerRef}
        role="dialog"
        aria-modal="true"
        aria-label="Setup steps"
        className={`fixed right-0 top-0 z-40 h-full w-80 max-w-[88vw] transform bg-surface-2 shadow-xl transition-transform duration-300 ease-in-out lg:hidden ${
          isMobileMenuOpen ? "translate-x-0" : "pointer-events-none translate-x-full"
        }`}
      >
        <div className="flex items-center justify-between border-b border-line px-4" style={{ height: HEADER_HEIGHT_PX }}>
          <span className="font-bold text-ink-950">
            {completedCount} of {workSteps.length} steps done
          </span>
          <button
            onClick={() => setIsMobileMenuOpen(false)}
            aria-label="Close"
            className="flex h-8 w-8 items-center justify-center rounded-lg transition-colors hover:bg-surface"
          >
            <X className="h-4 w-4 text-ink-600" />
          </button>
        </div>
        <div className="p-3">{stepList("mobile")}</div>
      </div>

      <div className="flex">
        <aside
          className="no-scrollbar sticky hidden w-[300px] flex-shrink-0 flex-col overflow-y-auto border-r border-line bg-surface-2 px-5 py-8 lg:flex"
          style={{ top: HEADER_HEIGHT_PX, height: `calc(100vh - ${HEADER_HEIGHT_PX}px)` }}
        >
          <div className="px-3">
            <p className="text-[15px] font-bold text-ink-950">
              {completedCount} of {workSteps.length} steps done
            </p>
            <div className="mt-3 flex gap-1.5" aria-hidden="true">
              {workSteps.map((step) => (
                <span
                  key={step.id}
                  className={`h-1.5 flex-1 rounded-full ${isStepComplete(step, stageStatuses) ? "bg-primary" : "bg-surface-3"}`}
                />
              ))}
            </div>
            {stageStatusError && (
              <button
                type="button"
                onClick={() => organization?.organizationId && loadStageStatuses()}
                className="mt-2 flex items-center gap-1 text-xs text-red hover:underline"
              >
                <AlertTriangle className="h-3 w-3" />
                Progress may be out of date. Retry
              </button>
            )}
          </div>

          <nav className="mt-6" aria-label="Setup steps">
            {stepList("desktop")}
          </nav>

          <div className="mt-auto rounded-2xl border border-line bg-surface p-4">
            <p className="text-[13px] font-bold text-ink-950">Need help?</p>
            <p className="mt-1 text-[13px] leading-relaxed text-ink-500">Our team can help you finish setting up.</p>
            <a href={`mailto:${SUPPORT_EMAIL}`} className="mt-2 inline-block text-[13px] font-bold text-label-accent hover:underline">
              Contact support
            </a>
          </div>
        </aside>

        <main className="min-w-0 flex-1 px-4 pb-16 pt-6 sm:px-8 lg:px-12 lg:pt-12">
          <div className="mx-auto w-full max-w-[860px]">{children}</div>
        </main>
      </div>
    </div>
  );
}

function LeaveSetup({ to }: { to: string }) {
  const router = useRouter();
  useEffect(() => {
    router.replace(to);
  }, [router, to]);
  return (
    <div className="min-h-dvh bg-surface-2">
      <Loading />
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
  if (!hydrated || !authenticated)
    return <div className="min-h-dvh bg-surface-2"><Loading /></div>;
  return (
    <OnboardingProvider key={`${identity}:${selected ?? "auto"}`}>
      <OnboardingShell>{children}</OnboardingShell>
    </OnboardingProvider>
  );
}
