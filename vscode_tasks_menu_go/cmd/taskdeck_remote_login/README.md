# TaskDeck trusted local E2EE login probe (Linux/macOS)

This **local CLI** is the only currently implemented, independently built client
capable of accepting a TaskDeck password in this experimental Remote design.
It is not Remote Control. It authenticates one TaskDeck password and receives
only an encrypted yes/no response; no cookies, session tokens, APIs, terminal,
files, Git or Patch execution are exposed.

**Independent artifact trust:** The manual GitHub Actions provenance build and
fail-closed Sigstore verifier are documented in
[TaskDeck native client provenance](../../../../docs/TASKDECK_REMOTE_NATIVE_CLIENT_PROVENANCE.md).
Never download a client executable or updater from the PHP Portal. Only run a
verified client from a separately trusted GitHub source SHA, and independently
pin the target Agent Device ID before entering credentials.

## Trust separation for a fully compromised PHP Portal

Assume an attacker can replace the Portal HTML/JavaScript, issue arbitrary
Portal-signed tickets, change the SQLite grant table, and return an arbitrary
grant-check verdict. Portal is **not** a trusted source of executable code or
password input. An E2EE web page *served by PHP* would be vulnerable to
pre-encryption credential theft.

- Build this CLI **from a source revision independently verified outside the
  PHP host**. Do not execute a downloadable binary/script supplied by Portal.
- Pin the target **Device ID directly from the owner-controlled TaskDeck
  instance**, never just trust the Portal's displayed device ID. The CLI
  requires a separate **owner-only local pin file**, and rejects any Device
  ID supplied on the command line that disagrees with it.
- Pass trusted HTTPS origins explicitly; the CLI does not dynamically load
  configuration, JavaScript, a remote binary or client secrets from Portal.
- Only a **public P-256 browser key**, **public nonce** and one-use ticket
  are exchanged via Portal. Private P-256 material stays in CLI process memory.
- The TaskDeck username/password are read **only from the local Unix TTY**.
  The CLI refuses password input via argv, environment variable or redirected
  stdin; password echo is disabled while typing. Requires `stty`.
- The client authenticates Agent's locally pinned Ed25519 identity, signed
  ephemeral ECDH transcript, AES-GCM ready and final encrypted result itself.
  A forged Relay/Portal cannot replace the verified TaskDeck device identity.

## Build and manual diagnostic

In the trusted, verified Git working tree:

```sh
cd vscode_tasks_menu_go
go build -o ./taskdeck_remote_login ./cmd/taskdeck_remote_login
# On the CLIENT computer, obtain the Device ID directly from LOCAL TaskDeck,
# not from the shared-host Portal's web page. Protect this independent pin.
mkdir -p "$HOME/.config/taskdeck/trusted-remote"
chmod 700 "$HOME/.config/taskdeck/trusted-remote"
printf '%s\\n' 'OWNER_VERIFIED_64_CHARACTER_DEVICE_ID' > "$HOME/.config/taskdeck/trusted-remote/device.id"
chmod 600 "$HOME/.config/taskdeck/trusted-remote/device.id"
./taskdeck_remote_login \
  -relay=https://relay.example.com \
  -portal=https://portal.example.com \
  -device=OWNER_VERIFIED_64_CHARACTER_DEVICE_ID \
  -pin-file="$HOME/.config/taskdeck/trusted-remote/device.id"
```

1. Confirm the browser's Portal account has MFA, a current grant to the device
   and that the Agent/Relay experimental probe is independently enabled.
2. Enter the **TaskDeck username only** in the locally installed CLI.
   The CLI then prints a public client nonce and ephemeral public key.
   **It must NOT request your TaskDeck password yet.**
3. On Portal's authenticated device row, open **"Cấp ticket cho client TaskDeck
   cài riêng"**. Paste **only** the nonce and public key. Click the ticket
   button and copy the `ticket` string from the JSON response.
4. Paste the ticket into the CLI within the ticket's **30-second TTL**.
   The CLI first validates the independently pinned Agent Ed25519 identity,
   its signed P-256 ECDH transcript, and the authenticated AES-GCM ready
   packet. **Only after this succeeds does the CLI prompt locally for the
   TaskDeck password** (TTY echo disabled). It then sends one AES-GCM-encrypted
   password proof and receives a boolean verdict. If PHP/Relay forges the
   Agent identity or drops the connection, no password prompt occurs.
   This never opens a Remote session.

The local pin must be an absolute-path, regular 0600 file inside a real 0700
directory; symlinked or public pins fail closed. A malicious Portal cannot
silently swap its preferred Device ID while this trusted pin remains unchanged.

The HTTP Portal origin is passed for the Relay's exact `Origin` check, **not**
to download or execute Portal code. The CLI connects only to the configured
Relay via WSS. A compromised Portal can refuse service, issue useless tickets
or trick someone into trusting a malicious executable or phishing UI; the CLI
**cannot** protect people who independently give their password to malicious
Portal content.

**Current limitations:** The local Agent's experimental login callback
requires the TaskDeck operator's explicit opt-in
(`TASKDECK_REMOTE_LOGIN_PROBE=1`) and local shared-server identity/config.
It is intentionally disabled by default. The PHP Portal must have
`REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1` and a valid Portal MFA session.
The native client is still a **developer prototype**, not an approved
production release: a manual GitHub build can now create attested archives,
but those attestations MUST actually be generated and verified against an
independently pinned source SHA. No deployment should describe Remote Control as
enabled or as fully secure until the independent client delivery/pinning,
per-action TaskDeck RBAC, stream cancellation and penetration-test gates pass.


## Mandatory trust rule during a PHP Portal compromise

A compromised PHP Portal can **issue signed admin tickets, change grants and
return fabricated `allowed=true` responses**, but these operations are not
TaskDeck authentication. The TaskDeck daemon independently verifies the
user's LOCAL TaskDeck password and project membership; the future action gate
requires that original password-authenticated, in-process E2EE session and a
fresh Portal check. It cannot adopt a normal TaskDeck browser cookie or a
role supplied by PHP. A changed local TaskDeck password invalidates the
staged session even if the Portal continues answering allowed=true.

Do not type TaskDeck credentials into a PHP-served page, a PHP-hosted download,
or a program whose signature/source you have not verified independently. A
fully compromised Portal can replace its own HTML/JS to phish users; no crypto
protocol can guarantee safety if the user voluntarily enters the password
into attacker-controlled code. The local CLI and independent Device ID pin are
therefore not optional components of this threat model.
