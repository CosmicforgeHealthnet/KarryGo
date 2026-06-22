#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PGPASSWORD="${PGPASSWORD:-cosmicforge_logistics}"

exec psql "postgres://cosmicforge_logistics:${PGPASSWORD}@localhost:5433/customer_service?sslmode=disable" "$@"
