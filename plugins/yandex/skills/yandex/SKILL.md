---
name: yandex
description: Read Yandex Tracker issues and queues; read and write Yandex Wiki pages. Use when the user asks to fetch issue details, list issues by queue, read a wiki page, or create/update wiki pages. Supports both Yandex Cloud organization and Yandex 360 for Business organization types.
---

# Yandex Tracker + Wiki

This skill exposes 12 commands via the `yandex-cli` binary (must be on PATH).

## Prerequisites

These environment variables must be set in the user's shell:

- `YANDEX_TOKEN` — IAM token (Yandex Cloud) or OAuth token (Yandex 360). Optional on Cloud when `YANDEX_USE_YC=1` (see below).
- `YANDEX_CLOUD_ORG_ID` — set this for a Yandex Cloud organization; OR
- `YANDEX_ORG_ID` — set this for Yandex 360 for Business

Organization type is inferred from which org-id var is set. Set exactly one.
See [Yandex Tracker → API access](https://yandex.ru/support/tracker/en/api-ref/access) for the underlying auth model.

Optional on Cloud: `YANDEX_USE_YC=1` makes the binary mint an IAM token via `yc iam create-token` when `YANDEX_TOKEN` is unset. The token is cached on disk and refreshed every `YANDEX_IAM_TOKEN_REFRESH_PERIOD` hours (default 10, max 12). Requires the `yc` CLI on PATH and an initialized profile. Has no effect on 360 (yc cannot mint OAuth tokens).

If any are missing, the CLI exits non-zero with a tenancy-specific hint in the error message. Direct the user to the project README for setup; do NOT try to set these yourself.

## Available commands

Tracker (read only):

- `yandex-cli tracker issues list --queue <KEY>` — list issues in a queue
- `yandex-cli tracker issues list --query '<Tracker query>'` — list issues by query language (combine filters: `Queue: FOO and Status: !Closed`)
- `yandex-cli tracker issues get <KEY>` — fetch one issue
- `yandex-cli tracker queues list` — list all queues
- `yandex-cli tracker queues get <KEY>` — fetch queue config

Wiki pages (read + write):

- `yandex-cli wiki pages list --parent <slug>` — list child page slugs of a parent
- `yandex-cli wiki pages get <slug>` — fetch a page (title, modified time, body)
- `yandex-cli wiki pages create --slug <new/path> --title <s> --body[-file] <s|path|->` — create a page
- `yandex-cli wiki pages update <slug> --body[-file] <s|path|->` — replace page body

Wiki attachments (read + write):

- `yandex-cli wiki attachments list <slug>` — list a page's attachments
- `yandex-cli wiki attachments upload <slug> --file <path> [--name <override>]` — upload a file (≤16 MiB)
- `yandex-cli wiki attachments download <slug> <filename> [--output <path>|-]` — stream binary content
- `yandex-cli wiki attachments delete <slug> <filename>` — remove an attachment

Body input: `--body "inline"` or `--body-file path/to/draft.md` or `--body-file -` (read from stdin). The two flags are mutually exclusive.

## Output format

Plain text by default — single-block format with blank-line separators that's easy to read in chat. Pass `--json` (after the binary, before the subcommand: `yandex-cli --json tracker ...`) when you need to parse the response.

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

## Limitations

- No Tracker writes (no comments, transitions, edits)
- Wiki attachment uploads are single-part only (≤16 MiB) — chunked uploads not implemented
- No free-text search for Wiki — `pages list` accepts `--parent` only
- Pagination is internal — large result sets fetch in full
