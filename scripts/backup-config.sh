#!/usr/bin/env bash
#
# VirtueStack Database Config Backup
#
# Creates encrypted backups of the VirtueStack PostgreSQL database using AES-256-GCM.
# Backups are saved with date-based filenames and cleaned up after 30 days.
#
# Required environment variables:
#   DATABASE_URL - PostgreSQL connection string
#   ENCRYPTION_KEY - 256-bit (32 byte) hex-encoded key for AES-256-GCM encryption
#
# Optional environment variables:
#   BACKUP_DIR   - Directory to store backups (default: ./backups)
#   RETENTION_DAYS - Days to retain backups (default: 30)
#
# Usage:
#   ./backup-config.sh
#   ENCRYPTION_KEY=$(openssl rand -hex 32) DATABASE_URL=postgresql://... ./backup-config.sh
#

set -euo pipefail

DATABASE_URL="${DATABASE_URL:?DATABASE_URL environment variable is required}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:?ENCRYPTION_KEY environment variable is required}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"

TIMESTAMP="$(date -u +"%Y%m%d_%H%M%S")"
BACKUP_FILE="${BACKUP_DIR}/virtuestack_${TIMESTAMP}.sql.gz.enc"
TEMP_DIR=""

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

validate_encryption_key() {
    local key="$1"
    if [[ ! "${key}" =~ ^[0-9a-fA-F]{64}$ ]]; then
        echo "ERROR: ENCRYPTION_KEY must be a 64-character hex string (32 bytes / 256 bits)" >&2
        exit 1
    fi
}

ensure_backup_dir() {
    mkdir -p "${BACKUP_DIR}"
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

    echo "Encrypting backup with AES-256-GCM..."

    openssl enc -aes-256-gcm \
        -in "${input_file}" \
        -out "${output_file}" \
        -K "${ENCRYPTION_KEY}" \
        -pbkdf2 \
        -iter 100000

    if [[ ! -f "${output_file}" ]]; then
        echo "ERROR: Encryption failed" >&2
        exit 1
    fi
    echo "Encrypted backup: $(du -h "${output_file}" | cut -f1)"
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

validate_encryption_key "${ENCRYPTION_KEY}"
ensure_backup_dir

TEMP_DIR="$(mktemp -d)"
DUMP_FILE="${TEMP_DIR}/virtuestack_${TIMESTAMP}.sql.gz"

dump_database "${DUMP_FILE}"
encrypt_file "${DUMP_FILE}" "${BACKUP_FILE}"
rotate_old_backups
print_summary
