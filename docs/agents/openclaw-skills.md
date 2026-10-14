# Using api-cli from OpenClaw skills

How OpenClaw agent skills call HTTP APIs through `api-cli` (the `api` command)
instead of `curl`, and how the shared usage contract is factored out so it is not
duplicated in every skill.

Audience: anyone writing or migrating an OpenClaw `SKILL.md` that talks to an HTTP
API. For the full output envelope, error codes, and auth schemes, see the project
[`README.md`](../../README.md) (*Output Contract*, *Authentication*, *Agent Usage*).

## Why not curl

With `curl`, each skill carries the secret-handling boilerplate: load a token from
an env var (or read `secrets.json` directly), pass it as an auth header / query
param, and repeat the "never echo the secret" rules. The token transits through
the shell and risks landing in the model's context.

With `api-cli` the agent supplies only **non-secret** inputs — a profile name, an
HTTP method, a relative path, an optional query, body, and non-secret headers. The
CLI loads credentials from the OS keyring, applies the profile's auth, and emits a
redacted JSON envelope. The secret never appears in a command, an env var, or the
model's context, so the per-skill secret section disappears entirely.

## The shared contract lives in TOOLS.md — once

OpenClaw has **no cross-skill composition**: a `SKILL.md` cannot load or invoke
another skill, and `{baseDir}` only references files inside the skill's own folder.
So a single "api-cli skill" that the others reference is not reliable.

Instead, the invariant usage contract lives once in the workspace
**`TOOLS.md`**, which OpenClaw injects into the system prompt at the start of every
session ("notes about your local tools and conventions; guidance only"). Each
skill keeps only its domain knowledge and points back to `TOOLS.md`.

Keep the block short — bootstrap files share a budget (`bootstrapMaxChars` 20k per
file, `bootstrapTotalMaxChars` 60k total). Push verbose detail to a per-skill
`references/*.md` loaded on demand via `{baseDir}` only when a skill needs it.

### TOOLS.md contract block (copy into the workspace TOOLS.md)

````markdown
## api-cli — authenticated HTTP calls

Skills that talk to an HTTP API use the `api` command (api-cli), **not** `curl`.
The agent supplies only non-secret inputs; api-cli loads credentials from the OS
keyring, applies the profile's auth, and returns a redacted JSON envelope. Secrets
never appear in a command, an env var, or the model's context.

**Call shape**

```
api request <profile> <METHOD> <path> [--query "k=v&k2=v2"] [--body file|-] [--header "Name=Value"]
```

- `<path>` is **relative** — the base URL lives in the profile; absolute URLs are rejected.
- Never pass a credential, an auth header, or an auth query param — the profile carries auth.
- `Content-Type` and `Accept` may be set with `--header`; `User-Agent` may not.
- Request body: stdin (`--body -`) or a temp file (`--body /tmp/x.json`).

**Output (stdout JSON):** inspect `ok`, `status`, then `body` / `bodyText` / `bodyBase64`.
When piping to `jq`, the parsed response lives under `.body` (e.g. `.body.data[0].id`).

**When `ok` is false / profile or secret missing** — configure once (operator step), then retry:

```
api profile create <profile> --service <name> --base-url <url> --auth <scheme> …
api secret set <profile> <credential>
```

Diagnose with `api profile list` and `api secret list --check`.
````

## Anatomy of a migrated SKILL.md

**Frontmatter** — gate on the binary, drop the env injection:

```yaml
metadata:
  openclaw:
    requires:
      bins: ["api"]
```

- Remove `requires.env` and `primaryEnv` (the secret is no longer an env var).
- Replace any `requires.bins: ["curl"]` with `["api"]`.

**Body**

- Delete the "secret handling" section and any token loading
  (`$XXX_TOKEN`, `jq … secrets.json`, base64 of credentials).
- Keep the domain knowledge: endpoints, IDs, business tables, conventions.
- Replace every `curl …` with `api request <profile> …`.
- Add one line: *"api-cli profile: `<name>` — call contract, output envelope and
  `ok:false` handling: see `TOOLS.md`."*

## Migration recipe (curl → api request)

| curl pattern | api-cli equivalent |
|---|---|
| Base URL prefix (`$BASE/v1/foo`) | drop it — base URL lives in the profile; pass the **relative** path `/foo` |
| Header auth `-H "DOLAPIKEY: $T"` | drop it — profile auth `header` carries it |
| Bearer `-H "Authorization: Bearer $T"` | drop it — profile auth `bearer` |
| Query auth `?api_token=$T` | drop it — profile auth `query`; **never** put it in `--query` |
| Basic `base64(id:key)` + `-H Authorization` | drop it — profile auth `basic`; keep **non-auth** headers via `--header`, or in the profile when fixed (e.g. `X-Crisp-Tier=plugin` as a profile default header) |
| Query string `?a=b&c=d` | `--query "a=b&c=d"` (api-cli encodes; no curl `-g`/globoff needed for `filter[...]`) |
| Inline body `-d '{...}'` | `printf '%s' '{...}' \| … --body -` |
| File body `-d @file.json` | `--body /tmp/file.json` |
| `... \| jq '.data'` | `... \| jq '.body.data'` (response sits under `.body`) |

Forbidden from the agent side: credentials, absolute URLs, auth headers, auth
query params, and `User-Agent`. `Content-Type` / `Accept` are allowed.

## Profiles (operator setup, not skill content)

A migrated skill only works once its profile and keyring credential exist. This is
an operator step, done once per machine — see `scripts/configure-real-profiles.sh`
and the *Real-Service Smoke Tests* section of the README.

| Profile | Auth scheme | Credential(s) | Base URL |
|---|---|---|---|
| dolibarr | `header` (`DOLAPIKEY`) | `api-key` | `https://…/api/index.php` |
| mailerlite | `bearer` | `api-token` | `https://connect.mailerlite.com/api` |
| pipedrive | `query` (`api_token`) | `api-token` | `https://api.pipedrive.com/v1` |
| pennylane | `bearer` | `access-token` | `https://app.pennylane.com/api/external/v2` |
| crisp | `basic` | `identifier` + `key` | `https://api.crisp.chat/v1` |

```sh
api secret set mailerlite api-token   # prompts for the value; never on the CLI line
```

## Gating and validation

`requires.bins: ["api"]` is a **PATH presence check only** — it confirms the binary
exists, nothing more. It does not (and cannot) verify that the profile or the
keyring secret exist.

- **Skill gating:** `openclaw skills check` reports which skills are ready /
  visible / missing requirements. After migration, each API skill should be
  "Ready and visible to model" with 0 missing requirements.
- **Runtime readiness:** profile/secret presence only surfaces at call time. Probe
  it ahead of use with `api profile list` and `api secret list --check`, or handle
  the `ok:false` envelope when a call fails.

## Handling `ok:false`

`api request` always emits the same envelope shape; on failure it sets
`ok:false` and adds an `error` object (`error.code`, `error.message`). A skill
should branch on `ok`, and for setup-related codes (`profile_not_found`,
`credential_missing`, `keyring_unavailable`) tell the operator to run the
`api profile create …` / `api secret set …` step above rather than retrying.
See the README *Error Codes* list for the full set.
