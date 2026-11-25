import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / "_patch_lib"))
from python_patch_project_state import ensure_project_identity
from python_patch_queue_dispatcher import _retire_failed_agent_package, _update_unresolved_registry, QueueSafetyError

class FailedQueueTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.identity = ensure_project_identity(self.root)
        self.queue = self.root / "patchs"
        self.queue.mkdir(exist_ok=True)
        self.history = self.root / "artifacts/patch_tool/history"
        self.history.mkdir(parents=True)
        self.name = "PATCH_CLEAR_TEXT_test.zip"
        self.body = b"exact failed package"
        self.sha = hashlib.sha256(self.body).hexdigest()
        (self.queue / self.name).write_bytes(self.body)
        self.source = self.root / "source.txt"
        self.source.write_text("partially modified")
        self.handoff = self.history.parent / "FAIL_HANDOFF_test.txt"
        self.handoff.write_text("failure evidence")
    def report(self, status="FAIL", kind="PATCH", run="r1"):
        row = {"name": self.name, "kind": kind, "status": status}
        if kind == "PATCH":
            row["patch_result"] = {"patch_sha256": self.sha, "manifest_patch": {"id": "p"}}
        else:
            row["request_sha256"] = self.sha
            row["native_run_id"] = run
        report = {"project_identity": self.identity, "run_id": run, "results": [row]}
        (self.history / (run + ".json")).write_text(json.dumps(report))
        _update_unresolved_registry(self.root, report)
        return report
    def retire(self, kind="PATCH", run="r1", digest=None, project=None):
        identity = project or self.identity
        return _retire_failed_agent_package(self.root, identity["key"], identity["id"], self.name, kind, digest or self.sha, run)
    def test_patch_retired_but_evidence_preserved(self):
        self.report()
        cp = subprocess.run([sys.executable, str(ROOT/"python_patch_entry.py"), "--project-root", str(self.root), "--",
            "retire-failed", "--project-key", self.identity["key"], "--project-id", self.identity["id"],
            "--failed-name", self.name, "--failed-kind", "PATCH", "--failed-sha256", self.sha, "--run-id", "r1"], capture_output=True, text=True)
        self.assertEqual(cp.returncode, 0, cp.stderr)
        receipt = json.loads(cp.stdout.splitlines()[-1])
        self.assertEqual(receipt["status"], "retired")
        self.assertFalse((self.queue/self.name).exists())
        self.assertEqual((self.root/receipt["archive"]).read_bytes(), self.body)
        self.assertEqual(self.source.read_text(), "partially modified")
        self.assertEqual(self.handoff.read_text(), "failure evidence")
        self.assertTrue((self.history/"r1.json").exists())
        reg = json.loads((self.history.parent/"UNRESOLVED_FAILURES.json").read_text())
        self.assertTrue(reg["entries"][0]["resolved"])
        self.assertEqual(self.retire()["status"], "not_queued")
    def test_pass_and_ambiguous_are_never_retired(self):
        self.report("PASS")
        with self.assertRaises(QueueSafetyError): self.retire()
        self.report("UNKNOWN", run="r2")
        with self.assertRaises(QueueSafetyError): self.retire(run="r2")
        self.assertEqual((self.queue/self.name).read_bytes(), self.body)
    def test_new_or_changed_or_wrong_project_refused(self):
        with self.assertRaises(QueueSafetyError): self.retire()
        self.report()
        with self.assertRaises(QueueSafetyError): self.retire(run="other")
        with self.assertRaises(QueueSafetyError): self.retire(project={"key":"other","id":self.identity["id"]})
        (self.queue/self.name).write_bytes(b"replacement")
        with self.assertRaises(QueueSafetyError): self.retire()
        self.assertEqual((self.queue/self.name).read_bytes(), b"replacement")
    def test_collect_failed_only(self):
        (self.queue/self.name).unlink()
        self.name = "CODE_COLLECTION_REQUEST_CLEAR_TEXT_test.zip"
        (self.queue/self.name).write_bytes(self.body)
        self.report("INCOMPLETE","COLLECT")
        self.assertEqual(self.retire("COLLECT")["status"], "retired")
    def test_success_archive_protected(self):
        self.report()
        (self.queue/"patched").mkdir(exist_ok=True)
        (self.queue/"patched"/self.name).write_bytes(self.body)
        with self.assertRaises(QueueSafetyError): self.retire()
        self.assertTrue((self.queue/self.name).exists())
        self.assertEqual((self.queue/"patched"/self.name).read_bytes(),self.body)

if __name__ == "__main__":
    unittest.main()
