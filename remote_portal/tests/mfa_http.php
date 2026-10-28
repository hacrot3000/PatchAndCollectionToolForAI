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
    return [$status,$body,$http_response_header];
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
$sessionDir=$dir.'/browser-sessions-'
    .substr(hash('sha256',realpath($dir).'/portal.sqlite3'),0,32);
// Do not create the directory in the test: the Portal must provision it
// securely, rather than trusting the hosting PHP session.save_path.
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
    $server=proc_open([PHP_BINARY,'-d','session.save_path='.sys_get_temp_dir(),
        '-S','127.0.0.1:8088','-t',$root,$router],
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
    [$status,$html,$responseHeaders]=requestPortal('/');
    $securityHeaders=implode("\n",$responseHeaders);
    foreach ([
        'Cross-Origin-Opener-Policy: same-origin',
        'Cross-Origin-Resource-Policy: same-origin',
        'Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()',
        'X-Frame-Options: DENY',
        'Cache-Control: no-store',
    ] as $expectedHeader) {
        httpCheck(str_contains(strtolower($securityHeaders),strtolower($expectedHeader)),
            'Portal response includes security policy: '.$expectedHeader);
    }
    // PHP built-in router must serve only the audited browser E2EE
    // assets as static resources. Unknown/private paths stay in PHP's
    // explicit 404 handler instead of bypassing app authorization.
    foreach ([
        '/remote_crypto.js' => 'taskdeck-e2ee-p256-v1',
        '/remote_probe.js' => 'reachability diagnostic',
    ] as $asset=>$expectedMarker) {
        [$assetStatus,$assetBody]=requestPortal($asset);
        httpCheck($assetStatus===200 && str_contains($assetBody,$expectedMarker),
            'allowlisted browser asset is served by loopback development router: '.$asset);
    }
    [$loginScriptStatus,]=requestPortal('/remote_login.js');
    httpCheck($loginScriptStatus===404,
        'PHP router never serves former TaskDeck-password JS');
    [$privateStatus,]=requestPortal('/src/Store.php');
    httpCheck($privateStatus===404,
        'PHP dev-router denies attempts to browse private Portal Store source');
    [$unlistedStatus,]=requestPortal('/not-allowlisted.js');
    httpCheck($unlistedStatus===404,
        'PHP dev-router does not serve unknown JavaScript paths');
    $anonymousSessionCookie=$cookie;
    clearstatcache(true,$sessionDir);
    httpCheck(is_dir($sessionDir) && (fileperms($sessionDir)&0077)===0,
        'live HTTP Portal creates private 0700 session directory beside SQLite');
    $sessionFiles=glob($sessionDir.'/sess_*')?:[];
    httpCheck(count($sessionFiles)>0, 'browser state written into dedicated private session storage');
    foreach ($sessionFiles as $sessionFile) {
        clearstatcache(true,$sessionFile);
        httpCheck(is_file($sessionFile) && !is_link($sessionFile)
            && (fileperms($sessionFile)&0077)===0,
            'HTTP session file is owner-only (0600)');
    }
    $originalCookie=$cookie;
    $before=count(glob($sessionDir.'/sess_*') ?: []);
    for ($attempt=0; $attempt<30; $attempt++) {
        // An unknown URL must never open/create a new PHP browser session.
        $cookie='';
        [$status,]=requestPortal('/definitely-not-a-Portal-route-'.$attempt);
        httpCheck($status===404,'unknown Portal GET path rejected before session allocation '.$attempt);
    }
    $cookie=$originalCookie;
    httpCheck(count(glob($sessionDir.'/sess_*') ?: [])===$before,
        'thirty anonymous invalid GET paths allocate no PHP session files');
    $csrf=csrfToken($html);
    [$status,]=requestPortal('/login',['csrf'=>$csrf,
        'username'=>'httpadmin','password'=>'http-admin-long-test-password-123!']);
    httpCheck($status===303,'password-only bootstrap login succeeds for MFA enrollment');
    httpCheck($cookie!=='' && $cookie!==$anonymousSessionCookie,
        'successful password login rotates the anonymous PHP session ID');
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
    $preMfaSessionCookie=$cookie;
    [$status,]=requestPortal('/account/mfa/confirm',
        ['csrf'=>csrfToken($html),'totp_code'=>$code]);
    httpCheck($status===303,'authenticator possession confirms MFA enrollment');
    httpCheck($cookie!=='' && $cookie!==$preMfaSessionCookie,
        'MFA activation rotates the authenticated PHP session ID');
    [$status,$html]=requestPortal('/');
    httpCheck($status===200 && str_contains($html,'Máy TaskDeck được cấp quyền')
        && str_contains($html,'Cấp quyền theo máy'),
        'after MFA confirmation admin UI becomes accessible');
    // This real PHP cli-server process is NOT PHP_SAPI=cli, so a web
    // request can only create accounts via the explicit admin+TOTP Store
    // transaction. The old bootstrap method is denied in HTTP runtimes.
    [$status,]=requestPortal('/users/create',[
        'csrf'=>csrfToken($html),
        'username'=>'webcreated',
        'password'=>'web-created-mfa-validated-password-1234!',
    ]);
    httpCheck($status===303,
        'MFA-enrolled admin creates an ordinary account through HTTP protected Store path');
    $created=array_values(array_filter($store->allUsers(),
        static fn(array $row): bool => $row['username']==='webcreated'));
    httpCheck(count($created)===1 && (int)$created[0]['is_admin']===0,
        'HTTP account creation cannot create a second Portal administrator');
    $auditSql=new PDO('sqlite:'.$dbFile);
    httpCheck((int)$auditSql->query("SELECT COUNT(*) FROM audit_events
        WHERE event_type='portal.user_create' AND actor_id=".(int)$admin)->fetchColumn()===1,
        'HTTP account creation persists an actor-attributed audit entry');
    unset($auditSql);
    [$status,$html]=requestPortal('/');
    $preLogoutSessionCookie=$cookie;
    [$status,]=requestPortal('/logout',['csrf'=>csrfToken($html)]);
    httpCheck($status===303,'MFA-enabled Portal user logs out');
    httpCheck($cookie!=='' && $cookie!==$preLogoutSessionCookie,
        'logout invalidates the previous PHP session ID');
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

    // Even malformed enroll POSTs must consume the low anonymous ingress
    // budget BEFORE JSON decoding. Otherwise attackers can flood JSON
    // validation without ever reaching signature / pairing checks.
    for ($attempt=0;$attempt<12;$attempt++) {
        [$status,]=requestPortal('/api/agent/enroll',['not_json'=>'1']);
        httpCheck($status===415,'invalid-content enrollment counted before JSON parse '.$attempt);
    }
    [$status,]=requestPortal('/api/agent/enroll',['not_json'=>'1']);
    httpCheck($status===429,'13th malformed enrollment denied by pre-JSON ingress quota');

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
    foreach (glob($sessionDir.'/sess_*') ?: [] as $sessionFile) @unlink($sessionFile);
    @rmdir($sessionDir);
    @rmdir($dir);
}
