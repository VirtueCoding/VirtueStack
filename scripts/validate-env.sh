#!/usr/bin/env bash
set -euo pipefail

ERRORS=0

check_required() {
  local var_name="$1"
  local value="${!var_name:-}"
  if [ -z "${value}" ]; then
    echo "❌ ${var_name} is required"
    ERRORS=$((ERRORS + 1))
  fi
}

check_hex() {
  local var_name="$1"
  local expected_len="$2"
  local value="${!var_name:-}"

  if [ -z "${value}" ]; then
    return
  fi

  if ! [[ "${value}" =~ ^[0-9a-fA-F]+$ ]]; then
    echo "❌ ${var_name} must be a hex string"
    ERRORS=$((ERRORS + 1))
    return
  fi

  if [ "${#value}" -ne "${expected_len}" ]; then
    echo "❌ ${var_name} must be ${expected_len} hex characters"
    ERRORS=$((ERRORS + 1))
  fi
}

check_int() {
  local var_name="$1"
  local value="${!var_name:-}"

  if ! [[ "${value}" =~ ^[1-9][0-9]*$ ]]; then
    echo "❌ ${var_name} must be a positive integer"
    ERRORS=$((ERRORS + 1))
  fi
}

check_prefix() {
  local var_name="$1"
  local prefix="$2"
  local value="${!var_name:-}"

  if [ -n "${value}" ] && [[ "${value}" != "${prefix}"* ]]; then
    echo "❌ ${var_name} must start with ${prefix}"
    ERRORS=$((ERRORS + 1))
  fi
}

check_required "DATABASE_URL"
check_required "NATS_URL"
check_required "NATS_AUTH_TOKEN"
check_required "JWT_SECRET"
check_required "ENCRYPTION_KEY"
check_required "GUEST_OP_HMAC_SECRET"
check_hex "ENCRYPTION_KEY" 64

# Validate optional values when present.
check_prefix "REDIS_URL" "redis://"
if [ -n "${SMTP_PORT:-}" ]; then
  check_int "SMTP_PORT"
fi

if [ "${ERRORS}" -gt 0 ]; then
  echo "❌ ${ERRORS} configuration error(s) found"
  exit 1
fi

echo "✅ All configuration validated"
