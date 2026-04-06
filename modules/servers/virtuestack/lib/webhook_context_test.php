<?php

declare(strict_types=1);

require_once __DIR__ . '/shared_functions.php';

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

$vmId = '123e4567-e89b-12d3-a456-426614174001';
$taskId = '123e4567-e89b-12d3-a456-426614174002';

$controllerPayload = [
    'event' => 'vm.started',
    'timestamp' => '2026-04-05T00:00:00Z',
    'idempotency_key' => 'idem-123',
    'data' => [
        'vm_id' => $vmId,
        'external_service_id' => 42,
    ],
];

$controllerContext = virtuestack_extractWebhookContext($controllerPayload);

assertSameValue(
    'controller envelope extracts nested identifiers without requiring task_id',
    [
        'event' => 'vm.started',
        'task_id' => '',
        'vm_id' => $vmId,
        'external_service_id' => 42,
        'timestamp' => '2026-04-05T00:00:00Z',
        'idempotency_key' => 'idem-123',
        'event_data' => [
            'vm_id' => $vmId,
            'external_service_id' => 42,
        ],
    ],
    $controllerContext
);

assertSameValue(
    'controller envelope without task_id validates successfully for supported event',
    null,
    virtuestack_validateWebhookContext($controllerContext, ['vm.started', 'webhook.test'])
);

$legacyPayload = [
    'event' => 'vm.created',
    'task_id' => $taskId,
    'vm_id' => $vmId,
    'external_service_id' => 77,
    'result' => [
        'ip_addresses' => [
            ['address' => '203.0.113.10'],
        ],
    ],
];

$legacyContext = virtuestack_extractWebhookContext($legacyPayload);

assertSameValue(
    'legacy flat payload keeps result fallback for backwards compatibility',
    [
        'event' => 'vm.created',
        'task_id' => $taskId,
        'vm_id' => $vmId,
        'external_service_id' => 77,
        'timestamp' => '',
        'idempotency_key' => '',
        'event_data' => [
            'ip_addresses' => [
                ['address' => '203.0.113.10'],
            ],
        ],
    ],
    $legacyContext
);

assertSameValue(
    'webhook test event validates without identifiers',
    null,
    virtuestack_validateWebhookContext(
        virtuestack_extractWebhookContext([
            'event' => 'webhook.test',
            'data' => ['message' => 'ping'],
        ]),
        ['vm.started', 'webhook.test']
    )
);

echo "ok\n";
