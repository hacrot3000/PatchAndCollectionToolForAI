<?php
declare(strict_types=1);
namespace TaskDeck\RemotePortal;

use RuntimeException;

// Control-plane authorization ticket only. This is NOT a TaskDeck login token.
// Relay verifies the signature; TaskDeck still enforces its own authentication.
final class TicketSigner
{
    private string $secretKey;
    private string $publicKey;

    public function __construct(string $seedPath)
    {
        self::assertPrivatePath($seedPath);
        if (!is_file($seedPath) || is_link($seedPath) || (fileperms($seedPath) & 0077) !== 0) {
            throw new RuntimeException('Portal signing seed must be an owner-only regular file');
        }
        $seed = file_get_contents($seedPath);
        if ($seed === false || strlen($seed) !== SODIUM_CRYPTO_SIGN_SEEDBYTES) {
            throw new RuntimeException('Portal signing seed must be exactly 32 bytes');
        }
        $keypair = sodium_crypto_sign_seed_keypair($seed);
        $this->secretKey = sodium_crypto_sign_secretkey($keypair);
        $this->publicKey = sodium_crypto_sign_publickey($keypair);
        sodium_memzero($seed);
    }

    public static function generateSeed(string $seedPath): void
    {
        self::assertPrivatePath($seedPath);
        if (file_exists($seedPath) || is_link($seedPath)) {
            throw new RuntimeException('Signing seed already exists; refusing overwrite');
        }
        $seed = random_bytes(SODIUM_CRYPTO_SIGN_SEEDBYTES);
        // Avoid a momentary world-readable secret before chmod executes.
        $previousMask = umask(0077);
        try {
            $handle = @fopen($seedPath, 'xb');
        } finally {
            umask($previousMask);
        }
        if (!$handle) {
            throw new RuntimeException('Cannot create signing seed; check private directory');
        }
        @chmod($seedPath, 0600);
        try {
            if (fwrite($handle, $seed) !== strlen($seed) || !fflush($handle)) {
                throw new RuntimeException('Could not write signing seed');
            }
        } catch (\Throwable $e) {
            fclose($handle);
            @unlink($seedPath);
            throw $e;
        }
        fclose($handle);
        sodium_memzero($seed);
    }

    public function publicKeyBase64(): string
    {
        return base64_encode($this->publicKey);
    }

    /**
     * Validate the browser's key encoding without using the private signing
     * seed or consuming a rate-limit reservation. Go Relay/Agent additionally
     * verify that the uncompressed point belongs to the P-256 curve.
     */
    public static function browserBinding(string $browserNonce, string $browserPublicKey): string
    {
        if (preg_match('/^[a-f0-9]{32}$/D', $browserNonce) !== 1) {
            throw new RuntimeException('Invalid browser challenge nonce');
        }
        $public = base64_decode($browserPublicKey, true);
        if ($public === false || strlen($public) !== 65 || ord($public[0]) !== 4
            || base64_encode($public) !== $browserPublicKey) {
            throw new RuntimeException('Invalid browser P-256 public key encoding');
        }
        return hash('sha256', "taskdeck-browser-ticket-v2\0" . $browserNonce . "\0" . $public);
    }

    public function issue(int $userId, string $deviceId, string $role, string $issuer,
        string $browserNonce, string $browserPublicKey, ?int $now = null): string
    {
        if ($userId < 1 || !preg_match('/^[a-f0-9]{64}$/D', $deviceId)
            || !in_array($role, ['viewer', 'operator', 'admin'], true)
            || !preg_match('#^https://[a-z0-9][a-z0-9.-]+(?::[0-9]{2,5})?$#iD', $issuer)) {
            throw new RuntimeException('Invalid ticket claims');
        }
        // The browser chooses its nonce and ephemeral P-256 public key
        // BEFORE requesting a ticket. Portal signs their binding so Relay
        // cannot substitute an attacker-controlled ECDH key or nonce.
        $binding = self::browserBinding($browserNonce, $browserPublicKey);
        $now ??= time();
        $claims = [
            'v' => 2,
            'browser_binding' => $binding,
            'iss' => $issuer,
            'aud' => 'taskdeck-relay',
            'sub' => (string) $userId,
            'device_id' => $deviceId,
            'role' => $role,
            'iat' => $now,
            'exp' => $now + 30,
            'jti' => bin2hex(random_bytes(16)),
        ];
        $body = self::base64url(json_encode($claims, JSON_THROW_ON_ERROR));
        $signature = sodium_crypto_sign_detached('taskdeck-relay-ticket-v1.' . $body, $this->secretKey);
        return $body . '.' . self::base64url($signature);
    }

    public function issueDeviceLease(string $deviceId, string $issuer, ?int $now = null): string
    {
        if (!preg_match('/^[a-f0-9]{64}$/D', $deviceId)
            || !preg_match('#^https://[a-z0-9][a-z0-9.-]+(?::[0-9]{2,5})?$#iD', $issuer)) {
            throw new RuntimeException('Invalid device lease claims');
        }
        $now ??= time();
        $claims = [
            'v' => 1, 'iss' => $issuer, 'aud' => 'taskdeck-relay-device',
            'device_id' => $deviceId, 'iat' => $now, 'exp' => $now + 70,
        ];
        $body = self::base64url(json_encode($claims, JSON_THROW_ON_ERROR));
        $signature = sodium_crypto_sign_detached(
            'taskdeck-relay-device-lease-v1.' . $body, $this->secretKey);
        return $body . '.' . self::base64url($signature);
    }

    private static function base64url(string $data): string
    {
        return rtrim(strtr(base64_encode($data), '+/', '-_'), '=');
    }

    private static function assertPrivatePath(string $path): void
    {
        if ($path === '' || $path[0] !== '/' || str_contains($path, "\0")
            || realpath(dirname($path)) === false) {
            throw new RuntimeException('Signing seed needs an absolute path in a private existing directory');
        }
        $pathDir = dirname($path);
        if (is_link($pathDir)) {
            throw new RuntimeException('Signing directory must not be a symlink');
        }
        $dir = realpath($pathDir);
        if ($dir === false || (fileperms($dir) & 0077) !== 0) {
            throw new RuntimeException('Signing directory must have owner-only permissions');
        }
        $public = realpath(__DIR__ . '/../public');
        if ($dir === $public || str_starts_with($dir . '/', $public . '/')) {
            throw new RuntimeException('Signing seed cannot be inside public webroot');
        }
    }
}
