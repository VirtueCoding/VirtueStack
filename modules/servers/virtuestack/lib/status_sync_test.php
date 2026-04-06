<?php

declare(strict_types=1);

namespace VirtueStack\WHMCS {
    if (!defined('CURLOPT_URL')) {
        define('CURLOPT_URL', 10002);
    }
    if (!defined('CURLOPT_RETURNTRANSFER')) {
        define('CURLOPT_RETURNTRANSFER', 19913);
    }
    if (!defined('CURLOPT_TIMEOUT')) {
        define('CURLOPT_TIMEOUT', 13);
    }
    if (!defined('CURLOPT_CONNECTTIMEOUT')) {
        define('CURLOPT_CONNECTTIMEOUT', 78);
    }
    if (!defined('CURLOPT_HTTPHEADER')) {
        define('CURLOPT_HTTPHEADER', 10023);
    }
    if (!defined('CURLOPT_CUSTOMREQUEST')) {
        define('CURLOPT_CUSTOMREQUEST', 10036);
    }
    if (!defined('CURLOPT_SSL_VERIFYPEER')) {
        define('CURLOPT_SSL_VERIFYPEER', 64);
    }
    if (!defined('CURLOPT_SSL_VERIFYHOST')) {
        define('CURLOPT_SSL_VERIFYHOST', 81);
    }
    if (!defined('CURLOPT_FOLLOWLOCATION')) {
        define('CURLOPT_FOLLOWLOCATION', 52);
    }
    if (!defined('CURLOPT_MAXREDIRS')) {
        define('CURLOPT_MAXREDIRS', 68);
    }
    if (!defined('CURLOPT_POSTFIELDS')) {
        define('CURLOPT_POSTFIELDS', 10015);
    }
    if (!defined('CURLINFO_HTTP_CODE')) {
        define('CURLINFO_HTTP_CODE', 2097154);
    }

    final class CurlStubRegistry
    {
        /** @var array<string, array{body:string, http_code:int}> */
        public static array $responses = [];

        /** @var array<int, array<int, mixed>> */
        public static array $options = [];

        /** @var array<int, int> */
        public static array $httpCodes = [];

        public static function reset(): void
        {
            self::$responses = [];
            self::$options = [];
            self::$httpCodes = [];
        }
    }

    function curl_init(): object
    {
        return new \stdClass();
    }

    /**
     * @param array<int, mixed> $options
     */
    function curl_setopt_array(object $handle, array $options): bool
    {
        CurlStubRegistry::$options[spl_object_id($handle)] = $options;
        return true;
    }

    function curl_exec(object $handle): string|false
    {
        $options = CurlStubRegistry::$options[spl_object_id($handle)] ?? [];
        $method = (string) ($options[CURLOPT_CUSTOMREQUEST] ?? 'GET');
        $url = (string) ($options[CURLOPT_URL] ?? '');
        $response = CurlStubRegistry::$responses["{$method} {$url}"] ?? null;
        if ($response === null) {
            CurlStubRegistry::$httpCodes[spl_object_id($handle)] = 500;
            return false;
        }

        CurlStubRegistry::$httpCodes[spl_object_id($handle)] = $response['http_code'];
        return $response['body'];
    }

    function curl_getinfo(object $handle, int $option): mixed
    {
        if ($option !== CURLINFO_HTTP_CODE) {
            return null;
        }

        return CurlStubRegistry::$httpCodes[spl_object_id($handle)] ?? 0;
    }

    function curl_error(object $handle): string
    {
        $options = CurlStubRegistry::$options[spl_object_id($handle)] ?? [];
        $method = (string) ($options[CURLOPT_CUSTOMREQUEST] ?? 'GET');
        $url = (string) ($options[CURLOPT_URL] ?? '');
        if (isset(CurlStubRegistry::$responses["{$method} {$url}"])) {
            return '';
        }

        return 'missing stub response';
    }

    function curl_errno(object $handle): int
    {
        $options = CurlStubRegistry::$options[spl_object_id($handle)] ?? [];
        $method = (string) ($options[CURLOPT_CUSTOMREQUEST] ?? 'GET');
        $url = (string) ($options[CURLOPT_URL] ?? '');
        if (isset(CurlStubRegistry::$responses["{$method} {$url}"])) {
            return 0;
        }

        return 1;
    }

    function curl_close(object $handle): void
    {
        unset(
            CurlStubRegistry::$options[spl_object_id($handle)],
            CurlStubRegistry::$httpCodes[spl_object_id($handle)]
        );
    }
}

namespace WHMCS\Database {
    final class Capsule
    {
        /** @var array<string, list<array<string, mixed>>> */
        public static array $tables = [
            'tblcustomfields' => [],
            'tblcustomfieldsvalues' => [],
        ];

        public static function reset(): void
        {
            self::$tables = [
                'tblcustomfields' => [],
                'tblcustomfieldsvalues' => [],
            ];
        }

        public static function table(string $name): FakeQueryBuilder
        {
            return new FakeQueryBuilder($name);
        }
    }

    final class FakeQueryBuilder
    {
        /** @var array<string, mixed> */
        private array $conditions = [];

        public function __construct(private readonly string $table)
        {
        }

        public function where(string $column, mixed $value): self
        {
            $clone = clone $this;
            $clone->conditions[$column] = $value;
            return $clone;
        }

        public function value(string $column): mixed
        {
            foreach (Capsule::$tables[$this->table] as $row) {
                if ($this->matches($row)) {
                    return $row[$column] ?? null;
                }
            }

            return null;
        }

        public function exists(): bool
        {
            foreach (Capsule::$tables[$this->table] as $row) {
                if ($this->matches($row)) {
                    return true;
                }
            }

            return false;
        }

        /**
         * @param array<string, mixed> $values
         */
        public function update(array $values): int
        {
            $updated = 0;

            foreach (Capsule::$tables[$this->table] as &$row) {
                if (!$this->matches($row)) {
                    continue;
                }

                foreach ($values as $column => $value) {
                    $row[$column] = $value;
                }

                $updated++;
            }
            unset($row);

            return $updated;
        }

        /**
         * @param array<string, mixed> $values
         */
        public function insert(array $values): bool
        {
            Capsule::$tables[$this->table][] = $values;
            return true;
        }

        /**
         * @param array<string, mixed> $values
         */
        public function insertGetId(array $values): int
        {
            $nextID = count(Capsule::$tables[$this->table]) + 1;
            $values['id'] = $nextID;
            Capsule::$tables[$this->table][] = $values;
            return $nextID;
        }

        /**
         * @param array<string, mixed> $row
         */
        private function matches(array $row): bool
        {
            foreach ($this->conditions as $column => $value) {
                if (($row[$column] ?? null) !== $value) {
                    return false;
                }
            }

            return true;
        }
    }
}

namespace {
    use VirtueStack\WHMCS\ApiClient;
    use VirtueStack\WHMCS\CurlStubRegistry;
    use WHMCS\Database\Capsule;

    function logActivity(string $message): void
    {
        unset($message);
    }

    /**
     * @param array<string, mixed> $conditions
     */
    function get_query_val(string $table, string $column, array $conditions): mixed
    {
        foreach (Capsule::$tables[$table] as $row) {
            if (rowMatches($row, $conditions)) {
                return $row[$column] ?? null;
            }
        }

        return null;
    }

    /**
     * @param array<string, mixed> $row
     * @param array<string, mixed> $conditions
     */
    function rowMatches(array $row, array $conditions): bool
    {
        foreach ($conditions as $column => $value) {
            if (($row[$column] ?? null) !== $value) {
                return false;
            }
        }

        return true;
    }

    require_once __DIR__ . '/../virtuestack.php';

    /**
     * @param mixed $expected
     * @param mixed $actual
     */
    function assertSameValue(string $name, mixed $expected, mixed $actual): void
    {
        if ($expected === $actual) {
            return;
        }

        fwrite(STDERR, $name . PHP_EOL);
        fwrite(STDERR, 'Expected: ' . var_export($expected, true) . PHP_EOL);
        fwrite(STDERR, 'Actual:   ' . var_export($actual, true) . PHP_EOL);
        exit(1);
    }

    function seedServiceFields(int $serviceId, string $vmId): void
    {
        Capsule::reset();
        Capsule::$tables['tblcustomfields'] = [
            ['id' => 1, 'fieldname' => 'provisioning_status', 'type' => 'product'],
            ['id' => 2, 'fieldname' => 'vm_id', 'type' => 'product'],
            ['id' => 3, 'fieldname' => 'vm_ip', 'type' => 'product'],
            ['id' => 4, 'fieldname' => 'vm_status', 'type' => 'product'],
        ];
        Capsule::$tables['tblcustomfieldsvalues'] = [
            ['fieldid' => 1, 'relid' => $serviceId, 'value' => 'active'],
            ['fieldid' => 2, 'relid' => $serviceId, 'value' => $vmId],
            ['fieldid' => 3, 'relid' => $serviceId, 'value' => ''],
            ['fieldid' => 4, 'relid' => $serviceId, 'value' => ''],
        ];
    }

    /**
     * @param array<string, mixed> $payload
     */
    function queueJSONResponse(string $method, string $url, array $payload, int $httpCode = 200): void
    {
        CurlStubRegistry::$responses["{$method} {$url}"] = [
            'body' => json_encode($payload, JSON_THROW_ON_ERROR),
            'http_code' => $httpCode,
        ];
    }

    /**
     * @param array<int, array{name:string, runtime_status:string, expected_provisioning_status:string}> $cases
     */
    function assertSyncStatusCases(array $params, string $vmId, array $cases): void
    {
        foreach ($cases as $case) {
            CurlStubRegistry::reset();
            seedServiceFields((int) $params['serviceid'], $vmId);
            queueJSONResponse(
                'GET',
                "https://controller.example.test:443/api/v1/provisioning/vms/{$vmId}/status",
                ['data' => ['status' => $case['runtime_status']]]
            );

            assertSameValue($case['name'] . ' returns success', 'success', virtuestack_syncStatus($params));
            assertSameValue(
                $case['name'] . ' stores vm runtime status in vm_status',
                $case['runtime_status'],
                virtuestack_getServiceField((int) $params['serviceid'], 'vm_status')
            );
            assertSameValue(
                $case['name'] . ' keeps coherent provisioning status mapping',
                $case['expected_provisioning_status'],
                virtuestack_getServiceField((int) $params['serviceid'], 'provisioning_status')
            );
        }
    }

    /**
     * @param array<int, array{name:string, runtime_status:string, expected_provisioning_status:string}> $cases
     */
    function assertSyncServiceStateCases(ApiClient $client, int $serviceId, string $vmId, array $cases): void
    {
        foreach ($cases as $case) {
            CurlStubRegistry::reset();
            seedServiceFields($serviceId, $vmId);
            queueJSONResponse(
                'GET',
                "https://controller.example.test:443/api/v1/provisioning/vms/by-service/{$serviceId}",
                ['data' => ['id' => $vmId]]
            );
            queueJSONResponse(
                'GET',
                "https://controller.example.test:443/api/v1/provisioning/vms/{$vmId}",
                [
                    'data' => [
                        'id' => $vmId,
                        'status' => $case['runtime_status'],
                        'ip_addresses' => [
                            ['address' => '198.51.100.7'],
                        ],
                    ],
                ]
            );

            $vm = virtuestack_syncServiceState($client, $serviceId);

            assertSameValue(
                $case['name'] . ' returns the detailed vm payload',
                $case['runtime_status'],
                $vm['status'] ?? null
            );
            assertSameValue(
                $case['name'] . ' stores vm runtime status in vm_status',
                $case['runtime_status'],
                virtuestack_getServiceField($serviceId, 'vm_status')
            );
            assertSameValue(
                $case['name'] . ' keeps coherent provisioning status mapping',
                $case['expected_provisioning_status'],
                virtuestack_getServiceField($serviceId, 'provisioning_status')
            );
            assertSameValue(
                $case['name'] . ' stores the primary IP address',
                '198.51.100.7',
                virtuestack_getServiceField($serviceId, 'vm_ip')
            );
        }
    }

    $params = [
        'serviceid' => 501,
        'serverhostname' => 'controller.example.test',
        'serversecure' => 'on',
        'serverport' => 443,
        'serverpassword' => 'test-api-key',
    ];
    $vmId = '123e4567-e89b-12d3-a456-426614174000';

    assertSyncStatusCases($params, $vmId, [
        [
            'name' => 'syncStatus with stopped runtime state',
            'runtime_status' => 'stopped',
            'expected_provisioning_status' => 'active',
        ],
        [
            'name' => 'syncStatus with paused runtime state',
            'runtime_status' => 'paused',
            'expected_provisioning_status' => 'active',
        ],
        [
            'name' => 'syncStatus with shutting_down runtime state',
            'runtime_status' => 'shutting_down',
            'expected_provisioning_status' => 'active',
        ],
        [
            'name' => 'syncStatus with crashed runtime state',
            'runtime_status' => 'crashed',
            'expected_provisioning_status' => 'error',
        ],
    ]);

    $serviceId = 777;
    $client = new ApiClient('https://controller.example.test:443/api/v1', 'test-api-key');
    assertSyncServiceStateCases($client, $serviceId, $vmId, [
        [
            'name' => 'syncServiceState with running runtime state',
            'runtime_status' => 'running',
            'expected_provisioning_status' => 'active',
        ],
        [
            'name' => 'syncServiceState with paused runtime state',
            'runtime_status' => 'paused',
            'expected_provisioning_status' => 'active',
        ],
        [
            'name' => 'syncServiceState with shutting_down runtime state',
            'runtime_status' => 'shutting_down',
            'expected_provisioning_status' => 'active',
        ],
        [
            'name' => 'syncServiceState with crashed runtime state',
            'runtime_status' => 'crashed',
            'expected_provisioning_status' => 'error',
        ],
    ]);

    echo "ok\n";
}
