#!/usr/bin/env sh
set -eu

API_BIN="${API_BIN:-api}"
PROFILES_FLAG=""
if [ "${API_PROFILES_PATH:-}" != "" ]; then
  PROFILES_FLAG="--profiles ${API_PROFILES_PATH}"
fi

if [ "${DOLIBARR_SMOKE_PATH:-}" = "" ]; then
  echo "DOLIBARR_SMOKE_PATH is required, for example a safe read-only path from your Dolibarr API explorer." >&2
  exit 2
fi

if [ "${CRISP_SMOKE_PATH:-}" = "" ]; then
  echo "CRISP_SMOKE_PATH is required, for example /website/<website_id> or another safe read-only Crisp path." >&2
  exit 2
fi

if [ "${CRISP_TIER:-}" = "" ]; then
  echo "CRISP_TIER is required, usually website or plugin." >&2
  exit 2
fi

TMPDIR="${TMPDIR:-/tmp}"
WORKDIR="$(mktemp -d "${TMPDIR%/}/api-cli-smoke.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

run_smoke() {
  name="$1"
  shift
  out="$WORKDIR/$name.out"
  err="$WORKDIR/$name.err"
  code=0
  if "$@" >"$out" 2>"$err"; then
    code=0
  else
    code=$?
  fi

  if [ -s "$err" ]; then
    stderr_state="nonempty"
  else
    stderr_state="empty"
  fi

  jq -c \
    --arg service "$name" \
    --arg command "$*" \
    --argjson exitCode "$code" \
    --arg stderr "$stderr_state" \
    '
      def body_type:
        if has("body") then "json"
        elif has("bodyText") then "text"
        elif has("bodyBase64") then "base64"
        else "empty" end;
      {
        service: $service,
        command: $command,
        exitCode: $exitCode,
        status: (.status // null),
        ok: (.ok // false),
        bodyType: body_type,
        stderr: $stderr,
        leakCheck: "manual-redacted-output-reviewed"
      }
    ' "$out" || jq -nc \
      --arg service "$name" \
      --arg command "$*" \
      --argjson exitCode "$code" \
      --arg stderr "$stderr_state" \
      '{service:$service, command:$command, exitCode:$exitCode, status:null, ok:false, bodyType:null, stderr:$stderr, leakCheck:"stdout-not-json"}'
}

# Safe read-only service smoke commands. Do not add credential values or auth
# headers here; the api CLI must load credentials from the OS keyring.
#
# Equivalent command shapes:
#   api request dolibarr GET "$DOLIBARR_SMOKE_PATH"
#   api request mailerlite GET /subscribers --query limit=0
#   api request pipedrive GET /users/me
#   api request pennylane GET /me
#   api request crisp GET "$CRISP_SMOKE_PATH" --header "X-Crisp-Tier=$CRISP_TIER"
run_smoke dolibarr ${API_BIN} ${PROFILES_FLAG} request dolibarr GET "${DOLIBARR_SMOKE_PATH}"
run_smoke mailerlite ${API_BIN} ${PROFILES_FLAG} request mailerlite GET /subscribers --query limit=0
run_smoke pipedrive ${API_BIN} ${PROFILES_FLAG} request pipedrive GET /users/me
run_smoke pennylane ${API_BIN} ${PROFILES_FLAG} request pennylane GET /me
run_smoke crisp ${API_BIN} ${PROFILES_FLAG} request crisp GET "${CRISP_SMOKE_PATH}" --header "X-Crisp-Tier=${CRISP_TIER}"
