import io
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path

from scripts import check_desktop_artifacts as checker


def write_zip(path: Path, members: dict[str, bytes]) -> None:
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for name, data in members.items():
            archive.writestr(name, data)


def tar_gz_bytes(members: dict[str, bytes]) -> bytes:
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz") as archive:
        for name, data in members.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))
    return out.getvalue()


def ar_member(name: str, data: bytes) -> bytes:
    header = (
        f"{name + '/':<16}"
        f"{0:<12}"
        f"{0:<6}"
        f"{0:<6}"
        f"{0o100644:<8}"
        f"{len(data):<10}`\n"
    ).encode("ascii")
    body = data + (b"\n" if len(data) % 2 else b"")
    return header + body


def deb_bytes() -> bytes:
    return (
        b"!<arch>\n"
        + ar_member("debian-binary", b"2.0\n")
        + ar_member("control.tar.gz", b"control")
        + ar_member("data.tar.gz", b"data")
    )


class DesktopArtifactCheckTests(unittest.TestCase):
    def make_good_tree(self, root: Path) -> None:
        (root / "windows").mkdir()
        (root / "linux").mkdir()
        (root / "darwin").mkdir()
        (root / "windows" / "Reames Agent-windows-amd64-installer.exe").write_bytes(b"installer")
        write_zip(
            root / "windows" / "Reames Agent-windows-amd64.zip",
            {
                "Reames Agent.exe": b"app",
                "reames-agent-update-helper.exe": b"helper",
            },
        )
        (root / "linux" / "Reames Agent-linux-amd64.tar.gz").write_bytes(
            tar_gz_bytes({"reames-agent-desktop": b"linux"})
        )
        (root / "linux" / "Reames Agent-linux-amd64.deb").write_bytes(deb_bytes())
        for arch in ("amd64", "arm64"):
            write_zip(
                root / "darwin" / f"Reames Agent-darwin-{arch}.zip",
                {
                    "Reames Agent.app/Contents/MacOS/reames-agent-desktop": b"mac",
                    "Reames Agent.app/Contents/Info.plist": b"plist",
                    "Reames Agent.app/Contents/Resources/iconfile.icns": b"icon",
                },
            )
        (root / "darwin" / "Reames Agent-darwin-universal.dmg").write_bytes(b"dmg")

    def test_good_expanded_artifacts_pass(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.make_good_tree(root)
            files = checker.collect_files([root])
            self.assertEqual(checker.check(files), [])

    def test_missing_windows_update_helper_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.make_good_tree(root)
            write_zip(root / "windows" / "Reames Agent-windows-amd64.zip", {"Reames Agent.exe": b"app"})
            failures = checker.check(checker.collect_files([root]))
            self.assertIn(
                "windows/Reames Agent-windows-amd64.zip missing reames-agent-update-helper.exe",
                failures,
            )

    def test_downloaded_outer_artifact_zips_pass(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            expanded = root / "expanded"
            expanded.mkdir()
            self.make_good_tree(expanded)
            outer = root / "windows-artifact.zip"
            write_zip(
                outer,
                {
                    "Reames Agent-windows-amd64-installer.exe": (expanded / "windows" / "Reames Agent-windows-amd64-installer.exe").read_bytes(),
                    "Reames Agent-windows-amd64.zip": (expanded / "windows" / "Reames Agent-windows-amd64.zip").read_bytes(),
                },
            )
            files = checker.collect_files([outer, expanded / "linux", expanded / "darwin"])
            self.assertEqual(checker.check(files), [])


if __name__ == "__main__":
    unittest.main()
