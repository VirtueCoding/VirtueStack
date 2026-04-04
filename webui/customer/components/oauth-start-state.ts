function formatProviderLabel(provider: string): string {
  switch (provider.toLowerCase()) {
    case "github":
      return "GitHub";
    case "google":
      return "Google";
    default:
      return "OAuth";
  }
}

export function getOAuthStartFailureMessage(provider: string): string {
  return `Unable to start ${formatProviderLabel(provider)} sign-in right now. Please try again.`;
}
