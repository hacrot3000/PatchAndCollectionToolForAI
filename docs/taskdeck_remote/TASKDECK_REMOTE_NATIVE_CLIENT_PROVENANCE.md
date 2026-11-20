# Native Remote LOGIN-ONLY client: independent provenance (RC-02)

**Security status: DEVELOPMENT / RC-02 PARTIAL / Remote Control NO-GO.**
This workflow does not create a remote application session or permit a TaskDeck
command. The native executable is distinct from PHP-delivered JavaScript.

## Build (operator-controlled GitHub)

1. Review the exact source commit of branch
   `main` from a trusted GitHub connection,
   not from the PHP Portal. Record the **full 40-character SHA** independently.
2. After this workflow file is available to GitHub Actions, an authorized
   maintainer manually runs `TaskDeck Remote native client provenance`.
   It runs no external PHP/Relay-controlled scripts and reads no PHP secrets.
3. The action cross-builds the native password client for Linux amd64/arm64 and
   macOS amd64/arm64, tests the critical login/key/E2EE packages, emits a
   `SOURCE_COMMIT`, `SHA256SUMS`, and generates GitHub/Sigstore signed SLSA
   provenance on the actual archives.
4. Download `taskdeck-remote-login-attested` **from GitHub Actions directly**.
   NEVER use a PHP Portal download, updater, script, prompt or link as the
   source of a trusted client. Anyone who controls the PHP site can alter it.

## Verify BEFORE extracting or running

Install GitHub CLI with the `gh attestation verify` command from a trusted
software source; authenticate to GitHub if needed. From a trusted checkout
of this repository (or a separately audited copy of the verifier), run:

```bash
bash deploy/remote/verify_client.sh \
  ./taskdeck-remote-login-darwin-arm64-FULL40CHAR_SOURCE_SHA.tar.gz \
  FULL40CHAR_SOURCE_SHA
```

Replace the placeholders with the actual archive and **source SHA obtained
independently of the PHP Portal**. The script performs cryptographic
verification of the repository, signer workflow, Git ref, source SHA,
artifact digest and hosted runner; it refuses mismatched archives, symlinks,
unexpected members and malformed archives. `SHA256SUMS` alone is not
cryptographic proof when the sender might have been compromised.

Only after PASS, manually extract in a private directory. The archive
contains the sole executable `taskdeck_remote_login`. Independently pin the
Agent Device ID outside PHP, then use the client first-use key approval flow.
Do not input a TaskDeck password into web pages or downloaded PHP assets.

## Remaining RC-02 evidence / approval

- Workflow and verifier tests must run green at the **exact final source SHA**;
  the workflow must actually produce attested archives and a verifier must
  validate them on physical Linux and macOS.
- The manual workflow runs only for independently reviewed `main` source and must
  be triggered intentionally. Presence on `main` does not approve Remote Control deployment.
- An attestation binds **what source/build workflow produced a binary**. It
  does not prove the source is bug-free, that its execution is safe, that a
  compromised GitHub account cannot push malicious source, or that a macOS
  binary is notarized. Independent security review is still required.
- Complete RC-01 and RC-03…RC-13, real deployments, independent pentest and
  signed release approval before claiming Remote CONTROL GO.

The PHP Portal and Relay must **never** gain add/revoke rights over the local
TaskDeck client-key allowlist; that remains a host CLI operation.
