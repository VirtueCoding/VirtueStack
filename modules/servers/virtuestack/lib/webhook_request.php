<?php

declare(strict_types=1);

/**
 * Parse the request Content-Length header if it is a valid non-negative integer.
 */
function virtuestack_getDeclaredContentLength(): ?int
{
    $header = $_SERVER['CONTENT_LENGTH'] ?? '';
    if (!is_string($header) || $header === '' || !ctype_digit($header)) {
        return null;
    }

    return (int) $header;
}

/**
 * Read the webhook request body while enforcing the configured size limit.
 *
 * The declared Content-Length is rejected before opening the request stream
 * when it already exceeds the limit. If the header is missing or inaccurate,
 * fall back to bounded chunked reads and stop as soon as the limit is crossed.
 *
 * @return array{status:int, body:string|null, error:string|null}
 */
function virtuestack_readWebhookBody(
    int $maxRequestSize,
    ?int $declaredContentLength = null,
    string $streamPath = 'php://input',
    int $chunkSize = 8192
): array {
    if ($declaredContentLength !== null && $declaredContentLength > $maxRequestSize) {
        return [
            'status' => 413,
            'body' => null,
            'error' => 'Request body too large',
        ];
    }

    $stream = fopen($streamPath, 'rb');
    if ($stream === false) {
        return [
            'status' => 400,
            'body' => null,
            'error' => 'Unable to read request body',
        ];
    }

    $body = '';
    while (!feof($stream)) {
        $remaining = ($maxRequestSize + 1) - strlen($body);
        if ($remaining <= 0) {
            break;
        }

        $readLength = min($chunkSize, $remaining);
        $chunk = fread($stream, $readLength);
        if ($chunk === false) {
            fclose($stream);

            return [
                'status' => 400,
                'body' => null,
                'error' => 'Unable to read request body',
            ];
        }

        $body .= $chunk;
        if (strlen($body) > $maxRequestSize) {
            fclose($stream);

            return [
                'status' => 413,
                'body' => null,
                'error' => 'Request body too large',
            ];
        }
    }

    fclose($stream);

    if ($body === '') {
        return [
            'status' => 400,
            'body' => null,
            'error' => 'Empty request body',
        ];
    }

    return [
        'status' => 200,
        'body' => $body,
        'error' => null,
    ];
}
