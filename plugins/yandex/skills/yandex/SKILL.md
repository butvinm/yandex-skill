---
name: yandex
description: Read Yandex Tracker issues and queues; read, search and write Yandex Wiki pages. Use when the user asks to fetch issue details, list issues by queue, read a wiki page, search the wiki, or create/update wiki pages. Supports both Yandex Cloud organization and Yandex 360 for Business organization types.
---

# Yandex Tracker + Wiki

This skill exposes 16 commands via the `yandex-cli` binary (must be on PATH).

## Prerequisites

These environment variables must be set in the user's shell:

- `YANDEX_TOKEN` — IAM token (Yandex Cloud) or OAuth token (Yandex 360). Optional on Cloud when `YANDEX_YC_PATH` is set (see below).
- `YANDEX_CLOUD_ORG_ID` — set this for a Yandex Cloud organization; OR
- `YANDEX_ORG_ID` — set this for Yandex 360 for Business

Organization type is inferred from which org-id var is set. Set exactly one.
See [Yandex Tracker → API access](https://yandex.ru/support/tracker/en/api-ref/access) for the underlying auth model.

Optional on Cloud: set `YANDEX_YC_PATH` to the path of your `yc` binary (e.g. `yc` for PATH lookup, or an absolute path) to make the binary mint an IAM token via `<yc> iam create-token` when `YANDEX_TOKEN` is unset. The token is cached on disk and refreshed every `YANDEX_IAM_TOKEN_REFRESH_PERIOD` hours (default 10, max 12). Requires an initialized `yc` profile. Has no effect on 360 (yc cannot mint OAuth tokens).

If any are missing, the CLI exits non-zero with a tenancy-specific hint in the error message. Direct the user to the project README for setup; do NOT try to set these yourself.

## Available commands

Tracker (read only):

- `yandex-cli tracker issues list --queue <KEY>` — list issues in a queue
- `yandex-cli tracker issues list --query '<Tracker query>'` — list issues by query language (combine filters: `Queue: FOO and Status: !Closed`)
- `yandex-cli tracker issues get <KEY>` - fetch one issue together with its links; the plain output has a `links:` block with one line per linked issue, `<linked-key> <relation> <KEY>  <status>  <assignee>  <title>`, read as a sentence (`BAR-3 Зависит от FOO-1` means BAR-3 depends on FOO-1). Check this block before concluding an issue has no related, blocking or parent tasks
- `yandex-cli tracker queues list` — list all queues
- `yandex-cli tracker queues get <KEY>` — fetch queue config
- `yandex-cli tracker comments list <KEY>` — list an issue's comments
- `yandex-cli tracker attachments list <KEY>` — list an issue's attachments
- `yandex-cli tracker attachments download <KEY> <id> [--output <path|->]` — download an issue attachment by id

Wiki pages (read + write):

- `yandex-cli wiki pages list --parent <slug>` — list child pages (full page URL + title) of a parent
- `yandex-cli wiki pages get <slug-or-url> [--output <path|->] [--attachments-dir <dir>]` — fetch a page; the first plain-output line is the page's full URL; with `--output`, write raw content to file (no URL/title prefix); with `--attachments-dir`, also download attachments and rewrite in-page URLs to local relative paths
- `yandex-cli wiki pages create --slug <new/path> --title <s> --body[-file] <s|path|-> [--attachments-dir <dir>]` — create a page; with `--attachments-dir`, upload local files referenced as `<dir>/<X>` and rewrite URLs to server form
- `yandex-cli wiki pages update <slug> --body[-file] <s|path|-> [--attachments-dir <dir>]` — replace page body; same `--attachments-dir` semantics as create
- `yandex-cli wiki search <query> [--limit <1-50>] [--order-by relevancy|creation_date|modified_date] [--type page|file]` - full-text search across the whole wiki; each row is `URL  [file]  title  modified_at  snippet` (default 10 hits, ranked by relevance)

Wiki attachments (read + write):

- `yandex-cli wiki attachments list <slug>` — list a page's attachments
- `yandex-cli wiki attachments upload <slug> --file <path> [--name <override>]` — upload a file (≤16 MiB)
- `yandex-cli wiki attachments download <slug> <filename> [--output <path>|-]` — stream binary content
- `yandex-cli wiki attachments delete <slug> <filename>` — remove an attachment

Body input: `--body "inline"` or `--body-file path/to/draft.md` or `--body-file -` (read from stdin). The two flags are mutually exclusive.

## Output format

Plain text by default — single-block format with blank-line separators that's easy to read in chat. Pass `--json` (after the binary, before the subcommand: `yandex-cli --json tracker ...`) when you need to parse the response.

Wiki page reads and writes surface the page's **full URL** (`https://wiki.yandex.ru/<slug>` by default; override with `YANDEX_WIKI_PUBLIC_URL`): `pages list` rows lead with it, `pages get` prints it as the first line, and `pages create`/`update` confirm with it. When you reference a wiki page back to the user in chat, cite that full URL, not the bare slug. Every slug argument (`pages get/update`, all `attachments` commands) also accepts a full page URL, so you can paste a URL straight back in.

Errors → stderr, non-zero exit. With `--json`, errors are JSON: `{"error":"...","status":<http-status>}`.

## Worked examples

### Read an issue

```sh
yandex-cli tracker issues get FOO-1
```

### List open issues in a queue

Use the Tracker query language (the search API takes one selector — combine filters via `query`, not by passing `--queue` alongside):

```sh
yandex-cli tracker issues list --query 'Queue: FOO and Status: !Closed'
```

### Find a wiki page by text

When the user doesn't know the slug, search first, then `pages get` the URL from the hit:

```sh
yandex-cli wiki search "deploy runbook" --limit 5 --type page
```

### Write a wiki page from a draft file

```sh
yandex-cli wiki pages create \
  --slug team/notes/2026-04-29 \
  --title "Notes from 2026-04-29" \
  --body-file draft.md
```

### Pipe LLM-generated content into a wiki page

```sh
echo "# Summary\n\nFoo" | yandex-cli wiki pages update team/notes/2026-04-29 --body-file -
```

### Attach a screenshot to a wiki page

```sh
yandex-cli wiki attachments upload team/notes/2026-04-29 \
  --file /tmp/screenshot.png
```

The filename defaults to the basename of `--file`; pass `--name diagram.png` to override.

### Save a wiki attachment locally

```sh
yandex-cli wiki attachments download team/notes/2026-04-29 diagram.png \
  --output ./diagram.png
```

Use `--output -` (default) to stream to stdout — pipe straight into another tool. Note that this is binary on most attachments; redirect to a file rather than printing in a terminal.

### Markdown round-trip (page + attachments together)

Use `--attachments-dir <dir>` to fetch a page with its attachments, edit locally, and push back without manually wiring URLs. URLs of the form `/<page-slug>/.files/<X>` ↔ `<dir>/<X>` are rewritten in both directions.

```sh
yandex-cli wiki pages get team/notes/2026-04-29 --output page.md --attachments-dir ./att
# edit page.md, drop new files into ./att, reference them as ./att/foo.png
yandex-cli wiki pages update team/notes/2026-04-29 --body-file page.md --attachments-dir ./att
```

`--attachments-dir` only operates safely on modern markdown (`page_type=wysiwyg`) pages. It refuses `grid` pages outright (structured tables, not markdown content) and warns on legacy `page` pages (the rewrite is usually a no-op on legacy syntax). Pages created via this CLI are always `wysiwyg`. Server-side attachments not referenced locally are NOT deleted — drift is one-directional.

## Limitations

- Tracker is read-only: issues, queues, comments and attachments can all be read, but nothing can be written (no posting comments, status transitions, or field edits)
- Wiki attachment uploads are single-part only (≤16 MiB) — chunked uploads not implemented
- `pages list` accepts `--parent` only - use `wiki search` to find pages by text
- Pagination is internal - list commands fetch in full; `wiki search` returns only the top `--limit` hits (max 50), there is no cursor to page further
- `--attachments-dir` does not delete server attachments that aren't referenced locally — use `wiki attachments delete` explicitly
