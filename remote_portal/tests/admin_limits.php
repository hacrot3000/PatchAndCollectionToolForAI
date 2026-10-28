<?php
declare(strict_types=1);
require __DIR__.'/../src/Store.php';
use TaskDeck\RemotePortal\Store;

function adminAssert(bool $valid, string $message): void {
    if (!$valid) throw new RuntimeException('FAIL: '.$message);
    echo 'PASS: '.$message."\n";
}
function adminDeny(callable $action, string $message): void {
    try { $action(); } catch (RuntimeException $e) {
        echo 'PASS (denied): '.$message."\n";
        return;
    }
    throw new RuntimeException('FAIL (allowed): '.$message);
}
if (($argv[1] ?? '') === '--reserve-worker') {
    $db=new Store((string)$argv[2]);
    $count=0;
    for ($i=0;$i<20;$i++) {
        try {
            $db->reserveAdminMutation((int)$argv[3],(int)$argv[4]);
            ++$count;
        } catch (RuntimeException $e) {
            if ($e->getMessage()!=='Admin mutation quota exceeded') {
                fwrite(STDERR,$e->getMessage());
                exit(2);
            }
        }
    }
    echo $count;
    exit(0);
}
$dir=sys_get_temp_dir().'/taskdeck-admin-throttle-'.bin2hex(random_bytes(8));
if (!mkdir($dir,0700)) throw new RuntimeException('private test dir unavailable');
$file=$dir.'/portal.sqlite3';
try {
    $db=new Store($file);
    $db->initialize();
    $admin=$db->createUser('mutadmin','admin-throttle-valid-password-1234!',true);
    $regular=$db->createUser('mutuser','regular-throttle-valid-password-1234!',false);
    $another=new Store($file);
    $now=time();
    adminDeny(fn()=>$db->reserveAdminMutation($regular,$now),
        'non-admin cannot reserve a privileged write');
    adminDeny(fn()=>$another->reserveAdminMutation(999999,$now),
        'unknown Portal user cannot reserve a privileged write');
    $workers=[];
    for ($i=0;$i<4;$i++) {
        $pipes=[];
        $process=proc_open([PHP_BINARY,__FILE__,'--reserve-worker',$file,
            (string)$admin,(string)$now],
            [0=>['pipe','r'],1=>['pipe','w'],2=>['pipe','w']],$pipes);
        if (!is_resource($process)) throw new RuntimeException('worker startup failed');
        fclose($pipes[0]);
        $workers[]=[$process,$pipes];
    }
    $accepted=0;
    foreach ($workers as [$process,$pipes]) {
        $stdout=stream_get_contents($pipes[1]);
        $stderr=stream_get_contents($pipes[2]);
        fclose($pipes[1]);fclose($pipes[2]);
        $code=proc_close($process);
        adminAssert($code===0 && ctype_digit(trim((string)$stdout)),
            'separate PHP worker finished without failure: '.$stderr);
        $accepted+=(int)trim($stdout);
    }
    adminAssert($accepted===60,'80 concurrent admin writes receive exactly 60 slots');
    $q=new PDO('sqlite:'.$file,null,null,[PDO::ATTR_ERRMODE=>PDO::ERRMODE_EXCEPTION]);
    $row=$q->query('SELECT attempted FROM portal_admin_limits WHERE user_id='.$admin)
        ->fetchColumn();
    adminAssert((int)$row===60,'quota persists in SQLite across processes');
    adminDeny(fn()=>$another->reserveAdminMutation($admin,$now+59),
        'privileged action blocked before 60 seconds expire');
    $db->reserveAdminMutation($admin,$now+60);
    $row=$q->query('SELECT attempted FROM portal_admin_limits WHERE user_id='.$admin)
        ->fetchColumn();
    adminAssert((int)$row===1,'new 60-second window permits one privileged write');
    adminDeny(fn()=>$another->reserveAdminMutation($admin,$now+59),
        'clock rollback cannot escape admin quota');
    $q->exec('UPDATE users SET is_admin=0 WHERE id='.$admin);
    adminDeny(fn()=>$db->reserveAdminMutation($admin,$now+61),
        'revoking admin flag denies already-logged-in session before writes');
    $q->exec('UPDATE users SET is_admin=1,enabled=0 WHERE id='.$admin);
    adminDeny(fn()=>$another->reserveAdminMutation($admin,$now+61),
        'disabled Portal account cannot mutate grants');
    $q->exec('UPDATE users SET enabled=1 WHERE id='.$admin);
    $db->reserveAdminMutation($admin,$now+61);
    adminAssert(true,'enabled admin can resume within remaining quota');
    $q->exec('DROP TABLE portal_admin_limits');
    adminDeny(fn()=>$db->reserveAdminMutation($admin,$now+61),
        'missing migration table fails closed instead of permitting a write');
    $db->initialize();
    $db->reserveAdminMutation($admin,$now+61);
    adminAssert(true,'idempotent CLI-style migration restores privileged quota table');
    echo "All Portal admin mutation quota tests passed.\n";
} finally {
    unset($db,$another,$q);
    @unlink($file);
    @unlink($dir.'/portal-mfa.key');
    @unlink($dir.'/portal-abuse-hmac.key');
    @rmdir($dir);
}
