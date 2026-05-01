# Wiki Attachments Support

## Overview

Add `yandex-cli wiki attachments` subcommands so the LLM can manage Yandex Wiki page assets (screenshots, PDFs, drafts) end-to-end: upload, list, download, delete.

This explicitly **breaks the existing scope rule** in `CLAUDE.md` and `README.md` ("No Wiki attachments / image uploads"). The branch `wiki-attachments` was created with this expansion in mind. Deliverable includes scrubbing the limitation from those docs.

**Why this is non-trivial:** the official Yandex Wiki upload contract is a 3-step _Upload Sessions_ protocol, not a single `multipart/form-data` POST. The existing `wiki.Client.Do()` helper is JSON-only and cannot carry binary bodies — we need a small low-level companion for `octet-stream` PUT/GET.

## Context (from discovery)

Files that grow or get touched:

- `internal/wiki/client.go` — add a binary-aware request helper alongside `Do()`
- `internal/wiki/attachments.go` _(new)_ — `Attachment` type + 4 client methods
- `internal/wiki/upload_sessions.go` _(new)_ — private 3-step upload helpers
- `internal/wiki/attachments_test.go` _(new)_
- `internal/wiki/upload_sessions_test.go` _(new)_
- `internal/cli/cli.go` — new `WikiAttachmentsCmd` group + 4 Run methods
- `internal/cli/e2e_test.go` — plain + JSON e2e per command
- `README.md` — drop the "No Wiki attachments" limitation; add a usage example
- `CLAUDE.md` — drop the "No Wiki attachments" line; document the upload-sessions invariant
- `plugins/yandex/skills/yandex/SKILL.md` — drop the limitation; add the 4 commands

Patterns reused verbatim:

- `auth.Load → wiki.New(cfg) → method → render.{One,Many,Confirm}` — every Run method follows this shape (`internal/cli/cli.go:81` and friends)
- Slug → numeric id resolution via `GetPage` already exists (`internal/wiki/pages.go:102` `UpdatePage`); list/upload/delete reuse that idiom
- Cursor pagination via `page.NextCursor` is already handled in `ListPages` (`internal/wiki/pages.go:60`); attachments list mirrors it
- `httptest.NewServer` driven through `YANDEX_WIKI_BASE_URL` is the e2e contract (`internal/cli/e2e_test.go:103`); attachment tests follow that

API endpoints (verified against official docs + cross-checked against `n-r-w/yandex-mcp`):

- `POST /v1/upload_sessions` body `{file_name, file_size}` → `{session_id, status, storage_type}`
- `PUT /v1/upload_sessions/{id}/upload_part?part_number=1` Content-Type `application/octet-stream` (≤16 MB single-part)
- `POST /v1/upload_sessions/{id}/finish`
- `POST /v1/pages/{idx}/attachments` body `{upload_sessions: [<id>]}` → `{results: [Attachment...]}`
- `GET /v1/pages/{idx}/attachments?cursor=&page_size=100` → `{results, next_cursor}`
- `GET /v1/pages/attachments/download_by_url?url=<page-slug>/<filename>&download=true` → binary
- `DELETE /v1/pages/{idx}/attachments/{file_id}` → 204

`Attachment` JSON fields used: `id, name, size, mimetype, download_url, created_at, check_status, has_preview`.

## Decisions baked in (from planning Q&A)

- **Scope:** full CRUD — `list, upload, download, delete`. No separate `get` (list already returns full metadata).
- **Upload size cap:** single-part only, ≤16 MB. Files larger fail fast with a clear error. No `--chunked` flag, no resume.
- **Addressing:** slug + filename. Internally resolve slug → page id; for `delete`, list attachments and look up file id by name. Duplicate filenames on the same page → fail with a clear error pointing the user at `--json list` to disambiguate.
- **Download UX:** `--output <path>`, default `-` (stdout). Binary streamed, never buffered fully.
- **Antivirus:** `check_status` is in JSON output but not in plain rows. Download refuses if `check_status != ready`.
- **Delete:** no `--yes` gate (consistent with the rest of the wiki CLI). The harness wraps destructive ops at a higher level.
- **Upload source:** `--file <path>` only. No stdin (we'd need a `--name` flag and the size cap forces buffering). Filename defaults to `filepath.Base(path)`; `--name <override>` is allowed.
- **Plain row format:** `name  size  mime  created_at` with two-space separator (matches `render.SkipEmpty` convention).
- **Test approach:** code-first per task; each task ends with its own httptest-driven e2e tests (plain + JSON).

## Development Approach

- **testing approach:** Regular (code first, then tests per task)
- complete each task fully before moving on
- every task ships with new/updated tests covering happy path + at least one error case
- `go test ./...` and `go vet ./...` must pass before starting the next task
- when scope changes mid-implementation, edit this file in the same commit

## Testing Strategy

- **unit tests:** per-package next to source files, table-driven where multiple cases share setup
- **e2e tests:** `internal/cli/e2e_test.go` style — spin up `httptest.NewServer`, point `YANDEX_WIKI_BASE_URL` at it, assert exact stdout in plain mode and key fields in JSON mode
- **no real network calls** in any test
- **no e2e UI** — this is a CLI-only project

## Progress Tracking

- mark `[x]` immediately when a checkbox is satisfied
- newly discovered tasks → ➕ prefix
- blockers → ⚠️ prefix
- if scope grows, update Overview + Decisions sections in this file in the same PR commit

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, doc files inside this repo
- **Post-Completion** (no checkboxes): manual verification with a real org, README badge nudges, downstream skill consumers

## Implementation Steps

### Task 1: Add binary-aware request helper to wiki client

**Files:**

- Modify: `internal/wiki/client.go`
- Modify: `internal/wiki/client_test.go`

The current `Do()` marshals body as JSON and decodes response as JSON. Upload-sessions and download both need raw byte streams. Add a `DoRaw(ctx, method, url, contentType string, body io.Reader) (*http.Response, error)` that callers close themselves — leaves response body open so callers can stream it.

- [x] add `DoRaw` method on `Client`: builds request with explicit content-type, passes body reader through, attaches the same `c.headers`, returns the raw `*http.Response` for non-2xx surfaces an `*APIError` reading body once
- [x] keep `Do()` unchanged — it is the JSON happy path and is used everywhere else
- [x] write unit test: PUT with octet-stream body, assert request method/path/content-type/body bytes seen by httptest server
- [x] write unit test: 4xx response surfaces `*APIError` (status preserved; `error_code`/`debug_message` fields fall back to raw body text since `extractErrorMsg` does not yet know those keys — wiki v1 errors typically use `detail`/`message` which are already supported)
- [x] run `go test ./internal/wiki/...` — must pass before Task 2

### Task 2: List attachments — type + client method + CLI

**Files:**

- Create: `internal/wiki/attachments.go`
- Create: `internal/wiki/attachments_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

Adds `Attachment` struct implementing both `Plainer` and `Rower` (List uses `Row`, future single-record callers use `Plain`). `ListAttachments` resolves slug → id via `GetPage`, then loops `GET /v1/pages/{idx}/attachments` until `next_cursor` is empty.

- [x] define `Attachment` struct: `ID int64, Name string, Size int64, Mimetype string, DownloadURL string, CreatedAt string, CheckStatus string, HasPreview bool` with JSON tags matching API
- [x] implement `Plain() string` using `render.SkipEmpty(name, humanSize, mimetype, created_at)` and `Row() string` returning the same (single-line)
- [x] add `humanSize(int64) string` helper (B/KB/MB/GB) — keep terse, no fractions for B/KB
- [x] implement `(*Client).ListAttachments(ctx, pageSlug string) ([]Attachment, error)`: GetPage → loop with cursor, page_size=100
- [x] add `WikiAttachmentsCmd` to `WikiCmd`, with `List ListAttachmentsCmd { PageSlug string \`arg:""\` }`; Run follows the standard pattern → `render.Many`
- [x] write unit test for `ListAttachments`: single-page response with 2 records, then a paginated case (next_cursor non-empty on first call, empty on second) — assert all records aggregated, assert request paths
- [x] write unit test for `Attachment.Row()` covering missing fields (size=0 still renders, missing mimetype skipped via SkipEmpty)
- [x] write e2e test (plain): stub both `GET /v1/pages?slug=...` (for slug→id) and `GET /v1/pages/{idx}/attachments`; assert exact stdout
- [x] write e2e test (JSON): same stub, with `--json`, assert `"name"`, `"size"`, `"check_status"` are present in the encoded array
- [x] run `go test ./...` — must pass before Task 3

### Task 3: Download attachment by slug + filename

**Files:**

- Modify: `internal/wiki/attachments.go`
- Modify: `internal/wiki/attachments_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

Uses the simpler `download_by_url` endpoint — no slug→id needed. Streams binary to the writer. Refuses to write when `check_status != ready`.

- [ ] add `(*Client).DownloadAttachment(ctx, pageSlug, filename string, w io.Writer) error`: build url-encoded `?url=<slug>/<filename>&download=true`, call `DoRaw`, copy resp.Body to w (always close body)
- [ ] before streaming, check `check_status` — but `download_by_url` returns binary not metadata, so the check happens via a precondition `ListAttachments` lookup; if duplicate filename on the page, fail with `multiple attachments named %q on %q; disambiguate via --json list`
- [ ] add `DownloadAttachmentCmd { PageSlug, Filename string; Output string \`name:"output" default:"-"\` }`; Run resolves writer (stdout if `-`, else `os.Create`)
- [ ] when `Output == "-"` and `Stdout` is the real os.Stdout AND it is a TTY, still allow it (no terminal-corruption guard — same as `cat`); document in SKILL.md
- [ ] write unit test: list returns one attachment with `check_status=ready`, then download endpoint returns bytes; assert bytes are written through unchanged
- [ ] write unit test: `check_status=infected` → no HTTP download call made, error mentions status
- [ ] write unit test: two attachments with same name → no HTTP download call, error message names the conflict
- [ ] write e2e test (plain): download to a temp file, assert file contents byte-for-byte
- [ ] write e2e test (plain, stdout): download to a `bytes.Buffer` via `--output -`, assert equality
- [ ] run `go test ./...` — must pass before Task 4

### Task 4: Upload sessions — private 3-step helper

**Files:**

- Create: `internal/wiki/upload_sessions.go`
- Create: `internal/wiki/upload_sessions_test.go`

Internal package surface only — `UploadAttachment` (next task) calls these. Splitting keeps each function under 30 lines and the test matrix readable.

- [ ] define `uploadSession struct { ID string \`json:"session_id"\`; Status string \`json:"status"\`; StorageType string \`json:"storage_type"\` }`
- [ ] `(c *Client) createUploadSession(ctx, fileName string, fileSize int64) (*uploadSession, error)` — POST /v1/upload_sessions
- [ ] `(c *Client) uploadPart(ctx, sessionID string, partNumber int, body io.Reader) error` — PUT via `DoRaw` with `application/octet-stream`, drain+close body
- [ ] `(c *Client) finishUploadSession(ctx, sessionID string) error` — POST .../finish, no body
- [ ] hard-code `MaxAttachmentSize = 16 * 1024 * 1024` constant; export it so the CLI layer can pre-check
- [ ] write unit test for each helper: assert URL, method, content-type, request body shape
- [ ] write unit test for full happy-path round trip (create → uploadPart → finish) using a single httptest server with a path switch
- [ ] write unit test: 5xx during uploadPart surfaces `*APIError` with the response status preserved
- [ ] run `go test ./internal/wiki/...` — must pass before Task 5

### Task 5: Upload attachment — public API + CLI command

**Files:**

- Modify: `internal/wiki/attachments.go`
- Modify: `internal/wiki/attachments_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

Glues the upload-sessions helper to the page-attach call. Returns the resulting `Attachment` so the CLI can print a `confirmed: <name>` line.

- [ ] `(c *Client) UploadAttachment(ctx, pageSlug, fileName string, body io.Reader, size int64) (*Attachment, error)`:
  - reject `size > MaxAttachmentSize` with a clear error mentioning the cap in MB
  - GetPage → resolve id
  - createUploadSession(fileName, size) → uploadPart(1, body) → finishUploadSession
  - POST /v1/pages/{id}/attachments with `{upload_sessions:[id]}` → decode `{results: [...]}`, return the first result
- [ ] `UploadAttachmentCmd { PageSlug string \`arg\`; File string \`name:"file" required\`; Name string \`name:"name"\` }`; Run opens the file with `os.Open`, stats it, calls UploadAttachment, then `render.Confirm("uploaded", attachment.Name)`
- [ ] use `--name` to override the file's basename; default to `filepath.Base(file)`
- [ ] write unit test: file body of 8 bytes, full 4-step round trip via path-switching httptest server, assert returned `Attachment`
- [ ] write unit test: size > 16 MB returns the size error before any HTTP call (verify by setting up server that fails on connection)
- [ ] write e2e test (plain): create temp file, run upload command, assert `uploaded: <name>\n`
- [ ] write e2e test (JSON): same, assert `{"uploaded":"<name>"}` shape
- [ ] write e2e test: missing `--file` exits non-zero with kong's standard error
- [ ] run `go test ./...` — must pass before Task 6

### Task 6: Delete attachment by slug + filename

**Files:**

- Modify: `internal/wiki/attachments.go`
- Modify: `internal/wiki/attachments_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

- [ ] `(c *Client) DeleteAttachment(ctx, pageSlug, filename string) error`:
  - GetPage → id; ListAttachments → find unique match by name; DELETE /v1/pages/{id}/attachments/{file_id}
  - duplicate filename → return same error message used in Download (single source of truth: factor a small `findAttachmentByName` helper)
  - missing filename → `attachment %q not found on page %q`
- [ ] `DeleteAttachmentCmd { PageSlug, Filename string }`; Run → `render.Confirm("deleted", filename)`
- [ ] write unit test: happy path, single match, DELETE issued with correct id
- [ ] write unit test: filename not found → no DELETE issued, error message matches
- [ ] write unit test: duplicate filenames → no DELETE issued, error matches Download's message
- [ ] write e2e test (plain): assert `deleted: <name>\n`
- [ ] write e2e test (JSON): assert `{"deleted":"<name>"}`
- [ ] run `go test ./...` — must pass before Task 7

### Task 7: Verify acceptance

- [ ] `go vet ./...` clean
- [ ] `go test ./...` clean
- [ ] manually inspect the four new commands' `--help` output (kong-generated) for accidental flag drift
- [ ] confirm no new env vars introduced (auth model untouched)
- [ ] confirm `wiki.Client` exports remain the only surface added; no new packages

### Task 8: Update documentation

**Files:**

- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `plugins/yandex/skills/yandex/SKILL.md`

- [ ] **README.md:** remove `**No Wiki attachments / image uploads**` from limitations; add a `wiki attachments` example block (one upload, one list); note the 16 MB cap in the limitations section instead
- [ ] **CLAUDE.md:** in "Things not to do," remove the `No Wiki attachments / image uploads` bullet; add a "Wiki attachments invariant" subsection naming the 3-step Upload Sessions protocol so future Claude does not re-invent a multipart shortcut; explicitly call out the `zolkinka/yandex-mcp` `POST /pages/{id}/files` pattern as wrong
- [ ] **CLAUDE.md:** update the "Layout" listing to add `internal/wiki/upload_sessions.go`
- [ ] **SKILL.md:** remove the `No Wiki attachments / image uploads` bullet; add the 4 new commands to the "Available commands" → Wiki section; bump the "8 commands" tally to 12
- [ ] **SKILL.md:** add a "Worked example" for uploading a screenshot
- [ ] move this plan: `mkdir -p docs/plans/completed && git mv docs/plans/20260501-wiki-attachments.md docs/plans/completed/`

## Technical Details

### Wire shapes

```go
// internal/wiki/attachments.go
type Attachment struct {
    ID          int64  `json:"id"`
    Name        string `json:"name"`
    Size        int64  `json:"size"`
    Mimetype    string `json:"mimetype"`
    DownloadURL string `json:"download_url"`
    CreatedAt   string `json:"created_at"`
    CheckStatus string `json:"check_status"`
    HasPreview  bool   `json:"has_preview"`
}

type attachmentsPage struct {
    Results    []Attachment `json:"results"`
    NextCursor string       `json:"next_cursor"`
}

type attachResultsBody struct {
    Results []Attachment `json:"results"`
}

type attachReq struct {
    UploadSessions []string `json:"upload_sessions"`
}
```

### Upload flow (single-part, ≤16 MB)

```
client.UploadAttachment(ctx, "team/notes", "ss.png", body, 12345)
  ├─ GetPage("team/notes")            → page.ID = 42
  ├─ POST   /v1/upload_sessions       {file_name:"ss.png", file_size:12345}
  │           → {session_id:"u-1", ...}
  ├─ PUT    /v1/upload_sessions/u-1/upload_part?part_number=1
  │           Content-Type: application/octet-stream, body=<12345 bytes>
  ├─ POST   /v1/upload_sessions/u-1/finish
  └─ POST   /v1/pages/42/attachments  {upload_sessions:["u-1"]}
              → {results:[{id:101, name:"ss.png", ...}]}
```

### CLI surface

```
yandex-cli wiki attachments list <page-slug>
yandex-cli wiki attachments upload <page-slug> --file <path> [--name <override>]
yandex-cli wiki attachments download <page-slug> <filename> [--output <path>|-]
yandex-cli wiki attachments delete <page-slug> <filename>
```

`--json` is inherited from the root flag, same as the rest of the CLI.

### Error messages we own

- `"file too large: %d bytes (max 16 MiB)"` — fail before any HTTP call
- `"multiple attachments named %q on %q; disambiguate via --json list"` — duplicate name on a page
- `"attachment %q not found on page %q"` — missing name
- `"attachment %q has check_status=%s; refusing to download"` — antivirus state

All other errors propagate from `*APIError` unchanged.

## Post-Completion

_Items requiring manual intervention or external systems — informational only._

**Manual verification with a real organization:**

- upload a small PNG to a test page in both Yandex Cloud and Yandex 360 organizations to confirm header parity
- list, download, delete the same file end-to-end
- confirm `check_status` cycles `check → ready` for a file that triggers AV scan (large or unusual binary)

**External systems:**

- the marketplace `butvinm-yandex-skill` plugin entry will pick up the new commands automatically once the SKILL.md ships, but downstream consumers using a pinned plugin version need to bump
- no infrastructure or deployment changes — pure CLI release
