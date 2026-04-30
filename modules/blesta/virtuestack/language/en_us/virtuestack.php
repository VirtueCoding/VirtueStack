<?php
/**
 * VirtueStack Language Definitions (English)
 *
 * @package blesta
 * @subpackage blesta.components.modules.virtuestack
 */

$lang['Virtuestack.name'] = 'VirtueStack';
$lang['Virtuestack.description'] = 'VirtueStack VPS provisioning module for managing KVM/QEMU virtual machines.';
$lang['Virtuestack.module_row'] = 'VirtueStack Server';
$lang['Virtuestack.module_row_plural'] = 'VirtueStack Servers';

// Add Row
$lang['Virtuestack.add_row.title'] = 'Add VirtueStack Server';
$lang['Virtuestack.add_row.hostname'] = 'Hostname';
$lang['Virtuestack.add_row.port'] = 'Port';
$lang['Virtuestack.add_row.api_key'] = 'API Key';
$lang['Virtuestack.add_row.use_ssl'] = 'Use SSL';
$lang['Virtuestack.add_row.webui_url'] = 'WebUI URL';
$lang['Virtuestack.add_row.submit'] = 'Add Server';

// Edit Row
$lang['Virtuestack.edit_row.title'] = 'Edit VirtueStack Server';
$lang['Virtuestack.edit_row.hostname'] = 'Hostname';
$lang['Virtuestack.edit_row.port'] = 'Port';
$lang['Virtuestack.edit_row.api_key'] = 'API Key';
$lang['Virtuestack.edit_row.use_ssl'] = 'Use SSL';
$lang['Virtuestack.edit_row.webui_url'] = 'WebUI URL';
$lang['Virtuestack.edit_row.submit'] = 'Update Server';

// Package Fields
$lang['Virtuestack.package_fields.plan_id'] = 'Plan ID';
$lang['Virtuestack.package_fields.template_id'] = 'Template ID';
$lang['Virtuestack.package_fields.location_id'] = 'Location ID';
$lang['Virtuestack.package_fields.hostname_prefix'] = 'Hostname Prefix';

// Service Fields
$lang['Virtuestack.service_fields.vm_id'] = 'VM ID';
$lang['Virtuestack.service_fields.vm_ip'] = 'IP Address';
$lang['Virtuestack.service_fields.vm_status'] = 'VM Status';

// Tabs
$lang['Virtuestack.tab_client_service'] = 'Server Management';
$lang['Virtuestack.tab_admin_service'] = 'Server Details';
$lang['Virtuestack.tab_client_console'] = 'Console';

// Status Labels
$lang['Virtuestack.status.running'] = 'Running';
$lang['Virtuestack.status.stopped'] = 'Stopped';
$lang['Virtuestack.status.suspended'] = 'Suspended';
$lang['Virtuestack.status.provisioning'] = 'Provisioning';
$lang['Virtuestack.status.migrating'] = 'Migrating';
$lang['Virtuestack.status.reinstalling'] = 'Reinstalling';
$lang['Virtuestack.status.error'] = 'Error';
$lang['Virtuestack.status.deleted'] = 'Deleted';
$lang['Virtuestack.status.unknown'] = 'Unknown';

// Action Labels
$lang['Virtuestack.action.start'] = 'Start';
$lang['Virtuestack.action.stop'] = 'Stop';
$lang['Virtuestack.action.restart'] = 'Restart';
$lang['Virtuestack.action.force_stop'] = 'Force Stop';
$lang['Virtuestack.action.reset_password'] = 'Reset Password';
$lang['Virtuestack.action.open_console'] = 'Open Console';
$lang['Virtuestack.action.sync_status'] = 'Sync Status';
$lang['Virtuestack.action.open_new_tab'] = 'Open in New Tab';

// Provisioning Messages
$lang['Virtuestack.provisioning.pending'] = 'Your server is being provisioned. This may take a few minutes.';
$lang['Virtuestack.provisioning.active'] = 'Your server is active and ready to use.';
$lang['Virtuestack.provisioning.error'] = 'An error occurred during provisioning. Please contact support.';
$lang['Virtuestack.provisioning.suspended'] = 'This service is currently suspended.';
$lang['Virtuestack.provisioning.terminated'] = 'This service has been terminated.';

// Admin Service Tab
$lang['Virtuestack.admin.vm_id'] = 'VM ID';
$lang['Virtuestack.admin.vm_ip'] = 'IP Address';
$lang['Virtuestack.admin.vm_status'] = 'VM Status';
$lang['Virtuestack.admin.node'] = 'Node';
$lang['Virtuestack.admin.plan'] = 'Plan';
$lang['Virtuestack.admin.vcpu'] = 'vCPU';
$lang['Virtuestack.admin.memory'] = 'Memory';
$lang['Virtuestack.admin.disk'] = 'Disk';
$lang['Virtuestack.admin.task_id'] = 'Task ID';
$lang['Virtuestack.admin.provisioning_status'] = 'Provisioning Status';
$lang['Virtuestack.admin.power_actions'] = 'Power Actions';
$lang['Virtuestack.admin.resources'] = 'Resources';
$lang['Virtuestack.admin.no_vm'] = 'No VM associated with this service.';

// Error Messages
$lang['Virtuestack.error.hostname_required'] = 'A server hostname is required.';
$lang['Virtuestack.error.api_key_required'] = 'An API key is required.';
$lang['Virtuestack.error.invalid_port'] = 'The port must be a number between 1 and 65535.';
$lang['Virtuestack.error.connection_failed'] = 'Could not connect to the VirtueStack server. Please verify the hostname, port, and API key.';
$lang['Virtuestack.error.plan_id_required'] = 'A valid Plan ID (UUID) is required.';
$lang['Virtuestack.error.template_id_required'] = 'A valid Template ID (UUID) is required.';
$lang['Virtuestack.error.invalid_uuid'] = 'The value provided is not a valid UUID.';
$lang['Virtuestack.error.vm_not_found'] = 'The virtual machine could not be found.';
$lang['Virtuestack.error.api_error'] = 'VirtueStack API error: %1$s';
$lang['Virtuestack.error.no_module_row'] = 'No VirtueStack server is configured for this package.';
$lang['Virtuestack.error.client_id_required'] = 'A positive Blesta client ID is required before provisioning can start.';
$lang['Virtuestack.error.service_id_required'] = 'A positive Blesta service ID is required before provisioning can start.';
