/**
 * OAuth PKCE (Proof Key for Code Exchange) utilities.
 *
 * Implements the client-side portion of the OAuth 2.0 PKCE flow:
 * 1. Generate a high-entropy random code_verifier
 * 2. Hash it with SHA-256 to produce the code_challenge
 * 3. Store verifier + state in sessionStorage for callback validation
 */

/** Generate a cryptographically random string for PKCE code_verifier. */
export function generateCodeVerifier(): string {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return base64URLEncode(array);
}

/** Compute the S256 code_challenge from a code_verifier. */
export async function generateCodeChallenge(
  verifier: string
): Promise<string> {
  const data = new TextEncoder().encode(verifier);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return base64URLEncode(new Uint8Array(digest));
}

/** Generate a random state parameter for CSRF protection. */
export function generateState(): string {
  const array = new Uint8Array(16);
  crypto.getRandomValues(array);
  return base64URLEncode(array);
}

function base64URLEncode(buffer: Uint8Array): string {
  let binary = "";
  for (const byte of buffer) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

const OAUTH_STORAGE_KEY = "oauth_pkce_state";
const OAUTH_STORAGE_KEY_PREFIX = `${OAUTH_STORAGE_KEY}:`;
const OAUTH_STATE_MAX_AGE_MS = 10 * 60 * 1000;

export type OAuthFlowMode = "login" | "link";

export interface OAuthStoredState {
  codeVerifier: string;
  state: string;
  provider: string;
  redirectURI: string;
  timestamp: number;
  mode?: OAuthFlowMode;
  returnTo?: string;
}

/** Persist PKCE state before redirecting to the OAuth provider. */
export function storeOAuthState(params: Omit<OAuthStoredState, "timestamp">): void {
  cleanupExpiredOAuthStates();
  const data: OAuthStoredState = { ...params, timestamp: Date.now() };
  sessionStorage.setItem(getOAuthStorageKey(params.state), JSON.stringify(data));
}

/**
 * Retrieve and clear the stored PKCE state after the OAuth redirect.
 * Returns null if no state is found, state is expired (>10 min), or
 * the state parameter doesn't match.
 */
export function retrieveOAuthState(
  expectedState: string
): OAuthStoredState | null {
  cleanupExpiredOAuthStates();

  const storageKey = getOAuthStorageKey(expectedState);
  const raw = sessionStorage.getItem(storageKey) ?? sessionStorage.getItem(OAUTH_STORAGE_KEY);
  if (!raw) return null;

  sessionStorage.removeItem(storageKey);
  sessionStorage.removeItem(OAUTH_STORAGE_KEY);

  try {
    const stored: OAuthStoredState = JSON.parse(raw);
    // Validate state matches (CSRF protection)
    if (stored.state !== expectedState) return null;
    // Expire after 10 minutes
    if (Date.now() - stored.timestamp > OAUTH_STATE_MAX_AGE_MS) return null;
    return stored;
  } catch {
    return null;
  }
}

function getOAuthStorageKey(state: string): string {
  return `${OAUTH_STORAGE_KEY_PREFIX}${state}`;
}

function cleanupExpiredOAuthStates(): void {
  const now = Date.now();

  for (let index = sessionStorage.length - 1; index >= 0; index -= 1) {
    const key = sessionStorage.key(index);
    if (!key || !key.startsWith(OAUTH_STORAGE_KEY_PREFIX)) continue;

    const raw = sessionStorage.getItem(key);
    if (!raw) {
      sessionStorage.removeItem(key);
      continue;
    }

    const stored = parseStoredState(raw);
    if (!stored || now - stored.timestamp > OAUTH_STATE_MAX_AGE_MS) {
      sessionStorage.removeItem(key);
    }
  }

  const legacyRaw = sessionStorage.getItem(OAUTH_STORAGE_KEY);
  if (!legacyRaw) return;

  const legacyStored = parseStoredState(legacyRaw);
  if (!legacyStored || now - legacyStored.timestamp > OAUTH_STATE_MAX_AGE_MS) {
    sessionStorage.removeItem(OAUTH_STORAGE_KEY);
  }
}

function parseStoredState(raw: string): OAuthStoredState | null {
  try {
    return JSON.parse(raw) as OAuthStoredState;
  } catch {
    return null;
  }
}
