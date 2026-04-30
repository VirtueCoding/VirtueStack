#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_PATH="${REPO_ROOT}/install.sh"

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

assert_output_contains() {
  local file="$1"
  local expected="$2"

  if ! grep -Fq "$expected" "$file"; then
    echo "Expected output to contain: $expected"
    echo "--- output: $file ---"
    cat "$file"
    fail "assert_output_contains failed"
  fi
}

run_controller_letsencrypt_test() {
  local workdir install_dir output log_file env_file status

  workdir="$(mktemp -d)"
  install_dir="${workdir}/install"
  output="${workdir}/controller-letsencrypt.out"
  log_file="${workdir}/controller-letsencrypt.commands"
  env_file="${install_dir}/.env"
  mkdir -p "${workdir}/home"

  set +e
  env -i \
    HOME="${workdir}/home" \
    PATH="${PATH}" \
    TERM="${TERM:-xterm}" \
    VIRTUESTACK_TEST_MODE=1 \
    VIRTUESTACK_NONINTERACTIVE=1 \
    VIRTUESTACK_SOURCE_DIR="${REPO_ROOT}" \
    VIRTUESTACK_INSTALL_DIR="${install_dir}" \
    VIRTUESTACK_COMMAND_LOG="${log_file}" \
    VIRTUESTACK_DOMAIN="panel.example.com" \
    VIRTUESTACK_USE_LETSENCRYPT="true" \
    VIRTUESTACK_ADMIN_EMAIL="admin@example.com" \
    VIRTUESTACK_ADMIN_NAME="Example Admin" \
    VIRTUESTACK_ADMIN_PASSWORD="ExampleAdminPass123!" \
    VIRTUESTACK_LE_EMAIL="ops@example.com" \
    bash "${SCRIPT_PATH}" --controller >"${output}" 2>&1
  status=$?
  set -e

  if [ "${status}" -ne 0 ]; then
    cat "${output}"
    fail "controller letsencrypt install exited with ${status}"
  fi

  [ -f "${env_file}" ] || fail ".env was not created"
  [ -f "${log_file}" ] || fail "command log was not created"

  assert_file_contains "${env_file}" "APP_ENV=production"
  assert_file_contains "${env_file}" "DATABASE_SSL_MODE=require"
  assert_file_contains "${env_file}" "SSL_CERT_PATH=/etc/letsencrypt/live/panel.example.com/fullchain.pem"
  assert_file_contains "${env_file}" "SSL_KEY_PATH=/etc/letsencrypt/live/panel.example.com/privkey.pem"
  assert_file_contains "${env_file}" "POSTGRES_PASSWORD="
  assert_file_contains "${env_file}" "NATS_AUTH_TOKEN="
  assert_file_contains "${env_file}" "JWT_SECRET="
  assert_file_contains "${env_file}" "ENCRYPTION_KEY="

  assert_file_contains "${log_file}" "apt-get install -y ca-certificates curl git openssl lsb-release gnupg uuid-runtime argon2"
  assert_file_contains "${log_file}" "apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
  assert_file_contains "${log_file}" "apt-get install -y certbot"
  assert_file_contains "${log_file}" "certbot certonly --standalone --non-interactive --agree-tos -m ops@example.com -d panel.example.com --keep-until-expiring"
  assert_file_contains "${log_file}" "docker compose -f docker-compose.yml -f docker-compose.prod.yml config"
  assert_file_contains "${log_file}" "docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d postgres nats"
  assert_file_contains "${log_file}" "migrate/migrate:v4.19.1"
  assert_file_contains "${log_file}" "docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build"
  assert_output_contains "${output}" "VirtueStack controller installation completed"
  assert_output_contains "${output}" "Environment file: ${env_file}"
  assert_output_contains "${output}" "TLS mode: Let's Encrypt"
  assert_output_contains "${output}" "Node agents run natively on hypervisor hosts via --node"
}

run_controller_interactive_prompt_test() {
  local workdir install_dir output log_file env_file status

  workdir="$(mktemp -d)"
  install_dir="${workdir}/install"
  output="${workdir}/controller-interactive.out"
  log_file="${workdir}/controller-interactive.commands"
  env_file="${install_dir}/.env"
  mkdir -p "${workdir}/home"

  set +e
  printf '%s\n' \
    'edge.example.com' \
    'y' \
    'ops@example.com' \
    'admin@example.com' \
    'Interactive Admin' \
    'InteractivePass123!' | env -i \
    HOME="${workdir}/home" \
    PATH="${PATH}" \
    TERM="${TERM:-xterm}" \
    VIRTUESTACK_TEST_MODE=1 \
    VIRTUESTACK_SOURCE_DIR="${REPO_ROOT}" \
    VIRTUESTACK_INSTALL_DIR="${install_dir}" \
    VIRTUESTACK_COMMAND_LOG="${log_file}" \
    bash "${SCRIPT_PATH}" --controller >"${output}" 2>&1
  status=$?
  set -e

  if [ "${status}" -ne 0 ]; then
    cat "${output}"
    fail "interactive controller install exited with ${status}"
  fi

  [ -f "${env_file}" ] || fail ".env was not created"

  assert_output_contains "${output}" "Enter the public domain or IP"
  assert_output_contains "${output}" "Use Let's Encrypt for this domain"
  assert_file_contains "${env_file}" "SSL_CERT_PATH=/etc/letsencrypt/live/edge.example.com/fullchain.pem"
  assert_file_contains "${log_file}" "certbot certonly --standalone --non-interactive --agree-tos -m ops@example.com -d edge.example.com --keep-until-expiring"
}

run_controller_self_signed_test() {
  local workdir install_dir output log_file env_file status

  workdir="$(mktemp -d)"
  install_dir="${workdir}/install"
  output="${workdir}/controller-self-signed.out"
  log_file="${workdir}/controller-self-signed.commands"
  env_file="${install_dir}/.env"
  mkdir -p "${workdir}/home"

  set +e
  env -i \
    HOME="${workdir}/home" \
    PATH="${PATH}" \
    TERM="${TERM:-xterm}" \
    VIRTUESTACK_TEST_MODE=1 \
    VIRTUESTACK_NONINTERACTIVE=1 \
    VIRTUESTACK_SOURCE_DIR="${REPO_ROOT}" \
    VIRTUESTACK_INSTALL_DIR="${install_dir}" \
    VIRTUESTACK_COMMAND_LOG="${log_file}" \
    VIRTUESTACK_DOMAIN="vm.example.com" \
    VIRTUESTACK_USE_LETSENCRYPT="false" \
    VIRTUESTACK_ADMIN_EMAIL="admin@example.com" \
    VIRTUESTACK_ADMIN_NAME="Example Admin" \
    VIRTUESTACK_ADMIN_PASSWORD="ExampleAdminPass123!" \
    bash "${SCRIPT_PATH}" --controller >"${output}" 2>&1
  status=$?
  set -e

  if [ "${status}" -ne 0 ]; then
    cat "${output}"
    fail "controller self-signed install exited with ${status}"
  fi

  [ -f "${env_file}" ] || fail ".env was not created"

  assert_file_contains "${env_file}" "SSL_CERT_PATH=./ssl/cert.pem"
  assert_file_contains "${env_file}" "SSL_KEY_PATH=./ssl/key.pem"
  assert_file_contains "${log_file}" "openssl req -x509 -nodes -newkey rsa:4096"
  assert_output_contains "${output}" "Self-signed TLS certificate configured"
  assert_output_contains "${output}" "Environment file: ${env_file}"
  assert_output_contains "${output}" "TLS mode: self-signed"
}

run_node_install_test() {
  local workdir install_dir output log_file status
  local tls_source_dir system_root env_file service_file binary_file
  local cert_file key_file ca_file

  workdir="$(mktemp -d)"
  install_dir="${workdir}/install"
  output="${workdir}/node.out"
  log_file="${workdir}/node.commands"
  tls_source_dir="${workdir}/tls-source"
  system_root="${install_dir}/system-root"
  env_file="${system_root}/etc/virtuestack/node-agent.env"
  service_file="${system_root}/etc/systemd/system/virtuestack-node-agent.service"
  binary_file="${system_root}/usr/local/bin/virtuestack-node-agent"
  cert_file="${system_root}/etc/virtuestack/certs/node-agent.crt"
  key_file="${system_root}/etc/virtuestack/certs/node-agent.key"
  ca_file="${system_root}/etc/virtuestack/certs/ca.crt"
  mkdir -p "${workdir}/home"
  mkdir -p "${tls_source_dir}"
  printf 'node-cert\n' > "${tls_source_dir}/node-agent.crt"
  printf 'node-key\n' > "${tls_source_dir}/node-agent.key"
  printf 'ca-cert\n' > "${tls_source_dir}/ca.crt"

  set +e
  env -i \
    HOME="${workdir}/home" \
    PATH="${PATH}" \
    TERM="${TERM:-xterm}" \
    VIRTUESTACK_TEST_MODE=1 \
    VIRTUESTACK_NONINTERACTIVE=1 \
    VIRTUESTACK_SOURCE_DIR="${REPO_ROOT}" \
    VIRTUESTACK_INSTALL_DIR="${install_dir}" \
    VIRTUESTACK_COMMAND_LOG="${log_file}" \
    VIRTUESTACK_CONTROLLER_GRPC_ADDR="controller.example.com:50051" \
    VIRTUESTACK_NODE_ID="11111111-2222-3333-4444-555555555555" \
    VIRTUESTACK_STORAGE_BACKEND="qcow" \
    VIRTUESTACK_STORAGE_PATH="/var/lib/virtuestack" \
    VIRTUESTACK_CLOUDINIT_PATH="/var/lib/virtuestack/cloud-init" \
    VIRTUESTACK_ISO_STORAGE_PATH="/var/lib/virtuestack/iso" \
    VIRTUESTACK_GUEST_OP_HMAC_SECRET="node-guest-op-secret-32-bytes-min" \
    VIRTUESTACK_TLS_CERT_SOURCE="${tls_source_dir}/node-agent.crt" \
    VIRTUESTACK_TLS_KEY_SOURCE="${tls_source_dir}/node-agent.key" \
    VIRTUESTACK_TLS_CA_SOURCE="${tls_source_dir}/ca.crt" \
    bash "${SCRIPT_PATH}" --node >"${output}" 2>&1
  status=$?
  set -e

  if [ "${status}" -ne 0 ]; then
    cat "${output}"
    fail "node install exited with ${status}"
  fi

  [ -f "${env_file}" ] || fail "node agent env file was not created"
  [ -f "${service_file}" ] || fail "node agent service file was not created"
  [ -f "${binary_file}" ] || fail "installed node agent binary was not created"
  [ -f "${cert_file}" ] || fail "copied node certificate was not created"
  [ -f "${key_file}" ] || fail "copied node key was not created"
  [ -f "${ca_file}" ] || fail "copied CA certificate was not created"

  assert_file_contains "${log_file}" "apt-get install -y build-essential make pkg-config libvirt-dev librados-dev librbd-dev qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils dnsmasq-base cloud-image-utils genisoimage"
  assert_file_contains "${log_file}" "curl -fsSL https://github.com/AbuGosok/VirtueStack/releases/download/"
  assert_file_contains "${log_file}" "make build-node-agent"
  assert_file_contains "${log_file}" "systemctl daemon-reload"
  assert_file_contains "${log_file}" "systemctl enable virtuestack-node-agent"
  assert_file_contains "${log_file}" "systemctl start virtuestack-node-agent"

  assert_file_contains "${env_file}" "CONTROLLER_GRPC_ADDR=controller.example.com:50051"
  assert_file_contains "${env_file}" "NODE_ID=11111111-2222-3333-4444-555555555555"
  assert_file_contains "${env_file}" "STORAGE_BACKEND=qcow"
  assert_file_contains "${env_file}" "STORAGE_PATH=/var/lib/virtuestack"
  assert_file_contains "${env_file}" "CLOUDINIT_PATH=/var/lib/virtuestack/cloud-init"
  assert_file_contains "${env_file}" "ISO_STORAGE_PATH=/var/lib/virtuestack/iso"
  assert_file_contains "${env_file}" "GUEST_OP_HMAC_SECRET=node-guest-op-secret-32-bytes-min"
  assert_file_contains "${env_file}" "TLS_CERT_FILE=/etc/virtuestack/certs/node-agent.crt"
  assert_file_contains "${env_file}" "TLS_KEY_FILE=/etc/virtuestack/certs/node-agent.key"
  assert_file_contains "${env_file}" "TLS_CA_FILE=/etc/virtuestack/certs/ca.crt"

  assert_file_contains "${service_file}" "ExecStart=/usr/local/bin/virtuestack-node-agent"
  assert_file_contains "${service_file}" "EnvironmentFile=/etc/virtuestack/node-agent.env"
  assert_file_contains "${service_file}" "Restart=always"

  assert_output_contains "${output}" "VirtueStack node agent installation completed"
  assert_output_contains "${output}" "Environment file: /etc/virtuestack/node-agent.env"
  assert_output_contains "${output}" "Service: /etc/systemd/system/virtuestack-node-agent.service"
  assert_output_contains "${output}" "journalctl -u virtuestack-node-agent -f"
}

run_usage_error_test() {
  local workdir output status

  workdir="$(mktemp -d)"
  output="${workdir}/usage.out"
  mkdir -p "${workdir}/home"

  set +e
  env -i \
    HOME="${workdir}/home" \
    PATH="${PATH}" \
    TERM="${TERM:-xterm}" \
    bash "${SCRIPT_PATH}" >"${output}" 2>&1
  status=$?
  set -e

  if [ "${status}" -eq 0 ]; then
    cat "${output}"
    fail "usage error test unexpectedly succeeded"
  fi

  assert_output_contains "${output}" "Usage:"
}

run_controller_letsencrypt_test
run_controller_interactive_prompt_test
run_controller_self_signed_test
run_node_install_test
run_usage_error_test

echo "✅ install.sh tests passed"
