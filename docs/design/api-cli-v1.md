# API CLI v1 Design

This document consolidates the validated v1 design for `api-cli`, which installs
the local `api` command. It is a design checkpoint, not an ADR.

## Goal

`api` lets OpenClaw/Codex skills call configured business APIs without exposing
credentials to the model. The tool is a local CLI, not an MCP server and not a
third-party broker integration.

The core command is:

```text
api request <profile> <METHOD> <path> [--query "..."] [--body file|-] [--header "Name=Value"]
```

The CLI must also provide discoverable help:

```text
api --help
api help
api request --help
api profile --help
api secret --help
```

## Vocabulary

The durable vocabulary lives in [CONTEXT.md](../../CONTEXT.md). Key terms:

- API Service: an external business API system.
- API Profile: a named local configuration for one API service.
- Profile Catalog: the non-secret local collection of API profiles.
- Credential: a confidential keyring value used by a profile.
- Auth Scheme: the non-secret rule for applying credentials to a request.
- API Request: a profile-scoped HTTP request built from non-secret agent inputs.
- Mutation: a `POST`, `PUT`, `PATCH`, or `DELETE` API request.
- Response Envelope: the JSON output object emitted for request results.
- Redaction: masking known confidential values before output.

## Naming

- Binary: `api`
- Repository: `api-cli`
- Go module: `github.com/jeromelaurens/api-cli`
- Command package: `cmd/api`
- Main internal package: `internal/runner`
- Keyring service: `api-cli`
- Default profile catalog: `~/.api/profiles.json`

## Profile Catalog

The profile catalog is JSON and contains no secrets. The default path is derived
with `os.UserHomeDir()` and `filepath.Join(home, ".api", "profiles.json")` on
all operating systems. The same path shape applies on Windows under the user
profile directory.

Override priority:

1. `--profiles <path>`
2. `API_PROFILES`
3. default path

The CLI can create profiles in v1. It does not edit or delete profiles.

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

Validation:

- `schemaVersion` is required and must equal `1`.
- Profile names and credential names must match `[a-zA-Z0-9._-]+`.
- `service` is required.
- `baseURL` must be absolute and use `http` or `https`.
- `readOnly` is optional and defaults to `false`.
- `auth.scheme` is required and must be one of the v1 schemes.

## Credential Storage

Credentials are stored with `github.com/zalando/go-keyring`.

- Keyring service: `api-cli`
- Keyring key format: `<service>/<credential>`
- Example: `dolibarr/api-key`
- Profiles of the same service share one entry per credential, so a credential
  used by several profiles (one per site of the same account) is stored once.

Credential commands:

```text
api secret set <profile> <credential> [--json]
api secret list [<profile>] [--check] [--json]
api secret delete <profile> <credential> [--json]
```

There is no `secret get` in v1. `secret set` never accepts a value as an
argument. It reads masked input from a TTY or reads from stdin when stdin is not
a TTY. `secret list` reads the profile catalog by default and accesses the
keyring only with `--check`.

`secret set` and `secret delete` validate the profile and credential name
against the profile catalog to avoid orphaned keyring entries from typos, then
act on the keyring entry of the profile's service.

## Auth Schemes

V1 supports four auth schemes:

- `bearer`: sets `Authorization: Bearer <credential>`.
- `header`: sets a configured custom header such as `DOLAPIKEY`.
- `query`: adds a configured query parameter such as `api_token`.
- `basic`: builds HTTP Basic auth from separate username and password
  credentials.

For query auth, if the agent-provided query already contains the auth parameter,
the request is rejected with `auth_query_collision`.

For Basic auth, `api` redacts the username credential, password credential,
`base64(username:password)`, and `Basic <base64>`.

## Request Policy

Allowed methods:

```text
GET POST PUT PATCH DELETE HEAD OPTIONS
```

Methods are normalized to uppercase. Other methods are rejected.

The path must be non-empty, start with `/`, and not contain `?`. Absolute URLs
and network-path references are rejected.

`--query` is passed as a raw query string and must not start with `?`. It is not
otherwise normalized, except for auth query collision checks.

`--body <path>` and `--body -` read bytes. There is no inline body argument.
Bodies are not required to be JSON. If the body is JSON parseable and no
`Content-Type` is supplied, `api` sets `Content-Type: application/json`.

Headers supplied with `--header "Name=Value"` are limited to non-secret request
headers. The CLI rejects auth headers, cookies, `User-Agent`, and the header
managed by the profile auth scheme. Header names must be valid and header values
must not contain newlines.

Default headers:

- `Accept: application/json, */*;q=0.1`
- `User-Agent: api-cli/0.1`

`Content-Type` and `Accept` may be overridden. `User-Agent` may not.

Default request limits:

- Timeout: `30s`
- Max response body: `5 MiB`

Per-request flags:

```text
--timeout 10s
--max-body-bytes 1048576
```

## Mutation Guard

An API profile may set `readOnly: true`. A read-only profile rejects `POST`,
`PUT`, `PATCH`, and `DELETE` with `profile_read_only`. The default is
`readOnly: false` for ease of use. There is no `--confirm`, endpoint allowlist,
or generic dry-run in v1.

This is intentionally method-based. If an API uses `POST` for a read-only search
endpoint, that endpoint requires a non-read-only profile in v1.

## Output Policy

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

Help output is the intentional exception. `api --help`, `api help`, and
subcommand `--help` emit human-readable text on stdout and exit `0`. The text
must be structured enough for agents to discover command syntax, config paths,
response envelope shape, auth schemes, forbidden headers, and examples without
secrets. There is no `--help-json` in v1.

`api request` emits a response envelope for every HTTP response. HTTP `2xx`
sets `ok: true` and exits `0`. HTTP non-`2xx` sets `ok: false`, includes the
same response fields, and exits non-zero.

For `api request`, CLI, validation, config, keyring, network, timeout, and
size-limit errors emit:

```json
{
  "ok": false,
  "error": {
    "code": "credential_missing",
    "message": "credential api-key is missing for profile dolibarr"
  }
}
```

Admin commands use that same error shape only in `--json` mode.

Successful admin commands in `--json` mode do not include `ok`.

Response body fields:

- JSON parseable body: `body`
- non-JSON UTF-8 body: `bodyText`
- non-UTF-8 body: `bodyBase64` plus `bodyEncoding: "base64"`
- empty body: no body field

Response headers are allowlisted. `contentType` is always surfaced separately
when present.

Allowed response headers:

- `request-id`
- `x-request-id`
- `x-correlation-id`
- `x-ratelimit-limit`
- `x-ratelimit-remaining`
- `x-ratelimit-reset`
- `retry-after`

## Admin Commands

`api profile path` succeeds even if the catalog is missing. Default output is
human-readable text; `--json` emits:

```json
{"path":"/Users/example/.api/profiles.json","exists":false}
```

`api profile list` succeeds with an empty list when the catalog is missing.
Default output is human-readable text; `--json` emits:

```json
{"profiles":[],"catalogExists":false}
```

`api profile show <profile>` returns `profile_not_found` if the catalog or
profile is missing. By default that error is text on stderr; with `--json`, it
is JSON on stdout. In text mode, `profile show` prints all non-secret auth
configuration fields needed to understand which credential names are expected
(`header`, `param`, `credential`, `usernameCredential`,
`passwordCredential`). It never reads or prints keyring credential values.

`api profile create <profile>` creates the profile catalog if missing and writes
non-secret profile data only. It creates the catalog directory with owner-only
permissions where the operating system supports them, writes the catalog file as
owner-only read/write, and rejects duplicate profiles before prompting for any
missing interactive field.

Profile create is generic. It accepts non-secret profile fields as flags:

```text
api profile create <profile> \
  --service <service> \
  --base-url <url> \
  --auth bearer|header|query|basic \
  [auth flags]
```

Auth-specific flags:

- `bearer`: `--credential <name>`
- `header`: `--header <name> --credential <name>`
- `query`: `--param <name> --credential <name>`
- `basic`: `--username-credential <name> --password-credential <name>`

Missing non-secret fields are prompted interactively. The service name defaults
to the profile name. `profile create` never prompts for credential values.

`api profile update <profile>` updates an existing profile's non-secret fields
interactively. It presents each current value in brackets and keeps that value
when the user presses Enter. It never prompts for credential values. If the auth
scheme or credential names change, it reports the current `api secret set`
commands needed for the updated profile. `api profile modify <profile>` is
accepted as an alias, but `update` is the canonical command name used in help
and documentation.

`api secret list [<profile>]` requires the catalog. Without `--check`, it does
not read the keyring. With `--check`, it adds `present` in `--json` mode or a
human-readable presence marker in default text mode.

## Error Codes

V1 structured error codes:

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

Exit codes:

- `0`: HTTP `2xx` or successful admin command
- `1`: all structured failures, including HTTP non-`2xx`

## Agent Compatibility

Skills call `api` with profile names and relative paths only. They must not
pass credentials, absolute URLs, auth headers, cookies, or auth query
parameters.

Skills parse stdout JSON and inspect:

- `ok`
- `status`
- `body`
- `bodyText`
- `bodyBase64`
- `error.code`

If a skill needs to inspect configuration through an admin command, it must pass
`--json`. Human-readable admin output is the default for operators, not for
agents.

Endpoint knowledge stays in skills. The CLI does not know Dolibarr,
Pennylane, Pipedrive, Crisp, MailerLite, or Sentry endpoint semantics.

## Documentation

- README: user-facing contract.
- `CONTEXT.md`: durable vocabulary only.
- `docs/adr/0001-local-cli-with-os-keyring.md`: local CLI and keyring decision.
- `docs/profiles/examples.json`: placeholder profile catalog for target
  services.

Public API base URLs in examples should be checked against official service
documentation before final publication.

## Test Scope

Required v1 tests:

- global and subcommand help output
- default human-readable admin output and admin `--json` output
- interactive and flag-driven profile creation
- profile catalog parsing and validation
- default and override profile catalog path resolution
- path and query rejection
- auth application for bearer, header, query, and basic
- auth query collision rejection
- forbidden request header rejection
- read-only mutation blocking
- body file/stdin handling and content type inference
- response envelope for JSON, text, binary, empty, and HTTP error bodies
- timeout and max body behavior
- keyring store with `go-keyring.MockInit()`
- memory credential store for command tests
- CLI admin commands in-process through injected dependencies
- binary e2e against a fake HTTP server using the test-only
  `apicli_e2e_mock_keyring` build tag, without touching the OS keyring
- non-leak tests for stdout and stderr

## Deferred

- MCP server
- third-party broker integrations
- OpenAPI-driven clients
- endpoint allowlists
- generic dry-run
- `secret get`
- profile edit/delete commands
- public override of the credential store
- response body truncation
