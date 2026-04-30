<?php
/**
 * VirtueStack API Client for Blesta
 *
 * HTTP client for communicating with the VirtueStack Provisioning REST API.
 * All requests are authenticated via X-API-Key header over HTTPS.
 *
 * @package blesta
 * @subpackage blesta.components.modules.virtuestack
 */
class VirtueStackApiClient
{
    private string $apiUrl;
    private string $apiKey;
    private int $timeout;

    private const USER_AGENT = 'VirtueStack-Blesta/1.0';
    private const MAX_RESPONSE_SIZE = 1048576; // 1MB
    private const VALID_POWER_ACTIONS = ['start', 'stop', 'restart'];

    /**
     * @param string $apiUrl Base API URL (must be HTTPS)
     * @param string $apiKey Provisioning API key
     * @param int $timeout Request timeout in seconds
     * @throws InvalidArgumentException If URL is not HTTPS or API key is empty
     */
    public function __construct(string $apiUrl, string $apiKey, int $timeout = 30)
    {
        $apiUrl = rtrim($apiUrl, '/');

        if (strpos($apiUrl, 'https://') !== 0) {
            throw new InvalidArgumentException(
                'API URL must use HTTPS. Received: ' . $apiUrl
            );
        }

        if (empty($apiKey)) {
            throw new InvalidArgumentException('API key must not be empty.');
        }

        $this->apiUrl = $apiUrl;
        $this->apiKey = $apiKey;
        $this->timeout = max(5, min(60, $timeout));
    }

    /**
     * Perform an HTTP request to the VirtueStack API.
     *
     * @param string $method HTTP method
     * @param string $path API path (e.g., /provisioning/vms)
     * @param array $params Request parameters
     * @param string|null $idempotencyKey Idempotency key for POST requests
     * @return array Decoded response data
     * @throws RuntimeException On HTTP or API errors
     */
    private function request(
        string $method,
        string $path,
        array $params = [],
        ?string $idempotencyKey = null
    ): array {
        $url = $this->apiUrl . $path;

        $headers = [
            'Accept: application/json',
            'X-API-Key: ' . $this->apiKey,
            'User-Agent: ' . self::USER_AGENT,
        ];

        if ($idempotencyKey !== null) {
            $headers[] = 'Idempotency-Key: ' . $idempotencyKey;
        }

        $ch = curl_init();

        curl_setopt_array($ch, [
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_TIMEOUT => $this->timeout,
            CURLOPT_CONNECTTIMEOUT => 10,
            CURLOPT_SSL_VERIFYPEER => true,
            CURLOPT_SSL_VERIFYHOST => 2,
            CURLOPT_FOLLOWLOCATION => false,
            CURLOPT_MAXREDIRS => 0,
        ]);

        $method = strtoupper($method);

        switch ($method) {
            case 'GET':
                if (!empty($params)) {
                    $url .= '?' . http_build_query($params);
                }
                break;
            case 'POST':
                curl_setopt($ch, CURLOPT_POST, true);
                if (!empty($params)) {
                    $headers[] = 'Content-Type: application/json';
                    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($params));
                }
                break;
            case 'PUT':
                curl_setopt($ch, CURLOPT_CUSTOMREQUEST, 'PUT');
                if (!empty($params)) {
                    $headers[] = 'Content-Type: application/json';
                    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($params));
                }
                break;
            case 'DELETE':
                curl_setopt($ch, CURLOPT_CUSTOMREQUEST, 'DELETE');
                if (!empty($params)) {
                    $headers[] = 'Content-Type: application/json';
                    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($params));
                }
                break;
            default:
                curl_close($ch);
                throw new InvalidArgumentException(
                    'Unsupported HTTP method: ' . $method
                );
        }

        curl_setopt($ch, CURLOPT_URL, $url);
        curl_setopt($ch, CURLOPT_HTTPHEADER, $headers);

        $responseBody = '';
        $responseSize = 0;
        curl_setopt($ch, CURLOPT_WRITEFUNCTION, function ($ch, $data) use (&$responseBody, &$responseSize) {
            $responseSize += strlen($data);
            if ($responseSize > self::MAX_RESPONSE_SIZE) {
                return 0;
            }
            $responseBody .= $data;
            return strlen($data);
        });

        curl_exec($ch);

        $curlError = curl_error($ch);
        $httpCode = (int) curl_getinfo($ch, CURLINFO_HTTP_CODE);

        curl_close($ch);

        if (!empty($curlError)) {
            throw new RuntimeException(
                'VirtueStack API connection error: ' . $curlError
            );
        }

        if ($responseSize > self::MAX_RESPONSE_SIZE) {
            throw new RuntimeException(
                'VirtueStack API response exceeded maximum size of 1MB.'
            );
        }

        $decoded = json_decode($responseBody, true);
        if ($decoded === null && json_last_error() !== JSON_ERROR_NONE) {
            throw new RuntimeException(
                'VirtueStack API returned invalid JSON (HTTP ' . $httpCode . '): '
                . json_last_error_msg()
            );
        }

        if ($httpCode >= 400) {
            $errorCode = $decoded['error']['code'] ?? 'UNKNOWN_ERROR';
            $errorMessage = $decoded['error']['message'] ?? 'Unknown error';
            throw new RuntimeException(
                'VirtueStack API error (HTTP ' . $httpCode . '): '
                . $errorCode . ' - ' . $errorMessage,
                $httpCode
            );
        }

        return $decoded['data'] ?? $decoded;
    }

    /**
     * Check API connectivity.
     *
     * @return array Health status
     */
    public function healthCheck(): array
    {
        return $this->request('GET', '/health');
    }

    /**
     * Create a new virtual machine (async).
     *
     * @param array $params VM creation parameters
     * @return array Response with task_id and vm_id
     * @throws InvalidArgumentException If required fields are missing
     */
    public function createVM(array $params): array
    {
        $required = [
            'customer_id',
            'plan_id',
            'template_id',
            'hostname',
            'external_service_id',
        ];

        foreach ($required as $field) {
            if (empty($params[$field])) {
                throw new InvalidArgumentException(
                    "Required field '{$field}' is missing for VM creation."
                );
            }
        }

        $idempotencyKey = 'blesta-service-' . $params['external_service_id'];

        return $this->request('POST', '/provisioning/vms', $params, $idempotencyKey);
    }

    /**
     * Delete a virtual machine.
     *
     * @param string $vmId VM UUID
     * @return array Response data
     */
    public function deleteVM(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request('DELETE', '/provisioning/vms/' . urlencode($vmId) . $this->ownershipQuery($serviceId, $clientId));
    }

    /**
     * Suspend a virtual machine.
     *
     * @param string $vmId VM UUID
     * @return array Response data
     */
    public function suspendVM(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request('POST', '/provisioning/vms/' . urlencode($vmId) . '/suspend' . $this->ownershipQuery($serviceId, $clientId));
    }

    /**
     * Unsuspend a virtual machine.
     *
     * @param string $vmId VM UUID
     * @return array Response data
     */
    public function unsuspendVM(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request('POST', '/provisioning/vms/' . urlencode($vmId) . '/unsuspend' . $this->ownershipQuery($serviceId, $clientId));
    }

    /**
     * Resize a virtual machine to a new plan.
     *
     * @param string $vmId VM UUID
     * @param string $planId New plan UUID
     * @return array Response data
     */
    public function resizeVM(string $vmId, int $serviceId, int $clientId, string $planId): array
    {
        return $this->request(
            'POST',
            '/provisioning/vms/' . urlencode($vmId) . '/resize' . $this->ownershipQuery($serviceId, $clientId),
            ['plan_id' => $planId]
        );
    }

    /**
     * Get VM information.
     *
     * @param string $vmId VM UUID
     * @return array VM details
     */
    public function getVMInfo(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request('GET', '/provisioning/vms/' . urlencode($vmId), $this->ownershipParams($serviceId, $clientId));
    }

    /**
     * Get VM power status.
     *
     * @param string $vmId VM UUID
     * @return array Status data
     */
    public function getVMStatus(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request('GET', '/provisioning/vms/' . urlencode($vmId) . '/status', $this->ownershipParams($serviceId, $clientId));
    }

    /**
     * Get VM resource usage (bandwidth, disk).
     *
     * @param string $vmId VM UUID
     * @return array Usage data
     */
    public function getVMUsage(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request('GET', '/provisioning/vms/' . urlencode($vmId) . '/usage', $this->ownershipParams($serviceId, $clientId));
    }

    /**
     * Look up VM by billing service ID.
     *
     * @param int $serviceId Blesta service ID
     * @param int $clientId Blesta client ID used as an ownership assertion
     * @return array VM data
     */
    public function getVMByServiceId(int $serviceId, int $clientId): array
    {
        if ($serviceId <= 0) {
            throw new InvalidArgumentException('Service ID must be a positive integer');
        }
        if ($clientId <= 0) {
            throw new InvalidArgumentException('Client ID must be a positive integer');
        }

        return $this->request(
            'GET',
            '/provisioning/vms/by-service/' . urlencode((string) $serviceId),
            ['external_client_id' => $clientId]
        );
    }

    /**
     * Perform a power operation on a VM.
     *
     * @param string $vmId VM UUID
     * @param string $action Power action (start, stop, restart)
     * @return array Response data
     * @throws InvalidArgumentException If action is invalid
     */
    public function powerOperation(string $vmId, int $serviceId, int $clientId, string $action): array
    {
        if (!in_array($action, self::VALID_POWER_ACTIONS, true)) {
            throw new InvalidArgumentException(
                'Invalid power action: ' . $action
                . '. Must be one of: ' . implode(', ', self::VALID_POWER_ACTIONS)
            );
        }

        return $this->request(
            'POST',
            '/provisioning/vms/' . urlencode($vmId) . '/power' . $this->ownershipQuery($serviceId, $clientId),
            ['operation' => $action]
        );
    }

    /**
     * Reset VM root password (auto-generated).
     *
     * @param string $vmId VM UUID
     * @return array Response with new password
     */
    public function resetPassword(string $vmId, int $serviceId, int $clientId): array
    {
        return $this->request(
            'POST',
            '/provisioning/vms/' . urlencode($vmId) . '/password/reset' . $this->ownershipQuery($serviceId, $clientId)
        );
    }

    /**
     * Set VM root password to a specific value.
     *
     * @param string $vmId VM UUID
     * @param string $password New password
     * @return array Response data
     */
    public function setPassword(string $vmId, int $serviceId, int $clientId, string $password): array
    {
        return $this->request(
            'POST',
            '/provisioning/vms/' . urlencode($vmId) . '/password' . $this->ownershipQuery($serviceId, $clientId),
            ['password' => $password]
        );
    }

    /**
     * Get async task status.
     *
     * @param string $taskId Task UUID
     * @return array Task status data
     */
    public function getTask(string $taskId): array
    {
        return $this->request('GET', '/provisioning/tasks/' . urlencode($taskId));
    }

    /**
     * Create an SSO token for customer portal access.
     *
     * @param int $serviceId Blesta service ID
     * @param string $vmId VM UUID
     * @return array Response with token
     */
    public function createSSOToken(int $serviceId, int $clientId, string $vmId): array
    {
        if ($serviceId <= 0) {
            throw new InvalidArgumentException('Service ID must be a positive integer');
        }
        if ($clientId <= 0) {
            throw new InvalidArgumentException('Client ID must be a positive integer');
        }
        if (trim($vmId) === '') {
            throw new InvalidArgumentException('VM ID must not be empty');
        }
        $params = ['external_service_id' => $serviceId, 'external_client_id' => $clientId, 'vm_id' => $vmId];

        return $this->request('POST', '/provisioning/sso-tokens', $params);
    }

    /**
     * Create or retrieve a customer in VirtueStack.
     *
     * @param string $email Customer email
     * @param string $name Customer name
     * @param int $clientId Blesta client ID
     * @return array Customer data with id
     */
    public function createCustomer(string $email, string $name, int $clientId): array
    {
        return $this->request('POST', '/provisioning/customers', [
            'email' => $email,
            'name' => $name,
            'external_client_id' => $clientId,
            'billing_provider' => 'blesta',
        ]);
    }

    private function ownershipParams(int $serviceId, int $clientId): array
    {
        if ($serviceId <= 0) {
            throw new InvalidArgumentException('Service ID must be a positive integer');
        }
        if ($clientId <= 0) {
            throw new InvalidArgumentException('Client ID must be a positive integer');
        }
        return [
            'external_service_id' => $serviceId,
            'external_client_id' => $clientId,
        ];
    }

    private function ownershipQuery(int $serviceId, int $clientId): string
    {
        return '?' . http_build_query($this->ownershipParams($serviceId, $clientId));
    }

    /**
     * List available plans.
     *
     * @return array List of plans
     */
    public function listPlans(): array
    {
        return $this->request('GET', '/provisioning/plans');
    }
}
