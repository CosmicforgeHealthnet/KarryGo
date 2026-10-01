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
  const isAuthRoute = PUBLIC_ROUTES.some((route) => pathname === route || pathname.startsWith(route + "/"));

  if (!hasSession && !isPublicRoute) {
    const redirectUrl = new URL("/sign-in", request.url);
    redirectUrl.searchParams.set("redirect", pathname);
    return NextResponse.redirect(redirectUrl);
  }

  if (orgStatus === "DRAFT" && !pathname.startsWith("/setup")) {
    return NextResponse.redirect(new URL("/setup", request.url));
  }

  if (hasSession && isAuthRoute) {
    return NextResponse.redirect(new URL("/", request.url));
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    "/((?!api|_next/static|_next/image|favicon.ico|images|fonts).*)",
  ],
};