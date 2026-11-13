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
import { usePathname } from "next/navigation";
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
import {
  RESOURCE_KEYS,
  ScopedCache,
  cacheScope,
  resourcesForPath,
  type ResourceKey,
} from "@/lib/onboarding/onboardingData";
import { ErrorNotice, Loading } from "./onboarding-ui";

export type { ResourceKey } from "@/lib/onboarding/onboardingData";

export type Snapshot = {
  identity: IdentityProfile;
  organization: Organization | null;
  profile: BusinessProfile | null;
  representatives: Representative[];
  capabilities: Record<string, string>;
  documents: Document[];
  status: OnboardingStatusResponse | null;
};
type Resources = Pick<Snapshot, ResourceKey>;
type Core = Pick<Snapshot, "identity" | "organization">;
type State = { core: Core | null; resources: Partial<Resources> };
type ContextValue = {
  data: Snapshot;
  // Re-reads every saved record (identity, organization and all resources).
  reload: () => Promise<Snapshot>;
  // Re-reads only the named records, keeping everything else as loaded.
  refresh: (keys: readonly ResourceKey[]) => Promise<Snapshot>;
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

const EMPTY: Resources = {
  profile: null,
  representatives: [],
  capabilities: {},
  documents: [],
  status: null,
};

type Read = { signal?: AbortSignal };
const readers: { [K in ResourceKey]: (id: string, o: Read) => Promise<Resources[K]> } = {
  profile: (id, o) => api.getProfile(id, o),
  representatives: (id, o) => api.listRepresentatives(id, o).then((r) => r ?? []),
  capabilities: (id, o) => api.getCapabilities(id, o),
  documents: (id, o) => api.listDocuments(id, o).then((d) => d ?? []),
  status: (id, o) => api.getOnboardingStatus(id, o),
};

// The last onboarding view for the current identity and organization, held in
// memory only. A returning visit shows it immediately while it is revalidated.
const snapshotCache = new ScopedCache<State>();
if (typeof window !== "undefined") {
  // Signing out or becoming a different identity drops cached records.
  let identity = useAuthStore.getState().identityId;
  useAuthStore.subscribe((state) => {
    if (!state.isAuthenticated || state.identityId !== identity) snapshotCache.clear();
    identity = state.identityId;
  });
}

function readCurrentOrganization(o: Read) {
  return api.getCurrentOrganization(o).catch((err: unknown) => {
    const status = (err as { response?: { status: number } }).response?.status;
    if (
      (status === 403 || status === 404) &&
      useAuthStore.getState().organizations.length === 0
    )
      return null;
    throw err;
  });
}

function isCanceled(err: unknown) {
  const e = err as { name?: string; code?: string };
  return e?.name === "CanceledError" || e?.code === "ERR_CANCELED" || e?.name === "AbortError";
}

function snapshotOf(state: State): Snapshot | null {
  if (!state.core) return null;
  return { ...state.core, ...EMPTY, ...state.resources };
}

export function OnboardingProvider({ children }: { children: ReactNode }) {
  const identityId = useAuthStore((s) => s.identityId);
  const selectedId = useAuthStore(
    (s) => s.selectedOrganization?.organizationId,
  );
  const hydrated = useAuthStore((s) => s._hasHydrated);
  const authenticated = useAuthStore((s) => s.isAuthenticated);
  const pathname = usePathname();
  const pathRef = useRef(pathname);
  pathRef.current = pathname;
  const scope = cacheScope(identityId, selectedId);
  const [state, setState] = useState<State>(
    () => snapshotCache.read(scope) ?? { core: null, resources: {} },
  );
  const [errors, setErrors] = useState<Partial<Record<ResourceKey | "core", unknown>>>({});
  const [formEpoch, setFormEpoch] = useState(0);
  const stateRef = useRef(state);
  // Bumped when this view's organization scope ends; in-flight work for an
  // earlier generation never applies.
  const generation = useRef(0);
  // Latest request per record: a slower, older read never overwrites a newer one.
  const latest = useRef<Record<ResourceKey, number>>({
    profile: 0, representatives: 0, capabilities: 0, documents: 0, status: 0,
  });
  const controller = useRef<AbortController | null>(null);
  const invalidate = useCallback(() => {
    generation.current++;
  }, []);
  const [drafts] = useState(() => new Map<string, unknown>());

  // The ref is the synchronous source of truth: a caller awaiting a read gets a
  // snapshot that already includes it, before React re-renders.
  const commit = useCallback((update: (s: State) => State) => {
    const next = update(stateRef.current);
    stateRef.current = next;
    const organizationId = next.core?.organization?.organizationId;
    if (next.core && organizationId)
      snapshotCache.write(cacheScope(next.core.identity.identityId, organizationId), next);
    setState(next);
  }, []);

  // Reads one record for an organization and applies it if it is still the
  // newest read of that record in the current generation.
  const readResource = useCallback(
    async <K extends ResourceKey>(key: K, organizationId: string, signal: AbortSignal | undefined, gen: number) => {
      const sequence = ++latest.current[key];
      let value: Resources[K];
      try {
        value = await readers[key](organizationId, { signal });
      } catch (err) {
        if (gen === generation.current && sequence === latest.current[key] && !isCanceled(err))
          setErrors((e) => ({ ...e, [key]: err }));
        throw err;
      }
      if (gen === generation.current && sequence === latest.current[key]) {
        commit((s) =>
          s.core?.organization?.organizationId === organizationId
            ? { ...s, resources: { ...s.resources, [key]: value } }
            : s,
        );
        setErrors((e) => (key in e ? { ...e, [key]: undefined } : e));
      }
      return value;
    },
    [commit],
  );

  // Initial load. When the organization is already known, the records the
  // current page needs are requested alongside the identity and organization
  // reads instead of after them; they are only applied once the organization is
  // confirmed.
  useEffect(() => {
    if (!hydrated || !authenticated) return;
    snapshotCache.retainOnly(scope);
    const gen = ++generation.current;
    const abort = new AbortController();
    controller.current = abort;
    const { signal } = abort;
    // The organization being onboarded is usually already known: selected at
    // sign-in, or remembered from this identity's last visit (that record is
    // cleared on sign-out, identity change and organization switch). It is only
    // a hint: its reads are applied after the server confirms it is current.
    const hint =
      selectedId ?? useOrganizationStore.getState().organization?.organizationId;
    // Only what the current page shows is requested up front, so it is not
    // queued behind records another step will need; those follow once the
    // organization is confirmed.
    const firstKeys = resourcesForPath(pathRef.current);
    const early = hint
      ? firstKeys.map((key) => ({
          key,
          promise: readers[key](hint, { signal }),
        }))
      : null;
    early?.forEach(({ promise }) => promise.catch(() => {}));
    (async () => {
      try {
        const [identity, organization] = await Promise.all([
          api.getIdentityProfile({ signal }),
          readCurrentOrganization({ signal }),
        ]);
        if (gen !== generation.current) return;
        commit((s) => ({
          core: { identity, organization },
          resources:
            s.core?.organization?.organizationId === organization?.organizationId
              ? s.resources
              : organization ? {} : { ...EMPTY },
        }));
        setErrors((e) => ({ ...e, core: undefined }));
        if (!organization) return;
        const id = organization.organizationId;
        const started = early && id === hint ? early : [];
        const reads = [
          ...started,
          ...RESOURCE_KEYS.filter((key) => !started.some((r) => r.key === key)).map(
            (key) => ({ key, promise: readers[key](id, { signal }) }),
          ),
        ];
        await Promise.all(
          reads.map(async ({ key, promise }) => {
            const sequence = ++latest.current[key];
            try {
              const value = await promise;
              if (gen !== generation.current || sequence !== latest.current[key]) return;
              commit((s) => ({ ...s, resources: { ...s.resources, [key]: value } }));
              setErrors((e) => ({ ...e, [key]: undefined }));
            } catch (err) {
              if (gen === generation.current && !isCanceled(err))
                setErrors((e) => ({ ...e, [key]: err }));
            }
          }),
        );
      } catch (err) {
        if (gen === generation.current && !isCanceled(err)) setErrors((e) => ({ ...e, core: err }));
      }
    })();
    return () => {
      invalidate();
      abort.abort();
    };
  }, [identityId, selectedId, scope, hydrated, authenticated, commit, invalidate]);

  useEffect(() => {
    const organization = state.core?.organization;
    if (!state.core) return;
    if (organization) useOrganizationStore.getState().setOrganization(organization);
    else useOrganizationStore.getState().clearOrganization();
  }, [state.core]);

  const orgChanged = () =>
    new Error("The selected organization changed. Reload its onboarding details.");

  const reload = useCallback(async () => {
    const gen = generation.current;
    const signal = controller.current?.signal;
    const known = stateRef.current.core?.organization?.organizationId;
    // The organization is already known, so its records are read in the same
    // round trip as the identity and organization.
    const early = known
      ? RESOURCE_KEYS.map((key) => readResource(key, known, signal, gen))
      : null;
    early?.forEach((p) => p.catch(() => {}));
    let identity: IdentityProfile, organization: Organization | null;
    try {
      [identity, organization] = await Promise.all([
        api.getIdentityProfile({ signal }),
        readCurrentOrganization({ signal }),
      ]);
    } catch (err) {
      if (gen === generation.current && !isCanceled(err)) setErrors((e) => ({ ...e, core: err }));
      throw err;
    }
    if (gen !== generation.current) throw orgChanged();
    setErrors((e) => ({ ...e, core: undefined }));
    commit((s) => ({
      core: { identity, organization },
      resources:
        s.core?.organization?.organizationId === organization?.organizationId
          ? s.resources
          : organization ? {} : { ...EMPTY },
    }));
    if (organization) {
      const id = organization.organizationId;
      await Promise.all(
        early && id === known
          ? early
          : RESOURCE_KEYS.map((key) => readResource(key, id, signal, gen)),
      );
    }
    if (gen !== generation.current) throw orgChanged();
    return snapshotOf(stateRef.current)!;
  }, [commit, readResource]);

  const refresh = useCallback(
    async (keys: readonly ResourceKey[]) => {
      const organizationId = stateRef.current.core?.organization?.organizationId;
      if (!organizationId) return reload();
      const gen = generation.current;
      const signal = controller.current?.signal;
      await Promise.all(keys.map((key) => readResource(key, organizationId, signal, gen)));
      if (gen !== generation.current) throw orgChanged();
      return snapshotOf(stateRef.current)!;
    },
    [reload, readResource],
  );

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

  const data = snapshotOf(state);
  const required = resourcesForPath(pathname);
  const missing = data?.organization
    ? required.filter((key) => !(key in state.resources))
    : [];
  // A failed read blocks only what cannot be shown without it. Records already
  // on screen stay visible when a background revalidation fails.
  const blockingError = !data
    ? errors.core
    : missing.map((key) => errors[key]).find((e) => e !== undefined);
  if (blockingError !== undefined)
    return (
      <ErrorNotice
        error={blockingError}
        onRetry={() => {
          setErrors({});
          reload().catch(() => {});
        }}
      />
    );
  if (!data || missing.length > 0) return <Loading />;
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
        refresh,
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
