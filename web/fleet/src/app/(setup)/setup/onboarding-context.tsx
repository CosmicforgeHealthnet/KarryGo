"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAuthStore } from "@/lib/stores/authStore";
import {
  useOrganizationStore,
  type Organization,
} from "@/lib/stores/organizationStore";
import {
  organizationService as api,
  type IdentityProfile,
  type BusinessProfile,
  type Representative,
  type Document,
  type OnboardingStatusResponse,
} from "@/lib/services/organizationService";
import { ErrorNotice, Loading } from "./onboarding-ui";

export type Snapshot = {
  identity: IdentityProfile;
  organization: Organization | null;
  profile: BusinessProfile | null;
  representatives: Representative[];
  capabilities: Record<string, string>;
  documents: Document[];
  status: OnboardingStatusResponse | null;
};
type ContextValue = {
  data: Snapshot;
  reload: () => Promise<Snapshot>;
  editable: boolean;
  drafts: Map<string, unknown>;
  clearDraft: (key: string) => void;
  reloadSaved: () => Promise<void>;
};
const Context = createContext<ContextValue | null>(null);
export function useOnboarding() {
  const value = useContext(Context);
  if (!value) throw new Error("Onboarding data is not loaded.");
  return value;
}
export function OnboardingProvider({ children }: { children: ReactNode }) {
  const identityId = useAuthStore((s) => s.identityId);
  const selectedId = useAuthStore(
    (s) => s.selectedOrganization?.organizationId,
  );
  const hydrated = useAuthStore((s) => s._hasHydrated);
  const authenticated = useAuthStore((s) => s.isAuthenticated);
  const [data, setData] = useState<Snapshot | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [formEpoch, setFormEpoch] = useState(0);
  const generation = useRef(0);
  const invalidate = useCallback(() => {
    generation.current++;
  }, []);
  const [drafts] = useState(() => new Map<string, unknown>());
  const fetchSnapshot = useCallback(async (): Promise<Snapshot> => {
    const [identity, organization] = await Promise.all([
      api.getIdentityProfile(),
      api.getCurrentOrganization().catch((err: unknown) => {
        const status = (err as { response?: { status: number } }).response
          ?.status;
        if (
          (status === 403 || status === 404) &&
          useAuthStore.getState().organizations.length === 0
        )
          return null;
        throw err;
      }),
    ]);
    if (!organization)
      return {
        identity,
        organization: null,
        profile: null,
        representatives: [],
        capabilities: {},
        documents: [],
        status: null,
      };
    const id = organization.organizationId;
    const [profile, representatives, capabilities, documents, status] =
      await Promise.all([
        api.getProfile(id),
        api.listRepresentatives(id),
        api.getCapabilities(id),
        api.listDocuments(id),
        api.getOnboardingStatus(id),
      ]);
    return {
      identity,
      organization,
      profile,
      representatives: representatives ?? [],
      capabilities,
      documents: documents ?? [],
      status,
    };
  }, []);
  const apply = useCallback((next: Snapshot) => {
    setData(next);
    setError(null);
    if (next.organization)
      useOrganizationStore.getState().setOrganization(next.organization);
    else useOrganizationStore.getState().clearOrganization();
  }, []);
  const reload = useCallback(async () => {
    const sequence = ++generation.current;
    const next = await fetchSnapshot();
    if (sequence !== generation.current)
      throw new Error(
        "The selected organization changed. Reload its onboarding details.",
      );
    apply(next);
    return next;
  }, [fetchSnapshot, apply]);
  useEffect(() => {
    if (!hydrated || !authenticated) return;
    let active = true;
    fetchSnapshot()
      .then((next) => {
        if (active) apply(next);
      })
      .catch((err) => {
        if (active) setError(err);
      });
    return () => {
      active = false;
      invalidate();
    };
  }, [
    identityId,
    selectedId,
    hydrated,
    authenticated,
    fetchSnapshot,
    apply,
    invalidate,
  ]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (drafts.size) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [drafts]);
  if (error)
    return (
      <ErrorNotice
        error={error}
        onRetry={() => {
          setError(null);
          reload().catch(setError);
        }}
      />
    );
  if (!data) return <Loading />;
  const editable =
    !data.organization ||
    [
      "DRAFT",
      "CHANGES_REQUIRED",
      "CONTACT_VERIFIED",
      "PENDING_REVIEW",
    ].includes(data.organization.status);
  return (
    <Context.Provider
      value={{
        data,
        reload,
        editable,
        drafts: drafts,
        clearDraft: (key) => drafts.delete(key),
        reloadSaved: async () => {
          await reload();
          drafts.clear();
          setFormEpoch((e) => e + 1);
        },
      }}
    >
      <div key={formEpoch}>{children}</div>
    </Context.Provider>
  );
}
// Drafts are scoped to this mounted identity/organization provider and survive
// back/overview navigation. Only the initial server snapshot initializes a form.
export function useOnboardingDraft<T>(key: string, initial: () => T) {
  const { drafts, clearDraft } = useOnboarding();
  const [value, setValue] = useState<T>(
    () => (drafts.get(key) as T | undefined) ?? initial(),
  );
  const change = (next: T) => {
    drafts.set(key, next);
    setValue(next);
  };
  return [value, change, () => clearDraft(key)] as const;
}
