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

$tests = [
    [
        'name' => 'completion from pending clears task and activates service',
        'actual' => virtuestack_getAsyncTaskCompletionUpdates('pending'),
        'expected' => [
            'task_id' => '',
            'provisioning_error' => '',
            'provisioning_status' => 'active',
        ],
    ],
    [
        'name' => 'completion from resizing clears task and returns service to active',
        'actual' => virtuestack_getAsyncTaskCompletionUpdates('resizing'),
        'expected' => [
            'task_id' => '',
            'provisioning_error' => '',
            'provisioning_status' => 'active',
        ],
    ],
    [
        'name' => 'failure from pending clears task and marks service error',
        'actual' => virtuestack_getAsyncTaskFailureUpdates('pending', 'boom'),
        'expected' => [
            'task_id' => '',
            'provisioning_error' => 'boom',
            'provisioning_status' => 'error',
        ],
    ],
    [
        'name' => 'failure from resizing clears task but keeps provisioned service active',
        'actual' => virtuestack_getAsyncTaskFailureUpdates('resizing', 'boom'),
        'expected' => [
            'task_id' => '',
            'provisioning_error' => 'boom',
            'provisioning_status' => 'active',
        ],
    ],
];

foreach ($tests as $test) {
    assertSameValue($test['name'], $test['expected'], $test['actual']);
}

assertSameValue(
    'polling statuses include resizing fallback',
    ['pending', 'resizing'],
    virtuestack_getAsyncPollableProvisioningStatuses()
);

assertSameValue(
    'resizing is accepted as a provisioning status',
    'resizing',
    validateFieldValue('provisioning_status', 'resizing')
);

assertSameValue(
    'paused is accepted as a vm runtime status',
    'paused',
    validateFieldValue('vm_status', 'paused')
);

assertSameValue(
    'shutting_down is accepted as a vm runtime status',
    'shutting_down',
    validateFieldValue('vm_status', 'shutting_down')
);

assertSameValue(
    'crashed is accepted as a vm runtime status',
    'crashed',
    validateFieldValue('vm_status', 'crashed')
);

assertSameValue(
    'only vm.create completions send welcome email',
    true,
    virtuestack_shouldSendProvisioningWelcomeEmail('vm.create')
);

assertSameValue(
    'resize completion does not send welcome email',
    false,
    virtuestack_shouldSendProvisioningWelcomeEmail('vm.resize')
);

assertSameValue(
    'running VM state maps to active provisioning state',
    'active',
    virtuestack_mapVMStatusToProvisioningStatus('running')
);

assertSameValue(
    'provisioning VM state maps to pending provisioning state',
    'pending',
    virtuestack_mapVMStatusToProvisioningStatus('provisioning')
);

assertSameValue(
    'deleted VM state maps to terminated provisioning state',
    'terminated',
    virtuestack_mapVMStatusToProvisioningStatus('deleted')
);

assertSameValue(
    'paused VM state maps to active provisioning state',
    'active',
    virtuestack_mapVMStatusToProvisioningStatus('paused')
);

assertSameValue(
    'shutting_down VM state maps to active provisioning state',
    'active',
    virtuestack_mapVMStatusToProvisioningStatus('shutting_down')
);

assertSameValue(
    'crashed VM state maps to error provisioning state',
    'error',
    virtuestack_mapVMStatusToProvisioningStatus('crashed')
);

echo "ok\n";
