"""Fail-closed staging release authorization. No SSH, secret values or mutations.

The audit token is environment-only, read-only metadata access (Actions,
Environments, repository/org secret names). API failures deny release.
"""
import argparse
import datetime as dt
import hashlib
import io
import json
import os
import re
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path

REPOSITORY = "CosmicforgeHealthnet/KarryGo"
ENVIRONMENT = "staging"
ACCEPTANCE = {
    "admin-bridge.yml": 2, "admin-owner-closure.yml": 1,
    "customer-v4-certification.yml": 1, "vendor-section46-48.yml": 1,
    "partner-gateway-split.yml": 2, "section31-mandatory.yml": 22,
    "deployment-safety.yml": 1,
}
CREDENTIALS = {"STAGING_DEPLOY_HOST", "STAGING_DEPLOY_USER", "STAGING_DEPLOY_SSH_KEY",
               "STAGING_DEPLOY_KNOWN_HOSTS", "RELEASE_AUDIT_TOKEN"}
FORBIDDEN = {"all", "admin-backoffice-service", "api-gateway", "partner-web-service",
             "analytics-service", "driver-dispatch-delivery-service"}


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def selected(raw, kind):
    services = raw.split(",")
    require(services and len(services) == len(set(services)), "empty/duplicate selection")
    rows = Path("deploy/platform/services.tsv").read_text().splitlines()
    allowed = {r.split("|")[0].strip() for r in rows if r.strip() and not r.lstrip().startswith("#")}
    allowed -= FORBIDDEN
    allowed = {"vendor-service"} if kind == "vendor" else allowed
    require(all(s in allowed and s not in FORBIDDEN for s in services), "service/cutover blocked")
    return services


def timestamp(value):
    parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    require(parsed.tzinfo is not None, "timestamp requires timezone")
    return parsed


def validate_restore(proof, sha, services, now):
    require(proof.get("schema") == 1 and proof.get("environment") == ENVIRONMENT,
            "restore must certify actual staging, not a disposable CI fixture")
    require(proof.get("release_sha") == sha, "restore SHA mismatch")
    completed = timestamp(proof["completed_at"])
    require(dt.timedelta(0) <= now - completed <= dt.timedelta(hours=24), "stale/future restore")
    require(proof.get("operator_attested") is True, "missing operator restore attestation")
    for service in services:
        item = proof.get("services", {}).get(service, {})
        require(item.get("validated") is True and item.get("commands_succeeded") is True,
                "missing/failed service restore: " + service)
        require(re.fullmatch(r"[0-9a-f]{64}", item.get("backup_sha256", "")), "missing backup digest")
        require(item.get("validation_sha256") and re.fullmatch(r"[0-9a-f]{64}", item["validation_sha256"]),
                "missing validation digest")
        require(item.get("source_database_version") == item.get("restore_database_version")
                and isinstance(item.get("source_database_version"), int), "database version mismatch")
        require(item.get("artifact_location") and item.get("validation_summary"), "missing recovery evidence")
        require(item.get("runtime_compatibility_attested") is True, "missing live compatibility/consumer review")
    return proof


class Redirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        require(urllib.parse.urlparse(newurl).scheme == "https", "non-HTTPS API redirect")
        result = super().redirect_request(req, fp, code, msg, headers, newurl)
        if urllib.parse.urlparse(newurl).netloc != urllib.parse.urlparse(req.full_url).netloc:
            result.remove_header("Authorization")
        return result


class GitHub:
    def __init__(self, token):
        require(token, "missing environment audit credential")
        self.token = token
        self.opener = urllib.request.build_opener(Redirect)

    def raw(self, path):
        request = urllib.request.Request("https://api.github.com/repos/" + REPOSITORY + "/" + path,
                    headers={"Authorization": "Bearer " + self.token, "Accept": "application/vnd.github+json",
                             "X-GitHub-Api-Version": "2022-11-28"})
        with self.opener.open(request, timeout=30) as response:
            result = response.read(16 * 1024 * 1024 + 1)
        require(len(result) <= 16 * 1024 * 1024, "oversized API response")
        return result

    def get(self, path):
        return json.loads(self.raw(path))

    def pages(self, path, key):
        results = []
        for page in range(1, 101):
            sep = "&" if "?" in path else "?"
            data = self.get(f"{path}{sep}per_page=100&page={page}")
            items = data[key]
            results.extend(items)
            if len(items) < 100:
                return results
        raise ValueError("pagination exceeded; cannot establish complete evidence")


def environment(api):
    info = api.get("environments/staging")
    reviewers = [r for r in info.get("protection_rules", []) if r.get("type") == "required_reviewers"]
    require(len(reviewers) == 1 and reviewers[0].get("reviewers")
            and reviewers[0].get("prevent_self_review") is True, "required separate reviewers missing")
    require(all(r.get("type") == "User" and isinstance(r.get("reviewer", {}).get("id"), int)
                for r in reviewers[0]["reviewers"]), "explicit individual reviewer identities required")
    require(info.get("can_admins_bypass") is False, "administrator bypass enabled/unknown")
    require(info.get("deployment_branch_policy") == {"protected_branches": False, "custom_branch_policies": True},
            "environment requires exact staging branch policy")
    policies = api.pages("environments/staging/deployment-branch-policies", "branch_policies")
    require(policies and all(p.get("name") == "staging" and p.get("type") == "branch" for p in policies),
            "non-staging branch/tag allowed by environment")
    names = {s["name"] for s in api.pages("environments/staging/secrets", "secrets")}
    require(CREDENTIALS <= names, "missing environment-scoped release credentials")
    owner_type = api.get("")["owner"]["type"]
    require(owner_type in {"User", "Organization"}, "unknown repository ownership")
    paths = ["actions/secrets"]
    if owner_type == "Organization":
        paths.append("actions/organization-secrets")
    for path in paths:
        inherited = {s["name"] for s in api.pages(path, "secrets")}
        require(not (CREDENTIALS | {"VPS_HOST", "VPS_USER", "VPS_SSH_KEY"}) & inherited,
                "repository/organization deployment credential bypass")
    config = api.get("environments/staging/variables/STAGING_RELEASE_INITIATORS")
    actors = json.loads(config["value"])
    require(isinstance(actors, list) and actors and all(isinstance(a, str) and a for a in actors),
            "missing release initiator allowlist")
    return info, actors


def approval(api, run_id, env_id, actor, configured_reviewers):
    history = api.get(f"actions/runs/{run_id}/approvals")
    reviews = [r for r in history if any(e.get("id") == env_id for e in r.get("environments", []))]
    require(reviews and not any(r.get("state") == "rejected" for r in reviews), "missing/rejected approval")
    identities = {r["reviewer"]["id"] for r in configured_reviewers}
    approved = [r for r in reviews if r.get("state") == "approved" and r.get("user", {}).get("login") != actor
                and r.get("user", {}).get("id") in identities]
    require(approved, "no separate deployment approval")
    return [r["user"]["login"] for r in approved]


def current(api, sha, context, kind):
    require(context.get("repository") == REPOSITORY and context.get("event_name") == "workflow_dispatch",
            "manual dispatch in canonical repository required")
    require(context.get("ref") == "refs/heads/staging" and context.get("sha") == sha
            and context.get("workflow_sha") == sha, "workflow/ref/release SHA mismatch")
    require(re.fullmatch(r"[0-9a-f]{40}", sha), "full lowercase commit SHA required")
    require(str(context.get("run_attempt")) == "1", "reruns require a fresh release decision")
    require(api.get("git/ref/heads/staging")["object"]["sha"] == sha, "stale/non-staging release SHA")
    run = api.get("actions/runs/" + str(context["run_id"]))
    path = "certify-staging-restore.yml" if kind == "certify" else "deploy-" + ("vendor-service" if kind == "vendor" else "platform") + ".yml"
    require(run.get("path", "").split("@")[0] == ".github/workflows/" + path
            and run.get("event") == "workflow_dispatch" and run.get("head_sha") == sha,
            "workflow reuse/foreign run forbidden")
    require(run["actor"]["login"] == context["actor"] == context["triggering_actor"], "foreign rerun initiator")


def acceptance(api, sha):
    result = []
    for workflow, minimum in ACCEPTANCE.items():
        runs = api.pages(f"actions/workflows/{workflow}/runs?head_sha={sha}", "workflow_runs")
        runs = [r for r in runs if r.get("head_sha") == sha and r.get("event") in {"pull_request", "push", "workflow_dispatch"}]
        require(runs, "missing acceptance: " + workflow)
        latest = max(runs, key=lambda r: r["id"])
        require(latest.get("status") == "completed" and latest.get("conclusion") == "success", "CI incomplete/failed: " + workflow)
        jobs = api.pages(f"actions/runs/{latest['id']}/attempts/{latest['run_attempt']}/jobs", "jobs")
        require(len(jobs) >= minimum and all(j.get("conclusion") == "success" for j in jobs), "CI jobs skipped/failed: " + workflow)
        result.append({"workflow": workflow, "run_id": latest["id"], "attempt": latest["run_attempt"]})
    return result


def restore(api, run_id, sha, services, env_id, now, configured_reviewers):
    require(re.fullmatch(r"[1-9][0-9]*", run_id), "restore certification run required")
    run = api.get(f"actions/runs/{run_id}")
    require(run.get("path", "").split("@")[0] == ".github/workflows/certify-staging-restore.yml"
            and run.get("head_sha") == sha and run.get("head_branch") == "staging"
            and run.get("event") == "workflow_dispatch" and run.get("status") == "completed"
            and run.get("conclusion") == "success" and run.get("run_attempt") == 1,
            "invalid restore certification provenance")
    approval(api, run_id, env_id, run["actor"]["login"], configured_reviewers)
    artifacts = api.pages(f"actions/runs/{run_id}/artifacts", "artifacts")
    items = [a for a in artifacts if a.get("name") == "staging-restore-evidence" and not a.get("expired")]
    require(len(items) == 1, "missing/ambiguous restore artifact")
    item = items[0]
    raw = api.raw(f"actions/artifacts/{item['id']}/zip")
    digest = "sha256:" + hashlib.sha256(raw).hexdigest()
    require(item.get("digest") == digest, "restore artifact digest mismatch/missing")
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        require(archive.namelist() == ["restore-evidence.json"], "unexpected restore archive")
        require(archive.getinfo("restore-evidence.json").file_size <= 1024 * 1024, "oversized restore proof")
        proof = json.loads(archive.read("restore-evidence.json"))
    validate_restore(proof, sha, services, now)
    return {"run_id": run_id, "artifact_id": item["id"], "digest": digest, "proof": proof}


def authorize(api, context, sha, raw_services, kind, restore_id, proof=None):
    services = selected(raw_services, "vendor" if kind == "vendor" else "platform") if kind != "certify" else raw_services.split(",")
    if kind == "certify":
        # Certification cannot authorize separately blocked transitions either.
        for service in services:
            selected(service, "vendor" if service == "vendor-service" else "platform")
        require(len(set(services)) == len(services), "duplicate certification service")
    current(api, sha, context, kind)
    info, actors = environment(api)
    require(context["actor"] in actors, "unauthorized release initiator")
    permission = api.get(f"collaborators/{urllib.parse.quote(context['actor'], safe='')}/permission")["permission"]
    require(permission in {"admin", "maintain"}, "release initiator lacks maintain/admin permission")
    configured_reviewers = next(r["reviewers"] for r in info["protection_rules"] if r["type"] == "required_reviewers")
    reviewers = approval(api, context["run_id"], info["id"], context["actor"], configured_reviewers)
    checks = acceptance(api, sha)
    now = dt.datetime.now(dt.timezone.utc)
    evidence = validate_restore(proof, sha, services, now) if kind == "certify" else restore(api, restore_id, sha, services, info["id"], now, configured_reviewers)
    return {"schema": 1, "release_sha": sha, "services": services, "kind": kind,
            "initiator": context["actor"], "approvers": reviewers, "environment_id": info["id"],
            "environment_protection": info,
            "run_id": context["run_id"], "authorized_at": now.isoformat(), "acceptance": checks, "restore": evidence}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--kind", choices=["platform", "vendor", "certify"], required=True)
    parser.add_argument("--output", default="release-evidence.json")
    args = parser.parse_args()
    context = {k: os.environ.get("GITHUB_" + k.upper(), "") for k in
               ("repository", "event_name", "ref", "sha", "workflow_sha", "run_attempt", "run_id", "actor", "triggering_actor")}
    proof = json.loads(os.environ["RESTORE_MANIFEST"]) if args.kind == "certify" else None
    result = authorize(GitHub(os.environ.get("RELEASE_AUDIT_TOKEN")), context, os.environ["RELEASE_SHA"],
                       os.environ["RELEASE_SERVICES"], args.kind, os.environ.get("RESTORE_RUN_ID", ""), proof)
    Path(args.output).write_text(json.dumps(result, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    if args.kind == "certify":
        Path("restore-evidence.json").write_text(json.dumps(proof, sort_keys=True) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
