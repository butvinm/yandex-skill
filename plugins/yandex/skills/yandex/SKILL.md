---
name: yandex
description: Read Yandex Tracker issues and queues; read and write Yandex Wiki pages. Use when the user asks to fetch issue details, list issues by queue, read a wiki page, or create/update wiki pages. Supports both Yandex Cloud organization and Yandex 360 for Business organization types.
---

# Yandex Tracker + Wiki

This skill exposes 8 commands via the `yandex-cli` binary (must be on PATH).

## Prerequisites

These environment variables must be set in the user's shell:

- `YANDEX_TOKEN` — IAM token (Yandex Cloud) or OAuth token (Yandex 360)
- `YANDEX_CLOUD_ORG_ID` — set this for a Yandex Cloud organization; OR
- `YANDEX_ORG_ID` — set this for Yandex 360 for Business

Organization type is inferred from which org-id var is set. Set exactly one.
See [Yandex Tracker → API access](https://yandex.ru/support/tracker/en/api-ref/access) for the underlying auth model.

If any are missing, the CLI exits non-zero with a tenancy-specific hint in the error message. Direct the user to the project README for setup; do NOT try to set these yourself.

## Available commands

Tracker (read only):

- `yandex-cli tracker issues list --queue <KEY>` — list issues in a queue
- `yandex-cli tracker issues list --query '<Tracker query>'` — list issues by query language (combine filters: `Queue: FOO and Status: !Closed`)
- `yandex-cli tracker issues get <KEY>` — fetch one issue
- `yandex-cli tracker queues list` — list all queues
- `yandex-cli tracker queues get <KEY>` — fetch queue config

Wiki (read + write):

- `yandex-cli wiki pages list --parent <slug>` — list child page slugs of a parent
- `yandex-cli wiki pages get <slug>` — fetch a page (title, modified time, body)
- `yandex-cli wiki pages create --slug <new/path> --title <s> --body[-file] <s|path|->` — create a page
- `yandex-cli wiki pages update <slug> --body[-file] <s|path|->` — replace page body

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

## Limitations

- No Tracker writes (no comments, transitions, edits)
- No Wiki attachments / image uploads
- No free-text search for Wiki — `pages list` accepts `--parent` only
- Pagination is internal — large result sets fetch in full
