<?php

declare(strict_types=1);

/**
 * VirtueStack WHMCS Module Hooks.
 *
 * Provides hooks for async provisioning completion,
 * product page customization, and webhook callbacks.
 *
 * @package   VirtueStack\WHMCS
 * @author    VirtueStack Team
 * @copyright 2026 VirtueStack
 * @license   MIT
 */

// Load module dependencies
require_once __DIR__ . '/lib/ApiClient.php';
require_once __DIR__ . '/lib/VirtueStackHelper.php';

use VirtueStack\WHMCS\ApiClient;
use VirtueStack\WHMCS\VirtueStackHelper;

// ============================================================================
// PROVISIONING COMPLETION HOOK
// ============================================================================

/**
 * Hook: Daily Cron Job - Poll pending provisioning tasks.
 *
 * Checks for VMs that are still provisioning and updates their status
 * by polling the Controller API for task completion.
 *
 * This provides a fallback for webhook-based notifications.
 */
add_hook('DailyCronJob', 1, function () {
    // Find all services with provisioning_status = 'pending'
    $pendingServices = getPendingProvisioningServices();
    
    if (empty($pendingServices)) {
        return;
    }

    foreach ($pendingServices as $service) {
        try {
            pollProvisioningTask($service);
        } catch (\Exception $e) {
            logActivity("VirtueStack: Failed to poll task for service {$service['id']}: " . $e->getMessage());
        }
    }
});

/**
 * Hook: After Module Create - Start async polling.
 *
 * Initiates background polling for VM creation tasks.
 * This hook runs immediately after CreateAccount is called.
 */
add_hook('AfterModuleCreate', 1, function (array $params) {
    // Only process VirtueStack services
    if (!isset($params['module']) || $params['module'] !== 'virtuestack') {
        return;
    }

    $serviceId = (int) ($params['params']['serviceid'] ?? 0);
    if ($serviceId <= 0) {
        return;
    }

    // Get task ID from custom field
    $taskId = getServiceField($serviceId, 'task_id');
    if (empty($taskId)) {
        return;
    }

    // Store creation timestamp for timeout tracking
    updateServiceField($serviceId, 'provisioning_started_at', date('Y-m-d H:i:s'));

    logActivity("VirtueStack: VM creation initiated for service {$serviceId}, task {$taskId}");
});

// ============================================================================
// WEBHOOK HANDLER
// ============================================================================

/**
 * Register webhook endpoint for Controller callbacks.
 *
 * The Controller will POST to this endpoint when async tasks complete.
 * Endpoint: /modules/servers/virtuestack/webhook.php
 */
add_hook('ClientAreaPage', 1, function (array $vars) {
    // Check if this is a webhook request
    $webhookAction = $_GET['vs_webhook'] ?? '';
    
    if ($webhookAction === 'callback') {
        handleProvisioningWebhook();
        exit;
    }
});

// ============================================================================
// PRODUCT PAGE CUSTOMIZATION HOOKS
// ============================================================================

/**
 * Hook: Product Configuration Page - Add VirtueStack-specific fields.
 *
 * Adds custom configuration options to the product ordering page.
 */
add_hook('ProductConfigurationPage', 1, function (array $vars) {
    // Only for VirtueStack products
    if (!isset($vars['module']) || $vars['module'] !== 'virtuestack') {
        return $vars;
    }

    // Add template selection dropdown
    $vars['virtuestackTemplates'] = getVirtueStackTemplates();
    $vars['virtuestackLocations'] = getVirtueStackLocations();

    return $vars;
});

/**
 * Hook: ShoppingCartCheckoutCompletePage - Post-order processing.
 *
 * Performs any necessary post-order processing for VirtueStack services.
 */
add_hook('ShoppingCartCheckoutCompletePage', 1, function (array $vars) {
    // Add JavaScript for order confirmation page
    $output = <<<HTML
<script type="text/javascript">
// Track VirtueStack order completion
document.addEventListener('DOMContentLoaded', function() {
    // Check if any VirtueStack services were ordered
    var vsServices = document.querySelectorAll('[data-virtuestack-service]');
    if (vsServices.length > 0) {
        console.log('VirtueStack VPS provisioning initiated. Check your email for details.');
    }
});
</script>
HTML;

    return $output;
});

// ============================================================================
// ADMIN AREA HOOKS
// ============================================================================

/**
 * Hook: Admin Area Header Output - Add VirtueStack admin CSS/JS.
 */
add_hook('AdminAreaHeaderOutput', 1, function (array $vars) {
    return <<<HTML
<style type="text/css">
    .virtuestack-badge {
        display: inline-block;
        padding: 3px 7px;
        font-size: 11px;
        font-weight: bold;
        border-radius: 3px;
    }
    
    .virtuestack-badge-provisioning {
        background: #fcf8e3;
        color: #8a6d3b;
        border: 1px solid #faebcc;
    }
    
    .virtuestack-badge-active {
        background: #dff0d8;
        color: #3c763d;
        border: 1px solid #d6e9c6;
    }
    
    .virtuestack-badge-error {
        background: #f2dede;
        color: #a94442;
        border: 1px solid #ebccd1;
    }
</style>
HTML;
});

/**
 * Hook: Admin Services List Table - Add VirtueStack status column.
 */
add_hook('AdminServicesListTable', 1, function (array $vars) {
    // Only for VirtueStack services
    if (!isset($vars['module']) || $vars['module'] !== 'virtuestack') {
        return $vars;
    }

    // Add VM status column
    $vars['tablehead'][] = 'VPS Status';
    $vars['tablehead'][] = 'VM ID';

    return $vars;
});

/**
 * Hook: Admin Client Services Tab - Add VM management section.
 */
add_hook('AdminClientServicesTabFields', 1, function (array $vars) {
    // Only for VirtueStack services
    if (!isset($vars['module']) || $vars['module'] !== 'virtuestack') {
        return [];
    }

    $serviceId = (int) ($vars['id'] ?? 0);
    
    $vmId = getServiceField($serviceId, 'vm_id');
    $vmIp = getServiceField($serviceId, 'vm_ip');
    $provisioningStatus = getServiceField($serviceId, 'provisioning_status');

    return [
        'VirtueStack VM ID' => $vmId ?: 'Not provisioned',
        'VM IP Address' => $vmIp ?: 'N/A',
        'Provisioning Status' => formatProvisioningStatus($provisioningStatus),
    ];
});

// ============================================================================
// CLIENT AREA HOOKS
// ============================================================================

/**
 * Hook: Client Area Head Output - Add VirtueStack client CSS.
 */
add_hook('ClientAreaHeadOutput', 1, function (array $vars) {
    return <<<HTML
<style type="text/css">
    .virtuestack-panel {
        border: 1px solid #ddd;
        border-radius: 4px;
        margin-bottom: 20px;
    }
    
    .virtuestack-panel-header {
        background: #f5f5f5;
        padding: 15px;
        border-bottom: 1px solid #ddd;
    }
    
    .virtuestack-panel-body {
        padding: 15px;
    }
    
    .virtuestack-status-running {
        color: #5cb85c;
    }
    
    .virtuestack-status-stopped {
        color: #777;
    }
    
    .virtuestack-status-provisioning {
        color: #f0ad4e;
    }
    
    .virtuestack-status-error {
        color: #d9534f;
    }
</style>
HTML;
});

/**
 * Hook: Client Area Footer Output - Add VirtueStack client JS.
 */
add_hook('ClientAreaFooterOutput', 1, function (array $vars) {
    // Add auto-refresh for provisioning pages
    $output = <<<HTML
<script type="text/javascript">
(function() {
    // Check if we're on a VirtueStack service page with provisioning status
    var provisioningStatus = document.querySelector('[data-vs-status="provisioning"]');
    if (provisioningStatus) {
        // Refresh every 10 seconds
        setTimeout(function() {
            location.reload();
        }, 10000);
    }
    
    // Handle console links
    var consoleLinks = document.querySelectorAll('[data-vs-console]');
    consoleLinks.forEach(function(link) {
        link.addEventListener('click', function(e) {
            e.preventDefault();
            var type = this.getAttribute('data-vs-console');
            var vmId = this.getAttribute('data-vs-vm-id');
            openConsoleWindow(type, vmId);
        });
    });
    
    function openConsoleWindow(type, vmId) {
        var width = 1024;
        var height = 768;
        var left = (screen.width - width) / 2;
        var top = (screen.height - height) / 2;
        var url = 'clientarea.php?action=productdetails&id=' + vmId + '&modop=custom&a=console&type=' + type;
        
        window.open(
            url,
            'vs-console-' + vmId,
            'width=' + width + ',height=' + height + ',left=' + left + ',top=' + top + ',resizable=yes,scrollbars=no'
        );
    }
})();
</script>
HTML;

    return $output;
});

// ============================================================================
// SERVICE STATUS HOOKS
// ============================================================================

/**
 * Hook: Service Status Check - Verify VM status with Controller.
 *
 * Periodically syncs VM status from Controller to WHMCS.
 */
add_hook('IntelligentSearchUpdate', 1, function (array $vars) {
    // Run VM status sync (less frequently)
    static $lastRun = 0;
    $now = time();
    
    // Only run every 5 minutes
    if ($now - $lastRun < 300) {
        return;
    }
    $lastRun = $now;

    syncVMStatuses();
});

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Get all services with pending provisioning status.
 *
 * @return array List of service data
 */
function getPendingProvisioningServices(): array
{
    $services = [];
    
    try {
        $query = "
            SELECT 
                s.id AS service_id,
                s.userid AS client_id,
                cfv_task.value AS task_id,
                cfv_vm.value AS vm_id
            FROM tblhosting s
            INNER JOIN tblproducts p ON s.packageid = p.id
            INNER JOIN tblservers srv ON s.server = srv.id
            LEFT JOIN tblcustomfieldsvalues cfv_task ON cfv_task.relid = s.id
            LEFT JOIN tblcustomfields cf_task ON cf_task.id = cfv_task.fieldid AND cf_task.fieldname = 'task_id'
            LEFT JOIN tblcustomfieldsvalues cfv_vm ON cfv_vm.relid = s.id
            LEFT JOIN tblcustomfields cf_vm ON cf_vm.id = cfv_vm.fieldid AND cf_vm.fieldname = 'vm_id'
            LEFT JOIN tblcustomfieldsvalues cfv_status ON cfv_status.relid = s.id
            LEFT JOIN tblcustomfields cf_status ON cf_status.id = cfv_status.fieldid AND cf_status.fieldname = 'provisioning_status'
            WHERE p.servertype = 'virtuestack'
            AND srv.disabled = 0
            AND (cfv_status.value = 'pending' OR cfv_status.value IS NULL)
            AND s.domainstatus = 'Active'
        ";

        $result = full_query($query);
        
        while ($row = mysql_fetch_assoc($result)) {
            $services[] = [
                'service_id' => (int) $row['service_id'],
                'client_id' => (int) $row['client_id'],
                'task_id' => $row['task_id'],
                'vm_id' => $row['vm_id'],
            ];
        }
    } catch (\Exception $e) {
        logActivity('VirtueStack: Error fetching pending services: ' . $e->getMessage());
    }

    return $services;
}

/**
 * Poll provisioning task and update service status.
 *
 * @param array $service Service data
 */
function pollProvisioningTask(array $service): void
{
    $taskId = $service['task_id'] ?? '';
    $serviceId = $service['service_id'];
    
    if (empty($taskId)) {
        return;
    }

    // Get API client for the service's server
    $client = getApiClientForService($serviceId);
    if (!$client) {
        return;
    }

    // Get task status
    $task = $client->getTask($taskId);
    
    if ($task['status'] === 'completed') {
        handleProvisioningComplete($serviceId, $task, $client);
    } elseif ($task['status'] === 'failed') {
        handleProvisioningFailed($serviceId, $task);
    }
}

/**
 * Handle completed provisioning task.
 *
 * @param int        $serviceId WHMCS service ID
 * @param array      $task      Task result
 * @param ApiClient  $client    API client
 */
function handleProvisioningComplete(int $serviceId, array $task, ApiClient $client): void
{
    $result = $task['result'] ?? [];
    $vmId = $result['vm_id'] ?? getServiceField($serviceId, 'vm_id');
    
    // Update VM ID if provided
    if (!empty($vmId)) {
        updateServiceField($serviceId, 'vm_id', $vmId);
    }

    // Get VM details
    try {
        $vmInfo = $client->getVMInfo($vmId);
        
        // Update IP address
        if (!empty($vmInfo['ip_addresses'])) {
            $primaryIp = $vmInfo['ip_addresses'][0]['address'] ?? '';
            if (!empty($primaryIp)) {
                updateServiceField($serviceId, 'vm_ip', $primaryIp);
                // Update dedicated IP field
                updateServiceDedicatedIp($serviceId, $primaryIp);
            }
        }

        // Store root password if provided
        if (!empty($result['password'])) {
            $encryptedPassword = VirtueStackHelper::encrypt($result['password']);
            updateServiceField($serviceId, 'vm_password', $encryptedPassword);
        }

        // Update status
        updateServiceField($serviceId, 'provisioning_status', 'active');
        updateServiceField($serviceId, 'task_id', '');

        logActivity("VirtueStack: VM provisioning completed for service {$serviceId}, VM {$vmId}");
        
        // Send welcome email with VM details
        sendProvisioningEmail($serviceId, [
            'vm_id' => $vmId,
            'ip_address' => $primaryIp ?? '',
            'password' => $result['password'] ?? '',
        ]);
    } catch (\Exception $e) {
        logActivity("VirtueStack: Error fetching VM info for service {$serviceId}: " . $e->getMessage());
    }
}

/**
 * Handle failed provisioning task.
 *
 * @param int   $serviceId WHMCS service ID
 * @param array $task      Task result
 */
function handleProvisioningFailed(int $serviceId, array $task): void
{
    $errorMessage = $task['message'] ?? 'Unknown error';
    
    updateServiceField($serviceId, 'provisioning_status', 'error');
    updateServiceField($serviceId, 'provisioning_error', $errorMessage);

    logActivity("VirtueStack: VM provisioning FAILED for service {$serviceId}: {$errorMessage}");
    
    // Send alert to admin
    sendAdminNotification(
        'VirtueStack Provisioning Failure',
        "VM provisioning failed for service ID {$serviceId}.\n\nError: {$errorMessage}"
    );
}

/**
 * Handle incoming webhook from Controller.
 */
function handleProvisioningWebhook(): void
{
    // Verify request method
    if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
        http_response_code(405);
        exit('Method not allowed');
    }

    // Get request body
    $body = file_get_contents('php://input');
    $data = json_decode($body, true);

    if (json_last_error() !== JSON_ERROR_NONE) {
        http_response_code(400);
        exit('Invalid JSON');
    }

    // Verify webhook signature
    $signature = $_SERVER['HTTP_X_VIRTUESTACK_SIGNATURE'] ?? '';
    if (!verifyWebhookSignature($body, $signature)) {
        http_response_code(401);
        exit('Invalid signature');
    }

    // Process webhook event
    $eventType = $data['event'] ?? '';
    $taskId = $data['task_id'] ?? '';
    $whmcsServiceId = $data['whmcs_service_id'] ?? 0;

    logActivity("VirtueStack: Received webhook event '{$eventType}' for task {$taskId}");

    switch ($eventType) {
        case 'vm.created':
            // Find service by task ID
            $serviceId = findServiceByTaskId($taskId);
            if ($serviceId > 0) {
                $client = getApiClientForService($serviceId);
                if ($client) {
                    $task = $client->getTask($taskId);
                    handleProvisioningComplete($serviceId, $task, $client);
                }
            }
            break;

        case 'vm.creation_failed':
            $serviceId = findServiceByTaskId($taskId);
            if ($serviceId > 0) {
                handleProvisioningFailed($serviceId, $data);
            }
            break;

        case 'vm.deleted':
            if ($whmcsServiceId > 0) {
                updateServiceField($whmcsServiceId, 'provisioning_status', 'terminated');
                logActivity("VirtueStack: VM deleted for service {$whmcsServiceId}");
            }
            break;

        case 'vm.started':
            if ($whmcsServiceId > 0) {
                updateServiceField($whmcsServiceId, 'vm_status', 'running');
            }
            break;

        case 'vm.stopped':
            if ($whmcsServiceId > 0) {
                updateServiceField($whmcsServiceId, 'vm_status', 'stopped');
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
                $errorMsg = $data['message'] ?? 'Unknown error';
                logActivity("VirtueStack: Backup FAILED for service {$whmcsServiceId}: {$errorMsg}");
                sendAdminNotification(
                    'VirtueStack Backup Failure',
                    "Backup failed for service ID {$whmcsServiceId}.\n\nError: {$errorMsg}"
                );
            }
            break;

        default:
            logActivity("VirtueStack: Unhandled webhook event: {$eventType}");
    }

    http_response_code(200);
    exit('OK');
}

/**
 * Get API client for a specific service.
 *
 * @param int $serviceId WHMCS service ID
 *
 * @return ApiClient|null
 */
function getApiClientForService(int $serviceId): ?ApiClient
{
    try {
        // Get server details for the service
        $query = "
            SELECT 
                s.server,
                srv.hostname,
                srv.ipaddress,
                srv.username,
                srv.password,
                srv.secure,
                srv.port
            FROM tblhosting s
            INNER JOIN tblservers srv ON s.server = srv.id
            WHERE s.id = ?
        ";

        $result = full_query($query, [$serviceId]);
        $row = mysql_fetch_assoc($result);

        if (!$row) {
            return null;
        }

        $hostname = $row['hostname'] ?: $row['ipaddress'];
        $secure = $row['secure'] === 'on';
        $port = (int) ($row['port'] ?: 443);
        $apiKey = decrypt($row['password']);

        $protocol = $secure ? 'https' : 'http';
        $apiUrl = "{$protocol}://{$hostname}:{$port}/api/v1";

        return new ApiClient($apiUrl, $apiKey);
    } catch (\Exception $e) {
        logActivity("VirtueStack: Failed to get API client for service {$serviceId}: " . $e->getMessage());
        return null;
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
    $query = "
        SELECT cfv.relid
        FROM tblcustomfieldsvalues cfv
        INNER JOIN tblcustomfields cf ON cf.id = cfv.fieldid
        WHERE cf.fieldname = 'task_id'
        AND cfv.value = ?
    ";

    $result = full_query($query, [$taskId]);
    $row = mysql_fetch_assoc($result);

    return $row ? (int) $row['relid'] : 0;
}

/**
 * Get service custom field value.
 *
 * @param int    $serviceId Service ID
 * @param string $fieldName Field name
 *
 * @return string Field value
 */
function getServiceField(int $serviceId, string $fieldName): string
{
    $query = "
        SELECT cfv.value
        FROM tblcustomfieldsvalues cfv
        INNER JOIN tblcustomfields cf ON cf.id = cfv.fieldid
        WHERE cfv.relid = ?
        AND cf.fieldname = ?
    ";

    $result = full_query($query, [$serviceId, $fieldName]);
    $row = mysql_fetch_assoc($result);

    return $row ? (string) $row['value'] : '';
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
    // Get or create field ID
    $fieldId = getCustomFieldId($fieldName);
    
    if ($fieldId <= 0) {
        $fieldId = createCustomField($fieldName);
    }

    if ($fieldId <= 0) {
        return;
    }

    // Check if value exists
    $exists = getServiceField($serviceId, $fieldName) !== '';

    if ($exists) {
        $query = "
            UPDATE tblcustomfieldsvalues
            SET value = ?
            WHERE relid = ?
            AND fieldid = ?
        ";
        full_query($query, [$value, $serviceId, $fieldId]);
    } else {
        $query = "
            INSERT INTO tblcustomfieldsvalues (fieldid, relid, value)
            VALUES (?, ?, ?)
        ";
        full_query($query, [$fieldId, $serviceId, $value]);
    }
}

/**
 * Get custom field ID by name.
 *
 * @param string $fieldName Field name
 *
 * @return int Field ID
 */
function getCustomFieldId(string $fieldName): int
{
    $query = "
        SELECT id FROM tblcustomfields
        WHERE fieldname = ? AND type = 'product'
    ";

    $result = full_query($query, [$fieldName]);
    $row = mysql_fetch_assoc($result);

    return $row ? (int) $row['id'] : 0;
}

/**
 * Create custom field for VM data.
 *
 * @param string $fieldName Field name
 *
 * @return int Field ID
 */
function createCustomField(string $fieldName): int
{
    $query = "
        INSERT INTO tblcustomfields (type, relid, fieldname, fieldtype, adminonly)
        VALUES ('product', 0, ?, 'text', 'on')
    ";

    full_query($query, [$fieldName]);
    return (int) mysql_insert_id();
}

/**
 * Update service dedicated IP.
 *
 * @param int    $serviceId Service ID
 * @param string $ipAddress IP address
 */
function updateServiceDedicatedIp(int $serviceId, string $ipAddress): void
{
    $query = "UPDATE tblhosting SET dedicatedip = ? WHERE id = ?";
    full_query($query, [$ipAddress, $serviceId]);
}

/**
 * Verify webhook signature.
 *
 * @param string $body      Request body
 * @param string $signature Provided signature
 *
 * @return bool True if valid
 */
function verifyWebhookSignature(string $body, string $signature): bool
{
    if (empty($signature)) {
        return false;
    }

    // Get webhook secret from configuration
    $webhookSecret = getWebhookSecret();
    
    $expectedSignature = hash_hmac('sha256', $body, $webhookSecret);
    
    return hash_equals($expectedSignature, $signature);
}

/**
 * Get webhook secret from configuration.
 *
 * @return string
 */
function getWebhookSecret(): string
{
    // This should be stored in WHMCS configuration
    $query = "SELECT value FROM tblconfiguration WHERE setting = 'VirtueStackWebhookSecret'";
    $result = full_query($query);
    $row = mysql_fetch_assoc($result);
    
    return $row ? decrypt($row['value']) : '';
}

/**
 * Sync VM statuses from Controller.
 */
function syncVMStatuses(): void
{
    $services = getActiveVirtueStackServices();
    
    foreach ($services as $service) {
        try {
            $client = getApiClientForService($service['service_id']);
            if (!$client) {
                continue;
            }

            $vmId = getServiceField($service['service_id'], 'vm_id');
            if (empty($vmId)) {
                continue;
            }

            $status = $client->getVMStatus($vmId);
            $newStatus = $status['status'] ?? 'unknown';
            
            updateServiceField($service['service_id'], 'vm_status', $newStatus);
        } catch (\Exception $e) {
            // Log but continue
            logActivity("VirtueStack: Status sync error for service {$service['service_id']}: " . $e->getMessage());
        }
    }
}

/**
 * Get active VirtueStack services.
 *
 * @return array
 */
function getActiveVirtueStackServices(): array
{
    $services = [];
    
    $query = "
        SELECT s.id AS service_id
        FROM tblhosting s
        INNER JOIN tblproducts p ON s.packageid = p.id
        WHERE p.servertype = 'virtuestack'
        AND s.domainstatus = 'Active'
    ";

    $result = full_query($query);
    
    while ($row = mysql_fetch_assoc($result)) {
        $services[] = $row;
    }

    return $services;
}

/**
 * Format provisioning status for display.
 *
 * @param string $status Status value
 *
 * @return string HTML formatted status
 */
function formatProvisioningStatus(string $status): string
{
    $badges = [
        'pending' => '<span class="virtuestack-badge virtuestack-badge-provisioning">Provisioning</span>',
        'active' => '<span class="virtuestack-badge virtuestack-badge-active">Active</span>',
        'suspended' => '<span class="virtuestack-badge virtuestack-badge-provisioning">Suspended</span>',
        'error' => '<span class="virtuestack-badge virtuestack-badge-error">Error</span>',
        'terminated' => '<span class="virtuestack-badge">Terminated</span>',
    ];

    return $badges[$status] ?? '<span class="virtuestack-badge">' . ucfirst($status) . '</span>';
}

/**
 * Get available VirtueStack templates.
 *
 * @return array
 */
function getVirtueStackTemplates(): array
{
    $templates = [];
    
    $services = getActiveVirtueStackServices();
    if (empty($services)) {
        return $templates;
    }

    foreach ($services as $service) {
        $serviceId = (int) ($service['service_id'] ?? 0);
        if ($serviceId <= 0) {
            continue;
        }

        $client = getApiClientForService($serviceId);
        if (!$client) {
            continue;
        }

        try {
            $items = $client->listTemplates();

            if (is_array($items)) {
                foreach ($items as $item) {
                    if (!is_array($item)) {
                        continue;
                    }
                    $id = (string) ($item['id'] ?? '');
                    $name = (string) ($item['name'] ?? '');
                    if ($id !== '' && $name !== '') {
                        $templates[$id] = $name;
                    }
                }
            }

            if (!empty($templates)) {
                break;
            }
        } catch (\Exception $e) {
            logActivity('VirtueStack: Failed to fetch templates: ' . $e->getMessage());
        }
    }

    return $templates;
}

/**
 * Get available VirtueStack locations.
 *
 * @return array
 */
function getVirtueStackLocations(): array
{
    $locations = [];

    $services = getActiveVirtueStackServices();
    if (empty($services)) {
        return $locations;
    }

    foreach ($services as $service) {
        $serviceId = (int) ($service['service_id'] ?? 0);
        if ($serviceId <= 0) {
            continue;
        }

        $client = getApiClientForService($serviceId);
        if (!$client) {
            continue;
        }

        try {
            $items = $client->listLocations();

            if (is_array($items)) {
                foreach ($items as $item) {
                    if (!is_array($item)) {
                        continue;
                    }
                    $id = (string) ($item['id'] ?? '');
                    if ($id === '') {
                        continue;
                    }

                    $name = (string) ($item['name'] ?? $id);
                    $region = (string) ($item['region'] ?? '');
                    if ($region !== '' && stripos($name, $region) === false) {
                        $name = $name . ' (' . $region . ')';
                    }

                    $locations[$id] = $name;
                }
            }

            if (!empty($locations)) {
                break;
            }
        } catch (\Exception $e) {
            logActivity('VirtueStack: Failed to fetch locations: ' . $e->getMessage());
        }
    }

    return $locations;
}

/**
 * Send provisioning complete email to client.
 *
 * @param int   $serviceId Service ID
 * @param array $vmData    VM data
 */
function sendProvisioningEmail(int $serviceId, array $vmData): void
{
    // Get service and client details
    $query = "
        SELECT 
            s.userid,
            s.domain,
            p.name AS product_name
        FROM tblhosting s
        INNER JOIN tblproducts p ON s.packageid = p.id
        WHERE s.id = ?
    ";

    $result = full_query($query, [$serviceId]);
    $service = mysql_fetch_assoc($result);

    if (!$service) {
        return;
    }

    // Send email via WHMCS
    $emailParams = [
        'service_id' => $serviceId,
        'product_name' => $service['product_name'],
        'vm_id' => $vmData['vm_id'],
        'ip_address' => $vmData['ip_address'],
        'password' => $vmData['password'],
        'hostname' => $service['domain'],
    ];

    sendMessage('VirtueStack VPS Welcome', $service['userid'], $emailParams);
}

/**
 * Send admin notification.
 *
 * @param string $subject Subject
 * @param string $message Message
 */
function sendAdminNotification(string $subject, string $message): void
{
    if (function_exists('logAdminNotification')) {
        logAdminNotification($subject, $message);
    }
}
