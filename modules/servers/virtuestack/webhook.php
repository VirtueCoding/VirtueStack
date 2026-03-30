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
require_once __DIR__ . '/lib/shared_functions.php';

use WHMCS\Database\Capsule;

// Constants
const WEBHOOK_SECRET_SETTING = 'VirtueStackWebhookSecret';
const SIGNATURE_HEADER = 'HTTP_X_VIRTUESTACK_SIGNATURE';
const MAX_REQUEST_SIZE = 65536; // 64KB

// Allowed webhook event types — reject anything not in this list.
const ALLOWED_EVENTS = [
    'vm.created',
    'vm.creation_failed',
    'vm.deleted',
    'vm.suspended',
    'vm.unsuspended',
    'vm.resized',
    'vm.started',
    'vm.stopped',
    'vm.reinstalled',
    'vm.migrated',
    'backup.completed',
    'backup.failed',
    'task.completed',
    'task.failed',
];

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
    if (!verifyWebhookSignature($body, $signature)) {
        logWebhook('error', 'Invalid webhook signature');
        sendResponse(401, ['error' => 'Invalid signature']);
        return;
    }

    // Process the event with input validation
    $eventType = isset($data['event']) && is_string($data['event']) ? trim($data['event']) : '';
    $taskId = isset($data['task_id']) && is_string($data['task_id']) ? trim($data['task_id']) : '';
    $vmId = isset($data['vm_id']) && is_string($data['vm_id']) ? trim($data['vm_id']) : '';
    $whmcsServiceId = isset($data['whmcs_service_id']) && is_int($data['whmcs_service_id']) ? $data['whmcs_service_id'] : 0;
    $result = isset($data['result']) && is_array($data['result']) ? $data['result'] : [];
    $timestamp = isset($data['timestamp']) && is_string($data['timestamp']) ? trim($data['timestamp']) : date('c');

    // Validate required fields
    if (empty($eventType) || empty($taskId)) {
        logWebhook('error', 'Missing required fields in webhook payload');
        sendResponse(400, ['error' => 'Missing required fields: event, task_id']);
        return;
    }

    // Whitelist event types to reject unknown/injected values
    if (!in_array($eventType, ALLOWED_EVENTS, true)) {
        logWebhook('warning', 'Rejected unknown webhook event type: ' . substr($eventType, 0, 100));
        sendResponse(400, ['error' => 'Unknown event type']);
        return;
    }

    // Validate UUID format for task_id and vm_id when present
    $uuidPattern = '/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i';
    if (!preg_match($uuidPattern, $taskId)) {
        logWebhook('error', 'Invalid task_id format in webhook payload');
        sendResponse(400, ['error' => 'Invalid task_id format']);
        return;
    }
    if (!empty($vmId) && !preg_match($uuidPattern, $vmId)) {
        logWebhook('error', 'Invalid vm_id format in webhook payload');
        sendResponse(400, ['error' => 'Invalid vm_id format']);
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

            case 'vm.started':
                if ($whmcsServiceId > 0) {
                    updateServiceField($whmcsServiceId, 'vm_status', 'running');
                } else if (!empty($vmId)) {
                    $serviceId = findServiceByVmId($vmId);
                    if ($serviceId > 0) {
                        updateServiceField($serviceId, 'vm_status', 'running');
                    }
                }
                break;

            case 'vm.stopped':
                if ($whmcsServiceId > 0) {
                    updateServiceField($whmcsServiceId, 'vm_status', 'stopped');
                } else if (!empty($vmId)) {
                    $serviceId = findServiceByVmId($vmId);
                    if ($serviceId > 0) {
                        updateServiceField($serviceId, 'vm_status', 'stopped');
                    }
                }
                break;

            case 'vm.reinstalled':
                if ($whmcsServiceId > 0) {
                    updateServiceField($whmcsServiceId, 'provisioning_status', 'active');
                    updateServiceField($whmcsServiceId, 'vm_status', 'running');
                    logActivity("VirtueStack: VM reinstalled for service {$whmcsServiceId}");
                }
                break;

            case 'vm.migrated':
                if ($whmcsServiceId > 0) {
                    $newNodeId = $data['node_id'] ?? '';
                    // SECURITY: Validate node_id is a valid UUID before storing
                    if (!empty($newNodeId) && !VirtueStackHelper::isValidUuid($newNodeId)) {
                        logWebhook('warning', "Invalid node_id received in vm.migrated webhook", [
                            'node_id' => $newNodeId,
                            'service_id' => $whmcsServiceId,
                        ]);
                        break;
                    }
                    updateServiceField($whmcsServiceId, 'node_id', $newNodeId);
                    logActivity("VirtueStack: VM migrated for service {$whmcsServiceId} to node {$newNodeId}");
                }
                break;

            case 'backup.completed':
                if ($whmcsServiceId > 0) {
                    logActivity("VirtueStack: Backup completed for service {$whmcsServiceId}");
                }
                break;

            case 'backup.failed':
                if ($whmcsServiceId > 0) {
                    $errorMsg = $data['message'] ?? $data['error'] ?? 'Unknown error';
                    logActivity("VirtueStack: Backup FAILED for service {$whmcsServiceId}: {$errorMsg}");
                    notifyAdmin(
                        'VirtueStack Backup Failure',
                        "Backup failed for service ID {$whmcsServiceId}\n\nError: {$errorMsg}"
                    );
                }
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
        try {
            $encryptedPassword = encryptPassword($password);
            updateServiceField($serviceId, 'vm_password', $encryptedPassword);
        } catch (\RuntimeException $e) {
            logWebhook('error', "Failed to encrypt password for service {$serviceId}: " . $e->getMessage());
        }
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

    logActivity("VirtueStack: VM resized via webhook - Service {$serviceId}: " . substr(json_encode($result, JSON_UNESCAPED_SLASHES), 0, 500));
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
// WEBHOOK-SPECIFIC HELPER FUNCTIONS
// ============================================================================

/**
 * Encrypt password using WHMCS encryption.
 *
 * @param string $password Plain text password
 *
 * @return string Encrypted password
 *
 * @throws \RuntimeException If encryption is not available
 */
function encryptPassword(string $password): string
{
    if (empty($password)) {
        return '';
    }

    if (!function_exists('encrypt')) {
        throw new \RuntimeException('WHMCS encrypt() function not available — cannot store password securely');
    }

    $encrypted = encrypt($password);

    if ($encrypted === false || $encrypted === '') {
        throw new \RuntimeException('WHMCS encrypt() returned an empty result');
    }

    return $encrypted;
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
    \logActivity($message);
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

    $logFile = dirname(__FILE__) . '/logs/webhook.log';
    $logDir = dirname($logFile);
    if (!is_dir($logDir)) {
        mkdir($logDir, 0755, true);
    }

    $maxLogSize = 10 * 1024 * 1024;
    if (file_exists($logFile) && filesize($logFile) > $maxLogSize) {
        $backupFile = $logFile . '.' . date('Y-m-d-His');
        rename($logFile, $backupFile);
        $logFiles = glob($logDir . '/webhook.log.*');
        if (count($logFiles) > 5) {
            usort($logFiles);
            array_map('unlink', array_slice($logFiles, 0, count($logFiles) - 5));
        }
    }

    $logEntry = sprintf(
        "[%s] [%s] %s %s\n",
        date('Y-m-d H:i:s'),
        strtoupper($level),
        $message,
        !empty($context) ? json_encode($context) : ''
    );

    $result = file_put_contents($logFile, $logEntry, FILE_APPEND | LOCK_EX);
    if ($result === false) {
        error_log("VirtueStack webhook: Failed to write to log file: {$logFile}");
    }
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
