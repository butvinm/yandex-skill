# yandex-cli — Go CLI for Yandex Tracker (read) and Yandex Wiki (read+write)

## Overview

Build a single Go binary `yandex-cli` exposing 8 commands across two product groups (Tracker, Wiki) with token-from-env auth, plain-text default output (`--json` opt-in), and a Claude Code skill in the same repo. Targets Yandex Cloud Organization tenancy only; 360/OAuth deferred but architected for later addition.

Solves: agent-driven Tracker reads and Wiki authoring without depending on the existing `n-r-w/yandex-mcp` (which lacks Wiki write) and without the friction of per-call MCP server setup. CLI composes via shell pipes — read with one command, transform with another, write with a third.

Integrates as a Claude Code skill at `.claude/skills/yandex/SKILL.md`. Skill invokes the binary on PATH; binary requires `$YANDEX_TOKEN` and `$YANDEX_CLOUD_ORG_ID` set in the calling shell.

## Context (from brainstorm)

- **Working dir:** `/home/butvinm/Dev/yandex-cli` (greenfield — no files yet)
- **Reference (rejected):** `n-r-w/yandex-mcp` (read-only, MCP-not-CLI, but its env-var conventions copied: `YANDEX_CLOUD_ORG_ID`, `YANDEX_TRACKER_BASE_URL`, `YANDEX_WIKI_BASE_URL`)
- **API endpoints (all verified before plan commit, sources cited below):**
  - **Tracker** `https://api.tracker.yandex.net/v3/` — header `X-Org-ID`. `Authorization: Bearer <IAM>`.
    - `GET /v3/issues/{key}` — get issue. Response includes `key`, `summary`, `status` (object — use `.display`), `assignee` (object — use `.display`), `updatedAt`, `description`. ([source](https://yandex.ru/support/tracker/en/concepts/issues/get-issue))
    - `POST /v3/issues/_search` — search. Body uses ONE of: `{"queue":"FOO"}`, `{"keys":[...]}`, `{"filter":{...}}`, `{"query":"..."}`. ([source](https://yandex.ru/support/tracker/en/concepts/issues/search-issues))
    - `GET /v3/queues/` and `GET /v3/queues/{key}` — list/get queues. Fields: `key`, `name`, `lead` (object), `defaultPriority` (object). ([source](https://yandex.ru/support/tracker/en/concepts/queues/get-queues))
  - **Wiki** `https://api.wiki.yandex.net/v1/` — header `X-Cloud-Org-Id` (different from Tracker — real API quirk). `Authorization: Bearer <IAM>`.
    - `GET /v1/pages?slug=<slug>&fields=content` — get page. **`content` is omitted by default** — must request via `fields=content`. Response: `id`, `slug`, `title`, `attributes.modified_at`, `content`. ([source](https://yandex.ru/support/wiki/en/api-ref/pages/pages__get_page_details))
    - `POST /v1/pages` — create. Body: `{slug, title, content}`. Optional query `?is_silent=true` to suppress notifications. ([source](https://yandex.ru/support/wiki/en/api-ref/pages/pages__create_page))
    - `POST /v1/pages/{idx}` — update. **Path takes numeric id, NOT slug.** Body: `{title?, content?}`. So `update <slug>` is a two-step operation: GET by slug → resolve `id` → POST to `/pages/{id}`. ([source](https://yandex.ru/support/wiki/en/api-ref/pages/pages__update_page_details))
    - `GET /v1/pages/descendants?slug=<parent>&page_size=50&cursor=...` — list children by parent slug. **Returns only `{id, slug}` per result** — title and modified_at NOT in list response. Cursor-paginated. ([source](https://yandex.ru/support/wiki/en/api-ref/pages/pages__descendants_by_slug))
    - **No free-text search exists** — `--query` is dropped from `wiki pages list`.
  - **Wiki content format:** docs say `content` is "page content as a string" but don't specify Markdown vs YFM. Yandex Wiki UI uses YFM (markdown superset). MVP assumption: pass user-provided text through verbatim; document in README that YFM features (cuts, notes, etc.) work, plain markdown also works.
- **Yandex Wiki bans service accounts** ([wiki access doc](https://yandex.ru/support/wiki/en/api-ref/access)) — the CLI runs as a real user identity. Token comes from `yc iam create-token` on a user-authenticated `yc` profile.

## Development Approach

- **Testing approach:** Regular (code first, then tests per task)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task with code changes MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting the next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Atomic commits per task. Stage specific files only — never `git add .`. Clear messages.

## Testing Strategy

- **Unit tests:** `testing` + `net/http/httptest` for API client mocking. Required per task that touches code.
- **No e2e tests:** project has no UI. The "integration test" would be hitting real Yandex APIs, which (a) needs credentials, (b) is flaky, (c) is the user's manual smoke-test step.
- **Manual smoke-test (post-completion):** documented in README as "verify install" steps.

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document blockers with ⚠️ prefix
- Keep plan in sync with actual work

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): all code, tests, docs in this repo
- **Post-Completion** (no checkboxes): manual smoke-test, registering the skill in user's Claude Code, future OAuth/360 work

---

## Implementation Steps

### Task 1: Bootstrap Go module and project skeleton

**Files:**

- Create: `go.mod`
- Create: `cmd/yandex-cli/main.go` (stub printing "yandex-cli vX")
- Create: `Makefile`
- Create: `.gitignore`
- Create: `README.md` (stub — auth setup section deferred to Task 11)

- [x] `go mod init github.com/butvinm/yandex-cli`
- [x] add minimal `cmd/yandex-cli/main.go` printing version (placeholder for kong wiring)
- [x] add `Makefile` targets: `build`, `install`, `test`, `lint`, `vet`, `fmt`. `build` runs `go build -ldflags "-X main.version=$(shell git describe --tags --always 2>/dev/null || echo dev)" -o bin/yandex-cli ./cmd/yandex-cli`
- [x] add `.gitignore` for `bin/`, `*.test`, `coverage.out`
- [x] verify `make build` produces working binary that prints version
- [x] (no tests this task — pure scaffolding, nothing to test yet)

### Task 2: Auth module — read env, build product-specific headers

**Files:**

- Create: `internal/auth/auth.go`
- Create: `internal/auth/auth_test.go`

- [x] define `type Config struct { Token, OrgID, TrackerBaseURL, WikiBaseURL string }` and `func Load() (Config, error)` reading `YANDEX_TOKEN`, `YANDEX_CLOUD_ORG_ID`, `YANDEX_TRACKER_BASE_URL` (default `https://api.tracker.yandex.net`), `YANDEX_WIKI_BASE_URL` (default `https://api.wiki.yandex.net`)
- [x] return clear error if `YANDEX_TOKEN` or `YANDEX_CLOUD_ORG_ID` missing — error message hints at `yc iam create-token` and `yc organization-manager organization list`
- [x] add `func (c Config) TrackerHeaders() http.Header` → `Authorization: Bearer <token>`, `X-Org-ID: <orgid>`, `Content-Type: application/json`
- [x] add `func (c Config) WikiHeaders() http.Header` → `Authorization: Bearer <token>`, `X-Cloud-Org-Id: <orgid>`, `Content-Type: application/json`
- [x] write tests: env-var precedence, missing-required-vars error, default base URLs, header builders for both products
- [x] run `go test ./internal/auth/...` — must pass before Task 3

### Task 3: Render module — plain + JSON formatters

**Files:**

- Create: `internal/render/render.go`
- Create: `internal/render/render_test.go`

- [ ] define `type Format string; const (Plain Format = "plain"; JSON Format = "json")`
- [ ] define `type Renderable interface { Plain() string }` — each domain type implements its own plain rendering
- [ ] add `func Render(w io.Writer, format Format, v any) error` — JSON mode marshals with indent; plain mode requires `Renderable` (or a slice of `Renderable` joined by newlines)
- [ ] add `func RenderError(w io.Writer, format Format, err error, status int)` — plain: prints to stderr; JSON: prints `{"error": "...", "status": <int>}` to stderr
- [ ] add helper `func skipEmpty(parts ...string) string` — joins non-empty parts with two-space separator (used by list formatters)
- [ ] write tests: JSON marshal correctness, plain rendering uses `Renderable.Plain()`, empty-field skipping in `skipEmpty`, error rendering in both formats
- [ ] run tests — must pass before Task 4

### Task 4: Tracker HTTP client (no commands yet)

**Files:**

- Create: `internal/tracker/client.go`
- Create: `internal/tracker/client_test.go`

- [ ] define `type Client struct { http *http.Client; baseURL string; headers http.Header }`, constructor `New(cfg auth.Config) *Client`
- [ ] add `func (c *Client) do(ctx context.Context, method, path string, body, out any) error` — marshals body, sets headers, decodes into `out`, returns typed error on non-2xx (`type APIError struct { Status int; Message string }`)
- [ ] handle pagination via Tracker's `Link` / `X-Total-Count` headers — return all pages as a single slice from list/search calls (document this; reconsider if response sizes get huge)
- [ ] write tests using `httptest.NewServer`: success path, 4xx error mapping, header propagation, multi-page pagination
- [ ] run tests — must pass before Task 5

### Task 5: Tracker domain types — Issue, Queue — and operations

**Files:**

- Create: `internal/tracker/issues.go`
- Create: `internal/tracker/queues.go`
- Create: `internal/tracker/issues_test.go`
- Create: `internal/tracker/queues_test.go`

- [ ] define `type Display struct { Display string \`json:"display"\` }`— Tracker returns nested objects with a`.display` field for status/assignee/lead/defaultPriority
- [ ] define `type Issue struct { Key, Summary, UpdatedAt, Description string; Status, Assignee Display }` (only fields we render)
- [ ] implement `GetIssue(ctx, key) (*Issue, error)` → `GET /v3/issues/{key}`
- [ ] implement `ListIssues(ctx, queue, query string) ([]Issue, error)` → `POST /v3/issues/_search`. Body: `{"queue":"FOO"}` if `queue` set, else `{"query":"..."}` if `query` set, else return validation error "specify --queue or --query". Handle pagination (Tracker uses `Link` header for next page) — fetch all pages.
- [ ] implement `Issue.Plain()` → `<KEY>: <summary>\n\n<status.display>  <assignee.display>  <updatedAt>\n\n<description>`. Skip empty fields.
- [ ] implement list-row plain helper `IssueRow(i) string` → `<KEY>  <status.display>  <assignee.display>  <summary>` (two-space-separated, skip empty)
- [ ] define `type Queue struct { Key, Name string; Lead, DefaultPriority Display }`
- [ ] implement `GetQueue(ctx, key) (*Queue, error)` → `GET /v3/queues/{key}`
- [ ] implement `ListQueues(ctx) ([]Queue, error)` → `GET /v3/queues/`
- [ ] implement `Queue.Plain()` and queue-row helper per design
- [ ] write tests for each method (success + 404 + 401-auth-failure) using `httptest`. Assert request body shape for `_search`.
- [ ] run tests — must pass before Task 6

### Task 6: Wiki HTTP client (no commands yet)

**Files:**

- Create: `internal/wiki/client.go`
- Create: `internal/wiki/client_test.go`

- [ ] mirror Task 4 structure but using `WikiHeaders()` and `WikiBaseURL`
- [ ] same `APIError` type pattern (consider extracting to a shared package later — for now duplicate, prefer duplication over premature abstraction)
- [ ] write tests using `httptest`: success path, 4xx mapping, header propagation including `X-Cloud-Org-Id` (different from Tracker — assert this in test)
- [ ] run tests — must pass before Task 7

### Task 7: Wiki Pages — types, get/list/create/update

**Files:**

- Create: `internal/wiki/pages.go`
- Create: `internal/wiki/pages_test.go`

- [ ] define `type Page struct { ID int64 \`json:"id"\`; Slug, Title, Content string; Attributes PageAttrs }`and`type PageAttrs struct { ModifiedAt string \`json:"modified_at"\` }`(drop`Author`— descendants endpoint doesn't return it;`get` endpoint doesn't either based on schema fetched. Re-verify during implementation; add if API exposes it)
- [ ] define `type PageRef struct { ID int64; Slug string }` for descendant list rows
- [ ] implement `GetPage(ctx, slug string) (*Page, error)` → `GET /v1/pages?slug=<urlencoded>&fields=content`. Always include `fields=content` — content is omitted by default per API.
- [ ] implement `ListPages(ctx, parent string) ([]PageRef, error)` → `GET /v1/pages/descendants?slug=<parent>&page_size=100`, follow `next_cursor` until exhausted. Returns `id,slug` only — title/modified_at NOT included by descendants API.
- [ ] implement `CreatePage(ctx, slug, title, content string) error` → `POST /v1/pages` with body `{slug, title, content}`. Pass `?is_silent=true` to suppress notifications (default; consider flag for opt-out later).
- [ ] implement `UpdatePage(ctx, slug, content string) error` — TWO-STEP: first `GetPage(slug)` to resolve `id`, then `POST /v1/pages/{id}` with body `{content}`. Document this in code comment (only place a comment is justified — non-obvious API quirk).
- [ ] implement `Page.Plain()` → `<title>\n\n<attributes.modified_at>\n\n<content>`. Skip empty fields. (No `author` in plain output until we confirm API exposes it — adjust plan if it does.)
- [ ] implement list-row plain helper `PageRefRow(p PageRef) string` → just `<slug>` (single column, since title/modified_at unavailable from descendants API)
- [ ] write tests for each operation (success + 4xx error paths) using `httptest`. Assert: GetPage URL contains `fields=content`, descendants paginates via cursor, UpdatePage does GET then POST in sequence.
- [ ] run tests — must pass before Task 8

### Task 8: Body input helper — flag pair handling

**Files:**

- Create: `internal/cli/body.go`
- Create: `internal/cli/body_test.go`

- [ ] define `type BodyInput struct { Body string; BodyFile string }` with kong tags `xor:"body"` so kong enforces mutual exclusivity
- [ ] add `func (b BodyInput) Read() (string, error)` — returns `Body` if set, reads file if `BodyFile` set (handles `-` as stdin via `os.Stdin`), errors if both empty
- [ ] write tests: inline body, file body, stdin via `-` (with redirected stdin), both-empty error
- [ ] run tests — must pass before Task 9

### Task 9: kong wiring — root + tracker + wiki commands

**Files:**

- Create: `internal/cli/cli.go` (kong CLI struct + dispatch)
- Modify: `cmd/yandex-cli/main.go` (use `internal/cli`)
- Create: `internal/cli/cli_test.go`

- [ ] define top-level CLI struct with nested subcommand structs:
  ```go
  type CLI struct {
    JSON bool `help:"emit JSON instead of plain text" name:"json"`
    Tracker struct {
      Issues struct {
        List ListIssuesCmd `cmd:"" help:"list issues"`
        Get  GetIssueCmd   `cmd:"" help:"get an issue"`
      } `cmd:""`
      Queues struct {
        List ListQueuesCmd `cmd:"" help:"list queues"`
        Get  GetQueueCmd   `cmd:"" help:"get a queue"`
      } `cmd:""`
    } `cmd:""`
    Wiki struct {
      Pages struct {
        List   ListPagesCmd   `cmd:"" help:"list pages"`
        Get    GetPageCmd     `cmd:"" help:"get a page"`
        Create CreatePageCmd  `cmd:"" help:"create a page"`
        Update UpdatePageCmd  `cmd:"" help:"update a page body"`
      } `cmd:""`
    } `cmd:""`
    Version VersionCmd `cmd:"" help:"print version"`
  }
  ```
- [ ] implement each command's `Run(ctx *kong.Context, parent *CLI)` method calling the corresponding client function and rendering via `internal/render`
- [ ] wire `--json` as a global persistent flag; pass to render layer
- [ ] for `create`/`update`: embed `BodyInput` from Task 8, call `Read()`
- [ ] propagate `context.Background()` (or signal-aware ctx) into client calls
- [ ] write tests: kong parses each command shape correctly; `--body` and `--body-file` mutual exclusion enforced; required-arg validation
- [ ] run tests — must pass before Task 10

### Task 10: End-to-end test — full binary against httptest mock

**Files:**

- Create: `internal/cli/e2e_test.go`

- [ ] table-driven test that invokes the kong parser with sample argv, points clients at `httptest.NewServer` mocks for both Tracker and Wiki, captures stdout/stderr, asserts exit code and output format (plain and JSON)
- [ ] cover at least: `tracker issues get FOO-1`, `tracker issues list --queue FOO`, `wiki pages get /some/slug`, `wiki pages create --slug X --title T --body B`, error case with auth failure
- [ ] run `go test ./...` — full suite must pass before Task 11

### Task 11: README — auth setup, fine-grained-token disclaimer, usage

**Files:**

- Modify: `README.md`

- [ ] sections:
  1. **What** — one-paragraph pitch
  2. **Install** — `go install github.com/butvinm/yandex-cli/cmd/yandex-cli@latest`, ensure `$GOBIN` on PATH
  3. **Setup** — install `yc` ([yandex.cloud/en/docs/cli/quickstart](https://yandex.cloud/en/docs/cli/quickstart)), `yc init`, `yc iam create-token`, `yc organization-manager organization list`, set env vars
  4. **HONEST DISCLAIMER on "fine-grained tokens"** — explicitly state that IAM tokens are NOT scope-limited, they inherit the user's full permissions; true scoped tokens require OAuth (deferred). Cite [yandex docs on api-key concepts](https://yandex.cloud/en/docs/iam/concepts/authorization/api-key) and OAuth scopes.
  5. **Token refresh** — IAM tokens are 12h max ([source](https://yandex.cloud/en/docs/iam/operations/iam-token/create-for-sa)); show `export YANDEX_TOKEN=$(yc iam create-token)` per session and `YANDEX_TOKEN=$(yc iam create-token) yandex-cli ...` per call
  6. **Usage** — all 8 commands with one example each
  7. **Limitations** — no 360, no OAuth, no attachments, Tracker write deferred. Contributions welcome.
  8. **Header inconsistency note** — for transparency, document why Tracker uses `X-Org-ID` and Wiki uses `X-Cloud-Org-Id` despite same tenancy
- [ ] verify all linked URLs resolve
- [ ] (no test step — docs)

### Task 12: Claude Code skill — SKILL.md

**Files:**

- Create: `.claude/skills/yandex/SKILL.md`

- [ ] frontmatter: `name: yandex`, `description: Read Yandex Tracker issues and queues; read and write Yandex Wiki pages. Use when the user asks to fetch issue details, list issues by queue, read a wiki page, or create/update wiki pages.`
- [ ] body sections:
  1. **Prerequisites** — `YANDEX_TOKEN` and `YANDEX_CLOUD_ORG_ID` must be set in the user's shell. If not, instruct user to read README.
  2. **Available commands** — bullet list of all 8 commands with one-line descriptions
  3. **Worked examples** (3):
     - "Read an issue": `yandex-cli tracker issues get FOO-1`
     - "List open issues in a queue": `yandex-cli tracker issues list --query "Queue: FOO and Status: !Closed"` (Tracker search API takes queue OR query, not both — combine via query language)
     - "Write a wiki page from a draft": `yandex-cli wiki pages create --slug team/notes/2026-04-29 --title "Notes" --body-file draft.md`
  4. **Output format note** — plain text by default, `--json` for parsing
  5. **Error handling** — non-zero exit means failure; `--json` flag emits structured error
- [ ] (no test step — markdown)

### Task 13: Verify acceptance criteria

- [ ] verify all 8 commands work against httptest mocks (covered by Task 10)
- [ ] verify `--json` works on every command
- [ ] verify error paths (missing env vars, bad token, 404s) emit clear messages in both plain and JSON modes
- [ ] verify `--body` and `--body-file` mutual exclusion (kong should reject both at once)
- [ ] verify `--body-file -` reads stdin
- [ ] verify `make install` puts binary on PATH
- [ ] run full test suite: `go test ./...`
- [ ] run `go vet ./...` and `golangci-lint run` (if installed)

### Task 14: [Final] Move plan to completed

- [ ] update README/CLAUDE.md if any patterns surfaced during implementation worth recording
- [ ] `mkdir -p docs/plans/completed && mv docs/plans/20260429-yandex-cli.md docs/plans/completed/`
- [ ] commit the move

---

## Technical Details

**Module path:** `github.com/butvinm/yandex-cli` (adjust if user has a different GitHub handle in mind — flagged for confirmation at Task 1)

**Go version:** 1.22+ (uses `cmp.Or` and stdlib `slog`)

**Dependencies (go.mod expected):**

- `github.com/alecthomas/kong` (CLI)
- stdlib only otherwise

**Pagination strategy:** clients fetch all pages internally and return aggregated slices. Acceptable for v1 because (a) Wiki page listings under a parent are typically small, (b) Tracker queue/queue-issue counts are bounded for personal use. If response sizes become a problem, add `--limit` flag in a follow-up.

**Error semantics:**

- Missing env var → exit 2, message to stderr ("YANDEX_TOKEN not set; run `export YANDEX_TOKEN=$(yc iam create-token)`")
- API 4xx → exit 1, body of API error message to stderr
- API 5xx → exit 1, "yandex API error <status>: <message>" to stderr
- `--json` flag changes stderr message format to JSON object

**Versioning:** linker-injected via `-ldflags "-X main.version=$(git describe ...)"`. `--version` prints once.

---

## Post-Completion

**Manual smoke-test (user runs after install):**

- Set env vars, run `yandex-cli tracker queues list` — should print queue list
- Run `yandex-cli wiki pages get <known-slug>` — should print page body
- Create a throwaway page with `yandex-cli wiki pages create --slug test/throwaway --title Test --body "hello"`, verify it appears in Yandex Wiki UI, delete via UI
- These steps belong to the user, not CI — CI uses mocks only.

**Future work (not in MVP):**

- OAuth + 360 tenancy support behind `YANDEX_TENANCY` env var
- Tracker writes (comment, transition, edit fields)
- Wiki attachments — requires implementing Upload Sessions API
- Pagination flags `--limit`, `--offset`
- Shell completions (`kong-completion`)

**Skill registration:** the user already has a Claude Code session. After commit, the SKILL.md will be loaded next session — no manual registration needed beyond having the file in `.claude/skills/yandex/`.
