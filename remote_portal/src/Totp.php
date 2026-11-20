<?php
declare(strict_types=1);

namespace TaskDeck\RemotePortal;

use RuntimeException;

/**
 * RFC 6238 TOTP, 30-second SHA-1 / 6 digits for standard authenticator apps.
 * This class handles Portal account MFA ONLY, never TaskDeck credentials.
 */
final class Totp
{
    public static function newSecret(): string
    {
        return self::base32Encode(random_bytes(20)); // 160-bit CSPRNG
    }

    public static function provisioningUri(string $username, string $secret): string
    {
        self::decodeSecret($secret);
        return 'otpauth://totp/' . rawurlencode('TaskDeck Portal:' . $username)
            . '?secret=' . rawurlencode($secret)
            . '&issuer=' . rawurlencode('TaskDeck Portal')
            . '&algorithm=SHA1&digits=6&period=30';
    }

    /** Return the matching monotonic TOTP timestep, or null. */
    public static function match(string $secret, string $code, ?int $now = null, int $lastStep = -1): ?int
    {
        if (!preg_match('/^[0-9]{6}$/D', $code)) {
            return null;
        }
        $key = self::decodeSecret($secret);
        $now ??= time();
        if ($now < 30) {
            return null;
        }
        $step = intdiv($now, 30);
        // Accept at most +/- 30 seconds of clock skew.
        foreach ([$step, $step-1, $step+1] as $candidate) {
            if ($candidate < 1 || $candidate <= $lastStep) {
                continue;
            }
            $bytes = pack('N2', ($candidate >> 32) & 0xffffffff, $candidate & 0xffffffff);
            $mac = hash_hmac('sha1', $bytes, $key, true);
            $offset = ord($mac[19]) & 15;
            $value = ((ord($mac[$offset]) & 0x7f) << 24)
                | (ord($mac[$offset+1]) << 16)
                | (ord($mac[$offset+2]) << 8)
                | ord($mac[$offset+3]);
            $digits = str_pad((string)($value % 1000000), 6, '0', STR_PAD_LEFT);
            if (hash_equals($digits, $code)) {
                return $candidate;
            }
        }
        return null;
    }

    private static function base32Encode(string $bytes): string
    {
        $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
        $bits = 0;
        $buffer = 0;
        $out = '';
        for ($i=0; $i<strlen($bytes); $i++) {
            $buffer = ($buffer << 8) | ord($bytes[$i]);
            $bits += 8;
            while ($bits >= 5) {
                $bits -= 5;
                $out .= $alphabet[($buffer >> $bits) & 31];
            }
            $buffer &= (1 << $bits) - 1;
        }
        if ($bits > 0) {
            $out .= $alphabet[($buffer << (5-$bits)) & 31];
        }
        return $out;
    }

    private static function decodeSecret(string $text): string
    {
        if (!preg_match('/^[A-Z2-7]{32}$/D', $text)) {
            throw new RuntimeException('Invalid Portal TOTP secret');
        }
        $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
        $buffer = 0;
        $bits = 0;
        $out = '';
        for ($i=0; $i<strlen($text); $i++) {
            $n = strpos($alphabet, $text[$i]);
            if ($n === false) {
                throw new RuntimeException('Invalid Portal TOTP encoding');
            }
            $buffer = ($buffer << 5) | $n;
            $bits += 5;
            if ($bits >= 8) {
                $bits -= 8;
                $out .= chr(($buffer >> $bits) & 255);
                $buffer &= (1 << $bits) - 1;
            }
        }
        if (strlen($out) !== 20 || $bits !== 0) {
            throw new RuntimeException('Invalid Portal TOTP length');
        }
        return $out;
    }
}
