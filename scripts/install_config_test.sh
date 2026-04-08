#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

fail() {
  echo "❌ $*"
  exit 1
}

assert_file_contains() {
  local file="$1"
  local expected="$2"

  if ! grep -Fq "$expected" "$file"; then
    echo "Expected to find: $expected"
    echo "--- file: $file ---"
    cat "$file"
    fail "assert_file_contains failed"
  fi
}

assert_file_not_contains() {
  local file="$1"
  local unexpected="$2"

  if grep -Fq "$unexpected" "$file"; then
    echo "Unexpectedly found: $unexpected"
    echo "--- file: $file ---"
    cat "$file"
    fail "assert_file_not_contains failed"
  fi
}

run_controller_env_passthrough_test() {
  local controller_block

  controller_block="$(awk '
    /^  controller:/ { in_block=1 }
    in_block && /^  [a-z0-9-]+:/ && $1 != "controller:" && NR > 1 { exit }
    in_block { print }
  ' "${REPO_ROOT}/docker-compose.yml")"

  echo "${controller_block}" | grep -Fq "env_file:" || fail "controller service must declare env_file"
  echo "${controller_block}" | grep -Fq -- "- .env" || fail "controller service must load .env through env_file"
  grep -Fq 'NATS_URL: nats://${NATS_AUTH_TOKEN}@nats:4222' "${REPO_ROOT}/docker-compose.prod.yml" || fail "prod compose must authenticate controller to NATS"
}

run_admin_seed_schema_test() {
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "INSERT INTO admins ("
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "id, email, password_hash, name,"
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "totp_secret_encrypted, totp_enabled"
  assert_file_not_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "INSERT INTO admins (id, email, password_hash, role, status"
}

run_installation_docs_schema_test() {
  assert_file_contains "${REPO_ROOT}/docs/installation.md" "INSERT INTO admins (email, password_hash, name, role, totp_enabled, totp_secret_encrypted, created_at)"
  assert_file_not_contains "${REPO_ROOT}/docs/installation.md" "INSERT INTO admins (id, email, password_hash, role, status, created_at, updated_at)"
  assert_file_contains "${REPO_ROOT}/docs/installation.md" "docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build"
}

run_installer_split_docs_test() {
  assert_file_contains "${REPO_ROOT}/README.md" 'The controller stack (`controller`, `nats`, `postgres`, `admin-webui`, `customer-webui`, and `nginx`) stays in Docker.'
  assert_file_contains "${REPO_ROOT}/README.md" 'The node installer writes `/etc/virtuestack/node-agent.env` and installs the `virtuestack-node-agent` systemd service.'
  assert_file_contains "${REPO_ROOT}/docs/installation.md" 'The controller stack (`controller`, `nats`, `postgres`, `admin-webui`, `customer-webui`, and `nginx`) remains Docker-based.'
  assert_file_contains "${REPO_ROOT}/docs/installation.md" 'The node installer copies your existing mTLS certificate, key, and CA files into `/etc/virtuestack/certs/`, writes `/etc/virtuestack/node-agent.env`, installs `/etc/systemd/system/virtuestack-node-agent.service`, and starts `virtuestack-node-agent`.'
}

run_e2e_seed_quality_test() {
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "INSERT INTO customers ("
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "id, email, password_hash, name, external_client_id, billing_provider, auth_provider,"
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "status, created_at, updated_at"
  assert_file_not_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "INSERT INTO customers (id, email, password_hash, status, created_at, updated_at)"
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "pnpm install --frozen-lockfile --ignore-scripts"
  assert_file_contains "${REPO_ROOT}/scripts/setup-e2e.sh" "pnpm exec playwright install --with-deps"
  assert_file_contains "${REPO_ROOT}/.github/workflows/e2e.yml" "pnpm install --frozen-lockfile --ignore-scripts"
}

run_controller_env_passthrough_test
run_admin_seed_schema_test
run_installation_docs_schema_test
run_installer_split_docs_test
run_e2e_seed_quality_test

echo "✅ install config tests passed"
