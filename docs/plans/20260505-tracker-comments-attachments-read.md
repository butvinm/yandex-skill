# Tracker Comments + Attachments (Read)

## Overview

Extend the `tracker` command tree with read-only access to issue comments and attachments. Today `tracker issues get` returns only the issue's own fields (`internal/tracker/issues.go:19-26`); there is no way to fetch the comment thread or attached files. This plan adds:

- `tracker comments list <issue-key>` — list comments on an issue, including each comment's attachment refs
- `tracker attachments list <issue-key>` — list attachment metadata for an issue (covers both issue-level and comment-level attachments)
- `tracker attachments download <issue-key> <id> [--out <path>]` — stream the binary content of an attachment

Scope is read-only. No comment posting, no attachment uploads, no edits — consistent with the existing "no Tracker writes" rule in `CLAUDE.md`. This plan formally expands that rule to cover _reading_ comments and attachments only.

The shape mirrors the wiki side: `wiki attachments list/download` already exists in `internal/wiki/attachments.go`, so adding `tracker attachments list/download` follows an established pattern rather than inventing a new one.

**Comment attachments — important Tracker API behavior (verified against `yandex.ru/support/tracker/en/concepts/issues/get-attachments-list` on 2026-05-05):**

- `GET /v3/issues/{key}/attachments` returns _both_ issue-level attachments **and** comment attachments. Direct quote from the docs: _"Use this request to get a list of files attached to an issue and to comments below it."_ So `tracker attachments list` and `tracker attachments download` cover comment attachments without any additional command.
- There is **no** dedicated `/comments/{id}/attachments` endpoint. The Tracker API does not separately enumerate comment attachments.
- Comments expose an `attachments` field (`[{self, id, display}]`) **only when the request includes `?expand=attachments`**. Without that param, a comment listing silently omits attachment refs, leaving the user unable to associate files with comments.
- The download URL for any attachment (issue-scoped or comment-scoped) is the `content` field returned by `/attachments` listing, or the standard `/v3/issues/{key}/attachments/{id}/{filename}` path. Same shape either way.

## Context (from discovery)

**Files involved:**

- New: `internal/tracker/comments.go`, `internal/tracker/comments_test.go`
- New: `internal/tracker/attachments.go`, `internal/tracker/attachments_test.go`
- Modify: `internal/tracker/client.go` — add `DoRaw` for binary streaming (mirrors `wiki.Client.DoRaw`)
- Modify: `internal/cli/cli.go` — register `TrackerCommentsCmd` + `TrackerAttachmentsCmd` under `TrackerCmd`; new `*Cmd` structs with `Run` methods
- Modify: `internal/cli/e2e_test.go` — add e2e tests covering plain + JSON for each new command
- Modify: `README.md` — document the new commands and update the "what this CLI doesn't do" section

**Patterns to follow:**

- `Client.Do` JSON pattern in `internal/tracker/client.go` (marshal body, decode into `out`, return `*http.Response`)
- `Client.DoPaginated` for list endpoints — Tracker uses Link header `rel=next`
- `render.Plainer` (single-record) and `render.Rower` (list-row) interfaces from `internal/render/render.go`
- `auth.Load() → New(cfg) → method → render.One/Many` pipeline in every Run method
- Wiki attachment download guard pattern in `internal/wiki/attachments.go:125-138` (resolve, check, stream)

**Yandex Tracker REST endpoints (v3, verified against docs on 2026-05-05):**

- `GET /v3/issues/{key}/comments?expand=attachments` → `[]Comment`. Pagination via Link `rel=next` (same as `/v3/issues/_search`). The `expand=attachments` query param is required to populate the `attachments` field on each comment; without it, comment-attachment refs are silently omitted.
- `GET /v3/issues/{key}/attachments` → `[]Attachment`. Same pagination. Returns both issue-level _and_ comment-level attachments in one flat list — confirmed by docs.
- `GET /v3/issues/{key}/attachments/{id}/{filename}` → binary stream. Filename is required by the API; we resolve it from the listing. Works for any attachment id regardless of whether it was attached to the issue or to a comment.

## Development Approach

- **Testing approach:** Regular (code first, then tests in the same task) — matches the existing per-package unit-test convention in `internal/tracker/issues_test.go` and `internal/tracker/queues_test.go`.
- Complete each task fully (code + tests passing) before starting the next.
- Every task that touches code MUST add or update tests in the same task. Tests are required, not optional.
- All tests must pass (`go test ./...`) before moving to the next task.
- `go vet ./...` must pass before each commit.
- Update this plan file when scope changes.
- Maintain backward compatibility — these are additive commands; no existing CLI surface changes.

## Testing Strategy

- **Unit tests** (per-package, next to source): client method behavior, request construction, response decoding, error paths. Use `httptest.NewServer` per the existing `internal/tracker/issues_test.go` pattern.
- **E2E tests** (`internal/cli/e2e_test.go`): assert exact plain output and JSON output for each new command. Plain output is the LLM-facing contract — pin it carefully.
- **No live API calls.** Every test stands up an `httptest.NewServer` and points the client at it via `YANDEX_TRACKER_BASE_URL`.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with `+` prefix (`+ [ ] ...`).
- Document blockers with `!` prefix (`! [ ] ...` plus a short note).
- Keep this plan file in sync with actual work — if scope changes, edit it.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): everything achievable in this codebase — Go code, tests, README updates, plan housekeeping.
- **Post-Completion** (no checkboxes): things outside this repo — manual verification against a real Tracker instance, plugin/skill changes (the `plugins/yandex/` skill may want to learn about the new commands but that's a separate task), version bump for `go install`.

## Implementation Steps

### Task 1: Add `DoRaw` to tracker.Client for binary streaming

**Files:**

- Modify: `internal/tracker/client.go`
- Modify: `internal/tracker/client_test.go`

- [x] Add `DoRaw(ctx, method, url, contentType string, body io.Reader) (*http.Response, error)` returning the open response so callers can stream `resp.Body`. Caller closes the body. Signature matches `wiki.Client.DoRaw` exactly.
- [x] On HTTP ≥400, drain and close the body and return `*APIError` (consistent with `Do`).
- [x] Write unit test: 200 response — assert body is readable and not pre-closed.
- [x] Write unit test: 404 response — assert `*APIError` with `Status: 404`, response returned.
- [x] Run `go test ./internal/tracker/... && go vet ./...` — passed.

### Task 2: Comments list — types + client method + tests

**Files:**

- Create: `internal/tracker/comments.go`
- Create: `internal/tracker/comments_test.go`

- [x] Define `CommentAttachmentRef` struct: `ID string`, `Display string`. JSON tags `id`, `display`. (`self` URL field is ignored.)
- [x] Define `Comment` struct: `ID int64`, `LongID string`, `Text string`, `CreatedBy Display`, `CreatedAt string`, `UpdatedAt string`, `Attachments []CommentAttachmentRef`.
- [x] Implement `Plain() string`: header `<author>  <createdAt>`, text body, then `attachments: <id>:<display>, ...` line if any.
- [x] Implement `Row() string`: `<author>  <createdAt>  <first-line-of-text>  [N attached]` (suffix omitted when no attachments). Multi-line text collapses via `firstLine` helper.
- [x] Implement `(c *Client) ListComments(ctx, issueKey string) ([]Comment, error)` against `/v3/issues/{key}/comments?expand=attachments` using `DoPaginated`.
- [x] Test: with-attachments — `Plain()` and `Row()` strings exact.
- [x] Test: no-attachments — no `attachments:` line in Plain, no `[N attached]` suffix in Row.
- [x] Test: outgoing query contains `expand=attachments` (asserts on `r.URL.RawQuery`).
- [x] Test: paginated response via Link header — asserts merge order.
- [x] Test: 404 — assert `*APIError`.
- [x] Run `go test ./internal/tracker/...` — passed.

### Task 3: Attachments list — types + client method + tests

**Files:**

- Create: `internal/tracker/attachments.go`
- Create: `internal/tracker/attachments_test.go`

- [x] Define `Attachment` struct (id, name, size, mimetype, content, createdBy, createdAt).
- [x] Implement `Plain()` / `Row()` via `render.SkipEmpty(ID, Name, Mimetype, humanSize(Size), CreatedAt, Content)`.
- [x] Implement `humanSize` helper (B/KiB/MiB/GiB; empty for ≤0).
- [x] Implement `ListAttachments` against `/v3/issues/{key}/attachments` via `DoPaginated`.
- [x] Test: single-page decode + Plain() string exact.
- [x] Test: humanSize boundary cases.
- [x] Test: paginated via Link header.
- [x] Test: 404.
- [x] Run `go test ./internal/tracker/...` — passed.

### Task 4: Attachment download — client method + tests

**Files:**

- Modify: `internal/tracker/attachments.go`
- Modify: `internal/tracker/attachments_test.go`

- [ ] Implement `(c *Client) DownloadAttachment(ctx context.Context, issueKey, id string, w io.Writer) error`:
  - First call `ListAttachments` to resolve `id → name` (Tracker requires the filename in the URL path). If id not found, return `*APIError{Status: 404, Message: "attachment not found"}`.
  - Then `DoRaw(GET, "/v3/issues/{key}/attachments/{id}/{name}", nil)`, copy body to `w`, close body.
- [ ] Write unit test: happy path — fixture server serves a known byte payload; assert exact bytes written to a `bytes.Buffer`.
- [ ] Write unit test: id-not-found in listing — assert error and that no second HTTP call is made (use a counter on the fixture handler).
- [ ] Write unit test: 404 on download endpoint — assert error propagates.
- [ ] Run `go test ./internal/tracker/...` — must pass before Task 5.

### Task 5: Wire CLI commands and e2e tests

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

- [ ] Add `Comments TrackerCommentsCmd \`cmd:""\``and`Attachments TrackerAttachmentsCmd \`cmd:""\``fields to`TrackerCmd`.
- [ ] Define `TrackerCommentsCmd` with subcommand `List ListTrackerCommentsCmd \`cmd:""\``. `ListTrackerCommentsCmd { Key string \`arg:"" help:"Issue key, e.g. PROJ-1"\` }`with a`Run(g Globals) error`that follows the standard pipeline and calls`render.Many`.
- [ ] Define `TrackerAttachmentsCmd` with subcommands `List` and `Download`. `Download` takes positional `Key` and `ID`, optional `--out <path>` (default stdout). Follow wiki download's stdout-vs-file split.
- [ ] Add e2e test: `tracker comments list FOO-1` plain output — pin exact string.
- [ ] Add e2e test: `tracker comments list FOO-1 --json` — assert JSON decodes back to the expected slice.
- [ ] Add e2e test: `tracker attachments list FOO-1` plain + JSON — same pattern.
- [ ] Add e2e test: `tracker attachments download FOO-1 <id> --out <tmpfile>` — assert file contents match the fixture bytes.
- [ ] Add e2e test: `tracker attachments download FOO-1 <bad-id>` — assert non-zero exit, error printed to stderr.
- [ ] Run `go test ./... && go vet ./...` — must pass before Task 6.

### Task 6: Verify acceptance criteria

- [ ] `tracker comments list <key>` returns comments in plain and JSON.
- [ ] `tracker attachments list <key>` returns attachments in plain and JSON, download URL is the trailing column.
- [ ] `tracker attachments download <key> <id>` writes bytes to stdout by default and to a file with `--out`.
- [ ] All error paths (issue not found, attachment id not found, server error) produce a single error line on stderr (or `{"error":...,"status":...}` with `--json`) and a non-zero exit code.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] Build a binary (`go build -o /tmp/yc ./cmd/yandex-cli`) and run `--help` against it; verify the new commands show up in the tree.

### Task 7: Documentation + plan housekeeping

**Files:**

- Modify: `README.md`
- Modify: `CLAUDE.md` (if patterns shifted)
- Move: this file to `docs/plans/completed/`

- [ ] Add a "Comments" and "Attachments" subsection under the Tracker section of `README.md` with a one-line description and one example invocation per new command.
- [ ] Update the "Limitations" section: keep "No Tracker writes" but remove the implicit "no comments/attachments" — it was never written explicitly, but if any phrasing implied it, soften.
- [ ] If `CLAUDE.md` `## Scope` mentions tracker write-only restriction, leave it; this plan only adds reads.
- [ ] If a new pattern emerged (e.g. `DoRaw` on tracker, attachment id→name resolution), add a one-line note under `## Conventions` in `CLAUDE.md`.
- [ ] `mkdir -p docs/plans/completed && git mv docs/plans/20260505-tracker-comments-attachments-read.md docs/plans/completed/`.

## Technical Details

**Comment JSON shape (with `expand=attachments`, per docs):**

```json
{
  "id": 12345,
  "longId": "5fa1b2c3d4e5f6...",
  "text": "See logs.",
  "createdBy": { "display": "Иван Иванов" },
  "createdAt": "2026-05-04T10:11:12.000+0000",
  "updatedAt": "2026-05-04T10:11:12.000+0000",
  "attachments": [
    {
      "self": "https://api.tracker.yandex.net/v3/issues/PROJ-1/attachments/67890",
      "id": "67890",
      "display": "screenshot.png"
    }
  ]
}
```

Comments without attachments simply omit the field (or return `[]`). Without `?expand=attachments`, the field is absent even when files are attached — that's the trap the `expand` param prevents.

**Attachment JSON shape (per docs):**

```json
{
  "id": "67890",
  "name": "screenshot.png",
  "size": 14823,
  "mimetype": "image/png",
  "content": "https://api.tracker.yandex.net/v3/issues/PROJ-1/attachments/67890/screenshot.png",
  "createdBy": { "display": "Иван Иванов" },
  "createdAt": "2026-05-04T10:11:12.000+0000"
}
```

The `/attachments` listing returns issue-level and comment-level files together — there is no flag, type field, or back-reference to the originating comment. If a user wants "which file belongs to which comment?", they cross-reference the comment's `attachments[].id` (from `comments list`) against the issue-level listing's `id`. That's why `Attachment.Plain()` leads with the id.

**Plain output examples:**

```
$ yandex-cli tracker comments list PROJ-1
Иван Иванов  2026-05-04T10:11:12.000+0000
See logs.
attachments: 67890:screenshot.png

Петр Петров  2026-05-04T11:00:00.000+0000
Merging.
```

```
$ yandex-cli tracker attachments list PROJ-1
67890  screenshot.png  image/png  14.5 KiB  2026-05-04T10:11:12.000+0000  https://api.tracker.yandex.net/v3/issues/PROJ-1/attachments/67890/screenshot.png
67891  log.txt  text/plain  2.1 KiB  2026-05-04T10:30:00.000+0000  https://api.tracker.yandex.net/v3/issues/PROJ-1/attachments/67891/log.txt
```

```
$ yandex-cli tracker attachments download PROJ-1 67890 --out ./screenshot.png
download: PROJ-1/67890 → ./screenshot.png
```

**Error contract:** unchanged — `render.Error` formats. Plain prints to stderr, JSON wraps as `{"error":"...","status":<http>}`.

## Post-Completion

_Items outside this repo or requiring manual verification._

**Manual verification:**

- Run each command against a real Yandex Tracker instance (Cloud-org tenancy and 360 tenancy, separately) to confirm the v3 paths and JSON shapes match the assumed structure. Adjust if the real API differs.
- Verify download works for a non-trivial binary (e.g. an image > 1 MiB) — confirm no truncation.

**External system updates:**

- The `plugins/yandex/` Claude Code skill may want to be taught about the new commands so the LLM can use them. Out of scope for this plan; track separately.
- After release, users with `go install …@latest` will pick up the new commands automatically. No version-pin coordination needed unless a downstream project pins a specific version.
