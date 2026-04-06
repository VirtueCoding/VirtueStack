export async function completeDialogAction(
  action: () => Promise<void>,
  onSuccess: () => void,
): Promise<boolean> {
  try {
    await action();
  } catch {
    return false;
  }

  onSuccess();
  return true;
}
