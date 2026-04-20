#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
REPO_URL="https://github.com/AbuGosok/VirtueStack.git"
REPO_SLUG="AbuGosok/VirtueStack"
DEFAULT_INSTALL_DIR="/opt/virtuestack"
PROD_COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.prod.yml)
MIGRATE_IMAGE="migrate/migrate:v4.19.1"
GO_VERSION="1.26.0"
GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"
NODE_ENV_FILE="/etc/virtuestack/node-agent.env"
NODE_CERTS_DIR="/etc/virtuestack/certs"
NODE_SERVICE_FILE="/etc/systemd/system/virtuestack-node-agent.service"
NODE_BINARY_PATH="/usr/local/bin/virtuestack-node-agent"

MODE=""
REPO_DIR=""
INSTALL_DIR="${VIRTUESTACK_INSTALL_DIR:-}"
SOURCE_DIR="${VIRTUESTACK_SOURCE_DIR:-}"
NONINTERACTIVE="${VIRTUESTACK_NONINTERACTIVE:-0}"
TEST_MODE="${VIRTUESTACK_TEST_MODE:-0}"
COMMAND_LOG="${VIRTUESTACK_COMMAND_LOG:-}"

DOMAIN=""
USE_LETSENCRYPT=""
LE_EMAIL=""
ADMIN_EMAIL=""
ADMIN_NAME=""
ADMIN_PASSWORD=""

POSTGRES_USER="virtuestack"
POSTGRES_DB="virtuestack"
POSTGRES_PASSWORD=""
NATS_AUTH_TOKEN=""
JWT_SECRET=""
ENCRYPTION_KEY=""

CONTROLLER_GRPC_ADDR=""
NODE_ID=""
STORAGE_BACKEND=""
STORAGE_PATH=""
CEPH_POOL=""
CEPH_USER=""
CEPH_CONF=""
LVM_VOLUME_GROUP=""
LVM_THIN_POOL=""
TLS_CERT_SOURCE=""
TLS_KEY_SOURCE=""
TLS_CA_SOURCE=""
TLS_CERT_FILE="${NODE_CERTS_DIR}/node-agent.crt"
TLS_KEY_FILE="${NODE_CERTS_DIR}/node-agent.key"
TLS_CA_FILE="${NODE_CERTS_DIR}/ca.crt"
CLOUDINIT_PATH=""
ISO_STORAGE_PATH=""
LOG_LEVEL="info"

log() {
  printf '[INFO] %s\n' "$*"
}

success() {
  printf '[OK] %s\n' "$*"
}

warn() {
  printf '[WARN] %s\n' "$*" >&2
}

die() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage: ${SCRIPT_NAME} --controller | --node

Options:
  --controller   Install the VirtueStack controller Docker stack
  --node         Install the VirtueStack node agent as a native systemd service
  -h, --help     Show this help message

This script supports fresh Ubuntu 24 and Debian 13 servers.
EOF
}

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|y|Y|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

log_command() {
  if [[ -n "${COMMAND_LOG}" ]]; then
    printf '%s\n' "$*" >> "${COMMAND_LOG}"
  fi
}

run_cmd() {
  log_command "$*"
  if [[ "${TEST_MODE}" == "1" ]]; then
    return 0
  fi
  "$@"
}

run_bash() {
  local command="$1"
  log_command "${command}"
  if [[ "${TEST_MODE}" == "1" ]]; then
    return 0
  fi
  bash -lc "${command}"
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

require_root() {
  if [[ "${TEST_MODE}" == "1" ]] || [[ "${EUID}" -eq 0 ]]; then
    return
  fi

  if ! command_exists sudo; then
    die "This installer needs root privileges and sudo is not available."
  fi

  exec sudo -E bash "$0" "$@"
}

parse_args() {
  if [[ $# -eq 0 ]]; then
    usage
    exit 1
  fi

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --controller)
        [[ -z "${MODE}" ]] || die "Choose only one mode."
        MODE="controller"
        ;;
      --node)
        [[ -z "${MODE}" ]] || die "Choose only one mode."
        MODE="node"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "Unknown argument: $1"
        ;;
    esac
    shift
  done

  [[ -n "${MODE}" ]] || die "You must choose --controller or --node."
}

ensure_supported_os() {
  if [[ "${TEST_MODE}" == "1" ]]; then
    success "Skipping OS gate in test mode"
    return
  fi
  [[ -r /etc/os-release ]] || die "Cannot detect operating system."
  # shellcheck disable=SC1091
  source /etc/os-release

  case "${ID:-}" in
    ubuntu)
      [[ "${VERSION_ID:-}" == "24.04" ]] || die "Ubuntu 24.04 is required; found ${VERSION_ID:-unknown}."
      ;;
    debian)
      [[ "${VERSION_ID:-}" == "13" ]] || die "Debian 13 is required; found ${VERSION_ID:-unknown}."
      ;;
    *)
      die "Unsupported operating system: ${ID:-unknown}"
      ;;
  esac

  success "Detected supported OS: ${PRETTY_NAME:-${ID}}"
}

apt_update_once() {
  if [[ "${APT_UPDATED:-0}" == "1" ]]; then
    return
  fi
  run_cmd apt-get update
  APT_UPDATED=1
}

install_packages() {
  local packages=("$@")
  apt_update_once
  DEBIAN_FRONTEND=noninteractive run_cmd apt-get install -y "${packages[@]}"
}

resolve_repo_dir() {
  if [[ "${TEST_MODE}" == "1" && -n "${INSTALL_DIR}" ]]; then
    REPO_DIR="${INSTALL_DIR}"
    mkdir -p "${REPO_DIR}"
    return
  fi

  if [[ -n "${SOURCE_DIR}" ]]; then
    REPO_DIR="${SOURCE_DIR}"
    return
  fi

  if [[ -f "./go.mod" ]] && grep -Fq 'module github.com/AbuGosok/VirtueStack' "./go.mod"; then
    REPO_DIR="$(pwd)"
    return
  fi

  INSTALL_DIR="${INSTALL_DIR:-${DEFAULT_INSTALL_DIR}}"
  REPO_DIR="${INSTALL_DIR}"
  mkdir -p "$(dirname "${REPO_DIR}")"

  if [[ -d "${REPO_DIR}/.git" ]]; then
    run_cmd git -C "${REPO_DIR}" fetch --tags --prune origin
    run_cmd git -C "${REPO_DIR}" pull --ff-only origin
  else
    run_cmd git clone "${REPO_URL}" "${REPO_DIR}"
  fi
}

random_hex() {
  local hex_len="$1"
  if command_exists openssl; then
    openssl rand -hex $((hex_len / 2))
    return
  fi
  python3 - <<PY
import secrets
print(secrets.token_hex(${hex_len} // 2))
PY
}

generate_uuid() {
  if command_exists uuidgen; then
    uuidgen | tr '[:upper:]' '[:lower:]'
    return
  fi

  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    tr '[:upper:]' '[:lower:]' < /proc/sys/kernel/random/uuid
    return
  fi

  random_hex 32
}

system_path() {
  local path="$1"

  if [[ "${TEST_MODE}" == "1" && -n "${INSTALL_DIR}" ]]; then
    printf '%s/system-root%s' "${INSTALL_DIR}" "${path}"
    return
  fi

  printf '%s' "${path}"
}

install_file_to_system() {
  local mode="$1"
  local source="$2"
  local destination="$3"
  local actual_destination

  actual_destination="$(system_path "${destination}")"
  mkdir -p "$(dirname "${actual_destination}")"
  log_command "install -m ${mode} ${source} ${destination}"
  install -m "${mode}" "${source}" "${actual_destination}"
}

write_node_env() {
  local env_file
  env_file="$(system_path "${NODE_ENV_FILE}")"
  mkdir -p "$(dirname "${env_file}")"

  cat > "${env_file}" <<EOF
# Generated by install.sh for VirtueStack node-agent deployment
CONTROLLER_GRPC_ADDR=${CONTROLLER_GRPC_ADDR}
NODE_ID=${NODE_ID}
STORAGE_BACKEND=${STORAGE_BACKEND}
STORAGE_PATH=${STORAGE_PATH}
CEPH_POOL=${CEPH_POOL}
CEPH_USER=${CEPH_USER}
CEPH_CONF=${CEPH_CONF}
LVM_VOLUME_GROUP=${LVM_VOLUME_GROUP}
LVM_THIN_POOL=${LVM_THIN_POOL}
TLS_CERT_FILE=${TLS_CERT_FILE}
TLS_KEY_FILE=${TLS_KEY_FILE}
TLS_CA_FILE=${TLS_CA_FILE}
CLOUDINIT_PATH=${CLOUDINIT_PATH}
ISO_STORAGE_PATH=${ISO_STORAGE_PATH}
LOG_LEVEL=${LOG_LEVEL}
EOF

  chmod 600 "${env_file}"
}

write_node_service() {
  local service_file
  service_file="$(system_path "${NODE_SERVICE_FILE}")"
  mkdir -p "$(dirname "${service_file}")"

  cat > "${service_file}" <<EOF
[Unit]
Description=VirtueStack Node Agent
After=network-online.target libvirtd.service
Wants=network-online.target libvirtd.service

[Service]
Type=simple
User=root
ExecStart=${NODE_BINARY_PATH}
Restart=always
RestartSec=5
EnvironmentFile=${NODE_ENV_FILE}

[Install]
WantedBy=multi-user.target
EOF

  chmod 644 "${service_file}"
}

copy_node_tls_assets() {
  [[ -f "${TLS_CERT_SOURCE}" ]] || die "TLS certificate source file not found: ${TLS_CERT_SOURCE}"
  [[ -f "${TLS_KEY_SOURCE}" ]] || die "TLS key source file not found: ${TLS_KEY_SOURCE}"
  [[ -f "${TLS_CA_SOURCE}" ]] || die "TLS CA source file not found: ${TLS_CA_SOURCE}"

  install_file_to_system 0644 "${TLS_CERT_SOURCE}" "${TLS_CERT_FILE}"
  install_file_to_system 0600 "${TLS_KEY_SOURCE}" "${TLS_KEY_FILE}"
  install_file_to_system 0644 "${TLS_CA_SOURCE}" "${TLS_CA_FILE}"
}

ensure_node_binary_artifact() {
  if [[ -f "${REPO_DIR}/bin/node-agent" ]]; then
    return
  fi

  if [[ "${TEST_MODE}" == "1" ]]; then
    mkdir -p "${REPO_DIR}/bin"
    cat > "${REPO_DIR}/bin/node-agent" <<'EOF'
#!/usr/bin/env bash
echo "test node agent"
EOF
    chmod 0755 "${REPO_DIR}/bin/node-agent"
    return
  fi

  die "Node-agent binary not found at ${REPO_DIR}/bin/node-agent"
}

install_node_binary_to_system() {
  ensure_node_binary_artifact
  install_file_to_system 0755 "${REPO_DIR}/bin/node-agent" "${NODE_BINARY_PATH}"
}

prompt_node_install_inputs() {
  prompt_value CONTROLLER_GRPC_ADDR "Enter the controller gRPC address (host:port)"
  prompt_value NODE_ID "Enter the node UUID" "$(generate_uuid)"
  prompt_value STORAGE_BACKEND "Enter the storage backend (qcow, ceph, or lvm)" "qcow"

  case "${STORAGE_BACKEND}" in
    qcow)
      prompt_value STORAGE_PATH "Enter the QCOW storage root" "/var/lib/virtuestack"
      ;;
    ceph)
      prompt_value CEPH_POOL "Enter the Ceph pool name" "vs-vms"
      prompt_value CEPH_USER "Enter the Ceph client user" "virtuestack"
      prompt_value CEPH_CONF "Enter the Ceph config path" "/etc/ceph/ceph.conf"
      ;;
    lvm)
      prompt_value LVM_VOLUME_GROUP "Enter the LVM volume group name"
      prompt_value LVM_THIN_POOL "Enter the LVM thin pool name"
      ;;
    *)
      die "Unsupported storage backend: ${STORAGE_BACKEND}"
      ;;
  esac

  prompt_value CLOUDINIT_PATH "Enter the cloud-init storage path" "/var/lib/virtuestack/cloud-init"
  prompt_value ISO_STORAGE_PATH "Enter the ISO storage path" "/var/lib/virtuestack/iso"
  prompt_value TLS_CERT_SOURCE "Enter the source path to the node TLS certificate"
  prompt_value TLS_KEY_SOURCE "Enter the source path to the node TLS private key"
  prompt_value TLS_CA_SOURCE "Enter the source path to the node TLS CA certificate"
}

prepare_node_runtime_dirs() {
  run_bash "mkdir -p '${CLOUDINIT_PATH}' '${ISO_STORAGE_PATH}'"

  case "${STORAGE_BACKEND}" in
    qcow)
      run_bash "mkdir -p '${STORAGE_PATH}' '${STORAGE_PATH}/templates' '${STORAGE_PATH}/vms'"
      ;;
    ceph|lvm)
      ;;
  esac
}

prompt_value() {
  local var_name="$1"
  local prompt_text="$2"
  local default_value="${3:-}"
  local secret="${4:-0}"
  local env_name="VIRTUESTACK_${var_name}"
  local value="${!var_name:-}"

  if [[ -z "${value}" && -n "${!env_name:-}" ]]; then
    value="${!env_name}"
  fi

  if [[ -n "${value}" ]]; then
    printf -v "${var_name}" '%s' "${value}"
    return
  fi

  if [[ "${NONINTERACTIVE}" == "1" ]]; then
    if [[ -n "${default_value}" ]]; then
      printf -v "${var_name}" '%s' "${default_value}"
      return
    fi
    die "Missing required non-interactive value for ${var_name}"
  fi

  while :; do
    printf '%s' "${prompt_text}${default_value:+ [${default_value}]}: "
    if [[ "${secret}" == "1" ]]; then
      read -r -s value
      printf '\n'
    else
      read -r value
    fi
    value="${value:-${default_value}}"
    [[ -n "${value}" ]] && break
    warn "A value is required."
  done

  printf -v "${var_name}" '%s' "${value}"
}

prompt_bool() {
  local var_name="$1"
  local prompt_text="$2"
  local default_value="${3:-n}"
  local env_name="VIRTUESTACK_${var_name}"
  local raw="${!var_name:-}"

  if [[ -z "${raw}" && -n "${!env_name:-}" ]]; then
    raw="${!env_name}"
  fi

  if [[ -n "${raw}" ]]; then
    if is_true "${raw}"; then
      printf -v "${var_name}" 'true'
    else
      printf -v "${var_name}" 'false'
    fi
    return
  fi

  if [[ "${NONINTERACTIVE}" == "1" ]]; then
    if is_true "${default_value}"; then
      printf -v "${var_name}" 'true'
    else
      printf -v "${var_name}" 'false'
    fi
    return
  fi

  printf '%s' "${prompt_text} [${default_value}]: "
  read -r raw
  raw="${raw:-${default_value}}"
  if is_true "${raw}"; then
    printf -v "${var_name}" 'true'
  else
    printf -v "${var_name}" 'false'
  fi
}

validate_email() {
  local email="$1"
  [[ "${email}" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]]
}

validate_password() {
  local password="$1"
  [[ ${#password} -ge 12 ]]
}

is_domain_name() {
  local value="$1"
  [[ "${value}" == *.* ]] && [[ ! "${value}" =~ ^[0-9.]+$ ]] && [[ ! "${value}" =~ : ]]
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
}

architecture_suffix() {
  local arch
  arch="$(dpkg --print-architecture 2>/dev/null || uname -m)"
  case "${arch}" in
    amd64|x86_64) printf 'linux-amd64' ;;
    arm64|aarch64) printf 'linux-arm64' ;;
    *)
      return 1
      ;;
  esac
}

controller_compose() {
  docker compose "${PROD_COMPOSE_FILES[@]}" "$@"
}

write_controller_env() {
  local env_file="${REPO_DIR}/.env"
  local ssl_cert_path="./ssl/cert.pem"
  local ssl_key_path="./ssl/key.pem"

  if [[ "${USE_LETSENCRYPT}" == "true" ]]; then
    ssl_cert_path="/etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
    ssl_key_path="/etc/letsencrypt/live/${DOMAIN}/privkey.pem"
  fi

  cat > "${env_file}" <<EOF
# Generated by install.sh for VirtueStack controller deployment
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=${POSTGRES_DB}
NATS_AUTH_TOKEN=${NATS_AUTH_TOKEN}
JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
APP_ENV=production
LOG_LEVEL=info
DATABASE_SSL_MODE=disable
NEXT_PUBLIC_API_URL=/api/v1
SSL_CERT_PATH=${ssl_cert_path}
SSL_KEY_PATH=${ssl_key_path}
ALLOW_SELF_REGISTRATION=false
REGISTRATION_EMAIL_VERIFICATION=true
EOF

  chmod 600 "${env_file}"
}

generate_self_signed_cert() {
  local ssl_dir="${REPO_DIR}/ssl"
  mkdir -p "${ssl_dir}"
  run_bash "cd '${REPO_DIR}' && openssl req -x509 -nodes -newkey rsa:4096 -days 365 -keyout '${ssl_dir}/key.pem' -out '${ssl_dir}/cert.pem' -subj '/CN=${DOMAIN}' -addext 'subjectAltName=DNS:${DOMAIN},IP:127.0.0.1'"
  success "Self-signed TLS certificate configured"
}

ensure_letsencrypt_cert() {
  [[ -n "${LE_EMAIL}" ]] || die "Let's Encrypt email is required."
  install_packages certbot
  run_cmd certbot certonly --standalone --non-interactive --agree-tos -m "${LE_EMAIL}" -d "${DOMAIN}" --keep-until-expiring
  success "Let's Encrypt certificate requested for ${DOMAIN}"
}

hash_password() {
  local password="$1"
  local helper_dir helper_rel output
  if [[ "${TEST_MODE}" == "1" ]]; then
    printf '$argon2id$v=19$m=65536,t=3,p=4$testsalt$testhash'
    return
  fi
  helper_dir="$(mktemp -d "${REPO_DIR}/.install-hash.XXXXXX")"
  helper_rel="./$(basename "${helper_dir}")"
  cat > "${helper_dir}/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"

	"github.com/alexedwards/argon2id"

	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

func main() {
	hash, err := argon2id.CreateHash(os.Getenv("VIRTUESTACK_ADMIN_PASSWORD"), services.Argon2idParams)
	if err != nil {
		panic(err)
	}
	fmt.Print(hash)
}
EOF
  if command_exists go; then
    log_command "go run password hash helper"
    output="$(
      cd "${REPO_DIR}" &&
      VIRTUESTACK_ADMIN_PASSWORD="${password}" go run "${helper_rel}"
    )"
    rm -rf "${helper_dir}"
    printf '%s' "${output}"
    return
  fi

  if command_exists docker; then
    log_command "docker run password hash helper"
    output="$(
      docker run --rm \
        -e VIRTUESTACK_ADMIN_PASSWORD="${password}" \
        -v "${REPO_DIR}:/src" \
        -w /src \
        golang:1.26-bookworm \
        go run "${helper_rel}"
    )"
    rm -rf "${helper_dir}"
    printf '%s' "${output}"
    return
  fi

  rm -rf "${helper_dir}"
  die "Unable to hash admin password: neither Go nor Docker is available."
}

bootstrap_admin_sql() {
  local hash admin_email_sql admin_name_sql hash_sql
  hash="$(hash_password "${ADMIN_PASSWORD}")"
  admin_email_sql="$(sql_escape "${ADMIN_EMAIL}")"
  admin_name_sql="$(sql_escape "${ADMIN_NAME}")"
  hash_sql="$(sql_escape "${hash}")"
  cat <<EOF
INSERT INTO admins (
    email,
    password_hash,
    name,
    role,
    totp_enabled,
    totp_secret_encrypted,
    created_at
)
SELECT
    '${admin_email_sql}',
    '${hash_sql}',
    '${admin_name_sql}',
    'super_admin',
    FALSE,
    '',
    NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM admins WHERE email = '${admin_email_sql}'
);
EOF
}

run_migrations() {
  local db_url
  db_url="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable"
  if [[ "${TEST_MODE}" == "1" ]]; then
    log_command "docker compose -f docker-compose.yml -f docker-compose.prod.yml config"
    log_command "docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d postgres nats"
  else
    controller_compose config >/dev/null
    controller_compose up -d postgres nats
  fi
  run_cmd docker run --rm --network virtuestack-network -v "${REPO_DIR}/migrations:/migrations:ro" "${MIGRATE_IMAGE}" -path=/migrations -database "${db_url}" up
}

seed_initial_admin() {
  local sql_file
  sql_file="$(mktemp)"
  bootstrap_admin_sql > "${sql_file}"
  run_bash "cd '${REPO_DIR}' && docker exec -i virtuestack-postgres psql -U '${POSTGRES_USER}' -d '${POSTGRES_DB}' < '${sql_file}'"
  rm -f "${sql_file}"
}

install_docker_engine() {
  install_packages ca-certificates curl git openssl lsb-release gnupg uuid-runtime argon2
  if [[ "${TEST_MODE}" != "1" ]] && command_exists docker && docker compose version >/dev/null 2>&1; then
    success "Docker and Docker Compose already available"
    return
  fi

  install_packages ca-certificates curl gnupg
  if [[ "${TEST_MODE}" == "1" ]]; then
    log_command "install -m 0755 -d /etc/apt/keyrings"
    log_command "curl -fsSL https://download.docker.com/linux/\$(. /etc/os-release && echo \"\$ID\")/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg"
    log_command "chmod a+r /etc/apt/keyrings/docker.gpg"
  else
    install -m 0755 -d /etc/apt/keyrings
    run_bash "curl -fsSL https://download.docker.com/linux/$(. /etc/os-release && echo \"$ID\")/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg"
    chmod a+r /etc/apt/keyrings/docker.gpg
  fi
  # shellcheck disable=SC1091
  source /etc/os-release
  if [[ "${TEST_MODE}" == "1" ]]; then
    log_command "cat > /etc/apt/sources.list.d/docker.list <<EOF"
    log_command "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable"
    log_command "EOF"
  else
    cat > /etc/apt/sources.list.d/docker.list <<EOF
deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable
EOF
  fi
  APT_UPDATED=0
  install_packages docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  run_cmd systemctl enable --now docker
}

install_go_toolchain() {
  if command_exists go; then
    local current_go
    current_go="$(go version 2>/dev/null | awk '{print $3}')"
    if [[ "${current_go}" =~ ^go1\.(26|[3-9][0-9])(\.|$) ]]; then
      success "Go already installed"
      return
    fi
    warn "Found ${current_go}; upgrading to Go ${GO_VERSION} for VirtueStack compatibility."
  fi

  log_command "curl -fsSL https://go.dev/dl/${GO_TARBALL} -o /tmp/${GO_TARBALL}"
  log_command "rm -rf /usr/local/go"
  log_command "tar -C /usr/local -xzf /tmp/${GO_TARBALL}"
  log_command "ln -sf /usr/local/go/bin/go /usr/local/bin/go"
  log_command "ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt"
  if [[ "${TEST_MODE}" == "1" ]]; then
    return
  fi

  run_cmd curl -fsSL "https://go.dev/dl/${GO_TARBALL}" -o "/tmp/${GO_TARBALL}"
  rm -rf /usr/local/go
  run_cmd tar -C /usr/local -xzf "/tmp/${GO_TARBALL}"
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
  rm -f "/tmp/${GO_TARBALL}"
}

install_node_agent_binary() {
  local version arch_suffix url temp_file

  if [[ -d "${REPO_DIR}/.git" ]]; then
    version="$(git -C "${REPO_DIR}" describe --tags --abbrev=0 2>/dev/null || true)"
  fi
  version="${version:-}"
  [[ -n "${version}" ]] || version="latest"

  if ! arch_suffix="$(architecture_suffix)"; then
    warn "Unsupported architecture for prebuilt node-agent binary; falling back to source build."
    return 1
  fi

  url="https://github.com/${REPO_SLUG}/releases/download/${version}/virtuestack-node-agent-${arch_suffix}"
  temp_file="/tmp/virtuestack-node-agent-${arch_suffix}"
  log_command "curl -fsSL ${url} -o ${temp_file}"

  if [[ "${TEST_MODE}" == "1" ]]; then
    return 1
  fi

  if ! curl -fsSL "${url}" -o "${temp_file}"; then
    warn "Prebuilt node-agent binary not available for ${version}/${arch_suffix}; falling back to source build."
    rm -f "${temp_file}"
    return 1
  fi

  install -m 0755 "${temp_file}" "${REPO_DIR}/bin/node-agent"
  rm -f "${temp_file}"
  return 0
}

run_controller_install() {
  local tls_mode

  install_docker_engine
  resolve_repo_dir
  mkdir -p "${REPO_DIR}"

  prompt_value DOMAIN "Enter the public domain or IP for VirtueStack"
  if is_domain_name "${DOMAIN}"; then
    prompt_bool USE_LETSENCRYPT "Use Let's Encrypt for this domain?" "y"
    if [[ "${USE_LETSENCRYPT}" == "true" ]]; then
      prompt_value LE_EMAIL "Enter the email for Let's Encrypt renewal notices"
      validate_email "${LE_EMAIL}" || die "Invalid Let's Encrypt email address."
    fi
  else
    USE_LETSENCRYPT="false"
  fi

  prompt_value ADMIN_EMAIL "Enter the initial admin email address"
  validate_email "${ADMIN_EMAIL}" || die "Invalid admin email address."
  prompt_value ADMIN_NAME "Enter the initial admin display name" "VirtueStack Administrator"
  prompt_value ADMIN_PASSWORD "Enter the initial admin password" "" 1
  validate_password "${ADMIN_PASSWORD}" || die "Admin password must be at least 12 characters."

  POSTGRES_PASSWORD="$(random_hex 32)"
  NATS_AUTH_TOKEN="$(random_hex 64)"
  JWT_SECRET="$(random_hex 64)"
  ENCRYPTION_KEY="$(random_hex 64)"

  write_controller_env

  if [[ "${USE_LETSENCRYPT}" == "true" ]]; then
    ensure_letsencrypt_cert
  else
    generate_self_signed_cert
  fi

  run_bash "cd '${REPO_DIR}' && docker compose -f docker-compose.yml -f docker-compose.prod.yml config >/dev/null"
  run_migrations
  run_bash "cd '${REPO_DIR}' && docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build"
  seed_initial_admin

  if [[ "${USE_LETSENCRYPT}" == "true" ]]; then
    tls_mode="Let's Encrypt"
  else
    tls_mode="self-signed"
  fi

  success "VirtueStack controller installation completed"
  cat <<EOF

Environment file: ${REPO_DIR}/.env
TLS mode: ${tls_mode}
Admin URL: https://${DOMAIN}/admin
Customer URL: https://${DOMAIN}
Initial admin email: ${ADMIN_EMAIL}
Node agents run natively on hypervisor hosts via --node
EOF
}

run_node_install() {
  install_packages ca-certificates curl git openssl lsb-release gnupg uuid-runtime argon2
  install_packages build-essential make pkg-config libvirt-dev librados-dev librbd-dev qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils dnsmasq-base cloud-image-utils genisoimage
  resolve_repo_dir
  mkdir -p "${REPO_DIR}/bin"

  if install_node_agent_binary; then
    success "VirtueStack node agent binary downloaded successfully"
  else
    install_go_toolchain
    run_bash "export PATH=/usr/local/go/bin:\$PATH && cd '${REPO_DIR}' && make build-node-agent"
    success "VirtueStack node agent build completed"
  fi

  prompt_node_install_inputs
  prepare_node_runtime_dirs
  copy_node_tls_assets
  write_node_env
  install_node_binary_to_system
  write_node_service
  run_cmd systemctl daemon-reload
  run_cmd systemctl enable virtuestack-node-agent
  run_cmd systemctl start virtuestack-node-agent

  success "VirtueStack node agent installation completed"
  cat <<EOF

Binary: ${NODE_BINARY_PATH}
Environment file: ${NODE_ENV_FILE}
Service: ${NODE_SERVICE_FILE}
Certificates: ${NODE_CERTS_DIR}

Inspect status: systemctl status virtuestack-node-agent --no-pager
Inspect logs: journalctl -u virtuestack-node-agent -f
EOF
}

main() {
  parse_args "$@"
  require_root "$@"
  ensure_supported_os

  case "${MODE}" in
    controller)
      run_controller_install
      ;;
    node)
      run_node_install
      ;;
    *)
      die "Unsupported mode: ${MODE}"
      ;;
  esac
}

main "$@"
