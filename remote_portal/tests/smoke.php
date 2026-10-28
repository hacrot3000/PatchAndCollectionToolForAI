<?php
declare(strict_types=1);
require __DIR__ . '/../src/Store.php';
use TaskDeck\RemotePortal\Store;

if (!extension_loaded('pdo_sqlite') || !extension_loaded('sodium')) {
    fwrite(STDERR, "pdo_sqlite and sodium are required\n");
    exit(2);
}
function check(bool $ok, string $name): void {
    if (!$ok) throw new RuntimeException("FAIL: $name");
    echo "PASS: $name\n";
}
function denied(callable $fn, string $name): void {
    try { $fn(); }
    catch (Throwable $e) { echo "PASS (denied): $name\n"; return; }
    throw new RuntimeException("FAIL (allowed): $name");
}
function wire(string $action, array $parts, string $id, string $pk, string $sk): array {
    $ts=time(); $nonce=bin2hex(random_bytes(16));
    $msg="taskdeck-remote-v1\n".$action."\n".implode("\n",$parts)."\n".$ts."\n".$nonce;
    return [
        'device_id'=>$id,
        'public_key'=>base64_encode($pk),
        'timestamp'=>$ts,
        'nonce'=>$nonce,
        'signature'=>base64_encode(sodium_crypto_sign_detached($msg,$sk)),
    ];
}
$dir=sys_get_temp_dir().'/taskdeck-portal-test-'.bin2hex(random_bytes(8));
mkdir($dir,0700);
$file=$dir.'/portal.sqlite3';
try {
    $db=new Store($file);
    $db->initialize();
    $db->initialize();
    $admin=$db->createUser('admin','correct-horse-battery-staple-123!',true);
    $viewer=$db->createUser('viewer','another-long-password-1234!',false);
    denied(fn()=>$db->createUser('weak','short',false),'reject weak password');
    check($db->authenticate('admin','correct-horse-battery-staple-123!')['id']===$admin,'admin login');
    check($db->authenticate('admin','wrong')===null,'wrong password denied');
    // Distinct unknown usernames must not create unbounded rows in the
    // shared-host SQLite database through the unauthenticated login form.
    for ($n=0; $n<60; $n++) {
        check($db->authenticate('unknown'.$n,'invalid-password')===null,
            'unknown username is denied without persistent failure row '.$n);
    }
    $loginDb=new PDO('sqlite:'.$file);
    check((int)$loginDb->query('SELECT COUNT(*) FROM login_attempts')->fetchColumn()===1,
        'unknown usernames never persist Portal login counters');
    // An upgrade must also remove old anonymous rows, preserving the
    // legitimate admin account's failed-attempt counter.
    $loginDb->exec("INSERT INTO login_attempts(username,failures,locked_until)
        VALUES('unknown_legacy',2,100)");
    unset($loginDb);
    $db->initialize();
    $loginDb=new PDO('sqlite:'.$file);
    check((int)$loginDb->query('SELECT COUNT(*) FROM login_attempts')->fetchColumn()===1,
        'schema init clears legacy unknown-account counters');
    unset($loginDb);
    check((int)$db->userById($viewer)['is_admin']===0,'viewer not admin');
    $oldPortalSession=$db->authenticate('viewer','another-long-password-1234!');
    check((int)$oldPortalSession['auth_epoch']===1,'initial Portal session generation');
    denied(fn()=>$db->changePassword($viewer,'wrong-current-password','new-portal-password-123456!'),
        'password rotation requires current password');
    denied(fn()=>$db->changePassword($viewer,'another-long-password-1234!','short'),
        'password rotation rejects weak replacement');
    $nextEpoch=$db->changePassword($viewer,'another-long-password-1234!','new-portal-password-123456!');
    check($nextEpoch===2 && $nextEpoch!==(int)$oldPortalSession['auth_epoch'],
        'password rotation invalidates old browser session generation');
    check($db->authenticate('viewer','another-long-password-1234!')===null,
        'old Portal password rejected after rotation');
    $newPortalSession=$db->authenticate('viewer','new-portal-password-123456!');
    check($newPortalSession!==null && $newPortalSession['auth_epoch']===2,
        'new Portal password authenticates with rotated session generation');
    denied(fn()=>$db->changePassword($viewer,'new-portal-password-123456!','new-portal-password-123456!'),
        'password rotation refuses identical replacement');
    $kp=sodium_crypto_sign_keypair();
    $pk=sodium_crypto_sign_publickey($kp);
    $sk=sodium_crypto_sign_secretkey($kp);
    $id=hash('sha256',$pk);
    $code=bin2hex(random_bytes(16));
    $name='Laptop QA / Project B';
    $pairHash=hash('sha256',hex2bin($code));
    $enroll=wire('enroll',[$id,$name,$pairHash],$id,$pk,$sk)+['display_name'=>$name,'pairing_hash'=>$pairHash];
    $db->enroll($enroll);
    check(count($db->devicesFor($admin,true))===0,'unclaimed device not visible');
    denied(fn()=>$db->enroll($enroll),'enrollment replay denied');
    denied(fn()=>$db->claim($admin,$id,str_repeat('0',32)),'wrong pairing code denied');
    $kp2=sodium_crypto_sign_keypair();
    $fake=wire('heartbeat',[$id],$id,sodium_crypto_sign_publickey($kp2),sodium_crypto_sign_secretkey($kp2));
    denied(fn()=>$db->heartbeat($fake),'fingerprint substitution denied');
    $db->claim($admin,$id,$code);
    check(count($db->devicesFor($admin,true))===1,'local pairing proof registers device');
    denied(fn()=>$db->claim($admin,$id,$code),'duplicate claim denied');
    denied(fn()=>$db->enroll(wire('enroll',[$id,$name,$pairHash],$id,$pk,$sk)+
        ['display_name'=>$name,'pairing_hash'=>$pairHash]),'cannot replace paired identity');
    $heartbeat=wire('heartbeat',[$id],$id,$pk,$sk);
    $db->heartbeat($heartbeat);
    check((int)$db->devicesFor($admin,true)[0]['online']===1,'signed heartbeat sets online');
    denied(fn()=>$db->heartbeat($heartbeat),'heartbeat replay denied');
    check(count($db->devicesFor($viewer,false))===0,'viewer cannot list ungranted devices');
    $db->grant($admin,$viewer,$id,'viewer');
    $jti=bin2hex(random_bytes(16));
    $viewerEpoch=(int)$db->userById($viewer)['auth_epoch'];
    $agentCheck=fn(int $user,string $role,?int $epoch=null): array =>
        wire('grant-check',[$id,(string)$user,$role,$jti,(string)($epoch??$viewerEpoch)],$id,$pk,$sk)
        + ['portal_user_id'=>(string)$user,'access_role'=>$role,'ticket_jti'=>$jti,
            'portal_auth_epoch'=>$epoch??$viewerEpoch];
    $signedCheck=$agentCheck($viewer,'viewer');
    check($db->checkCurrentAgentGrant($signedCheck),
        'fresh signed Agent grant check authorizes current Portal viewer');
    check(!$db->checkCurrentAgentGrant($agentCheck($viewer,'viewer',1)),
        'old password-auth epoch cannot authorize a current Portal device grant');
    denied(fn()=>$db->checkCurrentAgentGrant($signedCheck),
        'Agent grant proof nonce is single-use');
    $forgedRole=$agentCheck($viewer,'admin');
    $forgedRole['access_role']='viewer';
    denied(fn()=>$db->checkCurrentAgentGrant($forgedRole),
        'Agent signed grant check cannot swap role after signature');
    $signedAdmin=$agentCheck($viewer,'admin');
    check(!$db->checkCurrentAgentGrant($signedAdmin),
        'Portal viewer cannot reuse an older administrator-role ticket');
    check(!$db->checkCurrentAgentGrant($agentCheck($viewer+99999,'viewer')),
        'Portal admin without explicit device grant has no implicit access');
    check($db->eligibleRemoteRole($viewer,$id)==='viewer', 'viewer role eligible for signed connection ticket');
    // Durable authorization+rate limits under one BEGIN IMMEDIATE.
    // Multiple PHP/Apache workers point to the same SQLite file, rather
    // than each having a private in-memory counter.
    $otherWorker = new Store($file);
    $base = time();
    check($db->reserveRemoteTicket($viewer,$id,$base)==='viewer',
        'Portal ticket reservation checks authoritative grant');
    for ($attempt=1; $attempt<12; $attempt++) {
        check($otherWorker->reserveRemoteTicket($viewer,$id,$base)==='viewer',
            'Portal user ticket quota permits request '.($attempt+1));
    }
    denied(fn()=>$db->reserveRemoteTicket($viewer,$id,$base),
        '13th user ticket denied across PHP worker connections');
    denied(fn()=>$otherWorker->reserveRemoteTicket($viewer,$id,$base-1),
        'clock rollback cannot reset ticket quota');
    // Admin has a separate per-user budget, but both share device quota.
    for ($attempt=0; $attempt<12; $attempt++) {
        check($db->reserveRemoteTicket($admin,$id,$base)==='admin',
            'device ticket quota shared across users '.($attempt+13));
    }
    $third = $db->createUser('third','valid-third-user-password-123!',false);
    $db->grant($admin,$third,$id,'viewer');
    denied(fn()=>$otherWorker->reserveRemoteTicket($third,$id,$base),
        '25th ticket denied by device quota regardless of Portal username');
    // The quota window expires at exactly 60 seconds. Simulate a healthy
    // device heartbeat at the future test timestamp without sleeping.
    $clockDb = new PDO('sqlite:'.$file);
    $clockDb->prepare('UPDATE presence SET expires_at=? WHERE device_id=?')
        ->execute([$base+180,$id]);
    denied(fn()=>$db->reserveRemoteTicket($viewer,$id,$base+59),
        'rate window remains closed at 59 seconds');
    check($db->reserveRemoteTicket($viewer,$id,$base+60)==='viewer',
        'quota window resets after full 60 seconds');
    $db->revoke($admin,$viewer,$id);
    denied(fn()=>$otherWorker->reserveRemoteTicket($viewer,$id,$base+61),
        'revoked role cannot reserve a signed ticket');
    $db->grant($admin,$viewer,$id,'viewer');
    check($otherWorker->reserveRemoteTicket($viewer,$id,$base+61)==='viewer',
        'restored grant permits newly authorized ticket');
    unset($otherWorker,$clockDb);

    check($db->eligibleRemoteRole($viewer,str_repeat('1',64))===null, 'wrong device not eligible');
    $list=$db->devicesFor($viewer,false);
    check(count($list)===1 && $list[0]['access_role']==='viewer','device-scoped grant');
    denied(fn()=>$db->grant($admin,$viewer,$id,'root'),'invalid role denied');
    $db->revoke($admin,$viewer,$id);
    check(!$db->checkCurrentAgentGrant($agentCheck($viewer,'viewer')),
        'revoked Portal user grant is rejected even with an already issued ticket');
    check(count($db->devicesFor($viewer,false))===0,'revocation removes access');
    check($db->eligibleRemoteRole($viewer,$id)===null, 'revoked grant blocks ticket');
    $db->removeDevice($admin,$id);
    check(count($db->devicesFor($admin,true))===0,'device unlink removes device');
    denied(fn()=>$db->heartbeat(wire('heartbeat',[$id],$id,$pk,$sk)),
        'unlinked device heartbeat denied');
    $sql=new PDO('sqlite:'.$file);
    $fields=$sql->query('PRAGMA table_info(devices)')->fetchAll(PDO::FETCH_ASSOC);
    check(implode(',',array_column($fields,'name'))==='device_id,display_name,enabled,created_at',
        'devices do not store endpoints or credentials');
    echo "All portal smoke tests passed.\n";
} finally {
    unset($db,$sql);
    @unlink($file);
    @unlink($dir.'/portal-abuse-hmac.key');
    @rmdir($dir);
}
