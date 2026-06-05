package runner

func HelpText(topic string) string {
	switch topic {
	case "request":
		return requestHelp
	case "profile":
		return profileHelp
	case "secret":
		return secretHelp
	default:
		return globalHelp
	}
}

const globalHelp = `Usage:
  api [--profiles <path>] <command> [args]
  api help [command]

Commands:
  api request <profile> <METHOD> <path> [--query "..."] [--body file|-] [--header "Name=Value"]
  api profile path [--json]
  api profile list [--json]
  api profile show <profile> [--json]
  api profile create <profile> [--preset name] [--base-url url] [--read-only] [--json]
  api secret set <profile> <credential> [--json]
  api secret list [<profile>] [--check] [--json]
  api secret delete <profile> <credential> [--json]

Profile catalog:
  Default path: ~/.api/profiles.json
  Override priority: --profiles, API_PROFILES, default path

Response envelope:
  api request always writes JSON to stdout with ok, status, contentType, headers, body/bodyText/bodyBase64, or error.code.

Forbidden headers:
  Authorization, Cookie, Set-Cookie, User-Agent, and any auth header managed by the selected profile.

Auth schemes:
  bearer, header, query, basic.

Examples:
  api request dolibarr GET /thirdparties --query "limit=10"
  api profile list --json
  api profile create mailerlite --preset mailerlite
  api secret set crisp identifier
`

const requestHelp = `Usage:
  api request <profile> <METHOD> <path> [--query "..."] [--body file|-] [--header "Name=Value"] [--timeout 30s] [--max-body-bytes 5242880]

Methods:
  GET POST PUT PATCH DELETE HEAD OPTIONS

Request policy:
  The path must start with / and absolute URLs are rejected.
  --query is a raw query string without a leading ?.
  --body reads a file or stdin when the value is -.
  JSON bodies infer Content-Type: application/json when no Content-Type is provided.

Response envelope:
  JSON body uses body, UTF-8 text uses bodyText, binary uses bodyBase64.

Forbidden headers:
  Authorization, Cookie, Set-Cookie, User-Agent, and profile-managed auth headers.
`

const profileHelp = `Usage:
  api profile path [--json]
  api profile list [--json]
  api profile show <profile> [--json]
  api profile create <profile> [--preset dolibarr|mailerlite|pipedrive|pennylane|crisp|sentry] [--base-url url] [--read-only] [--json]

Profile catalog:
  Default path: ~/.api/profiles.json
  Override priority: --profiles, API_PROFILES, default path
  profile create writes non-secret profile data only. It does not store credentials.

Presets:
  --preset dolibarr prompts for the instance base URL and uses DOLAPIKEY with credential api-key.
  --preset mailerlite uses https://connect.mailerlite.com/api and credential api-token.
  --preset pipedrive uses https://api.pipedrive.com/v1 and query credential api-token.
  --preset pennylane uses https://app.pennylane.com/api/external/v2 and credential access-token.
  --preset crisp uses https://api.crisp.chat/v1 and credentials identifier and key.

Examples:
  api profile create dolibarr --preset dolibarr
  api profile create mailerlite --preset mailerlite
`

const secretHelp = `Usage:
  api secret set <profile> <credential> [--json]
  api secret list [<profile>] [--check] [--json]
  api secret delete <profile> <credential> [--json]

Credential storage:
  Credentials are stored in the OS keyring service api-cli with keys <profile>/<credential>.
  There is no secret get command.
  secret set reads from a masked prompt when attached to a terminal or from stdin when piped.
`
