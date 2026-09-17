import type {
  MembershipStatus,
  OnboardingSource,
  ProviderService,
  ProvisioningOverallStatus,
  RelationshipType,
  ServiceCategory,
} from '@/lib/types/provider';

export type Tone = 'blue' | 'green' | 'amber' | 'red' | 'neutral';

export interface ToneClasses {
  bg: string;
  text: string;
  dot: string;
}

const TONES: Record<Tone, ToneClasses> = {
  blue:    { bg: 'bg-primary-bg', text: 'text-primary', dot: 'bg-primary' },
  green:   { bg: 'bg-green-bg',   text: 'text-green',   dot: 'bg-green'   },
  amber:   { bg: 'bg-amber-bg',   text: 'text-amber',   dot: 'bg-amber'   },
  red:     { bg: 'bg-red-bg',     text: 'text-red',     dot: 'bg-red'     },
  neutral: { bg: 'bg-surface-2',  text: 'text-ink-500', dot: 'bg-ink-400' },
};

const STATUS_META: Record<MembershipStatus, { label: string; tone: Tone }> = {
  INVITED:                     { label: 'Invited',         tone: 'blue'    },
  PENDING_PROVIDER_ACCEPTANCE: { label: 'Awaiting provider', tone: 'amber' },
  PENDING_SETUP:               { label: 'Pending setup',   tone: 'amber'   },
  ACTIVE:                      { label: 'Active',          tone: 'green'   },
  SUSPENDED:                   { label: 'Suspended',       tone: 'amber'   },
  ENDING:                      { label: 'Ending',          tone: 'amber'   },
  REMOVED:                     { label: 'Removed',         tone: 'red'     },
  DECLINED:                    { label: 'Declined',        tone: 'red'     },
  EXPIRED:                     { label: 'Expired',         tone: 'neutral' },
  CANCELLED:                   { label: 'Cancelled',       tone: 'neutral' },
};

const SERVICE_CATEGORY_LABEL: Record<ServiceCategory, string> = {
  DISPATCH: 'Dispatch',
  TAXI:     'Taxi',
  HAULING:  'Hauling',
};

const PROVIDER_SERVICE_LABEL: Record<ProviderService, string> = {
  'driver-dispatch-delivery-service': 'Dispatch service',
  'driver-taxi-service':              'Taxi service',
  'driver-hauling-service':           'Hauling service',
};

const ONBOARDING_SOURCE_LABEL: Record<OnboardingSource, string> = {
  ORGANIZATION_SPONSORED:   'Sponsored',
  EXISTING_PROVIDER_LINKED: 'Linked',
};

const RELATIONSHIP_TYPE_LABEL: Record<RelationshipType, string> = {
  EMPLOYEE:   'Employee',
  CONTRACTOR: 'Contractor',
  PARTNER:    'Partner',
};

const PROVISIONING_META: Record<
  ProvisioningOverallStatus,
  { label: string; tone: Tone }
> = {
  IN_PROGRESS:             { label: 'In progress',             tone: 'blue'  },
  COMPLETED:               { label: 'Completed',               tone: 'green' },
  BLOCKED:                 { label: 'Blocked',                 tone: 'red'   },
  RECONCILIATION_REQUIRED: { label: 'Reconciliation required', tone: 'amber' },
  FAILED_PERMANENT:        { label: 'Failed',                  tone: 'red'   },
};

const PROVISIONING_STEP_LABEL: Record<string, string> = {
  providerResolutionStatus: 'Provider resolution',
  providerIdentityStatus:   'Provider identity',
  associationStatus:        'Association consent',
  subWalletStatus:          'Sub-wallet',
  notificationStatus:       'Notification',
};

export const statusLabel = (s: MembershipStatus | string): string =>
  STATUS_META[s as MembershipStatus]?.label ?? s;

export const statusTone = (s: MembershipStatus | string): ToneClasses =>
  TONES[STATUS_META[s as MembershipStatus]?.tone ?? 'neutral'];

export const serviceCategoryLabel = (c: ServiceCategory | string): string =>
  SERVICE_CATEGORY_LABEL[c as ServiceCategory] ?? c;

export const providerServiceLabel = (s: ProviderService | string): string =>
  PROVIDER_SERVICE_LABEL[s as ProviderService] ?? s;

export const onboardingSourceLabel = (s: OnboardingSource | string): string =>
  ONBOARDING_SOURCE_LABEL[s as OnboardingSource] ?? s;

export const relationshipTypeLabel = (r: RelationshipType | string): string =>
  RELATIONSHIP_TYPE_LABEL[r as RelationshipType] ?? r;

export const provisioningOverallLabel = (s: ProvisioningOverallStatus | string): string =>
  PROVISIONING_META[s as ProvisioningOverallStatus]?.label ?? s;

export const provisioningOverallTone = (s: ProvisioningOverallStatus | string): ToneClasses =>
  TONES[PROVISIONING_META[s as ProvisioningOverallStatus]?.tone ?? 'neutral'];

export const provisioningStepLabel = (key: string): string =>
  PROVISIONING_STEP_LABEL[key] ?? key;

export const formatDate = (iso: string | null | undefined): string => {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isFinite(d.getTime())
    ? d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
    : '—';
};

export const formatDateTime = (iso: string | null | undefined): string => {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isFinite(d.getTime())
    ? d.toLocaleString(undefined, {
        year: 'numeric', month: 'short', day: 'numeric',
        hour: '2-digit', minute: '2-digit',
      })
    : '—';
};