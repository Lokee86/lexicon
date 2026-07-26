from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

ADAPTER = Path(__file__).resolve().parents[1]
FIXTURE = ADAPTER / "testdata" / "smoke"
PROJECT = ADAPTER / "Lexicon.CSharp.csproj"
REQUIRED_RELATIONS = {"calls", "reads", "writes", "depends-on", "extends", "implements", "overrides"}


def dotnet() -> str:
    candidates = [
        os.environ.get("LEXICON_DOTNET"),
        shutil.which("dotnet"),
        r"C:\Program Files\dotnet\dotnet.exe",
        str(Path.home() / ".dotnet" / "dotnet.exe"),
    ]
    for candidate in candidates:
        if candidate and Path(candidate).is_file():
            return candidate
    raise FileNotFoundError("dotnet SDK not found; set LEXICON_DOTNET")


def run_adapter(output: Path, *scope: str) -> list[dict[str, object]]:
    command = [
        dotnet(),
        "run",
        "--no-restore",
        "--project",
        str(PROJECT),
        "--",
        "--repo",
        str(FIXTURE),
        "--output",
        str(output),
        *scope,
    ]
    completed = subprocess.run(command, cwd=ADAPTER, text=True, capture_output=True)
    if completed.returncode != 0:
        raise RuntimeError(completed.stdout + completed.stderr)
    return [json.loads(line) for line in output.read_text(encoding="utf-8").splitlines() if line]


def assert_valid(records: list[dict[str, object]]) -> None:
    assert records and records[0]["record"] == "lexicon"
    nodes = {record["id"] for record in records if record.get("record") == "node"}
    for edge in (record for record in records if record.get("record") == "edge"):
        assert edge["source"] in nodes, edge
        assert edge["target"] in nodes, edge


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="lexicon-csharp-test-") as temporary:
        directory = Path(temporary)
        first_path = directory / "first.jsonl"
        second_path = directory / "second.jsonl"
        incremental_path = directory / "incremental.jsonl"

        first = run_adapter(first_path)
        second = run_adapter(second_path)
        assert first_path.read_bytes() == second_path.read_bytes()
        assert_valid(first)
        relations = {record["relation"] for record in first if record.get("record") == "edge"}
        assert REQUIRED_RELATIONS <= relations, sorted(REQUIRED_RELATIONS - relations)

        incremental = run_adapter(incremental_path, "--changed-file", "Program.cs")
        assert_valid(incremental)
        header = incremental[0]
        assert header["mode"] == "incremental"
        assert header["changed_files"] == ["Program.cs"]
        assert header["removed_files"] == []
        assert header["shared_complete"] is True

    print("C# adapter smoke, determinism, relation, and incremental checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
