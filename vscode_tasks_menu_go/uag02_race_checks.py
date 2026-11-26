import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent
env = os.environ.copy()
env["GOPROXY"] = "off"
env["GOSUMDB"] = "off"
env["CGO_ENABLED"] = "1"
packages = ["./internal/agentchat", "./internal/server"]
print("UAG-02 race suite: go test -race (GOPROXY=off, CGO_ENABLED=1)", packages, flush=True)
result = subprocess.run(
    ["go", "test", "-race", "-mod=readonly", "-count=1", "-timeout", "20m"] + packages,
    cwd=root, env=env, check=False,
    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
)
print(result.stdout, end="", flush=True)
print("UAG02_RACE_RESULT: rc=%d" % result.returncode, flush=True)
sys.exit(result.returncode)
