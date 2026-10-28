<?php
declare(strict_types=1);

require __DIR__ . '/../src/Store.php';
require_once __DIR__ . '/../src/Totp.php';
require __DIR__ . '/../src/TicketSigner.php';

use TaskDeck\RemotePortal\Store;
use TaskDeck\RemotePortal\Totp;
use TaskDeck\RemotePortal\TicketSigner;

function h(mixed $v): string
{
    return htmlspecialchars((string) $v, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
}

function fail(int $status, string $message): never
{
    http_response_code($status);
    header('Content-Type: text/plain; charset=UTF-8');
    echo $message;
    exit;
}

function redirectHome(): never
{
    header('Location: /', true, 303);
    exit;
}

function csrf(): string
{
    if (!isset($_SESSION['csrf']) || !is_string($_SESSION['csrf'])) {
        $_SESSION['csrf'] = bin2hex(random_bytes(32));
    }
    return $_SESSION['csrf'];
}

function requireCsrf(): void
{
    if (!hash_equals(csrf(), (string) ($_POST['csrf'] ?? ''))) {
        fail(403, 'Invalid CSRF token');
    }
}

function formCsrf(): string
{
    return '<input type="hidden" name="csrf" value="' . h(csrf()) . '">';
}

$localHttp = getenv('REMOTE_PORTAL_DEV_HTTP') === '1'
    && in_array($_SERVER['REMOTE_ADDR'] ?? '', ['127.0.0.1', '::1'], true)
    && in_array(strtolower((string) ($_SERVER['HTTP_HOST'] ?? '')), ['localhost:8088','127.0.0.1:8088'], true);
$secure = !empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off';
if (!$secure && !$localHttp) {
    fail(403, 'HTTPS required');
}

// An explicit operator opt-in enables read-only browser reachability diagnostics.
// Never derive a WebSocket destination from a device name or request parameter.
$relayOrigin = getenv('REMOTE_PORTAL_RELAY_ORIGIN') ?: '';
$probeAvailable = getenv('REMOTE_PORTAL_EXPERIMENTAL_TICKETS') === '1'
    && (getenv('REMOTE_PORTAL_SIGNING_SEED_PATH') ?: '') !== ''
    && (getenv('REMOTE_PORTAL_ISSUER') ?: '') !== ''
    && preg_match('#^https://[a-z0-9][a-z0-9.-]+(?::[0-9]{2,5})?$#iD', $relayOrigin) === 1;
$loginProbeAvailable = $probeAvailable &&
    getenv('REMOTE_PORTAL_EXPERIMENTAL_LOGIN') === '1';
$connectOrigins = "'self'";
if ($probeAvailable) {
    $connectOrigins .= ' ' . substr_replace($relayOrigin, 'wss://', 0, 8);
}

header('Cache-Control: no-store, max-age=0');
header('X-Content-Type-Options: nosniff');
header('Referrer-Policy: no-referrer');
header('X-Frame-Options: DENY');
// This browser performs key agreement and may hold transient encrypted
// credentials in memory. Block hostile opener access, cross-origin
// resource reuse and browser APIs not needed for the Portal.
header('Cross-Origin-Opener-Policy: same-origin');
header('Cross-Origin-Resource-Policy: same-origin');
header('Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()');
header('X-Permitted-Cross-Domain-Policies: none');
$scriptPolicy = $probeAvailable ? "script-src 'self'" : "script-src 'none'";
header("Content-Security-Policy: default-src 'none'; style-src 'self'; " . $scriptPolicy .
    "; connect-src " . $connectOrigins . "; form-action 'self'; base-uri 'none'; frame-ancestors 'none'");
if ($secure) {
    header('Strict-Transport-Security: max-age=31536000');
}
ini_set('session.use_strict_mode', '1');
ini_set('session.use_only_cookies', '1');
ini_set('session.gc_maxlifetime', '3600');
session_name('taskdeck_remote_portal');
session_set_cookie_params([
    'lifetime' => 0, 'path' => '/', 'secure' => $secure, 'httponly' => true, 'samesite' => 'Strict',
]);
// Agent API does not open PHP browser sessions or generate session cookies.
// This prevents a heartbeat from creating a new session file on each request.
try {
    $db = new Store(getenv('REMOTE_PORTAL_DB_PATH') ?: '');
} catch (Throwable $e) {
    error_log('Portal initialization failed (check configuration)');
    fail(503, 'Portal unavailable');
}

$path = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH) ?: '/';
$method = $_SERVER['REQUEST_METHOD'] ?? 'GET';

if (str_starts_with($path, '/api/agent/')) {
    if ($method !== 'POST' || !in_array($path, ['/api/agent/enroll', '/api/agent/heartbeat', '/api/agent/grant-check'], true)) {
        fail(404, 'Not found');
    }
    // Reserve a durable per-source / site-wide slot before JSON decoding
    // and Ed25519 verification. An attacker needs no device key to burn CPU
    // on these paths, so the paired Device ID quota alone is insufficient.
    // Do not trust unverified forwarded headers for source identity.
    try {
        // Reserve the correct pre-auth budget for *every* Agent path,
        // including enroll, before JSON parsing and signature validation.
        $scope = $path === '/api/agent/enroll' ? 'enroll' : 'agent';
        $admitted = $db->reserveIngress($scope, (string) ($_SERVER['REMOTE_ADDR'] ?? ''));
    } catch (Throwable $e) {
        error_log('Portal Agent API ingress limiter unavailable');
        fail(503, 'Portal unavailable');
    }
    if (!$admitted) {
        header('Retry-After: 60');
        fail(429, 'Too many requests');
    }
    header('Content-Type: application/json; charset=UTF-8');
    if (!str_starts_with(strtolower($_SERVER['CONTENT_TYPE'] ?? ''), 'application/json')
        || (int) ($_SERVER['CONTENT_LENGTH'] ?? 0) > 4096) {
        fail(415, 'JSON request required (max 4096 bytes)');
    }
    $raw = file_get_contents('php://input', false, null, 0, 4097);
    if ($raw === false || strlen($raw) > 4096) {
        fail(413, 'Request too large');
    }
    $p = json_decode($raw, true);
    if (!is_array($p) || array_is_list($p)) {
        fail(400, 'Invalid JSON');
    }
    try {
        if ($path === '/api/agent/enroll') {
            $db->enroll($p);
            echo '{"ok":true}';
        } elseif ($path === '/api/agent/heartbeat') {
            $db->heartbeat($p);
            $result = ['ok' => true];
            $seed = getenv('REMOTE_PORTAL_SIGNING_SEED_PATH') ?: '';
            $issuer = getenv('REMOTE_PORTAL_ISSUER') ?: '';
            if ($seed !== '' && $issuer !== '') {
                $signer = new TicketSigner($seed);
                $result['device_lease'] = $signer->issueDeviceLease($p['device_id'], $issuer);
            }
            echo json_encode($result, JSON_THROW_ON_ERROR);
        } else {
            // This endpoint accepts ONLY a signed device proof, never a
            // browser cookie, Relay request or unverified Portal role.
            // A previously signed 30s ticket cannot override live revocation.
            $allowed = $db->checkCurrentAgentGrant($p);
            echo json_encode(['ok' => true,'allowed' => $allowed], JSON_THROW_ON_ERROR);
        }
    } catch (Throwable $e) {
        // Do not disclose whether a Device ID exists or is paired.
        http_response_code(403);
        echo '{"ok":false}';
    }
    exit;
}

// Reject invalid paths/methods BEFORE PHP creates a browser session file.
// Even an invalid CSRF POST otherwise invokes session_start() and can
// allocate unbounded filesystem state on shared hosting.
$browserPosts = [
    '/login','/logout','/account/password','/account/mfa/start',
    '/account/mfa/confirm','/api/remote/ticket','/users/create',
    '/devices/claim','/devices/remove','/grants/set','/grants/revoke',
];
if (($method === 'GET' && $path !== '/')
    || ($method === 'POST' && !in_array($path,$browserPosts,true))
    || !in_array($method,['GET','POST'],true)) {
    fail(404,'Not found');
}
if ($method === 'POST' && (int)($_SERVER['CONTENT_LENGTH'] ?? 0) > 8192) {
    fail(413,'Form request too large');
}
// One aggregate quota bounds anonymous GET and forged POST session-file
// creation, across multiple PHP workers. Independent tighter login quota
// applies before loading CSRF/session state.
try {
    $admitted = $db->reserveIngress('browser',(string)($_SERVER['REMOTE_ADDR'] ?? ''));
    if (!$admitted) {
        header('Retry-After: 60');
        fail(429,'Too many browser requests');
    }
    if ($method === 'POST' && $path === '/login') {
        $admitted = $db->reserveIngress('login',(string)($_SERVER['REMOTE_ADDR'] ?? ''));
        if (!$admitted) {
            header('Retry-After: 60');
            fail(429,'Too many login attempts');
        }
    }
} catch (Throwable $e) {
    error_log('Portal pre-session ingress limiter unavailable');
    fail(503,'Portal unavailable');
}

// Do not use shared-host PHP defaults (/tmp, shared session handlers,
// group-readable session folders). Pending TOTP seed and browser auth
// state must remain inside the same owner-only private Portal storage.
try {
    $privateSessions = $db->privateBrowserSessionDirectory();
    if (session_module_name('files') === false
        || session_module_name() !== 'files'
        || session_save_path($privateSessions) === false
        || session_save_path() !== $privateSessions
        || !session_start()) {
        throw new RuntimeException('Secure Portal session initialization failed');
    }
} catch (Throwable $e) {
    error_log('Portal private browser session storage unavailable');
    fail(503, 'Portal unavailable');
}

$user = null;
if (isset($_SESSION['user_id']) && is_int($_SESSION['user_id'])
    && (int) ($_SESSION['last_seen'] ?? 0) + 1800 > time()) {
    $user = $db->userById($_SESSION['user_id']);
}
if ($user === null || !isset($_SESSION['auth_epoch'])
    || (int) $_SESSION['auth_epoch'] !== (int) ($user['auth_epoch'] ?? 0)) {
    $user = null;
    unset($_SESSION['user_id'], $_SESSION['last_seen'], $_SESSION['auth_epoch']);
} else {
    $_SESSION['last_seen'] = time();
}
// A password-only session is allowed solely to enroll a first TOTP factor
// when the operator has enabled mandatory MFA; this is enforced on POST
// endpoints as well as by hiding any remote device data from GET responses.
// The opt-out is ONLY for legacy non-admin bootstrap. Administrative
// credentials must never bypass TOTP: a password-only admin could otherwise
// change device grants or delete pairings without a second factor.
$needsMfa = $user !== null
    && (getenv('REMOTE_PORTAL_REQUIRE_MFA') !== '0'
        || (int)$user['is_admin'] === 1)
    && (int)$user['mfa_enabled'] !== 1;
if ($user === null || (int)($user['mfa_enabled'] ?? 0) === 1) {
    unset($_SESSION['mfa_pending_secret'], $_SESSION['mfa_pending_until']);
}

if ($method === 'POST') {
    requireCsrf();
    if ($path === '/login') {
        // Login quota was reserved before session_start(); no file
        // allocation occurs for over-quota anonymous requests.
        $username = (string) ($_POST['username'] ?? '');
        $password = (string) ($_POST['password'] ?? '');
        $totpCode = (string) ($_POST['totp_code'] ?? '');
        try {
            $auth = $db->authenticate($username, $password, $totpCode);
        } catch (Throwable $e) {
            // Fail closed on SQLite contention, filesystem failures and
            // verifier exceptions. Never print DB paths or account existence.
            error_log('Portal login verifier unavailable');
            fail(503, 'Portal unavailable');
        }
        if ($auth === null) {
            $_SESSION['flash'] = 'Đăng nhập không thành công.';
        } else {
            session_regenerate_id(true);
            $_SESSION['user_id'] = (int) $auth['id'];
            $_SESSION['auth_epoch'] = (int) $auth['auth_epoch'];
            $_SESSION['last_seen'] = time();
            $_SESSION['csrf'] = bin2hex(random_bytes(32));
        }
        redirectHome();
    }
    if ($path === '/logout') {
        $_SESSION = [];
        session_regenerate_id(true);
        redirectHome();
    }
    if (!$user) {
        fail(401, 'Login required');
    }
    if ($path === '/account/mfa/start') {
        if ((int)$user['mfa_enabled'] === 1) {
            fail(403, 'MFA is already enabled');
        }
        try {
            $password = (string) ($_POST['current_password'] ?? '');
            if (!$db->verifyPasswordForMfaEnrollment((int)$user['id'], $password)) {
                throw new RuntimeException('Invalid credentials');
            }
            $_SESSION['mfa_pending_secret'] = Totp::newSecret();
            $_SESSION['mfa_pending_until'] = time() + 300;
            $_SESSION['flash'] = 'Đã tạo mã MFA. Thêm vào ứng dụng Authenticator rồi nhập mã xác nhận.';
        } catch (Throwable $e) {
            unset($_SESSION['mfa_pending_secret'], $_SESSION['mfa_pending_until']);
            $_SESSION['flash'] = 'Không thể bắt đầu MFA. Kiểm tra mật khẩu hoặc thử lại sau.';
        }
        redirectHome();
    }
    if ($path === '/account/mfa/confirm') {
        if ((int)$user['mfa_enabled'] === 1) {
            fail(403, 'MFA is already enabled');
        }
        try {
            $secret = $_SESSION['mfa_pending_secret'] ?? null;
            if (!is_string($secret) || (int)($_SESSION['mfa_pending_until'] ?? 0) < time()) {
                throw new RuntimeException('MFA setup expired');
            }
            $epoch = $db->activateTotp((int)$user['id'], $secret,
                (string)($_POST['totp_code'] ?? ''));
            unset($_SESSION['mfa_pending_secret'], $_SESSION['mfa_pending_until']);
            session_regenerate_id(true);
            $_SESSION['auth_epoch'] = $epoch;
            $_SESSION['last_seen'] = time();
            $_SESSION['csrf'] = bin2hex(random_bytes(32));
            $_SESSION['flash'] = 'Đã bật MFA; từ bây giờ mỗi lần đăng nhập cần mã Authenticator.';
        } catch (Throwable $e) {
            $_SESSION['flash'] = 'Mã xác thực chưa đúng, đã hết hạn hoặc vượt quá giới hạn thử.';
        }
        redirectHome();
    }
    if ($needsMfa) {
        fail(403, 'MFA enrollment required before accessing Portal services');
    }
    if ($path === '/account/password') {
        $oldPassword = (string) ($_POST['current_password'] ?? '');
        $newPassword = (string) ($_POST['new_password'] ?? '');
        $confirm = (string) ($_POST['confirm_password'] ?? '');
        if (!hash_equals($newPassword, $confirm)) {
            $_SESSION['flash'] = 'Mật khẩu mới và xác nhận không khớp.';
            redirectHome();
        }
        try {
            $epoch = $db->changePassword((int) $user['id'], $oldPassword, $newPassword,
                (string)($_POST['totp_code'] ?? ''));
            session_regenerate_id(true);
            $_SESSION['auth_epoch'] = $epoch;
            $_SESSION['csrf'] = bin2hex(random_bytes(32));
            $_SESSION['flash'] = 'Đã đổi mật khẩu Portal. Các phiên đăng nhập khác đã bị vô hiệu.';
        } catch (Throwable $e) {
            $_SESSION['flash'] = 'Không thể đổi mật khẩu. Kiểm tra mật khẩu hiện tại và mật khẩu mới (tối thiểu 14 ký tự).';
        }
        redirectHome();
    }
    if ($path === '/api/remote/ticket') {
        // Remote E2EE rendezvous has a stronger requirement than ordinary
        // Portal bootstrap. Even REMOTE_PORTAL_REQUIRE_MFA=0 (legacy recovery)
        // must never permit password-only accounts to mint Relay tickets.
        if ((int)($user['mfa_enabled'] ?? 0) !== 1) {
            fail(403, 'Portal MFA enrollment required for remote connection tickets');
        }
        // Feature gate remains OFF until MFA, revocation and relay/session
        // checks are complete. A ticket is NOT TaskDeck authorization.
        if (getenv('REMOTE_PORTAL_EXPERIMENTAL_TICKETS') !== '1') {
            fail(503, 'Remote session issuance is not enabled');
        }
        $deviceId = (string) ($_POST['device_id'] ?? '');
        $clientNonce = (string) ($_POST['client_nonce'] ?? '');
        $browserKey = (string) ($_POST['browser_key'] ?? '');
        // Reject malformed browser proof before consuming the shared
        // device quota. The real P-256 curve check is repeated by Relay.
        try {
            TicketSigner::browserBinding($clientNonce, $browserKey);
        } catch (RuntimeException $e) {
            fail(400, 'Invalid browser key or nonce');
        }
        // Atomic authorization + durable limits before signing:
        // 12 tickets/min per Portal user; 24/min per device (all users).
        // No raw IP address, username or browser key is stored in SQLite.
        try {
            $role = $db->reserveRemoteTicket((int) $user['id'], $deviceId);
        } catch (RuntimeException $e) {
            if ($e->getMessage() === 'Remote ticket quota exceeded') {
                header('Retry-After: 60');
                fail(429, 'Too many remote ticket requests');
            }
            if ($e->getMessage() === 'Remote ticket denied') {
                fail(403, 'Device not available for this user');
            }
            error_log('Portal ticket reservation failed');
            fail(503, 'Ticket issuer unavailable');
        } catch (Throwable $e) {
            error_log('Portal ticket reservation database failure');
            fail(503, 'Ticket issuer unavailable');
        }
        try {
            $signer = new TicketSigner(getenv('REMOTE_PORTAL_SIGNING_SEED_PATH') ?: '');
            $issuer = getenv('REMOTE_PORTAL_ISSUER') ?: '';
            $ticket = $signer->issue((int) $user['id'], $deviceId, $role, $issuer,
                $clientNonce, $browserKey, null, (int) $user['auth_epoch']);
            header('Content-Type: application/json; charset=UTF-8');
            echo json_encode(['ticket' => $ticket, 'expires_in_seconds' => 30],
                JSON_THROW_ON_ERROR);
        } catch (Throwable $e) {
            error_log('Portal ticket issuer unavailable');
            fail(503, 'Ticket issuer unavailable');
        }
        exit;
    }
    if (!(bool) $user['is_admin']) {
        fail(403, 'Admin required');
    }
    // Only known privileged state changes consume the durable per-admin
    // limit. Recheck current admin status atomically in Store; a stale
    // authenticated browser session cannot bypass a revoked admin role.
    if (!in_array($path, ['/users/create', '/devices/claim',
        '/grants/set', '/grants/revoke', '/devices/remove'], true)) {
        fail(404, 'Not found');
    }
    try {
        $db->reserveAdminMutation((int) $user['id']);
    } catch (RuntimeException $e) {
        if ($e->getMessage() === 'Admin mutation quota exceeded') {
            header('Retry-After: 60');
            fail(429, 'Too many administration requests');
        }
        fail(403, 'Admin request denied');
    } catch (Throwable $e) {
        error_log('Portal admin mutation limiter unavailable');
        fail(503, 'Portal unavailable');
    }
    try {
        if ($path === '/users/create') {
            $db->createUser((string) ($_POST['username'] ?? ''), (string) ($_POST['password'] ?? ''), false);
            $_SESSION['flash'] = 'Đã tạo tài khoản người dùng.';
        } elseif ($path === '/devices/claim') {
            $db->claim((int) $user['id'], (string) ($_POST['device_id'] ?? ''),
                (string) ($_POST['pairing_code'] ?? ''));
            $_SESSION['flash'] = 'Đã ghép thiết bị và cấp quyền quản trị cho tài khoản hiện tại.';
        } elseif ($path === '/grants/set') {
            $db->grant((int) $user['id'], (int) ($_POST['user_id'] ?? 0),
                (string) ($_POST['device_id'] ?? ''), (string) ($_POST['role'] ?? ''));
            $_SESSION['flash'] = 'Đã cập nhật quyền.';
        } elseif ($path === '/grants/revoke') {
            $db->revoke((int) $user['id'], (int) ($_POST['user_id'] ?? 0),
                (string) ($_POST['device_id'] ?? ''));
            $_SESSION['flash'] = 'Đã thu hồi quyền.';
        } elseif ($path === '/devices/remove') {
            $db->removeDevice((int) $user['id'], (string) ($_POST['device_id'] ?? ''));
            $_SESSION['flash'] = 'Đã gỡ ghép thiết bị. Phải ghép lại từ máy TaskDeck để sử dụng.';
        } else {
            fail(404, 'Not found');
        }
    } catch (Throwable $e) {
        error_log('Portal admin action failed: ' . $path);
        $_SESSION['flash'] = 'Thao tác thất bại; kiểm tra dữ liệu hoặc trạng thái ghép nối.';
    }
    redirectHome();
}
if ($method !== 'GET' || $path !== '/') {
    fail(404, 'Not found');
}
$flash = $_SESSION['flash'] ?? null;
unset($_SESSION['flash']);
?>
<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>TaskDeck Remote Portal</title>
<link rel="stylesheet" href="/style.css">
<?php if ($user && !$needsMfa && $probeAvailable): ?>
<script src="/remote_crypto.js" defer></script>
<script src="/remote_probe.js" defer></script>
<?php if ($loginProbeAvailable): ?><script src="/remote_login.js" defer></script><?php endif ?>
<?php endif ?>
</head>
<body>
<main class="container">
<header class="top">
  <div><h1>TaskDeck Remote</h1><p>Quản lý thiết bị và quyền kết nối · Bản thử nghiệm</p></div>
  <?php if ($user): ?>
    <form method="post" action="/logout"><?= formCsrf() ?><button type="submit">Đăng xuất (<?= h($user['username']) ?>)</button></form>
  <?php endif ?>
</header>
<?php if ($flash): ?><p class="flash" role="status"><?= h($flash) ?></p><?php endif ?>

<?php if (!$user): ?>
<section class="panel">
<h2>Đăng nhập quản lý</h2>
<form method="post" action="/login" class="stack">
<?= formCsrf() ?>
<label>Tài khoản<input name="username" autocomplete="username" required maxlength="40"></label>
<label>Mật khẩu<input type="password" name="password" autocomplete="current-password" required></label>
<label>Mã Authenticator (6 số, nếu đã bật MFA)<input type="text" name="totp_code" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="one-time-code"></label>
<button type="submit">Đăng nhập</button>
</form>
</section>
<?php else: ?>
<?php if ((int)$user['mfa_enabled'] !== 1): ?>
<section class="panel">
<h2>Bảo vệ tài khoản bằng mã Authenticator (TOTP)</h2>
<p>Đăng nhập bằng ứng dụng Authenticator tiêu chuẩn. Khi kích hoạt, các phiên Portal khác sẽ bị vô hiệu. Không sử dụng mã SMS hoặc lưu mã bí mật trong file công khai.</p>
<form method="post" action="/account/mfa/start" class="stack">
<?= formCsrf() ?>
<label>Xác nhận mật khẩu Portal để bắt đầu<input type="password" name="current_password" required autocomplete="current-password"></label>
<button type="submit">Tạo mã thiết lập MFA</button>
</form>
<?php if (is_string($_SESSION['mfa_pending_secret'] ?? null) && (int)($_SESSION['mfa_pending_until'] ?? 0) >= time()): ?>
<p>Thêm tài khoản vào ứng dụng Authenticator bằng mã bí mật sau (chỉ hiện trong 5 phút):</p>
<p><code><?= h($_SESSION['mfa_pending_secret']) ?></code></p>
<p class="note">Trong ứng dụng Authenticator, chọn thêm tài khoản bằng mã thiết lập, thời gian 30 giây, 6 chữ số, SHA-1.</p>
<form method="post" action="/account/mfa/confirm" class="stack">
<?= formCsrf() ?>
<label>Nhập mã 6 số từ Authenticator<input name="totp_code" type="text" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="one-time-code" required></label>
<button type="submit">Xác minh và kích hoạt MFA</button>
</form>
<?php endif ?>
</section>
<?php endif ?>
<?php if ($needsMfa): ?>
<section class="panel">
<h2>Yêu cầu xác thực hai yếu tố</h2>
<p>Quản trị Portal yêu cầu MFA. Hãy hoàn tất thiết lập Authenticator ở trên trước khi xem thiết bị, quản lý quyền hoặc tạo ticket.</p>
</section>
<?php else: ?>
<section class="panel">
<h2>Máy TaskDeck được cấp quyền</h2>
<p>Chỉ hiển thị định danh, tên và presence có thời hạn. Không lưu IP, port hoặc mật khẩu TaskDeck.</p>
<?php if ($probeAvailable): ?>
<div id="remote-probe-config" data-relay-origin="<?= h($relayOrigin) ?>" data-csrf="<?= h(csrf()) ?>"></div>
<?php endif ?>
<div class="table-wrap"><table><thead><tr><th>Tên</th><th>Device ID</th><th>Trạng thái</th><th>Quyền</th><?php if ($probeAvailable): ?><th>Kiểm tra</th><?php endif ?></tr></thead><tbody>
<?php foreach ($db->devicesFor((int) $user['id'], (bool) $user['is_admin']) as $device): ?>
<tr>
<td><?= h($device['display_name']) ?></td>
<td><code><?= h($device['device_id']) ?></code></td>
<td><?= (int) $device['online'] === 1 ? 'Online' : 'Offline' ?></td>
<td><?= h($device['access_role'] ?? 'Portal admin') ?></td>
<?php if ($probeAvailable): ?>
<td><button type="button" class="remote-probe-button" data-device-id="<?= h($device['device_id']) ?>" <?= (int) $device['online'] !== 1 ? 'disabled' : '' ?>>Kiểm tra Agent</button>
<span class="remote-probe-result" role="status" aria-live="polite"></span>
<?php if ($loginProbeAvailable): ?>
<details class="remote-login-details">
<summary>Thử đăng nhập TaskDeck qua mã hóa đầu cuối</summary>
<p class="note">Chỉ kiểm tra mật khẩu TaskDeck trên Agent; không mở phiên làm việc, terminal hay HTTP proxy. Mật khẩu không được gửi tới PHP Portal.</p>
<form class="remote-login-probe-form" autocomplete="off">
<label>Tài khoản TaskDeck<input type="text" name="taskdeck_username" required maxlength="128" autocomplete="off"></label>
<label>Mật khẩu TaskDeck<input type="password" name="taskdeck_password" required maxlength="4096" autocomplete="off"></label>
<button type="submit">Xác minh qua E2EE</button>
<span class="remote-login-probe-result" role="status" aria-live="polite"></span>
</form>
</details>
<?php endif ?>
</td>
<?php endif ?>
</tr>
<?php endforeach ?>
</tbody></table></div>
<p class="note">Điều khiển Remote chưa mở: cần hoàn tất Relay, Agent và xác thực TaskDeck bắt buộc.</p>
<?php if ((int) $user['is_admin'] === 1): ?>
<details><summary>Gỡ ghép thiết bị (thu hồi toàn bộ quyền)</summary>
<p>Thao tác này xóa định danh cùng toàn bộ quyền được cấp tại Portal; muốn sử dụng lại phải xác nhận ghép lại trực tiếp trên TaskDeck.</p>
<form method="post" action="/devices/remove" class="inline">
<?= formCsrf() ?>
<label>Thiết bị<select name="device_id" required>
<?php foreach ($db->devicesFor((int) $user['id'], true) as $d): ?>
<option value="<?= h($d['device_id']) ?>"><?= h($d['display_name']) ?></option>
<?php endforeach ?>
</select></label>
<button type="submit" class="danger">Gỡ ghép và thu hồi quyền</button>
</form>
</details>
<?php endif ?>
</section>
<section class="panel">
<details>
<summary>Đổi mật khẩu tài khoản Portal</summary>
<p>Yêu cầu mật khẩu hiện tại. Sau khi đổi, mọi phiên Portal khác của tài khoản sẽ hết hiệu lực.</p>
<form method="post" action="/account/password" class="stack">
<?= formCsrf() ?>
<label>Mật khẩu hiện tại<input type="password" name="current_password" required autocomplete="current-password"></label>
<label>Mật khẩu mới (tối thiểu 14 ký tự)<input type="password" name="new_password" required minlength="14" autocomplete="new-password"></label>
<label>Nhập lại mật khẩu mới<input type="password" name="confirm_password" required minlength="14" autocomplete="new-password"></label>
<?php if ((int)$user['mfa_enabled'] === 1): ?>
<label>Mã Authenticator mới (6 số)<input type="text" name="totp_code" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="one-time-code" required></label>
<?php endif ?>
<button type="submit">Đổi mật khẩu</button>
</form>
</details>
</section>
<?php if ((int) $user['is_admin'] === 1): ?>
<section class="grid">
<div class="panel">
<h2>Ghép TaskDeck</h2>
<p>Nhập Device ID và mã ngẫu nhiên 32 ký tự được hiển thị <strong>trực tiếp tại TaskDeck</strong>. Mã có thời hạn 5 phút.</p>
<form method="post" action="/devices/claim" class="stack">
<?= formCsrf() ?>
<label>Device ID<input name="device_id" maxlength="64" minlength="64" required></label>
<label>Mã ghép nối<input name="pairing_code" maxlength="32" minlength="32" required autocomplete="off"></label>
<button type="submit">Xác nhận ghép thiết bị</button>
</form>
</div>
<div class="panel">
<h2>Tạo tài khoản</h2>
<form method="post" action="/users/create" class="stack">
<?= formCsrf() ?>
<label>Username<input name="username" required minlength="3" maxlength="40"></label>
<label>Mật khẩu ban đầu (ít nhất 14 ký tự)<input name="password" type="password" autocomplete="new-password" minlength="14" required></label>
<button type="submit">Tạo người dùng</button>
</form>
<p class="note">MFA, self-service đổi mật khẩu và recovery an toàn chưa được triển khai. Chưa đưa ra Internet công khai.</p>
</div>
</section>
<section class="panel">
<h2>Cấp quyền theo máy</h2>
<form method="post" action="/grants/set" class="inline">
<?= formCsrf() ?>
<label>Người dùng<select name="user_id" required><?php foreach ($db->allUsers() as $u): ?><option value="<?= (int) $u['id'] ?>"><?= h($u['username']) ?></option><?php endforeach ?></select></label>
<label>TaskDeck<select name="device_id" required><?php foreach ($db->devicesFor((int) $user['id'], true) as $d): ?><option value="<?= h($d['device_id']) ?>"><?= h($d['display_name']) ?></option><?php endforeach ?></select></label>
<label>Quyền<select name="role"><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select></label>
<button type="submit">Cấp/Cập nhật</button>
</form>
<div class="table-wrap"><table><thead><tr><th>Người dùng</th><th>Thiết bị</th><th>Quyền</th><th></th></tr></thead><tbody>
<?php foreach ($db->grants() as $g): ?>
<tr><td><?= h($g['username']) ?></td><td><?= h($g['display_name']) ?></td><td><?= h($g['access_role']) ?></td>
<td><form method="post" action="/grants/revoke"><?= formCsrf() ?>
<input type="hidden" name="user_id" value="<?= (int) $g['user_id'] ?>">
<input type="hidden" name="device_id" value="<?= h($g['device_id']) ?>">
<button type="submit" class="danger">Thu hồi</button></form></td></tr>
<?php endforeach ?>
</tbody></table></div>
</section>
<?php endif ?>
<?php endif ?>
<?php endif ?>
<footer>TaskDeck Remote Control Plane · Chưa hỗ trợ mở phiên remote trong giai đoạn này.</footer>
</main>
</body></html>
