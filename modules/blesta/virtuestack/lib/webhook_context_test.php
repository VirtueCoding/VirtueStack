<?php

declare(strict_types=1);

require_once __DIR__ . '/VirtueStackHelper.php';

/**
 * @param string $name
 * @param mixed  $expected
 * @param mixed  $actual
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

$vmId = '123e4567-e89b-12d3-a456-426614174011';
$taskId = '123e4567-e89b-12d3-a456-426614174012';

$controllerPayload = [
    'event' => 'vm.created',
    'timestamp' => '2026-04-05T00:00:00Z',
    'idempotency_key' => 'idem-456',
    'data' => [
        'external_service_id' => 88,
        'vm_id' => $vmId,
        'task_id' => $taskId,
        'ip_addresses' => [
            ['address' => '203.0.113.55'],
        ],
    ],
];

assertSameValue(
    'blesta helper extracts controller envelope identifiers from nested data',
    [
        'event' => 'vm.created',
        'external_service_id' => 88,
        'vm_id' => $vmId,
        'task_id' => $taskId,
        'data' => [
            'external_service_id' => 88,
            'vm_id' => $vmId,
            'task_id' => $taskId,
            'ip_addresses' => [
                ['address' => '203.0.113.55'],
            ],
        ],
        'error_message' => '',
    ],
    VirtueStackHelper::extractWebhookContext($controllerPayload)
);

assertSameValue(
    'blesta helper preserves legacy top-level identifier compatibility',
    [
        'event' => 'backup.failed',
        'external_service_id' => 91,
        'vm_id' => '',
        'task_id' => '',
        'data' => [
            'message' => 'backup failed',
        ],
        'error_message' => 'backup failed',
    ],
    VirtueStackHelper::extractWebhookContext([
        'event' => 'backup.failed',
        'external_service_id' => 91,
        'data' => ['message' => 'backup failed'],
    ])
);

echo "ok\n";
