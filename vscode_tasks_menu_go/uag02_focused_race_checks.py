import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent
env = os.environ.copy()
env["GOPROXY"] = "off"
env["GOSUMDB"] = "off"
env["CGO_ENABLED"] = "1"

tests = [
    "TestProviderRunStageDoesNotOverwritePreparedPayload",
    "TestProviderRunConcurrentStagingPreservesCommittedPrepared",
    "TestProviderRunPreparedPayloadSurvivesRestartAndIsNotReplayableAfterSending",
    "TestProviderRunPayloadTamperingFailsClosed",
    "TestStageProviderPayloadLockedRejectsInvalidInput",
    "TestObsoletePreparedPayloadUnlinkFsyncIsIndependentFromProviderACK",
    "TestStoreRejectsUnscopedProviderSelectionChanges",
    "TestProviderRunCompetingStagedPayloadCAS",
    "TestStoreRejectsPendingTurnRemoval",
    "TestStoreRejectsPendingTurnReplacement",
    "TestPreservesPendingUserTurnInvariant",
    "TestPendingAdmissionRejectsEmptyOwner",
    "TestPendingAdmissionBindingMatches",
    "TestPendingAdmissionCandidateMatches",
    "TestPendingAdmissionConcurrentCommit",
    "TestPendingAdmissionStagingFailurePreservesTurn",
    "TestPendingAdmissionRejectsOtherOwner",
    "TestPendingAdmissionStagedSidecarBeforeCommit",
    "TestPendingTurnSurvivesUnrelatedJournalSave",
    "TestPendingTurnConcurrentJournalCAS",
    "TestAgentPreparedIntentConstruction",
    "TestAgentPreparedResumeRejectsCredentialDrift",
    "TestAgentPreparedProviderRunResumesExactlyOnceAfterRestart",
    "TestAgentSendingRunRestartBecomesAmbiguousWithoutProviderReplay",
    "TestAgentPendingAdmissionBridgeRejectsInvalidCandidates",
    "TestAgentPendingAdmissionBridgeCommitsPrepared",
    "TestAgentPendingAdmissionWithoutPendingFailsClosed",
    "TestAgentPendingAdmissionRejectsEffectiveModeDrift",
    "TestAgentPendingFirstTurnRetainedWhenPackChangesHistory",
    "TestAgentSuspendedPendingDispatchRetainsTurn",
    "TestAgentSuspendedPendingDispatchRejectsForeignOwner",
]
pattern = "^(" + "|".join(tests) + ")$"
packages = ["./internal/agentchat", "./internal/server"]
print("UAG-02 focused RACE suite: go test -race", packages, flush=True)
print("filter:", pattern, flush=True)
result = subprocess.run(
    ["go", "test", "-race", "-v", "-mod=readonly", "-count=1", "-run", pattern] + packages,
    cwd=root, env=env, check=False,
    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
)
print(result.stdout, end="", flush=True)
print("UAG02_FOCUSED_RACE_RESULT: rc=%d" % result.returncode, flush=True)
sys.exit(result.returncode)
