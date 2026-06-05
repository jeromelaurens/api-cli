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

Profile catalog:
  Default path: ~/.api/profiles.json
  Override priority: --profiles, API_PROFILES, default path
  The CLI does not create or edit profiles in v1.
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
