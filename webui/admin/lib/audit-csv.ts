function sanitizeAuditCSVCell(value: string): string {
  if (/^[=+\-@]/.test(value)) {
    return `'${value}`;
  }

  return value;
}

function escapeAuditCSVCell(value: string): string {
  return `"${sanitizeAuditCSVCell(value).replace(/"/g, '""')}"`;
}

export function buildAuditLogCSV(
  rows: ReadonlyArray<ReadonlyArray<string>>,
): string {
  return rows
    .map((row) => row.map((cell) => escapeAuditCSVCell(String(cell))).join(","))
    .join("\n");
}
