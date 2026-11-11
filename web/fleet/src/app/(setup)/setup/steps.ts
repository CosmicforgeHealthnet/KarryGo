// The company setup flow, in the order the backend checks it before a company
// can be submitted (see fleet-partner-service models/onboarding.go). Every
// setup page, the step list and Setup home read from here.

export type SetupStep = {
  id: string;
  label: string;
  description: string;
  path: string;
  // Backend stages this step completes; the step is done when all are COMPLETE.
  stages: string[];
};

export const SETUP_HOME = "/setup";
export const CREATE_COMPANY_PATH = "/setup/create-company";

export const SETUP_STEPS: SetupStep[] = [
  {
    id: "company",
    label: "Company details",
    description: "Legal name, registration and owner contact",
    // Before the company exists this step is the create form; afterwards it
    // opens the saved details for editing.
    path: "/setup/profile",
    stages: ["OWNER_CONTACT", "BUSINESS_PROFILE"],
  },
  {
    id: "address",
    label: "Business address",
    description: "Registered address and how to reach you",
    path: "/setup/headquarters",
    stages: ["HEADQUARTERS"],
  },
  {
    id: "representative",
    label: "Authorized representative",
    description: "The person who can act for the company",
    path: "/setup/authorized-representative",
    stages: ["AUTHORIZED_REPRESENTATIVE"],
  },
  {
    id: "services",
    label: "Services you offer",
    description: "The logistics services you will provide",
    path: "/setup/capabilities",
    stages: ["OPERATING_CAPABILITIES"],
  },
  {
    id: "documents",
    label: "Documents",
    description: "Registration, tax, insurance and licence",
    path: "/setup/business-docs",
    stages: ["BUSINESS_DOCUMENTS"],
  },
  {
    id: "review",
    label: "Review & submit",
    description: "Check everything and send for review",
    path: "/setup/submit",
    stages: [],
  },
];

export type StageStatuses = Record<string, string>;

export function stepForPath(pathname: string): SetupStep | undefined {
  if (pathname === CREATE_COMPANY_PATH) return SETUP_STEPS[0];
  return SETUP_STEPS.find((s) => s.path === pathname);
}

export function isStepComplete(
  step: SetupStep,
  stages: StageStatuses,
): boolean {
  return (
    step.stages.length > 0 && step.stages.every((s) => stages[s] === "COMPLETE")
  );
}

export function nextPath(id: string): string {
  const i = SETUP_STEPS.findIndex((s) => s.id === id);
  return i >= 0 && i < SETUP_STEPS.length - 1
    ? SETUP_STEPS[i + 1].path
    : SETUP_HOME;
}

export function previousPath(id: string): string {
  const i = SETUP_STEPS.findIndex((s) => s.id === id);
  return i > 0 ? SETUP_STEPS[i - 1].path : SETUP_HOME;
}

// Where "Continue setup" goes: the first step that is not finished.
export function firstOpenStep(
  stages: StageStatuses,
  hasCompany: boolean,
): SetupStep {
  if (!hasCompany) return SETUP_STEPS[0];
  return (
    SETUP_STEPS.find(
      (s) => s.stages.length > 0 && !isStepComplete(s, stages),
    ) ?? SETUP_STEPS[SETUP_STEPS.length - 1]
  );
}
