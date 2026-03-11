<?php

declare(strict_types=1);

/**
 * VirtueStack WHMCS Provisioning Module.
 *
 * Main module file providing VM lifecycle management for WHMCS.
 * Integrates with VirtueStack Controller API for all operations.
 *
 * @package   VirtueStack\WHMCS
 * @author    VirtueStack Team
 * @copyright 2026 VirtueStack
 * @license   MIT
 *
 * WHMCS Module Functions:
 * - virtuestack_MetaData() - Module metadata
 * - virtuestack_ConfigOptions() - Product configuration options
 * - virtuestack_CreateAccount() - Provision new VM
 * - virtuestack_SuspendAccount() - Suspend VM
 * - virtuestack_UnsuspendAccount() - Unsuspend VM
 * - virtuestack_TerminateAccount() - Delete VM
 * - virtuestack_ChangePackage() - Resize VM
 * - virtuestack_ChangePassword() - Reset root password
 * - virtuestack_ClientArea() - Client area output
 * - virtuestack_AdminServicesTabFields() - Admin panel fields
 */

use VirtueStack\WHMCS\ApiClient;
use VirtueStack\WHMCS\VirtueStackHelper;

// Load module dependencies
require_once __DIR__ . '/lib/ApiClient.php';
require_once __DIR__ . '/lib/VirtueStackHelper.php';

/**
 * Module metadata.
 *
 * @return array Module information
 */
function virtuestack_MetaData(): array
{
    return [
        'DisplayName'                => 'VirtueStack VPS',
        'APIVersion'                 => '1.2',
        'RequiresServer'             => true,
        'DefaultNonSSLPort'          => 80,
        'DefaultSSLPort'             => 443,
        'ServiceSingleSignOnLabel'   => 'Login to VirtueStack Panel',
        'AdminSingleSignOnLabel'     => 'Manage in VirtueStack',
        'ApplicationLinkDescription' => 'VirtueStack VPS Management Platform',
        'Version'                    => '1.0.0',
        'Author'                     => 'VirtueStack Team',
        'AuthorUri'                  => 'https://virtuestack.io',
        'MinPhpVersion'              => '8.3',
        'MinWHMCSVersion'            => '8.0.0',
    ];
}

/**
 * Product configuration options.
 *
 * Defines configurable options available when creating/editing products.
 *
 * @return array Configuration options
 */
function virtuestack_ConfigOptions(): array
{
    return [
        'Plan ID' => [
            'Type'        => 'text',
            'Size'        => 36,
            'Description' => 'VirtueStack Plan UUID (e.g., 550e8400-e29b-41d4-a716-446655440000)',
        ],
        'Template ID' => [
            'Type'        => 'text',
            'Size'        => 36,
            'Description' => 'Default OS Template UUID (customer can change on reinstall)',
        ],
        'Location ID' => [
            'Type'        => 'text',
            'Size'        => 36,
            'Description' => 'Default Location UUID (optional, leave empty for auto-placement)',
        ],
        'VM Hostname Prefix' => [
            'Type'        => 'text',
            'Size'        => 20,
            'Description' => 'Prefix for auto-generated hostnames (default: vps)',
            'Default'     => 'vps',
        ],
    ];
}

/**
 * Provision a new VM.
 *
 * Called when a new service is activated. Creates the VM via Controller API.
 * Uses async provisioning - returns success immediately, VM is created in background.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_CreateAccount(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);
    $clientId = (int) ($params['userid'] ?? 0);

    VirtueStackHelper::log('CreateAccount', json_encode($params), 'Starting provisioning');

    try {
        // Get API client
        $client = virtuestack_getApiClient($params);

        // Get or create customer in VirtueStack
        $customerData = virtuestack_ensureCustomer($params, $client, $clientId);

        // Get configuration
        $planId = VirtueStackHelper::getConfigValue($params, 'Plan ID', '');
        $templateId = VirtueStackHelper::getConfigValue($params, 'Template ID', '');
        $locationId = VirtueStackHelper::getConfigValue($params, 'Location ID', '');
        $hostnamePrefix = VirtueStackHelper::getConfigValue($params, 'VM Hostname Prefix', 'vps');

        // Validate required configuration
        if (empty($planId)) {
            return 'Plan ID is not configured for this product';
        }
        if (empty($templateId)) {
            return 'Template ID is not configured for this product';
        }

        // Build hostname
        $hostname = $params['domain'] ?? '';
        if (empty($hostname) || !VirtueStackHelper::isValidHostname($hostname)) {
            $hostname = strtolower($hostnamePrefix . '-' . $serviceId);
        }

        // Create VM request
        $createParams = [
            'customer_id'      => $customerData['customer_id'],
            'plan_id'          => $planId,
            'template_id'      => $templateId,
            'hostname'         => $hostname,
            'whmcs_service_id' => $serviceId,
        ];

        if (!empty($locationId)) {
            $createParams['location_id'] = $locationId;
        }

        // Call API
        $result = $client->createVM($createParams);

        // Store task ID and VM ID in custom fields
        virtuestack_updateServiceField($serviceId, 'vm_id', $result['vm_id']);
        virtuestack_updateServiceField($serviceId, 'task_id', $result['task_id']);
        virtuestack_updateServiceField($serviceId, 'provisioning_status', 'pending');

        VirtueStackHelper::log(
            'CreateAccount',
            'VM creation initiated',
            json_encode($result)
        );

        // Return success - actual VM creation happens async
        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log('CreateAccount', 'Failed', $e->getMessage());
        return 'Failed to create VM: ' . $e->getMessage();
    }
}

/**
 * Suspend a VM.
 *
 * Called when a service is suspended (e.g., non-payment).
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_SuspendAccount(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    VirtueStackHelper::log('SuspendAccount', "Service ID: {$serviceId}", 'Starting suspension');

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            return 'VM ID not found for this service';
        }

        $result = $client->suspendVM($vmId);

        virtuestack_updateServiceField($serviceId, 'provisioning_status', 'suspended');

        VirtueStackHelper::log('SuspendAccount', 'VM suspended', json_encode($result));

        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log('SuspendAccount', 'Failed', $e->getMessage());
        return 'Failed to suspend VM: ' . $e->getMessage();
    }
}

/**
 * Unsuspend a VM.
 *
 * Called when a suspended service is reactivated.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_UnsuspendAccount(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    VirtueStackHelper::log('UnsuspendAccount', "Service ID: {$serviceId}", 'Starting unsuspension');

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            return 'VM ID not found for this service';
        }

        $result = $client->unsuspendVM($vmId);

        virtuestack_updateServiceField($serviceId, 'provisioning_status', 'active');

        VirtueStackHelper::log('UnsuspendAccount', 'VM unsuspended', json_encode($result));

        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log('UnsuspendAccount', 'Failed', $e->getMessage());
        return 'Failed to unsuspend VM: ' . $e->getMessage();
    }
}

/**
 * Terminate (delete) a VM.
 *
 * Called when a service is cancelled/terminated.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_TerminateAccount(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    VirtueStackHelper::log('TerminateAccount', "Service ID: {$serviceId}", 'Starting termination');

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            // VM doesn't exist, consider it terminated
            VirtueStackHelper::log('TerminateAccount', 'VM ID not found', 'Already terminated or never created');
            return 'success';
        }

        $result = $client->deleteVM($vmId);

        // Clear stored data
        virtuestack_updateServiceField($serviceId, 'vm_id', '');
        virtuestack_updateServiceField($serviceId, 'vm_ip', '');
        virtuestack_updateServiceField($serviceId, 'vm_password', '');
        virtuestack_updateServiceField($serviceId, 'provisioning_status', 'terminated');

        VirtueStackHelper::log('TerminateAccount', 'VM terminated', json_encode($result));

        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log('TerminateAccount', 'Failed', $e->getMessage());
        return 'Failed to terminate VM: ' . $e->getMessage();
    }
}

/**
 * Change VM package (upgrade/downgrade).
 *
 * Called when a product upgrade/downgrade is processed.
 * Currently only supports upgrades (disk shrinking not supported).
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_ChangePackage(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    VirtueStackHelper::log('ChangePackage', "Service ID: {$serviceId}", 'Starting package change');

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            return 'VM ID not found for this service';
        }

        // Get new plan ID from config options
        $newPlanId = VirtueStackHelper::getConfigValue($params, 'Plan ID', '');
        
        if (empty($newPlanId)) {
            return 'No target plan specified';
        }

        // Fetch plan details from Controller API
        $planDetails = $client->getPlan($newPlanId);
        
        if (empty($planDetails)) {
            return 'Failed to fetch plan details for plan ID: ' . $newPlanId;
        }

        // Extract plan resources
        $vcpus = $planDetails['vcpu'] ?? 0;
        $memoryMb = $planDetails['memory_mb'] ?? 0;
        $diskGb = $planDetails['disk_gb'] ?? 0;

        if ($vcpus <= 0 || $memoryMb <= 0 || $diskGb <= 0) {
            return 'Invalid plan configuration - missing resource values';
        }

        // Call resize API
        $result = $client->resizeVM($vmId, $vcpus, $memoryMb, $diskGb);

        // Check if resize was successful
        if (isset($result['error'])) {
            throw new \Exception($result['error']);
        }

        // If async, store task_id for polling
        if (isset($result['task_id'])) {
            virtuestack_updateServiceField($serviceId, 'task_id', $result['task_id']);
            virtuestack_updateServiceField($serviceId, 'provisioning_status', 'resizing');
        }

        VirtueStackHelper::log('ChangePackage', 'Package change initiated', 'Plan ID: ' . $newPlanId);

        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log('ChangePackage', 'Failed', $e->getMessage());
        return 'Failed to change package: ' . $e->getMessage();
    }
}

/**
 * Change root password.
 *
 * Resets the root password for the VM.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_ChangePassword(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    VirtueStackHelper::log('ChangePassword', "Service ID: {$serviceId}", 'Starting password change');

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            return 'VM ID not found for this service';
        }

        $result = $client->resetPassword($vmId);

        // Store new password (encrypted)
        if (!empty($result['password'])) {
            $encryptedPassword = VirtueStackHelper::encrypt($result['password']);
            virtuestack_updateServiceField($serviceId, 'vm_password', $encryptedPassword);
        }

        VirtueStackHelper::log('ChangePassword', 'Password changed', 'Success');

        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log('ChangePassword', 'Failed', $e->getMessage());
        return 'Failed to change password: ' . $e->getMessage();
    }
}

/**
 * Client area output.
 *
 * Renders the client area interface for the VM service.
 * Displays VM details and embedded Customer WebUI.
 *
 * @param array $params WHMCS module parameters
 *
 * @return array Client area template variables
 */
function virtuestack_ClientArea(array $params): array
{
    $serviceId = (int) ($params['serviceid'] ?? 0);
    $clientId = (int) ($params['userid'] ?? 0);

    try {
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');
        $vmIp = virtuestack_getServiceField($serviceId, 'vm_ip');
        $provisioningStatus = virtuestack_getServiceField($serviceId, 'provisioning_status');

        // Check if VM is still being provisioned
        if ($provisioningStatus === 'pending') {
            $taskId = virtuestack_getServiceField($serviceId, 'task_id');
            return [
                'templatefile' => 'templates/overview.tpl',
                'vars' => [
                    'status'          => 'provisioning',
                    'task_id'         => $taskId,
                    'service_id'      => $serviceId,
                    'provisioningUrl' => virtuestack_getProvisioningStatusUrl($taskId),
                ],
            ];
        }

        // Get customer credentials for SSO
        $credentials = VirtueStackHelper::getCustomerCredentials($clientId);
        $webuiUrl = virtuestack_getWebuiUrl($params);

        // Generate SSO token
        $ssoToken = '';
        if (!empty($credentials) && !empty($vmId)) {
            try {
                $jwtSecret = virtuestack_getJwtSecret($params);
                $issuer = virtuestack_getApiUrl($params);
                $customerId = virtuestack_getServiceField($serviceId, 'virtuestack_customer_id');

                $ssoToken = VirtueStackHelper::generateSSOToken(
                    $customerId,
                    $credentials['api_id'],
                    $credentials['api_secret'],
                    $jwtSecret,
                    $issuer
                );
            } catch (\Exception $e) {
                VirtueStackHelper::log('ClientArea', 'SSO token generation failed', $e->getMessage());
            }
        }

        // Build iframe URL
        $iframeUrl = '';
        if (!empty($vmId) && !empty($ssoToken) && !empty($webuiUrl)) {
            $iframeUrl = VirtueStackHelper::buildWebuiUrl($webuiUrl, $vmId, $ssoToken);
        }

        return [
            'templatefile' => 'templates/overview.tpl',
            'vars' => [
                'status'      => $provisioningStatus ?? 'active',
                'vm_id'       => $vmId,
                'vm_ip'       => $vmIp,
                'iframe_url'  => $iframeUrl,
                'service_id'  => $serviceId,
                'webui_url'   => $webuiUrl,
                'sso_token'   => $ssoToken,
                'console_url' => $vmId && $ssoToken && $webuiUrl
                    ? VirtueStackHelper::buildConsoleUrl($webuiUrl, $vmId, $ssoToken, 'vnc')
                    : '',
            ],
        ];
    } catch (\Exception $e) {
        VirtueStackHelper::log('ClientArea', 'Error', $e->getMessage());
        return [
            'templatefile' => 'templates/overview.tpl',
            'vars' => [
                'status' => 'error',
                'error'  => $e->getMessage(),
            ],
        ];
    }
}

/**
 * Admin services tab fields.
 *
 * Adds custom fields to the admin services page for this service.
 *
 * @param array $params WHMCS module parameters
 *
 * @return array Admin tab fields
 */
function virtuestack_AdminServicesTabFields(array $params): array
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    $vmId = virtuestack_getServiceField($serviceId, 'vm_id');
    $vmIp = virtuestack_getServiceField($serviceId, 'vm_ip');
    $provisioningStatus = virtuestack_getServiceField($serviceId, 'provisioning_status');
    $taskId = virtuestack_getServiceField($serviceId, 'task_id');

    return [
        'VM ID'               => $vmId ?: 'Not provisioned',
        'VM IP Address'       => $vmIp ?: 'N/A',
        'Provisioning Status' => ucfirst($provisioningStatus ?: 'unknown'),
        'Task ID'             => $taskId ?: 'N/A',
    ];
}

/**
 * Test connection to Controller API.
 *
 * Used by WHMCS to verify server configuration.
 *
 * @param array $params WHMCS module parameters
 *
 * @return array Connection test result
 */
function virtuestack_TestConnection(array $params): array
{
    try {
        $client = virtuestack_getApiClient($params);
        
        // Try to get a simple status endpoint
        // Note: You may need to add a health check endpoint to your API
        // For now, we'll just try to instantiate the client
        $success = true;
        $errorMsg = '';
    } catch (\Exception $e) {
        $success = false;
        $errorMsg = $e->getMessage();
    }

    return [
        'success' => $success,
        'error'   => $errorMsg,
    ];
}

/**
 * Admin custom button array.
 *
 * Defines additional buttons shown in admin area for this service.
 *
 * @return array Button definitions
 */
function virtuestack_AdminCustomButtonArray(): array
{
    return [
        'Start VM'   => 'startVM',
        'Stop VM'    => 'stopVM',
        'Restart VM' => 'restartVM',
        'Sync Status' => 'syncStatus',
    ];
}

/**
 * Start VM button action.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_startVM(array $params): string
{
    return virtuestack_powerOperation($params, 'start');
}

/**
 * Stop VM button action.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_stopVM(array $params): string
{
    return virtuestack_powerOperation($params, 'stop');
}

/**
 * Restart VM button action.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_restartVM(array $params): string
{
    return virtuestack_powerOperation($params, 'restart');
}

/**
 * Sync VM status button action.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string 'success' or error message
 */
function virtuestack_syncStatus(array $params): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            return 'VM ID not found for this service';
        }

        $status = $client->getVMStatus($vmId);
        
        virtuestack_updateServiceField($serviceId, 'provisioning_status', $status['status'] ?? 'unknown');

        return 'success';
    } catch (\Exception $e) {
        return 'Failed to sync status: ' . $e->getMessage();
    }
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Get API client instance.
 *
 * @param array $params WHMCS module parameters
 *
 * @return ApiClient
 *
 * @throws \RuntimeException If configuration is invalid
 */
function virtuestack_getApiClient(array $params): ApiClient
{
    $apiUrl = virtuestack_getApiUrl($params);
    $apiKey = virtuestack_getApiKey($params);
    $timeout = (int) VirtueStackHelper::getConfigValue($params, 'api_timeout', 30);
    $verifySsl = (bool) VirtueStackHelper::getConfigValue($params, 'verify_ssl', true);

    return new ApiClient($apiUrl, $apiKey, $timeout, $verifySsl);
}

/**
 * Get Controller API URL from params.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string
 */
function virtuestack_getApiUrl(array $params): string
{
    $hostname = $params['serverhostname'] ?? $params['serverip'] ?? '';
    $secure = ($params['serversecure'] ?? 'on') === 'on';
    $port = (int) ($params['serverport'] ?: 443);

    $protocol = $secure ? 'https' : 'http';
    
    return "{$protocol}://{$hostname}:{$port}/api/v1";
}

/**
 * Get API key from server configuration.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string
 *
 * @throws \RuntimeException If API key not configured
 */
function virtuestack_getApiKey(array $params): string
{
    $apiKey = $params['serverpassword'] ?? '';
    
    if (empty($apiKey)) {
        throw new \RuntimeException('API Key not configured in server settings');
    }

    return $apiKey;
}

/**
 * Get JWT secret from configuration.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string
 *
 * @throws \RuntimeException If JWT secret not configured
 */
function virtuestack_getJwtSecret(array $params): string
{
    $secret = VirtueStackHelper::getConfigValue($params, 'jwt_secret', '');
    
    if (empty($secret)) {
        // Fall back to API key as secret (not recommended for production)
        $secret = virtuestack_getApiKey($params);
    }

    return $secret;
}

/**
 * Get Customer WebUI URL from configuration.
 *
 * @param array $params WHMCS module parameters
 *
 * @return string
 */
function virtuestack_getWebuiUrl(array $params): string
{
    $webuiUrl = VirtueStackHelper::getConfigValue($params, 'webui_url', '');
    
    if (empty($webuiUrl)) {
        // Construct from server hostname
        $hostname = $params['serverhostname'] ?? '';
        $webuiUrl = "https://panel.{$hostname}";
    }

    return $webuiUrl;
}

/**
 * Get provisioning status URL for async task polling.
 *
 * @param string $taskId Task ID
 *
 * @return string
 */
function virtuestack_getProvisioningStatusUrl(string $taskId): string
{
    return "modules/servers/virtuestack/task_status.php?task_id={$taskId}";
}

/**
 * Get service custom field value.
 *
 * @param int    $serviceId WHMCS service ID
 * @param string $fieldName Field name
 *
 * @return string Field value or empty string
 */
function virtuestack_getServiceField(int $serviceId, string $fieldName): string
{
    if (!function_exists('get_query_val')) {
        return '';
    }

    $result = get_query_val(
        'tblcustomfieldsvalues',
        'value',
        [
            'relid' => $serviceId,
            'fieldid' => virtuestack_getCustomFieldId($fieldName),
        ]
    );

    return is_string($result) ? $result : '';
}

/**
 * Update service custom field value.
 *
 * @param int    $serviceId WHMCS service ID
 * @param string $fieldName Field name
 * @param string $value     New value
 */
function virtuestack_updateServiceField(int $serviceId, string $fieldName, string $value): void
{
    if (!function_exists('update_query')) {
        return;
    }

    $fieldId = virtuestack_getCustomFieldId($fieldName);
    
    if (empty($fieldId)) {
        // Try to create the field
        $fieldId = virtuestack_createCustomField($fieldName);
    }

    if (!empty($fieldId)) {
        update_query(
            'tblcustomfieldsvalues',
            ['value' => $value],
            ['relid' => $serviceId, 'fieldid' => $fieldId]
        );
    }
}

/**
 * Get custom field ID by name.
 *
 * @param string $fieldName Field name
 *
 * @return int Field ID or 0 if not found
 */
function virtuestack_getCustomFieldId(string $fieldName): int
{
    if (!function_exists('get_query_val')) {
        return 0;
    }

    $result = get_query_val(
        'tblcustomfields',
        'id',
        [
            'fieldname' => $fieldName,
            'type' => 'product',
        ]
    );

    return is_numeric($result) ? (int) $result : 0;
}

/**
 * Create a custom field for VM data.
 *
 * @param string $fieldName Field name
 *
 * @return int Field ID or 0 on failure
 */
function virtuestack_createCustomField(string $fieldName): int
{
    if (!function_exists('insert_query')) {
        return 0;
    }

    // Check if field already exists
    $existingId = virtuestack_getCustomFieldId($fieldName);
    if ($existingId > 0) {
        return $existingId;
    }

    return (int) insert_query('tblcustomfields', [
        'type'          => 'product',
        'relid'         => 0, // Will be associated with products
        'fieldname'     => $fieldName,
        'fieldtype'     => 'text',
        'description'   => "VirtueStack {$fieldName}",
        'fieldoptions'  => '',
        'regexpr'       => '',
        'adminonly'     => 'on',
        'required'      => '',
        'showorder'     => '',
        'showinvoice'   => '',
        'sortorder'     => 0,
        'created_at'    => 'NOW()',
        'updated_at'    => 'NOW()',
    ]);
}

/**
 * Ensure customer exists in VirtueStack and return customer data.
 *
 * @param array     $params   WHMCS module parameters
 * @param ApiClient $client   API client
 * @param int       $clientId WHMCS client ID
 *
 * @return array Customer data
 */
function virtuestack_ensureCustomer(array $params, ApiClient $client, int $clientId): array
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    // Check if customer already has VirtueStack credentials
    $customerId = virtuestack_getServiceField($serviceId, 'virtuestack_customer_id');

    if (!empty($customerId)) {
        return ['customer_id' => $customerId];
    }

    // Check for existing credentials at client level
    $credentials = VirtueStackHelper::getCustomerCredentials($clientId);
    
    if ($credentials) {
        return ['customer_id' => $credentials['api_id']];
    }

    // In production, you would call the Controller API to create a customer
    // and store the credentials. For now, return a placeholder.
    // This should be implemented based on your Controller's customer creation API.
    
    return ['customer_id' => '']; // Will be filled by Controller during VM creation
}

/**
 * Perform a power operation on a VM.
 *
 * @param array  $params    WHMCS module parameters
 * @param string $operation Power operation (start, stop, restart)
 *
 * @return string 'success' or error message
 */
function virtuestack_powerOperation(array $params, string $operation): string
{
    $serviceId = (int) ($params['serviceid'] ?? 0);

    try {
        $client = virtuestack_getApiClient($params);
        $vmId = virtuestack_getServiceField($serviceId, 'vm_id');

        if (empty($vmId)) {
            return 'VM ID not found for this service';
        }

        $result = $client->powerOperation($vmId, $operation);

        VirtueStackHelper::log("powerOperation ({$operation})", 'Success', json_encode($result));

        return 'success';
    } catch (\Exception $e) {
        VirtueStackHelper::log("powerOperation ({$operation})", 'Failed', $e->getMessage());
        return 'Failed to ' . $operation . ' VM: ' . $e->getMessage();
    }
}