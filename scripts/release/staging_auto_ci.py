"""Fail-closed automatic staging acceptance; no release approval or SSH."""
import os
import re
import time
import gate

# All unconditional staging acceptance workflows. Keep the legacy approval
# policy unchanged. Path-triggered test suites are checked when they run.
REQUIRED = {**gate.ACCEPTANCE, "admin-sections-16-38.yml": 1, "email-branding.yml": 1}
OPTIONAL = {"fleet-auth-v2.yml"}


def check_ci(api, sha):
    evidence = []
    pending = []
    runs = api.pages(f"actions/runs?head_sha={sha}&event=push", "workflow_runs")
    by_path = {}
    for run in runs:
        if run.get("head_sha") != sha or run.get("event") != "push" or run.get("head_branch") != "staging":
            continue
        path = run["path"].split("@")[0].rsplit("/", 1)[-1]
        if path not in by_path or run["id"] > by_path[path]["id"]:
            by_path[path] = run
    required = dict(REQUIRED)
    required.update({path: 1 for path in OPTIONAL if path in by_path})
    for path, minimum in required.items():
        run = by_path.get(path)
        if run is None or run.get("status") != "completed":
            pending.append(path)
            continue
        gate.require(run.get("conclusion") == "success", "required CI failed: " + path)
        jobs = api.pages(f"actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs", "jobs")
        gate.require(len(jobs) >= minimum and all(job.get("conclusion") == "success" for job in jobs),
                     "CI jobs missing, skipped or failed: " + path)
        evidence.append((path, run["id"], run["run_attempt"]))
    return pending, evidence


def wait_for_ci(api, sha, timeout=1800, interval=15):
    gate.require(re.fullmatch(r"[0-9a-f]{40}", sha), "invalid staging SHA")
    deadline = time.monotonic() + timeout
    while True:
        gate.require(api.get("git/ref/heads/staging")["object"]["sha"] == sha,
                     "staging advanced; refusing stale deployment")
        pending, evidence = check_ci(api, sha)
        if not pending:
            gate.require(api.get("git/ref/heads/staging")["object"]["sha"] == sha,
                         "staging advanced during CI verification")
            for path, run, attempt in evidence:
                print(f"CI_OK {path} run={run} attempt={attempt}", flush=True)
            return
        if time.monotonic() >= deadline:
            raise RuntimeError("required CI did not finish: " + ", ".join(pending))
        print("Waiting for " + ", ".join(pending), flush=True)
        time.sleep(interval)


if __name__ == "__main__":
    gate.require(os.environ.get("GITHUB_REF") == "refs/heads/staging", "staging only")
    wait_for_ci(gate.GitHub(os.environ.get("GH_TOKEN")), os.environ["GITHUB_SHA"])
