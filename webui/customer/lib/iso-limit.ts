function formatISOLimit(bytes: number): string {
  const gib = 1024 * 1024 * 1024;
  const sizeInGiB = bytes / gib;
  const formatted = Number.isInteger(sizeInGiB) ? sizeInGiB.toString() : sizeInGiB.toFixed(1);
  return `${formatted} GB`;
}

export function getISOUploadDescription(maxISOSizeBytes?: number): string {
  if (!maxISOSizeBytes || maxISOSizeBytes <= 0) {
    return "Upload an ISO image to attach to this VM.";
  }

  return `Upload an ISO image to attach to this VM. Maximum file size is ${formatISOLimit(maxISOSizeBytes)}.`;
}

export function getISOUploadHint(maxISOSizeBytes?: number): string {
  if (!maxISOSizeBytes || maxISOSizeBytes <= 0) {
    return "Only .iso files are accepted";
  }

  return `Only .iso files are accepted (max ${formatISOLimit(maxISOSizeBytes)})`;
}

export function validateISOUploadFile(
  file: { name: string; size: number },
  maxISOSizeBytes?: number,
): string | null {
  if (!file.name.toLowerCase().endsWith(".iso")) {
    return "Only .iso files are allowed";
  }

  if (maxISOSizeBytes && maxISOSizeBytes > 0 && file.size > maxISOSizeBytes) {
    return `File size exceeds the ${formatISOLimit(maxISOSizeBytes)} limit`;
  }

  return null;
}
