# yandex-cli

A small Go CLI for **Yandex Tracker** (read) and **Yandex Wiki** (read + write), packaged as a Claude Code skill in `.claude/skills/yandex/`. Plain text by default, `--json` for parsing, token from env, no extra dependencies beyond [kong](https://github.com/alecthomas/kong).

## What it does

8 commands across two products:

| Command                                                                 | API                                            |
| ----------------------------------------------------------------------- | ---------------------------------------------- |
| `yandex-cli tracker issues list --queue FOO`                            | POST `/v3/issues/_search`                      |
| `yandex-cli tracker issues list --query '...'`                          | POST `/v3/issues/_search`                      |
| `yandex-cli tracker issues get FOO-1`                                   | GET `/v3/issues/{key}`                         |
| `yandex-cli tracker queues list`                                        | GET `/v3/queues/`                              |
| `yandex-cli tracker queues get FOO`                                     | GET `/v3/queues/{key}`                         |
| `yandex-cli wiki pages list --parent <slug>`                            | GET `/v1/pages/descendants`                    |
| `yandex-cli wiki pages get <slug>`                                      | GET `/v1/pages?slug=...&fields=content`        |
| `yandex-cli wiki pages create --slug ... --title ... --body[-file] ...` | POST `/v1/pages`                               |
| `yandex-cli wiki pages update <slug> --body[-file] ...`                 | POST `/v1/pages/{id}` (slug→id resolved first) |

## Install

### 1. Build the binary

```sh
go install github.com/butvinm/yandex-cli/cmd/yandex-cli@latest
```

Make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if `GOBIN` is unset) is on your `PATH`.

### 2. Install the Claude Code skill

The skill lives at `.claude/skills/yandex/SKILL.md` in this repo. Two ways to use it:

**Project-local (auto-discovered):** clone this repo and run `claude` from inside it. The skill is loaded automatically.

**Globally available across all projects:** symlink it into your user skills directory:

```sh
mkdir -p ~/.claude/skills
ln -s "$(pwd)/.claude/skills/yandex" ~/.claude/skills/yandex
```

### 3. Verify

After auth setup (see below), the binary should respond:

```sh
yandex-cli tracker queues list      # should print all queues
yandex-cli wiki pages get <known/slug>  # should print page body
```

From Claude Code, ask: _"list my Yandex Tracker queues"_ — Claude should auto-invoke the `yandex` skill and call the binary. Or list active skills via `/help`.

## Auth setup

This MVP supports **Yandex Cloud Organization** tenancy only. Yandex 360 Business is not supported yet.

### One-time

1. Install [`yc`](https://yandex.cloud/en/docs/cli/quickstart) and authenticate:

   ```sh
   yc init
   ```

2. Find your organization id:

   ```sh
   yc organization-manager organization list
   ```

3. Export it once (e.g. in `~/.zshrc`):

   ```sh
   export YANDEX_CLOUD_ORG_ID=<your-org-id>
   ```

### Per session

```sh
export YANDEX_TOKEN=$(yc iam create-token)
```

Or per call:

```sh
YANDEX_TOKEN=$(yc iam create-token) yandex-cli tracker issues get FOO-1
```

IAM tokens last at most 12 hours ([source](https://yandex.cloud/en/docs/iam/operations/iam-token/create-for-sa)) — refresh whenever you start a fresh session. Note that IAM tokens carry your full account permissions; they are not scope-limited.

## Environment variables

| Variable                  | Required | Default                          |
| ------------------------- | -------- | -------------------------------- |
| `YANDEX_TOKEN`            | yes      | —                                |
| `YANDEX_CLOUD_ORG_ID`     | yes      | —                                |
| `YANDEX_TRACKER_BASE_URL` | no       | `https://api.tracker.yandex.net` |
| `YANDEX_WIKI_BASE_URL`    | no       | `https://api.wiki.yandex.net`    |

## Output

Plain text by default with blank-line block separation, optimized for LLM consumption and shell pipes:

```
$ yandex-cli tracker issues get FOO-1
FOO-1: write the cli
Open  ivan  2026-04-29T10:00:00Z
Description goes here.
```

`--json` flag is available for all commands and enables structured output:

```sh
yandex-cli --json tracker issues get FOO-1
```

Errors go to stderr with non-zero exit. With `--json`, errors are JSON: `{"error":"...","status":<http>}`.

## Limitations

- **No Yandex 360 Business tenancy** (OAuth required, deferred)
- **No Tracker writes** (no comments, no transitions, no edits)
- **No Wiki attachments / image uploads**
- **No pagination flags** — clients fetch all pages internally
- **Wiki has no free-text search** — `wiki pages list` accepts `--parent` only

## Contributions

Welcome — especially OAuth support for Yandex 360, attachments, and Tracker writes.
