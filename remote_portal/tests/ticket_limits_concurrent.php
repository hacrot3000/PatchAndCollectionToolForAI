<?php
declare(strict_types=1);
require __DIR__ . '/../src/Store.php';
use TaskDeck\RemotePortal\Store;

// Called by CI only: launch independent PHP processes against one real
// SQLite file to catch non-atomic quota reservation across PHP workers.
if (($argv[1] ?? '') === '--worker') {
    [$unused,$action,$dbPath,$userId,$device,$clock,$attempts] = $argv;
    try {
        $store = new Store($dbPath);
        $allowed = 0;
        for ($i=0; $i<(int)$attempts; $i++) {
            try {
                $store->reserveRemoteTicket((int)$userId, $device, (int)$clock);
                $allowed++;
            } catch (RuntimeException $e) {
                if ($e->getMessage() !== 'Remote ticket quota exceeded') {
                    throw $e;
                }
            }
        }
        echo (string)$allowed;
        exit(0);
    } catch (Throwable $e) {
        fwrite(STDERR, 'Quota worker failed: '.$e->getMessage()."\n");
        exit(2);
    }
}

function assertConcurrent(bool $condition,string $description): void {
    if (!$condition) throw new RuntimeException('FAIL: '.$description);
    echo 'PASS: '.$description."\n";
}

/** @param list<array{user:int,attempts:int}> $spec */
function concurrentReservations(string $file,string $device,int $clock,array $spec): int {
    $workers=[];
    foreach ($spec as $worker) {
        $cmd=[PHP_BINARY,__FILE__,'--worker',$file,(string)$worker['user'],
            $device,(string)$clock,(string)$worker['attempts']];
        $pipes=[];
        $process=proc_open($cmd,[0=>['pipe','r'],1=>['pipe','w'],2=>['pipe','w']],$pipes);
        if (!is_resource($process)) {
            throw new RuntimeException('Failed to launch independent PHP quota worker');
        }
        fclose($pipes[0]);
        $workers[]=[$process,$pipes];
    }
    $accepted=0;
    foreach ($workers as [$process,$pipes]) {
        $stdout=stream_get_contents($pipes[1]);
        $stderr=stream_get_contents($pipes[2]);
        fclose($pipes[1]);
        fclose($pipes[2]);
        $exit=proc_close($process);
        if ($exit!==0 || !ctype_digit(trim((string)$stdout))) {
            throw new RuntimeException('Concurrent PHP quota worker failed: '.$stderr);
        }
        $accepted+=(int)trim($stdout);
    }
    return $accepted;
}

$folder=sys_get_temp_dir().'/taskdeck-quota-concurrent-'.bin2hex(random_bytes(8));
if (!mkdir($folder,0700)) throw new RuntimeException('No private temporary directory');
$file=$folder.'/portal.sqlite3';
try {
    $store=new Store($file);
    $store->initialize();
    $id=str_repeat('a',64);
    $now=time();
    $users=[];
    for ($i=1;$i<=4;$i++) {
        $users[]=$store->createUser('parallel'.$i,'long-test-password-'.$i.'-2026!',false);
    }
    // Test fixture owns its private SQLite DB; create one claimed, healthy
    // device with four Portal grants. No TaskDeck passwords are involved.
    $sql=new PDO('sqlite:'.$file,null,null,[PDO::ATTR_ERRMODE=>PDO::ERRMODE_EXCEPTION]);
    $sql->prepare('INSERT INTO devices(device_id,display_name,created_at) VALUES(?,?,?)')
        ->execute([$id,'Parallel QA Device',$now]);
    $sql->prepare('INSERT INTO presence(device_id,expires_at) VALUES(?,?)')
        ->execute([$id,$now+300]);
    foreach ($users as $user) {
        $sql->prepare('INSERT INTO user_devices(user_id,device_id,access_role) VALUES(?,?,"viewer")')
            ->execute([$user,$id]);
    }

    // Four unrelated PHP processes all compete for the same Portal user:
    // exactly 12 reservations can succeed despite 32 signed-ticket attempts.
    $burst=concurrentReservations($file,$id,$now,array_fill(0,4,
        ['user'=>$users[0],'attempts'=>8]));
    assertConcurrent($burst===12,'per-user ticket cap is atomic across four PHP workers');
    $count=(int)$sql->query("SELECT issued FROM remote_ticket_limits
        WHERE scope='user' AND scope_key='".(string)$users[0]."'")->fetchColumn();
    assertConcurrent($count===12,'cross-process SQLite user counter persisted exactly');

    // Two fresh users compete for the remaining device capacity: 24 total
    // for the device even though every user has unused capacity.
    $other=concurrentReservations($file,$id,$now,[
        ['user'=>$users[1],'attempts'=>12],
        ['user'=>$users[2],'attempts'=>12],
        ['user'=>$users[3],'attempts'=>12],
    ]);
    assertConcurrent($other===12,'per-device quota applies across distinct Portal accounts');
    $deviceCount=(int)$sql->query("SELECT issued FROM remote_ticket_limits
      WHERE scope='device' AND scope_key='".$id."'")->fetchColumn();
    assertConcurrent($deviceCount===24,'device capacity cannot exceed 24 tickets in same window');
    echo "All concurrent Portal ticket quota tests passed.\n";
} finally {
    unset($store,$sql);
    @unlink($file);
    @unlink($folder.'/portal-abuse-hmac.key');
    @rmdir($folder);
}
