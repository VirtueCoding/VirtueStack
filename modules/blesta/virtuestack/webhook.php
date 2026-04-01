<?php
/**
 * VirtueStack Webhook Handler for Blesta
 *
 * Receives VM lifecycle events from VirtueStack and updates Blesta service records.
 * URL: https://your-blesta.com/components/modules/virtuestack/webhook.php
 *
 * @package blesta
 * @subpackage blesta.components.modules.virtuestack
 */

// Ensure constant is defined for path separator
if (!defined('DS')) {
    define('DS', DIRECTORY_SEPARATOR);
}

// Bootstrap Blesta
$blestaRoot = dirname(dirname(dirname(dirname(__FILE__))));
$initPath = $blestaRoot . DS . 'vendors' . DS . 'non-composer'
    . DS . 'blesta' . DS . 'init.php';

if (!file_exists($initPath)) {
    sendResponse(500, ['error' => 'Blesta initialization file not found.']);
    exit;
}

require_once $initPath;
require_once dirname(__FILE__) . DS . 'lib' . DS . 'VirtueStackHelper.php';

// Constants
define('MAX_BODY_SIZE', 65536); // 64KB
define('ALLOWED_EVENTS', [
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
    'task.completed',
    'task.failed',
]);

processWebhook();

/**
 * Main webhook processing entry point.
 */
function processWebhook(): void
{
    // Only accept POST
    if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
        sendResponse(405, ['error' => 'Method not allowed.']);
        return;
    }

    // Validate Content-Type
    $contentType = $_SERVER['CONTENT_TYPE'] ?? '';
    if (stripos($contentType, 'application/json') === false) {
        sendResponse(415, ['error' => 'Content-Type must be application/json.']);
        return;
    }

    // Read body with size limit
    $body = file_get_contents('php://input', false, null, 0, MAX_BODY_SIZE + 1);
    if ($body === false || strlen($body) === 0) {
        sendResponse(400, ['error' => 'Empty request body.']);
        return;
    }

    if (strlen($body) > MAX_BODY_SIZE) {
        sendResponse(413, ['error' => 'Request body too large.']);
        return;
    }

    // Verify signature
    $signature = $_SERVER['HTTP_X_VIRTUESTACK_SIGNATURE'] ?? '';
    $webhookSecret = getWebhookSecret();

    if (empty($signature)) {
        sendResponse(401, ['error' => 'Missing signature header.']);
        return;
    }

    if (empty($webhookSecret)) {
        sendResponse(401, ['error' => 'Invalid signature.']);
        return;
    }

    if (!VirtueStackHelper::verifyWebhookSignature($body, $signature, $webhookSecret)) {
        sendResponse(401, ['error' => 'Invalid signature.']);
        return;
    }

    // Parse JSON
    $payload = json_decode($body, true);
    if ($payload === null) {
        sendResponse(400, ['error' => 'Invalid JSON payload.']);
        return;
    }

    // Validate required fields
    $event = $payload['event'] ?? '';
    if (empty($event)) {
        sendResponse(400, ['error' => 'Missing event field.']);
        return;
    }

    // Validate event whitelist
    if (!in_array($event, ALLOWED_EVENTS, true)) {
        sendResponse(400, ['error' => 'Unknown event type: ' . $event]);
        return;
    }

    // Extract fields
    $externalServiceId = $payload['external_service_id'] ?? null;
    $vmId = $payload['vm_id'] ?? null;
    $taskId = $payload['task_id'] ?? null;
    $data = $payload['data'] ?? [];
    $errorMessage = $payload['error'] ?? ($data['error'] ?? '');

    // Validate UUIDs where present
    if ($vmId !== null && !VirtueStackHelper::isValidUUID($vmId)) {
        sendResponse(400, ['error' => 'Invalid vm_id format.']);
        return;
    }
    if ($taskId !== null && !VirtueStackHelper::isValidUUID($taskId)) {
        sendResponse(400, ['error' => 'Invalid task_id format.']);
        return;
    }

    // Look up Blesta service
    $serviceId = findServiceId($externalServiceId, $vmId, $taskId);
    if ($serviceId === null) {
        sendResponse(200, ['status' => 'ok', 'message' => 'Service not found, event ignored.']);
        return;
    }

    // Process event
    handleEvent($event, $serviceId, $vmId, $taskId, $data, $errorMessage);

    sendResponse(200, ['status' => 'ok']);
}

/**
 * Handle a webhook event by updating service fields.
 *
 * @param string $event Event type
 * @param int $serviceId Blesta service ID
 * @param string|null $vmId VM UUID
 * @param string|null $taskId Task UUID
 * @param array $data Event data
 * @param string $errorMessage Error message for failure events
 */
function handleEvent(
    string $event,
    int $serviceId,
    ?string $vmId,
    ?string $taskId,
    array $data,
    string $errorMessage
): void {
    switch ($event) {
        case 'vm.created':
            $vmIp = '';
            if (!empty($data['ip_addresses']) && is_array($data['ip_addresses'])) {
                if (isset($data['ip_addresses'][0]) && is_array($data['ip_addresses'][0])) {
                    $vmIp = isset($data['ip_addresses'][0]['address'])
                        && is_string($data['ip_addresses'][0]['address'])
                        ? $data['ip_addresses'][0]['address']
                        : '';
                } elseif (isset($data['ip_addresses'][0]) && is_string($data['ip_addresses'][0])) {
                    $vmIp = $data['ip_addresses'][0];
                }
            }
            updateServiceField($serviceId, 'vm_id', $vmId ?? '');
            updateServiceField($serviceId, 'vm_ip', $vmIp);
            updateServiceField($serviceId, 'vm_status', 'running');
            updateServiceField($serviceId, 'provisioning_status', 'active');
            updateServiceField($serviceId, 'task_id', '');
            updateServiceField($serviceId, 'provisioning_error', '');
            break;

        case 'vm.creation_failed':
            updateServiceField($serviceId, 'provisioning_status', 'error');
            updateServiceField($serviceId, 'provisioning_error', $errorMessage);
            updateServiceField($serviceId, 'task_id', '');
            break;

        case 'vm.deleted':
            updateServiceField($serviceId, 'provisioning_status', 'terminated');
            break;

        case 'vm.suspended':
            updateServiceField($serviceId, 'vm_status', 'suspended');
            updateServiceField($serviceId, 'provisioning_status', 'suspended');
            break;

        case 'vm.unsuspended':
            updateServiceField($serviceId, 'vm_status', 'stopped');
            updateServiceField($serviceId, 'provisioning_status', 'active');
            break;

        case 'vm.started':
            updateServiceField($serviceId, 'vm_status', 'running');
            break;

        case 'vm.stopped':
            updateServiceField($serviceId, 'vm_status', 'stopped');
            break;

        case 'vm.reinstalled':
            updateServiceField($serviceId, 'vm_status', 'running');
            updateServiceField($serviceId, 'provisioning_status', 'active');
            break;

        case 'vm.migrated':
            $nodeId = $data['node_id'] ?? '';
            if (!empty($nodeId) && VirtueStackHelper::isValidUUID($nodeId)) {
                updateServiceField($serviceId, 'node_id', $nodeId);
            }
            break;

        case 'vm.resized':
            // Log only, no field updates needed
            break;

        case 'task.completed':
            updateServiceField($serviceId, 'task_id', '');
            break;

        case 'task.failed':
            updateServiceField($serviceId, 'provisioning_error', $errorMessage);
            updateServiceField($serviceId, 'task_id', '');
            break;
    }
}

/**
 * Find a Blesta service ID by external service ID, VM ID, or task ID.
 *
 * @param int|string|null $externalServiceId External service ID
 * @param string|null $vmId VM UUID
 * @param string|null $taskId Task UUID
 * @return int|null Service ID or null if not found
 */
function findServiceId($externalServiceId, ?string $vmId, ?string $taskId): ?int
{
    // Direct external_service_id match
    if ($externalServiceId !== null) {
        $id = (int) $externalServiceId;
        if ($id > 0) {
            return $id;
        }
    }

    // Look up by vm_id in service fields
    if ($vmId !== null) {
        $serviceId = findServiceByFieldValue('vm_id', $vmId);
        if ($serviceId !== null) {
            return $serviceId;
        }
    }

    // Look up by task_id in service fields
    if ($taskId !== null) {
        $serviceId = findServiceByFieldValue('task_id', $taskId);
        if ($serviceId !== null) {
            return $serviceId;
        }
    }

    return null;
}

/**
 * Find a Blesta service by a service field key/value pair.
 *
 * @param string $key Field key
 * @param string $value Field value
 * @return int|null Service ID or null
 */
function findServiceByFieldValue(string $key, string $value): ?int
{
    if (empty($value)) {
        return null;
    }

    try {
        $record = new Record();
        $result = $record->select(['service_id'])
            ->from('service_fields')
            ->where('key', '=', $key)
            ->where('value', '=', $value)
            ->fetch();

        if ($result && isset($result->service_id)) {
            return (int) $result->service_id;
        }
    } catch (Exception $e) {
        // Database error, cannot find service
    }

    return null;
}

/**
 * Update a service field value in Blesta.
 *
 * @param int $serviceId Blesta service ID
 * @param string $key Field key
 * @param string $value Field value
 */
function updateServiceField(int $serviceId, string $key, string $value): void
{
    try {
        $record = new Record();

        // Check if field exists
        $existing = $record->select(['id'])
            ->from('service_fields')
            ->where('service_id', '=', $serviceId)
            ->where('key', '=', $key)
            ->fetch();

        if ($existing) {
            $record->where('service_id', '=', $serviceId)
                ->where('key', '=', $key)
                ->update('service_fields', [
                    'value' => $value,
                ]);
        } else {
            $record->insert('service_fields', [
                'service_id' => $serviceId,
                'key' => $key,
                'value' => $value,
                'serialized' => 0,
                'encrypted' => 0,
            ]);
        }
    } catch (Exception $e) {
        // Log but don't fail the webhook
    }
}

/**
 * Get the webhook secret from the first VirtueStack module row.
 *
 * @return string Webhook secret
 */
function getWebhookSecret(): string
{
    try {
        $record = new Record();

        // Find the VirtueStack module
        $module = $record->select(['id'])
            ->from('modules')
            ->where('class', '=', 'virtuestack')
            ->fetch();

        if (!$module) {
            return '';
        }

        // Get the first module row's webhook_secret
        $row = $record->select(['module_rows.id'])
            ->from('module_rows')
            ->where('module_id', '=', $module->id)
            ->fetch();

        if (!$row) {
            return '';
        }

        $meta = $record->select(['value'])
            ->from('module_row_meta')
            ->where('module_row_id', '=', $row->id)
            ->where('key', '=', 'webhook_secret')
            ->fetch();

        if ($meta && !empty($meta->value)) {
            return $meta->value;
        }
    } catch (Exception $e) {
        // Cannot retrieve secret
    }

    return '';
}

/**
 * Send a JSON response and exit.
 *
 * @param int $statusCode HTTP status code
 * @param array $data Response data
 */
function sendResponse(int $statusCode, array $data): void
{
    http_response_code($statusCode);
    header('Content-Type: application/json');
    echo json_encode($data);
    exit;
}
