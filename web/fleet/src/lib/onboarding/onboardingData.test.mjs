import { test } from "node:test";
import assert from "node:assert/strict";
import {
  RESOURCE_KEYS,
  ScopedCache,
  cacheScope,
  previewErrorMessage,
  previewFilename,
  previewKind,
  resourcesForPath,
} from "./onboardingData.ts";

test("the documents step needs only documents and onboarding status", () => {
  assert.deepEqual([...resourcesForPath("/setup/business-docs")], ["documents", "status"]);
  assert.deepEqual([...resourcesForPath("/setup/business-docs/")], ["documents", "status"]);
});

test("every page needs onboarding status for the step list", () => {
  for (const path of ["/setup", "/setup/profile", "/setup/headquarters", "/setup/authorized-representative", "/setup/capabilities", "/setup/business-docs"])
    assert.ok(resourcesForPath(path).includes("status"), path);
});

test("pages that read a record wait for it; unknown pages wait for everything", () => {
  assert.ok(resourcesForPath("/setup/headquarters").includes("profile"));
  assert.ok(resourcesForPath("/setup/authorized-representative").includes("representatives"));
  assert.ok(resourcesForPath("/setup/capabilities").includes("capabilities"));
  assert.deepEqual([...resourcesForPath("/setup/submit")], [...RESOURCE_KEYS]);
  assert.deepEqual([...resourcesForPath("/somewhere-else")], [...RESOURCE_KEYS]);
});

test("cache scope requires both identity and organization", () => {
  assert.equal(cacheScope("id_1", "org_1"), "id_1:org_1");
  assert.equal(cacheScope("id_1", undefined), null);
  assert.equal(cacheScope(null, "org_1"), null);
});

test("a cached snapshot is returned only for its own identity and organization", () => {
  const cache = new ScopedCache();
  cache.write(cacheScope("id_1", "org_a"), { documents: ["a"] });
  assert.deepEqual(cache.read("id_1:org_a"), { documents: ["a"] });
  assert.equal(cache.read("id_1:org_b"), null, "another organization never sees it");
  assert.equal(cache.read("id_2:org_a"), null, "another identity never sees it");
  assert.equal(cache.read(null), null);
});

test("switching organization evicts the previous organization's records", () => {
  const cache = new ScopedCache();
  cache.write("id_1:org_a", 1);
  cache.retainOnly("id_1:org_b");
  assert.equal(cache.read("id_1:org_a"), null);
  cache.write("id_1:org_b", 2);
  cache.write("id_1:org_c", 3);
  assert.equal(cache.read("id_1:org_b"), null, "single entry: a new scope replaces the old one");
  cache.retainOnly("id_1:org_c");
  assert.equal(cache.read("id_1:org_c"), 3, "the current scope is retained");
  cache.clear();
  assert.equal(cache.read("id_1:org_c"), null, "sign-out clears it");
});

test("only the four accepted document types are rendered", () => {
  assert.equal(previewKind("application/pdf"), "pdf");
  assert.equal(previewKind("image/jpeg"), "image");
  assert.equal(previewKind("image/png"), "image");
  assert.equal(previewKind("IMAGE/WEBP; charset=binary"), "image");
  for (const unsafe of ["text/html", "image/svg+xml", "application/octet-stream", "", null, undefined])
    assert.equal(previewKind(unsafe), null, String(unsafe));
});

test("download names come from the document type and detected format", () => {
  assert.equal(previewFilename("TAX_CLEARANCE", "application/pdf"), "tax-clearance.pdf");
  assert.equal(previewFilename("CERTIFICATE_OF_INCORPORATION", "image/webp"), "certificate-of-incorporation.webp");
  assert.equal(previewFilename("../../etc", "image/png"), "etc.png");
});

const http = (status, message, requestId) => ({
  response: { status, data: message ? { error: { message, requestId } } : undefined },
});

test("preview failures explain what happened without breaking the page", () => {
  assert.match(previewErrorMessage({ message: "Network Error" }).message, /couldn't connect/);
  assert.match(previewErrorMessage(http(401)).message, /session expired/);
  assert.match(previewErrorMessage(http(403, "Forbidden.")).message, /permission/);
  assert.equal(previewErrorMessage(http(404, "This document is no longer available.")).message, "This document is no longer available.");
  assert.match(previewErrorMessage(http(404)).message, /no longer available/);
  assert.match(previewErrorMessage(http(409)).message, /not ready/);
  assert.match(previewErrorMessage(http(415)).message, /cannot be previewed/);
  assert.match(previewErrorMessage(http(503)).message, /temporarily unavailable/);
  assert.match(previewErrorMessage(http(500, "internal detail")).message, /could not be loaded/);
  assert.equal(previewErrorMessage(http(409, "Not ready.", "req-1")).requestId, "req-1");
});
