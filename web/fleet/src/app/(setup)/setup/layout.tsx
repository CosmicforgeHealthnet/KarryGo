// src/app/(setup)/setup/layout.tsx

"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { usePathname, useRouter } from "next/navigation";
import Link from "next/link";
import { Check, ChevronRight, LayoutGrid, LogOut, X, Loader2, AlertTriangle, RefreshCw } from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";
import { KarryamLogo } from "@/components/brand/KarryamLogo";
import { ThemeToggle } from "@/components/ThemeToggle";
import {
  SETUP_STEPS,
  SETUP_HOME,
  CREATE_COMPANY_PATH,
  stepForPath,
  isStepComplete,
  type SetupStep,
} from "./steps";

const HEADER_HEIGHT_PX = 70;

// TODO: replace with the real support contact channel (mailbox, help-desk
// URL, or in-app chat trigger) once one exists.
const SUPPORT_EMAIL = "support@karrygo.com";

type LoadState = "loading" | "ready" | "error";
type StepIconStatus = "completed" | "current" | "pending";

function clearSessionRoutingCookies() {
  document.cookie = "karrygo_has_session=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
  document.cookie = "karrygo_org_status=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
}

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

export default function OnboardingLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [stageStatusError, setStageStatusError] = useState(false);
  const mobileDrawerRef = useRef<HTMLDivElement>(null);
  const hasLoadedOnce = useRef(false);
  // True for a freshly signed-up account that has no company yet.
  const needsCompany = useRef(false);

  const { _hasHydrated, accessToken, logout } = useAuthStore();
  const [signingOut, setSigningOut] = useState(false);

  // Ends the session (server + cookies) and returns to sign-in. Progress is
  // already saved, so the user can sign back in and carry on.
  const signOut = async () => {
    setSigningOut(true);
    await logout();
    useOrganizationStore.getState().clearOrganization();
    router.replace("/sign-in");
  };
  const { organization, setOrganization, clearOrganization } = useOrganizationStore();
  const { stageStatuses, setStageStatuses } = useOnboardingStore();

  // Keyed by backend stage name (OWNER_CONTACT, HEADQUARTERS, ...).
  const loadStageStatuses = useCallback(
    async (organizationId: string) => {
      setStageStatusError(false);
      try {
        const status = await organizationService.getOnboardingStatus(organizationId);
        const statusMap: Record<string, string> = {};
        status.stages.forEach((stage) => {
          statusMap[stage.stage] = stage.status;
        });
        setStageStatuses(statusMap);
      } catch (e) {
        console.error("Failed to fetch onboarding status:", e);
        setStageStatusError(true);
      }
    },
    [setStageStatuses]
  );

  const loadOrganization = useCallback(async () => {
    setLoadState("loading");
    try {
      const org = await organizationService.getCurrentOrganization();
      setOrganization(org);

      if (org.status !== "DRAFT") {
        router.replace("/");
        return;
      }

      // The company exists: the create form is done, open the saved details.
      if (window.location.pathname === CREATE_COMPANY_PATH) {
        router.replace(SETUP_STEPS[0].path);
      }

      setLoadState("ready");
      hasLoadedOnce.current = true;
      await loadStageStatuses(org.organizationId);
    } catch (error: any) {
      if (error?.response?.status === 401) {
        clearSessionRoutingCookies();
        router.replace("/sign-in");
        return;
      }
      // A new account has no organization membership until it creates its
      // company, so /organizations/current answers 403. That is the normal
      // first step of onboarding, not a failure.
      if (error?.response?.status === 403 && error?.response?.data?.error?.code === "forbidden") {
        needsCompany.current = true;
        clearOrganization();
        if (window.location.pathname !== CREATE_COMPANY_PATH) {
          // Stay on the loader until Create Company is showing.
          router.replace(CREATE_COMPANY_PATH);
          return;
        }
        setLoadState("ready");
        return;
      }
      console.error("Failed to fetch organization:", error);
      setLoadState("error");
    }
  }, [router, setOrganization, clearOrganization, loadStageStatuses]);

  useEffect(() => {
    if (!_hasHydrated) return;
    if (!accessToken) {
      router.replace("/sign-in");
      return;
    }
    loadOrganization();
    // Intentionally run only when hydration/auth state changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accessToken, _hasHydrated, router]);

  // Refresh step statuses on navigation, so finishing a step shows at once.
  useEffect(() => {
    // Redirected here because there is no company yet: show the form.
    if (needsCompany.current && pathname === CREATE_COMPANY_PATH) {
      setLoadState("ready");
      return;
    }
    // Once the company has been created, load it on the next step.
    if (needsCompany.current && pathname !== CREATE_COMPANY_PATH) {
      needsCompany.current = false;
      loadOrganization();
      return;
    }
    if (!hasLoadedOnce.current || !organization?.organizationId) return;
    loadStageStatuses(organization.organizationId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  useEffect(() => {
    setIsMobileMenuOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!isMobileMenuOpen) return;
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setIsMobileMenuOpen(false);
    };
    document.addEventListener("keydown", handleEscape);
    const frame = requestAnimationFrame(() => {
      if (mobileDrawerRef.current && !mobileDrawerRef.current.contains(document.activeElement)) {
        mobileDrawerRef.current.querySelector<HTMLElement>("button")?.focus();
      }
    });
    return () => {
      document.removeEventListener("keydown", handleEscape);
      cancelAnimationFrame(frame);
    };
  }, [isMobileMenuOpen]);

  if (!_hasHydrated || loadState === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-2">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!accessToken) {
    return null;
  }

  if (loadState === "error") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-2 px-4">
        <div className="w-full max-w-sm rounded-2xl border border-line bg-surface p-6 text-center shadow-sm">
          <div className="mx-auto mb-3 flex h-11 w-11 items-center justify-center rounded-full bg-red-bg">
            <AlertTriangle className="h-5 w-5 text-red" />
          </div>
          <h2 className="text-[15px] font-semibold text-ink-950">Couldn&apos;t load your account</h2>
          <p className="mt-1 text-[13px] text-ink-500">
            Something went wrong reaching the server. Check your connection and try again.
          </p>
          <button type="button" onClick={loadOrganization} className="btn btn-primary btn-md mt-4 w-full">
            <RefreshCw className="h-3.5 w-3.5" />
            Try again
          </button>
          <button
            type="button"
            onClick={async () => {
              await logout();
              router.replace("/sign-in");
            }}
            className="mt-2 w-full text-[12px] font-medium text-ink-500 transition hover:text-ink-700"
          >
            Sign out instead
          </button>
        </div>
      </div>
    );
  }

  const hasCompany = !!organization?.organizationId;
  const isOnboardingHome = pathname === SETUP_HOME;
  const activeStep = stepForPath(pathname);
  const workSteps = SETUP_STEPS.filter((s) => s.stages.length > 0);
  const completedCount = workSteps.filter((s) => isStepComplete(s, stageStatuses)).length;
  const activeIndex = activeStep ? SETUP_STEPS.indexOf(activeStep) : -1;

  const iconStatusFor = (step: SetupStep): StepIconStatus =>
    isStepComplete(step, stageStatuses) ? "completed" : step === activeStep ? "current" : "pending";
  // Before the company exists only the first step can open; after, any step.
  const isStepClickable = (step: SetupStep) => step === activeStep || hasCompany || step === SETUP_STEPS[0];
  const pathFor = (step: SetupStep) => (step === SETUP_STEPS[0] && !hasCompany ? CREATE_COMPANY_PATH : step.path);
  const openStep = (step: SetupStep) => {
    if (!isStepClickable(step) || step === activeStep) return;
    router.push(pathFor(step));
  };

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
                onClick={() => organization?.organizationId && loadStageStatuses(organization.organizationId)}
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
