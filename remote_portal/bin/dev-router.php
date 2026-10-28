<?php
declare(strict_types=1);
// Use with: REMOTE_PORTAL_DEV_HTTP=1 php -S 127.0.0.1:8088 -t public bin/dev-router.php
$path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
if ($path === '/style.css') {
    return false;
}
require __DIR__ . '/../public/index.php';
