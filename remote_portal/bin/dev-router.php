<?php
declare(strict_types=1);
// Use with: REMOTE_PORTAL_DEV_HTTP=1 php -S 127.0.0.1:8088 -t public bin/dev-router.php
$path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
// Only audited, public static assets bypass the application router.
// Keep everything else (including PHP entrypoints, key filenames,
// path traversals and unknown files) on the explicit index.php deny path.
if (in_array($path, [
    '/style.css', '/remote_crypto.js', '/remote_probe.js', '/remote_login.js',
], true)) {
    return false;
}
require __DIR__ . '/../public/index.php';
