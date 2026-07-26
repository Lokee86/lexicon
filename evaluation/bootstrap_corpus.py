from __future__ import annotations

import json
import subprocess
from pathlib import Path

DOC_LEDGER_COMMIT = "7fbdb81307ff7c6b1ba28886c3a40b8837ebf785"

REPOSITORIES = {
    "java-gson": "https://github.com/google/gson.git",
    "java-hikaricp": "https://github.com/brettwooldridge/HikariCP.git",
    "java-jmh": "https://github.com/openjdk/jmh.git",
    "java-jsoup": "https://github.com/jhy/jsoup.git",
    "java-maven": "https://github.com/apache/maven.git",
    "kotlin-coroutines": "https://github.com/Kotlin/kotlinx.coroutines.git",
    "kotlin-detekt": "https://github.com/detekt/detekt.git",
    "kotlin-nowinandroid": "https://github.com/android/nowinandroid.git",
    "lexicanter": "https://github.com/Saturnine-Softworks/Lexicanter.git",
}

PINNED_REVISIONS = {
    "java-gson": "aebc51a56ca0793c13b841c29f73433b82446695",
    "java-hikaricp": "a4d93f4f85517f90e632b795486d7102e933d7ff",
    "java-jmh": "a194eead0136bb66e5e59e4fdb2e18543e730929",
    "java-jsoup": "d24b16d952530f3aefe87e91c344b23fe4b8a7fc",
    "java-maven": "b646a7a38e920f7590e7b25fb43b5756d8bf2b4d",
    "kotlin-coroutines": "04ada74fae2e8914ae92ece34e06e80bb15385e9",
    "kotlin-detekt": "f9e1d5cc239ab740ce499b1edb36b872012648e2",
    "kotlin-nowinandroid": "7d45eae4f8720a0c77f507712ba2437ff974b6ed",
    "lexicanter": "eac754788b3cf18a930c085c1c49f8f353e18107",
}


def run(*args: str, cwd: Path | None = None) -> str:
    completed = subprocess.run(
        args,
        cwd=cwd,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    return completed.stdout.strip()


def revision(repository: Path) -> str:
    completed = subprocess.run(
        ("git", "rev-parse", "HEAD"),
        cwd=repository,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
    return completed.stdout.strip() if completed.returncode == 0 else ""


def ensure_doc_ledger(workspace: Path, corpus_root: Path) -> Path:
    source = workspace / "demon-docs"
    target = corpus_root / "doc-ledger-python"
    if not target.exists():
        run(
            "git",
            "-C",
            str(source),
            "worktree",
            "add",
            "--detach",
            str(target),
            DOC_LEDGER_COMMIT,
        )
    return target


def ensure_repository(corpus_root: Path, name: str, url: str) -> Path:
    target = corpus_root / name
    pinned = PINNED_REVISIONS[name]
    if not target.exists():
        target.mkdir()
        run("git", "init", cwd=target)
        run("git", "remote", "add", "origin", url, cwd=target)
    if revision(target) != pinned:
        run("git", "fetch", "--depth", "1", "origin", pinned, cwd=target)
        run("git", "checkout", "--detach", "FETCH_HEAD", cwd=target)
    return target


def main() -> None:
    workspace = Path(__file__).resolve().parents[2]
    corpus_root = workspace / "corpus"
    corpus_root.mkdir(parents=True, exist_ok=True)

    repositories = {"doc-ledger-python": ensure_doc_ledger(workspace, corpus_root)}
    for name, url in REPOSITORIES.items():
        repositories[name] = ensure_repository(corpus_root, name, url)

    state = {
        name: {"path": repository.as_posix(), "revision": revision(repository)}
        for name, repository in sorted(repositories.items())
    }
    state_path = Path(__file__).with_name("corpus_state.json")
    state_path.write_text(json.dumps(state, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(state, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
