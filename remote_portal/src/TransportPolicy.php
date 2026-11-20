<?php
declare(strict_types=1);

namespace TaskDeck\RemotePortal;

/**
 * The only permitted HTTP plaintext exception is the loopback PHP built-in
 * development server. Shared-host FPM/Apache can be reached over loopback
 * from a reverse proxy, so REMOTE_ADDR=127.0.0.1 and spoofable Host alone
 * MUST NEVER disable HTTPS for real Portal accounts/TOTP/session cookies.
 */
final class TransportPolicy
{
    public static function permitsDevelopmentHttp(
        string $sapi, bool $operatorEnabled, string $peer, string $host
    ): bool {
        return $sapi === 'cli-server'
            && $operatorEnabled
            && in_array($peer, ['127.0.0.1','::1'], true)
            && in_array(strtolower($host),
                ['localhost:8088','127.0.0.1:8088'], true);
    }
}
