from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

IGNORED_DIRECTORIES = {
    ".git",
    ".lexicon",
    ".worktrees",
    ".workingtrees",
    "artifacts",
    "bin",
    "node_modules",
    "obj",
    "packages",
    "target",
}


def corpus_workspace(root: Path) -> Path:
    for candidate in (root.parent, *root.parents):
        if (candidate / "corpus").is_dir():
            return candidate
    return root.parent


def dotnet_executable() -> str:
    configured = os.environ.get("LEXICON_DOTNET")
    if configured:
        if not Path(configured).is_file():
            raise FileNotFoundError(f"LEXICON_DOTNET does not exist: {configured}")
        return configured
    found = shutil.which("dotnet")
    if found:
        return found
    candidates = (
        Path("C:/Program Files/dotnet/dotnet.exe"),
        Path.home() / ".dotnet" / ("dotnet.exe" if os.name == "nt" else "dotnet"),
    )
    for candidate in candidates:
        if candidate.is_file():
            return str(candidate)
    raise FileNotFoundError("dotnet SDK not found; set LEXICON_DOTNET")


def discover_projects(repository: Path) -> list[Path]:
    return sorted(
        (
            project
            for project in repository.rglob("*.csproj")
            if not any(
                part.lower() in IGNORED_DIRECTORIES
                for part in project.relative_to(repository).parts
            )
        ),
        key=lambda path: path.as_posix(),
    )


def restore_project(dotnet: str, repository: Path, project: Path) -> tuple[bool, str]:
    env = os.environ.copy()
    dotnet_root = str(Path(dotnet).resolve().parent)
    env["DOTNET_ROOT"] = dotnet_root
    env["PATH"] = dotnet_root + os.pathsep + env.get("PATH", "")
    env["DOTNET_CLI_TELEMETRY_OPTOUT"] = "1"
    completed = subprocess.run(
        [dotnet, "restore", str(project), "--nologo", "--ignore-failed-sources"],
        cwd=repository,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    return completed.returncode == 0, completed.stdout


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    workspace = corpus_workspace(root)
    manifest = json.loads(Path(__file__).with_name("corpus.json").read_text(encoding="utf-8"))
    repositories = sorted(
        {
            case["repository"]
            for case in manifest["cases"]
            if case.get("adapter") == "csharp"
        }
    )
    dotnet = dotnet_executable()
    failures: list[str] = []

    print("C# corpus preparation executes MSBuild restore for trusted pinned repositories.")
    for relative_repository in repositories:
        repository = workspace / relative_repository
        if not repository.is_dir():
            failures.append(f"missing repository: {repository}")
            continue
        projects = discover_projects(repository)
        print(f"[{repository.name}] restoring {len(projects)} projects", flush=True)
        for index, project in enumerate(projects, 1):
            relative_project = project.relative_to(repository).as_posix()
            print(f"[{repository.name} {index}/{len(projects)}] {relative_project}", flush=True)
            passed, output = restore_project(dotnet, repository, project)
            if not passed:
                failures.append(f"{repository.name}/{relative_project}")
                if output:
                    print(output.rstrip(), flush=True)

    if failures:
        print("C# corpus preparation failures:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1
    print("C# corpus project preparation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
