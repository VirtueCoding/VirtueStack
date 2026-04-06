<?php

declare(strict_types=1);

/**
 * VirtueStack WHMCS Module - Shared Helper Functions.
 *
 * Functions shared between hooks.php, webhook.php, and virtuestack.php
 * to avoid code duplication. Uses Capsule (Eloquent) for database access.
 *
 * @package   VirtueStack\WHMCS
 * @author    VirtueStack Team
 * @copyright 2026 VirtueStack
 * @license   MIT
 */

use WHMCS\Database\Capsule;

/** Regex pattern for validating UUID v4 format (case-insensitive). */
const UUID_PATTERN = '/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i';

/**
 * Get custom field ID by name.
 *
 * @param string $fieldName Field name
 *
 * @return int Field ID or 0 if not found
 */
function getCustomFieldId(string $fieldName): int
{
    try {
        return (int) Capsule::table('tblcustomfields')
            ->where('fieldname', $fieldName)
            ->where('type', 'product')
            ->value('id') ?? 0;
    } catch (\Exception $e) {
        return 0;
    }
}

/**
 * Validate a custom field value against the expected format for its field name.
 *
 * Known fields are checked strictly (UUIDs, IPs, statuses). Unknown fields
 * are capped at 500 characters and stripped of control characters.
 *
 * @param string $fieldName Field name
 * @param string $value     Value to validate
 *
 * @return string|null Sanitised value, or null if validation fails
 */
function validateFieldValue(string $fieldName, string $value): ?string
{
    // Empty values are always accepted (used to clear fields)
    if ($value === '') {
        return '';
    }

    $validStatuses = [
        'running', 'stopped', 'suspended', 'provisioning',
        'migrating', 'reinstalling', 'paused', 'shutting_down',
        'error', 'crashed', 'deleted',
    ];
    $validProvisioningStatuses = [
        'pending', 'resizing', 'active', 'error', 'terminated', 'suspended',
    ];

    switch ($fieldName) {
        case 'vm_id':
        case 'node_id':
        case 'virtuestack_customer_id':
            if (!preg_match(UUID_PATTERN, $value)) {
                return null;
            }
            return $value;

        case 'vm_ip':
            if (!filter_var($value, FILTER_VALIDATE_IP)) {
                return null;
            }
            return $value;

        case 'vm_status':
            if (!in_array($value, $validStatuses, true)) {
                return null;
            }
            return $value;

        case 'provisioning_status':
            if (!in_array($value, $validProvisioningStatuses, true)) {
                return null;
            }
            return $value;

        case 'task_id':
            // Task IDs are UUIDs or empty (when clearing)
            if (!preg_match(UUID_PATTERN, $value)) {
                return null;
            }
            return $value;

        default:
            // Unknown fields: cap at 500 chars (WHMCS tblcustomfieldsvalues.value is TEXT,
            // but we bound it to a sane limit to prevent abuse) and strip control chars.
            $value = substr($value, 0, 500);
            $value = preg_replace('/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/', '', $value);
            return $value;
    }
}

/**
 * Return provisioning statuses that represent in-flight async operations.
 *
 * @return list<string>
 */
function virtuestack_getAsyncPollableProvisioningStatuses(): array
{
    return ['pending', 'resizing'];
}

/**
 * Extract canonical webhook context from either the live controller envelope or
 * the legacy flat payload used by older module integrations.
 *
 * @param array<string, mixed> $payload
 *
 * @return array{
 *     event:string,
 *     task_id:string,
 *     vm_id:string,
 *     external_service_id:int,
 *     timestamp:string,
 *     idempotency_key:string,
 *     event_data:array<string, mixed>
 * }
 */
function virtuestack_extractWebhookContext(array $payload): array
{
    $eventData = [];
    if (isset($payload['data']) && is_array($payload['data'])) {
        $eventData = $payload['data'];
    } elseif (isset($payload['result']) && is_array($payload['result'])) {
        $eventData = $payload['result'];
    }

    $taskID = '';
    if (isset($payload['task_id']) && is_string($payload['task_id'])) {
        $taskID = trim($payload['task_id']);
    } elseif (isset($eventData['task_id']) && is_string($eventData['task_id'])) {
        $taskID = trim($eventData['task_id']);
    }

    $vmID = '';
    if (isset($payload['vm_id']) && is_string($payload['vm_id'])) {
        $vmID = trim($payload['vm_id']);
    } elseif (isset($eventData['vm_id']) && is_string($eventData['vm_id'])) {
        $vmID = trim($eventData['vm_id']);
    }

    $externalServiceID = 0;
    if (isset($payload['external_service_id']) && is_int($payload['external_service_id'])) {
        $externalServiceID = $payload['external_service_id'];
    } elseif (isset($eventData['external_service_id']) && is_int($eventData['external_service_id'])) {
        $externalServiceID = $eventData['external_service_id'];
    }

    return [
        'event' => isset($payload['event']) && is_string($payload['event']) ? trim($payload['event']) : '',
        'task_id' => $taskID,
        'vm_id' => $vmID,
        'external_service_id' => $externalServiceID,
        'timestamp' => isset($payload['timestamp']) && is_string($payload['timestamp']) ? trim($payload['timestamp']) : '',
        'idempotency_key' => isset($payload['idempotency_key']) && is_string($payload['idempotency_key']) ? trim($payload['idempotency_key']) : '',
        'event_data' => $eventData,
    ];
}

/**
 * Validate a canonical webhook context against the supported event list.
 *
 * @param array{
 *     event:string,
 *     task_id:string,
 *     vm_id:string,
 *     external_service_id:int,
 *     timestamp:string,
 *     idempotency_key:string,
 *     event_data:array<string, mixed>
 * } $context
 * @param list<string> $allowedEvents
 */
function virtuestack_validateWebhookContext(array $context, array $allowedEvents): ?string
{
    $eventType = $context['event'];
    if ($eventType === '') {
        return 'Missing required field: event';
    }

    if (!in_array($eventType, $allowedEvents, true)) {
        return 'Unknown event type';
    }

    if ($context['task_id'] !== '' && !preg_match(UUID_PATTERN, $context['task_id'])) {
        return 'Invalid task_id format';
    }

    if ($context['vm_id'] !== '' && !preg_match(UUID_PATTERN, $context['vm_id'])) {
        return 'Invalid vm_id format';
    }

    return null;
}

/**
 * Build the service-field updates for a successful async task.
 *
 * @param string $currentProvisioningStatus Current provisioning status
 *
 * @return array<string, string>
 */
function virtuestack_getAsyncTaskCompletionUpdates(string $currentProvisioningStatus): array
{
    unset($currentProvisioningStatus);

    return [
        'task_id' => '',
        'provisioning_error' => '',
        'provisioning_status' => 'active',
    ];
}

/**
 * Build the service-field updates for a failed async task.
 *
 * Resize failures should preserve access to an already-provisioned VM, while
 * create failures must surface the service-level error state.
 *
 * @param string $currentProvisioningStatus Current provisioning status
 * @param string $errorMessage              Failure reason
 *
 * @return array<string, string>
 */
function virtuestack_getAsyncTaskFailureUpdates(string $currentProvisioningStatus, string $errorMessage): array
{
    $provisioningStatus = $currentProvisioningStatus === 'resizing' ? 'active' : 'error';

    return [
        'task_id' => '',
        'provisioning_error' => $errorMessage,
        'provisioning_status' => $provisioningStatus,
    ];
}

/**
 * Determine whether a completed async task should send the provisioning email.
 */
function virtuestack_shouldSendProvisioningWelcomeEmail(string $taskType): bool
{
    return $taskType === 'vm.create';
}

/**
 * Map controller VM runtime states onto WHMCS provisioning-status values.
 */
function virtuestack_mapVMStatusToProvisioningStatus(string $vmStatus): string
{
    return match ($vmStatus) {
        'provisioning' => 'pending',
        'running', 'stopped', 'migrating', 'reinstalling', 'paused', 'shutting_down' => 'active',
        'suspended' => 'suspended',
        'error', 'crashed' => 'error',
        'deleted' => 'terminated',
        default => 'active',
    };
}

/**
 * Apply multiple validated custom-field updates to a service.
 *
 * @param int                  $serviceId Service ID
 * @param array<string,string> $updates   Field updates
 */
function virtuestack_applyServiceFieldUpdates(int $serviceId, array $updates): void
{
    foreach ($updates as $fieldName => $value) {
        updateServiceField($serviceId, $fieldName, $value);
    }
}

/**
 * Update service custom field value.
 *
 * Values are validated against expected formats before storage.
 *
 * @param int    $serviceId Service ID
 * @param string $fieldName Field name
 * @param string $value     New value
 */
function updateServiceField(int $serviceId, string $fieldName, string $value): void
{
    try {
        $validated = validateFieldValue($fieldName, $value);
        if ($validated === null) {
            logActivity("VirtueStack: Rejected invalid value for field {$fieldName} on service {$serviceId}");
            return;
        }
        $value = $validated;

        $fieldId = getCustomFieldId($fieldName);

        if ($fieldId <= 0) {
            $fieldId = createCustomField($fieldName);
        }

        if ($fieldId <= 0) {
            return;
        }

        $exists = Capsule::table('tblcustomfieldsvalues')
            ->where('relid', $serviceId)
            ->where('fieldid', $fieldId)
            ->exists();

        if ($exists) {
            Capsule::table('tblcustomfieldsvalues')
                ->where('relid', $serviceId)
                ->where('fieldid', $fieldId)
                ->update(['value' => $value]);
        } else {
            Capsule::table('tblcustomfieldsvalues')->insert([
                'fieldid' => $fieldId,
                'relid' => $serviceId,
                'value' => $value,
            ]);
        }
    } catch (\Exception $e) {
        logActivity("VirtueStack: Failed to update service field {$fieldName}: " . $e->getMessage());
    }
}

/**
 * Create custom field for VM data.
 *
 * @param string $fieldName Field name
 *
 * @return int Field ID or 0 on failure
 */
function createCustomField(string $fieldName): int
{
    try {
        return (int) Capsule::table('tblcustomfields')->insertGetId([
            'type' => 'product',
            'relid' => 0,
            'fieldname' => $fieldName,
            'fieldtype' => 'text',
            'adminonly' => 'on',
            'created_at' => date('Y-m-d H:i:s'),
            'updated_at' => date('Y-m-d H:i:s'),
        ]);
    } catch (\Exception $e) {
        logActivity('VirtueStack: Failed to create custom field: ' . $e->getMessage());
        return 0;
    }
}

/**
 * Find service ID by task ID.
 *
 * @param string $taskId Task UUID
 *
 * @return int Service ID or 0 if not found
 */
function findServiceByTaskId(string $taskId): int
{
    try {
        $fieldId = getCustomFieldId('task_id');
        if ($fieldId <= 0) {
            return 0;
        }

        return (int) Capsule::table('tblcustomfieldsvalues')
            ->where('fieldid', $fieldId)
            ->where('value', $taskId)
            ->value('relid') ?? 0;
    } catch (\Exception $e) {
        return 0;
    }
}

/**
 * Find service ID by VM ID.
 *
 * @param string $vmId VM UUID
 *
 * @return int Service ID or 0 if not found
 */
function findServiceByVmId(string $vmId): int
{
    try {
        $fieldId = getCustomFieldId('vm_id');
        if ($fieldId <= 0) {
            return 0;
        }

        return (int) Capsule::table('tblcustomfieldsvalues')
            ->where('fieldid', $fieldId)
            ->where('value', $vmId)
            ->value('relid') ?? 0;
    } catch (\Exception $e) {
        return 0;
    }
}

/**
 * Verify webhook signature.
 *
 * Computes HMAC-SHA256 regardless of input validity to prevent timing
 * side-channels that could reveal whether a secret is configured.
 *
 * @param string $body      Request body
 * @param string $signature Signature from the X-Webhook-Signature header
 *
 * @return bool True if valid
 */
function verifyWebhookSignature(string $body, string $signature): bool
{
    $webhookSecret = getWebhookSecret();
    $normalizedSignature = normalizeWebhookSignature($signature);

    // Always compute the HMAC so the function takes constant time
    // regardless of whether the secret or signature is present.
    // When the secret is missing we still need to burn equivalent CPU
    // time, so we derive a throwaway key from the body itself.
    $expectedSignature = !empty($webhookSecret)
        ? hash_hmac('sha256', $body, $webhookSecret)
        : hash_hmac('sha256', $body, hash('sha256', $body));

    if (empty($normalizedSignature) || empty($webhookSecret)) {
        return false;
    }

    return hash_equals($expectedSignature, $normalizedSignature);
}

/**
 * Normalize webhook signatures so modules accept the controller's raw hex HMAC
 * and legacy "sha256=" prefixed variants during rollout.
 *
 * @param string $signature Signature header value
 *
 * @return string Normalized raw hex signature
 */
function normalizeWebhookSignature(string $signature): string
{
    $normalized = trim($signature);
    if (stripos($normalized, 'sha256=') === 0) {
        return substr($normalized, 7);
    }

    return $normalized;
}

/**
 * Get webhook secret from WHMCS configuration.
 *
 * @return string
 */
function getWebhookSecret(): string
{
    try {
        $value = Capsule::table('tblconfiguration')
            ->where('setting', 'VirtueStackWebhookSecret')
            ->value('value');

        return $value ? decrypt($value) : '';
    } catch (\Exception $e) {
        return '';
    }
}

/**
 * Update service dedicated IP.
 *
 * @param int    $serviceId Service ID
 * @param string $ipAddress IP address
 */
function updateServiceDedicatedIp(int $serviceId, string $ipAddress): void
{
    try {
        Capsule::table('tblhosting')
            ->where('id', $serviceId)
            ->update(['dedicatedip' => $ipAddress]);
    } catch (\Exception $e) {
        logActivity("VirtueStack: Failed to update dedicated IP for service {$serviceId}: " . $e->getMessage());
    }
}

/**
 * Get available plans from VirtueStack API.
 *
 * Used for product configuration validation and dropdown population.
 *
 * @param array $serverParams Server parameters from WHMCS
 *
 * @return array List of plans with id, name, specs
 */
function getAvailablePlans(array $serverParams): array
{
    try {
        $apiUrl = buildApiUrl($serverParams);
        $apiKey = $serverParams['password'] ?? '';

        if (empty($apiKey)) {
            return [];
        }

        $client = new \VirtueStack\WHMCS\ApiClient(
            $apiUrl,
            $apiKey,
            30,
            true
        );

        return $client->listPlans();
    } catch (\Exception $e) {
        logActivity('VirtueStack: Failed to fetch available plans: ' . $e->getMessage());
        return [];
    }
}

/**
 * Build API URL from server parameters.
 *
 * @param array $serverParams Server parameters from WHMCS
 *
 * @return string API URL
 */
function buildApiUrl(array $serverParams): string
{
    $hostname = $serverParams['hostname'] ?? $serverParams['ipaddress'] ?? '';
    $secure = ($serverParams['secure'] ?? 'on') === 'on';
    $port = (int) ($serverParams['port'] ?: 443);

    $protocol = $secure ? 'https' : 'http';

    return "{$protocol}://{$hostname}:{$port}/api/v1";
}

/**
 * Validate product configuration before saving.
 *
 * Can be called from a WHMCS hook to validate Plan ID and Template ID
 * when an admin saves product configuration.
 *
 * @param array $configOptions Configuration options (Plan ID, Template ID, etc.)
 * @param array $serverParams  Server parameters for API access
 *
 * @return array Validation results with 'valid' bool and 'errors' array
 */
function validateProductConfiguration(array $configOptions, array $serverParams): array
{
    $errors = [];

    // Validate Plan ID
    $planId = $configOptions['Plan ID'] ?? '';
    if (!empty($planId)) {
        if (!preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $planId)) {
            $errors[] = 'Plan ID must be a valid UUID format.';
        } else {
            // Try to validate against API
            try {
                $apiUrl = buildApiUrl($serverParams);
                $apiKey = $serverParams['password'] ?? '';

                if (!empty($apiKey)) {
                    $client = new \VirtueStack\WHMCS\ApiClient($apiUrl, $apiKey, 10, true);
                    if (!$client->planExists($planId)) {
                        $errors[] = "Plan ID '{$planId}' not found or is inactive in VirtueStack.";
                    }
                }
            } catch (\Exception $e) {
                // API validation failed - log but don't block save
                logActivity('VirtueStack: Could not validate Plan ID: ' . $e->getMessage());
            }
        }
    }

    // Validate Template ID
    $templateId = $configOptions['Template ID'] ?? '';
    if (!empty($templateId)) {
        if (!preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $templateId)) {
            $errors[] = 'Template ID must be a valid UUID format.';
        }
    }

    return [
        'valid'  => empty($errors),
        'errors' => $errors,
    ];
}
