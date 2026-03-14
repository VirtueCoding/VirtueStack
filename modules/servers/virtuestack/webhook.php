<?php

declare(strict_types=1);

/**
 * VirtueStack WHMCS Module - Webhook Endpoint.
 *
 * Receives webhook callbacks from the VirtueStack Controller
 * for async provisioning completion notifications.
 *
 * Endpoint: /modules/servers/virtuestack/webhook.php
 *
 * @package   VirtueStack\WHMCS
 * @author    VirtueStack Team
 * @copyright 2026 VirtueStack
 * @license   MIT
 */

// Load WHMCS environment
$whmcsPath = dirname(__FILE__, 4);
require_once $whmcsPath . '/init.php';

use WHMCS\Database\Capsule;

// Constants
const WEBHOOK_SECRET_SETTING = 'VirtueStackWebhookSecret';
const SIGNATURE_HEADER = 'HTTP_X_VIRTUESTACK_SIGNATURE';
const MAX_REQUEST_SIZE = 65536; // 64KB

/**
 * Main webhook handler.
 */
function handleWebhook(): void
{
    // Only accept POST requests
    if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
        sendResponse(405, ['error' => 'Method not allowed']);
        return;
    }

    // Verify content type
    $contentType = $_SERVER['CONTENT_TYPE'] ?? '';
    if (stripos($contentType, 'application/json') === false) {
        sendResponse(415, ['error' => 'Content-Type must be application/json']);
        return;
    }

    // Read request body
    $body = file_get_contents('php://input');
    
    if (empty($body)) {
        sendResponse(400, ['error' => 'Empty request body']);
        return;
    }

    if (strlen($body) > MAX_REQUEST_SIZE) {
        sendResponse(413, ['error' => 'Request body too large']);
        return;
    }

    // Parse JSON
    $data = json_decode($body, true);
    if (json_last_error() !== JSON_ERROR_NONE) {
        sendResponse(400, ['error' => 'Invalid JSON: ' . json_last_error_msg()]);
        return;
    }

    // Verify signature
    $signature = $_SERVER[SIGNATURE_HEADER] ?? '';
    if (!verifySignature($body, $signature)) {
        logWebhook('error', 'Invalid webhook signature');
        sendResponse(401, ['error' => 'Invalid signature']);
        return;
    }

    // Process the event with input validation
    $eventType = isset($data['event']) && is_string($data['event']) ? htmlspecialchars(trim($data['event']), ENT_QUOTES, 'UTF-8') : '';
    $taskId = isset($data['task_id']) && is_string($data['task_id']) ? htmlspecialchars(trim($data['task_id']), ENT_QUOTES, 'UTF-8') : '';
    $vmId = isset($data['vm_id']) && is_string($data['vm_id']) ? htmlspecialchars(trim($data['vm_id']), ENT_QUOTES, 'UTF-8') : '';
    $whmcsServiceId = isset($data['whmcs_service_id']) && is_int($data['whmcs_service_id']) ? $data['whmcs_service_id'] : 0;
    $result = isset($data['result']) && is_array($data['result']) ? $data['result'] : [];
    $timestamp = isset($data['timestamp']) && is_string($data['timestamp']) ? htmlspecialchars(trim($data['timestamp']), ENT_QUOTES, 'UTF-8') : date('c');

    // Validate required fields
    if (empty($eventType) || empty($taskId)) {
        logWebhook('error', 'Missing required fields in webhook payload');
        sendResponse(400, ['error' => 'Missing required fields: event, task_id']);
        return;
    }

    logWebhook('info', "Received webhook: {$eventType}", [
        'task_id' => $taskId,
        'vm_id' => $vmId,
        'service_id' => $whmcsServiceId,
    ]);

    try {
        switch ($eventType) {
            case 'vm.created':
                handleVMCreated($taskId, $vmId, $whmcsServiceId, $result);
                break;

            case 'vm.creation_failed':
                handleVMCreationFailed($taskId, $whmcsServiceId, $data);
                break;

            case 'vm.deleted':
                handleVMDeleted($vmId, $whmcsServiceId);
                break;

            case 'vm.suspended':
                handleVMSuspended($vmId, $whmcsServiceId);
                break;

            case 'vm.unsuspended':
                handleVMUnsuspended($vmId, $whmcsServiceId);
                break;

            case 'vm.resized':
                handleVMResized($vmId, $whmcsServiceId, $result);
                break;

            case 'task.completed':
                handleTaskCompleted($taskId, $result);
                break;

            case 'task.failed':
                handleTaskFailed($taskId, $data);
                break;

            default:
                logWebhook('warning', "Unhandled webhook event type: {$eventType}");
        }

        sendResponse(200, ['status' => 'processed', 'event' => $eventType]);
    } catch (\Exception $e) {
        logWebhook('error', "Webhook processing error: " . $e->getMessage(), [
            'event' => $eventType,
            'task_id' => $taskId,
        ]);
        sendResponse(500, ['error' => 'Processing error']);
    }
}

/**
 * Handle VM creation completed event.
 *
 * @param string $taskId    Task UUID
 * @param string $vmId      VM UUID
 * @param int    $serviceId WHMCS service ID
 * @param array  $result    Task result data
 */
function handleVMCreated(string $taskId, string $vmId, int $serviceId, array $result): void
{
    // If service ID not provided, find it by task ID
    if ($serviceId <= 0) {
        $serviceId = findServiceByTaskId($taskId);
    }

    if ($serviceId <= 0) {
        logWebhook('error', "Cannot find service for task {$taskId}");
        return;
    }

    // Update VM ID
    if (!empty($vmId)) {
        updateServiceField($serviceId, 'vm_id', $vmId);
    }

    // Update IP addresses
    $ipAddresses = $result['ip_addresses'] ?? [];
    if (!empty($ipAddresses)) {
        $primaryIp = $ipAddresses[0]['address'] ?? '';
        if (!empty($primaryIp)) {
            updateServiceField($serviceId, 'vm_ip', $primaryIp);
            updateServiceDedicatedIp($serviceId, $primaryIp);
        }
    }

    // Store password if provided
    $password = $result['password'] ?? '';
    if (!empty($password)) {
        $encryptedPassword = encryptPassword($password);
        updateServiceField($serviceId, 'vm_password', $encryptedPassword);
    }

    // Store customer ID
    $customerId = $result['customer_id'] ?? '';
    if (!empty($customerId)) {
        updateServiceField($serviceId, 'virtuestack_customer_id', $customerId);
    }

    // Update status
    updateServiceField($serviceId, 'provisioning_status', 'active');
    updateServiceField($serviceId, 'task_id', '');

    // Log success
    logActivity("VirtueStack: VM provisioning completed via webhook - Service {$serviceId}, VM {$vmId}");

    // Send welcome email
    sendWelcomeEmail($serviceId, [
        'vm_id' => $vmId,
        'ip_address' => $primaryIp ?? '',
        'password' => $password,
    ]);
}

/**
 * Handle VM creation failed event.
 *
 * @param string $taskId    Task UUID
 * @param int    $serviceId WHMCS service ID
 * @param array  $data      Webhook data
 */
function handleVMCreationFailed(string $taskId, int $serviceId, array $data): void
{
    if ($serviceId <= 0) {
        $serviceId = findServiceByTaskId($taskId);
    }

    if ($serviceId <= 0) {
        logWebhook('error', "Cannot find service for failed task {$taskId}");
        return;
    }

    $errorMessage = $data['error'] ?? $data['message'] ?? 'Unknown error';

    updateServiceField($serviceId, 'provisioning_status', 'error');
    updateServiceField($serviceId, 'provisioning_error', $errorMessage);

    logActivity("VirtueStack: VM provisioning FAILED via webhook - Service {$serviceId}: {$errorMessage}");

    // Notify admin
    notifyAdmin(
        'VirtueStack Provisioning Failure',
        "VM provisioning failed for service ID {$serviceId}\n\nTask ID: {$taskId}\nError: {$errorMessage}"
    );
}

/**
 * Handle VM deleted event.
 *
 * @param string $vmId      VM UUID
 * @param int    $serviceId WHMCS service ID
 */
function handleVMDeleted(string $vmId, int $serviceId): void
{
    if ($serviceId <= 0) {
        $serviceId = findServiceByVmId($vmId);
    }

    if ($serviceId <= 0) {
        return;
    }

    updateServiceField($serviceId, 'provisioning_status', 'terminated');
    updateServiceField($serviceId, 'vm_id', '');
    updateServiceField($serviceId, 'vm_ip', '');
    updateServiceField($serviceId, 'vm_password', '');

    logActivity("VirtueStack: VM deleted via webhook - Service {$serviceId}, VM {$vmId}");
}

/**
 * Handle VM suspended event.
 *
 * @param string $vmId      VM UUID
 * @param int    $serviceId WHMCS service ID
 */
function handleVMSuspended(string $vmId, int $serviceId): void
{
    if ($serviceId <= 0) {
        $serviceId = findServiceByVmId($vmId);
    }

    if ($serviceId <= 0) {
        return;
    }

    updateServiceField($serviceId, 'provisioning_status', 'suspended');
    logActivity("VirtueStack: VM suspended via webhook - Service {$serviceId}");
}

/**
 * Handle VM unsuspended event.
 *
 * @param string $vmId      VM UUID
 * @param int    $serviceId WHMCS service ID
 */
function handleVMUnsuspended(string $vmId, int $serviceId): void
{
    if ($serviceId <= 0) {
        $serviceId = findServiceByVmId($vmId);
    }

    if ($serviceId <= 0) {
        return;
    }

    updateServiceField($serviceId, 'provisioning_status', 'active');
    logActivity("VirtueStack: VM unsuspended via webhook - Service {$serviceId}");
}

/**
 * Handle VM resized event.
 *
 * @param string $vmId      VM UUID
 * @param int    $serviceId WHMCS service ID
 * @param array  $result    Task result
 */
function handleVMResized(string $vmId, int $serviceId, array $result): void
{
    if ($serviceId <= 0) {
        $serviceId = findServiceByVmId($vmId);
    }

    if ($serviceId <= 0) {
        return;
    }

    logActivity("VirtueStack: VM resized via webhook - Service {$serviceId}: " . json_encode($result));
}

/**
 * Handle generic task completed event.
 *
 * @param string $taskId Task UUID
 * @param array  $result Task result
 */
function handleTaskCompleted(string $taskId, array $result): void
{
    $serviceId = findServiceByTaskId($taskId);

    if ($serviceId > 0) {
        updateServiceField($serviceId, 'task_id', '');
    }

    logWebhook('info', "Task completed: {$taskId}");
}

/**
 * Handle generic task failed event.
 *
 * @param string $taskId Task UUID
 * @param array  $data   Webhook data
 */
function handleTaskFailed(string $taskId, array $data): void
{
    $serviceId = findServiceByTaskId($taskId);

    if ($serviceId > 0) {
        $errorMessage = $data['error'] ?? $data['message'] ?? 'Unknown error';
        updateServiceField($serviceId, 'provisioning_error', $errorMessage);
    }

    logWebhook('error', "Task failed: {$taskId} - " . ($data['message'] ?? 'Unknown error'));
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Verify webhook signature.
 *
 * @param string $body      Request body
 * @param string $signature Signature from header
 *
 * @return bool
 */
function verifySignature(string $body, string $signature): bool
{
    if (empty($signature)) {
        return false;
    }

    $secret = getWebhookSecret();
    if (empty($secret)) {
        logWebhook('warning', 'Webhook secret not configured, rejecting request');
        return false;
    }

    $expectedSignature = hash_hmac('sha256', $body, $secret);
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
        $result = Capsule::table('tblconfiguration')
            ->where('setting', WEBHOOK_SECRET_SETTING)
            ->value('value');

        return $result ? decrypt($result) : '';
    } catch (\Exception $e) {
        return '';
    }
}

/**
 * Find service ID by task ID.
 *
 * @param string $taskId Task UUID
 *
 * @return int
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
 * @return int
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
 * Get custom field ID by name.
 *
 * @param string $fieldName Field name
 *
 * @return int
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
            // Create the field
            $fieldId = (int) Capsule::table('tblcustomfields')->insertGetId([
                'type' => 'product',
                'relid' => 0,
                'fieldname' => $fieldName,
                'fieldtype' => 'text',
                'adminonly' => 'on',
                'created_at' => date('Y-m-d H:i:s'),
                'updated_at' => date('Y-m-d H:i:s'),
            ]);
        }

        // Check if value exists
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
        logWebhook('error', "Failed to update service field {$fieldName}: " . $e->getMessage());
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
        logWebhook('error', "Failed to update dedicated IP: " . $e->getMessage());
    }
}

/**
 * Encrypt password using WHMCS encryption.
 *
 * @param string $password Plain text password
 *
 * @return string Encrypted password
 */
function encryptPassword(string $password): string
{
    if (function_exists('encrypt')) {
        return encrypt($password);
    }
    return $password;
}

/**
 * Send welcome email to customer.
 *
 * @param int   $serviceId Service ID
 * @param array $vmData    VM data
 */
function sendWelcomeEmail(int $serviceId, array $vmData): void
{
    try {
        $service = Capsule::table('tblhosting')
            ->join('tblproducts', 'tblhosting.packageid', '=', 'tblproducts.id')
            ->where('tblhosting.id', $serviceId)
            ->first(['tblhosting.userid', 'tblhosting.domain', 'tblproducts.name as product_name']);

        if (!$service) {
            return;
        }

        $emailParams = [
            'service_id' => $serviceId,
            'product_name' => $service->product_name,
            'vm_id' => $vmData['vm_id'] ?? '',
            'ip_address' => $vmData['ip_address'] ?? '',
            'password' => $vmData['password'] ?? '',
            'hostname' => $service->domain ?? '',
        ];

        if (function_exists('sendMessage')) {
            sendMessage('VirtueStack VPS Welcome', $service->userid, $emailParams);
        }
    } catch (\Exception $e) {
        logWebhook('error', "Failed to send welcome email: " . $e->getMessage());
    }
}

/**
 * Notify administrators.
 *
 * @param string $subject Subject
 * @param string $message Message
 */
function notifyAdmin(string $subject, string $message): void
{
    if (function_exists('logAdminNotification')) {
        logAdminNotification($subject, $message);
    }
}

/**
 * Log activity to WHMCS.
 *
 * @param string $message Log message
 */
function logActivity(string $message): void
{
    if (function_exists('logActivity')) {
        logActivity($message);
    }
}

/**
 * Log webhook event.
 *
 * @param string $level   Log level
 * @param string $message Log message
 * @param array  $context Additional context
 */
function logWebhook(string $level, string $message, array $context = []): void
{
    $logMessage = "VirtueStack Webhook: {$message}";
    if (!empty($context)) {
        $logMessage .= " | " . json_encode($context);
    }

    if (function_exists('logModuleCall')) {
        logModuleCall('virtuestack', 'webhook', $logMessage, '', '', '');
    }

    // Also write to module log
    $logFile = dirname(__FILE__) . '/logs/webhook.log';
    $logDir = dirname($logFile);
    if (!is_dir($logDir)) {
        mkdir($logDir, 0755, true);
    }

    $logEntry = sprintf(
        "[%s] [%s] %s %s\n",
        date('Y-m-d H:i:s'),
        strtoupper($level),
        $message,
        !empty($context) ? json_encode($context) : ''
    );

    file_put_contents($logFile, $logEntry, FILE_APPEND | LOCK_EX);
}

/**
 * Send JSON response.
 *
 * @param int   $status HTTP status code
 * @param array $data   Response data
 */
function sendResponse(int $status, array $data): void
{
    http_response_code($status);
    header('Content-Type: application/json');
    echo json_encode($data, JSON_PRETTY_PRINT);
}

// Run the webhook handler
handleWebhook();
