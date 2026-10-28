<?php
declare(strict_types=1);

namespace TaskDeck\RemotePortal;

use PDO;
use RuntimeException;

/**
 * Control-plane storage only. It MUST NOT store TaskDeck credentials,
 * addresses, endpoints, remote session tokens or terminal/file content.
 */
final class Store
{
    private PDO $db;

    public function __construct(string $path)
    {
        if ($path !== ':memory:') {
            if ($path === '' || $path[0] !== '/') {
                throw new RuntimeException('REMOTE_PORTAL_DB_PATH must be absolute and outside public webroot');
            }
            $dir = realpath(dirname($path));
            $public = realpath(__DIR__ . '/../public');
            if ($dir === false || $public === false
                || $dir === $public || str_starts_with($dir . '/', $public . '/')) {
                throw new RuntimeException('Invalid DB directory: create a private directory outside webroot');
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
CREATE TABLE IF NOT EXISTS presence (
  device_id TEXT PRIMARY KEY REFERENCES devices(device_id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS login_attempts (
  username TEXT PRIMARY KEY COLLATE NOCASE,
  failures INTEGER NOT NULL,
  locked_until INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS remote_ticket_limits (
  scope TEXT NOT NULL CHECK(scope IN ('user','device')),
  scope_key TEXT NOT NULL,
  window_started INTEGER NOT NULL,
  issued INTEGER NOT NULL CHECK(issued >= 0),
  PRIMARY KEY(scope, scope_key)
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
SQL);
        // Existing installations retain all user IDs, hashes and grants.
        $columns = array_column($this->db->query('PRAGMA table_info(users)')->fetchAll(), 'name');
        if (!in_array('auth_epoch', $columns, true)) {
            $this->db->exec('ALTER TABLE users ADD COLUMN auth_epoch INTEGER NOT NULL DEFAULT 1');
        }
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

    public function authenticate(string $username, string $password): ?array
    {
        $username = strtolower(trim($username));
        if (!preg_match('/^[a-z][a-z0-9_.-]{2,40}$/D', $username) || strlen($password) > 2048) {
            return null;
        }
        $now = time();
        $lock = $this->queryOne('SELECT failures,locked_until FROM login_attempts WHERE username=?', [$username]);
        if ($lock && (int) $lock['failures'] >= 5 && (int) $lock['locked_until'] > $now) {
            return null;
        }
        $user = $this->queryOne('SELECT * FROM users WHERE username=? AND enabled=1', [$username]);
        $ok = $user !== null && password_verify($password, $user['password_hash']);
        if (!$ok) {
            // Sliding short failure window. Avoid permanent lockout after TTL expiry.
            $failures = $lock && (int) $lock['locked_until'] > $now
                ? (int) $lock['failures'] + 1 : 1;
            // Temporary username-based throttle. Edge-level rate limiting is also required.
            $until = $failures >= 5 ? $now + 120 : $now + 180;
            $this->db->prepare('INSERT INTO login_attempts(username,failures,locked_until) VALUES(?,?,?)
              ON CONFLICT(username) DO UPDATE SET failures=excluded.failures,locked_until=excluded.locked_until')
                ->execute([$username, $failures, $until]);
            return null;
        }
        $this->db->prepare('DELETE FROM login_attempts WHERE username=?')->execute([$username]);
        return ['id' => (int) $user['id'], 'username' => $user['username'],
            'is_admin' => (bool) $user['is_admin'], 'auth_epoch' => (int) $user['auth_epoch']];
    }

    public function userById(int $id): ?array
    {
        $r = $this->queryOne('SELECT id,username,is_admin,enabled,auth_epoch FROM users WHERE id=?', [$id]);
        return $r && (int) $r['enabled'] === 1 ? $r : null;
    }

    /**
     * Changing a Portal password invalidates all other Portal browser sessions.
     * No TaskDeck authentication material is read or stored here.
     * The caller must regenerate their PHP session ID and adopt auth_epoch.
     */
    public function changePassword(int $userId, string $oldPassword, string $newPassword): int
    {
        if ($userId < 1 || strlen($oldPassword) > 2048
            || strlen($newPassword) < 14 || strlen($newPassword) > 1024
            || hash_equals($oldPassword, $newPassword)) {
            throw new RuntimeException('Password policy check failed');
        }
        $user = $this->queryOne(
            'SELECT password_hash FROM users WHERE id=? AND enabled=1', [$userId]);
        if ($user === null || !password_verify($oldPassword, $user['password_hash'])) {
            throw new RuntimeException('Current password invalid');
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
