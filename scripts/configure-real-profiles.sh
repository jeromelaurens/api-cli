#!/usr/bin/env sh
set -eu

API_BIN="${API_BIN:-api}"

if [ "${DOLIBARR_BASE_URL:-}" = "" ]; then
  echo "DOLIBARR_BASE_URL is required, for example https://example.test/api/index.php" >&2
  exit 2
fi

create_profile() {
  name="$1"
  shift
  if ${API_BIN} profile show "$name" --json >/dev/null 2>&1; then
    echo "Profile already exists: $name"
    return 0
  fi
  ${API_BIN} profile create "$name" "$@"
}

# Equivalent command shapes:
#   api profile create dolibarr --service dolibarr --base-url "$DOLIBARR_BASE_URL" --auth header --header DOLAPIKEY --credential api-key
#   api profile create mailerlite --service mailerlite --base-url "https://connect.mailerlite.com/api" --auth bearer --credential api-token
#   api profile create pipedrive --service pipedrive --base-url "https://api.pipedrive.com/v1" --auth query --param api_token --credential api-token
#   api profile create pennylane --service pennylane --base-url "https://app.pennylane.com/api/external/v2" --auth bearer --credential access-token
#   api profile create crisp --service crisp --base-url "https://api.crisp.chat/v1" --auth basic --username-credential identifier --password-credential key

create_profile dolibarr \
  --service dolibarr \
  --base-url "${DOLIBARR_BASE_URL}" \
  --auth header \
  --header DOLAPIKEY \
  --credential api-key

create_profile mailerlite \
  --service mailerlite \
  --base-url "https://connect.mailerlite.com/api" \
  --auth bearer \
  --credential api-token

create_profile pipedrive \
  --service pipedrive \
  --base-url "https://api.pipedrive.com/v1" \
  --auth query \
  --param api_token \
  --credential api-token

create_profile pennylane \
  --service pennylane \
  --base-url "https://app.pennylane.com/api/external/v2" \
  --auth bearer \
  --credential access-token

create_profile crisp \
  --service crisp \
  --base-url "https://api.crisp.chat/v1" \
  --auth basic \
  --username-credential identifier \
  --password-credential key
