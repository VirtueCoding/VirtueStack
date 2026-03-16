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
 * Update service custom field value.
 *
 * @param int    $serviceId Service ID
 * @param string $fieldName Field name
 * @param string $value     New value
 */
function updateServiceField(int $serviceId, string $fieldName, string $value): void
{
    try {
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
 * @param string $body      Request body
 * @param string $signature Signature from X-VirtueStack-Signature header
 *
 * @return bool True if valid
 */
function verifyWebhookSignature(string $body, string $signature): bool
{
    if (empty($signature)) {
        return false;
    }

    $webhookSecret = getWebhookSecret();
    if (empty($webhookSecret)) {
        return false;
    }

    $expectedSignature = hash_hmac('sha256', $body, $webhookSecret);
    return hash_equals($expectedSignature, $signature);
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
