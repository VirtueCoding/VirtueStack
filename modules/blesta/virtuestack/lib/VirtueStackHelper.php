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
     * @param string $signature Signature from the X-Webhook-Signature header
     * @param string $secret Webhook shared secret
     * @return bool True if signature is valid
     */
    public static function verifyWebhookSignature(
        string $body,
        string $signature,
        string $secret
    ): bool {
        $computed = hash_hmac('sha256', $body, $secret);
        $normalizedSignature = self::normalizeWebhookSignature($signature);

        return $normalizedSignature !== '' && hash_equals($computed, $normalizedSignature);
    }

    /**
     * Normalize webhook signatures so Blesta accepts the controller's raw hex
     * HMAC and the legacy "sha256=" prefixed format during rollout.
     *
     * @param string $signature Signature header value
     * @return string Normalized raw hex signature
     */
    public static function normalizeWebhookSignature(string $signature): string
    {
        $normalized = trim($signature);
        if (stripos($normalized, 'sha256=') === 0) {
            return substr($normalized, 7);
        }

        return $normalized;
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
     * Extract canonical webhook context from either the live controller envelope
     * or the older flat payload shape.
     *
     * @param array<string, mixed> $payload
     * @return array{
     *     event:string,
     *     external_service_id:int|null,
     *     vm_id:string,
     *     task_id:string,
     *     data:array<string, mixed>,
     *     error_message:string
     * }
     */
    public static function extractWebhookContext(array $payload): array
    {
        $data = [];
        if (isset($payload['data']) && is_array($payload['data'])) {
            $data = $payload['data'];
        }

        $externalServiceID = null;
        if (isset($payload['external_service_id']) && is_int($payload['external_service_id'])) {
            $externalServiceID = $payload['external_service_id'];
        } elseif (isset($data['external_service_id']) && is_int($data['external_service_id'])) {
            $externalServiceID = $data['external_service_id'];
        }

        $vmID = '';
        if (isset($payload['vm_id']) && is_string($payload['vm_id'])) {
            $vmID = trim($payload['vm_id']);
        } elseif (isset($data['vm_id']) && is_string($data['vm_id'])) {
            $vmID = trim($data['vm_id']);
        }

        $taskID = '';
        if (isset($payload['task_id']) && is_string($payload['task_id'])) {
            $taskID = trim($payload['task_id']);
        } elseif (isset($data['task_id']) && is_string($data['task_id'])) {
            $taskID = trim($data['task_id']);
        }

        $errorMessage = '';
        if (isset($payload['error']) && is_string($payload['error'])) {
            $errorMessage = $payload['error'];
        } elseif (isset($data['error']) && is_string($data['error'])) {
            $errorMessage = $data['error'];
        } elseif (isset($data['message']) && is_string($data['message'])) {
            $errorMessage = $data['message'];
        }

        return [
            'event' => isset($payload['event']) && is_string($payload['event']) ? trim($payload['event']) : '',
            'external_service_id' => $externalServiceID,
            'vm_id' => $vmID,
            'task_id' => $taskID,
            'data' => $data,
            'error_message' => $errorMessage,
        ];
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
