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

    private const VALID_VM_STATUSES = [
        'running',
        'stopped',
        'suspended',
        'provisioning',
        'migrating',
        'reinstalling',
        'error',
        'deleted',
    ];

    private const PASSWORD_UPPER = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
    private const PASSWORD_LOWER = 'abcdefghijklmnopqrstuvwxyz';
    private const PASSWORD_DIGITS = '0123456789';
    private const PASSWORD_SPECIAL = '!@#$%^&*';

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
     * Validate an IP address (IPv4 or IPv6).
     *
     * @param string $value Value to validate
     * @return bool True if valid IP
     */
    public static function isValidIP(string $value): bool
    {
        return filter_var($value, FILTER_VALIDATE_IP) !== false;
    }

    /**
     * Validate a VM status string.
     *
     * @param string $value Status to validate
     * @return bool True if valid status
     */
    public static function isValidVMStatus(string $value): bool
    {
        return in_array($value, self::VALID_VM_STATUSES, true);
    }

    /**
     * Generate a cryptographically secure password.
     *
     * Guarantees at least one uppercase, one lowercase, one digit,
     * and one special character. Uses random_int() for all randomness.
     *
     * @param int $length Password length (min 8, max 128)
     * @return string Generated password
     * @throws InvalidArgumentException If length is out of range
     */
    public static function generatePassword(int $length = 16): string
    {
        $length = max(8, min(128, $length));

        $allChars = self::PASSWORD_UPPER
            . self::PASSWORD_LOWER
            . self::PASSWORD_DIGITS
            . self::PASSWORD_SPECIAL;
        $allLen = strlen($allChars);

        $password = '';
        $password .= self::PASSWORD_UPPER[random_int(0, strlen(self::PASSWORD_UPPER) - 1)];
        $password .= self::PASSWORD_LOWER[random_int(0, strlen(self::PASSWORD_LOWER) - 1)];
        $password .= self::PASSWORD_DIGITS[random_int(0, strlen(self::PASSWORD_DIGITS) - 1)];
        $password .= self::PASSWORD_SPECIAL[random_int(0, strlen(self::PASSWORD_SPECIAL) - 1)];

        for ($i = 4; $i < $length; $i++) {
            $password .= $allChars[random_int(0, $allLen - 1)];
        }

        // Fisher-Yates shuffle with random_int()
        $chars = str_split($password);
        $count = count($chars);
        for ($i = $count - 1; $i > 0; $i--) {
            $j = random_int(0, $i);
            $temp = $chars[$i];
            $chars[$i] = $chars[$j];
            $chars[$j] = $temp;
        }

        return implode('', $chars);
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

    /**
     * Extract a service field value from a Blesta service object.
     *
     * @param object $service Blesta service object
     * @param string $fieldName Field key name
     * @return string|null Field value or null if not found
     */
    public static function getServiceField($service, string $fieldName): ?string
    {
        if (!isset($service->fields) || !is_array($service->fields)) {
            return null;
        }

        foreach ($service->fields as $field) {
            if (isset($field->key) && $field->key === $fieldName) {
                return $field->value ?? null;
            }
        }

        return null;
    }

    /**
     * Extract a value from a module row's meta data.
     *
     * @param object $moduleRow Blesta module row object
     * @param string $key Meta key
     * @return string|null Meta value or null if not found
     */
    public static function getModuleRowMeta($moduleRow, string $key): ?string
    {
        if (!isset($moduleRow->meta) || !is_object($moduleRow->meta)) {
            return null;
        }

        return $moduleRow->meta->{$key} ?? null;
    }

    /**
     * Extract a value from a package's meta data.
     *
     * @param object $package Blesta package object
     * @param string $key Meta key
     * @return string|null Meta value or null if not found
     */
    public static function getPackageMeta($package, string $key): ?string
    {
        if (!isset($package->meta) || !is_object($package->meta)) {
            return null;
        }

        return $package->meta->{$key} ?? null;
    }
}
