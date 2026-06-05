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
  api profile create <profile> [--service name] [--base-url url] [--auth scheme] [auth flags] [--read-only] [--json]
  api profile update <profile> [--json]
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
  api request billing GET /invoices --query "limit=10"
  api profile list --json
  api profile create billing --service billing --base-url "https://api.example.test/v1" --auth bearer --credential api-token
  api secret set billing api-token
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
  api profile create <profile> [--service name] [--base-url url] [--auth bearer|header|query|basic] [--read-only] [--json]
  api profile update <profile> [--json]

Profile catalog:
  Default path: ~/.api/profiles.json
  Override priority: --profiles, API_PROFILES, default path
  profile create writes non-secret profile data only. It does not store credentials.
  profile update modifies non-secret profile data only. It does not store credentials.

Auth flags:
  bearer: --credential name
  header: --header name --credential name
  query: --param name --credential name
  basic: --username-credential name --password-credential name

Interactive mode:
  Missing non-secret fields are prompted. The service name defaults to the profile name.
  profile update shows current values in brackets. Press Enter to keep them.

Examples:
  api profile create billing --service billing --base-url "https://api.example.test/v1" --auth header --header X-API-Key --credential api-key
  api profile update billing
  api profile create support --service support --base-url "https://support.example.test/v1" --auth basic --username-credential identifier --password-credential key
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
