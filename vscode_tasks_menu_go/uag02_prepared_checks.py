import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent
env = os.environ.copy()
env["GOPROXY"] = "off"
env["GOSUMDB"] = "off"
checks = [
    ("./internal/agentchat", "^TestProviderRunStageDoesNotOverwritePreparedPayload$"),
    ("./internal/agentchat", "^TestProviderRunConcurrentStagingPreservesCommittedPrepared$"),
    ("./internal/agentchat", "^TestProviderRunPreparedPayloadSurvivesRestartAndIsNotReplayableAfterSending$"),
    ("./internal/agentchat", "^TestProviderRunPayloadTamperingFailsClosed$"),
    ("./internal/agentchat", "^TestStageProviderPayloadLockedRejectsInvalidInput$"),
    ("./internal/agentchat", "^TestObsoletePreparedPayloadUnlinkFsyncIsIndependentFromProviderACK$"),
    ("./internal/agentchat", "^TestStoreRejectsUnscopedProviderSelectionChanges$"),
    ("./internal/agentchat", "^TestProviderRunCompetingStagedPayloadCAS$"),
    ("./internal/agentchat", "^TestStoreRejectsPendingTurnRemoval$"),
    ("./internal/agentchat", "^TestStoreRejectsPendingTurnReplacement$"),
    ("./internal/agentchat", "^TestPreservesPendingUserTurnInvariant$"),
    ("./internal/agentchat", "^TestPendingAdmissionRejectsEmptyOwner$"),
    ("./internal/agentchat", "^TestPendingAdmissionBindingMatches$"),
    ("./internal/agentchat", "^TestPendingAdmissionCandidateMatches$"),
    ("./internal/agentchat", "^TestPendingAdmissionConcurrentCommit$"),
    ("./internal/agentchat", "^TestPendingAdmissionStagingFailurePreservesTurn$"),
    ("./internal/agentchat", "^TestPendingAdmissionRejectsOtherOwner$"),
    ("./internal/agentchat", "^TestPendingAdmissionStagedSidecarBeforeCommit$"),
    ("./internal/agentchat", "^TestPendingTurnSurvivesUnrelatedJournalSave$"),
    ("./internal/agentchat", "^TestPendingTurnConcurrentJournalCAS$"),
    ("./internal/server", "^TestAgentPreparedIntentConstruction$"),
    ("./internal/server", "^TestAgentPreparedResumeRejectsCredentialDrift$"),
    ("./internal/server", "^TestAgentPreparedProviderRunResumesExactlyOnceAfterRestart$"),
    ("./internal/server", "^TestAgentSendingRunRestartBecomesAmbiguousWithoutProviderReplay$"),
    ("./internal/server", "^TestAgentPendingAdmissionBridgeRejectsInvalidCandidates$"),
    ("./internal/server", "^TestAgentPendingAdmissionBridgeCommitsPrepared$"),
    ("./internal/server", "^TestAgentPendingAdmissionWithoutPendingFailsClosed$"),
    ("./internal/server", "^TestAgentPendingAdmissionRejectsEffectiveModeDrift$"),
    ("./internal/server", "^TestAgentPendingFirstTurnRetainedWhenPackChangesHistory$"),
    ("./internal/server", "^TestAgentSuspendedPendingDispatchRetainsTurn$"),
    ("./internal/server", "^TestAgentSuspendedPendingDispatchRejectsForeignOwner$"),
]
for package, pattern in checks:
    print("Checking", package, pattern, flush=True)
    result = subprocess.run(["go", "test", "-v", "-mod=readonly", "-count=1", "-run", pattern, package], cwd=root, env=env, check=False, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    print(result.stdout, end="", flush=True)
    if result.returncode:
        sys.exit(result.returncode)
    if "testing: warning: no tests to run" in result.stdout:
        sys.exit("Focused check matched no tests: " + pattern)
