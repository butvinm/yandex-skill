# Markdown Round-trip with Auto-synced Attachments

## Overview

Add `--attachments-dir <dir>` to `wiki pages get/create/update` and `--output <path>` to `wiki pages get`. With `--attachments-dir`, the command rewrites Yandex-Wiki attachment URLs (`/<slug>/.files/X`) to/from local relative paths and uploads/downloads attachment bytes alongside the markdown body. Without those flags, the existing thin-wrapper behavior is unchanged.

Why this earns its place beside the existing primitives: LLM agents asked to edit Wiki pages today have to compose `wiki pages get` + `wiki attachments list` + N× `wiki attachments download` and then perform the URL rewriting in their own head. That's where they break — duplicate filenames, transliterated server names, the `:file[]` directive vs `![](=WxH)` divergence. The CLI can do this deterministically; the model cannot.

## Context (from discovery)

Files that grow or get touched:

- `internal/wiki/pages.go` — extend `Page` with `PageType string \`json:"page_type"\``; expose it on the existing struct (no new field requested via `fields=`, the API returns it unsolicited).
- `internal/wiki/pages_test.go` — cover decode of all observed `page_type` values (`wysiwyg`, `page`, `grid`, missing).
- `internal/cli/markdown.go` _(new)_ — link-rewrite helpers (server↔local), attachment-sync orchestrator. CLI-side because it does file I/O; the wiki package stays API-shaped per CLAUDE.md.
- `internal/cli/markdown_test.go` _(new)_ — unit-test the regex + helpers exhaustively (cheaper than hitting full e2e for every edge case).
- `internal/cli/cli.go` — extend `GetPageCmd`, `CreatePageCmd`, `UpdatePageCmd` with new flags and the page-type guard.
- `internal/cli/e2e_test.go` — add the matrix (page_type × command × flag presence).
- `README.md` — document new flags + the page-type matrix in plain language.
- `plugins/yandex/skills/yandex/SKILL.md` — surface the new capability for the agent.
- `CLAUDE.md` — note that `--attachments-dir` is a YFM-only feature and the rewrite-regex invariant.

Patterns reused verbatim:

- `auth.Load → wiki.New(cfg) → method → render.{One,Many,Confirm}` — every Run method follows this shape (`internal/cli/cli.go:90`+). New flag handling extends but does not break the pattern.
- Slug → numeric id resolution via `GetPage` (already used by `UpdatePage` and `UploadAttachment`) — reuse, don't duplicate.
- `httptest.NewServer` driven through `YANDEX_WIKI_BASE_URL` is the e2e contract (`internal/cli/e2e_test.go:103`+) — every new e2e follows it.

API endpoints already in the client (no new ones for this feature):

- `GET /v1/pages?slug=X&fields=content` returns `page_type` unsolicited.
- `GET /v1/pages/{id}/attachments` for listing
- `POST /v1/upload_sessions` (3-step) for uploads
- `GET /v1/pages/attachments/download_by_url?url=<slug>/<file>` for downloads

## Decisions baked in (from brainstorm Q&A)

- **No new commands.** Flags on existing commands. The unification trade-off was weighed against the failure mode of "user runs get without flag, edits, runs update without flag, page corrupted" — mitigated by the `--attachments-dir` opt-in being explicit and by the page-type warning when present.
- **Page-type matrix:**

  | page_type              | get with flag | update with flag | create with flag            |
  | ---------------------- | ------------- | ---------------- | --------------------------- |
  | `wysiwyg`              | ✓             | ✓                | ✓ (always)                  |
  | `page` (legacy)        | warn, proceed | warn, proceed    | n/a (create always wysiwyg) |
  | `grid` (dynamic table) | refuse        | refuse           | n/a                         |
  | unknown                | warn, proceed | warn, proceed    | n/a                         |

- **Regex contract (the only stable invariant):** `/<escaped-current-slug>/\.files/[^\s)\]"'}]+`. Match URL substring only; ignore the surrounding markdown construct (image, file directive, legacy `0x0:`, future extensions). Cross-page references (`/<other-slug>/.files/X`) are left untouched.
- **Local filename:** `path.Base(download_url)` always. The `Attachment.Name` field is non-unique on real pages (sweep observed 3× `изображение.png` distinguished only by URL suffix). On upload, server returns the canonical `download_url` — use that for the rewrite, never predict it.
- **Download every attachment, not only referenced ones.** The kekloack page proves attachments can live in the sidebar with no inline reference. A `get` that drops sidebar files would silently lose data on round-trip.
- **No server-side deletion** of orphaned attachments on update. One-directional drift may be intentional. A separate explicit `wiki attachments delete` already exists.
- **`--output` semantics:** when set, writes raw `.Content` to file. The stdout default behavior (title-prefixed `Plain()` rendering) is unchanged. With `--attachments-dir` and no `--output`, write raw content to stdout — markdown round-trip semantics override the human-friendly rendering.
- **Helper location:** CLI-side (`internal/cli/markdown.go`), not wiki-side. The wiki package is API-shaped per CLAUDE.md; file I/O belongs in the CLI layer.
- **No `--strict` flag for now.** Warn-and-proceed for `page` and unknown is sufficient based on the sweep data (only 6 legacy pages have inline `/.files/` URLs across 1173 pages, all in benign `0x0:/X` form).
- **Test approach:** code-first per task; each task ends with its own tests (unit + e2e where the path crosses kong/render).

## Development Approach

- **testing approach:** Regular (code first, then tests per task)
- complete each task fully before moving on
- every task ships with new/updated tests covering happy path + at least one error case
- `go test ./...` and `go vet ./...` must pass before starting the next task
- when scope changes mid-implementation, edit this file in the same commit
- atomic commits, specific `git add <files>`, never `git add .`

## Testing Strategy

- **unit tests:** `internal/cli/markdown_test.go` carries the regex + rewrite edge cases (image syntax, file directive, legacy `0x0:`, cross-page refs, duplicates, empty content, unicode names). Cheap, exhaustive, no HTTP setup.
- **e2e tests:** `internal/cli/e2e_test.go` covers each flagged command path against an `httptest` Wiki, asserting plain stdout, JSON output, stderr warnings, and disk side-effects (attachment files written/read).
- **no real network calls** in any test.
- **no UI tests** — CLI-only project.

## Progress Tracking

- mark `[x]` immediately when a checkbox is satisfied
- newly discovered tasks → ➕ prefix
- blockers → ⚠️ prefix
- if scope grows, update Overview + Decisions in this file in the same PR commit

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): downstream agent skill verification, manual UI sanity check on real pages.

## Implementation Steps

### Task 1: Plumb `page_type` through the wiki client

**Files:**

- Modify: `internal/wiki/pages.go`
- Modify: `internal/wiki/pages_test.go`

- [x] add `PageType string \`json:"page_type"\``to`Page`struct in`internal/wiki/pages.go:19`
- [x] add a constant set near the struct: `const (PageTypeWysiwyg = "wysiwyg"; PageTypePage = "page"; PageTypeGrid = "grid")` to keep magic strings out of the CLI layer
- [x] (no change to `fields=content` query — the API returns `page_type` regardless; verified by probe)
- [x] (no change to `Plain()` output — `PageType` is for programmatic use, not LLM-facing display)
- [x] add table-driven `TestPage_DecodePageType` in `internal/wiki/pages_test.go` covering: `"wysiwyg"`, `"page"`, `"grid"`, missing field (zero value)
- [x] run `go test ./internal/wiki/... && go vet ./...` — must pass before Task 2

### Task 2: Link-rewrite helpers (server → local, "read" direction)

**Files:**

- Create: `internal/cli/markdown.go`
- Create: `internal/cli/markdown_test.go`

- [x] create `internal/cli/markdown.go` with `func rewriteServerToLocal(content, pageSlug, attachmentsDir string) string`
- [x] regex per Decisions: `regexp.MustCompile("/" + regexp.QuoteMeta(pageSlug) + `/\.files/[^\s)\]"'\}]+`)`. Build per-call (page slug is variable); cache via `sync.Map[slug]*regexp.Regexp` if profiling later shows cost. **For first cut: build per call, no cache.**
- [x] replacement: rewrite each match to `<attachmentsDir>/<basename>` where basename is the path tail after `/.files/`
- [x] `attachmentsDir` is used verbatim (no trailing-slash normalization other than what the caller passes); document this in a one-line comment if non-obvious
- [x] write `TestRewriteServerToLocal` (table-driven) covering:
  - YFM image: `![alt](/<slug>/.files/X =375x383)` → `![alt](<dir>/X =375x383)`
  - YFM file directive: `:file[name](/<slug>/.files/X){type="..."}` → `:file[name](<dir>/X){type="..."}`
  - Legacy: `0x0:/<slug>/.files/X` → `0x0:<dir>/X`
  - Cross-page: `/<other-slug>/.files/X` → unchanged
  - Multiple matches in one content
  - Unicode filename (`изображение.png` style with collision suffix `-1`)
  - Empty content → empty
  - Slug with regex metacharacters (`a.b/c+d`) — must be escaped via `QuoteMeta`
- [x] run `go test ./internal/cli/... && go vet ./...` — must pass before Task 3

### Task 3: Link-rewrite helpers (local → server, "write" direction)

**Files:**

- Modify: `internal/cli/markdown.go`
- Modify: `internal/cli/markdown_test.go`

- [x] add `func findLocalAttachmentRefs(content, attachmentsDir string) []string` — returns unique basenames found as `<attachmentsDir>/<X>` substrings in content. Same URL-substring approach (no markdown parsing).
- [x] add `func rewriteLocalToServer(content, attachmentsDir string, urlByBasename map[string]string) string` — replace each `<attachmentsDir>/<X>` with `urlByBasename[X]`. If a basename isn't in the map, leave it (caller decides whether to error; this helper is dumb).
- [x] regex for discovery: `regexp.MustCompile(regexp.QuoteMeta(attachmentsDir) + `/[^\s)\]"'\}]+`)`. Same matcher principle as Task 2.
- [x] write `TestFindLocalAttachmentRefs`: image, file directive, mixed, none, duplicates collapsed to unique
- [x] write `TestRewriteLocalToServer`: image rewritten to server URL, file directive rewritten, missing-basename left alone
- [x] run tests — must pass before Task 4

### Task 4: `--output` flag on `wiki pages get` (no `--attachments-dir` path)

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

- [x] add `Output string \`name:"output" help:"write raw page content to file (default: stdout via Plain rendering)"\``to`GetPageCmd` (`internal/cli/cli.go:151`)
- [x] when `Output != ""`, after fetching, write `page.Content` (raw) to the file via `os.WriteFile`, not via `render.One`. Stdout fallback path stays as it is (full `Plain()` rendering with title + timestamp).
- [x] handle `--output -` explicitly as "stdout but raw content (no title prefix)" — useful for piping
- [x] add e2e test `TestGetPage_OutputFile`: stub a wysiwyg page, assert file on disk equals `.Content` exactly (no Plain prefix)
- [x] add e2e test `TestGetPage_OutputDash`: `--output -` writes raw to stdout
- [x] add e2e test `TestGetPage_NoOutput_DefaultUnchanged`: existing behavior preserved
- [x] run `go test ./... && go vet ./...` — must pass before Task 5

### Task 5: `--attachments-dir` on `wiki pages get` (full read flow)

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/markdown.go`
- Modify: `internal/cli/markdown_test.go`
- Modify: `internal/cli/e2e_test.go`

- [x] add `AttachmentsDir string \`name:"attachments-dir" help:"sync attachments to local directory and rewrite content URLs"\``to`GetPageCmd`
- [x] in `internal/cli/markdown.go`, add the orchestrator: `func syncAttachmentsForGet(ctx, client *wiki.Client, page *wiki.Page, attachmentsDir string, stderr io.Writer) (rewrittenContent string, err error)` — handles guard, mkdir, downloads (every attachment, not just referenced), rewrite
- [x] page-type guard logic: `grid` → return error `"page_type=grid: structured table, not markdown — see /v1/grids/{id} (out of scope for this CLI)"`; not `wysiwyg` and not `grid` → write warning to `stderr` (`"warning: page_type=%s: content may not be Yandex Flavored Markdown; attachment-link rewriting may have no effect"`); proceed
- [x] `os.MkdirAll(attachmentsDir, 0o755)` before downloads
- [x] for each attachment: `dst := filepath.Join(attachmentsDir, path.Base(att.DownloadURL))`; open dst (0o644); call `client.DownloadAttachment(ctx, slug, att.Name, dst)` — wait, current `DownloadAttachment` takes a name argument and does the lookup. Use the existing call shape. **NB:** double-check — given B2 was fixed to download by URL, this should already work; verify in implementation.
- [x] integrate with `GetPageCmd.Run`: if `AttachmentsDir != ""`, after fetch, run sync + rewrite, then write rewritten content (to `--output` or stdout-raw, NOT through Plain())
- [x] add unit test `TestSyncAttachmentsForGet_Wysiwyg`: stub client, assert files written + content rewritten
- [x] add unit test `TestSyncAttachmentsForGet_Grid_Refuses`
- [x] add unit test `TestSyncAttachmentsForGet_Page_Warns`
- [x] add e2e test `TestGetPage_AttachmentsDir_Wysiwyg_ImageAndFileDirective`: stub Wiki returning content with both syntaxes + 2 attachments; assert disk files + rewritten stdout
- [x] add e2e test `TestGetPage_AttachmentsDir_CrossPageRef_Untouched`
- [x] add e2e test `TestGetPage_AttachmentsDir_DuplicateNames`: 3 attachments named X but with `download_url` X, X-1, X-2 → 3 distinct local files
- [x] add e2e test `TestGetPage_AttachmentsDir_Page_WarningOnStderr`
- [x] add e2e test `TestGetPage_AttachmentsDir_Grid_RefuseWithError`
- [x] run `go test ./... && go vet ./...` — must pass before Task 6

### Task 6: `--attachments-dir` on `wiki pages update`

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/markdown.go`
- Modify: `internal/cli/markdown_test.go`
- Modify: `internal/cli/e2e_test.go`

- [x] add `AttachmentsDir string \`name:"attachments-dir"\``to`UpdatePageCmd`
- [x] add orchestrator `func syncAttachmentsForWrite(ctx, client *wiki.Client, page *wiki.Page, localContent, attachmentsDir string, stderr io.Writer) (rewrittenContent string, err error)`:
  - same page-type guard as get (refuse grid; warn else)
  - call `findLocalAttachmentRefs(localContent, attachmentsDir)` → list of basenames
  - list current server attachments via `client.ListAttachments`; build `serverByBasename map[string]*Attachment` keyed by `path.Base(att.DownloadURL)` (canonical)
  - for each local basename: if not in `serverByBasename`, upload from `<dir>/<basename>` via existing `UploadAttachment`; capture returned `download_url`. If already present, reuse the existing URL.
  - build `urlByBasename map[string]string` from union; rewrite via `rewriteLocalToServer`
- [x] hook into `UpdatePageCmd.Run`: if `AttachmentsDir != ""`, after `BodyInput.Read`, run sync; pass rewritten content to `UpdatePage`
- [x] unit test `TestSyncAttachmentsForWrite_NewAttachment_Uploads`
- [x] unit test `TestSyncAttachmentsForWrite_ExistingAttachment_NoReupload` — exact basename match by URL, not by Name
- [x] unit test `TestSyncAttachmentsForWrite_Grid_Refuses`
- [x] unit test `TestSyncAttachmentsForWrite_NoLocalRefs_NoUploads_NoRewrite`
- [x] e2e `TestUpdatePage_AttachmentsDir_NewFile`: local file referenced + uploaded; final POST body has rewritten URL
- [x] e2e `TestUpdatePage_AttachmentsDir_ExistingFile_Skips`: server already has matching attachment; assert no PUT to upload-sessions endpoint
- [x] e2e `TestUpdatePage_AttachmentsDir_Grid_Refuses`
- [x] run `go test ./... && go vet ./...` — must pass before Task 7

### Task 7: `--attachments-dir` on `wiki pages create`

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/e2e_test.go`

- [x] add `AttachmentsDir string \`name:"attachments-dir"\``to`CreatePageCmd`
- [x] in `CreatePageCmd.Run`: when `AttachmentsDir != ""`, call `CreatePage` with empty content first to obtain a slug+id; then call the same `syncAttachmentsForWrite` orchestrator from Task 6 with the local body + the freshly-created page; then call `UpdatePage` with rewritten content. **Yes, this is two API calls instead of one** — but the alternative is uploading attachments before knowing the page slug, which the API doesn't support (attachments bind to a page id).
- [x] e2e `TestCreatePage_AttachmentsDir_NewPageWithImage`: assert sequence (POST create → POST upload-sessions → POST attach → POST update with rewritten body)
- [x] e2e `TestCreatePage_NoAttachmentsDir_BehaviorUnchanged`
- [x] run `go test ./... && go vet ./...` — must pass before Task 8

### Task 8: Documentation

**Files:**

- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `plugins/yandex/skills/yandex/SKILL.md`

- [x] `README.md`: add a "Markdown round-trip" section under the wiki commands; show one read example, one write example, mention page-type matrix in plain language; update the limitations section if it claims something this feature changes
- [x] `CLAUDE.md`: add a paragraph in the Wiki conventions section: "`--attachments-dir` is YFM-only; the rewrite regex is scoped to `/<current-slug>/\.files/X` to avoid mangling cross-page refs; grid pages are refused"; reference the sweep test for re-validation
- [x] `plugins/yandex/skills/yandex/SKILL.md`: surface the new flags and the page-type matrix so the agent knows when to refuse upfront
- [x] no test changes for docs

### Task 9: Verify acceptance criteria

- [x] verify all rows in the page-type matrix in Decisions are exercised by an e2e (9 e2e tests cover wysiwyg/page/grid × get/update + create)
- [x] verify both YFM syntaxes (`![](=WxH)` and `:file[]{type=...}`) and the legacy `0x0:` syntax are exercised by markdown_test (TestRewriteServerToLocal table cases)
- [x] verify cross-page reference is exercised (TestRewriteServerToLocal + TestE2E_WikiPagesGet_AttachmentsDir_CrossPageRefUntouched)
- [x] verify duplicate-name attachments are exercised (TestE2E_WikiPagesGet_AttachmentsDir_DuplicateNames)
- [x] run `go test ./...` — all green
- [x] run `go vet ./...` — clean
- [x] run `go build -o /tmp/yandex-cli-final ./cmd/yandex-cli` — clean
- [ ] manual smoke (user-driven): `wiki pages get users/m.butvin/<live-wysiwyg-slug> --output /tmp/x.md --attachments-dir /tmp/att/`; verify file contents and attachment files. (Test pages from brainstorm were cleaned up; needs a fresh live wysiwyg page with at least one attachment.)
- [ ] manual smoke (user-driven): edit `/tmp/x.md`, then `wiki pages update <slug> --body-file /tmp/x.md --attachments-dir /tmp/att/`; verify the page in the UI

### Task 10: Move plan to completed

- [ ] `git mv docs/plans/20260502-markdown-roundtrip.md docs/plans/completed/20260502-markdown-roundtrip.md`
- [ ] commit the move

## Technical Details

**Regex anchors and escaping.** The slug can contain `/`, `-`, `.`, alphanumerics, and (per the org's data) transliterated cyrillic. Always pass through `regexp.QuoteMeta` before embedding in the pattern. The terminator class `[^\s)\]"'\}]+` was chosen to stop at common surrounding constructs:

- `\s` — whitespace
- `)` — markdown link/image close
- `]` — directive/attribute end
- `"`, `'` — attribute quote close
- `}` — directive attribute block close

This greedy class will over-match if a URL legitimately contains those characters (rare for `/.files/` paths). If we ever observe such a case, we tighten — for now, sufficient.

**Attachment-orchestrator return shapes.**

```go
func syncAttachmentsForGet(ctx, client, page, dir, stderr) (string, error)
func syncAttachmentsForWrite(ctx, client, page, localContent, dir, stderr) (string, error)
```

Both return the rewritten content (caller writes it onward). Both write warnings to the provided `stderr` so tests can capture and assert.

**`page` (legacy) write path.** Per Decisions, we warn and proceed — the user is choosing to overwrite legacy syntax with markdown. That's a content swap, not silent corruption. The warning makes the intent explicit.

**`grid` refusal error.** Single error message: `"page_type=grid: structured table, not markdown content (see /v1/grids/{id})"`. JSON mode wraps it via `render.Error`.

**Why no `--strict` / opt-out for the warning.** The sweep showed only 6/1173 pages would surface a warning, all with benign `0x0:` URLs that the rewrite handles symmetrically (`0x0:/slug/.files/X` ↔ `0x0:dir/X`). Adding a flag for an edge case nobody has asked for is YAGNI.

## Post-Completion

_Items requiring manual intervention or external systems — informational only_

**Manual verification:**

- run `wiki pages get` + edit + `wiki pages update` round-trip on a real wysiwyg page in `users/m.butvin/`; confirm the page renders correctly in the Wiki UI (no broken images, attachments still attached)
- repeat with a `:file[]` directive present
- repeat with duplicate-named attachments

**Follow-up work intentionally deferred:**

- ➕ Optional: extend `enrichTitles` in `ListPages` to also pull `page_type` (zero extra HTTP cost since it's already in the per-item GET) and add a `[legacy]`/`[grid]` marker in the list output. Pure UX win, not a blocker for this feature.
- ➕ B5: existing `wiki pages update <grid-slug> --body-file foo.md` path has no `page_type` guard and could overwrite grid data. Out of scope here; track separately.
- ➕ Possible future `wiki grids list/get` commands (read-only, mirroring `n-r-w/yandex-mcp` precedent).
