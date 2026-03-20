#!/usr/bin/env bash

set -euo pipefail

: "${RUN_URL:?RUN_URL is required}"
: "${E2E_AGENT:?E2E_AGENT is required}"
: "${TRIAGE_OUTPUT_FILE:?TRIAGE_OUTPUT_FILE is required}"

mkdir -p "$(dirname "$TRIAGE_OUTPUT_FILE")"

triage_args="/e2e:triage-ci ${RUN_URL} --agent ${E2E_AGENT}"
if [ -n "${TRIAGE_SHA:-}" ]; then
  triage_args="${triage_args} --sha ${TRIAGE_SHA}"
fi

repo="${GITHUB_REPOSITORY:-entireio/cli}"

claude \
  --plugin-dir .claude/plugins/e2e \
  --allowedTools \
    "Bash(scripts/download-e2e-artifacts.sh ${RUN_URL})" \
    "Bash(gh run view * --repo ${repo} --json *)" \
    "Bash(gh run download * --repo ${repo} --dir *)" \
    "Bash(gh run list --workflow e2e.yml --repo ${repo} *)" \
    "Bash(mise run test:e2e --agent ${E2E_AGENT} *)" \
    "Read" \
    "Grep" \
    "Glob" \
  -p "$triage_args" \
  2>&1 | tee "$TRIAGE_OUTPUT_FILE"
