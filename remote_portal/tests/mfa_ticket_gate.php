<?php
declare(strict_types=1);

require __DIR__.'/../src/Store.php';
use TaskDeck\RemotePortal\Store;

function ticketGateCheck(bool $ok,string $description): void {
    if (!$ok) throw new RuntimeException('FAIL: '.$description);
    echo "PASS: ".$description."\n";
}
function ticketGateRequest(string $path,?array $post=null): array {
    global $sessionCookie;
    $headers="Host: 127.0.0.1:8088\r\n";
    if ($sessionCookie!=='') $headers.="Cookie: taskdeck_remote_portal=".$sessionCookie."\r\n";
    if ($post!==null) $headers.="Content-Type: application/x-www-form-urlencoded\r\n";
    $context=stream_context_create(['http'=>[
        'method'=>$post===null?'GET':'POST',
        'header'=>$headers,
        'content'=>$post===null?'':http_build_query($post),
        'follow_location'=>0,
        'ignore_errors'=>true,
        'timeout'=>3,
    ]]);
    $body=@file_get_contents('http://127.0.0.1:8088'.$path,false,$context);
    if (!is_string($body)) throw new RuntimeException('Portal unavailable');
    $status=(int)(explode(' ',$http_response_header[0])[1]??0);
    foreach ($http_response_header as $header) {
        if (preg_match('/^Set-Cookie:\s*taskdeck_remote_portal=([^;]+)/i',$header,$match)) {
            $sessionCookie=$match[1];
        }
    }
    return [$status,$body];
}
function ticketCsrf(string $html): string {
    if (!preg_match('/name="csrf" value="([a-f0-9]{64})"/',$html,$match)) {
        throw new RuntimeException('CSRF token missing');
    }
    return $match[1];
}
$dir=sys_get_temp_dir().'/taskdeck-ticket-mfa-'.bin2hex(random_bytes(8));
if (!mkdir($dir,0700)) throw new RuntimeException('private fixture directory unavailable');
$dbPath=$dir.'/portal.sqlite3';
$sessionCookie='';
$server=null;
try {
    $store=new Store($dbPath);
    $store->initialize();
    $id=$store->createUser('nomfaticket','test-password-before-mfa-gate-12345!',false);
    $sessionDir=$store->privateBrowserSessionDirectory();
    $root=realpath(__DIR__.'/../public');
    $router=realpath(__DIR__.'/../bin/dev-router.php');
    // This is a legacy/bootstrap setting: it must NOT disable the remote
    // ticket issuance MFA requirement.
    $env=array_merge($_ENV,[
        'REMOTE_PORTAL_DB_PATH'=>$dbPath,
        'REMOTE_PORTAL_DEV_HTTP'=>'1',
        'REMOTE_PORTAL_REQUIRE_MFA'=>'0',
        'REMOTE_PORTAL_EXPERIMENTAL_TICKETS'=>'1',
    ]);
    $log=$dir.'/server.log';
    $pipes=[];
    $server=proc_open([PHP_BINARY,'-S','127.0.0.1:8088','-t',$root,$router],
        [0=>['pipe','r'],1=>['file',$log,'w'],2=>['file',$log,'a']],
        $pipes,null,$env);
    if (!is_resource($server)) throw new RuntimeException('Cannot start Portal fixture');
    fclose($pipes[0]);
    $ready=false;
    for ($attempt=0;$attempt<35;$attempt++) {
        usleep(100000);
        try { [$status,$page]=ticketGateRequest('/');$ready=$status===200; }
        catch (Throwable $e) {}
        if ($ready) break;
    }
    if (!$ready) {
        fwrite(STDERR,"Ticket MFA HTTP startup diagnostics:\n".(@file_get_contents($log)?:'none')."\n");
    }
    ticketGateCheck($ready,'legacy bootstrap Portal starts in test environment');
    $csrf=ticketCsrf($page);
    [$status,]=ticketGateRequest('/login',[
        'csrf'=>$csrf,'username'=>'nomfaticket',
        'password'=>'test-password-before-mfa-gate-12345!',
    ]);
    ticketGateCheck($status===303,'password-only Portal login permitted by explicit bootstrap override');
    [$status,$page]=ticketGateRequest('/');
    ticketGateCheck($status===200 && str_contains($page,'Máy TaskDeck được cấp quyền'),
        'bootstrap override permits ordinary Portal account pages without MFA');
    [$status,$error]=ticketGateRequest('/api/remote/ticket',[
        'csrf'=>ticketCsrf($page),
        'device_id'=>str_repeat('a',64),
        'client_nonce'=>str_repeat('0',32),
        'browser_key'=>'',
    ]);
    ticketGateCheck($status===403 && str_contains($error,'MFA'),
        'signed Relay ticket is blocked without MFA even with bootstrap override');
    $q=new PDO('sqlite:'.$dbPath);
    ticketGateCheck((int)$q->query('SELECT COUNT(*) FROM remote_ticket_limits')->fetchColumn()===0,
        'MFA preflight rejects ticket before consuming/issuing authorization');
    ticketGateCheck((int)$store->userById($id)['mfa_enabled']===0,
        'test account remains without MFA (no hidden enrollment)');
    echo "All remote-ticket independent MFA guard tests passed.\n";
} finally {
    if (is_resource($server)) { proc_terminate($server); proc_close($server); }
    unset($store,$q);
    @unlink($dbPath);
    @unlink($dir.'/portal-abuse-hmac.key');
    @unlink($dir.'/portal-mfa.key');
    @unlink($dir.'/server.log');
    if (isset($sessionDir)) {
        foreach (glob($sessionDir.'/sess_*')?:[] as $p) @unlink($p);
        @rmdir($sessionDir);
    }
    @rmdir($dir);
}
