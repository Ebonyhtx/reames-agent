#!/usr/bin/env python3
"""Validate Desktop candidate artifact structure.

The Desktop candidate workflow uploads one GitHub Actions artifact per native
platform. This checker is intentionally offline: pass downloaded artifact zip
files, expanded artifact directories, or both.
"""

from __future__ import annotations

import argparse
import io
import sys
import tarfile
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Iterable


@dataclass(frozen=True)
class CandidateFile:
    name: str
    read_bytes: Callable[[], bytes]


def normalize(name: str) -> str:
    return name.replace("\\", "/").lstrip("/")


def collect_from_directory(path: Path) -> list[CandidateFile]:
    files: list[CandidateFile] = []
    for child in path.rglob("*"):
        if not child.is_file():
            continue
        rel = normalize(str(child.relative_to(path)))
        files.append(CandidateFile(rel, child.read_bytes))
    return files


def collect_from_zip(path: Path) -> list[CandidateFile]:
    files: list[CandidateFile] = []
    data = path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        for info in archive.infolist():
            if info.is_dir():
                continue
            name = normalize(info.filename)

            def reader(member=name, archive_data=data) -> bytes:
                with zipfile.ZipFile(io.BytesIO(archive_data)) as nested:
                    return nested.read(member)

            files.append(CandidateFile(name, reader))
    return files


def collect_files(paths: Iterable[Path]) -> list[CandidateFile]:
    files: list[CandidateFile] = []
    for path in paths:
        if path.is_dir():
            files.extend(collect_from_directory(path))
        elif path.is_file() and path.suffix.lower() == ".zip":
            files.extend(collect_from_zip(path))
        elif path.is_file():
            files.append(CandidateFile(path.name, path.read_bytes))
        else:
            raise FileNotFoundError(path)
    return files


def basename(file: CandidateFile) -> str:
    return Path(file.name).name


def find_one(files: list[CandidateFile], predicate: Callable[[CandidateFile], bool], label: str, failures: list[str]) -> CandidateFile | None:
    matches = [file for file in files if predicate(file)]
    if not matches:
        failures.append(f"missing {label}")
        return None
    if len(matches) > 1:
        failures.append(f"expected one {label}, found {len(matches)}: {', '.join(file.name for file in matches)}")
        return None
    return matches[0]


def zip_contains(file: CandidateFile, required: list[str]) -> list[str]:
    try:
        with zipfile.ZipFile(io.BytesIO(file.read_bytes())) as archive:
            names = {normalize(info.filename) for info in archive.infolist() if not info.is_dir()}
    except zipfile.BadZipFile as exc:
        return [f"{file.name} is not a valid zip: {exc}"]
    return [f"{file.name} missing {want}" for want in required if want not in names]


def tar_contains(file: CandidateFile, required: list[str]) -> list[str]:
    try:
        with tarfile.open(fileobj=io.BytesIO(file.read_bytes()), mode="r:*") as archive:
            names = {normalize(member.name) for member in archive.getmembers() if member.isfile()}
    except tarfile.TarError as exc:
        return [f"{file.name} is not a valid tar archive: {exc}"]
    return [f"{file.name} missing {want}" for want in required if want not in names]


def deb_members(file: CandidateFile) -> set[str]:
    data = file.read_bytes()
    if not data.startswith(b"!<arch>\n"):
        raise ValueError("missing ar global header")
    offset = 8
    names: set[str] = set()
    while offset + 60 <= len(data):
        header = data[offset : offset + 60]
        raw_name = header[:16].decode("utf-8", errors="replace").strip()
        raw_size = header[48:58].decode("ascii", errors="replace").strip()
        try:
            size = int(raw_size)
        except ValueError as exc:
            raise ValueError(f"invalid ar member size {raw_size!r}") from exc
        name = raw_name.rstrip("/")
        if name:
            names.add(name)
        offset += 60 + size
        if offset % 2:
            offset += 1
    return names


def deb_contains(file: CandidateFile, required_prefixes: list[str]) -> list[str]:
    try:
        members = deb_members(file)
    except ValueError as exc:
        return [f"{file.name} is not a valid deb/ar archive: {exc}"]
    failures: list[str] = []
    for prefix in required_prefixes:
        if not any(member == prefix or member.startswith(prefix) for member in members):
            failures.append(f"{file.name} missing {prefix}")
    return failures


def check_windows(files: list[CandidateFile], failures: list[str]) -> None:
    find_one(
        files,
        lambda file: basename(file) == "Reames Agent-windows-amd64-installer.exe",
        "Windows NSIS installer",
        failures,
    )
    portable = find_one(
        files,
        lambda file: basename(file) == "Reames Agent-windows-amd64.zip",
        "Windows portable zip",
        failures,
    )
    if portable:
        failures.extend(zip_contains(portable, ["Reames Agent.exe", "reames-agent-update-helper.exe"]))


def check_linux(files: list[CandidateFile], failures: list[str]) -> None:
    tarball = find_one(
        files,
        lambda file: basename(file) == "Reames Agent-linux-amd64.tar.gz",
        "Linux tarball",
        failures,
    )
    if tarball:
        failures.extend(tar_contains(tarball, ["reames-agent-desktop"]))
    deb = find_one(
        files,
        lambda file: basename(file) == "Reames Agent-linux-amd64.deb",
        "Linux deb package",
        failures,
    )
    if deb:
        failures.extend(deb_contains(deb, ["debian-binary", "control.tar", "data.tar"]))


def check_macos_zip(files: list[CandidateFile], arch: str, failures: list[str]) -> None:
    archive = find_one(
        files,
        lambda file: basename(file) == f"Reames Agent-darwin-{arch}.zip",
        f"macOS {arch} zip",
        failures,
    )
    if archive:
        failures.extend(
            zip_contains(
                archive,
                [
                    "Reames Agent.app/Contents/MacOS/reames-agent-desktop",
                    "Reames Agent.app/Contents/Info.plist",
                    "Reames Agent.app/Contents/Resources/iconfile.icns",
                ],
            )
        )


def check_macos(files: list[CandidateFile], failures: list[str]) -> None:
    check_macos_zip(files, "amd64", failures)
    check_macos_zip(files, "arm64", failures)
    find_one(
        files,
        lambda file: basename(file) == "Reames Agent-darwin-universal.dmg",
        "macOS universal dmg",
        failures,
    )


def check(files: list[CandidateFile]) -> list[str]:
    failures: list[str] = []
    check_windows(files, failures)
    check_linux(files, failures)
    check_macos(files, failures)
    return failures


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Validate Reames Desktop candidate artifacts.")
    parser.add_argument("paths", nargs="+", type=Path, help="Downloaded artifact zip files or expanded artifact directories.")
    args = parser.parse_args(argv)

    files = collect_files(args.paths)
    failures = check(files)
    if failures:
        print("Desktop artifact check failed:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1
    print("Desktop artifact check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
