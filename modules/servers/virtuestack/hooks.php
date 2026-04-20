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
require_once __DIR__ . '/lib/shared_functions.php';

use VirtueStack\WHMCS\ApiClient;
use VirtueStack\WHMCS\VirtueStackHelper;

use WHMCS\Database\Capsule;

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
add_hook('Cron', 1, function () {
    $now = time();
    static $lastPollRun = 0;
    if ($now - $lastPollRun < 300) {
        return;
    }
    $lastPollRun = $now;

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

    $startedAt = getServiceField($serviceId, 'provisioning_started_at');
    if (!empty($startedAt)) {
        $startTime = strtotime($startedAt);
        if ($startTime !== false && (time() - $startTime) > 1800) {
            updateServiceField($serviceId, 'provisioning_status', 'error');
            updateServiceField($serviceId, 'provisioning_error', 'Provisioning timed out after 30 minutes');
            logActivity("VirtueStack: VM provisioning timed out for service {$serviceId}");
            sendAdminNotification(
                'VirtueStack Provisioning Timeout',
                "VM provisioning timed out (30 min) for service ID {$serviceId}."
            );
            return;
        }
    }

    updateServiceField($serviceId, 'provisioning_started_at', date('Y-m-d H:i:s'));

    logActivity("VirtueStack: VM creation initiated for service {$serviceId}, task {$taskId}");
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
 * Hook: Product Edit - Validate VirtueStack configuration.
 *
 * Validates Plan ID and Template ID when an admin saves product configuration.
 * This catches configuration errors early before a customer tries to order.
 */
add_hook('ProductEdit', 1, function (array $vars) {
    // Only for VirtueStack products
    if (!isset($vars['servertype']) || $vars['servertype'] !== 'virtuestack') {
        return;
    }

    $productId = (int) ($vars['id'] ?? 0);
    if ($productId <= 0) {
        return;
    }

    // Get the configuration options from the form
    $configOption1 = trim((string) ($vars['configoption1'] ?? '')); // Plan ID
    $configOption2 = trim((string) ($vars['configoption2'] ?? '')); // Template ID

    // Validate Plan ID format
    if (!empty($configOption1)) {
        if (!preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $configOption1)) {
            logActivity("VirtueStack: Product {$productId} has invalid Plan ID format: {$configOption1}");
            // Note: WHMCS doesn't provide a way to abort the save from a hook,
            // but we log the warning for admin visibility
        } else {
            // Try to validate against API if server is configured
            try {
                $server = getProductServer($productId);
                if ($server) {
                    $apiUrl = buildApiUrlFromServer($server);
                    $apiKey = decrypt($server->password ?? '');

                    if (!empty($apiKey)) {
                        $client = new ApiClient($apiUrl, $apiKey, 10, true);
                        if (!$client->planExists($configOption1)) {
                            logActivity("VirtueStack WARNING: Product {$productId} has non-existent Plan ID: {$configOption1}");
                        } else {
                            $plan = $client->getPlan($configOption1);
                            logActivity("VirtueStack: Product {$productId} Plan validated: " . ($plan['name'] ?? $configOption1));
                        }
                    }
                }
            } catch (\Exception $e) {
                logActivity("VirtueStack: Could not validate Plan ID for product {$productId}: " . $e->getMessage());
            }
        }
    }

    // Validate Template ID format
    if (!empty($configOption2)) {
        if (!preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $configOption2)) {
            logActivity("VirtueStack: Product {$productId} has invalid Template ID format: {$configOption2}");
        }
    }
});

/**
 * Hook: Product Add - Validate VirtueStack configuration on creation.
 *
 * Validates Plan ID and Template ID when an admin creates a new product.
 */
add_hook('ProductAdd', 1, function (array $vars) {
    // Only for VirtueStack products
    if (!isset($vars['servertype']) || $vars['servertype'] !== 'virtuestack') {
        return;
    }

    // Get the configuration options from the form
    $configOption1 = trim((string) ($vars['configoption1'] ?? '')); // Plan ID
    $configOption2 = trim((string) ($vars['configoption2'] ?? '')); // Template ID

    // Log validation warnings (WHMCS doesn't allow blocking from hooks)
    if (!empty($configOption1) && !preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $configOption1)) {
        logActivity("VirtueStack WARNING: New product has invalid Plan ID format: {$configOption1}");
    }

    if (!empty($configOption2) && !preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $configOption2)) {
        logActivity("VirtueStack WARNING: New product has invalid Template ID format: {$configOption2}");
    }
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

add_hook('AdminServicesListTableRow', 1, function (array $vars) {
    if (!isset($vars['module']) || $vars['module'] !== 'virtuestack') {
        return $vars;
    }

    $serviceId = (int) ($vars['id'] ?? 0);
    $vmId = getServiceField($serviceId, 'vm_id');
    $vmStatus = getServiceField($serviceId, 'vm_status') ?: getServiceField($serviceId, 'provisioning_status');

    $vars['tablevalues'][] = formatProvisioningStatus($vmStatus ?: 'unknown');
    $vars['tablevalues'][] = $vmId ?: 'N/A';

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
    try {
        $lastSync = Capsule::table('tblconfiguration')
            ->where('setting', 'virtuestack_last_status_sync')
            ->value('value');
        $lastSyncTime = is_numeric($lastSync) ? (int) $lastSync : 0;
        $now = time();

        if ($now - $lastSyncTime < 300) {
            return;
        }

        Capsule::table('tblconfiguration')->updateOrInsert(
            ['setting' => 'virtuestack_last_status_sync'],
            ['value' => (string) $now, 'created_at' => date('Y-m-d H:i:s'), 'updated_at' => date('Y-m-d H:i:s')]
        );
    } catch (\Exception $e) {
        return;
    }

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
        $rows = Capsule::table('tblhosting')
            ->join('tblproducts', 'tblhosting.packageid', '=', 'tblproducts.id')
            ->join('tblservers', 'tblhosting.server', '=', 'tblservers.id')
            ->leftJoin('tblcustomfields', 'tblcustomfields.fieldname', '=', Capsule::raw("'task_id'"))
            ->leftJoin('tblcustomfieldsvalues AS cfv_task', function ($join) {
                $join->on('cfv_task.relid', '=', 'tblhosting.id')
                     ->on('cfv_task.fieldid', '=', 'tblcustomfields.id');
            })
            ->leftJoin('tblcustomfields AS cf2', 'cf2.fieldname', '=', Capsule::raw("'vm_id'"))
            ->leftJoin('tblcustomfieldsvalues AS cfv_vm', function ($join) {
                $join->on('cfv_vm.relid', '=', 'tblhosting.id')
                     ->on('cfv_vm.fieldid', '=', 'cf2.id');
            })
            ->leftJoin('tblcustomfields AS cf3', 'cf3.fieldname', '=', Capsule::raw("'provisioning_status'"))
            ->leftJoin('tblcustomfieldsvalues AS cfv_status', function ($join) {
                $join->on('cfv_status.relid', '=', 'tblhosting.id')
                     ->on('cfv_status.fieldid', '=', 'cf3.id');
            })
            ->where('tblproducts.servertype', 'virtuestack')
            ->where('tblservers.disabled', 0)
            ->where('tblhosting.domainstatus', 'Active')
            ->where(function ($query) {
                $query->where('cfv_status.value', 'pending')
                      ->orWhereNull('cfv_status.value');
            })
            ->select(
                'tblhosting.id AS service_id',
                'tblhosting.userid AS client_id',
                'cfv_task.value AS task_id',
                'cfv_vm.value AS vm_id'
            )
            ->get();

        foreach ($rows as $row) {
            $services[] = [
                'service_id' => (int) $row->service_id,
                'client_id' => (int) $row->client_id,
                'task_id' => $row->task_id,
                'vm_id' => $row->vm_id,
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
 * Get API client for a specific service.
 *
 * @param int $serviceId WHMCS service ID
 *
 * @return ApiClient|null
 */
function getApiClientForService(int $serviceId): ?ApiClient
{
    try {
        $row = Capsule::table('tblhosting')
            ->join('tblservers', 'tblhosting.server', '=', 'tblservers.id')
            ->where('tblhosting.id', $serviceId)
            ->first([
                'tblhosting.server',
                'tblservers.hostname',
                'tblservers.ipaddress',
                'tblservers.username',
                'tblservers.password',
                'tblservers.secure',
                'tblservers.port',
            ]);

        if (!$row) {
            return null;
        }

        $hostname = $row->hostname ?: $row->ipaddress;
        $secure = $row->secure === 'on';
        $port = (int) ($row->port ?: 443);
        $apiKey = decrypt($row->password);

        $protocol = $secure ? 'https' : 'http';
        $apiUrl = "{$protocol}://{$hostname}:{$port}/api/v1";

        return new ApiClient($apiUrl, $apiKey);
    } catch (\Exception $e) {
        logActivity("VirtueStack: Failed to get API client for service {$serviceId}: " . $e->getMessage());
        return null;
    }
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
    try {
        $fieldId = getCustomFieldId($fieldName);
        if ($fieldId <= 0) {
            return '';
        }

        $value = Capsule::table('tblcustomfieldsvalues')
            ->where('relid', $serviceId)
            ->where('fieldid', $fieldId)
            ->value('value');

        return $value !== null ? (string) $value : '';
    } catch (\Exception $e) {
        return '';
    }
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
    try {
        return Capsule::table('tblhosting')
            ->join('tblproducts', 'tblhosting.packageid', '=', 'tblproducts.id')
            ->where('tblproducts.servertype', 'virtuestack')
            ->where('tblhosting.domainstatus', 'Active')
            ->select('tblhosting.id as service_id')
            ->get()
            ->map(function ($row) {
                return (array) $row;
            })
            ->toArray();
    } catch (\Exception $e) {
        logActivity('VirtueStack: Error fetching active services: ' . $e->getMessage());
        return [];
    }
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

    return $badges[$status] ?? '<span class="virtuestack-badge">' . htmlspecialchars(ucfirst($status), ENT_QUOTES, 'UTF-8') . '</span>';
}

/**
 * Get available VirtueStack templates.
 *
 * @return array
 */
function getVirtueStackTemplates(): array
{
    static $cache = ['data' => null, 'expires' => 0];

    if ($cache['data'] !== null && time() < $cache['expires']) {
        return $cache['data'];
    }

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

    $cache['data'] = $templates;
    $cache['expires'] = time() + 300;

    return $templates;
}

/**
 * Get available VirtueStack locations.
 *
 * @return array
 */
function getVirtueStackLocations(): array
{
    static $cache = ['data' => null, 'expires' => 0];

    if ($cache['data'] !== null && time() < $cache['expires']) {
        return $cache['data'];
    }

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

    $cache['data'] = $locations;
    $cache['expires'] = time() + 300;

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
            'vm_id' => $vmData['vm_id'],
            'ip_address' => $vmData['ip_address'],
            'password' => $vmData['password'],
            'hostname' => $service->domain,
        ];

        sendMessage('VirtueStack VPS Welcome', $service->userid, $emailParams);
    } catch (\Exception $e) {
        logActivity("VirtueStack: Failed to send provisioning email for service {$serviceId}: " . $e->getMessage());
    }
}

/**
 * Send admin notification.
 *
 * @param string $subject Subject
 * @param string $message Message
 */
function sendAdminNotification(string $subject, string $message): void
{
    if (function_exists('sendMessage')) {
        $adminUserId = 1;
        sendMessage($subject, $adminUserId, ['message' => $message]);
    }

    logActivity("VirtueStack Admin Notification: {$subject}");
}

/**
 * Get server associated with a product.
 *
 * @param int $productId Product ID
 *
 * @return object|null Server data or null
 */
function getProductServer(int $productId): ?object
{
    try {
        $product = Capsule::table('tblproducts')
            ->where('id', $productId)
            ->first(['servergroup']);

        if (!$product || empty($product->servergroup)) {
            return null;
        }

        // Get first server from the server group
        $serverGroup = Capsule::table('tblservergroups')
            ->where('id', $product->servergroup)
            ->first(['filltype', 'id']);

        if (!$serverGroup) {
            return null;
        }

        $serverRel = Capsule::table('tblservergroupsrel')
            ->where('groupid', $serverGroup->id)
            ->orderBy('id')
            ->first(['serverid']);

        if (!$serverRel) {
            return null;
        }

        return Capsule::table('tblservers')
            ->where('id', $serverRel->serverid)
            ->where('disabled', 0)
            ->first(['id', 'hostname', 'ipaddress', 'password', 'secure', 'port']);
    } catch (\Exception $e) {
        logActivity("VirtueStack: Error getting server for product {$productId}: " . $e->getMessage());
        return null;
    }
}

/**
 * Build API URL from server object.
 *
 * @param object $server Server data
 *
 * @return string API URL
 */
function buildApiUrlFromServer(object $server): string
{
    $hostname = $server->hostname ?? $server->ipaddress ?? '';
    $secure = ($server->secure ?? 'on') === 'on';
    $port = (int) ($server->port ?: 443);

    $protocol = $secure ? 'https' : 'http';

    return "{$protocol}://{$hostname}:{$port}/api/v1";
}

/**
 * Get available VirtueStack plans.
 *
 * Used for product configuration dropdown population.
 *
 * @return array List of plans with id => name
 */
function getVirtueStackPlans(): array
{
    static $cache = ['data' => null, 'expires' => 0];

    if ($cache['data'] !== null && time() < $cache['expires']) {
        return $cache['data'];
    }

    $plans = [];

    $services = getActiveVirtueStackServices();
    if (empty($services)) {
        return $plans;
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
            $items = $client->listPlans();

            if (is_array($items)) {
                foreach ($items as $item) {
                    if (!is_array($item)) {
                        continue;
                    }
                    $id = (string) ($item['id'] ?? '');
                    $name = (string) ($item['name'] ?? '');
                    if ($id !== '' && $name !== '') {
                        // Include specs in the name for easier identification
                        $specs = [];
                        if (isset($item['vcpu'])) {
                            $specs[] = $item['vcpu'] . ' vCPU';
                        }
                        if (isset($item['memory_mb'])) {
                            $specs[] = round($item['memory_mb'] / 1024, 1) . 'GB RAM';
                        }
                        if (isset($item['disk_gb'])) {
                            $specs[] = $item['disk_gb'] . 'GB Disk';
                        }
                        $displayName = $name;
                        if (!empty($specs)) {
                            $displayName .= ' (' . implode(', ', $specs) . ')';
                        }
                        $plans[$id] = $displayName;
                    }
                }
            }

            if (!empty($plans)) {
                break;
            }
        } catch (\Exception $e) {
            logActivity('VirtueStack: Failed to fetch plans: ' . $e->getMessage());
        }
    }

    $cache['data'] = $plans;
    $cache['expires'] = time() + 300;

    return $plans;
}
