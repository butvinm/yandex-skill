# yandex-skill

A Claude Code skill for **Yandex Tracker** and **Yandex Wiki**, backed by a small `yandex-cli` Go binary.

## What it does

8 commands across two products.

**Tracker**

- `yandex-cli tracker issues list [--queue FOO | --query '...']` — list issues
- `yandex-cli tracker issues get FOO-1` — get one issue
- `yandex-cli tracker queues list` — list queues
- `yandex-cli tracker queues get FOO` — get one queue

**Wiki**

- `yandex-cli wiki pages list --parent <slug>` — list descendants of a page
- `yandex-cli wiki pages get <slug>` — get page content
- `yandex-cli wiki pages create --slug ... --title ... --body[-file] ...` — create a page
- `yandex-cli wiki pages update <slug> --body[-file] ...` — update a page

## Setup

### Install the binary

```sh
go install github.com/butvinm/yandex-skill/cmd/yandex-cli@latest
```

Make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if `GOBIN` is unset) is on your `PATH`.

### Install the Claude Code plugin

In a Claude Code session:

```
/plugin marketplace add butvinm/yandex-skill
/plugin install yandex@butvinm-yandex-skill
```

### Auth

For how to obtain tokens and find your organization id, see the official docs: [Yandex Tracker → API access](https://yandex.ru/support/tracker/en/api-ref/access).

Once you have a token and org id, set the env vars below (full list under [Configuration](#environment-variables)). Which org-id var you set selects the organization type:

- `YANDEX_CLOUD_ORG_ID` → **Yandex Cloud organization** (expects an IAM token in `YANDEX_TOKEN`)
- `YANDEX_ORG_ID` → **Yandex 360 for Business** (expects an OAuth token in `YANDEX_TOKEN`)

Set exactly one — both at once is rejected.

For Yandex Cloud, you can let the CLI mint an IAM token automatically by opting in to the `yc` fallback: set `YANDEX_USE_YC=1` and leave `YANDEX_TOKEN` unset. The binary will run `yc iam create-token` on each invocation — requires the [Yandex Cloud CLI](https://yandex.cloud/en/docs/cli/quickstart) on `PATH` and an initialized profile (`yc init`). 360 tenancies are unaffected (yc cannot mint OAuth tokens).

### Verify

The binary should respond:

```sh
yandex-cli tracker queues list      # should print all queues
yandex-cli wiki pages get <known/slug>  # should print page body
```

From Claude Code, ask: _"list my Yandex Tracker queues"_ — Claude should auto-invoke the `yandex` skill and call the binary. Or list active skills via `/skills`.

## Configuration

| Variable                  | Required                                | Default                          | Notes                                                                                                |
| ------------------------- | --------------------------------------- | -------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `YANDEX_TOKEN`            | yes (unless `YANDEX_USE_YC=1` on Cloud) | —                                | IAM token (Yandex Cloud) or OAuth token (Yandex 360)                                                 |
| `YANDEX_CLOUD_ORG_ID`     | one of these is needed                  | —                                | Yandex Cloud organization id; presence selects this org type                                         |
| `YANDEX_ORG_ID`           | one of these is needed                  | —                                | Yandex 360 for Business organization id; presence selects this                                       |
| `YANDEX_USE_YC`           | no                                      | unset                            | Set to `1` (Cloud only) to fetch an IAM token via `yc iam create-token` when `YANDEX_TOKEN` is unset |
| `YANDEX_TRACKER_BASE_URL` | no                                      | `https://api.tracker.yandex.net` |                                                                                                      |
| `YANDEX_WIKI_BASE_URL`    | no                                      | `https://api.wiki.yandex.net`    |                                                                                                      |

## Output

Plain text by default, optimized for LLM consumption and shell pipes:

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

- **No Tracker writes** (no comments, no transitions, no edits)
- **No Wiki attachments / image uploads**
- **No pagination flags** — clients fetch all pages internally
- **Wiki has no free-text search** — `wiki pages list` accepts `--parent` only
