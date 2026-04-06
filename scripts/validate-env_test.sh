#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SCRIPT_PATH="${REPO_ROOT}/scripts/validate-env.sh"

run_missing_vars_test() {
  set +e
  output="$(
    env -i bash -c '
      set -euo pipefail
      DATABASE_URL="postgresql://example" \
      NATS_URL="nats://localhost:4222" \
      JWT_SECRET="secret" \
      "'"${SCRIPT_PATH}"'"
    ' 2>&1
  )"
  status=$?
  set -e

  if [ "${status}" -eq 1 ]; then
    echo "✅ missing vars test passed"
  else
    echo "❌ missing vars test failed (status=${status})"
    echo "${output}"
    exit 1
  fi
}

run_valid_env_test() {
  set +e
  output="$(
    env -i bash -c '
      set -euo pipefail
      DATABASE_URL="postgresql://example" \
      NATS_URL="nats://localhost:4222" \
      NATS_AUTH_TOKEN="token" \
      JWT_SECRET="secret" \
      ENCRYPTION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
      GUEST_OP_HMAC_SECRET="12345678901234567890123456789012" \
      "'"${SCRIPT_PATH}"'"
    ' 2>&1
  )"
  status=$?
  set -e

  if [ "${status}" -eq 0 ]; then
    echo "✅ valid env test passed"
  else
    echo "❌ valid env test failed (status=${status})"
    echo "${output}"
    exit 1
  fi
}

run_missing_guest_op_secret_test() {
  set +e
  output="$(
    env -i bash -c '
      set -euo pipefail
      DATABASE_URL="postgresql://example" \
      NATS_URL="nats://localhost:4222" \
      NATS_AUTH_TOKEN="token" \
      JWT_SECRET="secret" \
      ENCRYPTION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
      "'"${SCRIPT_PATH}"'"
    ' 2>&1
  )"
  status=$?
  set -e

  if [ "${status}" -eq 1 ] && [[ "${output}" == *"GUEST_OP_HMAC_SECRET is required"* ]]; then
    echo "✅ missing guest op secret test passed"
  else
    echo "❌ missing guest op secret test failed (status=${status})"
    echo "${output}"
    exit 1
  fi
}

run_missing_vars_test
run_missing_guest_op_secret_test
run_valid_env_test

echo "✅ validate-env script tests passed"
