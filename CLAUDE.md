# CLAUDE.md

Guidance for Claude Code working in this repo. Keep it terse — the README covers user-facing docs.

## What this is

A Go CLI (`yandex-cli`) that wraps Yandex Tracker (read) and Yandex Wiki (read+write) for LLM consumption, plus a Claude Code plugin/skill (`plugins/yandex/`) that invokes it.

Naming gotcha: the module is `github.com/butvinm/yandex-skill`, the binary is `yandex-cli`, the plugin is `yandex`, the marketplace entry is `butvinm-yandex-skill`. All four are intentional — don't "fix" mismatches.

## Build, test, run

```sh
go build -o bin/yandex-cli ./cmd/yandex-cli
go install ./cmd/yandex-cli       # to $GOBIN
go test ./...
go vet ./...
```

Version resolution (in `cmd/yandex-cli/main.go`):

1. If `main.version` is set via ldflags, use that.
2. Else, if installed via `go install pkg@version`, fall back to `runtime/debug.ReadBuildInfo().Main.Version` (gives the real module version for `@latest` users).
3. Else, `"dev"` (e.g. local `go build` from a fresh worktree).

To force-inject a custom build identifier (commit hash, date, etc.):

```sh
go install -ldflags "-X main.version=$(git describe --tags --always)" ./cmd/yandex-cli
```

## Layout

- `cmd/yandex-cli/main.go` — thin entry, calls `cli.Main(version)`
- `internal/cli/` — kong CLI definitions, command Run methods, e2e tests
- `internal/auth/` — env-var config, tenancy detection, header builders
- `internal/tracker/` — Tracker REST client + types (issues, queues)
- `internal/wiki/` — Wiki REST client + types (pages)
- `internal/render/` — Plain/JSON output, `Plainer`/`Rower` interfaces
- `plugins/yandex/` — Claude Code plugin manifest and skill files

## Conventions

**Output rendering.** Every type returned to the user implements `render.Plainer` (single-record `Plain() string`) and/or `render.Rower` (list-row `Row() string`). Commands call `render.One`, `render.Many`, or `render.Confirm` — never write to stdout directly. JSON output is the same struct serialized; plain output is hand-formatted for LLM/shell consumption.

Use `render.SkipEmpty` (two-space separator) for inline fields and `render.SkipEmptyLines` for stacked fields. Don't introduce new separators without a reason.

**Errors.** Errors bubble up through `Run` and get formatted by `render.Error`. With `--json` they become `{"error":"...","status":<http>}`. Don't print errors mid-command.

**Auth.** `auth.Load()` reads env vars and returns a `Config`. Tenancy is implicit from which org-id var is set (`YANDEX_CLI_CLOUD_ORG_ID` → Cloud Bearer + `X-Cloud-Org-ID`; `YANDEX_CLI_ORG_ID` → 360 OAuth + `X-Org-ID`). Setting both is rejected. Don't add a tenancy flag — the env-var-presence dispatch is the contract.

**Commands.** Every command Run method follows the same shape: `auth.Load()` → instantiate client → call client method → `render.One/Many/Confirm`. Match this pattern when adding commands; don't add caching, retries, or progress output.

**CLI parser.** `kong` (alecthomas/kong). Subcommands are nested structs with `cmd:""` tags. The global `--json` is on the root struct; `Globals` carries it plus stdout/stderr/stdin/ctx via `kong.Bind`.

## Testing

Per-package unit tests sit next to source files. End-to-end tests live in `internal/cli/e2e_test.go` and use `httptest.NewServer` with `YANDEX_CLI_TRACKER_BASE_URL` / `YANDEX_CLI_WIKI_BASE_URL` env vars to point the client at the test server. Token/org-id env vars are set per-test via `t.Setenv`.

When adding a command, add: (1) a unit test for the client method, (2) an e2e test that asserts both plain and JSON output. Plain-output assertions are the only place we pin the exact LLM-facing format — break those carefully.

## Scope

Limitations are stated in the README. Don't silently expand:

- No Tracker writes (no comments, transitions, edits)
- No Wiki attachments / image uploads
- No pagination flags (clients fetch all pages internally via Link `rel=next`)
- Wiki page list is `--parent`-only (no free-text search; the API doesn't expose one)

If a task requires breaking one of these, surface it as a scope question before implementing.

## Things not to do

- Don't `git add .` — stage specific files. Atomic commits.
- Don't add a Makefile back unless asked; the bare `go` commands are the contract.
- Don't add comments that restate code. Explain _why_ only when non-obvious.
- Don't broaden the auth model (no token files, no keyring) without an explicit ask.
