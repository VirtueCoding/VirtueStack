<?php
/**
 * VirtueStack Module for Blesta
 *
 * Manages KVM/QEMU virtual machines via the VirtueStack Provisioning API.
 * Provides VM provisioning, suspension, termination, power management,
 * and SSO-based customer portal access.
 *
 * @package blesta
 * @subpackage blesta.components.modules.virtuestack
 */
class Virtuestack extends \Module
{
    /**
     * @var array Cached API client instances keyed by module row ID
     */
    private array $apiClients = [];

    /**
     * Initialize the VirtueStack module.
     */
    public function __construct()
    {
        Loader::loadComponents($this, ['Input', 'Record']);
        Language::loadLang(
            'virtuestack',
            null,
            dirname(__FILE__) . DS . 'language' . DS
        );
        $this->loadConfig(dirname(__FILE__) . DS . 'config.json');
    }

    // ---- Module Row (Server Configuration) ----

    /**
     * Add a new VirtueStack server configuration.
     *
     * @param array $vars Server configuration values
     * @return array Module row meta on success
     */
    public function addModuleRow(array &$vars)
    {
        $meta = $this->validateModuleRow($vars);
        if ($this->Input->errors()) {
            return;
        }

        $meta['hostname'] = $vars['hostname'] ?? '';
        $meta['port'] = !empty($vars['port']) ? (int) $vars['port'] : 443;
        $meta['api_key'] = $vars['api_key'] ?? '';
        $meta['use_ssl'] = isset($vars['use_ssl']) ? '1' : '1';
        $meta['webui_url'] = $vars['webui_url'] ?? '';
        $meta['webhook_secret'] = $vars['webhook_secret'] ?? '';

        if (!$this->testConnection($meta)) {
            return;
        }

        return [['key' => 'hostname', 'value' => $meta['hostname'], 'encrypted' => 0],
            ['key' => 'port', 'value' => (string) $meta['port'], 'encrypted' => 0],
            ['key' => 'api_key', 'value' => $meta['api_key'], 'encrypted' => 1],
            ['key' => 'use_ssl', 'value' => $meta['use_ssl'], 'encrypted' => 0],
            ['key' => 'webui_url', 'value' => $meta['webui_url'], 'encrypted' => 0],
            ['key' => 'webhook_secret', 'value' => $meta['webhook_secret'], 'encrypted' => 1],
        ];
    }

    /**
     * Edit an existing VirtueStack server configuration.
     *
     * @param object $module_row The existing module row
     * @param array $vars Updated configuration values
     * @return array Module row meta on success
     */
    public function editModuleRow($module_row, array &$vars)
    {
        $meta = $this->validateModuleRow($vars);
        if ($this->Input->errors()) {
            return;
        }

        $meta['hostname'] = $vars['hostname'] ?? '';
        $meta['port'] = !empty($vars['port']) ? (int) $vars['port'] : 443;
        $meta['api_key'] = $vars['api_key'] ?? '';
        $meta['use_ssl'] = isset($vars['use_ssl']) ? '1' : '1';
        $meta['webui_url'] = $vars['webui_url'] ?? '';
        $meta['webhook_secret'] = $vars['webhook_secret'] ?? '';

        if (!$this->testConnection($meta)) {
            return;
        }

        return [['key' => 'hostname', 'value' => $meta['hostname'], 'encrypted' => 0],
            ['key' => 'port', 'value' => (string) $meta['port'], 'encrypted' => 0],
            ['key' => 'api_key', 'value' => $meta['api_key'], 'encrypted' => 1],
            ['key' => 'use_ssl', 'value' => $meta['use_ssl'], 'encrypted' => 0],
            ['key' => 'webui_url', 'value' => $meta['webui_url'], 'encrypted' => 0],
            ['key' => 'webhook_secret', 'value' => $meta['webhook_secret'], 'encrypted' => 1],
        ];
    }

    /**
     * Delete a module row (no-op).
     *
     * @param object $module_row The module row to delete
     */
    public function deleteModuleRow($module_row)
    {
        // Nothing to clean up
    }

    /**
     * Render the add server form.
     *
     * @param array $vars Form values
     * @return string Rendered HTML
     */
    public function manageAddRow(array &$vars)
    {
        $this->view = new View('add_row', 'default');
        $this->view->base_uri = $this->base_uri;
        $this->view->setDefaultView(
            'components' . DS . 'modules' . DS . 'virtuestack' . DS
        );

        Loader::loadHelpers($this, ['Form', 'Html']);

        $this->view->set('vars', (object) $vars);

        return $this->view->fetch();
    }

    /**
     * Render the edit server form.
     *
     * @param object $module_row The existing module row
     * @param array $vars Form values
     * @return string Rendered HTML
     */
    public function manageEditRow($module_row, array &$vars)
    {
        if (empty($vars)) {
            $vars = [
                'hostname' => $module_row->meta->hostname ?? '',
                'port' => $module_row->meta->port ?? '443',
                'api_key' => '',
                'use_ssl' => $module_row->meta->use_ssl ?? '1',
                'webui_url' => $module_row->meta->webui_url ?? '',
                'webhook_secret' => '',
            ];
        }

        $this->view = new View('edit_row', 'default');
        $this->view->base_uri = $this->base_uri;
        $this->view->setDefaultView(
            'components' . DS . 'modules' . DS . 'virtuestack' . DS
        );

        Loader::loadHelpers($this, ['Form', 'Html']);

        $this->view->set('vars', (object) $vars);

        return $this->view->fetch();
    }

    // ---- Package Fields ----

    /**
     * Define package configuration fields.
     *
     * @param mixed $vars Current field values
     * @return ModuleFields
     */
    public function getPackageFields($vars = null)
    {
        Loader::loadHelpers($this, ['Html']);

        $fields = new ModuleFields();

        $planId = $fields->label(
            Language::_('Virtuestack.package_fields.plan_id', true),
            'virtuestack_plan_id'
        );
        $planId->attach(
            $fields->fieldText(
                'meta[plan_id]',
                $this->Html->ifSet($vars->meta['plan_id'] ?? null),
                ['id' => 'virtuestack_plan_id', 'maxlength' => 36]
            )
        );
        $fields->setField($planId);

        $templateId = $fields->label(
            Language::_('Virtuestack.package_fields.template_id', true),
            'virtuestack_template_id'
        );
        $templateId->attach(
            $fields->fieldText(
                'meta[template_id]',
                $this->Html->ifSet($vars->meta['template_id'] ?? null),
                ['id' => 'virtuestack_template_id', 'maxlength' => 36]
            )
        );
        $fields->setField($templateId);

        $locationId = $fields->label(
            Language::_('Virtuestack.package_fields.location_id', true),
            'virtuestack_location_id'
        );
        $locationId->attach(
            $fields->fieldText(
                'meta[location_id]',
                $this->Html->ifSet($vars->meta['location_id'] ?? null),
                ['id' => 'virtuestack_location_id', 'maxlength' => 36]
            )
        );
        $fields->setField($locationId);

        $hostnamePrefix = $fields->label(
            Language::_('Virtuestack.package_fields.hostname_prefix', true),
            'virtuestack_hostname_prefix'
        );
        $hostnamePrefix->attach(
            $fields->fieldText(
                'meta[hostname_prefix]',
                $this->Html->ifSet($vars->meta['hostname_prefix'] ?? null, 'vps'),
                ['id' => 'virtuestack_hostname_prefix', 'maxlength' => 20]
            )
        );
        $fields->setField($hostnamePrefix);

        return $fields;
    }

    /**
     * Validate package configuration.
     *
     * @param array $vars Package configuration values
     * @return bool True if valid
     */
    public function validatePackage(?array $vars = null)
    {
        require_once dirname(__FILE__) . DS . 'lib' . DS . 'VirtueStackHelper.php';

        $errors = [];
        $meta = $vars['meta'] ?? [];

        $planId = $meta['plan_id'] ?? '';
        if (empty($planId) || !VirtueStackHelper::isValidUUID($planId)) {
            $errors['plan_id']['valid'] = Language::_(
                'Virtuestack.error.plan_id_required',
                true
            );
        }

        $templateId = $meta['template_id'] ?? '';
        if (empty($templateId) || !VirtueStackHelper::isValidUUID($templateId)) {
            $errors['template_id']['valid'] = Language::_(
                'Virtuestack.error.template_id_required',
                true
            );
        }

        $locationId = $meta['location_id'] ?? '';
        if (!empty($locationId) && !VirtueStackHelper::isValidUUID($locationId)) {
            $errors['location_id']['valid'] = Language::_(
                'Virtuestack.error.invalid_uuid',
                true
            );
        }

        if (!empty($errors)) {
            $this->Input->setErrors($errors);
            return false;
        }

        return true;
    }

    // ---- API Client Factory ----

    /**
     * Build or retrieve a cached API client for a module row.
     *
     * @param object $moduleRow Blesta module row
     * @return VirtueStackApiClient
     */
    private function getApi($moduleRow)
    {
        $rowId = $moduleRow->id ?? 0;
        if (isset($this->apiClients[$rowId])) {
            return $this->apiClients[$rowId];
        }

        require_once dirname(__FILE__) . DS . 'lib' . DS . 'ApiClient.php';

        $hostname = $moduleRow->meta->hostname ?? '';
        $port = $moduleRow->meta->port ?? '443';
        $apiKey = $moduleRow->meta->api_key ?? '';

        $apiUrl = 'https://' . $hostname . ':' . $port . '/api/v1';

        $client = new VirtueStackApiClient($apiUrl, $apiKey);
        $this->apiClients[$rowId] = $client;

        return $client;
    }

    // ---- Service Lifecycle ----

    /**
     * Provision a new VM service.
     *
     * @param object $package The service package
     * @param array $vars Service creation variables
     * @param object|null $parent_package Parent package (unused)
     * @param object|null $parent_service Parent service (unused)
     * @param string $status Initial service status
     * @return array Service fields on success
     */
    public function addService(
        $package,
        ?array $vars = null,
        $parent_package = null,
        $parent_service = null,
        $status = 'pending'
    ) {
        $moduleRow = $this->getModuleRow($package->module_row ?? 0);
        if (!$moduleRow) {
            $this->Input->setErrors([
                'module_row' => [
                    'missing' => Language::_(
                        'Virtuestack.error.no_module_row',
                        true
                    ),
                ],
            ]);
            return;
        }

        require_once dirname(__FILE__) . DS . 'lib' . DS . 'VirtueStackHelper.php';

        $api = $this->getApi($moduleRow);

        $planId = $package->meta->plan_id ?? '';
        $templateId = $package->meta->template_id ?? '';
        $locationId = $package->meta->location_id ?? '';
        $hostnamePrefix = $package->meta->hostname_prefix ?? 'vps';

        $clientId = $vars['client_id'] ?? 0;
        $serviceId = $vars['service_id'] ?? ('temp-' . time());

        // Idempotency: check if VM already exists for this service
        try {
            $existing = $api->getVMByServiceId((int) $serviceId);
            if (!empty($existing['id'])) {
                $vmIp = '';
                if (!empty($existing['ip_addresses'])
                    && is_array($existing['ip_addresses'])) {
                    $vmIp = $existing['ip_addresses'][0]['address'] ?? '';
                }
                return [
                    ['key' => 'vm_id', 'value' => $existing['id'], 'encrypted' => 0],
                    ['key' => 'vm_ip', 'value' => $vmIp, 'encrypted' => 0],
                    ['key' => 'vm_status', 'value' => $existing['status'] ?? 'running', 'encrypted' => 0],
                    ['key' => 'provisioning_status', 'value' => 'active', 'encrypted' => 0],
                    ['key' => 'task_id', 'value' => '', 'encrypted' => 0],
                    ['key' => 'virtuestack_customer_id', 'value' => $existing['customer_id'] ?? '', 'encrypted' => 0],
                    ['key' => 'provisioning_error', 'value' => '', 'encrypted' => 0],
                ];
            }
        } catch (Exception $e) {
            // VM doesn't exist yet — proceed with creation
        }

        // Ensure customer exists in VirtueStack
        $customerId = '';
        try {
            Loader::loadModels($this, ['Clients']);
            $client = $this->Clients->get($clientId);

            $email = $client->email ?? '';
            $name = trim(
                ($client->first_name ?? '') . ' ' . ($client->last_name ?? '')
            );

            $customerData = $api->createCustomer($email, $name, (int) $clientId);
            $customerId = $customerData['id'] ?? '';
        } catch (Exception $e) {
            $this->Input->setErrors([
                'customer' => [
                    'create' => sprintf(
                        Language::_('Virtuestack.error.api_error', true),
                        $e->getMessage()
                    ),
                ],
            ]);
            return;
        }

        // Build hostname
        $hostname = $vars['hostname']
            ?? ($hostnamePrefix . '-' . $serviceId);

        // Create VM
        try {
            $vmParams = [
                'customer_id' => $customerId,
                'plan_id' => $planId,
                'template_id' => $templateId,
                'hostname' => $hostname,
                'external_service_id' => (int) $serviceId,
            ];

            if (!empty($locationId)) {
                $vmParams['location_id'] = $locationId;
            }

            $vmData = $api->createVM($vmParams);

            return [
                ['key' => 'vm_id', 'value' => $vmData['vm_id'] ?? '', 'encrypted' => 0],
                ['key' => 'vm_ip', 'value' => '', 'encrypted' => 0],
                ['key' => 'vm_status', 'value' => 'provisioning', 'encrypted' => 0],
                ['key' => 'provisioning_status', 'value' => 'pending', 'encrypted' => 0],
                ['key' => 'task_id', 'value' => $vmData['task_id'] ?? '', 'encrypted' => 0],
                ['key' => 'virtuestack_customer_id', 'value' => $customerId, 'encrypted' => 0],
                ['key' => 'provisioning_error', 'value' => '', 'encrypted' => 0],
            ];
        } catch (Exception $e) {
            $this->Input->setErrors([
                'vm' => [
                    'create' => sprintf(
                        Language::_('Virtuestack.error.api_error', true),
                        $e->getMessage()
                    ),
                ],
            ]);
            return;
        }
    }

    /**
     * Terminate/cancel a VM service.
     *
     * @param object $package The service package
     * @param object $service The service to cancel
     * @param object|null $parent_package Parent package (unused)
     * @param object|null $parent_service Parent service (unused)
     * @return array Updated service fields
     */
    public function cancelService(
        $package,
        $service,
        $parent_package = null,
        $parent_service = null
    ) {
        $vmId = $this->getFieldValue($service, 'vm_id');
        if (empty($vmId)) {
            return null;
        }

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);
        if (!$moduleRow) {
            return null;
        }

        try {
            $api = $this->getApi($moduleRow);
            $api->deleteVM($vmId);
        } catch (Exception $e) {
            $this->Input->setErrors([
                'vm' => [
                    'cancel' => sprintf(
                        Language::_('Virtuestack.error.api_error', true),
                        $e->getMessage()
                    ),
                ],
            ]);
            return;
        }

        return null;
    }

    /**
     * Suspend a VM service.
     *
     * @param object $package The service package
     * @param object $service The service to suspend
     * @param object|null $parent_package Parent package (unused)
     * @param object|null $parent_service Parent service (unused)
     * @return array Updated service fields
     */
    public function suspendService(
        $package,
        $service,
        $parent_package = null,
        $parent_service = null
    ) {
        $vmId = $this->getFieldValue($service, 'vm_id');
        if (empty($vmId)) {
            return null;
        }

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);
        if (!$moduleRow) {
            return null;
        }

        try {
            $api = $this->getApi($moduleRow);
            $api->suspendVM($vmId);
        } catch (Exception $e) {
            $this->Input->setErrors([
                'vm' => [
                    'suspend' => sprintf(
                        Language::_('Virtuestack.error.api_error', true),
                        $e->getMessage()
                    ),
                ],
            ]);
            return;
        }

        return null;
    }

    /**
     * Unsuspend a VM service.
     *
     * @param object $package The service package
     * @param object $service The service to unsuspend
     * @param object|null $parent_package Parent package (unused)
     * @param object|null $parent_service Parent service (unused)
     * @return array Updated service fields
     */
    public function unsuspendService(
        $package,
        $service,
        $parent_package = null,
        $parent_service = null
    ) {
        $vmId = $this->getFieldValue($service, 'vm_id');
        if (empty($vmId)) {
            return null;
        }

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);
        if (!$moduleRow) {
            return null;
        }

        try {
            $api = $this->getApi($moduleRow);
            $api->unsuspendVM($vmId);
        } catch (Exception $e) {
            $this->Input->setErrors([
                'vm' => [
                    'unsuspend' => sprintf(
                        Language::_('Virtuestack.error.api_error', true),
                        $e->getMessage()
                    ),
                ],
            ]);
            return;
        }

        return null;
    }

    /**
     * Edit a service (handles plan resize on package change).
     *
     * @param object $package The new package
     * @param object $service The service to edit
     * @param array $vars Updated service variables
     * @param object|null $parent_package Parent package (unused)
     * @param object|null $parent_service Parent service (unused)
     * @return array Updated service fields
     */
    public function editService(
        $package,
        $service,
        array $vars = [],
        $parent_package = null,
        $parent_service = null
    ) {
        $vmId = $this->getFieldValue($service, 'vm_id');
        if (empty($vmId)) {
            return null;
        }

        $newPlanId = $package->meta->plan_id ?? '';
        if (empty($newPlanId)) {
            return null;
        }

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);
        if (!$moduleRow) {
            return null;
        }

        try {
            $api = $this->getApi($moduleRow);
            $api->resizeVM($vmId, $newPlanId);
        } catch (Exception $e) {
            $this->Input->setErrors([
                'vm' => [
                    'resize' => sprintf(
                        Language::_('Virtuestack.error.api_error', true),
                        $e->getMessage()
                    ),
                ],
            ]);
            return;
        }

        return null;
    }

    // ---- Tabs ----

    /**
     * Return admin tab definitions.
     *
     * @param object $package The service package
     * @return array Tab definitions
     */
    public function getAdminTabs($package)
    {
        return [
            'tabAdminService' => Language::_(
                'Virtuestack.tab_admin_service',
                true
            ),
        ];
    }

    /**
     * Return client tab definitions.
     *
     * @param object $package The service package
     * @return array Tab definitions
     */
    public function getClientTabs($package)
    {
        return [
            'tabClientService' => Language::_(
                'Virtuestack.tab_client_service',
                true
            ),
            'tabClientConsole' => Language::_(
                'Virtuestack.tab_client_console',
                true
            ),
        ];
    }

    /**
     * Admin service details tab.
     *
     * @param object $package The service package
     * @param object $service The service
     * @param array $get GET parameters
     * @param array $post POST parameters
     * @param array $files File uploads
     * @return string Rendered tab content
     */
    public function tabAdminService(
        $package,
        $service,
        ?array $get = null,
        ?array $post = null,
        ?array $files = null
    ) {
        $this->view = new View('tab_admin_service', 'default');
        $this->view->base_uri = $this->base_uri;
        $this->view->setDefaultView(
            'components' . DS . 'modules' . DS . 'virtuestack' . DS
        );

        Loader::loadHelpers($this, ['Form', 'Html']);

        $vmId = $this->getFieldValue($service, 'vm_id');
        $vmIp = $this->getFieldValue($service, 'vm_ip');
        $vmStatus = $this->getFieldValue($service, 'vm_status');
        $taskId = $this->getFieldValue($service, 'task_id');
        $provStatus = $this->getFieldValue($service, 'provisioning_status');

        $vmInfo = null;
        $statusData = null;

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);

        // Handle POST actions
        if (!empty($post['action']) && !empty($vmId) && $moduleRow) {
            try {
                $api = $this->getApi($moduleRow);
                switch ($post['action']) {
                    case 'start':
                        $api->powerOperation($vmId, 'start');
                        break;
                    case 'stop':
                        $api->powerOperation($vmId, 'stop');
                        break;
                    case 'restart':
                        $api->powerOperation($vmId, 'restart');
                        break;
                    case 'force_stop':
                        $api->powerOperation($vmId, 'stop');
                        break;
                    case 'sync_status':
                        // Fetch and update below
                        break;
                }
            } catch (Exception $e) {
                // Show error in view
                $this->view->set('error', $e->getMessage());
            }
        }

        // Fetch VM info if available
        if (!empty($vmId) && $moduleRow) {
            try {
                $api = $this->getApi($moduleRow);
                $vmInfo = $api->getVMInfo($vmId);
                $statusData = $api->getVMStatus($vmId);
                $vmStatus = $statusData['status'] ?? $vmStatus;
            } catch (Exception $e) {
                // VM may not exist yet
            }
        }

        $this->view->set('vm_id', $vmId);
        $this->view->set('vm_ip', $vmIp);
        $this->view->set('vm_status', $vmStatus);
        $this->view->set('task_id', $taskId);
        $this->view->set('provisioning_status', $provStatus);
        $this->view->set('vm_info', $vmInfo);
        $this->view->set('service_id', $service->id ?? '');

        return $this->view->fetch();
    }

    /**
     * Client service management tab.
     *
     * @param object $package The service package
     * @param object $service The service
     * @param array $get GET parameters
     * @param array $post POST parameters
     * @param array $files File uploads
     * @return string Rendered tab content
     */
    public function tabClientService(
        $package,
        $service,
        ?array $get = null,
        ?array $post = null,
        ?array $files = null
    ) {
        $this->view = new View('tab_client_service', 'default');
        $this->view->base_uri = $this->base_uri;
        $this->view->setDefaultView(
            'components' . DS . 'modules' . DS . 'virtuestack' . DS
        );

        Loader::loadHelpers($this, ['Form', 'Html']);
        require_once dirname(__FILE__) . DS . 'lib' . DS . 'VirtueStackHelper.php';

        $vmId = $this->getFieldValue($service, 'vm_id');
        $provStatus = $this->getFieldValue($service, 'provisioning_status');
        $taskId = $this->getFieldValue($service, 'task_id');
        $provError = $this->getFieldValue($service, 'provisioning_error');

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);

        $this->view->set('vm_id', $vmId);
        $this->view->set('provisioning_status', $provStatus);
        $this->view->set('task_id', $taskId);
        $safeProvisioningError = !empty($provError)
            ? Language::_('Virtuestack.provisioning.error', true)
            : '';

        $this->view->set('provisioning_error', $safeProvisioningError);
        $this->view->set('iframe_url', '');
        $this->view->set('error', '');

        if ($provStatus === 'pending' && !empty($taskId) && $moduleRow) {
            // Poll task status
            try {
                $api = $this->getApi($moduleRow);
                $task = $api->getTask($taskId);
                $this->view->set('task_status', $task['status'] ?? 'pending');
                $this->view->set('task_progress', $task['progress'] ?? 0);
            } catch (Exception $e) {
                // Task polling failed
            }
        } elseif ($provStatus === 'active' && !empty($vmId) && $moduleRow) {
            // Handle POST power operations
            if (!empty($post['action'])) {
                try {
                    $api = $this->getApi($moduleRow);
                    $action = $post['action'];
                    if (in_array($action, ['start', 'stop', 'restart'], true)) {
                        $api->powerOperation($vmId, $action);
                    } elseif ($action === 'reset_password') {
                        $api->resetPassword($vmId);
                    }
                } catch (Exception $e) {
                    $this->view->set(
                        'error',
                        'The VirtueStack portal is temporarily unavailable. Please try again later.'
                    );
                }
            }

            // Build SSO iframe URL
            try {
                $api = $this->getApi($moduleRow);
                $webuiUrl = $moduleRow->meta->webui_url ?? '';
                if (!empty($webuiUrl)) {
                    $ssoData = $api->createSSOToken(
                        (int) ($service->id ?? 0),
                        $vmId
                    );
                    $token = $ssoData['token'] ?? '';
                    if (!empty($token)) {
                        $iframeUrl = VirtueStackHelper::buildSSOUrl(
                            $webuiUrl,
                            $token
                        );
                        $this->view->set('iframe_url', $iframeUrl);
                    }
                }
            } catch (Exception $e) {
                $this->view->set(
                    'error',
                    'The VirtueStack portal is temporarily unavailable. Please try again later.'
                );
            }
        }

        return $this->view->fetch();
    }

    /**
     * Client console tab.
     *
     * @param object $package The service package
     * @param object $service The service
     * @param array $get GET parameters
     * @param array $post POST parameters
     * @param array $files File uploads
     * @return string Rendered tab content
     */
    public function tabClientConsole(
        $package,
        $service,
        ?array $get = null,
        ?array $post = null,
        ?array $files = null
    ) {
        $this->view = new View('tab_client_console', 'default');
        $this->view->base_uri = $this->base_uri;
        $this->view->setDefaultView(
            'components' . DS . 'modules' . DS . 'virtuestack' . DS
        );

        Loader::loadHelpers($this, ['Html']);
        require_once dirname(__FILE__) . DS . 'lib' . DS . 'VirtueStackHelper.php';

        $vmId = $this->getFieldValue($service, 'vm_id');
        $consoleUrl = '';

        $moduleRow = $this->getModuleRow($package->module_row ?? 0);

        if (!empty($vmId) && $moduleRow) {
            try {
                $api = $this->getApi($moduleRow);
                $webuiUrl = $moduleRow->meta->webui_url ?? '';
                if (!empty($webuiUrl)) {
                    $ssoData = $api->createSSOToken(
                        (int) ($service->id ?? 0),
                        $vmId
                    );
                    $token = $ssoData['token'] ?? '';
                    if (!empty($token)) {
                        $consoleUrl = VirtueStackHelper::buildConsoleUrl(
                            $webuiUrl,
                            $token
                        );
                    }
                }
            } catch (Exception $e) {
                // Console unavailable
            }
        }

        $this->view->set('console_url', $consoleUrl);
        $this->view->set('vm_id', $vmId);

        return $this->view->fetch();
    }

    /**
     * Get the service name for display.
     *
     * @param object $service The service
     * @return string Service name
     */
    public function getServiceName($service)
    {
        $vmId = $this->getFieldValue($service, 'vm_id');
        if (!empty($vmId)) {
            return $vmId;
        }

        return '';
    }

    // ---- Private Helpers ----

    /**
     * Extract a field value from a Blesta service.
     *
     * @param object $service Blesta service object
     * @param string $key Field key
     * @return string Field value or empty string
     */
    private function getFieldValue($service, string $key): string
    {
        if (isset($service->fields) && is_array($service->fields)) {
            foreach ($service->fields as $field) {
                if (isset($field->key) && $field->key === $key) {
                    return $field->value ?? '';
                }
            }
        }

        return '';
    }

    /**
     * Validate module row (server configuration) input.
     *
     * @param array $vars Input values
     * @return array Validated meta values
     */
    private function validateModuleRow(array $vars): array
    {
        $errors = [];

        if (empty($vars['hostname'])) {
            $errors['hostname']['empty'] = Language::_(
                'Virtuestack.error.hostname_required',
                true
            );
        }

        if (empty($vars['api_key'])) {
            $errors['api_key']['empty'] = Language::_(
                'Virtuestack.error.api_key_required',
                true
            );
        }

        $port = !empty($vars['port']) ? (int) $vars['port'] : 443;
        if ($port < 1 || $port > 65535) {
            $errors['port']['valid'] = Language::_(
                'Virtuestack.error.invalid_port',
                true
            );
        }

        if (!empty($errors)) {
            $this->Input->setErrors($errors);
        }

        return [
            'hostname' => $vars['hostname'] ?? '',
            'port' => $port,
            'api_key' => $vars['api_key'] ?? '',
            'use_ssl' => '1',
            'webui_url' => $vars['webui_url'] ?? '',
            'webhook_secret' => $vars['webhook_secret'] ?? '',
        ];
    }

    /**
     * Test connection to the VirtueStack API.
     *
     * @param array $meta Server meta values
     * @return bool True if connection succeeds
     */
    private function testConnection(array $meta): bool
    {
        try {
            require_once dirname(__FILE__) . DS . 'lib' . DS . 'ApiClient.php';

            $apiUrl = 'https://' . $meta['hostname'] . ':' . $meta['port']
                . '/api/v1';
            $client = new VirtueStackApiClient($apiUrl, $meta['api_key']);
            $client->healthCheck();

            return true;
        } catch (Exception $e) {
            $this->Input->setErrors([
                'connection' => [
                    'failed' => Language::_(
                        'Virtuestack.error.connection_failed',
                        true
                    ),
                ],
            ]);
            return false;
        }
    }
}
