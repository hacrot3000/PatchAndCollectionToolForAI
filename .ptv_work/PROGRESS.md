# Provenance/signature feature progress

Task scope remains strictly limited to local cryptographic provenance / signed PATCH manifest trust.

Current feature branch state:
- Ed25519 verifier, provenance schema/preflight/legacy policy integration, dedicated contract documentation, and semantic regression are committed.
- Targeted provenance, PATCH preflight, public-entry, historical-compatibility, and installed-layout checksum tests passed on the committed integration tree.
- Remaining completion gates: add provenance artifacts to mandatory Tool Health/package/master/AI-sync gates, synchronize only the relevant status/ledger docs, regenerate deterministic metadata, run the full master suite from the feature-branch HEAD in PROJECT_ROOT/tools layout, then merge to main only if every final gate passes.
- Explicitly out of scope: PKI, remote trust registry/network lookup, private-key generation/management, COLLECT signing, and reproducible ZIP/package bytes.

This checkpoint is committed because unfinished feature work must leave a branch commit on every request.
