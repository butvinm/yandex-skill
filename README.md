# yandex-skill

A Claude Code skill for **Yandex Tracker** and **Yandex Wiki**, backed by a small `yandex-cli` Go binary.

## What it does

12 commands across two products.

**Tracker**

- `yandex-cli tracker issues list [--queue FOO | --query '...']` — list issues
- `yandex-cli tracker issues get FOO-1` — get one issue
- `yandex-cli tracker queues list` — list queues
- `yandex-cli tracker queues get FOO` — get one queue

**Wiki**

- `yandex-cli wiki pages list --parent <slug>` — list descendants (slug + title) of a page
- `yandex-cli wiki pages get <slug> [--output PATH|-] [--attachments-dir DIR]` — get page content (and optionally sync attachments + rewrite links for local round-trip)
- `yandex-cli wiki pages create --slug ... --title ... --body[-file] ... [--attachments-dir DIR]` — create a page (uploads any attachments referenced as `<DIR>/<file>` first)
- `yandex-cli wiki pages update <slug> --body[-file] ... [--attachments-dir DIR]` — update a page (same attachment sync as create)
- `yandex-cli wiki attachments list <slug>` — list a page's attachments
- `yandex-cli wiki attachments upload <slug> --file PATH [--name NAME]` — upload (≤16 MiB)
- `yandex-cli wiki attachments download <slug> <filename> [--output PATH|-]` — stream binary
- `yandex-cli wiki attachments delete <slug> <filename>` — remove an attachment

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

For Yandex Cloud, you can let the CLI mint an IAM token automatically by opting in to the `yc` fallback: set `YANDEX_USE_YC=1` and leave `YANDEX_TOKEN` unset. The binary will run `yc iam create-token`, then cache the result at `os.UserCacheDir()/yandex-cli/iam-token.json` (mode 0600) and re-use it on subsequent invocations until it's older than `YANDEX_IAM_TOKEN_REFRESH_PERIOD` hours (default 10, max 12). Requires the [Yandex Cloud CLI](https://yandex.cloud/en/docs/cli/quickstart) on `PATH` and an initialized profile (`yc init`). 360 tenancies are unaffected (yc cannot mint OAuth tokens). To force a re-mint, delete the cache file.

### Verify

The binary should respond:

```sh
yandex-cli tracker queues list      # should print all queues
yandex-cli wiki pages get <known/slug>  # should print page body
```

From Claude Code, ask: _"list my Yandex Tracker queues"_ — Claude should auto-invoke the `yandex` skill and call the binary. Or list active skills via `/skills`.

## Configuration

| Variable                          | Required                                | Notes                                                                                                                                                                   |
| --------------------------------- | --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `YANDEX_TOKEN`                    | yes (unless `YANDEX_USE_YC=1` on Cloud) | IAM token (Yandex Cloud) or OAuth token (Yandex 360)                                                                                                                    |
| `YANDEX_CLOUD_ORG_ID`             | one of these is needed                  | Yandex Cloud organization id; presence selects this org type                                                                                                            |
| `YANDEX_ORG_ID`                   | one of these is needed                  | Yandex 360 for Business organization id; presence selects this                                                                                                          |
| `YANDEX_USE_YC`                   | no                                      | Set to `1` (Cloud only) to fetch an IAM token via `yc iam create-token` when `YANDEX_TOKEN` is unset; result is cached at `os.UserCacheDir()/yandex-cli/iam-token.json` |
| `YANDEX_IAM_TOKEN_REFRESH_PERIOD` | no                                      | Cache lifetime in hours (default `10`, clamped to `12`). Only consulted when `YANDEX_USE_YC=1`. Invalid values silently fall back to default.                           |
| `YANDEX_TRACKER_BASE_URL`         | no                                      | Default: `https://api.tracker.yandex.net`                                                                                                                               |
| `YANDEX_WIKI_BASE_URL`            | no                                      | Default: `https://api.wiki.yandex.net`                                                                                                                                  |

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

## Markdown round-trip

`--attachments-dir DIR` on `pages get/create/update` syncs attachments alongside the markdown body and rewrites in-page URLs between the server form (`/<page-slug>/.files/<file>`) and a local relative form (`<DIR>/<file>`). Use it for "fetch a page locally, edit, push back" workflows where attachments should travel with the markdown.

```sh
# fetch page + all its attachments to local dir, write rewritten markdown to file
yandex-cli wiki pages get team/notes/2026-04-29 --output page.md --attachments-dir ./att

# edit page.md, add ./att/new.png, then push back — new.png is uploaded automatically
yandex-cli wiki pages update team/notes/2026-04-29 --body-file page.md --attachments-dir ./att
```

Behavior depends on the page's `page_type` (which the API exposes per page):

| page_type | with `--attachments-dir`                                                                                 |
| --------- | -------------------------------------------------------------------------------------------------------- |
| `wysiwyg` | full feature (modern Yandex Flavored Markdown pages)                                                     |
| `page`    | warning to stderr, proceeds (legacy "static markup" — content is not markdown, rewrite is usually no-op) |
| `grid`    | refused (dynamic table; structured data, not markdown)                                                   |

Attachments not referenced in the markdown body are still downloaded by `get` (some pages keep them in the sidebar) and are not deleted by `update` (drift is one-directional).

## Limitations

- **No Tracker writes** (no comments, no transitions, no edits)
- **Wiki attachment uploads are single-part only** — files larger than 16 MiB are rejected. The Yandex Wiki upload-sessions API supports chunked uploads up to ~160 GB; we ship single-part to keep the client lean.
- **No pagination flags** — clients fetch all pages internally
- **Wiki has no free-text search** — `wiki pages list` accepts `--parent` only
- **`--attachments-dir` does not delete server attachments** that aren't referenced locally; use `wiki attachments delete` explicitly
