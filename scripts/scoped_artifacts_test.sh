#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_TEST_FILE="${PROJECT_ROOT}/docker-compose.test.yml"
SETUP_E2E_SCRIPT="${PROJECT_ROOT}/scripts/setup-e2e.sh"
MIGRATION_FILE="${PROJECT_ROOT}/migrations/000029_add_tasks_status_created_at_index.up.sql"

fail() {
  echo "❌ $1"
  exit 1
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if grep -Fq -- "$pattern" "$file"; then
    fail "${description}: found '${pattern}' in ${file}"
  fi
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if ! grep -Fq -- "$pattern" "$file"; then
    fail "${description}: missing '${pattern}' in ${file}"
  fi
}

assert_not_matches() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if grep -Eq -- "$pattern" "$file"; then
    fail "${description}: pattern '${pattern}' matched in ${file}"
  fi
}

assert_not_contains "${COMPOSE_TEST_FILE}" "virtuestack_test_password" "test compose should not hardcode the database password"
assert_not_contains "${COMPOSE_TEST_FILE}" "virtuestack_nats_test_token" "test compose should not hardcode the NATS token"
assert_not_contains "${COMPOSE_TEST_FILE}" "test_jwt_secret_for_e2e_testing_minimum_32_chars" "test compose should not hardcode the JWT secret"
assert_not_contains "${COMPOSE_TEST_FILE}" "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" "test compose should not hardcode the encryption key"
assert_contains "${SETUP_E2E_SCRIPT}" "--env-file .env.test" "setup-e2e should run docker compose with the generated env file"
assert_not_contains "${SETUP_E2E_SCRIPT}" "postgresql://virtuestack:virtuestack_test_password@postgres:5432/virtuestack?sslmode=disable" "setup-e2e should not hardcode the migration database URL"
assert_not_matches "${MIGRATION_FILE}" "^[[:space:]]*CREATE INDEX CONCURRENTLY" "transactional migrations must avoid concurrent index creation"

echo "✅ scoped artifact tests passed"
