<?php
declare(strict_types=1);
require __DIR__.'/../src/Store.php';
use TaskDeck\RemotePortal\Store;
function assertSensitive(bool $ok,string $label): void {
    if (!$ok) throw new RuntimeException('FAIL: '.$label);
    echo "PASS: ".$label."\n";
}
function mustDeny(callable $cb,string $label): void {
    try { $cb(); } catch(Throwable $e) {
        echo "PASS (denied): ".$label."\n";
        return;
    }
    throw new RuntimeException('FAIL (allowed): '.$label);
}
if (($argv[1]??'')==='--reserve') {
    try {
        $db=new Store((string)$argv[2]);
        $ok=0;
        for ($i=0;$i<8;$i++) {
            try { $db->reserveSensitiveAttempt((int)$argv[3]); ++$ok; }
            catch(RuntimeException $e) {
                if ($e->getMessage()!=='Sensitive action quota exceeded') throw $e;
            }
        }
        echo $ok;
        exit(0);
    } catch(Throwable $e) {
        fwrite(STDERR,$e->getMessage()."\n");
        exit(2);
    }
}
$dir=sys_get_temp_dir().'/taskdeck-sensitive-'.bin2hex(random_bytes(8));
if (!mkdir($dir,0700)) throw new RuntimeException('mkdir failed');
$file=$dir.'/portal.sqlite3';
try {
    $db=new Store($file);
    $db->initialize();
    $user=$db->createUser('sensitive', 'some-test-portal-password-123!', false);
    $other=$db->createUser('second', 'second-test-portal-password-123!', false);
    $ps=[];
    for ($n=0;$n<6;$n++) {
        $pipes=[];
        $p=proc_open([PHP_BINARY,__FILE__,'--reserve',$file,(string)$user],
            [0=>['pipe','r'],1=>['pipe','w'],2=>['pipe','w']],$pipes);
        if (!is_resource($p)) throw new RuntimeException('worker launch failed');
        fclose($pipes[0]);$ps[]=[$p,$pipes];
    }
    $accepted=0;
    foreach ($ps as [$p,$pipes]) {
        $count=trim((string)stream_get_contents($pipes[1]));
        $err=stream_get_contents($pipes[2]);
        fclose($pipes[1]);fclose($pipes[2]);
        assertSensitive(proc_close($p)===0&&ctype_digit($count),
            'each independent PHP worker completed: '.$err);
        $accepted+=(int)$count;
    }
    assertSensitive($accepted===5,'48 concurrent reservations receive only five slots');
    mustDeny(fn()=>$db->changePassword($user,'some-test-portal-password-123!','changed-portal-password-123!'),
        'even correct current password cannot skip exhausted quota');
    assertSensitive($db->authenticate('sensitive','some-test-portal-password-123!')!==null,
        'sensitive-action quota does not lock normal Portal login');
    assertSensitive($db->reserveSensitiveAttempt($other,time())===null,
        'another account has an independent sensitive action budget');
    $q=new PDO('sqlite:'.$file,null,null,[PDO::ATTR_ERRMODE=>PDO::ERRMODE_EXCEPTION]);
    $q->prepare('UPDATE account_sensitive_limits SET window_started=? WHERE user_id=?')
        ->execute([time()-601,$user]);
    mustDeny(fn()=>$db->changePassword($user,'wrong-old-password',
        'changed-portal-password-123!'),'wrong current password still denied');
    $count=$q->prepare('SELECT attempted FROM account_sensitive_limits WHERE user_id=?');
    $count->execute([$user]);
    assertSensitive((int)$count->fetchColumn()===1,
        'password-change verifier reserves attempt before comparing password');
    $count->closeCursor();
    $newEpoch=$db->changePassword($user,'some-test-portal-password-123!',
        'changed-portal-password-123!');
    assertSensitive($newEpoch===2,'correct password change succeeds under available quota');
    $count->execute([$user]);
    assertSensitive((int)$count->fetchColumn()===2,
        'successful password change also consumes sensitive action quota');
    $count->closeCursor();
    $q->prepare('UPDATE account_sensitive_limits SET window_started=? WHERE user_id=?')
        ->execute([time()+3600,$user]);
    mustDeny(fn()=>$db->reserveSensitiveAttempt($user),
        'clock rollback or future quota timestamp fails closed');
    echo "All sensitive-action budget tests passed.\n";
} finally {
    unset($db,$q);
    @unlink($file);
    @unlink($dir.'/portal-abuse-hmac.key');
    @rmdir($dir);
}
