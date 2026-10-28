<?php
declare(strict_types=1);
require __DIR__ . '/../src/TicketSigner.php';
use TaskDeck\RemotePortal\TicketSigner;

function assertTicket(bool $ok, string $name): void {
    if (!$ok) throw new RuntimeException("FAIL: " . $name);
    echo "PASS: " . $name . "\n";
}
function forbiddenTicket(callable $fn, string $name): void {
    try { $fn(); } catch (Throwable $e) { echo "PASS (denied): " . $name . "\n"; return; }
    throw new RuntimeException("FAIL (allowed): " . $name);
}
function decodeUrl(string $v): string {
    $d = base64_decode(strtr($v, '-_', '+/'), true);
    if ($d === false) throw new RuntimeException('Invalid base64url test vector');
    return $d;
}
if (!extension_loaded('sodium')) throw new RuntimeException('sodium extension required');

$dir = sys_get_temp_dir() . '/taskdeck-ticket-test-' . bin2hex(random_bytes(8));
mkdir($dir, 0700);
$key = $dir . '/relay-key.seed';
try {
    TicketSigner::generateSeed($key);
    $signer = new TicketSigner($key);
    $public = base64_decode($signer->publicKeyBase64(), true);
    assertTicket(strlen($public) === SODIUM_CRYPTO_SIGN_PUBLICKEYBYTES, '32-byte relay verification key');
    $device = str_repeat('a', 64);
    $now = time();
    // Standard uncompressed NIST P-256 generator point (well-formed key).
    $p256 = hex2bin('04'
        . '6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296'
        . '4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5');
    $publicEncoded = base64_encode($p256);
    $nonce = str_repeat('a', 32);
    $ticket = $signer->issue(22, $device, 'viewer', 'https://portal.example.com',
        $nonce, $publicEncoded, $now);
    $parts = explode('.', $ticket);
    assertTicket(count($parts) === 2, 'ticket has two signed components');
    $claims = json_decode(decodeUrl($parts[0]), true, 10, JSON_THROW_ON_ERROR);
    assertTicket($claims['sub'] === '22' && $claims['aud'] === 'taskdeck-relay'
        && $claims['device_id'] === $device && $claims['role'] === 'viewer'
        && $claims['v'] === 2
        && $claims['browser_binding'] === hash('sha256',
            "taskdeck-browser-ticket-v2\0" . $nonce . "\0" . $p256), 'ticket binds audience, user, role, device');
    assertTicket($claims['browser_binding'] ===
        '2a85833552e8544f7231ff2f66cccd38559d668b046128fb5dd8022a9b84b31b',
        'PHP P-256 nonce/key binding matches Go cross-language test vector');
    assertTicket($claims['iat'] === $now && $claims['exp'] === $now + 30
        && preg_match('/^[a-f0-9]{32}$/D', $claims['jti']) === 1, 'short-lived random ticket');
    assertTicket(sodium_crypto_sign_verify_detached(decodeUrl($parts[1]),
        'taskdeck-relay-ticket-v1.' . $parts[0], $public), 'Ed25519 ticket signature verifies');
    $lease = $signer->issueDeviceLease($device, 'https://portal.example.com', $now);
    $leaseParts = explode('.', $lease);
    assertTicket(count($leaseParts) === 2, 'device lease has two signed components');
    $leaseClaims = json_decode(decodeUrl($leaseParts[0]), true, 10, JSON_THROW_ON_ERROR);
    assertTicket($leaseClaims['device_id'] === $device &&
        $leaseClaims['aud'] === 'taskdeck-relay-device' &&
        $leaseClaims['exp'] === $now + 70 && $leaseClaims['iat'] === $now,
        'device lease binds identity and lifetime');
    assertTicket(sodium_crypto_sign_verify_detached(decodeUrl($leaseParts[1]),
        'taskdeck-relay-device-lease-v1.' . $leaseParts[0], $public),
        'device lease signature verifies');
    forbiddenTicket(fn() => $signer->issueDeviceLease('invalid', 'https://portal.example.com'),
        'invalid lease Device ID denied');
    forbiddenTicket(fn() => TicketSigner::generateSeed($key), 'key seed overwrite denied');
    forbiddenTicket(fn() => $signer->issue(22, $device, 'root', 'https://portal.example.com', $nonce, $publicEncoded), 'invalid role denied');
    forbiddenTicket(fn() => $signer->issue(22, 'invalid', 'viewer', 'https://portal.example.com', $nonce, $publicEncoded), 'invalid device denied');
    forbiddenTicket(fn() => $signer->issue(22, $device, 'viewer', 'http://portal.example.com', $nonce, $publicEncoded), 'non-HTTPS issuer denied');
    forbiddenTicket(fn() => $signer->issue(22, $device, 'viewer',
        'https://portal.example.com', 'bad', $publicEncoded),
        'malformed browser nonce rejected');
    forbiddenTicket(fn() => $signer->issue(22, $device, 'viewer',
        'https://portal.example.com', $nonce, base64_encode('not a P-256 key')),
        'malformed browser key rejected');
    $different = $signer->issue(22, $device, 'viewer', 'https://portal.example.com',
        str_repeat('b', 32), $publicEncoded, $now);
    $differentClaims = json_decode(decodeUrl(explode('.', $different)[0]), true, 10, JSON_THROW_ON_ERROR);
    assertTicket($differentClaims['browser_binding'] !== $claims['browser_binding'],
        'changing browser nonce changes signed Portal binding');
    $wrong = $parts[0] . 'x';
    assertTicket(!sodium_crypto_sign_verify_detached(decodeUrl($parts[1]),
        'taskdeck-relay-ticket-v1.' . $wrong, $public), 'signature breaks after payload tampering');
    echo "All signing tests passed.\n";
} finally {
    @unlink($key); @rmdir($dir);
}
