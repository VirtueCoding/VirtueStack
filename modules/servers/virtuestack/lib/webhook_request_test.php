<?php

declare(strict_types=1);

require_once __DIR__ . '/webhook_request.php';

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

final class VirtueStackWebhookRequestTestStream
{
    /** @var resource|null */
    public $context;

    private int $position = 0;

    private static string $payload = '';
    private static int $openCount = 0;
    private static int $readCount = 0;
    private static int $bytesServed = 0;

    public static function register(string $payload): string
    {
        self::$payload = $payload;
        self::$openCount = 0;
        self::$readCount = 0;
        self::$bytesServed = 0;

        if (in_array('virtuestack-test', stream_get_wrappers(), true)) {
            stream_wrapper_unregister('virtuestack-test');
        }

        stream_wrapper_register('virtuestack-test', self::class);

        return 'virtuestack-test://payload';
    }

    /**
     * @return array{open_count:int, read_count:int, bytes_served:int}
     */
    public static function stats(): array
    {
        return [
            'open_count' => self::$openCount,
            'read_count' => self::$readCount,
            'bytes_served' => self::$bytesServed,
        ];
    }

    public function stream_open(string $path, string $mode, int $options, ?string &$openedPath): bool
    {
        unset($path, $mode, $options, $openedPath);

        $this->position = 0;
        self::$openCount++;

        return true;
    }

    public function stream_read(int $count): string
    {
        self::$readCount++;

        $chunk = substr(self::$payload, $this->position, $count);
        $this->position += strlen($chunk);
        self::$bytesServed += strlen($chunk);

        return $chunk;
    }

    public function stream_eof(): bool
    {
        return $this->position >= strlen(self::$payload);
    }

    /**
     * @return array<int, int>
     */
    public function stream_stat(): array
    {
        return [];
    }
}

$maxRequestSize = 1024;
$oversizedPath = VirtueStackWebhookRequestTestStream::register(str_repeat('a', $maxRequestSize + 128));
$oversizedResult = virtuestack_readWebhookBody($maxRequestSize, $maxRequestSize + 1, $oversizedPath, 128);

assertSameValue('declared oversize request is rejected', 413, $oversizedResult['status']);
assertSameValue('declared oversize request returns error', 'Request body too large', $oversizedResult['error']);
assertSameValue('declared oversize request does not open input stream', 0, VirtueStackWebhookRequestTestStream::stats()['open_count']);
assertSameValue('declared oversize request does not read input stream', 0, VirtueStackWebhookRequestTestStream::stats()['read_count']);

$fallbackPath = VirtueStackWebhookRequestTestStream::register(str_repeat('b', $maxRequestSize + 512));
$fallbackResult = virtuestack_readWebhookBody($maxRequestSize, null, $fallbackPath, 128);
$fallbackStats = VirtueStackWebhookRequestTestStream::stats();

assertSameValue('fallback oversize request is rejected', 413, $fallbackResult['status']);
assertSameValue('fallback oversize request returns error', 'Request body too large', $fallbackResult['error']);
assertSameValue('fallback oversize request opens input stream once', 1, $fallbackStats['open_count']);
assertSameValue('fallback oversize request attempts bounded reads', true, $fallbackStats['read_count'] > 0);

$validPath = VirtueStackWebhookRequestTestStream::register('{"event":"webhook.test"}');
$validResult = virtuestack_readWebhookBody($maxRequestSize, 24, $validPath, 128);

assertSameValue('valid request status stays successful', 200, $validResult['status']);
assertSameValue('valid request body is returned', '{"event":"webhook.test"}', $validResult['body']);
assertSameValue('valid request has no read error', null, $validResult['error']);

stream_wrapper_unregister('virtuestack-test');

echo "ok\n";
