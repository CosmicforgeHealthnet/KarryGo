// src/middleware.ts

import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const PUBLIC_ROUTES = ["/sign-in", "/sign-up", "/verify", "/reverify", "/choose-organization"];
const SESSION_MARKER_COOKIE = "karrygo_has_session";
const ORG_STATUS_COOKIE = "karrygo_org_status";

export function middleware(request: NextRequest) {
  const hasSession = request.cookies.get(SESSION_MARKER_COOKIE)?.value;
  const orgStatus = request.cookies.get(ORG_STATUS_COOKIE)?.value;
  const { pathname } = request.nextUrl;

  const isPublicRoute = PUBLIC_ROUTES.some((route) => pathname.startsWith(route));
  const isAuthRoute = PUBLIC_ROUTES.filter(route => route !== "/choose-organization").some((route) => pathname === route || pathname.startsWith(route + "/"));

  if (!hasSession && !isPublicRoute) {
    const redirectUrl = new URL("/sign-in", request.url);
    redirectUrl.searchParams.set("redirect", pathname);
    return NextResponse.redirect(redirectUrl);
  }

  // Organization lifecycle routing is NOT decided here. It used to follow a
  // client-written status cookie, which went stale: an account that signed up
  // carried "DRAFT" through submission, so every visit to the dashboard was
  // redirected back to /setup. Where a member belongs is decided from the
  // backend's authoritative status by lib/auth/organizationDestination in the
  // dashboard and setup layouts, and the backend enforces it independently.
  // A signed-in person confirming a high-risk action (step-up) verifies on
  // the same page as sign-in; that visit must not bounce them to "/".
  const isStepUpVerification = pathname === "/verify" && request.nextUrl.searchParams.get("intent") === "step-up";
  let response: NextResponse;
  if (hasSession && isAuthRoute && !isStepUpVerification) {
    response = NextResponse.redirect(new URL("/", request.url));
  } else {
    response = NextResponse.next();
  }
  // Remove the retired status cookie so a stale value cannot linger.
  if (orgStatus !== undefined) response.cookies.delete(ORG_STATUS_COOKIE);
  return response;
}

export const config = {
  matcher: [
    // /api and /fleet-partner are proxied straight to the backend (see
    // next.config.ts). Page redirects must never apply to them: redirecting
    // POST /fleet-partner/v1/auth/refresh or /logout to a page broke token
    // refresh and server-side logout.
    "/((?!api|fleet-partner|_next/static|_next/image|favicon.ico|images|fonts).*)",
  ],
};