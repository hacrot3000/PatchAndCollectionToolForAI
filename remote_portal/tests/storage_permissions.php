<?php
declare(strict_types=1);

require __DIR__ . '/../src/Store.php';

use TaskDeck\RemotePortal\Store;

function storageAssert(bool $ok, string $message): void
{
    if (!$ok) {
        throw new RuntimeException('FAIL: ' . $message);
    }
    echo 'PASS: ' . $message . "\n";
}

function storageDenied(callable $check, string $message): void
{
    try {
        $check();
    } catch (Throwable $e) {
        echo 'PASS (denied): ' . $message . "\n";
        return;
    }
    throw new RuntimeException('FAIL (allowed): ' . $message);
}

$root = sys_get_temp_dir() . '/taskdeck-db-security-' . bin2hex(random_bytes(8));
if (!mkdir($root, 0700)) {
    throw new RuntimeException('Failed to create private test directory');
}
$database = $root . '/portal.sqlite3';
try {
    $store = new Store($database);
    $store->initialize();
    clearstatcache(true, $database);
    storageAssert(is_file($database) && (fileperms($database) & 0077) === 0,
        'SQLite database created owner-only before use');
    storageAssert((fileperms($root) & 0077) === 0,
        'SQLite journal parent is private');

    chmod($database, 0644);
    clearstatcache(true, $database);
    storageDenied(fn() => new Store($database),
        'existing world-readable Portal database rejected');
    chmod($database, 0600);
    clearstatcache(true, $database);
    $again = new Store($database);
    storageAssert($again instanceof Store, 'existing private SQLite DB opens safely');
    unset($again);

    $openDir = $root . '/exposed';
    mkdir($openDir, 0755);
    chmod($openDir, 0755);
    storageDenied(fn() => new Store($openDir . '/db.sqlite3'),
        'shared-host public-readable DB directory rejected');
    storageAssert(!file_exists($openDir . '/db.sqlite3'),
        'invalid directory cannot create database');
    rmdir($openDir);

    $link = $root . '/linked.sqlite3';
    if (function_exists('symlink') && @symlink($database, $link)) {
        storageDenied(fn() => new Store($link), 'SQLite database symlink rejected');
        unlink($link);
    }
    $dirLink = $root . '/linked-directory';
    if (function_exists('symlink') && @symlink($root, $dirLink)) {
        storageDenied(fn() => new Store($dirLink . '/other.sqlite3'),
            'SQLite database parent symlink rejected');
        unlink($dirLink);
    }
    echo "All Portal SQLite privacy tests passed.\n";
} finally {
    unset($store);
    @unlink($database);
    @unlink($root.'/portal-abuse-hmac.key');
    @rmdir($root);
}
