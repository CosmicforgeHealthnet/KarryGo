"""Wait for the exact staging revision's existing mandatory CI gates.

No release approvals, deployment credentials, or server access are used here.
The existing explicit release gate is unchanged.
"""
import os
import re
import time
import gate


def wait_for_ci(api, sha, timeout=1800, interval=15):
    gate.require(re.fullmatch(r"[0-9a-f]{40}", sha), "invalid staging SHA")
    deadline = time.monotonic() + timeout
    while True:
        gate.require(api.get("git/ref/heads/staging")["object"]["sha"] == sha,
                     "staging advanced; refusing stale deployment")
        try:
            evidence = gate.acceptance(api, sha)
        except ValueError as exc:
            # Missing runs and queued/in-progress runs may be registered later.
            # A completed failure must block rather than wait for a different SHA.
            if not str(exc).startswith(("missing acceptance:", "CI incomplete/failed:")):
                raise
            if time.monotonic() >= deadline:
                raise RuntimeError("required CI did not pass before deadline") from exc
            print(f"Waiting for required CI: {exc}", flush=True)
            time.sleep(interval)
        else:
            gate.require(api.get("git/ref/heads/staging")["object"]["sha"] == sha,
                         "staging advanced during CI verification")
            for item in evidence:
                print(f"CI_OK {item['workflow']} run={item['run_id']} attempt={item['attempt']}")
            return


if __name__ == "__main__":
    gate.require(os.environ.get("GITHUB_REF") == "refs/heads/staging", "staging only")
    wait_for_ci(gate.GitHub(os.environ.get("GH_TOKEN")), os.environ["GITHUB_SHA"])
