#!/usr/bin/env bash
# Section 20/29 external exposure scan for an AUTHORIZED staging target.
#
# NOT EXECUTED BY THE AUTHORING CHANGE. This script was written and syntax-
# checked only; it has never been run against real infrastructure from the
# repository. Run it only against a host you are explicitly authorized to test,
# from OUTSIDE the server (a laptop or CI runner on the public internet), never
# from the server itself (loopback would hide the very exposure it looks for).
#
#   external_scan.sh --i-am-authorized \
#       --base-url https://api-gateway.staging.karryam.com \
#       --host api-gateway.staging.karryam.com \
#       [--out evidence.json] [--timeout 5] [--no-ports]
#
# It probes a matrix of admin/internal paths x methods x encodings x hostile
# headers over HTTP(S), and TCP connects to the private service/data ports.
# PASS means: every HTTP probe answered 404 (TRACE/CONNECT may instead be
# refused by the web server at the protocol level: 400/405/501), and every TCP
# port is closed or filtered. Anything else -- including an unreachable
# target, which proves nothing -- exits non-zero.
#
# It sends no credentials, accepts no tokens, rejects URLs with userinfo, and
# never prints or stores response bodies or headers: only status codes.
set -euo pipefail

authorized=0 base_url="" host="" out="" timeout=5 scan_ports=1
usage() { sed -n '2,24p' "$0" >&2; exit 2; }
while [ $# -gt 0 ]; do
  case "$1" in
    --i-am-authorized) authorized=1 ;;
    --base-url) base_url="${2:-}"; shift ;;
    --host) host="${2:-}"; shift ;;
    --out) out="${2:-}"; shift ;;
    --timeout) timeout="${2:-}"; shift ;;
    --no-ports) scan_ports=0 ;;
    -h|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
  shift
done

[ "$authorized" = 1 ] || { echo "refusing: pass --i-am-authorized only for a target you are authorized to scan" >&2; exit 2; }
[ -n "$base_url" ] && [ -n "$host" ] || { echo "refusing: --base-url and --host are both required" >&2; exit 2; }
[[ "$base_url" =~ ^https?://[A-Za-z0-9.-]+(:[0-9]+)?/?$ ]] || { echo "refusing: --base-url must be scheme://host[:port] with no path, credentials or query" >&2; exit 2; }
[[ "$host" =~ ^[A-Za-z0-9.-]{1,253}$ ]] || { echo "refusing: --host must be a bare hostname or IPv4 address" >&2; exit 2; }
[[ "$timeout" =~ ^[0-9]{1,2}$ ]] && [ "$timeout" -ge 1 ] || { echo "refusing: --timeout must be 1-99 seconds" >&2; exit 2; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 2; }
base_url="${base_url%/}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="${out:-exposure-scan-$stamp.json}"

json_escape() { local s="$1"; s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; printf '%s' "$s"; }

PATHS=(
  /api/v1/admin /api/v1/admin/ /api/v1/admin/users /API/V1/ADMIN /API/V1/ADMIN/USERS /Api/V1/Admin/
  /api/v1%2Fadmin/users /api/v1%2fadmin /api%2Fv1%2Fadmin%2Fusers /api/v1%252Fadmin/users /api/v1/%61dmin/users
  /api/v1/customer/%2e%2e/admin/users /api/v1/customer/%2E%2E/admin/users /api/v1/customer/%252e%252e/admin/users
  /api/v1/customer/../admin/users /api/v1/./admin/users //api/v1/admin/users /api//v1//admin/users
  /api/v1/customer/..%2fadmin /api/v1/customer/%5c..%5cadmin /api/v1/admin%00/users '/api/v1/admin;x=1/users'
  /internal /internal/ /internal/karryam-admin/v1/capabilities /INTERNAL/karryam-admin/v1/capabilities
  /internal/vendor/v1/admin/commands /api/v1/internal/callbacks/dispatch /api/v1/customer/internal/admin/customers/x
  /api/v1/taxi/%2569nternal/x /api/v1/taxi/internal%2Fx /api/v1/support-disputes/admin/complaints
  /api/v1/verification-compliance/cases /api/v1/taxi/metrics /api/v1/taxi/outbox-metrics /api/v1/notifications/send
)
METHODS=(GET HEAD POST PUT PATCH DELETE OPTIONS TRACE CONNECT)
# name|curl header arguments (no secrets; all values are fixed probes)
VARIANTS=(
  "plain|"
  "websocket|-H Upgrade:websocket -H Connection:Upgrade -H Sec-WebSocket-Version:13 -H Sec-WebSocket-Key:dGhlIHNhbXBsZSBub25jZQ=="
  "h2c|-H Upgrade:h2c -H Connection:Upgrade,HTTP2-Settings -H HTTP2-Settings:AAMAAABkAARAAAAAAAIAAAAA"
  "forwarded-host|-H X-Forwarded-Host:admin-backoffice-service:8110 -H Forwarded:host=admin-backoffice-service -H X-Original-URL:/api/v1/admin/users -H X-Rewrite-URL:/api/v1/admin/users"
  "internal-host|-H Host:admin-backoffice-service:8110"
)

failures=0 probes=0
results_file="$(mktemp)"; trap 'rm -f "$results_file"' EXIT

record() { # kind target method variant observed verdict
  printf '{"kind":"%s","target":"%s","method":"%s","variant":"%s","observed":"%s","verdict":"%s"},' \
    "$1" "$(json_escape "$2")" "$3" "$4" "$5" "$6" >>"$results_file"
}

http_probe() { # method path variant-name variant-args [request-target]
  local method="$1" path="$2" name="$3" args="$4" target="${5:-}" code verdict
  local -a extra=()
  # shellcheck disable=SC2206
  [ -n "$args" ] && extra=($args)
  [ -n "$target" ] && extra+=(--request-target "$target")
  if [ "$method" = HEAD ]; then extra+=(--head); else extra+=(-X "$method"); fi
  code="$(curl -sS -g --path-as-is -o /dev/null -w '%{http_code}' --max-time "$timeout" \
    --proto '=http,https' --max-redirs 0 "${extra[@]}" "$base_url$path" 2>/dev/null || true)"
  code="${code:-000}"
  probes=$((probes + 1))
  if [ "$code" = 404 ]; then verdict=pass
  elif { [ "$method" = TRACE ] || [ "$method" = CONNECT ]; } && [[ "$code" =~ ^(400|405|501)$ ]]; then verdict=pass-protocol-refusal
  else verdict=FAIL; failures=$((failures + 1)); echo "FAIL $method ${target:-$path} [$name] -> $code" >&2
  fi
  record http "${target:-$path}" "$method" "$name" "$code" "$verdict"
}

# Control: the target must be reachable and answering, or the scan proves nothing.
control="$(curl -sS -o /dev/null -w '%{http_code}' --max-time "$timeout" "$base_url/health" 2>/dev/null || true)"
if [ -z "$control" ] || [ "$control" = 000 ]; then
  echo "FAIL: control request to $base_url/health got no answer; refusing to report a clean scan" >&2
  failures=$((failures + 1))
fi
record control "/health" GET plain "${control:-000}" "$([ "${control:-000}" != 000 ] && echo reachable || echo FAIL)"

for path in "${PATHS[@]}"; do
  for method in "${METHODS[@]}"; do
    for variant in "${VARIANTS[@]}"; do
      http_probe "$method" "$path" "${variant%%|*}" "${variant#*|}"
    done
  done
done
# Absolute-form request targets naming the private listener directly.
for target in "http://admin-backoffice-service:8110/api/v1/admin/users" \
              "http://admin-backoffice-service:8110/internal/karryam-admin/v1/capabilities" \
              "http://127.0.0.1:8110/api/v1/admin" "http://karrygo-admin-db:5432/"; do
  for method in GET HEAD OPTIONS; do
    http_probe "$method" "/" absolute-form "" "$target"
  done
done

PORTS=(4100 4101 4103 8080 8110 5432 6379)
for port in $(seq 8101 8116); do PORTS+=("$port"); done
if [ "$scan_ports" = 1 ]; then
  command -v timeout >/dev/null || { echo "coreutils timeout is required for port probes" >&2; exit 2; }
  for port in "${PORTS[@]}"; do
    probes=$((probes + 1))
    if timeout "$timeout" bash -c "exec 3<>/dev/tcp/$host/$port" 2>/dev/null; then
      failures=$((failures + 1)); echo "FAIL tcp $host:$port is OPEN" >&2
      record tcp "$host:$port" CONNECT - open FAIL
    else
      record tcp "$host:$port" CONNECT - closed-or-filtered pass
    fi
  done
fi

results="$(sed 's/,$//' "$results_file")"
cat >"$out" <<JSON
{"section":"20/29 external exposure scan","generated_at":"$stamp","base_url":"$(json_escape "$base_url")","host":"$(json_escape "$host")","probes":$probes,"failures":$failures,"passed":$([ "$failures" = 0 ] && echo true || echo false),"results":[${results}]}
JSON
echo "evidence: $out ($probes probes, $failures failures)"
[ "$failures" = 0 ]
