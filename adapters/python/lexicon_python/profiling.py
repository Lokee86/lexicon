"""Optional adapter phase profiling for Lexicon orchestration."""

from __future__ import annotations

import json
import os
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator


class AdapterProfiler:
    def __init__(self) -> None:
        self.path = os.environ.get("LEXICON_ADAPTER_PROFILE", "")
        self.phases: list[dict[str, int | str]] = []
        self.counts: dict[str, int] = {}

    @contextmanager
    def measure(self, name: str) -> Iterator[None]:
        if not self.path:
            yield
            return
        started = time.perf_counter_ns()
        try:
            yield
        finally:
            self.phases.append({"name": name, "duration_ns": time.perf_counter_ns() - started})

    def set(self, name: str, value: int) -> None:
        if self.path:
            self.counts[name] = int(value)

    def write(self) -> None:
        if not self.path:
            return
        destination = Path(self.path)
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(
            json.dumps(
                {"version": 1, "phases": self.phases, "counts": self.counts},
                indent=2,
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )


adapter_profile = AdapterProfiler()
