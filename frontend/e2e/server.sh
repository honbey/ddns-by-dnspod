#!/usr/bin/env bash
# Builds the app and serves it once for the Playwright run: real binary, real
# OIDC verification against the local identity provider.
#
# PLAYWRIGHT_BROWSERS_PATH and GOPROXY come from the environment unchanged; set
# them where the browsers and the module proxy actually live.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
export GOPATH="${GOPATH:-$HOME/.go}"

DATA="$(mktemp -d /tmp/ddns-e2e.XXXXXX)"
IDP_PORT="${IDP_PORT:-9443}"
APP_PORT="${APP_PORT:-3998}"
ISSUER="http://127.0.0.1:${IDP_PORT}"
CLIENT_ID="ddns-by-dnspod"
APP_ORIGIN="http://127.0.0.1:${APP_PORT}"
# AES-256 key, generated per run so the test never reuses a deployment's file.
CRYPTO_KEY="$(head -c 32 /dev/urandom | base64)"

idp_pid=""
app_pid=""

cleanup() {
  [[ -n "$app_pid" ]] && kill "$app_pid" 2>/dev/null || true
  [[ -n "$idp_pid" ]] && kill "$idp_pid" 2>/dev/null || true
  rm -rf "$DATA"
}
trap cleanup EXIT INT TERM

cd "$ROOT/frontend"
npm run build

cd "$ROOT/backend"
go build -o "$DATA/ddns-server" ./cmd/server

PORT="$IDP_PORT" CLIENT_ID="$CLIENT_ID" node "$ROOT/frontend/e2e/mock-idp.mjs" &
idp_pid=$!

# The scheduler and the certificate manager are real network work with no
# bearing on the UI; they stay off so the suite is about the browser.
#
DDNS_CRYPTO_KEY="$CRYPTO_KEY" \
  DDNS_SERVER_LISTEN="127.0.0.1:${APP_PORT}" \
  DDNS_DATABASE_PATH="$DATA/e2e.db" \
  DDNS_OIDC_ISSUER="$ISSUER" \
  DDNS_OIDC_CLIENT_ID="$CLIENT_ID" \
  DDNS_FRONTEND_SCOPE="openid profile email groups offline_access" \
  DDNS_FRONTEND_PROVIDER_NAME="Local IdP" \
  DDNS_SCHEDULER_ENABLED=false \
  DDNS_ACME_ENABLED=false \
  "$DATA/ddns-server" &
app_pid=$!

wait
