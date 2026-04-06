export class MutationRefreshError extends Error {
  override cause: unknown;

  constructor(message: string, cause: unknown) {
    super(message);
    this.name = "MutationRefreshError";
    this.cause = cause;
  }
}

export async function completeMutationWithRefresh(
  mutate: () => Promise<void>,
  refresh: () => Promise<void>,
  onSuccess: () => void,
  refreshErrorMessage: string,
): Promise<void> {
  await mutate();

  try {
    await refresh();
  } catch (error) {
    throw new MutationRefreshError(refreshErrorMessage, error);
  }

  onSuccess();
}
