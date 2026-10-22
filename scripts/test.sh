#!/usr/bin/env bash
# Full local test run: lint, Go unit, frontend unit, and (when the dev
# services answer) integration tests. Results go to stdout for TEST_REPORT.md.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

make lint
make test-go
make test-web
if (exec 3<>/dev/tcp/127.0.0.1/13306) 2>/dev/null; then
  make test-integration
else
  echo "SKIPPED integration tests: dev MariaDB not reachable on 127.0.0.1:13306 (run make dev-db)" >&2
fi
