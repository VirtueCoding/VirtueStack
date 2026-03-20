<?php

declare(strict_types=1);

/**
 * VirtueStack Helper Utilities for WHMCS Provisioning Module.
 *
 * Provides utility functions for encryption/decryption,
 * customer API credential management, and SSO token generation.
 *
 * @package   VirtueStack\WHMCS
 * @author    VirtueStack Team
 * @copyright 2026 VirtueStack
 * @license   MIT
 */

namespace VirtueStack\WHMCS;

use InvalidArgumentException;
use RuntimeException;

/**
 * Helper class for VirtueStack WHMCS module utilities.
 */
final class VirtueStackHelper
{
    private const SSO_TOKEN_EXPIRY_HOURS = 1;
    private const JWT_ALGORITHM = 'HS256';

    /**
     * Encrypt a value using WHMCS encryption.
     *
     * Uses WHMCS's built-in encrypt() function which uses AES-256-CBC.
     *
     * @param string $value Plain text value to encrypt
     *
     * @return string Encrypted value (base64 encoded)
     *
     * @throws RuntimeException If encryption fails
     */
    public static function encrypt(string $value): string
    {
        if (empty($value)) {
            return '';
        }

        if (!function_exists('encrypt')) {
            throw new RuntimeException('WHMCS encrypt() function not available');
        }

        $encrypted = encrypt($value);

        if ($encrypted === false) {
            throw new RuntimeException('Failed to encrypt value');
        }

        return $encrypted;
    }

    /**
     * Decrypt a value using WHMCS decryption.
     *
     * @param string $encryptedValue Encrypted value from encrypt()
     *
     * @return string Decrypted plain text value
     *
     * @throws RuntimeException If decryption fails
     */
    public static function decrypt(string $encryptedValue): string
    {
        if (empty($encryptedValue)) {
            return '';
        }

        if (!function_exists('decrypt')) {
            throw new RuntimeException('WHMCS decrypt() function not available');
        }

        $decrypted = decrypt($encryptedValue);

        if ($decrypted === false) {
            throw new RuntimeException('Failed to decrypt value');
        }

        return $decrypted;
    }

    /**
     * Generate a cryptographically secure random token.
     *
     * @param int $length Token length in bytes (will be doubled in hex output)
     *
     * @return string Hex-encoded random token
     *
     * @throws RuntimeException If secure random generation fails
     */
    public static function generateToken(int $length = 32): string
    {
        if ($length < 1) {
            throw new InvalidArgumentException('Token length must be at least 1');
        }

        $bytes = random_bytes($length);
        return bin2hex($bytes);
    }

    /**
     * Generate a random password suitable for VM root access.
     *
     * @param int $length Password length (default 16, max 32)
     *
     * @return string Generated password
     *
     * @throws RuntimeException If generation fails
     */
    public static function generatePassword(int $length = 16): string
    {
        $length = min(max(12, $length), 32);

        // Character sets for strong password
        $lower = 'abcdefghijklmnopqrstuvwxyz';
        $upper = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
        $numbers = '0123456789';
        $special = '!@#$%^&*';
        $all = $lower . $upper . $numbers . $special;

        $password = '';
        $password .= $lower[random_int(0, strlen($lower) - 1)];
        $password .= $upper[random_int(0, strlen($upper) - 1)];
        $password .= $numbers[random_int(0, strlen($numbers) - 1)];
        $password .= $special[random_int(0, strlen($special) - 1)];

        for ($i = 4; $i < $length; $i++) {
            $password .= $all[random_int(0, strlen($all) - 1)];
        }

        // Shuffle to randomize position of required characters
        return str_shuffle($password);
    }

    /**
     * Generate an SSO JWT token for Customer WebUI.
     *
     * Creates a short-lived JWT that authenticates the customer
     * with the Customer WebUI without requiring separate login.
     *
     * NOTE: The API secret is NOT included in the JWT payload.
     * API secrets should never be transmitted in JWTs as they are
     * only base64 encoded, not encrypted. The Customer WebUI should
     * look up the secret securely from the database using api_id.
     *
     * @param string $customerId         Customer UUID from VirtueStack
     * @param string $customerApiId      Customer API ID
     * @param string $jwtSecret          JWT signing secret
     * @param string $issuer             JWT issuer (usually controller URL)
     * @param int    $expiryHours        Token expiry in hours (default 1)
     *
     * @return string JWT token
     *
     * @throws RuntimeException If JWT generation fails
     */
    public static function generateSSOToken(
        string $customerId,
        string $customerApiId,
        string $jwtSecret,
        string $issuer,
        int $expiryHours = self::SSO_TOKEN_EXPIRY_HOURS
    ): string {
        if (empty($customerId) || empty($customerApiId)) {
            throw new InvalidArgumentException('Customer ID and API ID are required');
        }

        if (empty($jwtSecret)) {
            throw new InvalidArgumentException('JWT secret is required');
        }

        $now = time();
        $expiry = $now + ($expiryHours * 3600);

        // JWT header
        $header = [
            'typ' => 'JWT',
            'alg' => self::JWT_ALGORITHM,
        ];

        // JWT payload
        $payload = [
            'iss' => $issuer,
            'sub' => $customerId,
            'aud' => 'customer-webui',
            'iat' => $now,
            'nbf' => $now,
            'exp' => $expiry,
            'api_id' => $customerApiId,
            'type' => 'sso',
        ];

        // Encode header and payload
        $headerEncoded = self::base64UrlEncode(json_encode($header, JSON_THROW_ON_ERROR));
        $payloadEncoded = self::base64UrlEncode(json_encode($payload, JSON_THROW_ON_ERROR));

        // Create signature
        $signature = hash_hmac('sha256', "{$headerEncoded}.{$payloadEncoded}", $jwtSecret, true);
        $signatureEncoded = self::base64UrlEncode($signature);

        return "{$headerEncoded}.{$payloadEncoded}.{$signatureEncoded}";
    }

    /**
     * Validate hostname format (RFC 1123 compliant).
     *
     * @param string $hostname Hostname to validate
     *
     * @return bool True if valid hostname
     */
    public static function isValidHostname(string $hostname): bool
    {
        if (empty($hostname)) {
            return false;
        }

        // Must be 1-63 characters
        $length = strlen($hostname);
        if ($length < 1 || $length > 63) {
            return false;
        }

        // Must start with a letter or digit
        if (!preg_match('/^[a-z0-9]/i', $hostname)) {
            return false;
        }

        // Must end with a letter or digit
        if (!preg_match('/[a-z0-9]$/i', $hostname)) {
            return false;
        }

        // Can only contain letters, digits, and hyphens
        if (!preg_match('/^[a-z0-9-]+$/i', $hostname)) {
            return false;
        }

        // No consecutive hyphens
        if (strpos($hostname, '--') !== false) {
            return false;
        }

        return true;
    }

    /**
     * Validate UUID format.
     *
     * @param string $uuid UUID to validate
     *
     * @return bool True if valid UUID
     */
    public static function isValidUuid(string $uuid): bool
    {
        return (bool) preg_match(
            '/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i',
            $uuid
        );
    }

    /**
     * Get customer API credentials from WHMCS custom fields.
     *
     * Looks up stored credentials for the given client ID.
     * Credentials are stored encrypted in custom fields.
     *
     * @param int    $clientId       WHMCS client ID
     * @param string $apiIdFieldName Custom field name for API ID
     * @param string $apiSecretFieldName Custom field name for API Secret
     *
     * @return array{api_id: string, api_secret: string}|null Credentials or null if not found
     */
    public static function getCustomerCredentials(
        int $clientId,
        string $apiIdFieldName = 'virtuestack_api_id',
        string $apiSecretFieldName = 'virtuestack_api_secret'
    ): ?array {
        if (!function_exists('getCustomFields')) {
            return null;
        }

        $customFields = getCustomFields('client', $clientId);

        if (!is_array($customFields)) {
            return null;
        }

        $apiId = null;
        $apiSecret = null;

        foreach ($customFields as $field) {
            if (isset($field['fieldname'], $field['value'])) {
                $fieldname = strtolower((string) $field['fieldname']);
                
                if ($fieldname === strtolower($apiIdFieldName)) {
                    $apiId = (string) $field['value'];
                } elseif ($fieldname === strtolower($apiSecretFieldName)) {
                    $apiSecret = self::decrypt((string) $field['value']);
                }
            }
        }

        if (empty($apiId) || empty($apiSecret)) {
            return null;
        }

        return [
            'api_id' => $apiId,
            'api_secret' => $apiSecret,
        ];
    }

    /**
     * Store customer API credentials in WHMCS custom fields.
     *
     * @param int    $clientId       WHMCS client ID
     * @param string $apiId          Customer API ID
     * @param string $apiSecret      Customer API Secret (will be encrypted)
     * @param string $apiIdFieldName Custom field name for API ID
     * @param string $apiSecretFieldName Custom field name for API Secret
     *
     * @return bool True if stored successfully
     */
    public static function storeCustomerCredentials(
        int $clientId,
        string $apiId,
        string $apiSecret,
        string $apiIdFieldName = 'virtuestack_api_id',
        string $apiSecretFieldName = 'virtuestack_api_secret'
    ): bool {
        if (!function_exists('saveCustomFields')) {
            return false;
        }

        $encryptedSecret = self::encrypt($apiSecret);

        $fields = [
            $apiIdFieldName => $apiId,
            $apiSecretFieldName => $encryptedSecret,
        ];

        return saveCustomFields('client', $clientId, $fields);
    }

    /**
     * Get module configuration value for a service.
     *
     * @param array  $params WHMCS module parameters
     * @param string $key    Configuration key
     * @param mixed  $default Default value if not found
     *
     * @return mixed Configuration value
     */
    public static function getConfigValue(array $params, string $key, mixed $default = null): mixed
    {
        // Check server config first
        if (isset($params['serverconfig'][$key])) {
            return $params['serverconfig'][$key];
        }

        // Check module config
        if (isset($params['configoption'][$key])) {
            return $params['configoption'][$key];
        }

        // Check custom fields
        if (isset($params['customfields'][$key])) {
            return $params['customfields'][$key];
        }

        return $default;
    }

    /**
     * Log an operation to WHMCS module log.
     *
     * @param string $action    Action being performed
     * @param string $request   Request data (will be sanitized)
     * @param string $response  Response data
     * @param string $data      Additional data
     */
    public static function log(string $action, string $request, string $response, string $data = ''): void
    {
        if (function_exists('logModuleCall')) {
            $request = self::sanitizeLog($request);
            $response = self::sanitizeLog($response);
            $data = self::sanitizeLog($data);
            
            logModuleCall('virtuestack', $action, $request, $response, $data, '');
        }
    }

    /**
     * Sanitize log output to remove sensitive data.
     *
     * @param string $data Data to sanitize
     *
     * @return string Sanitized data
     */
    public static function sanitizeLog(string $data): string
    {
        // Remove passwords
        $data = preg_replace('/"password"\s*:\s*"[^"]*"/i', '"password":"***"', $data);
        $data = preg_replace('/"root_password"\s*:\s*"[^"]*"/i', '"root_password":"***"', $data);
        
        // Remove API keys
        $data = preg_replace('/"api_key"\s*:\s*"[^"]*"/i', '"api_key":"***"', $data);
        $data = preg_replace('/"api_secret"\s*:\s*"[^"]*"/i', '"api_secret":"***"', $data);
        
        // Remove tokens
        $data = preg_replace('/"token"\s*:\s*"[^"]*"/i', '"token":"***"', $data);
        
        // Remove SSH keys (partial)
        $data = preg_replace('/"ssh_keys"\s*:\s*\[[^\]]*\]/i', '"ssh_keys":["***"]', $data);

        return $data;
    }

    /**
     * Build the Customer WebUI URL with SSO token.
     *
     * SECURITY NOTE: The SSO token is passed via query parameter which exposes it
     * in browser history, referrer headers, and server logs. To minimize risk:
     * - Use a short token expiry (recommend 5 minutes)
     * - Tokens should be single-use where possible
     *
     * @param string $webuiUrl   Base Customer WebUI URL
     * @param string $vmId       VM UUID
     * @param string $ssoToken   SSO JWT token (use short expiry)
     *
     * @return string Full URL with token
     */
    public static function buildWebuiUrl(string $webuiUrl, string $vmId, string $ssoToken): string
    {
        $webuiUrl = rtrim($webuiUrl, '/');
        return "{$webuiUrl}/vm/{$vmId}?sso_token={$ssoToken}";
    }

    /**
     * Build the Console WebUI URL with SSO token.
     *
     * SECURITY NOTE: Same concern as buildWebuiUrl(). Use short token expiry.
     *
     * @param string $webuiUrl   Base Customer WebUI URL
     * @param string $vmId       VM UUID
     * @param string $ssoToken   SSO JWT token (use short expiry)
     * @param string $type       Console type: vnc or serial
     *
     * @return string Full URL with token
     */
    public static function buildConsoleUrl(string $webuiUrl, string $vmId, string $ssoToken, string $type = 'vnc'): string
    {
        $webuiUrl = rtrim($webuiUrl, '/');
        $type = in_array($type, ['vnc', 'serial'], true) ? $type : 'vnc';
        return "{$webuiUrl}/vm/{$vmId}/console/{$type}?token={$ssoToken}";
    }

    /**
     * Base64 URL encode (RFC 4648 compliant).
     *
     * @param string $data Data to encode
     *
     * @return string URL-safe base64 encoded string
     */
    private static function base64UrlEncode(string $data): string
    {
        return rtrim(strtr(base64_encode($data), '+/', '-_'), '=');
    }

    /**
     * Generate a unique VM ID based on WHMCS service ID.
     *
     * Note: The actual VM ID is generated by the Controller.
     * This is a helper for consistent internal references.
     *
     * @param int $serviceId WHMCS service ID
     *
     * @return string Prefixed identifier
     */
    public static function generateVmReference(int $serviceId): string
    {
        return "vs-svc-{$serviceId}";
    }

    /**
     * Parse a correlation ID from API response headers or generate one.
     *
     * @param string|null $correlationId Correlation ID from response
     *
     * @return string Correlation ID
     */
    public static function ensureCorrelationId(?string $correlationId): string
    {
        if (!empty($correlationId)) {
            return $correlationId;
        }

        return 'whmcs-' . self::generateToken(8);
    }
}