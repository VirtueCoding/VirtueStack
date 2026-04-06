export function shouldTreatProfileErrorAsUnauthenticated(error: unknown): boolean {
  if (
    typeof error === "object" &&
    error !== null &&
    "status" in error &&
    typeof error.status === "number"
  ) {
    return error.status === 401 || error.status === 403;
  }

  return false;
}
