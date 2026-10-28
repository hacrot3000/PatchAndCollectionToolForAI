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
    check((int)$db->userById($viewer)['is_admin']===0,'viewer not admin');
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
    $list=$db->devicesFor($viewer,false);
    check(count($list)===1 && $list[0]['access_role']==='viewer','device-scoped grant');
    denied(fn()=>$db->grant($admin,$viewer,$id,'root'),'invalid role denied');
    $db->revoke($admin,$viewer,$id);
    check(count($db->devicesFor($viewer,false))===0,'revocation removes access');
    $sql=new PDO('sqlite:'.$file);
    $fields=$sql->query('PRAGMA table_info(devices)')->fetchAll(PDO::FETCH_ASSOC);
    check(implode(',',array_column($fields,'name'))==='device_id,display_name,enabled,created_at',
        'devices do not store endpoints or credentials');
    echo "All portal smoke tests passed.\n";
} finally {
    unset($db,$sql);
    @unlink($file);
    @rmdir($dir);
}
