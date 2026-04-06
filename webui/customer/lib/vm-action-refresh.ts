export class VMActionRefreshError extends Error {
  override cause: unknown;

  constructor(message: string, cause: unknown) {
    super(message);
    this.name = "VMActionRefreshError";
    this.cause = cause;
  }
}

export async function completeVMActionWithRefresh(
  mutate: () => Promise<void>,
  refresh?: () => Promise<void> | void,
): Promise<void> {
  await mutate();
  if (!refresh) {
    return;
  }

  try {
    await refresh();
  } catch (error) {
    throw new VMActionRefreshError(
      "VM action succeeded but the VM view could not be refreshed.",
      error,
    );
  }
}
