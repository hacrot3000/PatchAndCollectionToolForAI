<?php
declare(strict_types=1);
require __DIR__.'/../src/Store.php';
use TaskDeck\RemotePortal\Store;
function assertBrowser(bool $ok,string $desc): void {
    if (!$ok) throw new RuntimeException('FAIL: '.$desc);
    echo "PASS: ".$desc."\n";
}
if (($argv[1]??'')==='--worker') {
    try {
        $db=new Store((string)$argv[2]);
        $allowed=0;
        for ($n=0;$n<20;$n++) {
            if ($db->reserveIngress('browser','192.0.2.77',(int)$argv[3])) ++$allowed;
        }
        echo $allowed;
        exit(0);
    } catch(Throwable $e) {
        fwrite(STDERR,$e->getMessage()."\n");exit(2);
    }
}
$dir=sys_get_temp_dir().'/taskdeck-browser-budget-'.bin2hex(random_bytes(8));
if (!mkdir($dir,0700)) throw new RuntimeException('tmpdir failed');
$file=$dir.'/portal.sqlite3';
try {
    $db=new Store($file);$db->initialize();
    $clock=time();
    $workers=[];
    for ($w=0;$w<12;$w++) {
        $pipes=[];
        $p=proc_open([PHP_BINARY,__FILE__,'--worker',$file,(string)$clock],
            [0=>['pipe','r'],1=>['pipe','w'],2=>['pipe','w']],$pipes);
        if (!is_resource($p)) throw new RuntimeException('Cannot spawn browser quota worker');
        fclose($pipes[0]);$workers[]=[$p,$pipes];
    }
    $total=0;
    foreach ($workers as [$p,$pipes]) {
        $result=trim((string)stream_get_contents($pipes[1]));
        $error=stream_get_contents($pipes[2]);
        fclose($pipes[1]);fclose($pipes[2]);
        assertBrowser(proc_close($p)===0&&ctype_digit($result),
            'browser quota worker succeeded: '.$error);
        $total+=(int)$result;
    }
    assertBrowser($total===120,'12 PHP workers cannot allocate >120 browser page slots');
    $other=new Store($file);
    assertBrowser(!$other->reserveIngress('browser','192.0.2.77',$clock),
        'browser pre-session admission survives worker restart');
    assertBrowser($db->reserveIngress('browser','2001:db8::1111',$clock),
        'independent IPv6 client has a separate rate bucket');
    assertBrowser(!$db->reserveIngress('browser','192.0.2.77',$clock+59),
        'browser cap remains until 60-second window expires');
    assertBrowser($db->reserveIngress('browser','192.0.2.77',$clock+60),
        'browser cap renews after 60 seconds');
    $s=new PDO('sqlite:'.$file);
    $values=$s->query("SELECT scope_key FROM portal_browser_ingress_limits
        WHERE scope='source'")->fetchAll(PDO::FETCH_COLUMN);
    foreach ($values as $identity) {
        assertBrowser((bool)preg_match('/^[a-f0-9]{64}$/D',(string)$identity),
            'browser source stored as HMAC, never raw IP');
    }
    echo "All pre-session browser admission tests passed.\n";
} finally {
    unset($db,$other,$s);
    @unlink($file);
    @unlink($dir.'/portal-abuse-hmac.key');
    @unlink($dir.'/portal-mfa.key');
    @rmdir($dir);
}
