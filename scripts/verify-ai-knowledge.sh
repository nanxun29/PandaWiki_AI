#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir/backend"
go build ./...
go test -count=1 ./config ./domain ./pkg/knowledge ./handler/mq ./handler/v1 ./usecase ./mq/nats ./utils

if [[ -z "${PANDAWIKI_TEST_DSN:-}" ]]; then
  printf '%s\n' 'PostgreSQL/HTTP integration tests were skipped: set PANDAWIKI_TEST_DSN to a disposable PostgreSQL URL.'
fi
if [[ -z "${PANDAWIKI_TEST_NATS:-}" ]]; then
  printf '%s\n' 'NATS integration test was skipped: set PANDAWIKI_TEST_NATS to an isolated JetStream server.'
fi

if [[ "${1:-}" == "--frontend" ]]; then
  cd "$repo_dir/web"
  pnpm api
  pnpm --dir admin exec tsc -b
  pnpm --dir admin exec eslint src/pages/knowledge-agent src/request/knowledgeAgent.ts
fi
