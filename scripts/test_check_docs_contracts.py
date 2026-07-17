import importlib.util
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("check_docs_contracts", ROOT / "scripts" / "check_docs_contracts.py")
docs = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(docs)


class DocumentationContractDiscoveryTests(unittest.TestCase):
    def test_tracked_files_includes_untracked_non_ignored_docs(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            audit_dir = repo / "docs" / "audits"
            audit_dir.mkdir(parents=True)
            (audit_dir / "tracked.md").write_text("tracked\n", encoding="utf-8")
            (audit_dir / "untracked.md").write_text("untracked\n", encoding="utf-8")
            (audit_dir / "ignored.md").write_text("ignored\n", encoding="utf-8")
            (repo / ".gitignore").write_text("docs/audits/ignored.md\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(repo), "add", ".gitignore", "docs/audits/tracked.md"], check=True)

            with mock.patch.object(docs, "ROOT", repo):
                found = {path.name for path in docs.tracked_files("docs/audits/*.md")}

            self.assertEqual(found, {"tracked.md", "untracked.md"})


if __name__ == "__main__":
    unittest.main()
