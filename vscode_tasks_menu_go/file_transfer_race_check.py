import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent
env = os.environ.copy()
env["GOPROXY"] = "off"
env["GOSUMDB"] = "off"
env["CGO_ENABLED"] = "1"
pattern = "^TestStagedBrowserUploadReturnsImmediatelyAndUsesDaemonQueue$"
print("file-transfer upload -race check", pattern, flush=True)
result = subprocess.run(
    ["go", "test", "-race", "-v", "-mod=readonly", "-count=1", "-timeout", "10m",
     "-run", pattern, "./internal/server"],
    cwd=root, env=env, check=False,
    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
)
print(result.stdout, end="", flush=True)
print("FILE_TRANSFER_RACE_RESULT: rc=%d" % result.returncode, flush=True)
sys.exit(result.returncode)
