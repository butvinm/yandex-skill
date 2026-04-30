# yc Token Fallback (opt-in)

## Overview

Let users obtain a Yandex Cloud IAM token automatically by shelling out to the `yc` CLI (`yc iam create-token`) when `YANDEX_TOKEN` is unset, gated on a new opt-in env var `YANDEX_USE_YC=1`. Removes the friction of manually running `export YANDEX_TOKEN=$(yc iam create-token)` before every session, while keeping the current behavior the default — users who script their own token retrieval are unaffected.

Scope is **Cloud tenancy only**. `yc` mints IAM tokens (Bearer); 360 uses OAuth tokens which `yc` does not provide. If `YANDEX_USE_YC=1` is set on a 360 tenancy, the flag is a no-op and the existing 360 error message stands.

Integrates as a small additive change in `internal/auth/`. The CLI Run pattern (`auth.Load()` → client → render) is unchanged.

## Context (from discovery)

- **Files involved:**
  - `internal/auth/auth.go` — `Load()` currently errors when `YANDEX_TOKEN` is empty (`auth.go:70-75`)
  - `internal/auth/auth_test.go` — covers env-var scenarios via `t.Setenv`
- **Reference impl (yandex-mcp `internal/adapters/ytoken/`):** uses `ICommandExecutor` interface + regex extraction + caching/single-flight. We adopt only the executor interface; the rest is over-engineered for a one-shot CLI.
- **`yc iam create-token` output (verified):** stdout = token + `\n` only. Warnings (e.g. CLI-init connectivity) go to stderr. **No regex extraction needed — `strings.TrimSpace(stdout)` is correct.** This is a deliberate simplification vs yandex-mcp.
- **Token refresh:** out of scope. Each CLI invocation is short-lived (sub-second to seconds); calling `yc` once per process is acceptable. `yc` itself caches the OAuth credential and only re-mints the IAM token per call (~200–500ms).
- **No on-disk token cache.** Adds file-permission, atomicity, and XDG-path concerns for marginal speedup.
- **Existing pattern to preserve:** env-var-presence-as-config (`YANDEX_CLOUD_ORG_ID` vs `YANDEX_ORG_ID` selects tenancy). The new `YANDEX_USE_YC` follows the same boolean-presence-via-`=1` convention as test scenarios already in the codebase.

## Development Approach

- **Testing approach:** Regular (code first, then tests in same task). Auth changes are mechanical; TDD adds friction without catching anything that the post-write tests won't.
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run `go test ./...` and `go vet ./...` after each change
- maintain backward compatibility: when `YANDEX_USE_YC` is unset, behavior is byte-identical to today

## Testing Strategy

- **unit tests** for the new `fetchYCToken` function via a swappable executor (no real `yc` invocation in tests)
- **unit tests** for `Load()` covering: opt-in disabled (current behavior), opt-in enabled + yc returns token, opt-in enabled + yc fails, opt-in enabled + 360 tenancy (must still error — yc doesn't help)
- **e2e tests:** none required. The `internal/cli/e2e_test.go` suite stubs HTTP servers; auth env-var setup happens per-test. We will _not_ invoke real `yc` from tests — the executor abstraction stops there.
- **manual smoke** with real `yc` after merge (see Post-Completion).

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, README/CLAUDE.md updates
- **Post-Completion** (no checkboxes): manual verification with a real `yc` profile

## Implementation Steps

### Task 1: Add executor abstraction and `fetchYCToken` helper

**Files:**

- Create: `internal/auth/yc.go`
- Create: `internal/auth/yc_test.go`

- [ ] in `internal/auth/yc.go`, define unexported interface `ycExecutor { runYC(ctx context.Context) ([]byte, error) }` and a default impl `realYCExecutor{}` that calls `exec.CommandContext(ctx, "yc", "iam", "create-token").Output()`
- [ ] add package-level `var ycExec ycExecutor = realYCExecutor{}` so tests can swap it
- [ ] add `func fetchYCToken(ctx context.Context) (string, error)` that calls `ycExec.runYC`, returns `strings.TrimSpace(string(out))`; on `*exec.ExitError`, include `strings.TrimSpace(string(ee.Stderr))` in the error so users see why `yc` failed; on empty-after-trim, return a distinct error
- [ ] in `internal/auth/yc_test.go`, write a fake executor (struct with `outFn func() ([]byte, error)`) and table tests covering: success (returns token), exit-error with stderr (error message contains stderr text), unknown error (e.g. `exec.ErrNotFound`-style), empty stdout (error mentions empty output)
- [ ] each test saves `prev := ycExec`, swaps in the fake, defers restore — no global leakage between tests
- [ ] run `go test ./internal/auth/...` and `go vet ./...` — must pass before Task 2

### Task 2: Wire opt-in fallback into `auth.Load`

**Files:**

- Modify: `internal/auth/auth.go`
- Modify: `internal/auth/auth_test.go`

- [ ] in `auth.go`, add constant `envUseYC = "YANDEX_USE_YC"` near the other env-var constants
- [ ] in `Load()`, in the existing `if c.Token == ""` block: keep the 360 branch unchanged; in the Cloud branch, if `os.Getenv(envUseYC) == "1"`, call `fetchYCToken(context.Background())` — on success set `c.Token` and continue; on error wrap as `fmt.Errorf("%s unset and yc fallback failed: %w", envToken, err)` so the user sees both why fallback was attempted and what `yc` reported
- [ ] keep the existing "run: export YANDEX_TOKEN=$(yc iam create-token)" hint when `YANDEX_USE_YC` is _not_ set (current behavior preserved verbatim)
- [ ] in `auth_test.go`, add test cases (each saves/restores `ycExec`):
  - Cloud + `YANDEX_TOKEN=""` + `YANDEX_USE_YC` unset → existing error returned (regression guard)
  - Cloud + `YANDEX_TOKEN=""` + `YANDEX_USE_YC=1` + fake exec returns `"t1.abc\n"` → `c.Token == "t1.abc"`, no error
  - Cloud + `YANDEX_TOKEN=""` + `YANDEX_USE_YC=1` + fake exec returns error → `Load` returns wrapped error mentioning `yc fallback failed`
  - 360 + `YANDEX_TOKEN=""` + `YANDEX_USE_YC=1` → still returns the existing 360-OAuth error (yc must NOT be invoked); assert the fake executor was not called
  - Cloud + `YANDEX_TOKEN="t1.x"` + `YANDEX_USE_YC=1` → user-set token wins, fake executor not called
- [ ] run `go test ./...` and `go vet ./...` — must pass before Task 3

### Task 3: Document the env var

**Files:**

- Modify: `README.md`
- Modify: `CLAUDE.md` (only if a non-obvious convention emerged)

- [ ] add `YANDEX_USE_YC` to the env-var section of README with a one-paragraph note: opt-in, Cloud-only, requires `yc` on PATH and an initialized profile, mention that the existing manual approach still works
- [ ] only update `CLAUDE.md` if a new project convention was introduced (e.g. test-time executor swapping) that future contributors need to know — otherwise leave it alone (the file is intentionally terse)
- [ ] run `go test ./...` once more end-to-end

### Task 4: Verify acceptance criteria

- [ ] `YANDEX_USE_YC` unset → all existing tests still pass (no regression)
- [ ] `YANDEX_USE_YC=1` + Cloud + no `YANDEX_TOKEN` → `Load()` returns a populated config when fake yc returns a token
- [ ] `YANDEX_USE_YC=1` + 360 → 360 OAuth error, `yc` not invoked
- [ ] `go vet ./...` clean, `go test ./...` green
- [ ] move this plan to `docs/plans/completed/` (`mkdir -p` not needed — directory exists)

## Technical Details

### New file `internal/auth/yc.go` (sketch — not committed yet)

```go
package auth

import (
    "context"
    "errors"
    "fmt"
    "os/exec"
    "strings"
)

type ycExecutor interface {
    runYC(ctx context.Context) ([]byte, error)
}

type realYCExecutor struct{}

func (realYCExecutor) runYC(ctx context.Context) ([]byte, error) {
    return exec.CommandContext(ctx, "yc", "iam", "create-token").Output()
}

var ycExec ycExecutor = realYCExecutor{}

func fetchYCToken(ctx context.Context) (string, error) {
    out, err := ycExec.runYC(ctx)
    if err != nil {
        var ee *exec.ExitError
        if errors.As(err, &ee) {
            stderr := strings.TrimSpace(string(ee.Stderr))
            if stderr != "" {
                return "", fmt.Errorf("yc iam create-token: %s", stderr)
            }
        }
        return "", fmt.Errorf("running yc iam create-token: %w", err)
    }
    tok := strings.TrimSpace(string(out))
    if tok == "" {
        return "", errors.New("yc iam create-token returned empty output")
    }
    return tok, nil
}
```

### Patch to `Load()` in `auth.go` (sketch)

Replace the current `if c.Token == ""` block with:

```go
if c.Token == "" {
    if tenancy == Y360 {
        return Config{}, errors.New(envToken + " not set; for 360, get an OAuth token at oauth.yandex.com")
    }
    if os.Getenv(envUseYC) == "1" {
        tok, err := fetchYCToken(context.Background())
        if err != nil {
            return Config{}, fmt.Errorf("%s unset and yc fallback failed: %w", envToken, err)
        }
        c.Token = tok
    } else {
        return Config{}, errors.New(envToken + " not set; run: export " + envToken + "=$(yc iam create-token), or set " + envUseYC + "=1 to call yc automatically")
    }
}
```

Notes:

- `context.Background()` is intentional. CLI invocations are short-lived; piping a real `ctx` would require changing `Load()`'s signature, which is a bigger refactor outside this task's scope. If a future timeout requirement appears, revisit.
- The 360 branch is **before** the `YANDEX_USE_YC` check on purpose — yc cannot mint OAuth tokens, so silently invoking it on 360 would fail confusingly.

### Why no regex / no caching / no single-flight

- `yc iam create-token` output (verified locally with `yc` 0.198.0): stdout is exactly the token + `\n`, nothing else. Warnings go to stderr.
- One CLI invocation = one `Load()` call. No concurrency inside one process.
- `yc` already caches the OAuth credential; per-invocation cost is acceptable.

## Post-Completion

**Manual verification** (after merge, on a machine with `yc` installed and initialized):

- `YANDEX_USE_YC=1 YANDEX_CLOUD_ORG_ID=<real> ./bin/yandex-cli tracker queues list` succeeds without `YANDEX_TOKEN` set.
- `YANDEX_USE_YC=1 YANDEX_CLOUD_ORG_ID=<real> ./bin/yandex-cli tracker queues list` with a deliberately broken `yc` (e.g. `PATH` missing yc, or `yc config unset` to break the profile) returns a clear error mentioning `yc fallback failed` and includes yc's stderr.
- Without `YANDEX_USE_YC`, the existing failure mode is byte-identical (regression check).

**External system updates**: none. No consuming projects, no deployment changes.
