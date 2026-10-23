#!/usr/bin/env python3
"""Customer V4 route audit: capture, public-exposure classification, OpenAPI
drift and route-register freshness (V4 §20, §23, B20, B22).

This is the tracked, portable form of the §§19-27 local harness. It builds
every service exactly as production does, with three audit-only changes made
through `go -overlay` (the source tree is never edited):

  * gin's debugPrintRoute prints each route with its FULL handler chain, so
    auth is judged from the real middleware, not from the path spelling. Go
    forbids overlays inside GOMODCACHE, so the pinned gin release is copied
    to a temporary directory, patched there and bound with a generated
    go.work `replace`; nothing vendored is committed;
  * shared/go/serviceapp.Run and any `server.ListenAndServe()` return right
    after route registration when CUSTOMER_V4_ROUTE_AUDIT=1;
  * cmd/main.go skips .env loading, and Media continues without Firebase
    credentials, so no local secret file is ever read.

Each service connects to a disposable database (created here, named
cert_routes_test_*) and to Redis; nothing is served.

Usage: routeaudit.py [capture|classify|drift|register|all] [--check]
Environment: CERT_PG_URL (maintenance URL of a disposable server, e.g.
postgres://user:pass@127.0.0.1:5432/postgres), CERT_REDIS_ADDR.
"""
from __future__ import annotations

import argparse
import collections
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE))
import pgbackup  # noqa: E402  (shared connection handling; never logs credentials)

OUT = Path(os.environ.get("CERT_OUT_DIR", ROOT / "tmp" / "customer-v4-cert")) / "routes"
SERVICES = [
    "api-gateway", "customer-service", "driver-taxi-service", "driver-dispatch-delivery-service",
    "driver-hauling-service", "media-file-service", "payment-wallet-service", "notification-service",
    "support-dispute-service", "verification-compliance-service", "vendor-service",
]
SECRET = "customer-v4-disposable-route-audit-secret-0001"
CALLERS = ["customer-service", "vendor-service", "driver-taxi-service", "driver-hauling-service",
           "driver-dispatch-delivery-service", "admin-backoffice-service", "verification-compliance-service",
           "fleet-partner-service", "analytics-service"]
# Customer V4 surfaces are mounted only behind their flags; the audit turns
# them on so every contract that can be enabled is checked.
CUSTOMER_FLAGS = dict(
    CUSTOMER_SHARE_JOURNEY_ENABLED="true", CUSTOMER_UNIFIED_MOVEMENT_HISTORY_ENABLED="true",
    CUSTOMER_MOVEMENT_EVENTS_CONSUMER_ENABLED="true", CUSTOMER_MOBILITY_PREFERENCES_ENABLED="true",
    CUSTOMER_GUEST_TRAVELLERS_ENABLED="true", CUSTOMER_TRUSTED_CONTACTS_ENABLED="true",
    TAXI_CUSTOMER_LIVE_LOCATION_ENABLED="true", MEDIA_FILE_PRIVATE_API_ENABLED="true",
    PAYMENT_WALLET_FINANCE_PRIVATE_API_ENABLED="true", SUPPORT_NETWORK_CONTEXT_ENABLED="true",
)
SERVICE_FLAGS = {
    # Dedicated standing enrollment stays distinct from the generic disposable
    # service key; startup validates this same isolation in real deployments.
    "customer-service": dict(CUSTOMER_VENDOR_STANDING_SECRET="customer-v4-disposable-standing-only-key-0001"),
	# Gateway reads its upstream inventory from shared/gatewayupstreams, outside
	# the service config scan below. The audit never serves or calls this origin.
	"api-gateway": dict(GATEWAY_UPSTREAM_PARTNER_WEB_URL="http://127.0.0.1:1"),
    # Dispatch's customer secure-handover and recovery routes mount only for
    # an enabled rollout cohort (their config refuses the flag without one).
    "driver-dispatch-delivery-service": dict(
        DISPATCH_HANDOVER_CHALLENGE_ENABLED="true", DISPATCH_HANDOVER_ROLLOUT_SOURCE_TYPES="CUSTOMER",
        DISPATCH_QUEUE_ENABLED="true", DISPATCH_SEQUENTIAL_MATCHING_ENABLED="true",
        DISPATCH_ASSIGNMENT_ROLLOUT_SERVICE_TIERS="standard", DISPATCH_DELIVERY_RECOVERY_ENABLED="true",
        DISPATCH_ROUTE_ALERTS_ENABLED="true", DISPATCH_ROUTE_SERVICEABILITY_ENABLED="true",
        DISPATCH_PACKAGE_POLICY_ENABLED="true"),
    "vendor-service": dict(VENDOR_SERVICE_ENABLED="true", VENDOR_PROCESS_MODE="api", VENDOR_CUSTOMER_STANDING_SECRET="customer-v4-disposable-standing-only-key-0001"),
}
AUDIT_HELPER = '''package main

import "os"

// customerV4RouteAuditExit is compiled in only by scripts/customer-v4/routeaudit.py.
func customerV4RouteAuditExit() {
	if os.Getenv("CUSTOMER_V4_ROUTE_AUDIT") == "1" {
		os.Exit(0)
	}
}
'''


def log(message: str) -> None:
    print(f"[routeaudit] {message}", flush=True)


def go_env(key: str) -> str:
    return subprocess.run(["go", "env", key], capture_output=True, text=True, check=True).stdout.strip()


# --------------------------------------------------------------------------
# build workspace
# --------------------------------------------------------------------------
class Workspace:
    def __init__(self):
        OUT.parent.mkdir(parents=True, exist_ok=True)
        self.dir = Path(tempfile.mkdtemp(prefix="workspace-", dir=OUT.parent))
        gin = subprocess.run(["go", "list", "-m", "-f", "{{.Dir}}", "github.com/gin-gonic/gin"], cwd=ROOT / "shared" / "go",
                             capture_output=True, text=True, check=True).stdout.strip()
        patched = self.dir / "gin"
        shutil.copytree(gin, patched)
        for path in patched.rglob("*"):
            os.chmod(path, 0o755 if path.is_dir() else 0o644)
        debug = patched / "debug.go"
        source = debug.read_text(encoding="utf-8")
        anchor = "func debugPrintRoute(httpMethod, absolutePath string, handlers HandlersChain) {\n"
        if anchor not in source or '"strings"' not in source or '"fmt"' not in source:
            raise SystemExit("gin debug.go changed shape; update routeaudit.py's route hook")
        hook = ('\tnames := []string{}\n\tfor _, h := range handlers {\n\t\tnames = append(names, nameOfFunction(h))\n\t}\n'
                '\tfmt.Printf("V4ROUTE|%s|%s|%s\\n", httpMethod, absolutePath, strings.Join(names, ","))\n')
        debug.write_text(source.replace(anchor, anchor + hook), encoding="utf-8")
        uses = re.findall(r"^\s*(\./\S+)\s*$", (ROOT / "go.work").read_text(encoding="utf-8"), re.M)
        version = re.search(r"^go (\S+)", (ROOT / "go.work").read_text(encoding="utf-8"), re.M).group(1)
        # Relative paths: go.work resolves `use` entries against its own
        # directory, and absolute Windows paths are not matched to modules.
        relative = lambda p: Path(os.path.relpath(p, self.dir)).as_posix()
        lines = [f"go {version}", "use ("] + [f'\t"{relative((ROOT / u).resolve())}"' for u in uses] + [")",
                 "replace github.com/gin-gonic/gin => ./gin", ""]
        self.gowork = self.dir / "go.work"
        self.gowork.write_text("\n".join(lines), encoding="utf-8")
        if (ROOT / "go.work.sum").exists():
            shutil.copy(ROOT / "go.work.sum", self.dir / "go.work.sum")
        self.overlay: dict[str, str] = {}
        serviceapp = ROOT / "shared/go/serviceapp/serviceapp.go"
        text = serviceapp.read_text(encoding="utf-8").replace("\r\n", "\n")
        anchor = "\t\tif opts.RegisterRoot != nil {\n\t\t\topts.RegisterRoot(router)\n\t\t}\n"
        if anchor not in text:
            raise SystemExit("serviceapp.Run changed shape; update routeaudit.py's early return")
        text = text.replace(anchor, anchor + '\t\tif os.Getenv("CUSTOMER_V4_ROUTE_AUDIT") == "1" {\n\t\t\treturn\n\t\t}\n')
        self._add(serviceapp, text)

    def _add(self, original: Path, content: str) -> None:
        target = self.dir / "overlay" / (str(len(self.overlay)) + "_" + original.name)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
        self.overlay[str(original.resolve())] = str(target.resolve())

    def overlay_for(self, service: str) -> Path:
        cmd = ROOT / "services" / service / "cmd"
        main = (cmd / "main.go").read_text(encoding="utf-8").replace("\r\n", "\n")
        main = main.replace("_ = godotenv.Load()", "_ = godotenv.Load")
        main = main.replace('logging.Fatal("firebase", "create storage client: %v", err)',
                            'logging.Notice("firebase", "route audit: storage unavailable: %v", err)')
        main = re.sub(r"(\n(\s*))(if err := \w+\.ListenAndServe\(\))", r"\1customerV4RouteAuditExit()\1\3", main)
        overlay = dict(self.overlay)
        for name, content in (("main.go", main), ("zz_customer_v4_route_audit.go", AUDIT_HELPER)):
            target = self.dir / "overlay" / service / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content, encoding="utf-8")
            overlay[str((cmd / name).resolve())] = str(target.resolve())
        path = self.dir / f"overlay-{service}.json"
        path.write_text(json.dumps({"Replace": overlay}), encoding="utf-8")
        return path

    def close(self) -> None:
        shutil.rmtree(self.dir, ignore_errors=True)


def service_env(service: str, database_url: str, redis: str, workspace: Workspace) -> dict:
    keep = {"PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "HOMEDRIVE",
            "HOMEPATH", "COMSPEC", "PATHEXT", "PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMDATA", "HOME",
            "GOPATH", "GOCACHE", "GOMODCACHE", "XDG_CACHE_HOME", "TMPDIR"}
    env = {k: v for k, v in os.environ.items() if k.upper() in keep}
    env.update(APP_ENV="development", GIN_MODE="release", CUSTOMER_V4_ROUTE_AUDIT="1", HTTP_ADDR="127.0.0.1:0",
               GOPROXY="off", GOTOOLCHAIN="local", GOFLAGS="", MIGRATION="false", GOWORK=str(workspace.gowork))
    config = "\n".join(p.read_text(encoding="utf-8", errors="replace")
                       for p in (ROOT / "services" / service / "internal" / "config").glob("*.go") if not p.name.endswith("_test.go"))
    for key in set(re.findall(r'"([A-Z][A-Z0-9_]+)"', config)):
        if key.endswith("DATABASE_URL"):
            env[key] = database_url
        elif key.endswith("REDIS_ADDR"):
            env[key] = redis
        elif key.endswith(("SERVICE_SECRETS", "INTERNAL_SECRETS")):
            env[key] = ",".join(f"{c}={SECRET}" for c in CALLERS)
        elif key.endswith(("SECRET", "TOKEN", "TOKEN_SECRET", "SIGNING_KEY")):
            env[key] = SECRET
        elif key.endswith(("_URL", "_ORIGIN")):
            env[key] = "http://127.0.0.1:1"
        elif "MIGRATION" in key or key.endswith("WORKERS_ENABLED"):
            env[key] = "false"
    env["HANDOVER_CODE_SECRET"] = SECRET + "-handover"
    env.update(CUSTOMER_FLAGS)
    env.update(SERVICE_FLAGS.get(service, {}))
    return env


def pg_endpoints() -> tuple["pgbackup.Conn", "pgbackup.Conn"]:
    """CERT_PG_URL is how Go code on this host reaches the disposable server;
    CERT_PG_TOOLS_URL is how the PostgreSQL client tools reach it (they may
    run inside PGTOOLS_DOCKER_CONTAINER). Both must name a maintenance DB."""
    url = os.environ.get("CERT_PG_URL")
    if not url:
        raise SystemExit("CERT_PG_URL (maintenance URL of a disposable PostgreSQL server) is required")
    tools = os.environ.get("CERT_PG_TOOLS_URL") or url
    return pgbackup.Conn(url, None), pgbackup.Conn(tools, os.environ.get("PGTOOLS_DOCKER_CONTAINER") or None)


def routes_in(text: str) -> list[str]:
    return [line for line in text.splitlines() if line.startswith("V4ROUTE|")]


def capture(services: list[str]) -> dict:
    host, maintenance = pg_endpoints()
    redis = os.environ.get("CERT_REDIS_ADDR", "127.0.0.1:6379")
    OUT.mkdir(parents=True, exist_ok=True)
    workspace = Workspace()
    counts = {}
    try:
        for service in services:
            database = "cert_routes_test_" + service.replace("-", "_")
            maintenance.run("psql", ["-X", "-q", "-c", f'DROP DATABASE IF EXISTS "{database}"'])
            created = maintenance.run("psql", ["-X", "-q", "-v", "ON_ERROR_STOP=1", "-c", f'CREATE DATABASE "{database}"'])
            if created.returncode:
                raise SystemExit(f"could not create {database}: {pgbackup.scrub(created.stderr)}")
            db_url = host.url_for(database)
            env = service_env(service, db_url, redis, workspace)
            overlay = workspace.overlay_for(service)
            log(f"capturing {service}")
            result = subprocess.run(["go", "run", "-overlay", str(overlay), "./cmd"], cwd=ROOT / "services" / service,
                                    env=env, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=900)
            output = result.stdout + result.stderr
            lines = routes_in(output)
            if result.returncode != 0 or not lines:
                (OUT / f"{service}.error.log").write_text(output, encoding="utf-8")
                raise SystemExit(f"{service}: capture failed (exit {result.returncode}, {len(lines)} routes); see {service}.error.log")
            if service == "vendor-service":
                # Vendor's accepted surface is its all-flags production
                # inventory; the default-flag main.go mounts a subset. Root
                # routes that only main.go registers are merged in.
                inventory = subprocess.run(["go", "test", "-overlay", str(overlay), "./internal/crmcontract",
                                            "-run", "^TestProductionRouteCountIsTheAcceptedSurface$", "-count=1", "-v"],
                                           cwd=ROOT / "services" / service, env=env, capture_output=True, text=True,
                                           encoding="utf-8", errors="replace", timeout=900)
                listed = routes_in(inventory.stdout)
                if inventory.returncode != 0 or not listed:
                    (OUT / "vendor-inventory.error.log").write_text(inventory.stdout + inventory.stderr, encoding="utf-8")
                    raise SystemExit("vendor-service: route inventory capture failed")
                key = lambda line: tuple(line.split("|")[1:3])
                have = {key(line) for line in listed}
                lines = listed + [line for line in lines if key(line) not in have]
            (OUT / f"{service}.log").write_text("\n".join(lines) + "\n", encoding="utf-8")
            counts[service] = len({tuple(line.split("|")[1:3]) for line in lines})
            maintenance.run("psql", ["-X", "-q", "-c", f'DROP DATABASE IF EXISTS "{database}"'])
    finally:
        workspace.close()
    return counts


# --------------------------------------------------------------------------
# classification (nginx + api-gateway resolution)
# --------------------------------------------------------------------------
PORT_SERVICE = {"8080": "api-gateway", "8112": "vendor-service", "8109": "media-file-service"}


def gateway_routes() -> list[tuple[str, str]]:
    """The api-gateway route table, read from its source of truth."""
    source = (ROOT / "services/api-gateway/internal/features/routing/models/routes.go").read_text(encoding="utf-8")
    names = dict(re.findall(r"^\s*(Upstream\w+)\s*=\s*\"([a-z_]+)\"", source, re.M))
    owners = {"customer": "customer-service", "taxi": "driver-taxi-service", "dispatch": "driver-dispatch-delivery-service",
              "hauling": "driver-hauling-service", "payment_wallet": "payment-wallet-service", "notification": "notification-service",
              "support": "support-dispute-service", "vendor": "vendor-service", "fleet": "fleet-partner-service",
              "partner_web": "partner-web-service", "admin": "admin-backoffice-service", "verification": "verification-compliance-service"}
    body = source.split("func DefaultRoutes()", 1)[1].split("\n}\n", 1)[0]
    table = []
    for prefix, constant in re.findall(r'\{\s*"([^"]+)"\s*,\s*(Upstream\w+)\s*,', body):
        upstream = names.get(constant)
        if upstream not in owners:
            raise SystemExit(f"api-gateway upstream {constant} has no known owning service; update routeaudit.gateway_routes")
        table.append((prefix, owners[upstream]))
    if not table:
        raise SystemExit("could not read the api-gateway route table; update routeaudit.gateway_routes")
    return sorted(table, key=lambda r: -len(r[0]))


def gateway_internal(path: str) -> bool:
    lower = path.lower()
    for prefix in ("/api/v1/support-disputes/admin", "/api/v1/verification-compliance"):
        if lower == prefix or lower.startswith(prefix + "/"):
            return True
    seg = lower.strip("/").split("/")
    if len(seg) == 4 and seg[:2] == ["api", "v1"] and seg[3] in ("metrics", "outbox-metrics"):
        return True
    if (lower == "/api/v1/notifications" or lower.startswith("/api/v1/notifications/")) and lower != "/api/v1/notifications/ws":
        return True
    return any(s == "internal" or s == ".." or s.startswith("internal%") for s in lower.split("/"))


def load_nginx():
    exact, regex, prefix = {}, [], []
    text = (ROOT / "deploy/customer-public-ingress.locations.conf").read_text(encoding="utf-8")
    for m in re.finditer(r"location\s+(=|~\*|~|\^~)?\s*(\S+)\s*\{(.*?)\n?\}", text, re.S):
        mod, pat, body = m.group(1), m.group(2), m.group(3)
        target = "DENY"
        pp = re.search(r"proxy_pass\s+http://127\.0\.0\.1:(\d+)", body)
        if pp:
            target = PORT_SERVICE[pp.group(1)]
        methods = re.search(r"limit_except\s+(\w+)", body)
        entry = (pat, target, methods.group(1) if methods else None)
        if mod == "=":
            exact[pat] = entry
        elif mod in ("~", "~*"):
            regex.append((re.compile(pat, re.I if mod == "~*" else 0), entry))
        else:
            prefix.append(entry)
    return exact, regex, prefix


UPDATE_BASELINE = False
HMAC = re.compile(r"(?<![Bb]earer)(?<!Command)(?<!CustomerCommand)(?<!providerBearer)\.Middleware$|ServiceBoundary|BoundaryWithDB|requireNonce|allowCaller|AllowCallers|claimNonce|rejectReplay|custodyops\.RegisterInternal|requireService\b|vendorOnlyAndNonce|RedisBoundary|RegisterPrivateRoutes$|requireAllowedCaller|signed|HMAC|Hmac")
BEARER = re.compile(r"Bearer|AuthRequired|RequireAuth|CustomerCommandMiddleware|CommandMiddleware|RequirePartnerSession|requireCustomer|Session")


def classify() -> dict:
    exact, regex, prefix = load_nginx()
    gateway = gateway_routes()

    def nginx(path, method):
        entry = exact.get(path)
        if entry is None:
            entry = next((e for rx, e in regex if rx.search(path)), None)
        if entry is None:
            entry = max((e for e in prefix if path.startswith(e[0])), key=lambda e: len(e[0]), default=None)
        if entry is None or entry[1] == "DENY" or (entry[2] and method != entry[2]):
            return None
        return entry[1]

    def owner(path, method):
        hop = nginx(path, method)
        if hop != "api-gateway":
            return hop
        if gateway_internal(path):
            return None
        for pfx, upstream in gateway:
            p = pfx.rstrip("/")
            if path == p or path.startswith(p + "/"):
                return upstream
        return None

    rows = []
    for log_file in sorted(OUT.glob("*.log")):
        if log_file.name.endswith(".error.log"):
            continue
        service, seen = log_file.stem, set()
        for line in log_file.read_text(encoding="utf-8", errors="replace").splitlines():
            if not line.startswith("V4ROUTE|"):
                continue
            _, method, path, handlers = (line.split("|", 3) + [""])[:4]
            if (method, path) in seen:
                continue
            seen.add((method, path))
            names = [re.sub(r"\.func\d+(\.\d+)*$", "", h) for h in handlers.split(",") if h]
            hmac = any(HMAC.search(n) for n in names)
            bearer = any(BEARER.search(n) for n in names)
            concrete = re.sub(r"\*\w*", "x", re.sub(r":\w+", "x", path))
            resolved = owner(concrete, method)
            exposed = resolved == service or (service == "api-gateway" and resolved is not None)
            if path in ("/health", "/ready") or path.endswith("/meta"):
                cls = "OPS"
            elif any(s.lower() == "internal" for s in path.split("/")) or hmac:
                cls = "SERVICE_HMAC"
            elif "/admin" in path.lower():
                cls = "ADMIN"
            elif bearer:
                cls = "USER_BEARER"
            else:
                cls = "NO_AUTH_MIDDLEWARE"
            rows.append(dict(service=service, method=method, path=path, cls=cls, exposed=exposed, chain=" ".join(names)))
    (OUT / "classification.json").write_text(json.dumps(rows, indent=1), encoding="utf-8")
    violations = [r for r in rows if r["exposed"] and r["cls"] in ("SERVICE_HMAC", "ADMIN")]
    summary = {}
    for service in sorted({r["service"] for r in rows}):
        rs = [r for r in rows if r["service"] == service]
        summary[service] = {"routes": len(rs), "service_hmac": sum(r["cls"] == "SERVICE_HMAC" for r in rs),
                            "service_hmac_public": sum(r["cls"] == "SERVICE_HMAC" and r["exposed"] for r in rs),
                            "user_bearer": sum(r["cls"] == "USER_BEARER" for r in rs),
                            "public_without_auth_middleware": sum(r["cls"] == "NO_AUTH_MIDDLEWARE" and r["exposed"] for r in rs)}
        log(f"{service:34s} {summary[service]}")
    for r in violations:
        log(f"VIOLATION {r['service']} {r['method']} {r['path']}")
    # Publicly reachable routes without a recognised auth middleware are
    # legitimate only when reviewed (auth entry, estimates, public catalogues,
    # the Share public read, signature-verified webhooks, in-handler-authed
    # websockets). Vendor's inventory router uses its production RequireAuth
    # group. Anything new fails until it is reviewed.
    open_routes = sorted(f"{r['service']} {r['method']} {r['path']}" for r in rows if r["exposed"] and r["cls"] == "NO_AUTH_MIDDLEWARE")
    baseline_path = HERE / "public-unauthenticated-routes.reviewed.txt"
    unreviewed = []
    if UPDATE_BASELINE:
        header = ["# Reviewed publicly reachable routes with no auth middleware (routeaudit.py).",
                  "# Regenerate with: routeaudit.py classify --update-public-baseline"]
        baseline_path.write_text("\n".join(header + open_routes) + "\n", encoding="utf-8")
    elif baseline_path.exists():
        reviewed = {line.strip() for line in baseline_path.read_text(encoding="utf-8").splitlines() if line.strip() and not line.startswith("#")}
        unreviewed = [route for route in open_routes if route not in reviewed]
        for route in unreviewed:
            log(f"UNREVIEWED PUBLIC ROUTE {route}")
    else:
        unreviewed = open_routes
        log("no reviewed public-route baseline; every open route counts as unreviewed")
    return {"services": summary, "violations": len(violations), "unreviewed_public_routes": len(unreviewed)}


# --------------------------------------------------------------------------
# OpenAPI drift + security parity
# --------------------------------------------------------------------------
FAMILIES = {
    "customer-service": [r"^/api/v1/customer/", r"^/internal/customer/"],
    "driver-taxi-service": [r"^/api/v1/taxi/customer/", r"^/internal/taxi/v1/customer-share/"],
    "driver-dispatch-delivery-service": [r"^/api/v1/customer/dispatch/", r"^/internal/dispatch/v1/customer-share/"],
    "driver-hauling-service": [r"^/api/v1/hauling/customer/", r"^/internal/hauling/v1/customer-share/"],
    "vendor-service": [r"^/api/v1/commerce/", r"^/internal/vendor/v1/customer-share/"],
    "media-file-service": [r"^/api/v1/media-files/customer-uploads/"],
}
REGISTER = "customer-v4-route-register.openapi.json"


def norm(path: str) -> str:
    return re.sub(r"\{[^}]+\}|:[A-Za-z_]+|\*[A-Za-z_]*", "{}", path.rstrip("/") or "/")


def openapi_docs(service: str, skip_register: bool = False):
    for f in sorted(glob.glob(str(ROOT / "services" / service / "docs" / "openapi" / "*.json"))):
        if skip_register and os.path.basename(f) == REGISTER:
            continue
        try:
            doc = json.load(open(f, encoding="utf-8"))
        except Exception:
            continue
        bases = [s.get("url", "") for s in doc.get("servers", []) if s.get("url", "").startswith("/")] or [""]
        yield f, doc, bases


def operations(service: str, skip_register: bool = False) -> dict:
    out = {}
    for f, doc, bases in openapi_docs(service, skip_register):
        schemes = (doc.get("components") or {}).get("securitySchemes") or {}
        top = doc.get("security")
        for path, ops in (doc.get("paths") or {}).items():
            for method, op in ops.items():
                if method.lower() not in ("get", "post", "put", "patch", "delete"):
                    continue
                kinds = set()
                for req in (op.get("security", top) if isinstance(op, dict) else top) or []:
                    for name in req:
                        sch = schemes.get(name, {})
                        kinds.add("BEARER" if sch.get("scheme") == "bearer" or "bearer" in name.lower() else "SERVICE")
                for base in set(bases + [""]):
                    out[(method.upper(), norm(base.rstrip("/") + path))] = (kinds, os.path.basename(f))
    return out


def drift() -> dict:
    rows = json.loads((OUT / "classification.json").read_text(encoding="utf-8"))
    undocumented = phantom = mismatch = 0
    for service, patterns in FAMILIES.items():
        docs = operations(service)
        in_family = lambda key: any(re.search(p, key[1]) for p in patterns)
        mounted = {(r["method"], norm(r["path"])): r for r in rows if r["service"] == service
                   and any(re.search(p, r["path"]) for p in patterns) and not r["path"].endswith("/meta")}
        missing = sorted(k for k in mounted if k not in docs)
        ghosts = sorted(k for k in docs if in_family(k) and k not in mounted)
        log(f"drift {service}: mounted {len(mounted)}, undocumented {len(missing)}, phantom {len(ghosts)}")
        for k in missing:
            log(f"  UNDOCUMENTED {k[0]} {k[1]}")
        for k in ghosts:
            log(f"  PHANTOM {k[0]} {k[1]} {docs[k][1]}")
        undocumented += len(missing)
        phantom += len(ghosts)
        for key, r in mounted.items():
            if key not in docs:
                continue
            kinds, f = docs[key]
            want = {"SERVICE_HMAC": {"SERVICE"}, "USER_BEARER": {"BEARER"}}.get(r["cls"])
            ok = ("SERVICE" not in kinds) if want is None else (bool(kinds & want) and not (
                r["cls"] == "USER_BEARER" and "SERVICE" in kinds and "BEARER" not in kinds))
            if not ok:
                mismatch += 1
                log(f"  SECURITY {r['method']} {r['path']} mounted {r['cls']} declared {sorted(kinds) or 'none'} in {f}")
    log(f"drift total {undocumented + phantom}, security mismatch {mismatch}")
    return {"undocumented": undocumented, "phantom": phantom, "security_mismatch": mismatch}


# --------------------------------------------------------------------------
# route register (generated OpenAPI for mounted-but-undocumented customer routes)
# --------------------------------------------------------------------------
REGISTER_FAMILIES = {k: v for k, v in FAMILIES.items() if k != "vendor-service"}


def build_register() -> dict:
    rows = json.loads((OUT / "classification.json").read_text(encoding="utf-8"))
    todo = []
    for service, patterns in REGISTER_FAMILIES.items():
        docs, seen = operations(service, skip_register=True), set()
        for r in rows:
            if r["service"] != service or not any(re.search(p, r["path"]) for p in patterns) or r["path"].endswith("/meta"):
                continue
            key = (r["method"], norm(r["path"]))
            if key in docs or key in seen:
                continue
            seen.add(key)
            todo.append({"service": service, "method": r["method"], "path": r["path"], "cls": r["cls"],
                         "handler": r["chain"].split(" ")[-1] if r["chain"] else ""})
    gen = subprocess.run(["go", "run", ".", str(ROOT)], cwd=HERE / "openapigen", input=json.dumps(todo), capture_output=True,
                         text=True, env={**os.environ, "GOWORK": "off", "GOFLAGS": "-mod=mod"})
    if gen.returncode:
        raise SystemExit("openapigen failed: " + gen.stderr[-2000:])
    ops = json.loads(gen.stdout)
    envelope = {"type": "object", "required": ["success"], "properties": {"success": {"type": "boolean"}, "data": {}}}
    error = {"type": "object", "required": ["success", "error"], "properties": {"success": {"type": "boolean", "enum": [False]},
             "error": {"type": "object", "required": ["code", "message"], "properties": {"code": {"type": "string"},
                       "message": {"type": "string"}, "request_id": {"type": "string"}, "fields": {"type": "array", "items": {}}}}}}
    documents = {}
    for item in todo:
        service = item["service"]
        op = ops.get(service, {}).get(item["method"] + " " + item["path"], {})
        doc = documents.setdefault(service, {
            "openapi": "3.0.3",
            "info": {"title": f"{service} customer route register (Customer V4 §23)", "version": "v4-23",
                     "description": "Mounted customer-family routes that had no OpenAPI document. Every operation is derived from the "
                                    "registered gin route and its handler source (request DTO fields, query and header reads); "
                                    "no field is hand-authored. Response bodies use the platform success/error envelope."},
            "paths": {},
            "components": {"securitySchemes": {
                "CustomerBearer": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
                "ServiceHMAC": {"type": "apiKey", "in": "header", "name": "X-Service-Signature",
                                "description": "HMAC v2 service auth: X-Service-Name, X-Service-Timestamp, X-Service-Nonce and X-Service-Signature; exact caller allowlist; replayed nonce refused."}},
                "schemas": {"SuccessEnvelope": envelope, "ErrorEnvelope": error}}})
        path = re.sub(r":([A-Za-z_]+)", r"{\1}", item["path"])
        params = [{"name": p, "in": "path", "required": True, "schema": {"type": "string"}} for p in re.findall(r"\{([A-Za-z_]+)\}", path)]
        params += [{"name": q, "in": "query", "required": False, "schema": {"type": "string"}} for q in op.get("query") or []]
        params += [{"name": h, "in": "header", "required": h.lower() in ("idempotency-key",) and item["method"] != "GET", "schema": {"type": "string"}}
                   for h in op.get("headers") or [] if h.lower() not in ("authorization",)]
        operation = {
            "operationId": re.sub(r"[^A-Za-z0-9]+", "_", item["method"].lower() + path).strip("_"),
            "x-handler": op.get("handler", item["handler"]),
            "parameters": params,
            "responses": {"200": {"description": "Success", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SuccessEnvelope"}}}},
                          "default": {"description": "Error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorEnvelope"}}}}},
        }
        if item["cls"] == "SERVICE_HMAC":
            operation["security"], operation["x-internal"] = [{"ServiceHMAC": []}], True
        elif item["cls"] == "USER_BEARER":
            operation["security"] = [{"CustomerBearer": []}]
        else:
            operation["security"] = []
        if op.get("body"):
            props = {}
            for f in op["body"]:
                schema = {"type": f["type"]}
                if f["type"] == "array":
                    schema["items"] = {"type": f.get("items") or "object"}
                props[f["name"]] = schema
            operation["requestBody"] = {"required": True, "content": {"application/json": {"schema": {
                "type": "object", "x-go-dto": op.get("body_dto"), "properties": props}}}}
        # Bodies the AST pass cannot reach, read from the handler source.
        if path in ("/internal/customer/v1/movement-events", "/internal/customer/v1/movement-groupings"):
            operation["requestBody"] = {"required": True, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SharedEventEnvelope"}}}}
            doc["components"]["schemas"]["SharedEventEnvelope"] = {
                "type": "object", "x-go-dto": "shared/go/events.Envelope", "additionalProperties": False,
                "required": ["event_id", "event_name", "schema_version", "source_service", "source_entity_type", "source_entity_id", "source_version", "occurred_at", "payload"],
                "properties": {"event_id": {"type": "string"}, "event_name": {"type": "string"}, "schema_version": {"type": "integer"},
                               "source_service": {"type": "string"}, "source_entity_type": {"type": "string"}, "source_entity_id": {"type": "string"},
                               "source_version": {"type": "integer"}, "occurred_at": {"type": "string", "format": "date-time"},
                               "correlation_id": {"type": "string"}, "causation_id": {"type": "string"}, "payload": {"type": "object"}}}
        if path == "/api/v1/media-files/customer-uploads/{id}/content":
            operation["security"] = []
            operation["description"] = "Redeems a one-use customer upload capability issued by N12; the capability, not a service credential, authorizes the bytes."
            operation["parameters"] += [{"name": h, "in": "header", "required": True, "schema": {"type": "string"}}
                                        for h in ("X-Upload-Authorization-ID", "X-Upload-Token", "X-Declared-Size", "X-Declared-Checksum-SHA256", "Content-Type")]
            operation["requestBody"] = {"required": True, "content": {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}}
        if path == "/api/v1/customer/profile/photo":
            operation["requestBody"] = {"required": True, "content": {"multipart/form-data": {"schema": {
                "type": "object", "required": ["photo"], "properties": {"photo": {"type": "string", "format": "binary"}}}}}}
        doc["paths"].setdefault(path, {})[item["method"].lower()] = operation
    return documents


def register(check: bool) -> dict:
    documents = build_register()
    stale = []
    for service, doc in documents.items():
        target = ROOT / "services" / service / "docs" / "openapi" / REGISTER
        text = json.dumps(doc, indent=2, sort_keys=False) + "\n"
        current = target.read_text(encoding="utf-8").replace("\r\n", "\n") if target.exists() else ""
        if current != text:
            stale.append(service)
            if not check:
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(text.encode("utf-8"))
    orphaned = [s for s in REGISTER_FAMILIES if s not in documents and (ROOT / "services" / s / "docs/openapi" / REGISTER).exists()]
    log(f"register {'check' if check else 'write'}: {len(documents)} documents, stale {stale or 'none'}, orphaned {orphaned or 'none'}")
    return {"stale": stale if check else [], "orphaned": orphaned}


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("step", choices=["capture", "classify", "drift", "register", "all"], nargs="?", default="all")
    parser.add_argument("--services", nargs="*", default=SERVICES)
    parser.add_argument("--check", action="store_true", help="register: fail instead of rewriting stale documents")
    parser.add_argument("--update-public-baseline", action="store_true", help="classify: record the current open routes as reviewed")
    args = parser.parse_args(argv)
    global UPDATE_BASELINE
    UPDATE_BASELINE = args.update_public_baseline
    result, failed = {}, False
    if args.step in ("capture", "all"):
        result["captured"] = capture(args.services)
    if args.step in ("classify", "all"):
        result["classification"] = classify()
        failed |= result["classification"]["violations"] > 0 or result["classification"]["unreviewed_public_routes"] > 0
    if args.step in ("drift", "all"):
        result["drift"] = drift()
        failed |= any(result["drift"].values())
    if args.step in ("register", "all"):
        result["register"] = register(args.check or args.step == "all")
        failed |= bool(result["register"]["stale"] or result["register"]["orphaned"])
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / f"summary-{args.step}.json").write_text(json.dumps(result, indent=1, sort_keys=True), encoding="utf-8")
    print(json.dumps(result, sort_keys=True))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
