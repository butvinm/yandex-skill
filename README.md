# yandex-skill

A Claude Code skill for **Yandex Tracker** (read) and **Yandex Wiki** (read + write), backed by a small `yandex-cli` Go binary.

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
go install github.com/butvinm/yandex-skill/cmd/yandex-cli@latest
```

Make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if `GOBIN` is unset) is on your `PATH`.

### 2. Install the Claude Code plugin

In a Claude Code session:

```
/plugin marketplace add butvinm/yandex-skill
/plugin install yandex@butvinm-yandex-skill
```

To test the plugin without installing it (e.g. during development of this repo):

```sh
claude --plugin-dir plugins/yandex
```

### 3. Verify

After auth setup (see below), the binary should respond:

```sh
yandex-cli tracker queues list      # should print all queues
yandex-cli wiki pages get <known/slug>  # should print page body
```

From Claude Code, ask: _"list my Yandex Tracker queues"_ — Claude should auto-invoke the `yandex` skill and call the binary. Or list active skills via `/skills`.

## Auth setup

For how to obtain tokens and find your organization id, see the official docs: [Yandex Tracker → API access](https://yandex.ru/support/tracker/en/api-ref/access).

Once you have a token and org id, set the env vars below. Which org-id var you set selects the organization type:

- `YANDEX_CLOUD_ORG_ID` → **Yandex Cloud organization** (expects an IAM token in `YANDEX_TOKEN`)
- `YANDEX_ORG_ID` → **Yandex 360 for Business** (expects an OAuth token in `YANDEX_TOKEN`)

Set exactly one — both at once is rejected.

## Environment variables

| Variable                  | Required               | Default                          | Notes                                                          |
| ------------------------- | ---------------------- | -------------------------------- | -------------------------------------------------------------- |
| `YANDEX_TOKEN`            | yes                    | —                                | IAM token (Yandex Cloud) or OAuth token (Yandex 360)           |
| `YANDEX_CLOUD_ORG_ID`     | one of these is needed | —                                | Yandex Cloud organization id; presence selects this org type   |
| `YANDEX_ORG_ID`           | one of these is needed | —                                | Yandex 360 for Business organization id; presence selects this |
| `YANDEX_TRACKER_BASE_URL` | no                     | `https://api.tracker.yandex.net` |                                                                |
| `YANDEX_WIKI_BASE_URL`    | no                     | `https://api.wiki.yandex.net`    |                                                                |

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

- **No Tracker writes** (no comments, no transitions, no edits)
- **No Wiki attachments / image uploads**
- **No pagination flags** — clients fetch all pages internally
- **Wiki has no free-text search** — `wiki pages list` accepts `--parent` only

## Contributions

Welcome — especially attachments and Tracker writes.
