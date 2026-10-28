<?php
declare(strict_types=1);

require __DIR__ . '/../src/Store.php';
use TaskDeck\RemotePortal\Store;

function assertProtected(bool $condition, string $description): void {
    if (!$condition) throw new RuntimeException('FAIL: '.$description);
    echo "PASS: ".$description."\n";
}
function denyAction(callable $fn, string $description): void {
    try { $fn(); } catch (Throwable $e) {
        echo "PASS (denied): ".$description."\n";
        return;
    }
    throw new RuntimeException('FAIL (allowed): '.$description);
}

// Each worker has a fresh PHP process/SQLite connection. All workers guess
// the same real account simultaneously; no account-specific lock can be
// bypassed by exploiting the time between reading and hashing a password.
if (($argv[1] ?? '') === '--login-worker') {
    try {
        $db = new Store((string)$argv[2]);
        $db->authenticate('protected', 'intentionally-wrong-password');
        exit(0);
    } catch (Throwable $e) {
        fwrite(STDERR, "worker failure: ".$e->getMessage()."\n");
        exit(2);
    }
}
// Standalone concurrent ingress workers share SQLite's durable rate bucket.
if (($argv[1] ?? '') === '--ingress-worker') {
    try {
        $db = new Store((string)$argv[2]);
        $success=0;
        for ($n=0; $n<(int)$argv[5]; $n++) {
            if ($db->reserveIngress('login',(string)$argv[3],(int)$argv[4])) ++$success;
        }
        echo $success;
        exit(0);
    } catch (Throwable $e) {
        fwrite(STDERR, 'ingress worker failed: '.$e->getMessage()."\\n");
        exit(2);
    }
}
$folder = sys_get_temp_dir().'/taskdeck-bruteforce-'.bin2hex(random_bytes(8));
if (!mkdir($folder, 0700)) throw new RuntimeException('Could not create test directory');
$file = $folder.'/portal.sqlite3';
try {
    $store = new Store($file);
    $store->initialize();
    $admin = $store->createUser('protected', 'correct-password-for-test-2026!', true);
    $other = new Store($file);
    $now = time();

    $src1 = '192.0.2.10';
    $src2 = '2001:db8::abcd';
    for ($i=0;$i<10;$i++) {
        assertProtected($store->reserveIngress('login',$src1,$now),
            'first IP reservation '.$i);
    }
    assertProtected(!$other->reserveIngress('login',$src1,$now),
        'source login cap survives PHP/SQLite worker reload');
    assertProtected($other->reserveIngress('login',$src2,$now),
        'independent source has a separate quota');
    assertProtected(!$other->reserveIngress('login',$src1,$now+59),
        'source rate quota cannot refill prematurely');
    denyAction(fn()=>$store->reserveIngress('login','not-an-ip',$now),
        'untrusted/malformed forwarding header not accepted as source');
    denyAction(fn()=>$store->reserveIngress('admin',$src1,$now),
        'unrecognized action cannot reserve quota');

    // Enrollment has a separate budget; an attacker cannot grow an
    // unlimited number of pending enrollments by rotating device identities.
    for ($i=0;$i<12;$i++) {
        assertProtected($store->reserveIngress('enroll',$src2,$now),
            'source enrollment reservation '.$i);
    }
    assertProtected(!$store->reserveIngress('enroll',$src2,$now),
        'enrollment attempts capped before expensive Ed25519 checks');

    // Simulate many unrelated source addresses. Even with different HMACs,
    // the site-wide cap bounds memory and DB cardinality per minute.
    $allowed = 0;
    for ($i=0;$i<250;$i++) {
        $ip = '198.51.'.intdiv($i,256).'.'.($i%256);
        if ($other->reserveIngress('login',$ip,$now)) ++$allowed;
    }
    // 10 from source 1 and 1 from source 2 already reserved.
    assertProtected($allowed===169, 'global login quota is 180 attempts/minute');
    assertProtected($other->reserveIngress('login',$src1,$now+60),
        'source and global rate quotas refresh after 60 seconds');
    assertProtected(!$store->reserveIngress('login',$src1,$now+59),
        'rollback of the clock cannot bypass refreshed quota');
    $q = new PDO('sqlite:'.$file);
    $total=(int)$q->query("SELECT COUNT(*) FROM portal_ingress_limits WHERE action='login' AND scope='source'")->fetchColumn();
    assertProtected($total<=171, 'global quota bounds stored keyed source records');
    $keys=$q->query("SELECT scope_key FROM portal_ingress_limits WHERE scope='source'")->fetchAll(PDO::FETCH_COLUMN);
    foreach ($keys as $k) {
        assertProtected((bool)preg_match('/^[a-f0-9]{64}$/D',(string)$k),
            'source ID stored only as HMAC-SHA256');
    }
    $raw=file_get_contents($file);
    assertProtected(is_string($raw) && !str_contains($raw,$src1) && !str_contains($raw,$src2),
        'SQLite does not contain raw IPv4/IPv6 source addresses');
    // 32-byte HMAC key must be separate from DB with owner-only mode.
    $rateKey=$folder.'/portal-abuse-hmac.key';
    clearstatcache(true,$rateKey);
    assertProtected(is_file($rateKey) && filesize($rateKey)===32 &&
        (fileperms($rateKey)&0077)===0,'HMAC secret stored separately in 0600 file');

    // Four independent PHP processes race for the same fresh source bucket.
    // The SQL transaction must accept exactly ten out of forty requests.
    $admissionWorkers=[];
    for ($worker=0;$worker<4;$worker++) {
        $pipes=[];
        $p=proc_open([PHP_BINARY,__FILE__,'--ingress-worker',$file,
            '203.0.113.37',(string)($now+60),'10'],
            [0=>['pipe','r'],1=>['pipe','w'],2=>['pipe','w']],$pipes);
        if (!is_resource($p)) throw new RuntimeException('Cannot start ingress worker');
        fclose($pipes[0]);
        $admissionWorkers[]=[$p,$pipes];
    }
    $accepted=0;
    foreach ($admissionWorkers as [$p,$pipes]) {
        $value=stream_get_contents($pipes[1]);
        $error=stream_get_contents($pipes[2]);
        fclose($pipes[1]);
        fclose($pipes[2]);
        $rc=proc_close($p);
        assertProtected($rc===0 && ctype_digit(trim((string)$value)),
            'ingress worker completed without error: '.$error);
        $accepted+=(int)trim($value);
    }
    assertProtected($accepted===10,
        'four independent PHP workers cannot bypass per-source 10/minute quota');

    // A concurrently guessed real account may pay for at most five expensive
    // password verifications before the lock is reserved. Start 24 separate
    // workers to expose a read-then-hash-update race.
    $workers=[];
    for ($i=0;$i<24;$i++) {
        $pipes=[];
        $p=proc_open([PHP_BINARY,__FILE__,'--login-worker',$file],
            [0=>['pipe','r'],1=>['pipe','w'],2=>['pipe','w']],$pipes);
        if (!is_resource($p)) throw new RuntimeException('Failed spawning PHP worker');
        fclose($pipes[0]);
        $workers[]=[$p,$pipes];
    }
    foreach ($workers as [$p,$pipes]) {
        $out=stream_get_contents($pipes[1]);
        $err=stream_get_contents($pipes[2]);
        fclose($pipes[1]);
        fclose($pipes[2]);
        assertProtected(proc_close($p)===0,
            'parallel worker rejected or reserved without exception: '.$err.$out);
    }
    $row=$q->query("SELECT failures,locked_until FROM login_attempts WHERE username='protected'")->fetch(PDO::FETCH_ASSOC);
    assertProtected($row!==false && (int)$row['failures']===5 &&
        (int)$row['locked_until']>time(),'atomic account lock caps 24 concurrent guesses at five');
    assertProtected($store->authenticate('protected','correct-password-for-test-2026!')===null,
        'even a correct password cannot bypass active account lock');
    $q->exec("UPDATE login_attempts SET locked_until=1 WHERE username='protected'");
    $logged=$store->authenticate('protected','correct-password-for-test-2026!');
    assertProtected($logged!==null && $logged['id']===$admin,
        'account automatically recovers after lock expiry');
    assertProtected((int)$q->query("SELECT COUNT(*) FROM login_attempts")->fetchColumn()===0,
        'successful login clears the account failure counter');

    echo "All Portal brute-force security tests passed.\n";
} finally {
    unset($store,$other,$q);
    @unlink($file);
    @unlink($folder.'/portal-abuse-hmac.key');
    @rmdir($folder);
}
