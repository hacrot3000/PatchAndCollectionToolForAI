<?php
declare(strict_types=1);
require __DIR__.'/../src/Store.php';
use TaskDeck\RemotePortal\Store;
use TaskDeck\RemotePortal\Totp;
function checkMfa(bool $ok,string $desc): void {
    if (!$ok) throw new RuntimeException('FAIL: '.$desc);
    echo "PASS: ".$desc."\n";
}
function makeOtp(string $base32,int $now): string {
    $alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
    $raw='';$bits=0;$buf=0;
    foreach (str_split($base32) as $c) {
        $buf=($buf << 5)|strpos($alphabet,$c);
        $bits+=5;
        if ($bits>=8) {
            $bits-=8;$raw.=chr(($buf >> $bits)&255);
            $buf&=(1 << $bits)-1;
        }
    }
    $step=intdiv($now,30);
    $mac=hash_hmac('sha1',pack('N2',($step >> 32)&0xffffffff,$step&0xffffffff),$raw,true);
    $offset=ord($mac[19])&15;
    $binary=((ord($mac[$offset])&127)<<24)|(ord($mac[$offset+1])<<16)
        |(ord($mac[$offset+2])<<8)|ord($mac[$offset+3]);
    return str_pad((string)($binary%1000000),6,'0',STR_PAD_LEFT);
}
function mustFailMfa(callable $fn,string $desc): void {
    try { $fn(); } catch(Throwable $e) {
        echo "PASS (denied): ".$desc."\n";
        return;
    }
    throw new RuntimeException('FAIL (allowed): '.$desc);
}
$rfc='GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';
checkMfa(Totp::match($rfc,'287082',59)===1,'RFC6238 59s SHA1/6d known answer');
checkMfa(Totp::match($rfc,'081804',1111111109)===37037036,
    'RFC6238 2005 SHA1/6d known answer');
checkMfa(Totp::match($rfc,'050471',1111111111)===37037037,
    'RFC6238 2005 next-time SHA1/6d known answer');
checkMfa(Totp::match($rfc,'287082',59,1)===null,'one-time step cannot replay');
checkMfa(Totp::match($rfc,'000000',59)===null,'wrong TOTP denied');
checkMfa(Totp::match($rfc,'2870827',59)===null,'wrong digit count denied');
checkMfa(str_starts_with(Totp::provisioningUri('alice',$rfc),'otpauth://totp/'),
    'standard authenticator app provisioning URI');
$dir=sys_get_temp_dir().'/taskdeck-mfa-'.bin2hex(random_bytes(8));
if (!mkdir($dir,0700)) throw new RuntimeException('private test directory unavailable');
$file=$dir.'/portal.sqlite3';
try {
    $db=new Store($file);
    $db->initialize();
    $userId=$db->createUser('mfauser','mfa-user-original-long-password!',true);
    checkMfa(!$db->userById($userId)['mfa_enabled'],'unenrolled user starts without MFA');
    checkMfa($db->verifyPasswordForMfaEnrollment($userId,
        'mfa-user-original-long-password!'),'setup re-verifies account password');
    checkMfa(!$db->verifyPasswordForMfaEnrollment($userId,'wrong'),
        'setup rejects wrong current password');
    $secret=Totp::newSecret();
    $code=makeOtp($secret,time());
    mustFailMfa(fn()=>$db->activateTotp($userId,$secret,'00000x'),
        'cannot activate MFA without authenticator proof');
    $oldEpoch=(int)$db->userById($userId)['auth_epoch'];
    $nextEpoch=$db->activateTotp($userId,$secret,$code);
    checkMfa($nextEpoch===$oldEpoch+1 && (int)$db->userById($userId)['mfa_enabled']===1,
        'first MFA enrollment invalidates existing PHP sessions');
    $sqlite=new PDO('sqlite:'.$file);
    $cipher=(string)$sqlite->query('SELECT mfa_secret_enc FROM users WHERE id='.$userId)->fetchColumn();
    checkMfa($cipher!=='' && !str_contains($cipher,$secret) &&
        !str_contains((string)file_get_contents($file),$secret),
        'TOTP secret is encrypted before storing in Portal SQLite');
    $key=$dir.'/portal-mfa.key';
    clearstatcache(true,$key);
    checkMfa(is_file($key)&&filesize($key)===32&&(fileperms($key)&0077)===0,
        'independent 256-bit MFA encryption key is private');
    checkMfa($db->authenticate('mfauser','mfa-user-original-long-password!')===null,
        'password-only authentication is denied once MFA enrolled');
    checkMfa($db->authenticate('mfauser','wrong',$code)===null,
        'known TOTP alone cannot authenticate without account password');
    // Enrollment consumed current timestep. Advance it within the database
    // test fixture to emulate the next 30s without sleeping in CI.
    $sqlite->exec('UPDATE users SET mfa_last_step=-1 WHERE id='.$userId);
    $logged=$db->authenticate('mfauser','mfa-user-original-long-password!',$code);
    checkMfa($logged!==null && $logged['auth_epoch']===$nextEpoch,
        'password plus one valid MFA code authenticates');
    checkMfa($db->authenticate('mfauser','mfa-user-original-long-password!',$code)===null,
        'TOTP code cannot be replayed for a second session');
    $wrongKey=str_repeat("\x11",32);
    $originalKey=file_get_contents($key);
    // Crypto key loss or substitution must fail authentication, not skip MFA.
    unlink($key);
    mustFailMfa(fn()=>$db->authenticate('mfauser','mfa-user-original-long-password!',$code),
        'missing Portal TOTP master key fails closed');
    file_put_contents($key,$wrongKey);chmod($key,0600);
    mustFailMfa(fn()=>$db->authenticate('mfauser','mfa-user-original-long-password!',$code),
        'wrong 256-bit master key cannot decrypt enrolled MFA');
    unlink($key);file_put_contents($key,$originalKey);chmod($key,0600);
    $db->initialize();
    checkMfa(hash_equals($originalKey,file_get_contents($key)),
        'idempotent schema migration keeps MFA master key unchanged');
    $q=new PDO('sqlite:'.$file);
    $columns=array_column($q->query('PRAGMA table_info(users)')->fetchAll(PDO::FETCH_ASSOC),'name');
    checkMfa(in_array('mfa_secret_enc',$columns,true)&&in_array('mfa_last_step',$columns,true),
        'MFA columns exist after migration');
    echo "All Portal MFA regression tests passed.\n";
} finally {
    unset($db,$sqlite,$q);
    @unlink($file);
    @unlink($dir.'/portal-mfa.key');
    @unlink($dir.'/portal-abuse-hmac.key');
    @rmdir($dir);
}
