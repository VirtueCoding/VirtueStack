<?php

declare(strict_types=1);

namespace WHMCS\Database {
    final class Capsule
    {
        /** @var array<string, list<array<string, mixed>>> */
        public static array $tables = [
            'tblcustomfields' => [],
            'tblcustomfieldsvalues' => [],
        ];
    }
}

namespace {
    use WHMCS\Database\Capsule;

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

    require_once __DIR__ . '/shared_functions.php';
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

    function seedServiceFields(int $serviceId, string $provisioningStatus, string $taskId, string $vmStatus = ''): void
    {
        Capsule::$tables['tblcustomfields'] = [
            ['id' => 1, 'fieldname' => 'provisioning_status', 'type' => 'product'],
            ['id' => 2, 'fieldname' => 'task_id', 'type' => 'product'],
            ['id' => 3, 'fieldname' => 'vm_id', 'type' => 'product'],
            ['id' => 4, 'fieldname' => 'vm_ip', 'type' => 'product'],
            ['id' => 5, 'fieldname' => 'vm_status', 'type' => 'product'],
        ];
        Capsule::$tables['tblcustomfieldsvalues'] = [
            ['fieldid' => 1, 'relid' => $serviceId, 'value' => $provisioningStatus],
            ['fieldid' => 2, 'relid' => $serviceId, 'value' => $taskId],
            ['fieldid' => 3, 'relid' => $serviceId, 'value' => ''],
            ['fieldid' => 4, 'relid' => $serviceId, 'value' => ''],
            ['fieldid' => 5, 'relid' => $serviceId, 'value' => $vmStatus],
        ];
    }

    $serviceId = 321;
    $taskId = '123e4567-e89b-12d3-a456-426614174000';

    seedServiceFields($serviceId, 'pending', $taskId);
    $params = [
        'serviceid' => $serviceId,
        'userid' => 42,
        'serverhostname' => 'controller.example.test',
        'serversecure' => 'on',
        'serverport' => 443,
        'serverpassword' => 'test-api-key',
    ];

    $pendingResult = virtuestack_ClientArea($params);
    assertSameValue('pending status renders provisioning template', 'provisioning', $pendingResult['vars']['status'] ?? null);
    assertSameValue('pending status exposes task id', $taskId, $pendingResult['vars']['task_id'] ?? null);

    seedServiceFields($serviceId, 'resizing', $taskId);
    $resizingResult = virtuestack_ClientArea($params);
    assertSameValue('resizing status stays on async overview template instead of falling through', 'resizing', $resizingResult['vars']['status'] ?? null);
    assertSameValue('resizing status keeps task id available for polling UI', $taskId, $resizingResult['vars']['task_id'] ?? null);

    seedServiceFields($serviceId, 'active', '', 'stopped');
    $stoppedResult = virtuestack_ClientArea($params);
    assertSameValue(
        'active provisioning status still renders stopped runtime state from vm_status',
        'stopped',
        $stoppedResult['vars']['status'] ?? null
    );

    $template = file_get_contents(__DIR__ . '/../templates/overview.tpl');
    assertSameValue(
        'overview template includes resizing copy for customer-facing async state',
        true,
        is_string($template) && str_contains($template, 'being resized')
    );

    echo "ok\n";
}
