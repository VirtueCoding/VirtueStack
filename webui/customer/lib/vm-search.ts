export function getURLSyncedSearchTerm(
  currentSearchTerm: string,
  searchParam: string | null,
): string {
  const nextSearchTerm = searchParam ?? "";
  return currentSearchTerm === nextSearchTerm ? currentSearchTerm : nextSearchTerm;
}
