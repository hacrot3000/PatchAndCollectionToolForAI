import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent
env = os.environ.copy()
env["GOPROXY"] = "off"
env["GOSUMDB"] = "off"
checks = [
    ("./internal/agentchat", "TestProviderRun(StageDoesNotOverwritePreparedPayload|ConcurrentStagingPreservesCommittedPrepared|PreparedPayloadSurvivesRestartAndIsNotReplayableAfterSending|PayloadTamperingFailsClosed)$"),
    ("./internal/agentchat", "^TestObsoletePreparedPayloadUnlinkFsyncIsIndependentFromProviderACK$"),
    ("./internal/agentchat", "^TestStoreRejectsUnscopedProviderSelectionChanges$"),
    ("./internal/agentchat", "^TestProviderRunCompetingStagedPayloadCAS$"),
    ("./internal/agentchat", "^TestStoreRejectsPendingTurnRemoval$"),
    ("./internal/agentchat", "^TestStoreRejectsPendingTurnReplacement$"),
    ("./internal/server", "TestAgent(PreparedResumeRejectsCredentialDrift|PreparedProviderRunResumesExactlyOnceAfterRestart|SendingRunRestartBecomesAmbiguousWithoutProviderReplay|SuspendedPendingDispatchRetainsTurn)$"),
]
for package, pattern in checks:
    print("Checking", package, pattern, flush=True)
    result = subprocess.run(["go", "test", "-v", "-mod=readonly", "-count=1", "-run", pattern, package], cwd=root, env=env, check=False, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    print(result.stdout, end="", flush=True)
    if result.returncode:
        sys.exit(result.returncode)
    if "testing: warning: no tests to run" in result.stdout:
        sys.exit("Focused check matched no tests: " + pattern)
