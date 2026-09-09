# CLAUDE.md

Guidance for Claude Code working in this repo. Keep it terse — the README covers user-facing docs.

## What this is

A Go CLI (`yandex-cli`) that wraps Yandex Tracker (read) and Yandex Wiki (read+write) for LLM consumption, plus a Claude Code plugin/skill (`plugins/yandex/`) that invokes it.

Naming gotcha: the module is `github.com/butvinm/yandex-skill`, the binary is `yandex-cli`, the plugin is `yandex`, the marketplace entry is `butvinm-yandex-skill`. All four are intentional — don't "fix" mismatches.

Versioning: the project follows semver, and one version drives two surfaces. The `version` field in `plugins/yandex/.claude-plugin/plugin.json` is the plugin version (Claude Code equality-gates plugin updates on it — a commit ships to installed users only when this string is bumped; SHA-based auto-update was dropped in 1.0.0). The matching `vX.Y.Z` git tag is the binary version (`go install @latest` picks it up via `runtime/debug.ReadBuildInfo`). Keep the two in lockstep.

Releasing: bump `version` in `plugin.json`, refresh the single line in the README "Recent updates" section, then after the release commit lands on `master` cut the tag and publish the notes: `git tag vX.Y.Z && git push origin vX.Y.Z && gh release create vX.Y.Z --notes "..."`. Per-version history lives in GitHub Releases, not an in-tree changelog (a plugin/`go install` consumer never sees repo files anyway); the README line is the only in-tree teaser. Bump rules:

- MAJOR: user must act after updating (a command/flag/env-var removed or renamed, output format broken, install method changed).
- MINOR: backward-compatible feature (new command, new flag, notable new capability).
- PATCH: backward-compatible fix (bugfix, doc-only or skill-only tweak).

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
- `internal/tracker/` — Tracker REST client + types (issues, queues, comments, attachments). `client.go` has `Do` (JSON), `DoRaw` (binary streams), and `DoPaginated` (Link `rel=next`). `attachments.go` covers list+download; comment attachments are not separately enumerable — the issue-level `/attachments` listing returns them already. `links.go` fetches `/issues/{key}/links` (the plain issue body never carries links, and `?expand=links` drops the linked issues' status/assignee); `GetIssue` attaches the raw array under the issue's `links` key. Which of `type.inward`/`type.outward` names the linked issue depends on `direction` - see `Link.Label` before touching that logic.
- `internal/wiki/` — Wiki REST client + types (pages, attachments). `client.go` has both `Do` (JSON) and `DoRaw` (binary streams). `attachments.go` exposes the public ops; `upload_sessions.go` is the private 3-step helper used only by `UploadAttachment`.
- `internal/render/` — Plain/JSON output, `Plainer`/`Rower` interfaces
- `plugins/yandex/` — Claude Code plugin manifest and skill files

## Conventions

**Output rendering.** Every type returned to the user implements `render.Plainer` (single-record `Plain() string`) and/or `render.Rower` (list-row `Row() string`). Commands call `render.One`, `render.Many`, or `render.Confirm` — never write to stdout directly. JSON output is the same struct serialized; plain output is hand-formatted for LLM/shell consumption.

Use `render.SkipEmpty` (two-space separator) for inline fields and `render.SkipEmptyLines` for stacked fields. Don't introduce new separators without a reason.

**Wiki page URLs.** `Page`/`PageRef` carry a `URL` field (the human-facing `https://wiki.yandex.ru/<slug>` link, built by `Client.PageURL` from `YANDEX_WIKI_PUBLIC_URL`, default `https://wiki.yandex.ru` — distinct from the `YANDEX_WIKI_BASE_URL` API host). The client populates it on returned pages, and it leads the plain-output row/block so callers cite full links instead of bare slugs. Symmetrically, `Client.canonSlug` normalizes every user-supplied slug (all page + attachment commands route through it) so a pasted full page URL is accepted wherever a slug is. The e2e plain-output assertions pin the default `https://wiki.yandex.ru` prefix — update them together with the default if it ever changes.

**Errors.** Errors bubble up through `Run` and get formatted by `render.Error`. With `--json` they become `{"error":"...","status":<http>}`. Don't print errors mid-command.

**Auth.** `auth.Load()` reads env vars and returns a `Config`. Tenancy is implicit from which org-id var is set (`YANDEX_CLOUD_ORG_ID` → Cloud Bearer + `X-Cloud-Org-ID`; `YANDEX_ORG_ID` → 360 OAuth + `X-Org-ID`). Setting both is rejected. Don't add a tenancy flag — the env-var-presence dispatch is the contract.

**Commands.** Every command Run method follows the same shape: `auth.Load()` → instantiate client → call client method → `render.One/Many/Confirm`. Match this pattern when adding commands; don't add caching, retries, or progress output.

**Markdown round-trip flag (`--attachments-dir`).** Opt-in flag on `wiki pages get/create/update`. Triggers attachment sync + URL rewriting. Rewrite scope is exactly the substring `/<current-page-slug>/.files/X` — ignores the surrounding markdown construct (image, file directive, legacy `0x0:` form). Cross-page references (`/<other-slug>/.files/X`) are intentionally untouched. Local on-disk filenames use `path.Base(download_url)`, not `Attachment.Name` — server URL basenames are unique by construction; Names are not. Page-type guard: refuse `grid` (structured table; would overwrite non-markdown data); warn on `page` and unknown; proceed on `wysiwyg`. The org-wide sweep test in `internal/wiki/sweep_test.go` (`//go:build sweep`) re-validates the safety of these contracts against live data.

**CLI parser.** `kong` (alecthomas/kong). Subcommands are nested structs with `cmd:""` tags. The global `--json` is on the root struct; `Globals` carries it plus stdout/stderr/stdin/ctx via `kong.Bind`.

## Testing

Per-package unit tests sit next to source files. End-to-end tests live in `internal/cli/e2e_test.go` and use `httptest.NewServer` with `YANDEX_TRACKER_BASE_URL` / `YANDEX_WIKI_BASE_URL` env vars to point the client at the test server. Token/org-id env vars are set per-test via `t.Setenv`.

When adding a command, add: (1) a unit test for the client method, (2) an e2e test that asserts both plain and JSON output. Plain-output assertions are the only place we pin the exact LLM-facing format — break those carefully.

## Scope

Limitations are stated in the README. Don't silently expand:

- No Tracker writes (no comment posting, no transitions, no edits). Reads cover issues, queues, comments (with attachment refs), and attachments (issue-level + comment-level, unified by the API).
- Wiki attachment uploads are single-part only (≤16 MiB); no chunked or resumable upload path
- No pagination flags (clients fetch all pages internally via Link `rel=next`). The one deliberate exception is `wiki search --limit` (1-50, single request): search is ranked, so walking the cursor to fetch everything would be wrong; the cursor itself stays unexposed.
- Wiki page list is `--parent`-only; free-text lookup is `wiki search` (`POST /v1/search`, `internal/wiki/search.go`)

If a task requires breaking one of these, surface it as a scope question before implementing.

**Wiki attachments invariant.** Uploads use the official 3-step Upload Sessions protocol:

1. `POST /v1/upload_sessions` `{file_name, file_size}` → `{session_id}`
2. `PUT /v1/upload_sessions/{id}/upload_part?part_number=1` with `Content-Type: application/octet-stream` (single part, ≤16 MiB)
3. `POST /v1/upload_sessions/{id}/finish`
4. `POST /v1/pages/{idx}/attachments` `{upload_sessions: [<id>]}` to bind the session to a page

Some third-party clients (e.g. `zolkinka/yandex-mcp`) document a `POST /v1/pages/{id}/files` multipart endpoint — that does **not** exist in the public Yandex Wiki spec. Don't reintroduce it.

Download by slug + filename uses `GET /v1/pages/attachments/download_by_url?url=<slug>/<filename>` so the CLI doesn't need to expose the numeric file id. Delete still needs the id and looks it up via the page-id list.

## Things not to do

- Don't `git add .` — stage specific files. Atomic commits.
- Don't add a Makefile back unless asked; the bare `go` commands are the contract.
- Don't add comments that restate code. Explain _why_ only when non-obvious.
- Don't broaden the auth model (no token files, no keyring) without an explicit ask. The one exception is the IAM token cache at `os.UserCacheDir()/yandex-cli/iam-token.json` (mode 0600), only populated when `YANDEX_YC_PATH` is set. Don't extend disk persistence to OAuth tokens, refresh tokens, or org-id without an explicit ask.
