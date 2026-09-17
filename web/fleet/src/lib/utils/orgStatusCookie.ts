// lib/utils/orgStatusCookie.ts

const ORG_STATUS_COOKIE = "karrygo_org_status";
const COOKIE_MAX_AGE_SECONDS = 60 * 60 * 24 * 7;

export function setOrgStatusCookie(status: string) {
  if (typeof document === "undefined") return;
  document.cookie = `${ORG_STATUS_COOKIE}=${status}; path=/; max-age=${COOKIE_MAX_AGE_SECONDS}; SameSite=Lax`;
}

export function clearOrgStatusCookie() {
  if (typeof document === "undefined") return;
  document.cookie = `${ORG_STATUS_COOKIE}=; path=/; max-age=0; SameSite=Lax`;
}

export { ORG_STATUS_COOKIE };