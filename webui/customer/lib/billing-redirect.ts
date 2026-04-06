export function getSafeTopUpRedirectURL(paymentURL: string): string | null {
  try {
    const parsed = new URL(paymentURL);
    if (
      parsed.protocol !== "https:" ||
      parsed.hostname === "" ||
      parsed.username !== "" ||
      parsed.password !== ""
    ) {
      return null;
    }
    return parsed.toString();
  } catch {
    return null;
  }
}
