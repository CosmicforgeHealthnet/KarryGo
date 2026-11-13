// The session marker cookie only tells the middleware a sign-in happened; the
// access token lives in the client. If the marker outlives the token (site
// data partly cleared, another tab signed out), the dashboard would send the
// user to /sign-in and the middleware would send them straight back: an
// endless spinner. Removing the marker before going to sign-in ends that loop.
export function clearStaleSessionMarker() {
  if (typeof document === "undefined") return;
  document.cookie = "karrygo_has_session=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
}
