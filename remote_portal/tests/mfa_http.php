<?php
declare(strict_types=1);
require __DIR__.'/../src/Store.php';
use TaskDeck\RemotePortal\Store;

function httpCheck(bool $yes,string $description): void {
    if (!$yes) throw new RuntimeException('FAIL: '.$description);
    echo "PASS: ".$description."\n";
}
function makeTOTP(string $secret): string {
    $alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';$buffer=0;$bits=0;$key='';
    foreach (str_split($secret) as $char) {
        $buffer=($buffer << 5)|strpos($alphabet,$char);$bits+=5;
        if ($bits>=8) {
            $bits-=8;$key.=chr(($buffer >> $bits)&255);
            $buffer&=(1 << $bits)-1;
        }
    }
    $step=intdiv(time(),30);
    $mac=hash_hmac('sha1',pack('N2',($step >> 32)&0xffffffff,$step&0xffffffff),$key,true);
    $offset=ord($mac[19])&15;
    $value=((ord($mac[$offset])&127)<<24)|(ord($mac[$offset+1])<<16)
        |(ord($mac[$offset+2])<<8)|ord($mac[$offset+3]);
    return str_pad((string)($value%1000000),6,'0',STR_PAD_LEFT);
}
function requestPortal(string $path,?array $post=null): array {
    global $cookie;
    $headers="Host: 127.0.0.1:8088\r\n";
    if ($cookie!=='') $headers.="Cookie: taskdeck_remote_portal=".$cookie."\r\n";
    if ($post!==null) $headers.="Content-Type: application/x-www-form-urlencoded\r\n";
    $ctx=stream_context_create(['http'=>[
        'method'=>$post===null?'GET':'POST',
        'header'=>$headers,
        'content'=>$post===null?'':http_build_query($post),
        'ignore_errors'=>true,
        'follow_location'=>0,
        'timeout'=>3
    ]]);
    $body=@file_get_contents('http://127.0.0.1:8088'.$path,false,$ctx);
    if (!is_string($body)) throw new RuntimeException('HTTP Portal unavailable');
    $status=(int)(explode(' ',$http_response_header[0])[1]??0);
    foreach ($http_response_header as $header) {
        if (preg_match('/^Set-Cookie:\s*taskdeck_remote_portal=([^;]+)/i',$header,$m)) {
            $cookie=$m[1];
        }
    }
    return [$status,$body];
}
function csrfToken(string $page): string {
    if (!preg_match('/name="csrf" value="([a-f0-9]{64})"/',$page,$m)) {
        throw new RuntimeException('Portal CSRF token missing');
    }
    return $m[1];
}
$dir=sys_get_temp_dir().'/taskdeck-mfa-http-'.bin2hex(random_bytes(8));
if (!mkdir($dir,0700)) throw new RuntimeException('private temp dir unavailable');
$dbFile=$dir.'/portal.sqlite3';
$cookie='';$server=null;
try {
    $store=new Store($dbFile);
    $store->initialize();
    $admin=$store->createUser('httpadmin','http-admin-long-test-password-123!',true);
    $root=realpath(__DIR__.'/../public');
    $router=realpath(__DIR__.'/../bin/dev-router.php');
    $serverEnv=array_merge($_ENV,[
        'REMOTE_PORTAL_DB_PATH'=>$dbFile,
        'REMOTE_PORTAL_DEV_HTTP'=>'1',
        'REMOTE_PORTAL_REQUIRE_MFA'=>'1',
    ]);
    $pipes=[];
    $server=proc_open([PHP_BINARY,'-S','127.0.0.1:8088','-t',$root,$router],
        [0=>['pipe','r'],1=>['file',$dir.'/server.log','w'],2=>['file',$dir.'/server.log','a']],
        $pipes,null,$serverEnv);
    if (!is_resource($server)) throw new RuntimeException('Cannot start local test Portal');
    fclose($pipes[0]);
    $ready=false;
    for ($i=0;$i<30;$i++) {
        usleep(100000);
        try { [$status,$html]=requestPortal('/');$ready=$status===200; }
        catch(Throwable $e) { /* server bootstrap */ }
        if ($ready) break;
    }
    if (!$ready) {
        fwrite(STDERR, "MFA HTTP local server startup failed; diagnostics: "
            .(@file_get_contents($dir.'/server.log') ?: '(no server log)')
            ."\n");
    }
    httpCheck($ready,'local Portal HTTP server started');
    $csrf=csrfToken($html);
    [$status,]=requestPortal('/login',['csrf'=>$csrf,
        'username'=>'httpadmin','password'=>'http-admin-long-test-password-123!']);
    httpCheck($status===303,'password-only bootstrap login succeeds for MFA enrollment');
    [$status,$html]=requestPortal('/');
    httpCheck($status===200 && str_contains($html,'Yêu cầu xác thực hai yếu tố'),
        'unenrolled session displays mandatory MFA enrollment');
    httpCheck(!str_contains($html,'Máy TaskDeck được cấp quyền')
        && !str_contains($html,'Cấp quyền theo máy'),
        'MFA-limited GET cannot disclose device list or admin grants');
    $csrf=csrfToken($html);
    foreach (['/users/create','/grants/set','/devices/remove',
        '/account/password','/api/remote/ticket'] as $path) {
        [$status,]=requestPortal($path,['csrf'=>$csrf]);
        httpCheck($status===403, 'MFA-limited POST rejects privileged endpoint '.$path);
    }
    [$status,]=requestPortal('/account/mfa/start',['csrf'=>$csrf,
        'current_password'=>'http-admin-long-test-password-123!']);
    httpCheck($status===303,'password verification starts MFA enrollment');
    [$status,$html]=requestPortal('/');
    httpCheck($status===200 && preg_match('/<code>([A-Z2-7]{32})<\/code>/',$html,$m)===1,
        'TOTP seed visible only in authenticated short-lived enrollment page');
    $code=makeTOTP($m[1]);
    [$status,]=requestPortal('/account/mfa/confirm',
        ['csrf'=>csrfToken($html),'totp_code'=>$code]);
    httpCheck($status===303,'authenticator possession confirms MFA enrollment');
    [$status,$html]=requestPortal('/');
    httpCheck($status===200 && str_contains($html,'Máy TaskDeck được cấp quyền')
        && str_contains($html,'Cấp quyền theo máy'),
        'after MFA confirmation admin UI becomes accessible');
    [$status,]=requestPortal('/logout',['csrf'=>csrfToken($html)]);
    httpCheck($status===303,'MFA-enabled Portal user logs out');
    [$status,$html]=requestPortal('/');
    [$status,]=requestPortal('/login',['csrf'=>csrfToken($html),
        'username'=>'httpadmin','password'=>'http-admin-long-test-password-123!']);
    httpCheck($status===303,'password-only login receives generic redirect');
    [$status,$html]=requestPortal('/');
    httpCheck($status===200 && str_contains($html,'Đăng nhập quản lý')
        && !str_contains($html,'Cấp quyền theo máy'),
        'TOTP-enabled account cannot log back in with password alone');
    httpCheck((int)$store->userById($admin)['mfa_enabled']===1,
        'real HTTP MFA session committed encrypted factor into account');

    echo "All Portal HTTP MFA access-gate tests passed.\n";
} finally {
    if (is_resource($server)) {
        proc_terminate($server);
        proc_close($server);
    }
    unset($store);
    @unlink($dbFile);
    @unlink($dir.'/portal-abuse-hmac.key');
    @unlink($dir.'/portal-mfa.key');
    @unlink($dir.'/server.log');
    @rmdir($dir);
}
