<?php
declare(strict_types=1);
require __DIR__ . '/../src/TransportPolicy.php';

use TaskDeck\RemotePortal\TransportPolicy;

function assertPolicy(bool $condition, string $description): void {
    if (!$condition) throw new RuntimeException('FAIL: '.$description);
    echo "PASS: ".$description."\n";
}
foreach (['127.0.0.1','::1'] as $loopback) {
    assertPolicy(TransportPolicy::permitsDevelopmentHttp('cli-server',true,
        $loopback,'localhost:8088'), 'explicit loopback development server may use HTTP');
}
foreach (['fpm-fcgi','apache2handler','cgi-fcgi','cli'] as $sapi) {
    // Reverse proxies frequently connect to shared PHP using loopback; the
    // Host header may be controlled by an external client. Neither is proof
    // that the request originated from a trusted local operator.
    assertPolicy(!TransportPolicy::permitsDevelopmentHttp($sapi,true,
        '127.0.0.1','localhost:8088'),
        'shared-host or CLI runtime never accepts spoofed local Host: '.$sapi);
}
foreach ([
    ['cli-server',false,'127.0.0.1','localhost:8088'],
    ['cli-server',true,'198.51.100.10','localhost:8088'],
    ['cli-server',true,'127.0.0.1','malicious.example'],
    ['cli-server',true,'127.0.0.1','localhost:443'],
    ['cli-server',true,'127.0.0.1','localhost:8088.evil.example'],
] as [$sapi,$enabled,$source,$host]) {
    assertPolicy(!TransportPolicy::permitsDevelopmentHttp($sapi,$enabled,$source,$host),
        'development exception requires exact operator flag, source and Host');
}
assertPolicy(TransportPolicy::permitsDevelopmentHttp('cli-server',true,
    '127.0.0.1','LOCALHOST:8088'), 'Host case normalization does not defeat local development');
echo "All Portal HTTPS enforcement policy tests passed.\n";
