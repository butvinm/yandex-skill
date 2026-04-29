# yandex-cli

A small Go CLI for **Yandex Tracker** (read) and **Yandex Wiki** (read + write), distributed as a Claude Code plugin (`/plugin install yandex@butvinm-yandex-cli`). Plain text by default, `--json` for parsing, token from env, no extra dependencies beyond [kong](https://github.com/alecthomas/kong).

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

### 2. Install the Claude Code plugin

In a Claude Code session:

```
/plugin marketplace add butvinm/yandex-cli
/plugin install yandex@butvinm-yandex-cli
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

The CLI supports both **Yandex Cloud Organization** and **Yandex 360 for Business** tenancies. Tenancy is selected by which org-id env var you set:

- `YANDEX_CLOUD_ORG_ID` set → Cloud (uses IAM `Bearer` token + `X-Cloud-Org-ID` header)
- `YANDEX_ORG_ID` set → 360 (uses OAuth token + `X-Org-ID` header)

Set exactly one — both at once is rejected.

### Cloud Organization (IAM token via `yc`)

1. Install [`yc`](https://yandex.cloud/en/docs/cli/quickstart) and authenticate:

   ```sh
   yc init
   ```

2. Find your Cloud Organization id and export it (e.g. in `~/.zshrc`):

   ```sh
   export YANDEX_CLOUD_ORG_ID=$(yc organization-manager organization list --format json | jq -r '.[0].id')
   ```

3. Per session, refresh the IAM token:

   ```sh
   export YANDEX_TOKEN=$(yc iam create-token)
   ```

   IAM tokens last at most 12 hours ([source](https://yandex.cloud/en/docs/iam/operations/iam-token/create-for-sa)). They carry your full account permissions; they are not scope-limited.

### Yandex 360 for Business (OAuth token)

1. Get an OAuth token:
   - Register an app at [oauth.yandex.com](https://oauth.yandex.com/) (one-time)
   - Pick scopes: `tracker:read` for Tracker; `wiki:read` and `wiki:write` for Wiki
   - Visit `https://oauth.yandex.com/authorize?response_type=token&client_id=<your-client-id>` in a browser
   - Copy the token from the redirect URL fragment
   - Export it:

   ```sh
   export YANDEX_TOKEN=<oauth-token>
   ```

   OAuth tokens last ≥1 year and respect the scopes you selected at app registration.

2. Find your 360 organization id at **Yandex Tracker → Administration → Organizations** ([source](https://yandex.ru/support/wiki/en/api-ref/access)) and export:

   ```sh
   export YANDEX_ORG_ID=<your-360-org-id>
   ```

## Environment variables

| Variable                  | Required               | Default                          | Notes                                            |
| ------------------------- | ---------------------- | -------------------------------- | ------------------------------------------------ |
| `YANDEX_TOKEN`            | yes                    | —                                | IAM (Cloud) or OAuth (360)                       |
| `YANDEX_CLOUD_ORG_ID`     | one of these is needed | —                                | Cloud Organization id; presence selects Cloud    |
| `YANDEX_ORG_ID`           | one of these is needed | —                                | Yandex 360 organization id; presence selects 360 |
| `YANDEX_TRACKER_BASE_URL` | no                     | `https://api.tracker.yandex.net` |                                                  |
| `YANDEX_WIKI_BASE_URL`    | no                     | `https://api.wiki.yandex.net`    |                                                  |

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
