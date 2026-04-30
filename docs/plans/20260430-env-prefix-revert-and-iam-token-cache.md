# Env Prefix Revert + IAM Token Cache

## Overview

Two coupled changes to the auth model:

1. **Revert env-var prefix from `YANDEX_CLI_` back to `YANDEX_`**, deliberately aligning with [n-r-w/yandex-mcp](https://github.com/n-r-w/yandex-mcp) so users running both tools share a single set of env vars.
2. **Cache the IAM token from `yc` on disk** with a configurable refresh period, instead of shelling out to `yc iam create-token` on every CLI invocation.

### Reversals (read these before reviewing)

Both changes contradict explicit decisions made in earlier commits/plans on this same branch. The reversals are deliberate and the rationale is recorded here so future-us doesn't re-revert.

- **Rename reverses commit `ad5e80f`** ("refactor(auth)!: rename env vars to YANDEX*CLI* prefix"), whose stated motivation was to **avoid** collision with yandex-mcp's generic `YANDEX_` prefix. The new framing is the opposite: collision is desirable. yandex-mcp uses exactly `YANDEX_CLOUD_ORG_ID`, `YANDEX_TRACKER_BASE_URL`, `YANDEX_WIKI_BASE_URL` (verified via README fetch). If yandex-cli adopts the same names, a user running both tools sets each variable once. Cost: any user who has already set `YANDEX_CLI_*` vars on this branch must rename. Pre-1.0, no consumers; acceptable.
- **Caching reverses the explicit "out of scope / marginal speedup" decision** in `docs/plans/completed/20260430-yc-token-fallback.md:18-19`. The previous framing assumed one CLI invocation per process is the typical case. New framing: LLM-driven workflows (Claude Code skill, etc.) generate 5-15 invocations per session, and `yc iam create-token` measured at ~200-500ms per call. The cumulative shellout cost is meaningful. The XDG-path / file-permission concerns flagged in the previous plan are addressable: `os.UserCacheDir()` for cross-platform location, mode `0600` on the file, atomic write via `os.Rename`.

### What changes for users

| Old (this branch's HEAD)        | New                                                           |
| ------------------------------- | ------------------------------------------------------------- |
| `YANDEX_CLI_TOKEN`              | `YANDEX_TOKEN`                                                |
| `YANDEX_CLI_CLOUD_ORG_ID`       | `YANDEX_CLOUD_ORG_ID`                                         |
| `YANDEX_CLI_ORG_ID`             | `YANDEX_ORG_ID`                                               |
| `YANDEX_CLI_TRACKER_BASE_URL`   | `YANDEX_TRACKER_BASE_URL`                                     |
| `YANDEX_CLI_WIKI_BASE_URL`      | `YANDEX_WIKI_BASE_URL`                                        |
| `YANDEX_CLI_USE_YC`             | `YANDEX_USE_YC`                                               |
| (no cache; shellout every call) | Cache file at `os.UserCacheDir()/yandex-cli/iam-token.json`   |
| (no refresh period)             | `YANDEX_IAM_TOKEN_REFRESH_PERIOD` (hours, default 10, max 12) |

`YANDEX_TOKEN` set explicitly always wins and bypasses the cache entirely. 360 tenancy is unchanged (yc cannot mint OAuth tokens; cache logic is only entered on the `YANDEX_USE_YC=1` path).

## Context (from discovery)

- **Files involved (from grep — ~123 references):**
  - `internal/auth/auth.go` — env-var constants + `Load()` flow
  - `internal/auth/auth_test.go` — env-var-driven test cases via `t.Setenv`
  - `internal/auth/yc.go` — `ycExecutor` interface + `fetchYCToken`
  - `internal/auth/yc_test.go` — fake executor pattern
  - `internal/cli/cli_test.go`, `internal/cli/e2e_test.go` — set env vars per test
  - `README.md` — config table at lines ~65-75
  - `plugins/yandex/skills/yandex/SKILL.md` — env var list (already has uncommitted edits)
  - `CLAUDE.md` — terse, only update if a new convention emerges (it will: file-based cache)
- **Existing test patterns to reuse:**
  - **Executor swap** (`internal/auth/yc.go:21` — `var ycExec ycExecutor = realYCExecutor{}`): tests save `prev := ycExec`, set fake, defer restore. Same pattern works for clock and cache-dir if needed.
  - `t.Setenv` for per-test env-var isolation.
  - `t.TempDir()` for filesystem isolation in cache tests.
- **`os.UserCacheDir()` behavior (verified at <https://pkg.go.dev/os#UserCacheDir>):**
  - Linux: `$XDG_CACHE_HOME` or `~/.cache`
  - macOS: `~/Library/Caches`
  - Windows: `%LocalAppData%`
  - Returns error only if home dir cannot be determined — extremely rare. Treat as fatal.
- **n-r-w/yandex-mcp's refresh-period semantics (verified via README fetch):** `YANDEX_IAM_TOKEN_REFRESH_PERIOD` in **hours**, default **10**, max **12** (Yandex IAM tokens are valid for ≤12h). We adopt the same env var name and bounds for cross-tool consistency.

## Development Approach

- **Testing approach**: Regular (code first, then tests in same task). Matches the previous yc plan; auth changes are mechanical and TDD adds friction without catching anything that post-write tests won't.
- complete each task fully before moving to the next
- make small, focused changes — each task is independently committable
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run `go test ./...` and `go vet ./...` after each change
- atomic commits — never `git add .`, stage specific files only (per CLAUDE.md)
- **no compatibility shim** for the rename. Pre-1.0 branch, no published consumers. The skill plugin is in this same repo and gets updated in lockstep.

## Testing Strategy

- **unit tests** for cache read/write (success, missing file, malformed JSON, permission denied, expired vs fresh)
- **unit tests** for `Load()` covering matrix:
  - Cache hit (fresh) → no shellout
  - Cache miss → shellout, write cache, return token
  - Cache expired → shellout, overwrite cache, return token
  - Malformed cache → treat as miss
  - `YANDEX_TOKEN` set → bypass cache entirely (cache file untouched)
  - 360 tenancy with `YANDEX_USE_YC=1` → cache logic NOT entered, existing 360 error stands
  - `YANDEX_IAM_TOKEN_REFRESH_PERIOD` parse: valid, invalid string, > 12 (clamp to 12), ≤ 0 (use default)
- **e2e tests** in `internal/cli/e2e_test.go`: rename env-var references; no new e2e cases for cache (e2e suite stubs HTTP, doesn't exercise auth flow deeply).
- **manual smoke** with real `yc` after merge (see Post-Completion).

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, README/SKILL.md/CLAUDE.md updates
- **Post-Completion** (no checkboxes): manual verification with a real `yc` profile

## Implementation Steps

### Task 1: Rename env-var prefix from `YANDEX_CLI_` to `YANDEX_` (mechanical, no behavior change)

**Files:**

- Modify: `internal/auth/auth.go`
- Modify: `internal/auth/auth_test.go`
- Modify: `internal/auth/yc.go` (no env-var refs but verify)
- Modify: `internal/auth/yc_test.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `internal/cli/e2e_test.go`
- Modify: `README.md`
- Modify: `plugins/yandex/skills/yandex/SKILL.md`
- Modify: `CLAUDE.md` (if it mentions specific env-var names — check)

- [ ] in `internal/auth/auth.go`, rename constants:
  - `envToken` value: `YANDEX_CLI_TOKEN` → `YANDEX_TOKEN`
  - `envCloudOrgID` value: `YANDEX_CLI_CLOUD_ORG_ID` → `YANDEX_CLOUD_ORG_ID`
  - `envOrgID` value: `YANDEX_CLI_ORG_ID` → `YANDEX_ORG_ID`
  - `envTrackerBaseURL` value: `YANDEX_CLI_TRACKER_BASE_URL` → `YANDEX_TRACKER_BASE_URL`
  - `envWikiBaseURL` value: `YANDEX_CLI_WIKI_BASE_URL` → `YANDEX_WIKI_BASE_URL`
  - `envUseYC` value: `YANDEX_CLI_USE_YC` → `YANDEX_USE_YC`
- [ ] update the error message strings in `Load()` that mention the old names verbatim
- [ ] update all `t.Setenv("YANDEX_CLI_…", …)` calls in `auth_test.go`, `yc_test.go`, `cli_test.go`, `e2e_test.go` to the new names
- [ ] update `README.md` config table (currently has uncommitted edits — preserve those edits, only swap names)
- [ ] update `plugins/yandex/skills/yandex/SKILL.md` env var list (currently has uncommitted edits — preserve those edits, only swap names)
- [ ] grep `YANDEX_CLI_` across the repo (`grep -rn "YANDEX_CLI_" --include="*.go" --include="*.md" .`) — must return zero hits
- [ ] run `go test ./...` and `go vet ./...` — must pass before Task 2
- [ ] commit: `refactor(auth)!: align env-var names with yandex-mcp` — message body explains the reversal of `ad5e80f`

### Task 2: Add disk cache for IAM token (read/write/expiry)

**Files:**

- Create: `internal/auth/cache.go`
- Create: `internal/auth/cache_test.go`

- [ ] in `internal/auth/cache.go`, define struct `cachedToken { Token string; AcquiredAt time.Time }` with JSON tags (`acquired_at` for the timestamp field)
- [ ] add `cacheFilePath() (string, error)` returning `filepath.Join(os.UserCacheDir(), "yandex-cli", "iam-token.json")`. Error wraps `os.UserCacheDir`'s error.
- [ ] add `readCachedToken(path string) (cachedToken, error)`:
  - reads file, json-unmarshals
  - returns wrapped error on missing file (`os.IsNotExist`-detectable so callers can distinguish), malformed JSON, or empty token field
- [ ] add `writeCachedToken(path string, ct cachedToken) error`:
  - `os.MkdirAll(filepath.Dir(path), 0700)`
  - write to `path + ".tmp"` with mode `0600`, then `os.Rename` to `path` for atomicity
  - returns wrapped error on any failure
- [ ] add `func (ct cachedToken) isFresh(refreshPeriod time.Duration, now time.Time) bool` — returns `now.Sub(ct.AcquiredAt) < refreshPeriod`
- [ ] in `cache_test.go`, table tests using `t.TempDir()` for path isolation:
  - `writeCachedToken` then `readCachedToken` round-trips
  - `readCachedToken` on missing file returns error matching `os.IsNotExist`
  - `readCachedToken` on malformed JSON returns error
  - `readCachedToken` on empty token field returns error
  - `writeCachedToken` creates parent dir with mode `0700` and file with mode `0600` (verify via `os.Stat`)
  - `isFresh` returns true when age < period, false when age == period or age > period
- [ ] run `go test ./internal/auth/...` and `go vet ./...` — must pass before Task 3

### Task 3: Add refresh-period parsing with bounds checking

**Files:**

- Modify: `internal/auth/cache.go`
- Modify: `internal/auth/cache_test.go`

- [ ] add constant `envRefreshPeriod = "YANDEX_IAM_TOKEN_REFRESH_PERIOD"` and `defaultRefreshHours = 10`, `maxRefreshHours = 12` in `cache.go`
- [ ] add `parseRefreshPeriod(envValue string) time.Duration`:
  - empty string → `defaultRefreshHours * time.Hour`
  - parses as integer hours via `strconv.Atoi`
  - on parse error, returns default (do NOT error — env-var typos shouldn't break the CLI; document this)
  - clamps to `[1, maxRefreshHours]`. Values ≤ 0 fall back to default. Values > 12 clamp to 12 silently (matches Yandex's hard limit; users shouldn't be forced to error-recover from a config-file typo).
- [ ] in `cache_test.go`, table tests for `parseRefreshPeriod`:
  - `""` → 10h
  - `"5"` → 5h
  - `"12"` → 12h
  - `"100"` → 12h (clamped)
  - `"0"` → 10h (default)
  - `"-3"` → 10h (default)
  - `"abc"` → 10h (default)
- [ ] run `go test ./internal/auth/...` — must pass before Task 4

### Task 4: Wire cache into `Load()` flow

**Files:**

- Modify: `internal/auth/auth.go`
- Modify: `internal/auth/auth_test.go`

- [ ] in `auth.go`, factor the `YANDEX_USE_YC=1` branch into a new helper `loadYCToken(ctx context.Context) (string, error)`:
  - resolve cache path via `cacheFilePath()`; on error, log via `fmt.Errorf` and proceed without cache (don't fail the whole Load — disk-cache failure is non-fatal degradation)
  - read cache; if hit AND `isFresh(parseRefreshPeriod(os.Getenv(envRefreshPeriod)), time.Now())` → return cached token, no shellout
  - else call `fetchYCToken(ctx)`. On success, attempt `writeCachedToken`; if write fails, log to stderr but still return the token (cache write is best-effort, never blocks the user)
  - on `fetchYCToken` error, return wrapped error matching the existing format
- [ ] inject a clock seam: package-level `var nowFn = time.Now` so tests can freeze time. Same pattern as `ycExec`.
- [ ] inject a cache-dir seam: package-level `var cacheDirFn = os.UserCacheDir` so tests can redirect to a tmpdir. Same pattern.
- [ ] in `Load()`, replace the existing `if os.Getenv(envUseYC) == "1" { tok, err := fetchYCToken(...) }` block with `tok, err := loadYCToken(context.Background())`
- [ ] in `auth_test.go`, add cases (each saves/restores `ycExec`, `nowFn`, `cacheDirFn`):
  - **Cache hit (fresh)**: pre-populate cache file via `writeCachedToken` with `AcquiredAt = now - 1h`; fake exec configured to FAIL if called; assert returned token matches cached, fake exec was not invoked
  - **Cache miss**: empty tmpdir; fake exec returns `"t1.fresh\n"`; assert returned token == `"t1.fresh"`, cache file now exists with that token and `AcquiredAt ≈ now`
  - **Cache expired**: pre-populate cache with `AcquiredAt = now - 11h`; refresh period default (10h); fake exec returns `"t1.new\n"`; assert returned token == `"t1.new"`, cache overwritten
  - **Custom refresh period**: `t.Setenv("YANDEX_IAM_TOKEN_REFRESH_PERIOD", "1")`; cache `AcquiredAt = now - 30min`; fake exec configured to FAIL if called; assert cache hit (30min < 1h)
  - **Malformed cache**: write garbage bytes to cache file; fake exec returns `"t1.recover\n"`; assert returned token == `"t1.recover"`, cache overwritten with valid JSON
  - **`YANDEX_TOKEN` set**: cache file present with token `"t1.cached"`; env `YANDEX_TOKEN=t1.env`; fake exec configured to FAIL if called; assert `c.Token == "t1.env"` AND cache file is unchanged (mtime/contents)
  - **360 tenancy + `YANDEX_USE_YC=1`**: cache file present (Cloud cache); 360 env vars set; assert existing 360 OAuth error returned, fake exec not invoked, cache untouched
  - **Cache write fails (read-only dir)**: `cacheDirFn` returns a path under a read-only parent; fake exec returns `"t1.transient\n"`; assert returned token == `"t1.transient"` (write failure is non-fatal)
- [ ] run `go test ./...` and `go vet ./...` — must pass before Task 5

### Task 5: Update documentation

**Files:**

- Modify: `README.md`
- Modify: `plugins/yandex/skills/yandex/SKILL.md`
- Modify: `CLAUDE.md`

- [ ] `README.md`: add `YANDEX_IAM_TOKEN_REFRESH_PERIOD` row to the config table; update the `YANDEX_USE_YC` row to mention disk caching at `os.UserCacheDir()/yandex-cli/iam-token.json` and the refresh window
- [ ] `SKILL.md`: one-sentence note that the binary caches the IAM token and re-runs `yc` periodically
- [ ] `CLAUDE.md`: add a single bullet to "Things not to do" / scope: "Auth state on disk is limited to the IAM-token cache at `os.UserCacheDir()/yandex-cli/`. Do not extend this to OAuth tokens, refresh tokens, or org-id without an explicit ask." — this is a new convention worth recording.
- [ ] run `go test ./...` once more end-to-end

### Task 6: Verify acceptance criteria

- [ ] `grep -rn "YANDEX_CLI_" --include="*.go" --include="*.md" .` returns zero hits
- [ ] All env-var references in tests use the new names
- [ ] `YANDEX_TOKEN` unset + `YANDEX_USE_YC=1` + Cloud + empty cache → first call shells out and writes cache; second call reads cache (verified by fake exec configured to fail on second call)
- [ ] Cache expiry triggers re-mint (verified by frozen clock and cache age > refresh period)
- [ ] 360 path unchanged (verified by existing 360 test still passing)
- [ ] `go vet ./...` clean, `go test ./...` green
- [ ] move this plan to `docs/plans/completed/`

## Technical Details

### Cache file format

```json
{ "token": "t1.<full IAM token>", "acquired_at": "2026-04-30T12:34:56Z" }
```

- `token` — the IAM token string as returned by `yc iam create-token` (already trimmed)
- `acquired_at` — RFC3339 timestamp, always UTC. Used to compute age.

We do not store the refresh period in the file, because the period is read from env at each `Load()` call. This means a user can shorten the refresh period and immediately invalidate older cached tokens, which is the right semantics for a debugging knob.

### Why no file lock / single-flight

Two concurrent `yandex-cli` invocations could both miss the cache, both shellout, both write — last writer wins. Outcomes:

- Both writers' tokens are valid (yc mints fresh tokens each call). Whichever wins the rename race becomes the cached token; the loser's token is used by its own process and discarded after.
- No corruption (atomic rename via tmp-file).
- Worst case: redundant `yc` shellout on the cold-cache concurrent-startup edge. Acceptable. A file lock would add complexity for a CLI that almost never runs concurrently against itself.

### Why refresh-period default is 10h, not 12h

Yandex IAM tokens expire at exactly 12h. Refreshing at 10h leaves a 2h safety margin so that if a CLI invocation starts at the boundary, the token has not expired by the time it reaches the API. Matches n-r-w/yandex-mcp's default.

### Why `parseRefreshPeriod` swallows errors instead of returning them

`Load()` is the entry point for every command. If a user has a typo in `YANDEX_IAM_TOKEN_REFRESH_PERIOD` (e.g. `=12h` instead of `=12`), failing the whole CLI is hostile when a sane default is available. The cost of "your typo silently fell back to default" is low (token refreshes happen invisibly); the cost of "every command errors with a parse error until you find the typo" is high. Document the silent fallback in the README.

### What stays out of scope

- **Org-id auto-inference from `yc organization-manager organization list`.** Discussed earlier in this branch — multi-org users (verified: this user has 2) make it impossible to do unambiguously. User still sets `YANDEX_CLOUD_ORG_ID` manually.
- **Filesystem locking / single-flight.** See above.
- **In-memory cache for repeated `Load()` calls within a single process.** Currently `Load()` is called once per CLI invocation; if a future change adds multiple `Load()` calls, revisit.
- **Cache invalidation on yc profile change.** If the user runs `yc config profile activate <other>`, our cache is now stale (different identity). We don't detect this. Acceptable: tokens still validate against the old identity until the API rejects them, and the user can `rm` the cache file to force a refresh. Document the cache-file path so users can clear it.

## Post-Completion

**Manual verification** (after merge, on a machine with `yc` installed and initialized):

- `YANDEX_USE_YC=1 YANDEX_CLOUD_ORG_ID=<real> ./bin/yandex-cli tracker queues list` succeeds; `~/.cache/yandex-cli/iam-token.json` is created with mode 0600
- Inspect the file: contains valid JSON with token + `acquired_at`
- Run the same command again immediately: succeeds; `mtime` of the cache file is unchanged (cache hit)
- `touch -d "13 hours ago" ~/.cache/yandex-cli/iam-token.json` then re-run: succeeds; cache file is overwritten (`acquired_at` updates)
  - Note: the on-disk `mtime` only triggers a refresh because we re-read `acquired_at` from the JSON. To force a refresh more directly: edit the JSON `acquired_at` field to a past timestamp.
- Set `YANDEX_TOKEN=$(yc iam create-token)` and run again: succeeds; cache file is **not** modified
- Unset `YANDEX_USE_YC` and `YANDEX_TOKEN`: existing error message is byte-identical to before this PR (regression check)
- 360 tenancy: `YANDEX_ORG_ID=<real> YANDEX_USE_YC=1 ./bin/yandex-cli ...` returns the existing 360 OAuth error; cache file is **not** created

**External system updates**: none. The skill plugin's `SKILL.md` lives in this same repo and is updated in lockstep with the rename.
