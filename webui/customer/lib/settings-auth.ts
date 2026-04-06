export interface SettingsAuthGate {
  isAuthenticated: boolean;
  isLoading: boolean;
  hasBootstrapError: boolean;
}

export function shouldEnableSettingsQueries(state: SettingsAuthGate): boolean {
  return !state.isLoading && state.isAuthenticated && !state.hasBootstrapError;
}
