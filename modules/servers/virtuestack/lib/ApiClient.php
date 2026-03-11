<?php

declare(strict_types=1);

/**
 * VirtueStack API Client for WHMCS Provisioning Module.
 *
 * Handles HTTP communication with the VirtueStack Controller Provisioning API.
 * All requests are authenticated via X-API-Key header and support async operations.
 *
 * @package   VirtueStack\WHMCS
 * @author    VirtueStack Team
 * @copyright 2026 VirtueStack
 * @license   MIT
 */

namespace VirtueStack\WHMCS;

use RuntimeException;
use InvalidArgumentException;

/**
 * HTTP client for VirtueStack Controller Provisioning API.
 *
 * Supports async operations via HTTP 202 + task_id polling.
 */
final class ApiClient
{
    private const DEFAULT_TIMEOUT = 30;
    private const TASK_POLL_INTERVAL = 3;
    private const TASK_MAX_POLLS = 60;

    private string $apiUrl;
    private string $apiKey;
    private int $timeout;
    private bool $verifySsl;
    private string $userAgent;

    /**
     * @param string $apiUrl    Base URL for Controller API (e.g., https://controller.example.com/api/v1)
     * @param string $apiKey    Provisioning API key
     * @param int    $timeout   Request timeout in seconds
     * @param bool   $verifySsl Whether to verify SSL certificates
     */
    public function __construct(
        string $apiUrl,
        string $apiKey,
        int $timeout = self::DEFAULT_TIMEOUT,
        bool $verifySsl = true
    ) {
        $apiUrl = rtrim($apiUrl, '/');
        
        if (!filter_var($apiUrl, FILTER_VALIDATE_URL)) {
            throw new InvalidArgumentException('Invalid API URL provided');
        }
        
        if (empty($apiKey)) {
            throw new InvalidArgumentException('API key cannot be empty');
        }

        $this->apiUrl = $apiUrl;
        $this->apiKey = $apiKey;
        $this->timeout = max(5, $timeout);
        $this->verifySsl = $verifySsl;
        $this->userAgent = 'VirtueStack-WHMCS/1.0 (PHP ' . PHP_VERSION . ')';
    }

    /**
     * Create a new VM asynchronously.
     *
     * @param array $params VM creation parameters:
     *                      - customer_id (string, UUID): Customer ID
     *                      - plan_id (string, UUID): Plan ID
     *                      - template_id (string, UUID): Template ID
     *                      - hostname (string): VM hostname
     *                      - whmcs_service_id (int): WHMCS service ID
     *                      - ssh_keys (array, optional): SSH public keys
     *                      - location_id (string, optional): Location ID
     *
     * @return array Response with task_id and vm_id
     *
     * @throws RuntimeException On API error
     */
    public function createVM(array $params): array
    {
        $required = ['customer_id', 'plan_id', 'template_id', 'hostname', 'whmcs_service_id'];
        $this->validateRequired($params, $required);

        $response = $this->request('POST', '/provisioning/vms', $params);

        if (!isset($response['data']['task_id'], $response['data']['vm_id'])) {
            throw new RuntimeException('Invalid response from API: missing task_id or vm_id');
        }

        return [
            'task_id' => $response['data']['task_id'],
            'vm_id'   => $response['data']['vm_id'],
        ];
    }

    /**
     * Suspend a VM.
     *
     * @param string $vmId VM UUID
     *
     * @return array Response data
     *
     * @throws RuntimeException On API error
     */
    public function suspendVM(string $vmId): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $response = $this->request('POST', "/provisioning/vms/{$vmId}/suspend");

        return $response['data'] ?? [];
    }

    /**
     * Unsuspend a VM.
     *
     * @param string $vmId VM UUID
     *
     * @return array Response data
     *
     * @throws RuntimeException On API error
     */
    public function unsuspendVM(string $vmId): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $response = $this->request('POST', "/provisioning/vms/{$vmId}/unsuspend");

        return $response['data'] ?? [];
    }

    /**
     * Delete (terminate) a VM asynchronously.
     *
     * @param string $vmId VM UUID
     *
     * @return array Response with task_id
     *
     * @throws RuntimeException On API error
     */
    public function deleteVM(string $vmId): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $response = $this->request('DELETE', "/provisioning/vms/{$vmId}");

        return $response['data'] ?? [];
    }

    /**
     * Resize a VM (change package).
     *
     * @param string   $vmId     VM UUID
     * @param int|null $vcpu     New vCPU count (null to keep current)
     * @param int|null $memoryMB New memory in MB (null to keep current)
     * @param int|null $diskGB   New disk size in GB (null to keep current)
     *
     * @return array Response data
     *
     * @throws RuntimeException On API error
     */
    public function resizeVM(string $vmId, ?int $vcpu = null, ?int $memoryMB = null, ?int $diskGB = null): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $params = [];
        if ($vcpu !== null) {
            $params['vcpu'] = $vcpu;
        }
        if ($memoryMB !== null) {
            $params['memory_mb'] = $memoryMB;
        }
        if ($diskGB !== null) {
            $params['disk_gb'] = $diskGB;
        }

        if (empty($params)) {
            throw new InvalidArgumentException('At least one resize parameter must be provided');
        }

        $response = $this->request('POST', "/provisioning/vms/{$vmId}/resize", $params);

        return $response['data'] ?? [];
    }

    /**
     * Get plan details by ID.
     *
     * @param string $planId Plan UUID
     *
     * @return array Plan details with vcpu, memory_mb, disk_gb
     *
     * @throws RuntimeException On API error
     */
    public function getPlan(string $planId): array
    {
        $this->validateUuid($planId, 'Plan ID');

        $response = $this->request('GET', "/provisioning/plans/{$planId}");

        return $response['data'] ?? [];
    }

    /**
     * Reset root password for a VM.
     *
     * @param string $vmId VM UUID
     *
     * @return array Response with new password
     *
     * @throws RuntimeException On API error
     */
    public function resetPassword(string $vmId): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $response = $this->request('POST', "/provisioning/vms/{$vmId}/password/reset");

        return $response['data'] ?? [];
    }

    /**
     * Set root password for a VM.
     *
     * @param string $vmId     VM UUID
     * @param string $password New password (8-128 characters)
     *
     * @return array Response data
     *
     * @throws RuntimeException On API error
     */
    public function setPassword(string $vmId, string $password): array
    {
        $this->validateUuid($vmId, 'VM ID');

        if (strlen($password) < 8 || strlen($password) > 128) {
            throw new InvalidArgumentException('Password must be between 8 and 128 characters');
        }

        $response = $this->request('POST', "/provisioning/vms/{$vmId}/password", [
            'password' => $password,
        ]);

        return $response['data'] ?? [];
    }

    /**
     * Get VM status.
     *
     * @param string $vmId VM UUID
     *
     * @return array Response with status and node_id
     *
     * @throws RuntimeException On API error
     */
    public function getVMStatus(string $vmId): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $response = $this->request('GET', "/provisioning/vms/{$vmId}/status");

        return $response['data'] ?? [];
    }

    /**
     * Get VM detailed information.
     *
     * @param string $vmId VM UUID
     *
     * @return array VM details
     *
     * @throws RuntimeException On API error
     */
    public function getVMInfo(string $vmId): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $response = $this->request('GET', "/provisioning/vms/{$vmId}");

        return $response['data'] ?? [];
    }

    /**
     * Get VM by WHMCS service ID.
     *
     * @param int $serviceId WHMCS service ID
     *
     * @return array VM details
     *
     * @throws RuntimeException On API error
     */
    public function getVMByServiceId(int $serviceId): array
    {
        if ($serviceId <= 0) {
            throw new InvalidArgumentException('Service ID must be a positive integer');
        }

        $response = $this->request('GET', "/provisioning/vms/by-service/{$serviceId}");

        return $response['data'] ?? [];
    }

    /**
     * Get async task status.
     *
     * @param string $taskId Task UUID
     *
     * @return array Task status data:
     *               - id (string): Task ID
     *               - type (string): Task type
     *               - status (string): pending|running|completed|failed|cancelled
     *               - progress (int): 0-100
     *               - message (string): Human-readable message
     *               - result (array, optional): Task result if completed
     *
     * @throws RuntimeException On API error
     */
    public function getTask(string $taskId): array
    {
        $this->validateUuid($taskId, 'Task ID');

        $response = $this->request('GET', "/provisioning/tasks/{$taskId}");

        return $response['data'] ?? [];
    }

    /**
     * Poll an async task until completion or failure.
     *
     * @param string $taskId   Task UUID
     * @param int    $maxPolls Maximum number of polling attempts
     *
     * @return array Final task result
     *
     * @throws RuntimeException On API error or task failure
     * @throws RuntimeException On timeout
     */
    public function pollTask(string $taskId, int $maxPolls = self::TASK_MAX_POLLS): array
    {
        $this->validateUuid($taskId, 'Task ID');

        $polls = 0;
        $lastProgress = -1;

        while ($polls < $maxPolls) {
            $task = $this->getTask($taskId);

            $status = $task['status'] ?? 'unknown';

            // Log progress changes
            $progress = $task['progress'] ?? 0;
            if ($progress !== $lastProgress) {
                $this->log('info', "Task {$taskId}: {$status} ({$progress}%) - " . ($task['message'] ?? ''));
                $lastProgress = $progress;
            }

            switch ($status) {
                case 'completed':
                    return $task;

                case 'failed':
                    $errorMessage = $task['message'] ?? 'Task failed without error message';
                    throw new RuntimeException("Task failed: {$errorMessage}");

                case 'cancelled':
                    throw new RuntimeException('Task was cancelled');

                case 'pending':
                case 'running':
                    // Continue polling
                    sleep(self::TASK_POLL_INTERVAL);
                    $polls++;
                    break;

                default:
                    throw new RuntimeException("Unknown task status: {$status}");
            }
        }

        throw new RuntimeException("Task polling timed out after {$maxPolls} attempts");
    }

    /**
     * Perform power operation on a VM.
     *
     * @param string $vmId      VM UUID
     * @param string $operation Operation: start, stop, or restart
     *
     * @return array Response data
     *
     * @throws RuntimeException On API error
     */
    public function powerOperation(string $vmId, string $operation): array
    {
        $this->validateUuid($vmId, 'VM ID');

        $operation = strtolower($operation);
        if (!in_array($operation, ['start', 'stop', 'restart'], true)) {
            throw new InvalidArgumentException('Operation must be start, stop, or restart');
        }

        $response = $this->request('POST', "/provisioning/vms/{$vmId}/power", [
            'operation' => $operation,
        ]);

        return $response['data'] ?? [];
    }

    /**
     * Make an HTTP request to the API.
     *
     * @param string $method HTTP method
     * @param string $path   API path (relative to base URL)
     * @param array  $data   Request body data (for POST/PATCH/PUT)
     *
     * @return array Decoded JSON response
     *
     * @throws RuntimeException On HTTP error or invalid response
     */
    private function request(string $method, string $path, array $data = []): array
    {
        $url = $this->apiUrl . $path;
        
        $this->log('debug', "API Request: {$method} {$url}");

        $ch = curl_init();
        if ($ch === false) {
            throw new RuntimeException('Failed to initialize cURL');
        }

        $headers = [
            'Content-Type: application/json',
            'Accept: application/json',
            'X-API-Key: ' . $this->apiKey,
            'User-Agent: ' . $this->userAgent,
        ];

        $options = [
            CURLOPT_URL            => $url,
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_TIMEOUT        => $this->timeout,
            CURLOPT_CONNECTTIMEOUT => 10,
            CURLOPT_HTTPHEADER     => $headers,
            CURLOPT_CUSTOMREQUEST  => $method,
            CURLOPT_SSL_VERIFYPEER => $this->verifySsl,
            CURLOPT_SSL_VERIFYHOST => $this->verifySsl ? 2 : 0,
            CURLOPT_FOLLOWLOCATION => false,
            CURLOPT_MAXREDIRS      => 0,
        ];

        if (!empty($data) && in_array($method, ['POST', 'PUT', 'PATCH'], true)) {
            $jsonBody = json_encode($data, JSON_THROW_ON_ERROR);
            $options[CURLOPT_POSTFIELDS] = $jsonBody;
        }

        curl_setopt_array($ch, $options);

        $response = curl_exec($ch);
        $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
        $error = curl_error($ch);
        $errno = curl_errno($ch);

        curl_close($ch);

        if ($errno !== 0) {
            throw new RuntimeException("cURL error ({$errno}): {$error}");
        }

        if (!is_string($response)) {
            throw new RuntimeException('Invalid response from API');
        }

        $this->log('debug', "API Response: HTTP {$httpCode}");

        // Decode JSON response
        try {
            $decoded = json_decode($response, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $e) {
            throw new RuntimeException('Failed to decode API response: ' . $e->getMessage());
        }

        // Handle error responses
        if ($httpCode >= 400) {
            $errorCode = $decoded['error']['code'] ?? 'UNKNOWN_ERROR';
            $errorMessage = $decoded['error']['message'] ?? "HTTP error {$httpCode}";
            $correlationId = $decoded['error']['correlation_id'] ?? '';

            $this->log('error', "API Error: {$errorCode} - {$errorMessage}" . 
                ($correlationId ? " (correlation_id: {$correlationId})" : ''));

            throw new RuntimeException("API error ({$errorCode}): {$errorMessage}");
        }

        return $decoded;
    }

    /**
     * Validate that all required keys are present in an array.
     *
     * @param array $data     Data array to validate
     * @param array $required Required keys
     *
     * @throws InvalidArgumentException If any required key is missing
     */
    private function validateRequired(array $data, array $required): void
    {
        $missing = [];
        foreach ($required as $key) {
            if (!isset($data[$key]) || $data[$key] === '') {
                $missing[] = $key;
            }
        }

        if (!empty($missing)) {
            throw new InvalidArgumentException('Missing required parameters: ' . implode(', ', $missing));
        }
    }

    /**
     * Validate that a string is a valid UUID.
     *
     * @param string $value Value to validate
     * @param string $name  Parameter name for error message
     *
     * @throws InvalidArgumentException If not a valid UUID
     */
    private function validateUuid(string $value, string $name): void
    {
        if (!preg_match('/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i', $value)) {
            throw new InvalidArgumentException("{$name} must be a valid UUID");
        }
    }

    /**
     * Log a message (integrates with WHMCS log module).
     *
     * @param string $level   Log level: debug, info, warning, error
     * @param string $message Log message
     */
    private function log(string $level, string $message): void
    {
        // Use WHMCS logModuleCall if available
        if (function_exists('logModuleCall')) {
            logModuleCall('virtuestack', $level, $message, '', '', '');
        }
    }
}