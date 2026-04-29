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

```sh
go install github.com/butvinm/yandex-cli/cmd/yandex-cli@latest
```

Make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin` if `GOBIN` is unset) is on your `PATH`.

## Auth setup

This MVP supports **Yandex Cloud Organization** tenancy only. Yandex 360 Business is not supported yet (would require OAuth — see [Limitations](#limitations)).

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

IAM tokens last **at most 12 hours** ([source](https://yandex.cloud/en/docs/iam/operations/iam-token/create-for-sa)) — refresh whenever you start a fresh session.

## Honest disclaimer about "fine-grained tokens"

If you read about "fine-grained" Yandex tokens elsewhere, that almost always means **OAuth tokens with explicit scopes** (e.g. `tracker:read`, `wiki:write`). This CLI uses **IAM tokens**, which are **not scope-limited** — they inherit the user identity's full permissions. If your account can edit any wiki page, the IAM token can too.

True fine-grained scopes require the OAuth flow, which we don't support yet (it's friction for personal use). If your threat model needs scoped tokens, run on a dedicated user with restricted permissions, or wait for OAuth support.

References:

- [IAM token concepts](https://yandex.cloud/en/docs/iam/concepts/authorization/iam-token)
- [API key concepts](https://yandex.cloud/en/docs/iam/concepts/authorization/api-key) (service-account-only; **Wiki blocks service accounts**, so API keys are not viable here)
- [Yandex OAuth scopes](https://yandex.com/dev/id/doc/en/concepts/ya-oauth-intro)

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

`--json` is universal:

```sh
yandex-cli --json tracker issues get FOO-1
```

Errors go to stderr with non-zero exit. With `--json`, errors are JSON: `{"error":"...","status":<http>}`.

## Header inconsistency note

For transparency: the same Yandex Cloud Organization is referenced by **different headers** in the two APIs:

- Tracker uses `X-Org-ID` ([source](https://yandex.ru/support/tracker/en/concepts/access))
- Wiki uses `X-Cloud-Org-Id` ([source](https://yandex.ru/support/wiki/en/api-ref/access))

The CLI hard-codes the right header per product. You set `YANDEX_CLOUD_ORG_ID` once.

## Limitations

- **No Yandex 360 Business tenancy** (OAuth required, deferred)
- **No Tracker writes** (no comments, no transitions, no edits)
- **No Wiki attachments / image uploads**
- **No pagination flags** — clients fetch all pages internally
- **Wiki has no free-text search** — `wiki pages list` accepts `--parent` only

## Verifying install

After install + env setup:

```sh
yandex-cli tracker queues list      # should print all queues
yandex-cli wiki pages get <known/slug>  # should print page body
```

## Contributions

Welcome — especially OAuth support for Yandex 360, attachments, and Tracker writes.
