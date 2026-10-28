<?php
declare(strict_types=1);

require __DIR__ . '/../src/Store.php';
use TaskDeck\RemotePortal\Store;

if (!extension_loaded('pdo_sqlite') || !extension_loaded('sodium')) {
    fwrite(STDERR, "pdo_sqlite and sodium required\n");
    exit(2);
}
function okProof(bool $condition, string $description): void {
    if (!$condition) throw new RuntimeException('FAIL: '.$description);
    echo "PASS: ".$description."\n";
}
function deniedProof(callable $action, string $description): void {
    try { $action(); } catch (Throwable $e) {
        echo "PASS (denied): ".$description."\n";
        return;
    }
    throw new RuntimeException('FAIL (allowed): '.$description);
}
function signedProof(string $action, array $parts, string $deviceId, string $public, string $secret): array {
    $time = time();
    $nonce = bin2hex(random_bytes(16));
    $wire = "taskdeck-remote-v1\n".$action."\n".implode("\n", $parts)."\n".$time."\n".$nonce;
    return [
        'device_id' => $deviceId,
        'public_key' => base64_encode($public),
        'timestamp' => $time,
        'nonce' => $nonce,
        'signature' => base64_encode(sodium_crypto_sign_detached($wire, $secret)),
    ];
}

$dir = sys_get_temp_dir().'/taskdeck-device-proof-'.bin2hex(random_bytes(8));
if (!mkdir($dir, 0700)) throw new RuntimeException('Private directory unavailable');
$file = $dir.'/portal.sqlite3';
try {
    $store = new Store($file);
    $store->initialize();
    $other = new Store($file);
    $admin = $store->createUser('prooftest', 'proof-test-password-long-enough-123!', true);
    // Store now independently enforces enrolled admin MFA, not only HTTP.
    $mfaOnly=new PDO('sqlite:'.$file);
    $mfaOnly->prepare('UPDATE users SET mfa_secret_enc=? WHERE id=?')
        ->execute(['fixture-only-enrolled-mfa',$admin]);
    unset($mfaOnly);
    $keys = sodium_crypto_sign_keypair();
    $pk = sodium_crypto_sign_publickey($keys);
    $sk = sodium_crypto_sign_secretkey($keys);
    $id = hash('sha256', $pk);
    $pairCode = bin2hex(random_bytes(16));
    $pairHash = hash('sha256', hex2bin($pairCode));
    $display = 'Proof quota fixture';
    $store->enroll(signedProof('enroll', [$id,$display,$pairHash],$id,$pk,$sk)
        + ['display_name'=>$display, 'pairing_hash'=>$pairHash]);
    $store->claim($admin,$id,$pairCode);
    $sql = new PDO('sqlite:'.$file, null, null, [PDO::ATTR_ERRMODE=>PDO::ERRMODE_EXCEPTION]);
    $grant = function() use ($id,$pk,$sk,$admin): array {
        $role = 'admin';
        $jti = bin2hex(random_bytes(16));
        return signedProof('grant-check',[$id,(string)$admin,$role,$jti,'1'],$id,$pk,$sk)
            + ['portal_user_id'=>(string)$admin,'access_role'=>$role,
                'ticket_jti'=>$jti,'portal_auth_epoch'=>1];
    };
    // A paired device is not authorized for grants until its first
    // signed heartbeat establishes unexpired Portal presence.
    $store->heartbeat(signedProof('heartbeat', [$id],$id,$pk,$sk));
    okProof($store->checkCurrentAgentGrant($grant()),'paired online signed grant authorizes once');
    for ($attempt=2; $attempt<90; $attempt++) {
        $heartbeat = signedProof('heartbeat', [$id],$id,$pk,$sk);
        $other->heartbeat($heartbeat);
    }
    $q = $sql->prepare('SELECT attempted FROM signed_agent_limits WHERE device_id=?');
    $q->execute([$id]);
    okProof((int)$q->fetchColumn()===90,
        'paired heartbeat and grant share 90/minute cross-worker capacity');
    $q->closeCursor(); // Release SQLite reader before writers on other connections.
    $nonceBefore=(int)$sql->query('SELECT COUNT(*) FROM device_nonces')->fetchColumn();
    deniedProof(fn()=>$store->heartbeat(signedProof('heartbeat',[$id],$id,$pk,$sk)),
        '91st signed heartbeat rejected before allocating replay nonce');
    deniedProof(fn()=>$other->checkCurrentAgentGrant($grant()),
        '91st signed grant proof cannot bypass device quota');
    okProof((int)$sql->query('SELECT COUNT(*) FROM device_nonces')->fetchColumn()===$nonceBefore,
        'denied signed Agent proofs do not grow the nonce table');

    $store->revoke($admin,$admin,$id);
    okProof(!$other->checkCurrentAgentGrant($grant()),
        'revoked grant denied before quota or replay storage');
    okProof((int)$sql->query('SELECT COUNT(*) FROM device_nonces')->fetchColumn()===$nonceBefore,
        'revoked grant check does not allocate nonce rows');
    $store->grant($admin,$admin,$id,'admin');

    // Simulate a naturally expired proof window while retaining the same
    // paired identity and already-consumed replay nonces.
    $reset = $sql->prepare('UPDATE signed_agent_limits SET window_started=? WHERE device_id=?');
    $reset->execute([time()-61,$id]);
    $store->heartbeat(signedProof('heartbeat',[$id],$id,$pk,$sk));
    $q->execute([$id]);
    okProof((int)$q->fetchColumn()===1,'proof window renews after 60 seconds');
    $q->closeCursor();

    $store->removeDevice($admin,$id);
    $q->execute([$id]);
    okProof($q->fetchColumn()===false,
        'unlink cascades removal of paired identity quota row');
    $q->closeCursor();
    deniedProof(fn()=>$other->heartbeat(signedProof('heartbeat',[$id],$id,$pk,$sk)),
        'unpaired device heartbeat denied before nonce allocation');

    echo "All signed Agent proof quota tests passed.\n";
} finally {
    unset($sql,$store,$other);
    @unlink($file);
    @unlink($dir.'/portal-mfa.key');
    @unlink($dir.'/portal-abuse-hmac.key');
    @rmdir($dir);
}
