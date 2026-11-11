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

  // The org-status cookie is readable by the page and can outlive the
  // session (e.g. a server-side logout). Only honour it while signed in,
  // otherwise /setup -> /sign-in -> /setup loops forever.
  if (hasSession && orgStatus === "DRAFT" && !pathname.startsWith("/setup") && pathname !== "/choose-organization") {
    return NextResponse.redirect(new URL("/setup", request.url));
  }

  if (hasSession && isAuthRoute) {
    return NextResponse.redirect(new URL("/", request.url));
  }

  return NextResponse.next();
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