export class ISOMutationRefreshError extends Error {
  override cause: unknown;

  constructor(message: string, cause: unknown) {
    super(message);
    this.name = "ISOMutationRefreshError";
    this.cause = cause;
  }
}

export async function loadMutationRefreshData<T>(
  load: () => Promise<T>,
  apply: (data: T) => void,
): Promise<void> {
  const data = await load();
  apply(data);
}

export async function refreshISOMutationState(
  refreshISOs: () => Promise<void>,
  refreshVM: () => Promise<void>,
): Promise<void> {
  await Promise.all([refreshISOs(), refreshVM()]);
}

export async function completeISOMutationWithRefresh<T>(
  mutate: () => Promise<T>,
  refreshISOs: () => Promise<void>,
  refreshVM: () => Promise<void>,
  onSuccess: () => void,
): Promise<void> {
  await mutate();
  try {
    await refreshISOMutationState(refreshISOs, refreshVM);
  } catch (error) {
    throw new ISOMutationRefreshError(
      "ISO change succeeded but the VM view could not be refreshed.",
      error,
    );
  }
  onSuccess();
}
