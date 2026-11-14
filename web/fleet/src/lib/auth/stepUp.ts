// Step-up (re-verification) for high-risk actions.
//
// fleet-partner-service refuses a high-risk action (role and permission
// changes, staff suspension, organization closure, wallet allocations and
// withdrawals, payroll approval/posting, provider removal, sensitive exports)
// with 403 step_up_required or step_up_expired until the request carries a
// fresh step-up grant in the X-Step-Up-Reference header. The refusal names the
// action class to verify for in its details ({field: "actionClass"}).
//
// The flow: the API client sees the refusal, starts a challenge for that
// class (POST /auth/step-up/start), and sends the person to the existing
// verification page. Verifying stores the grant here (per action class, in
// session storage, until it expires) and returns to the previous page; when
// the action is tried again the refusal is answered by retrying with the
// stored grant. The client never decides which actions need step-up: the
// server's refusal does.

export const STEP_UP_HEADER = 'X-Step-Up-Reference';
export const STEP_UP_INTENT = 'step-up';

const STORAGE_KEY = 'fleet:step-up-grants';
const STEP_UP_CODES = new Set(['step_up_required', 'step_up_expired']);

export interface StepUpRefusal {
  actionClass: string;
  code: string;
}

export type StepUpStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

type Grants = Record<string, { reference: string; expiresAt: number }>;

type RefusalShape = {
  response?: {
    status?: number;
    data?: { error?: { code?: string; details?: Array<{ field?: string; message?: string }> } };
  };
};

// The step-up refusal in a failed request, or null when it is any other error.
export function stepUpRefusal(error: unknown): StepUpRefusal | null {
  const response = (error as RefusalShape)?.response;
  const body = response?.data?.error;
  if (response?.status !== 403 || !body?.code || !STEP_UP_CODES.has(body.code)) {
    return null;
  }
  const detail = (body.details ?? []).find(
    (entry) => entry?.field === 'actionClass' && typeof entry.message === 'string' && entry.message !== '',
  );
  if (!detail?.message) {
    return null;
  }
  return { actionClass: detail.message, code: body.code };
}

function load(storage: StepUpStorage): Grants {
  try {
    const parsed = JSON.parse(storage.getItem(STORAGE_KEY) ?? '{}');
    return parsed && typeof parsed === 'object' ? (parsed as Grants) : {};
  } catch {
    return {};
  }
}

function store(storage: StepUpStorage, grants: Grants): void {
  try {
    if (Object.keys(grants).length === 0) {
      storage.removeItem(STORAGE_KEY);
    } else {
      storage.setItem(STORAGE_KEY, JSON.stringify(grants));
    }
  } catch {
    // Storage unavailable (private mode, quota): the next attempt asks again.
  }
}

// A stored, unexpired grant for the action class, or null.
export function readStepUpGrant(storage: StepUpStorage, actionClass: string, now = Date.now()): string | null {
  const grant = load(storage)[actionClass];
  if (!grant || typeof grant.reference !== 'string' || !(grant.expiresAt > now)) {
    return null;
  }
  return grant.reference;
}

// Remember a verified grant until the server's expiry.
export function saveStepUpGrant(storage: StepUpStorage, actionClass: string, reference: string, expiresAt: string | number): void {
  const deadline = typeof expiresAt === 'number' ? expiresAt : Date.parse(expiresAt);
  if (!actionClass || !reference || !Number.isFinite(deadline)) {
    return;
  }
  const grants = load(storage);
  grants[actionClass] = { reference, expiresAt: deadline };
  store(storage, grants);
}

// Forget a grant the server refused.
export function dropStepUpGrant(storage: StepUpStorage, actionClass: string): void {
  const grants = load(storage);
  if (actionClass in grants) {
    delete grants[actionClass];
    store(storage, grants);
  }
}

export interface StartedStepUp {
  challengeId: string;
  maskedDestination?: string;
  expiresInSeconds?: number;
}

// The verification page query for a started step-up challenge.
export function stepUpVerifyQuery(actionClass: string, started: StartedStepUp): string {
  const query = new URLSearchParams({ challengeId: started.challengeId, intent: STEP_UP_INTENT, actionClass });
  if (started.maskedDestination) query.set('destination', started.maskedDestination);
  if (started.expiresInSeconds) query.set('expiresIn', String(started.expiresInSeconds));
  return query.toString();
}
