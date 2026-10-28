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
    $ticket = $signer->issue(22, $device, 'viewer', 'https://portal.example.com', $now);
    $parts = explode('.', $ticket);
    assertTicket(count($parts) === 2, 'ticket has two signed components');
    $claims = json_decode(decodeUrl($parts[0]), true, 10, JSON_THROW_ON_ERROR);
    assertTicket($claims['sub'] === '22' && $claims['aud'] === 'taskdeck-relay'
        && $claims['device_id'] === $device && $claims['role'] === 'viewer', 'ticket binds audience, user, role, device');
    assertTicket($claims['iat'] === $now && $claims['exp'] === $now + 30
        && preg_match('/^[a-f0-9]{32}$/D', $claims['jti']) === 1, 'short-lived random ticket');
    assertTicket(sodium_crypto_sign_verify_detached(decodeUrl($parts[1]),
        'taskdeck-relay-ticket-v1.' . $parts[0], $public), 'Ed25519 ticket signature verifies');
    forbiddenTicket(fn() => TicketSigner::generateSeed($key), 'key seed overwrite denied');
    forbiddenTicket(fn() => $signer->issue(22, $device, 'root', 'https://portal.example.com'), 'invalid role denied');
    forbiddenTicket(fn() => $signer->issue(22, 'invalid', 'viewer', 'https://portal.example.com'), 'invalid device denied');
    forbiddenTicket(fn() => $signer->issue(22, $device, 'viewer', 'http://portal.example.com'), 'non-HTTPS issuer denied');
    $wrong = $parts[0] . 'x';
    assertTicket(!sodium_crypto_sign_verify_detached(decodeUrl($parts[1]),
        'taskdeck-relay-ticket-v1.' . $wrong, $public), 'signature breaks after payload tampering');
    echo "All signing tests passed.\n";
} finally {
    @unlink($key); @rmdir($dir);
}
