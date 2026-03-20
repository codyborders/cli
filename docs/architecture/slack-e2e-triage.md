# Slack-Triggered E2E Triage

When E2E tests fail on `main`, a Slack alert is posted with a clickable "Run Triage" link. Clicking it triggers the triage workflow via a Cloudflare Worker.

## Flow

1. `.github/workflows/e2e.yml` posts a failure alert to Slack using the bot token (via `chat.postMessage`), then posts a threaded "Run Triage" link that encodes the run URL and Slack thread context.
2. A user clicks the link, which hits the Cloudflare Worker at `e2e-triage.entireio.workers.dev`.
3. The Worker validates the `run_url` and calls `workflow_dispatch` on `.github/workflows/e2e-triage.yml` via the GitHub API.
4. The triage workflow checks out the failed SHA, runs the Claude triage skill per failed agent, and posts results back to the Slack thread.

```
E2E fails -> bot posts alert to Slack (with "Run Triage" link)
  -> user clicks link -> Cloudflare Worker -> GitHub API (workflow_dispatch)
    -> e2e-triage.yml runs -> posts results back to Slack thread
```

## Cloudflare Worker

Source lives in the infra repo at `cloudflare/workers/e2e-triage-trigger/`.

Accepts a GET request at `/triage` with query parameters:

- `run_url` (required) — must match `https://github.com/entireio/cli/actions/runs/\d+`
- `slack_channel` — Slack channel ID for thread replies
- `slack_thread_ts` — Slack thread timestamp for thread replies

The Worker dispatches `e2e-triage.yml` with these values as `workflow_dispatch` inputs.

**Secret:** `GITHUB_TOKEN` — a PAT with `actions:write` scope, stored in Cloudflare secrets (`wrangler secret put GITHUB_TOKEN`).

## Slack Setup

The Slack app needs:

- `chat:write` scope — to post alerts and triage results
- Bot must be invited to the alert channel

No event subscriptions or incoming webhooks are needed.

## GitHub Config

**Repository variables:**

- `E2E_SLACK_CHANNEL` — Slack channel ID where failure alerts are posted

**Repository secrets:**

- `SLACK_BOT_TOKEN` — Slack bot token with `chat:write` scope
- `ANTHROPIC_API_KEY` — for Claude triage

The built-in `${{ github.token }}` is used for GitHub API calls within workflows.

## Manual Fallback

Run `.github/workflows/e2e-triage.yml` manually with `workflow_dispatch`:

- `run_url` (required) — the failed run URL
- `sha` — commit SHA (auto-detected from run if omitted)
- `failed_agents` — comma-separated list (auto-detected from run if omitted)
- `slack_channel` — for Slack thread replies
- `slack_thread_ts` — for Slack thread replies
