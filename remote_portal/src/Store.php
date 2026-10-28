<?php
declare(strict_types=1);

namespace TaskDeck\RemotePortal;

require_once __DIR__ . '/Totp.php';

use PDO;
use RuntimeException;

/**
 * Control-plane storage only. It MUST NOT store TaskDeck credentials,
 * addresses, endpoints, remote session tokens or terminal/file content.
 */
final class Store
{
    private PDO $db;
    private ?string $abuseKeyPath = null;
    private ?string $memoryAbuseKey = null;
    private ?string $mfaKeyPath = null;
    private ?string $memoryMfaKey = null;

    public function __construct(string $path)
    {
        if ($path === ':memory:') {
            $this->memoryAbuseKey = random_bytes(32);
            $this->memoryMfaKey = random_bytes(32);
        }
        if ($path !== ':memory:') {
            if ($path === '' || $path[0] !== '/') {
                throw new RuntimeException('REMOTE_PORTAL_DB_PATH must be absolute and outside public webroot');
            }
            $dir = realpath(dirname($path));
            $this->abuseKeyPath = dirname($path) . '/portal-abuse-hmac.key';
            $this->mfaKeyPath = dirname($path) . '/portal-mfa.key';
            $public = realpath(__DIR__ . '/../public');
            if ($dir === false || $public === false
                || $dir === $public || str_starts_with($dir . '/', $public . '/')) {
                throw new RuntimeException('Invalid DB directory: create a private directory outside webroot');
            }
            // On shared hosting the SQLite DB contains Portal password
            // hashes and user/device grants. Being outside the webroot is
            // not sufficient if neighboring Unix users can read the files.
            // Keep the parent private: SQLite's journal files live here too.
            if (is_link(dirname($path)) || (fileperms($dir) & 0077) !== 0) {
                throw new RuntimeException('Portal DB directory must be owner-only (chmod 700)');
            }
            if (!file_exists($path) && !is_link($path)) {
                // Pre-create with a restrictive umask BEFORE SQLite opens
                // it; chmod after a normal PDO connect leaves a disclosure
                // window on shared hosts. Exclusive create is safe if two
                // PHP workers race to initialize a new database.
                $oldMask = umask(0077);
                try {
                    $fd = @fopen($path, 'x+b');
                } finally {
                    umask($oldMask);
                }
                if ($fd !== false) {
                    fclose($fd);
                }
            }
            clearstatcache(true, $path);
            if (!is_file($path) || is_link($path)
                || (fileperms($path) & 0077) !== 0) {
                throw new RuntimeException('Portal DB file must be regular, not a symlink, and chmod 600');
            }
        }
        $this->db = new PDO('sqlite:' . $path, null, null, [
            PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
            PDO::ATTR_DEFAULT_FETCH_MODE => PDO::FETCH_ASSOC,
            PDO::ATTR_TIMEOUT => 5,
        ]);
        $this->db->exec('PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;');
    }

    public function initialize(): void
    {
        $this->db->exec(<<<'SQL'
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0,1)),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS devices (
  device_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS user_devices (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
  access_role TEXT NOT NULL CHECK(access_role IN ('viewer','operator','admin')),
  PRIMARY KEY(user_id, device_id)
);
CREATE TABLE IF NOT EXISTS pending_enrollments (
  device_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  pairing_hash TEXT NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS device_nonces (
  device_id TEXT NOT NULL,
  nonce TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  PRIMARY KEY(device_id, nonce)
);
-- Bounded proof admission for *paired* devices. Unlike enrollment,
-- authenticated Agent heartbeat/grant-check have no anonymous IP budget.
CREATE TABLE IF NOT EXISTS signed_agent_limits (
  device_id TEXT PRIMARY KEY REFERENCES devices(device_id) ON DELETE CASCADE,
  window_started INTEGER NOT NULL,
  attempted INTEGER NOT NULL CHECK(attempted >= 0 AND attempted <= 90)
);
CREATE TABLE IF NOT EXISTS presence (
  device_id TEXT PRIMARY KEY REFERENCES devices(device_id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS login_attempts (
  username TEXT PRIMARY KEY COLLATE NOCASE,
  failures INTEGER NOT NULL,
  locked_until INTEGER NOT NULL
);
-- Per-account durable throttle on privileged password verification
-- performed inside an existing Portal session. No IP or password stored.
CREATE TABLE IF NOT EXISTS account_sensitive_limits (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  window_started INTEGER NOT NULL,
  attempted INTEGER NOT NULL CHECK(attempted >= 0 AND attempted <= 5)
);
CREATE TABLE IF NOT EXISTS remote_ticket_limits (
  scope TEXT NOT NULL CHECK(scope IN ('user','device')),
  scope_key TEXT NOT NULL,
  window_started INTEGER NOT NULL,
  issued INTEGER NOT NULL CHECK(issued >= 0),
  PRIMARY KEY(scope, scope_key)
);
-- No raw IPs, usernames or tokens: source_key is HMAC-SHA256(IP).
-- The secret lives in a separate owner-only file outside the public root.
CREATE TABLE IF NOT EXISTS portal_ingress_limits (
  action TEXT NOT NULL CHECK(action IN ('login','enroll')),
  scope TEXT NOT NULL CHECK(scope IN ('global','source')),
  scope_key TEXT NOT NULL,
  window_started INTEGER NOT NULL,
  attempts INTEGER NOT NULL CHECK(attempts >= 0),
  PRIMARY KEY(action,scope,scope_key)
);
-- Separate additive table: existing installations have a SQLite CHECK
-- constraint allowing only login/enroll in portal_ingress_limits.
-- Agent ingress cannot simply add an action to the old table on upgrade.
CREATE TABLE IF NOT EXISTS portal_agent_ingress_limits (
  scope TEXT NOT NULL CHECK(scope IN ('global','source')),
  scope_key TEXT NOT NULL,
  window_started INTEGER NOT NULL,
  attempts INTEGER NOT NULL CHECK(attempts >= 0),
  PRIMARY KEY(scope,scope_key)
);
-- An anonymous browser page/POST can create PHP session files even when
-- CSRF fails. Gate session creation before session_start() using this
-- independent, durable admission bucket.
CREATE TABLE IF NOT EXISTS portal_browser_ingress_limits (
  scope TEXT NOT NULL CHECK(scope IN ('global','source')),
  scope_key TEXT NOT NULL,
  window_started INTEGER NOT NULL,
  attempts INTEGER NOT NULL CHECK(attempts >= 0),
  PRIMARY KEY(scope,scope_key)
);
CREATE TABLE IF NOT EXISTS portal_admin_limits (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  window_started INTEGER NOT NULL,
  attempted INTEGER NOT NULL CHECK(attempted >= 0 AND attempted <= 60)
);
-- Keep non-secret SHA-256 commitments of private keys. If a deployed HMAC
-- key disappears, migration must never silently regenerate source identity.
CREATE TABLE IF NOT EXISTS portal_key_checks (
  purpose TEXT PRIMARY KEY CHECK(purpose IN ('ingress_hmac')),
  sha256 TEXT NOT NULL CHECK(length(sha256)=64)
);
CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY,
  actor_id INTEGER,
  event_type TEXT NOT NULL,
  device_id TEXT,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_devices_device ON user_devices(device_id);
CREATE INDEX IF NOT EXISTS idx_pending_expiry ON pending_enrollments(expires_at);
-- Replay records are short lived (180s). Prune with an expiry index rather
-- than scanning all active paired-device nonces on every signed heartbeat.
CREATE INDEX IF NOT EXISTS idx_device_nonces_expiry ON device_nonces(expires_at);
SQL);
        // Existing installations retain all user IDs, hashes and grants.
        $columns = array_column($this->db->query('PRAGMA table_info(users)')->fetchAll(), 'name');
        if (!in_array('auth_epoch', $columns, true)) {
            $this->db->exec('ALTER TABLE users ADD COLUMN auth_epoch INTEGER NOT NULL DEFAULT 1');
        }
        if (!in_array('mfa_secret_enc', $columns, true)) {
            $this->db->exec("ALTER TABLE users ADD COLUMN mfa_secret_enc TEXT NOT NULL DEFAULT ''");
        }
        if (!in_array('mfa_last_step', $columns, true)) {
            $this->db->exec('ALTER TABLE users ADD COLUMN mfa_last_step INTEGER NOT NULL DEFAULT -1');
        }
        // Earlier Portal releases allowed unknown login names to create
        // persistent rows. Idempotent migration removes those stale entries,
        // while retaining the live account's brute-force counters.
        $this->db->exec('DELETE FROM login_attempts WHERE NOT EXISTS (
            SELECT 1 FROM users WHERE users.username = login_attempts.username COLLATE NOCASE
        )');
        $this->initializeAbuseKey();
        $this->registerAbuseKeyFingerprint();
        $this->initializeMfaKey();
    }

    public function createUser(string $username, string $password, bool $admin): int
    {
        $username = strtolower(trim($username));
        if (!preg_match('/^[a-z][a-z0-9_.-]{2,40}$/D', $username)
            || strlen($password) < 14 || strlen($password) > 1024) {
            throw new RuntimeException('Username or password violates policy (password >=14 characters)');
        }
        $hash = password_hash($password, defined('PASSWORD_ARGON2ID') ? PASSWORD_ARGON2ID : PASSWORD_DEFAULT);
        $stmt = $this->db->prepare('INSERT INTO users(username,password_hash,is_admin,created_at) VALUES(?,?,?,?)');
        $stmt->execute([$username, $hash, $admin ? 1 : 0, time()]);
        return (int) $this->db->lastInsertId();
    }

    /**
     * Reserve each password verification BEFORE hashing. BEGIN IMMEDIATE
     * serializes parallel PHP workers, so 50 simultaneous guesses cannot all
     * read the same pre-attack failure count. A successful login removes its
     * own short-lived account throttle; failures never persist unknown users.
     */
    public function authenticate(string $username, string $password, string $totpCode = ''): ?array
    {
        $username = strtolower(trim($username));
        if (!preg_match('/^[a-z][a-z0-9_.-]{2,40}$/D', $username)
            || strlen($password) > 2048) {
            return null;
        }
        $now = time();
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $user = $this->queryOne('SELECT * FROM users WHERE username=? AND enabled=1', [$username]);
            $lock = $user === null ? null : $this->queryOne(
                'SELECT failures,locked_until FROM login_attempts WHERE username=?', [$username]);
            if ($user === null || ($lock !== null && (int)$lock['failures'] >= 5
                    && (int)$lock['locked_until'] > $now)) {
                $this->db->exec('COMMIT');
                // Missing usernames AND locked existing usernames must
                // perform equivalent dummy verification. Otherwise an
                // attacker can submit >5 invalid guesses and use the fast
                // locked-account response as a username enumeration oracle.
                // This work is protected by the pre-auth ingress quota.
                password_verify($password, self::UNKNOWN_USER_HASH);
                return null;
            }
            $failures = $lock !== null && (int)$lock['locked_until'] > $now
                ? (int)$lock['failures'] + 1 : 1;
            $until = $failures >= 5 ? $now + 120 : $now + 180;
            $this->db->prepare('INSERT INTO login_attempts(username,failures,locked_until) VALUES(?,?,?)
                ON CONFLICT(username) DO UPDATE SET failures=excluded.failures,locked_until=excluded.locked_until')
                ->execute([$username, $failures, $until]);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
        if (!password_verify($password, $user['password_hash'])) {
            return null;
        }
        // Password rotation or disabling the user during a slow Argon2 check
        // cannot resurrect a stale session generation.
        $current = $this->queryOne(
            'SELECT password_hash,auth_epoch,mfa_secret_enc,mfa_last_step
             FROM users WHERE id=? AND enabled=1', [$user['id']]);
        if ($current === null || !hash_equals($user['password_hash'],$current['password_hash'])
            || (int)$current['auth_epoch'] !== (int)$user['auth_epoch']) {
            return null;
        }
        // Enrolled MFA is compulsory even if the deployment has not yet
        // enabled REQUIRE_MFA for accounts still waiting to enroll.
        if ($current['mfa_secret_enc'] !== ''
            && !$this->consumeMfaCode((int)$user['id'], $current, $totpCode)) {
            return null;
        }
        $this->db->prepare('DELETE FROM login_attempts WHERE username=?')->execute([$username]);
        return ['id' => (int)$user['id'], 'username' => $user['username'],
            'is_admin' => (bool)$user['is_admin'], 'auth_epoch' => (int)$user['auth_epoch']];
    }

    // A fixed non-account hash prevents a trivial existence timing oracle.
    // Its literal password is intentionally unknown to all Portal accounts.
    private const UNKNOWN_USER_HASH =
        '$argon2id$v=19$m=65536,t=4,p=1$aUNLYlZpNVNVcE5xdm92RA$NC7gZOrexjCrr51FNyW3ztlJQj14AxcbWLsYqxvs9Is';

    /**
     * Durable pre-auth admission for anonymous web endpoints, shared across
     * PHP-FPM processes via SQLite BEGIN IMMEDIATE. No IP is ever persisted:
     * scope_key stores HMAC-SHA256(remote_addr) under a separate private key.
     * Never trust caller-supplied X-Forwarded-For / forwarding headers.
     *
     * Login: 10 attempts/source and 180 global / 60 seconds.
     * Enrollment: 12 attempts/source and 120 global / 60 seconds.
     * Signed Agent HTTP API: 900 attempts/source and 9000 global / 60s
     * (in addition to stricter 90/min on each authenticated Device ID).
     * Browser GET/POST before PHP session: 120/source and 2400 global / 60s.
     * These caps account for many legitimate Agents behind one NAT.
     * Failure to load key or write the quota table denies the request.
     * The hard global cap bounds database cardinality under IPv6 rotation.
     */
    public function reserveIngress(string $action, string $remoteAddress, ?int $now = null): bool
    {
        if (!in_array($action, ['login', 'enroll', 'agent', 'browser'], true)
            || filter_var($remoteAddress, FILTER_VALIDATE_IP) === false) {
            throw new RuntimeException('Invalid ingress source');
        }
        $key = $this->loadAbuseKey();
        $source = hash_hmac('sha256', $remoteAddress, $key);
        $now ??= time();
        if ($now < 1) {
            throw new RuntimeException('Invalid ingress clock');
        }
        $sourceLimit = match ($action) {
            'login' => 10, 'enroll' => 12, 'agent' => 900, 'browser' => 120,
        };
        $globalLimit = match ($action) {
            'login' => 180, 'enroll' => 120, 'agent' => 9000, 'browser' => 2400,
        };
        $isAgent = $action === 'agent';
        $isBrowser = $action === 'browser';
        $isStandalone = $isAgent || $isBrowser;
        // Table names are selected from fixed server constants, never
        // interpolated from user input.
        $table = $isAgent ? 'portal_agent_ingress_limits'
            : ($isBrowser ? 'portal_browser_ingress_limits' : 'portal_ingress_limits');
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $checks = [['global','*',$globalLimit], ['source',$source,$sourceLimit]];
            $updates = [];
            foreach ($checks as [$scope,$identity,$limit]) {
                $row = $isStandalone
                    ? $this->queryOne('SELECT window_started,attempts FROM '.$table.'
                        WHERE scope=? AND scope_key=?', [$scope,$identity])
                    : $this->queryOne('SELECT window_started,attempts FROM portal_ingress_limits
                        WHERE action=? AND scope=? AND scope_key=?', [$action,$scope,$identity]);
                if ($row === null || (int)$row['window_started'] <= $now - 60) {
                    $updates[] = [$scope,$identity,$now,1];
                } elseif ((int)$row['window_started'] > $now
                    || (int)$row['attempts'] >= $limit) {
                    $this->db->exec('ROLLBACK');
                    return false;
                } else {
                    $updates[] = [$scope,$identity,(int)$row['window_started'],(int)$row['attempts']+1];
                }
            }
            foreach ($updates as [$scope,$identity,$window,$count]) {
                if ($isStandalone) {
                    $this->db->prepare('INSERT INTO '.$table.'
                        (scope,scope_key,window_started,attempts) VALUES(?,?,?,?)
                        ON CONFLICT(scope,scope_key) DO UPDATE SET
                        window_started=excluded.window_started,attempts=excluded.attempts')
                        ->execute([$scope,$identity,$window,$count]);
                } else {
                    $this->db->prepare('INSERT INTO portal_ingress_limits
                        (action,scope,scope_key,window_started,attempts) VALUES(?,?,?,?,?)
                        ON CONFLICT(action,scope,scope_key) DO UPDATE SET
                        window_started=excluded.window_started,attempts=excluded.attempts')
                        ->execute([$action,$scope,$identity,$window,$count]);
                }
            }
            $this->db->prepare('DELETE FROM ' . $table . ' WHERE window_started<?')
                ->execute([$now-120]);
            $this->db->exec('COMMIT');
            return true;
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    private function initializeAbuseKey(): void
    {
        if ($this->abuseKeyPath === null) {
            return;
        }
        $path = $this->abuseKeyPath;
        if (!file_exists($path) && !is_link($path)) {
            // A missing key on an upgraded database that has already
            // enrolled its identity is a security incident, not a new
            // deployment. Never mint a fresh key to reset source quotas.
            if ($this->queryOne('SELECT 1 FROM portal_key_checks
                WHERE purpose=?', ['ingress_hmac']) !== null) {
                throw new RuntimeException('Portal HMAC key missing; restore private backup');
            }
        }
        if (file_exists($path) || is_link($path)) {
            $this->loadAbuseKey();
            return;
        }
        // A lost HMAC key cannot silently reset persisted per-source
        // identities; only a new/empty deployment may create its first key.
        if ($this->db->query('SELECT 1 FROM portal_ingress_limits LIMIT 1')->fetch() !== false
            || $this->db->query('SELECT 1 FROM portal_agent_ingress_limits LIMIT 1')->fetch() !== false
            || $this->db->query('SELECT 1 FROM portal_browser_ingress_limits LIMIT 1')->fetch() !== false) {
            throw new RuntimeException('Portal ingress key missing: restore original private backup');
        }
        $oldMask = umask(0077);
        try {
            $fd = @fopen($path, 'x+b');
        } finally {
            umask($oldMask);
        }
        if ($fd !== false) {
            try {
                $bytes = random_bytes(32);
                if (fwrite($fd, $bytes) !== 32 || !fflush($fd)) {
                    throw new RuntimeException('Cannot write Portal abuse protection key');
                }
            } finally {
                fclose($fd);
            }
        }
        // A competing CLI init may have created the file. The final file
        // must be a valid owner-only secret; partial writes fail closed.
        $this->loadAbuseKey();
    }

    /**
     * One-time additive migration for databases that already have a key,
     * including legacy deployments. The commitment is not a password/hash
     * of a credential: it only detects substitution of a random 256-bit key.
     * Two concurrent CLI init processes converge on the same fingerprint.
     */
    private function registerAbuseKeyFingerprint(): void
    {
        $key = $this->loadAbuseKey();
        $digest = hash('sha256', $key);
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $this->db->prepare('INSERT OR IGNORE INTO portal_key_checks(purpose,sha256)
                VALUES(?,?)')->execute(['ingress_hmac', $digest]);
            $stored = $this->queryOne('SELECT sha256 FROM portal_key_checks
                WHERE purpose=?', ['ingress_hmac']);
            if ($stored === null || !hash_equals($stored['sha256'], $digest)) {
                throw new RuntimeException('Portal HMAC key has changed; restore original backup');
            }
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    private function loadAbuseKey(): string
    {
        if ($this->memoryAbuseKey !== null) {
            return $this->memoryAbuseKey;
        }
        $path = $this->abuseKeyPath;
        if ($path === null || is_link($path) || !is_file($path)) {
            throw new RuntimeException('Portal ingress rate key missing; run admin.php init');
        }
        clearstatcache(true, $path);
        if ((fileperms($path) & 0077) !== 0) {
            throw new RuntimeException('Portal ingress rate key must be chmod 600');
        }
        $data = @file_get_contents($path);
        if (!is_string($data) || strlen($data) !== 32) {
            throw new RuntimeException('Invalid Portal ingress rate key');
        }
        return $data;
    }

    public function userById(int $id): ?array
    {
        $r = $this->queryOne("SELECT id,username,is_admin,enabled,auth_epoch,(mfa_secret_enc!='') AS mfa_enabled FROM users WHERE id=?", [$id]);
        return $r && (int) $r['enabled'] === 1 ? $r : null;
    }

    /**
     * Changing a Portal password invalidates all other Portal browser sessions.
     * No TaskDeck authentication material is read or stored here.
     * The caller must regenerate their PHP session ID and adopt auth_epoch.
     */
    public function changePassword(int $userId, string $oldPassword, string $newPassword, string $totpCode = ''): int
    {
        if ($userId < 1 || strlen($oldPassword) > 2048
            || strlen($newPassword) < 14 || strlen($newPassword) > 1024
            || hash_equals($oldPassword, $newPassword)) {
            throw new RuntimeException('Password policy check failed');
        }
        // An authenticated (or hijacked) browser session cannot perform
        // unlimited Argon2 comparisons against the current password.
        // Quota is reserved before hashing and is shared by all workers.
        $this->reserveSensitiveAttempt($userId);
        $user = $this->queryOne(
            'SELECT password_hash,auth_epoch,mfa_secret_enc,mfa_last_step
             FROM users WHERE id=? AND enabled=1', [$userId]);
        if ($user === null || !password_verify($oldPassword, $user['password_hash'])) {
            throw new RuntimeException('Current password invalid');
        }
        if ($user['mfa_secret_enc'] !== ''
            && !$this->consumeMfaCode($userId, $user, $totpCode)) {
            throw new RuntimeException('MFA proof invalid');
        }
        $newHash = password_hash($newPassword,
            defined('PASSWORD_ARGON2ID') ? PASSWORD_ARGON2ID : PASSWORD_DEFAULT);
        $stmt = $this->db->prepare(
            'UPDATE users SET password_hash=?,auth_epoch=auth_epoch+1 WHERE id=? AND password_hash=? AND enabled=1');
        $stmt->execute([$newHash, $userId, $user['password_hash']]);
        if ($stmt->rowCount() !== 1) {
            throw new RuntimeException('Password changed concurrently; retry');
        }
        $this->audit($userId, 'portal.password_changed', null);
        $updated = $this->userById($userId);
        if ($updated === null) {
            throw new RuntimeException('Account was disabled');
        }
        return (int) $updated['auth_epoch'];
    }

    /**
     * Authorize expensive operations for one already-authenticated Portal
     * user: five attempts per ten minutes across all sessions and PHP-FPM
     * workers. The counter is committed before password verification.
     * It intentionally counts success as well to prevent quota resets by
     * alternating valid and invalid credentials.
     */
    public function reserveSensitiveAttempt(int $userId, ?int $now = null): void
    {
        if ($userId < 1) {
            throw new RuntimeException('Sensitive action denied');
        }
        $now ??= time();
        if ($now < 1) {
            throw new RuntimeException('Invalid sensitive action clock');
        }
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            if (!$this->queryOne('SELECT 1 FROM users WHERE id=? AND enabled=1', [$userId])) {
                throw new RuntimeException('Sensitive action denied');
            }
            $row = $this->queryOne(
                'SELECT window_started,attempted FROM account_sensitive_limits WHERE user_id=?',
                [$userId]
            );
            if ($row === null || (int)$row['window_started'] <= $now - 600) {
                $window = $now;
                $attempts = 1;
            } else {
                if ((int)$row['window_started'] > $now || (int)$row['attempted'] >= 5) {
                    throw new RuntimeException('Sensitive action quota exceeded');
                }
                $window = (int)$row['window_started'];
                $attempts = (int)$row['attempted'] + 1;
            }
            $this->db->prepare(
                'INSERT INTO account_sensitive_limits(user_id,window_started,attempted) VALUES(?,?,?)
                 ON CONFLICT(user_id) DO UPDATE SET
                 window_started=excluded.window_started,attempted=excluded.attempted'
            )->execute([$userId,$window,$attempts]);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }


    /**
     * MFA enrollment requires the current Portal password even for an
     * already-authenticated browser. Each attempt consumes the same durable
     * per-user quota as password changes, preventing session-based flooding.
     */
    public function verifyPasswordForMfaEnrollment(int $userId, string $password): bool
    {
        if ($userId < 1 || strlen($password) > 2048) {
            return false;
        }
        $this->reserveSensitiveAttempt($userId);
        $current = $this->queryOne(
            "SELECT password_hash,mfa_secret_enc FROM users WHERE id=? AND enabled=1", [$userId]);
        return $current !== null && $current['mfa_secret_enc'] === ''
            && password_verify($password, $current['password_hash']);
    }

    /**
     * A pending 160-bit TOTP secret stays in the short-lived PHP session
     * until the user demonstrates possession of an authenticator. No secret
     * enters SQLite before confirmation. The committed secret is encrypted
     * with libsodium and an independent 0600 key outside the webroot.
     */
    public function activateTotp(int $userId, string $secret, string $code): int
    {
        $this->reserveSensitiveAttempt($userId);
        $step = Totp::match($secret, $code);
        if ($step === null) {
            throw new RuntimeException('Invalid MFA verification code');
        }
        $encrypted = $this->encryptTotp($secret);
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $user = $this->queryOne(
                "SELECT auth_epoch FROM users WHERE id=? AND enabled=1 AND mfa_secret_enc=''",
                [$userId]);
            if ($user === null) {
                throw new RuntimeException('MFA enrollment denied');
            }
            $q = $this->db->prepare(
                "UPDATE users SET mfa_secret_enc=?,mfa_last_step=?,auth_epoch=auth_epoch+1
                 WHERE id=? AND enabled=1 AND mfa_secret_enc='' AND auth_epoch=?");
            $q->execute([$encrypted,$step,$userId,$user['auth_epoch']]);
            if ($q->rowCount() !== 1) {
                throw new RuntimeException('Concurrent MFA enrollment rejected');
            }
            $this->db->exec('COMMIT');
            return (int)$user['auth_epoch'] + 1;
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    /**
     * Offline, operator-controlled MFA loss recovery only. Not routed by
     * public/index.php: the private CLI must explicitly confirm the action.
     * Increment auth_epoch to invalidate all browser sessions and old
     * signed tickets. On default MFA-required deployments this account
     * gets password-only access exclusively to reenroll its factor.
     */
    public function resetTotpForRecovery(int $userId): void
    {
        if ($userId < 1) {
            throw new RuntimeException('Invalid Portal account');
        }
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $q = $this->db->prepare(
                "UPDATE users SET mfa_secret_enc='',mfa_last_step=-1,
                 auth_epoch=auth_epoch+1 WHERE id=? AND enabled=1 AND mfa_secret_enc!=''");
            $q->execute([$userId]);
            if ($q->rowCount() !== 1) {
                throw new RuntimeException('Account has no active MFA factor');
            }
            $this->audit($userId,'portal.mfa_reset_offline',null);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    private function consumeMfaCode(int $userId, array $user, string $code): bool
    {
        $secret = $this->decryptTotp((string)$user['mfa_secret_enc']);
        $step = Totp::match($secret, $code, null, (int)$user['mfa_last_step']);
        if ($step === null) {
            return false;
        }
        // A successful TOTP code is usable only once across *all* PHP
        // workers and Portal sessions. No SELECT -> UPDATE race is possible.
        $q = $this->db->prepare(
            'UPDATE users SET mfa_last_step=? WHERE id=? AND mfa_last_step<?
             AND auth_epoch=? AND password_hash=? AND mfa_secret_enc=? AND enabled=1');
        $q->execute([$step,$userId,$step,$user['auth_epoch'],
            $user['password_hash'],$user['mfa_secret_enc']]);
        return $q->rowCount() === 1;
    }

    private function encryptTotp(string $secret): string
    {
        $key = $this->loadMfaKey();
        $nonce = random_bytes(SODIUM_CRYPTO_SECRETBOX_NONCEBYTES);
        return base64_encode($nonce . sodium_crypto_secretbox($secret, $nonce, $key));
    }

    private function decryptTotp(string $encrypted): string
    {
        $bytes = base64_decode($encrypted, true);
        if (!is_string($bytes) || strlen($bytes) < SODIUM_CRYPTO_SECRETBOX_NONCEBYTES+16) {
            throw new RuntimeException('Invalid Portal MFA storage');
        }
        $nonce = substr($bytes,0,SODIUM_CRYPTO_SECRETBOX_NONCEBYTES);
        $secret = sodium_crypto_secretbox_open(substr($bytes,SODIUM_CRYPTO_SECRETBOX_NONCEBYTES),
            $nonce,$this->loadMfaKey());
        if (!is_string($secret)) {
            throw new RuntimeException('Portal MFA secret could not be decrypted');
        }
        return $secret;
    }

    private function initializeMfaKey(): void
    {
        if ($this->mfaKeyPath === null) {
            return;
        }
        $path = $this->mfaKeyPath;
        if (!file_exists($path) && !is_link($path)) {
            // Recreating this key would render every previously enrolled
            // TOTP factor undecipherable. Require recovery of the original.
            if ($this->db->query("SELECT 1 FROM users WHERE mfa_secret_enc!='' LIMIT 1")->fetch() !== false) {
                throw new RuntimeException('Portal MFA key missing: restore original private backup');
            }
            $oldMask = umask(0077);
            try {
                $fd = @fopen($path,'x+b');
            } finally {
                umask($oldMask);
            }
            if ($fd !== false) {
                try {
                    if (fwrite($fd, random_bytes(32)) !== 32 || !fflush($fd)) {
                        throw new RuntimeException('Cannot store Portal MFA encryption key');
                    }
                } finally {
                    fclose($fd);
                }
            }
        }
        $this->loadMfaKey();
    }

    private function loadMfaKey(): string
    {
        if ($this->memoryMfaKey !== null) {
            return $this->memoryMfaKey;
        }
        $path = $this->mfaKeyPath;
        if ($path === null || is_link($path) || !is_file($path)) {
            throw new RuntimeException('Portal MFA key missing; run admin.php init');
        }
        clearstatcache(true,$path);
        if ((fileperms($path)&0077)!==0) {
            throw new RuntimeException('Portal MFA key must be chmod 600');
        }
        $data = @file_get_contents($path);
        if (!is_string($data) || strlen($data)!==32) {
            throw new RuntimeException('Portal MFA key invalid');
        }
        return $data;
    }

    /**
     * Reserve a bounded privileged mutation BEFORE altering Portal grants,
     * accounts or pairing. A hijacked admin PHP session cannot create
     * unlimited accounts/audit rows or alter grants at unbounded speed.
     *
     * Each enabled admin user has 60 mutations/60s across PHP workers.
     * Permission and quota are checked in the SAME SQLite write transaction.
     */
    public function reserveAdminMutation(int $adminId, ?int $now = null): void
    {
        if ($adminId < 1) {
            throw new RuntimeException('Admin mutation denied');
        }
        $now ??= time();
        if ($now < 1) {
            throw new RuntimeException('Invalid admin mutation clock');
        }
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            if (!$this->queryOne('SELECT 1 FROM users
                WHERE id=? AND enabled=1 AND is_admin=1', [$adminId])) {
                throw new RuntimeException('Admin mutation denied');
            }
            $row = $this->queryOne('SELECT window_started,attempted FROM portal_admin_limits
                WHERE user_id=?', [$adminId]);
            if ($row === null || (int)$row['window_started'] <= $now-60) {
                $window=$now;
                $attempted=1;
            } else {
                if ((int)$row['window_started'] > $now || (int)$row['attempted'] >= 60) {
                    throw new RuntimeException('Admin mutation quota exceeded');
                }
                $window=(int)$row['window_started'];
                $attempted=(int)$row['attempted']+1;
            }
            $this->db->prepare('INSERT INTO portal_admin_limits
                (user_id,window_started,attempted) VALUES(?,?,?)
                ON CONFLICT(user_id) DO UPDATE SET
                window_started=excluded.window_started,attempted=excluded.attempted')
                ->execute([$adminId,$window,$attempted]);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    public function allUsers(): array
    {
        return $this->db->query('SELECT id,username,is_admin,enabled FROM users ORDER BY username')->fetchAll();
    }

    public function devicesFor(int $userId, bool $admin): array
    {
        if ($admin) {
            $q = $this->db->prepare('SELECT d.device_id,d.display_name,d.enabled,
              CASE WHEN p.expires_at > ? THEN 1 ELSE 0 END AS online
              FROM devices d LEFT JOIN presence p ON p.device_id=d.device_id ORDER BY d.display_name');
            $q->execute([time()]);
            return $q->fetchAll();
        }
        $q = $this->db->prepare('SELECT d.device_id,d.display_name,d.enabled,ud.access_role,
          CASE WHEN p.expires_at > ? THEN 1 ELSE 0 END AS online
          FROM devices d JOIN user_devices ud ON ud.device_id=d.device_id
          LEFT JOIN presence p ON p.device_id=d.device_id WHERE ud.user_id=? AND d.enabled=1 ORDER BY d.display_name');
        $q->execute([time(), $userId]);
        return $q->fetchAll();
    }

    /**
     * A signed connection ticket requires an explicit per-user grant AND a
     * claimed, enabled, currently-online device. Portal admin status alone
     * does not silently authorize a remote session.
     */
    public function eligibleRemoteRole(int $userId, string $deviceId): ?string
    {
        if (!preg_match('/^[a-f0-9]{64}$/D', $deviceId)) {
            return null;
        }
        $q = $this->queryOne(
            'SELECT ud.access_role FROM user_devices ud ' .
            'JOIN users u ON u.id=ud.user_id ' .
            'JOIN devices d ON d.device_id=ud.device_id ' .
            'JOIN presence p ON p.device_id=d.device_id ' .
            'WHERE ud.user_id=? AND ud.device_id=? AND u.enabled=1 ' .
            'AND d.enabled=1 AND p.expires_at>?', [$userId, $deviceId, time()]);
        return $q === null ? null : $q['access_role'];
    }

    /**
     * Atomically authorize and reserve ONE Portal browser ticket. This is a
     * durable (SQLite) sliding-start 60-second rate window, independent of
     * PHP worker, session, device target and Browser ECDH key.
     *
     * Per user: 12 tickets / 60s across all devices.
     * Per device: 24 tickets / 60s across all Portal accounts.
     *
     * Authorization is rechecked INSIDE the same BEGIN IMMEDIATE transaction
     * before counting; a concurrent revoke/unlink cannot slip between the
     * permission lookup and quota reservation. No IP, raw username, password,
     * client nonce, Browser key, ticket or Relay ciphertext is stored here.
     *
     * These limits are a defense-in-depth control for the experimental
     * diagnostic, NOT a substitute for ingress IP limits or live revocation.
     */
    public function reserveRemoteTicket(int $userId, string $deviceId, ?int $now = null): string
    {
        if ($userId < 1 || preg_match('/^[a-f0-9]{64}$/D', $deviceId) !== 1) {
            throw new RuntimeException('Remote ticket denied');
        }
        $now ??= time();
        if ($now < 1) {
            throw new RuntimeException('Invalid quota clock');
        }
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $q = $this->queryOne(
                'SELECT ud.access_role FROM user_devices ud
                  JOIN users u ON u.id=ud.user_id
                  JOIN devices d ON d.device_id=ud.device_id
                  JOIN presence p ON p.device_id=d.device_id
                  WHERE ud.user_id=? AND ud.device_id=? AND u.enabled=1
                    AND d.enabled=1 AND p.expires_at>?',
                [$userId, $deviceId, $now]
            );
            if ($q === null) {
                throw new RuntimeException('Remote ticket denied');
            }
            $checks = [
                ['user', (string) $userId, 12],
                ['device', $deviceId, 24],
            ];
            $updates = [];
            foreach ($checks as [$scope, $key, $limit]) {
                $row = $this->queryOne(
                    'SELECT window_started,issued FROM remote_ticket_limits
                      WHERE scope=? AND scope_key=?', [$scope, $key]);
                if ($row === null || (int) $row['window_started'] <= $now - 60) {
                    $updates[] = [$scope, $key, $now, 1];
                } else {
                    // Fail closed if the clock is rolled back or a
                    // corrupted/future counter is encountered.
                    if ((int) $row['window_started'] > $now
                        || (int) $row['issued'] >= $limit) {
                        throw new RuntimeException('Remote ticket quota exceeded');
                    }
                    $updates[] = [$scope, $key, (int) $row['window_started'],
                        (int) $row['issued'] + 1];
                }
            }
            foreach ($updates as [$scope, $key, $window, $issued]) {
                $stmt = $this->db->prepare(
                    'INSERT INTO remote_ticket_limits(scope,scope_key,window_started,issued)
                      VALUES(?,?,?,?)
                      ON CONFLICT(scope,scope_key) DO UPDATE
                      SET window_started=excluded.window_started,issued=excluded.issued');
                $stmt->execute([$scope, $key, $window, $issued]);
            }
            // Expired quota rows are bounded by recent account/device
            // activity; pruning never changes a still-active 60s window.
            $this->db->prepare('DELETE FROM remote_ticket_limits WHERE window_started<?')
                ->execute([$now - 120]);
            $this->db->exec('COMMIT');
            return (string) $q['access_role'];
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    /**
     * Agent-origin signed authorization introspection for an already-issued
     * 30-second Portal browser ticket. This call is made *just before* the
     * experimental TaskDeck password check. An old signed ticket is NOT
     * sufficient after the Portal grant was removed or downgraded.
     *
     * Returns only a boolean, never usernames, password hashes, grants,
     * TaskDeck credentials or session material. The Agent must prove
     * possession of its local Ed25519 device key with a fresh timestamp
     * and one-time nonce. No anonymous user/device enumeration.
     */
    public function checkCurrentAgentGrant(array $payload): bool
    {
        foreach (['device_id','public_key','timestamp','nonce','signature',
            'portal_user_id','access_role','ticket_jti','portal_auth_epoch'] as $field) {
            if (!array_key_exists($field,$payload)) {
                throw new RuntimeException('Malformed grant check');
            }
        }
        $id=$payload['device_id'];
        $subject=$payload['portal_user_id'];
        $role=$payload['access_role'];
        $jti=$payload['ticket_jti'];
        $authEpoch=$payload['portal_auth_epoch'];
        if (!is_int($authEpoch) || $authEpoch < 1) {throw new RuntimeException('Invalid Portal auth generation');}
        if (!is_string($id) || preg_match('/^[a-f0-9]{64}$/D',$id)!==1 ||
            !is_string($subject) || preg_match('/^[1-9][0-9]{0,18}$/D',$subject)!==1 ||
            !is_string($role) || !in_array($role,['viewer','operator','admin'],true) ||
            !is_string($jti) || preg_match('/^[a-f0-9]{32}$/D',$jti)!==1) {
            throw new RuntimeException('Invalid grant check fields');
        }
        $this->verifySignedAgentMessage('grant-check',$payload,[$id,$subject,$role,$jti,(string)$authEpoch]);
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $this->prune();
            // A self-generated Ed25519 identity is NOT a paired device.
            // Reject unknown/disabled identities BEFORE writing one-time
            // nonce state, otherwise a caller can rotate public keys and
            // exhaust SQLite even without owning any TaskDeck installation.
            if (!$this->queryOne('SELECT 1 FROM devices WHERE device_id=? AND enabled=1', [$id])) {
                $this->db->exec('COMMIT');
                return false;
            }
            $current=$this->eligibleRemoteRole((int)$subject,$id);
            $owner=$this->userById((int)$subject);
            $order=['viewer'=>0,'operator'=>1,'admin'=>2];
            $allowed=$owner!==null && (int)$owner['auth_epoch']===$authEpoch &&
                $current!==null && isset($order[$current])
                && $order[$current]>=$order[$role];
            if ($allowed) {
                // Denied, revoked and epoch-stale requests must not grow the
                // nonce table. Even a compromised paired device identity is
                // bounded by the shared 90/min signed-proof quota.
                $this->reserveSignedDeviceProof($id);
                $this->consumeNonce($id,$payload['nonce']);
            }
            $this->db->exec('COMMIT');
            return $allowed;
        } catch (\Throwable $e) {
            try {$this->db->exec('ROLLBACK');}catch(\Throwable $ignored){}
            throw $e;
        }
    }

    public function grants(): array
    {
        return $this->db->query('SELECT ud.user_id,ud.device_id,ud.access_role,u.username,d.display_name
          FROM user_devices ud JOIN users u ON u.id=ud.user_id
          JOIN devices d ON d.device_id=ud.device_id ORDER BY d.display_name,u.username')->fetchAll();
    }

    public function grant(int $actor, int $user, string $deviceId, string $role): void
    {
        if (!in_array($role, ['viewer','operator','admin'], true)) {
            throw new RuntimeException('Invalid grant role');
        }
        $this->db->prepare('INSERT INTO user_devices(user_id,device_id,access_role) VALUES(?,?,?)
          ON CONFLICT(user_id,device_id) DO UPDATE SET access_role=excluded.access_role')
            ->execute([$user, $deviceId, $role]);
        $this->audit($actor, 'grant.' . $role, $deviceId);
    }

    /**
     * Permanently unlink this instance. Existing grants, presence and pairing
     * state are removed atomically. Reconnection requires a fresh local claim.
     * Live session revocation MUST be added before remote control is enabled.
     */
    public function removeDevice(int $actor, string $deviceId): void
    {
        if (!preg_match('/^[a-f0-9]{64}$/D', $deviceId)) {
            throw new RuntimeException('Invalid Device ID');
        }
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $q = $this->db->prepare('DELETE FROM devices WHERE device_id=?');
            $q->execute([$deviceId]);
            if ($q->rowCount() !== 1) {
                throw new RuntimeException('Device not found');
            }
            $this->db->prepare('DELETE FROM device_nonces WHERE device_id=?')->execute([$deviceId]);
            $this->db->prepare('DELETE FROM pending_enrollments WHERE device_id=?')->execute([$deviceId]);
            $this->audit($actor, 'device.unlink', $deviceId);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    public function revoke(int $actor, int $user, string $deviceId): void
    {
        $this->db->prepare('DELETE FROM user_devices WHERE user_id=? AND device_id=?')->execute([$user, $deviceId]);
        $this->audit($actor, 'revoke', $deviceId);
    }

    /** Signed ephemeral agent enrollment. Server never receives the plaintext pairing secret. */
    public function enroll(array $payload): void
    {
        $fields = ['device_id','public_key','display_name','pairing_hash','timestamp','nonce','signature'];
        foreach ($fields as $field) {
            if (!array_key_exists($field, $payload)) {
                throw new RuntimeException('Malformed enrollment');
            }
        }
        $id = $payload['device_id'];
        $name = $payload['display_name'];
        $pair = $payload['pairing_hash'];
        if (!is_string($name) || strlen($name) < 1 || strlen($name) > 80
            || preg_match('//u', $name) !== 1
            || preg_match('/[\x00-\x1F\x7F]/', $name) === 1
            || !is_string($pair) || !preg_match('/^[a-f0-9]{64}$/D', $pair)) {
            throw new RuntimeException('Invalid enrollment data');
        }
        $this->verifySignedAgentMessage('enroll', $payload, [$id, $name, $pair]);
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $this->prune();
            if ($this->queryOne('SELECT 1 FROM devices WHERE device_id=?', [$id])) {
                throw new RuntimeException('Device already registered; local approval required for rotation');
            }
            $count = (int) $this->db->query('SELECT COUNT(*) FROM pending_enrollments')->fetchColumn();
            if ($count >= 1000) {
                throw new RuntimeException('Enrollment temporarily unavailable');
            }
            $this->consumeNonce($id, $payload['nonce']);
            $this->db->prepare('INSERT INTO pending_enrollments(device_id,display_name,pairing_hash,expires_at)
              VALUES(?,?,?,?) ON CONFLICT(device_id) DO UPDATE SET display_name=excluded.display_name,
              pairing_hash=excluded.pairing_hash,expires_at=excluded.expires_at')
                ->execute([$id, $name, $pair, time() + 300]);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            // BEGIN IMMEDIATE was issued with exec(), not PDO::beginTransaction().
            // PDO::rollBack() cannot track it; rollback explicitly and preserve the original error.
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    /** Administrator claims a device by entering the secret shown locally on that device. */
    public function claim(int $actor, string $deviceId, string $pairingCode): void
    {
        if (!preg_match('/^[a-f0-9]{64}$/D', $deviceId)
            || !preg_match('/^[a-f0-9]{32}$/D', $pairingCode)) {
            throw new RuntimeException('Invalid Device ID or pairing code');
        }
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $p = $this->queryOne('SELECT * FROM pending_enrollments WHERE device_id=? AND expires_at>?',
                [$deviceId, time()]);
            if (!$p || !hash_equals($p['pairing_hash'], hash('sha256', hex2bin($pairingCode)))) {
                throw new RuntimeException('Pairing code invalid or expired');
            }
            $this->db->prepare('INSERT INTO devices(device_id,display_name,created_at) VALUES(?,?,?)')
                ->execute([$deviceId, $p['display_name'], time()]);
            $this->db->prepare('DELETE FROM pending_enrollments WHERE device_id=?')->execute([$deviceId]);
            $this->db->prepare('INSERT INTO user_devices(user_id,device_id,access_role) VALUES(?,?,"admin")')
                ->execute([$actor, $deviceId]);
            $this->audit($actor, 'device.claim', $deviceId);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            // BEGIN IMMEDIATE was issued with exec(), not PDO::beginTransaction().
            // PDO::rollBack() cannot track it; rollback explicitly and preserve the original error.
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    public function heartbeat(array $payload): void
    {
        foreach (['device_id','public_key','timestamp','nonce','signature'] as $field) {
            if (!array_key_exists($field, $payload)) {
                throw new RuntimeException('Malformed heartbeat');
            }
        }
        $id = $payload['device_id'];
        $this->verifySignedAgentMessage('heartbeat', $payload, [$id]);
        $this->db->exec('BEGIN IMMEDIATE');
        try {
            $this->prune();
            if (!$this->queryOne('SELECT 1 FROM devices WHERE device_id=? AND enabled=1', [$id])) {
                throw new RuntimeException('Unknown or disabled device');
            }
            $this->reserveSignedDeviceProof($id);
            $this->consumeNonce($id, $payload['nonce']);
            $this->db->prepare('INSERT INTO presence(device_id,expires_at) VALUES(?,?)
              ON CONFLICT(device_id) DO UPDATE SET expires_at=excluded.expires_at')
                ->execute([$id, time() + 60]);
            $this->db->exec('COMMIT');
        } catch (\Throwable $e) {
            // BEGIN IMMEDIATE was issued with exec(), not PDO::beginTransaction().
            // PDO::rollBack() cannot track it; rollback explicitly and preserve the original error.
            try { $this->db->exec('ROLLBACK'); } catch (\Throwable $ignored) {}
            throw $e;
        }
    }

    /**
     * Wire signature v1:
     * "taskdeck-remote-v1\n" + action + "\n" + each field + "\n"
     * + timestamp + "\n" + nonce.
     * Public key is supplied with each proof, checked against fingerprint ID.
     */
    private function verifySignedAgentMessage(string $action, array $p, array $parts): void
    {
        if (!extension_loaded('sodium')) {
            throw new RuntimeException('Sodium extension required');
        }
        $id = $p['device_id'];
        $ts = $p['timestamp'];
        $nonce = $p['nonce'];
        if (!is_string($id) || !preg_match('/^[a-f0-9]{64}$/D', $id)
            || !is_int($ts) || abs(time() - $ts) > 60
            || !is_string($nonce) || !preg_match('/^[a-f0-9]{32,64}$/D', $nonce)
            || !is_string($p['public_key']) || !is_string($p['signature'])) {
            throw new RuntimeException('Invalid device signature envelope');
        }
        $key = base64_decode($p['public_key'], true);
        $sig = base64_decode($p['signature'], true);
        if ($key === false || strlen($key) !== SODIUM_CRYPTO_SIGN_PUBLICKEYBYTES
            || $sig === false || strlen($sig) !== SODIUM_CRYPTO_SIGN_BYTES
            || !hash_equals($id, hash('sha256', $key))) {
            throw new RuntimeException('Device fingerprint mismatch');
        }
        $message = "taskdeck-remote-v1\n" . $action . "\n" . implode("\n", $parts)
            . "\n" . $ts . "\n" . $nonce;
        if (!sodium_crypto_sign_verify_detached($sig, $message, $key)) {
            throw new RuntimeException('Invalid device signature');
        }
    }

    /**
     * Device identity alone is insufficient to allocate unbounded nonce
     * records. Only paired, enabled devices call this helper, and it runs
     * under the same BEGIN IMMEDIATE transaction as replay protection.
     *
     * 90 accepted signed heartbeat/grant proofs per device per 60 seconds.
     * No IP or endpoint is stored. Existing devices already have FK rows,
     * and unpairing cascades removal of quota records.
     */
    private function reserveSignedDeviceProof(string $deviceId): void
    {
        $now = time();
        $row = $this->queryOne(
            'SELECT window_started,attempted FROM signed_agent_limits WHERE device_id=?',
            [$deviceId]
        );
        if ($row === null || (int)$row['window_started'] <= $now - 60) {
            $window = $now;
            $attempts = 1;
        } else {
            if ((int)$row['window_started'] > $now || (int)$row['attempted'] >= 90) {
                throw new RuntimeException('Signed device proof quota exceeded');
            }
            $window = (int)$row['window_started'];
            $attempts = (int)$row['attempted'] + 1;
        }
        $this->db->prepare(
            'INSERT INTO signed_agent_limits(device_id,window_started,attempted)
             VALUES(?,?,?) ON CONFLICT(device_id) DO UPDATE SET
             window_started=excluded.window_started,attempted=excluded.attempted'
        )->execute([$deviceId,$window,$attempts]);
    }

    private function consumeNonce(string $id, string $nonce): void
    {
        $q = $this->db->prepare('INSERT OR IGNORE INTO device_nonces(device_id,nonce,expires_at) VALUES(?,?,?)');
        $q->execute([$id, $nonce, time() + 180]);
        if ($q->rowCount() !== 1) {
            throw new RuntimeException('Replay detected');
        }
    }

    private function prune(): void
    {
        $now = time();
        $this->db->prepare('DELETE FROM device_nonces WHERE expires_at<?')->execute([$now]);
        $this->db->prepare('DELETE FROM pending_enrollments WHERE expires_at<?')->execute([$now]);
        $this->db->prepare('DELETE FROM presence WHERE expires_at<?')->execute([$now]);
    }

    private function audit(?int $actor, string $event, ?string $deviceId): void
    {
        $this->db->prepare('INSERT INTO audit_events(actor_id,event_type,device_id,created_at) VALUES(?,?,?,?)')
            ->execute([$actor, $event, $deviceId, time()]);
    }

    private function queryOne(string $sql, array $parameters): ?array
    {
        $q = $this->db->prepare($sql);
        $q->execute($parameters);
        $r = $q->fetch();
        return $r === false ? null : $r;
    }
}
