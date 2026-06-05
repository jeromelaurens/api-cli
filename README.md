# api-cli

`api-cli` provides the `api` command: a local, secret-aware HTTP runner for
agent skills. Skills provide non-secret request inputs; the CLI loads
credentials from the operating-system keyring, applies authentication, and
emits redacted output.

## Product Shape

The installed binary is named `api`.

```text
api request <profile> <METHOD> <path> [--query "..."] [--body file|-] [--header "Name=Value"]
api profile path [--json]
api profile list [--json]
api profile show <profile> [--json]
api secret set <profile> <credential> [--json]
api secret list [<profile>] [--check] [--json]
api secret delete <profile> <credential> [--json]
api --help
api request --help
api profile --help
api secret --help
```

The Go module is:

```text
github.com/jeromelaurens/api-cli
```

## Security Posture

- The agent supplies an API profile name, an HTTP method, a relative path,
  optional query, optional body, and optional non-secret headers.
- Absolute model-provided URLs are rejected.
- Credentials are never stored in the profile catalog, `.env`, OpenClaw config,
  Codex config, MCP config, shell arguments, logs, or model-visible context.
- Credentials live in the OS keyring through `github.com/zalando/go-keyring`
  under the keyring service `api-cli`.
- Keyring keys use `<profile>/<credential>`, for example
  `dolibarr/api-key`.
- The CLI registers every loaded credential and credential-derived value with
  the redactor before emitting output.
- There is no MCP server and no third-party API broker in v1.

See [ADR-0001](docs/adr/0001-local-cli-with-os-keyring.md).

## Profile Catalog

Non-secret API profiles live in:

```text
~/.api/profiles.json
```

On Windows, the same rule is applied from the user home directory:
`%USERPROFILE%\.api\profiles.json`.

Overrides:

1. `--profiles <path>`
2. `API_PROFILES`
3. default `~/.api/profiles.json`

The CLI does not create or modify the profile catalog in v1. Create the
directory with owner-only access and the file with owner-only read/write access
where the operating system supports POSIX-like permissions.

Schema:

```json
{
  "schemaVersion": 1,
  "profiles": {
    "dolibarr": {
      "service": "dolibarr",
      "baseURL": "https://dolibarr.example.com/api/index.php",
      "readOnly": false,
      "auth": {
        "scheme": "header",
        "header": "DOLAPIKEY",
        "credential": "api-key"
      }
    }
  }
}
```

Profile names and credential names use `[a-zA-Z0-9._-]+`.
`readOnly` defaults to `false`. When `readOnly` is `true`, the profile rejects
`POST`, `PUT`, `PATCH`, and `DELETE`.

See [docs/profiles/examples.json](docs/profiles/examples.json) for placeholder
profiles covering the initial target services.

## Auth Schemes

Supported v1 auth schemes:

- `bearer`: `Authorization: Bearer <credential>`
- `header`: a custom header such as `DOLAPIKEY: <credential>`
- `query`: a query parameter such as `api_token=<credential>`
- `basic`: HTTP Basic auth from separate username and password credentials

For query auth, if the agent-provided query already contains the auth parameter,
the request is rejected instead of replacing it.

For Crisp-style Basic auth, the identifier, key, `base64(identifier:key)`, and
`Basic <base64>` value are all registered for redaction.

## Request Contract

Allowed methods:

```text
GET POST PUT PATCH DELETE HEAD OPTIONS
```

The path must be non-empty and start with `/`. Absolute URLs such as
`https://...` and network-path references such as `//host/path` are rejected.

`--query` is a raw query string without a leading `?`. A leading `?` is rejected
instead of silently rewritten.

`--body <path>` and `--body -` read bytes from a file or stdin. There is no
inline body argument. The body does not need to be JSON. If a body is JSON
parseable and no `Content-Type` was supplied, the CLI sends
`Content-Type: application/json`.

Agent-provided headers use `--header "Name=Value"`. Auth headers, cookies,
`User-Agent`, and the header managed by the profile auth scheme are forbidden.

Default request headers:

- `Accept: application/json, */*;q=0.1`
- `User-Agent: api-cli/0.1`

`Content-Type` and `Accept` may be supplied by the agent. `User-Agent` may not.

Default limits:

- timeout: `30s`
- max response body: `5 MiB`

Per-request overrides:

```text
--timeout 10s
--max-body-bytes 1048576
```

## Output Contract

`api request` is agent-first and always emits JSON on stdout. Stderr is empty
during normal request operation.

Admin commands are human-first by default:

- successful `profile` and `secret` commands emit human-readable text on stdout
- failed `profile` and `secret` commands emit human-readable text on stderr
- admin commands exit non-zero on failure

Admin commands accept `--json` for tests, scripts, and agents that need
machine-readable configuration output. In `--json` mode, successful admin
commands emit JSON on stdout; failed admin commands emit JSON with `ok: false`
on stdout and leave stderr empty.

Help commands are the intentional exception: `api --help`, `api help`,
`api request --help`, `api profile --help`, and `api secret --help` emit
human-readable text on stdout and exit `0`. The help text is also intended to be
agent-readable and should include command syntax, config paths, output envelope
shape, auth schemes, forbidden headers, and examples without secrets.

`api request` always emits a response envelope. HTTP `2xx` exits `0`. HTTP
non-`2xx` exits non-zero but uses the same envelope shape with `ok: false`.
For `api request`, CLI, config, keyring, validation, network, timeout, and
size-limit errors also emit JSON with `ok: false` and an `error` object. Admin
commands use that same error shape only in `--json` mode.

JSON response body:

```json
{
  "ok": true,
  "profile": "dolibarr",
  "method": "GET",
  "path": "/thirdparties",
  "status": 200,
  "contentType": "application/json",
  "headers": {
    "x-request-id": "req_123"
  },
  "body": {
    "data": []
  }
}
```

Text response body:

```json
{
  "ok": true,
  "profile": "crisp",
  "method": "GET",
  "path": "/status",
  "status": 200,
  "contentType": "text/plain",
  "bodyText": "ok"
}
```

Binary response body:

```json
{
  "ok": true,
  "profile": "sentry",
  "method": "GET",
  "path": "/artifact",
  "status": 200,
  "contentType": "application/octet-stream",
  "bodyEncoding": "base64",
  "bodyBase64": "..."
}
```

CLI error:

```json
{
  "ok": false,
  "profile": "dolibarr",
  "method": "GET",
  "path": "/thirdparties",
  "error": {
    "code": "credential_missing",
    "message": "credential api-key is missing for profile dolibarr"
  }
}
```

Successful admin commands in `--json` mode do not include `ok`.

```json
{"profiles":["dolibarr"],"catalogExists":true}
```

The same result is text by default:

```text
Profiles catalog: /Users/me/.api/profiles.json
Profiles:
  dolibarr
```

## Response Headers

The response envelope includes `contentType` and an allowlisted `headers` object.
Allowed response headers:

- `request-id`
- `x-request-id`
- `x-correlation-id`
- `x-ratelimit-limit`
- `x-ratelimit-remaining`
- `x-ratelimit-reset`
- `retry-after`

Included headers are redacted before output.

## Error Codes

V1 error codes:

- `invalid_args`
- `profile_catalog_missing`
- `profile_catalog_invalid`
- `profile_not_found`
- `credential_missing`
- `keyring_unavailable`
- `invalid_request_path`
- `invalid_query`
- `forbidden_header`
- `auth_query_collision`
- `profile_read_only`
- `request_timeout`
- `response_too_large`
- `http_error`
- `network_error`
- `internal_error`

Exit codes are intentionally simple:

- `0`: HTTP `2xx` or successful admin command
- `1`: all structured failures, including HTTP non-`2xx`

## Agent Usage

Skills should call the CLI with profile names and relative paths only:

```sh
api request dolibarr GET /thirdparties --query "limit=10"
```

For request bodies, use stdin or a temporary file:

```sh
printf '%s' '{"name":"Acme"}' | api request dolibarr POST /thirdparties --body -
```

Skills parse stdout JSON and inspect `ok`, `status`, `body`, `bodyText`, or
`bodyBase64`. Skills must not pass credentials, absolute URLs, auth headers, or
auth query parameters.

## Implementation Notes

- Use `cmd/api` for the command.
- Use `internal/runner` as the main internal package.
- Keep endpoint knowledge in skills, not in `api`.
- Use standard-library CLI parsing, `golang.org/x/term` for masked input, and
  `github.com/zalando/go-keyring` for OS keyring access.
- The automated binary e2e test builds with the test-only build tag
  `apicli_e2e_mock_keyring`; the normal binary has no runtime credential-store
  override.
