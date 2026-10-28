<?php
declare(strict_types=1);

// Bootstrap is strictly CLI-only. Never expose this script through webroot.
if (PHP_SAPI !== 'cli') {
    http_response_code(403);
    exit;
}
require __DIR__ . '/../src/Store.php';
require __DIR__ . '/../src/TicketSigner.php';

use TaskDeck\RemotePortal\Store;
use TaskDeck\RemotePortal\TicketSigner;

$path = getenv('REMOTE_PORTAL_DB_PATH') ?: '';
if ($path === '') {
    fwrite(STDERR, "Set REMOTE_PORTAL_DB_PATH to an absolute path outside public/.\n");
    exit(2);
}
try {
    $db = new Store($path);
    $command = $argv[1] ?? '';
    if ($command === 'init') {
        $db->initialize();
        echo "Portal schema initialized.\n";
    } elseif ($command === 'create-admin') {
        $username = $argv[2] ?? '';
        if ($username === '') {
            throw new RuntimeException('Usage: php bin/admin.php create-admin <username>; enter password on STDIN');
        }
        fwrite(STDERR, "Enter admin password on STDIN (not in shell arguments): ");
        $password = rtrim((string) fgets(STDIN), "\r\n");
        $id = $db->createUser($username, $password, true);
        echo "Admin created (id=$id).\n";
    } elseif ($command === 'init-ticket-key') {
        $seed = getenv('REMOTE_PORTAL_SIGNING_SEED_PATH') ?: '';
        TicketSigner::generateSeed($seed);
        echo "Created Portal Ed25519 ticket signing seed in private storage.\n";
    } elseif ($command === 'relay-public-key') {
        $seed = getenv('REMOTE_PORTAL_SIGNING_SEED_PATH') ?: '';
        $signer = new TicketSigner($seed);
        echo $signer->publicKeyBase64() . "\n";
    } else {
        throw new RuntimeException('Usage: php bin/admin.php init | create-admin <username> | init-ticket-key | relay-public-key');
    }
} catch (Throwable $e) {
    fwrite(STDERR, "Error: " . $e->getMessage() . "\n");
    exit(1);
}
