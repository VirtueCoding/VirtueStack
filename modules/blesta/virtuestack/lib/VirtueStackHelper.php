<?php
/**
 * VirtueStack Helper Utilities for Blesta
 *
 * Static utility class providing validation, cryptographic,
 * and data extraction helpers for the VirtueStack module.
 *
 * @package blesta
 * @subpackage blesta.components.modules.virtuestack
 */
class VirtueStackHelper
{
    private const UUID_PATTERN = '/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i';

    /**
     * Verify a webhook HMAC-SHA256 signature using timing-safe comparison.
     *
     * @param string $body Raw request body
     * @param string $signature Signature from X-VirtueStack-Signature header
     * @param string $secret Webhook shared secret
     * @return bool True if signature is valid
     */
    public static function verifyWebhookSignature(
        string $body,
        string $signature,
        string $secret
    ): bool {
        $computed = 'sha256=' . hash_hmac('sha256', $body, $secret);

        return hash_equals($computed, $signature);
    }

    /**
     * Validate a UUID string.
     *
     * @param string $value Value to validate
     * @return bool True if valid UUID
     */
    public static function isValidUUID(string $value): bool
    {
        return (bool) preg_match(self::UUID_PATTERN, $value);
    }

    /**
     * Build the SSO exchange URL for the customer portal.
     *
     * @param string $webuiUrl Base WebUI URL
     * @param string $token SSO token
     * @return string Full SSO URL
     */
    public static function buildSSOUrl(string $webuiUrl, string $token): string
    {
        $webuiUrl = rtrim($webuiUrl, '/');

        return $webuiUrl . '/api/v1/customer/auth/sso-exchange?token='
            . urlencode($token);
    }

    /**
     * Build the console URL (currently same as SSO URL).
     *
     * @param string $webuiUrl Base WebUI URL
     * @param string $token SSO token
     * @return string Full console URL
     */
    public static function buildConsoleUrl(string $webuiUrl, string $token): string
    {
        return self::buildSSOUrl($webuiUrl, $token);
    }

}
