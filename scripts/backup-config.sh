#!/usr/bin/env bash
#
# VirtueStack Database Config Backup
#
# Creates authenticated encrypted backups of the VirtueStack PostgreSQL database
# using AES-256-CTR plus HMAC-SHA256 (encrypt-then-MAC).
# Backups are saved with date-based filenames and cleaned up after 30 days.
#
# Required environment variables:
#   ENCRYPTION_KEY - 256-bit (32 byte) hex-encoded master key
#
# Backup mode environment variables:
#   DATABASE_URL - PostgreSQL connection string
#
# Optional environment variables:
#   BACKUP_DIR      - Directory to store backups (default: ./backups)
#   RETENTION_DAYS  - Days to retain backups (default: 30)
#
# Usage:
#   ./backup-config.sh
#   ENCRYPTION_KEY=$(openssl rand -hex 32) DATABASE_URL=postgresql://... ./backup-config.sh
#   ENCRYPTION_KEY=<hex> ./backup-config.sh --decrypt backups/backup.sql.gz.enc backup.sql.gz
#   ENCRYPTION_KEY=<hex> ./backup-config.sh --encrypt-file input.sql.gz output.sql.gz.enc
#

set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
MAGIC_HEADER="VIRTUESTACK_BACKUP_V1"
MODE="backup"
INPUT_FILE=""
OUTPUT_FILE=""
DATABASE_URL="${DATABASE_URL:-}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
TEMP_DIR=""
ENCRYPTION_SUBKEY=""
HMAC_SUBKEY=""
BACKUP_FILE=""

usage() {
    cat <<USAGE
Usage:
  ./backup-config.sh
  ./backup-config.sh --decrypt <input.enc> <output>
  ./backup-config.sh --encrypt-file <input> <output.enc>
USAGE
}

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

parse_args() {
    if [[ $# -eq 0 ]]; then
        return
    fi

    case "$1" in
        --decrypt)
            [[ $# -eq 3 ]] || { usage >&2; exit 1; }
            MODE="decrypt"
            INPUT_FILE="$2"
            OUTPUT_FILE="$3"
            ;;
        --encrypt-file)
            [[ $# -eq 3 ]] || { usage >&2; exit 1; }
            MODE="encrypt-file"
            INPUT_FILE="$2"
            OUTPUT_FILE="$3"
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            usage >&2
            exit 1
            ;;
    esac
}

validate_encryption_key() {
    local key="$1"
    if [[ ! "${key}" =~ ^[0-9a-fA-F]{64}$ ]]; then
        echo "ERROR: ENCRYPTION_KEY must be a 64-character hex string (32 bytes / 256 bits)" >&2
        exit 1
    fi
}

binary_to_hex() {
    od -An -vtx1 | tr -d ' \n'
}

derive_subkey() {
    local purpose="$1"
    printf '%s:%s' "${ENCRYPTION_KEY}" "${purpose}" | openssl dgst -sha256 -binary | binary_to_hex
}

initialize_keys() {
    ENCRYPTION_SUBKEY="$(derive_subkey encrypt)"
    HMAC_SUBKEY="$(derive_subkey authenticate)"
}

ensure_backup_dir() {
    mkdir -p "${BACKUP_DIR}"
}

prepare_temp_dir() {
    local parent_dir="$1"
    TEMP_DIR="$(mktemp -d "${parent_dir}/.backup-tmp-XXXXXXXX")"
}

compute_hmac() {
    local iv_hex="$1"
    local ciphertext_file="$2"

    {
        printf '%s\n' "${MAGIC_HEADER}"
        printf '%s\n' "${iv_hex}"
        cat "${ciphertext_file}"
    } | openssl dgst -sha256 -mac HMAC -macopt "hexkey:${HMAC_SUBKEY}" -binary | binary_to_hex
}

dump_database() {
    local output_file="$1"
    echo "Dumping database..."
    if ! pg_dump "${DATABASE_URL}" | gzip > "${output_file}"; then
        echo "ERROR: pg_dump failed" >&2
        exit 1
    fi
    echo "Database dump complete: $(du -h "${output_file}" | cut -f1)"
}

encrypt_file() {
    local input_file="$1"
    local output_file="$2"
    local ciphertext_file iv_hex hmac_hex

    echo "Encrypting backup with AES-256-CTR + HMAC-SHA256..."
    ciphertext_file="${TEMP_DIR}/ciphertext.bin"
    iv_hex="$(openssl rand -hex 16)"

    openssl enc -aes-256-ctr -nosalt \
        -in "${input_file}" \
        -out "${ciphertext_file}" \
        -K "${ENCRYPTION_SUBKEY}" \
        -iv "${iv_hex}"

    hmac_hex="$(compute_hmac "${iv_hex}" "${ciphertext_file}")"

    {
        printf '%s\n' "${MAGIC_HEADER}"
        printf 'iv=%s\n' "${iv_hex}"
        printf 'hmac=%s\n' "${hmac_hex}"
        openssl base64 -A -in "${ciphertext_file}"
        printf '\n'
    } > "${output_file}"

    if [[ ! -f "${output_file}" ]]; then
        echo "ERROR: Encryption failed" >&2
        exit 1
    fi
    chmod 600 "${output_file}"
    echo "Encrypted backup: $(du -h "${output_file}" | cut -f1)"
}

decrypt_file() {
    local input_file="$1"
    local output_file="$2"
    local header iv_line hmac_line iv_hex stored_hmac expected_hmac ciphertext_file

    [[ -f "${input_file}" ]] || { echo "ERROR: Input file not found: ${input_file}" >&2; exit 1; }

    header="$(sed -n '1p' "${input_file}")"
    iv_line="$(sed -n '2p' "${input_file}")"
    hmac_line="$(sed -n '3p' "${input_file}")"

    [[ "${header}" == "${MAGIC_HEADER}" ]] || { echo "ERROR: Unsupported backup format" >&2; exit 1; }
    [[ "${iv_line}" == iv=* ]] || { echo "ERROR: Missing IV metadata" >&2; exit 1; }
    [[ "${hmac_line}" == hmac=* ]] || { echo "ERROR: Missing HMAC metadata" >&2; exit 1; }

    iv_hex="${iv_line#iv=}"
    stored_hmac="${hmac_line#hmac=}"
    ciphertext_file="${TEMP_DIR}/ciphertext.bin"

    tail -n +4 "${input_file}" | openssl base64 -d -A > "${ciphertext_file}"
    expected_hmac="$(compute_hmac "${iv_hex}" "${ciphertext_file}")"

    if [[ "${stored_hmac}" != "${expected_hmac}" ]]; then
        echo "ERROR: Backup authentication failed (HMAC mismatch)" >&2
        exit 1
    fi

    openssl enc -d -aes-256-ctr -nosalt \
        -in "${ciphertext_file}" \
        -out "${output_file}" \
        -K "${ENCRYPTION_SUBKEY}" \
        -iv "${iv_hex}"
    chmod 600 "${output_file}"

    echo "Decrypted backup: $(du -h "${output_file}" | cut -f1)"
}

rotate_old_backups() {
    echo "Removing backups older than ${RETENTION_DAYS} days..."
    local removed=0
    while IFS= read -r -d '' file; do
        echo "  Removing: $(basename "${file}")"
        rm -f "${file}"
        ((removed++))
    done < <(find "${BACKUP_DIR}" -name "virtuestack_*.sql.gz.enc" -type f -mtime "+${RETENTION_DAYS}" -print0 2>/dev/null || true)
    echo "Removed ${removed} old backup(s)"
}

print_summary() {
    echo ""
    echo "=== Backup Complete ==="
    echo "  File: ${BACKUP_FILE}"
    echo "  Size: $(du -h "${BACKUP_FILE}" | cut -f1)"
    echo "  Retention: ${RETENTION_DAYS} days"
    total_backups="$(find "${BACKUP_DIR}" -name "virtuestack_*.sql.gz.enc" -type f | wc -l)"
    echo "  Total backups: ${total_backups}"
}

run_backup_mode() {
    local timestamp dump_file

    DATABASE_URL="${DATABASE_URL:?DATABASE_URL environment variable is required}"
    ensure_backup_dir
    timestamp="$(date -u +"%Y%m%d_%H%M%S")"
    BACKUP_FILE="${BACKUP_DIR}/virtuestack_${timestamp}.sql.gz.enc"
    prepare_temp_dir "${BACKUP_DIR}"
    dump_file="${TEMP_DIR}/virtuestack_${timestamp}.sql.gz"

    dump_database "${dump_file}"
    encrypt_file "${dump_file}" "${BACKUP_FILE}"
    rotate_old_backups
    print_summary
}

run_encrypt_file_mode() {
    [[ -f "${INPUT_FILE}" ]] || { echo "ERROR: Input file not found: ${INPUT_FILE}" >&2; exit 1; }
    mkdir -p "$(dirname "${OUTPUT_FILE}")"
    prepare_temp_dir "$(dirname "${OUTPUT_FILE}")"
    encrypt_file "${INPUT_FILE}" "${OUTPUT_FILE}"
}

run_decrypt_mode() {
    mkdir -p "$(dirname "${OUTPUT_FILE}")"
    prepare_temp_dir "$(dirname "${OUTPUT_FILE}")"
    decrypt_file "${INPUT_FILE}" "${OUTPUT_FILE}"
}

parse_args "$@"
validate_encryption_key "${ENCRYPTION_KEY}"
initialize_keys

case "${MODE}" in
    backup)
        run_backup_mode
        ;;
    encrypt-file)
        run_encrypt_file_mode
        ;;
    decrypt)
        run_decrypt_mode
        ;;
    *)
        echo "ERROR: Unsupported mode ${MODE}" >&2
        exit 1
        ;;
esac
