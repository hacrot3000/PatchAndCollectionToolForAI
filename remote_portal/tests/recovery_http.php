<?php
declare(strict_types=1);

require __DIR__ . '/../src/Store.php';
use TaskDeck\RemotePortal\Store;

function recoveryCheck(bool $ok, string $label): void {
    if (!$ok) throw new RuntimeException('FAIL: '.$label);
    echo "PASS: ".$label."\n";
}

$dir = sys_get_temp_dir().'/taskdeck-offline-recovery-'.bin2hex(random_bytes(8));
$docroot = $dir.'/test-public';
if (!mkdir($dir,0700) || !mkdir($docroot,0700)) {
    throw new RuntimeException('Cannot make isolated recovery test directory');
}
$dbFile=$dir.'/portal.sqlite3';
$server=null;
$probeFile=$docroot.'/recovery-probe.php';
$serverLog=$dir.'/http.log';
try {
    $db=new Store($dbFile);
    $db->initialize();
    $admin=$db->createUser('recoveryguard','host-only-test-password-2026!',true);
    $sql=new PDO('sqlite:'.$dbFile,null,null,[PDO::ATTR_ERRMODE=>PDO::ERRMODE_EXCEPTION]);
    $sql->prepare('UPDATE users SET mfa_secret_enc=? WHERE id=?')
        ->execute(['fixture-only-enrolled-factor',$admin]);
    $before=$db->userById($admin);
    $storePath=realpath(__DIR__.'/../src/Store.php');
    if ($storePath===false) throw new RuntimeException('Missing Store.php');
    // This is a private, test-only PHP endpoint. It is never committed
    // into public/ and exists only for the lifetime of the isolated runner.
    $script='<?php declare(strict_types=1); require '.var_export($storePath,true).';
        try {
            $db=new \\TaskDeck\\RemotePortal\\Store(getenv("REMOTE_PORTAL_DB_PATH"));
            $db->resetTotpForRecovery('.(int)$admin.');
            http_response_code(500); echo "UNSAFE-RECOVERY-ALLOWED";
        } catch (\\RuntimeException $e) {
            http_response_code(403); echo "OFFLINE-CLI-ONLY";
        }';
    if (file_put_contents($probeFile,$script)===false) {
        throw new RuntimeException('Cannot write test-only recovery endpoint');
    }
    $env=array_merge($_ENV,['REMOTE_PORTAL_DB_PATH'=>$dbFile]);
    $pipes=[];
    $server=proc_open([PHP_BINARY,'-S','127.0.0.1:8089','-t',$docroot],
        [0=>['pipe','r'],1=>['file',$serverLog,'w'],2=>['file',$serverLog,'a']],
        $pipes,null,$env);
    if (!is_resource($server)) throw new RuntimeException('Cannot start test PHP HTTP server');
    fclose($pipes[0]);
    $status=0;$body='';
    for($i=0;$i<35;$i++){
        usleep(100000);
        $context=stream_context_create(['http'=>[
            'method'=>'GET','ignore_errors'=>true,'timeout'=>3]]);
        $response=@file_get_contents('http://127.0.0.1:8089/recovery-probe.php',false,$context);
        if (!is_string($response) || empty($http_response_header)) continue;
        $status=(int)(explode(' ',$http_response_header[0])[1]??0);
        $body=$response;
        break;
    }
    if($status===0){
        fwrite(STDERR,'PHP HTTP recovery fixture failed: '.(@file_get_contents($serverLog)?:'no log')."\n");
    }
    recoveryCheck($status===403 && $body==='OFFLINE-CLI-ONLY',
        'PHP cli-server cannot invoke Store offline TOTP recovery');
    $after=$db->userById($admin);
    recoveryCheck($before!==null && $after!==null &&
        $after['auth_epoch']===$before['auth_epoch'] &&
        $after['mfa_enabled']===$before['mfa_enabled'] &&
        (int)$after['mfa_enabled']===1,
        'failed HTTP recovery changes neither TOTP enrollment nor auth epoch');
    recoveryCheck((int)$sql->query("SELECT COUNT(*) FROM audit_events
        WHERE event_type='portal.mfa_reset_offline'")->fetchColumn()===0,
        'rejected HTTP TOTP reset writes no false recovery audit');
    echo "All HTTP offline-MFA-recovery guards passed.\n";
} finally {
    if(is_resource($server)){proc_terminate($server);proc_close($server);}
    unset($db,$sql);
    @unlink($probeFile);
    @unlink($serverLog);
    @unlink($dbFile);
    @unlink($dir.'/portal-mfa.key');
    @unlink($dir.'/portal-abuse-hmac.key');
    @rmdir($docroot);
    @rmdir($dir);
}
