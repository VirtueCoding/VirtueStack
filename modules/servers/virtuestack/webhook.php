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
require_once __DIR__ . '/lib/webhook_request.php';

use WHMCS\Database\Capsule;

// Constants
const WEBHOOK_SECRET_SETTING = 'VirtueStackWebhookSecret';
const PRIMARY_SIGNATURE_HEADER = 'HTTP_X_WEBHOOK_SIGNATURE';
const LEGACY_SIGNATURE_HEADER = 'HTTP_X_VIRTUESTACK_SIGNATURE';
const MAX_REQUEST_SIZE = 65536; // 64KB

// Allowed webhook event types — reject anything not in this list.
const ALLOWED_EVENTS = [
    'vm.created',
    'vm.deleted',
    'vm.started',
    'vm.stopped',
    'vm.reinstalled',
    'vm.migrated',
    'backup.completed',
    'backup.failed',
    'webhook.test',
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

    $requestBody = virtuestack_readWebhookBody(MAX_REQUEST_SIZE, virtuestack_getDeclaredContentLength());
    if ($requestBody['error'] !== null) {
        sendResponse($requestBody['status'], ['error' => $requestBody['error']]);
        return;
    }
    $body = $requestBody['body'];

    // Parse JSON
    $data = json_decode($body, true);
    if (json_last_error() !== JSON_ERROR_NONE) {
        sendResponse(400, ['error' => 'Invalid JSON: ' . json_last_error_msg()]);
        return;
    }

    // Verify signature
    $signature = getWebhookSignatureHeader();
    if (!verifyWebhookSignature($body, $signature)) {
        logWebhook('error', 'Invalid webhook signature');
        sendResponse(401, ['error' => 'Invalid signature']);
        return;
    }

    // Process the event with input validation.
    $context = virtuestack_extractWebhookContext($data);
    $validationError = virtuestack_validateWebhookContext($context, ALLOWED_EVENTS);
    if ($validationError !== null) {
        logWebhook('error', $validationError . ' in webhook payload');
        sendResponse(400, ['error' => $validationError]);
        return;
    }

    $eventType = $context['event'];
    $taskId = $context['task_id'];
    $vmId = $context['vm_id'];
    $externalServiceId = $context['external_service_id'];
    $eventData = $context['event_data'];
    $timestamp = $context['timestamp'] !== '' ? $context['timestamp'] : date('c');

    logWebhook('info', "Received webhook: {$eventType}", [
        'task_id' => $taskId,
        'vm_id' => $vmId,
        'service_id' => $externalServiceId,
        'idempotency_key' => $context['idempotency_key'],
    ]);

    try {
        switch ($eventType) {
            case 'vm.created':
                handleVMCreated($taskId, $vmId, $externalServiceId, $eventData);
                break;

            case 'vm.deleted':
                handleVMDeleted($vmId, $externalServiceId);
                break;

            case 'vm.started':
                if ($externalServiceId > 0) {
                    updateServiceField($externalServiceId, 'vm_status', 'running');
                } else if (!empty($vmId)) {
                    $serviceId = findServiceByVmId($vmId);
                    if ($serviceId > 0) {
                        updateServiceField($serviceId, 'vm_status', 'running');
                    }
                }
                break;

            case 'vm.stopped':
                if ($externalServiceId > 0) {
                    updateServiceField($externalServiceId, 'vm_status', 'stopped');
                } else if (!empty($vmId)) {
                    $serviceId = findServiceByVmId($vmId);
                    if ($serviceId > 0) {
                        updateServiceField($serviceId, 'vm_status', 'stopped');
                    }
                }
                break;

            case 'vm.reinstalled':
                if ($externalServiceId > 0) {
                    updateServiceField($externalServiceId, 'provisioning_status', 'active');
                    updateServiceField($externalServiceId, 'vm_status', 'running');
                    logActivity("VirtueStack: VM reinstalled for service {$externalServiceId}");
                }
                break;

            case 'vm.migrated':
                if ($externalServiceId > 0) {
                    $newNodeId = $eventData['node_id'] ?? '';
                    // SECURITY: Validate node_id is a valid UUID before storing
                    if (!empty($newNodeId) && !VirtueStackHelper::isValidUuid($newNodeId)) {
                        logWebhook('warning', "Invalid node_id received in vm.migrated webhook", [
                            'node_id' => $newNodeId,
                            'service_id' => $externalServiceId,
                        ]);
                        break;
                    }
                    updateServiceField($externalServiceId, 'node_id', $newNodeId);
                    logActivity("VirtueStack: VM migrated for service {$externalServiceId} to node {$newNodeId}");
                }
                break;

            case 'backup.completed':
                if ($externalServiceId > 0) {
                    logActivity("VirtueStack: Backup completed for service {$externalServiceId}");
                }
                break;

            case 'backup.failed':
                if ($externalServiceId > 0) {
                    $errorMsg = $eventData['message'] ?? $eventData['error'] ?? 'Unknown error';
                    logActivity("VirtueStack: Backup FAILED for service {$externalServiceId}: {$errorMsg}");
                    notifyAdmin(
                        'VirtueStack Backup Failure',
                        "Backup failed for service ID {$externalServiceId}\n\nError: {$errorMsg}"
                    );
                }
                break;

            case 'webhook.test':
                logWebhook('info', 'Received webhook test event');
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
 * Read the webhook signature header while supporting the controller's current
 * X-Webhook-Signature header and the legacy X-VirtueStack-Signature variant.
 *
 * @return string
 */
function getWebhookSignatureHeader(): string
{
    $signature = $_SERVER[PRIMARY_SIGNATURE_HEADER] ?? '';
    if ($signature !== '') {
        return $signature;
    }

    return $_SERVER[LEGACY_SIGNATURE_HEADER] ?? '';
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
        $primaryIp = '';
        if (isset($ipAddresses[0]) && is_array($ipAddresses[0])) {
            $primaryIp = isset($ipAddresses[0]['address']) && is_string($ipAddresses[0]['address'])
                ? $ipAddresses[0]['address']
                : '';
        } elseif (isset($ipAddresses[0]) && is_string($ipAddresses[0])) {
            $primaryIp = $ipAddresses[0];
        }
        if (!empty($primaryIp)) {
            updateServiceField($serviceId, 'vm_ip', $primaryIp);
            updateServiceDedicatedIp($serviceId, $primaryIp);
        }
    }

    // Store password if provided
    if (!empty($result['password'])) {
        try {
            $encryptedPassword = encryptPassword($result['password']);
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

    if (empty($encrypted)) {
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
            usort(
                $logFiles,
                static function (string $left, string $right): int {
                    return strcmp($left, $right);
                }
            );
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
