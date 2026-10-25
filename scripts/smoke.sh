#!/usr/bin/env bash
# API smoke test. Requires a running control plane and etcd.
# With at least one node agent and Docker, it also waits for the pod to run.
# Usage: API=http://127.0.0.1:8080 ./scripts/smoke.sh
set -euo pipefail

API="${API:-http://127.0.0.1:8080}"
POD="smoke-$(date +%s)"
pass() { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1" >&2; exit 1; }

# status <method> <path> [json-body] -> prints HTTP status, body saved in $BODY_FILE
BODY_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE"; curl -s -o /dev/null -X DELETE "$API/api/v1/pods/$POD" || true' EXIT
status() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -s -o "$BODY_FILE" -w '%{http_code}' -X "$method" -H 'Content-Type: application/json' -d "$body" "$API$path"
  else
    curl -s -o "$BODY_FILE" -w '%{http_code}' -X "$method" "$API$path"
  fi
}

echo "Smoke testing $API"
[[ "$(status GET /healthz)" == 200 ]] && pass "liveness /healthz" || fail "/healthz"
[[ "$(status GET /readyz)" == 200 ]] && pass "readiness /readyz (etcd reachable)" || fail "/readyz: $(cat "$BODY_FILE")"
[[ "$(status POST /api/v1/pods '{"name":"Bad_Name","image":"nginx"}')" == 400 ]] && pass "validation rejects bad name" || fail "validation"
[[ "$(status POST /api/v1/pods "{\"name\":\"$POD\",\"image\":\"nginx:stable\"}")" == 201 ]] && pass "create pod $POD" || fail "create: $(cat "$BODY_FILE")"
[[ "$(status POST /api/v1/pods "{\"name\":\"$POD\",\"image\":\"nginx:stable\"}")" == 409 ]] && pass "duplicate rejected with 409" || fail "duplicate"
[[ "$(status GET "/api/v1/pods/$POD")" == 200 ]] && pass "get pod" || fail "get pod"
[[ "$(status GET /api/v1/nodes)" == 200 ]] && pass "list nodes" || fail "list nodes"
[[ "$(status GET /api/v1/events?limit=5)" == 200 ]] && pass "list events" || fail "list events"

if grep -q '"ready":true' <(curl -s "$API/api/v1/nodes"); then
  for _ in $(seq 1 60); do
    status GET "/api/v1/pods/$POD" >/dev/null
    if grep -q '"phase":"Running"' "$BODY_FILE"; then pass "pod reached Running"; break; fi
    if grep -q '"phase":"Failed"' "$BODY_FILE"; then fail "pod failed: $(cat "$BODY_FILE")"; fi
    sleep 2
  done
  grep -q '"phase":"Running"' "$BODY_FILE" || fail "pod did not reach Running within 120s"
  [[ "$(status GET "/api/v1/pods/$POD/logs?tailLines=20")" == 200 ]] && pass "logs fetched through the agent" || fail "logs: $(cat "$BODY_FILE")"
else
  echo "  skip lifecycle checks: no ready node (start one with: make run-agent)"
fi

code="$(status DELETE "/api/v1/pods/$POD")"
[[ "$code" == 200 || "$code" == 202 ]] && pass "delete pod ($code)" || fail "delete: $code"
echo "Smoke test passed."
