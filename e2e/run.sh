#!/bin/sh
# Run the Playwright E2E suite against an offline compose stack.
#
#   e2e/run.sh            build the runner, start the stack, run all tests
#   e2e/run.sh -- ARGS    pass ARGS to `playwright test` (e.g. -g "axe")
#
# The app image defaults to linkspage:e2e (`docker build -t linkspage:e2e .`);
# set E2E_APP_IMAGE to test another tag. Results: e2e/test-results and
# e2e/playwright-report; app logs: e2e/test-results/stack.log.
set -eu
cd "$(dirname "$0")"

if [ "${1:-}" = "--" ]; then shift; fi

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-linkspage-e2e}"
dc() { docker compose -f compose.yaml "$@"; }

cleanup() {
  status=$?
  dc logs --no-color db stub app > test-results/stack.log 2>&1 || true
  dc down -v --remove-orphans > /dev/null 2>&1 || true
  exit "$status"
}

# Pre-create the bind-mounted output folders so they belong to the caller.
rm -rf test-results playwright-report
mkdir -p test-results playwright-report
trap cleanup EXIT INT TERM

dc build playwright
dc up -d --wait --wait-timeout 180 app
dc run --rm --no-deps playwright pnpm exec playwright test "$@"
