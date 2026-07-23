"""Orchestrate discovery, extraction, resolution, and emission."""

from __future__ import annotations

from pathlib import Path, PurePosixPath
from typing import Any

from . import __version__
from .contract import content_id
from .discovery import _name_from_dotted, _posix_relative, discover
from .emission import emit_records, write_records
from .extraction import DeclarationVisitor
from .model import Facts
from .profiling import adapter_profile
from .resolution import resolve_facts


def build_facts(
    repo: Path,
    changed_files: list[str] | None = None,
    removed_files: list[str] | None = None,
) -> list[dict[str, Any]]:
    with adapter_profile.measure("discovery_parsing"):
        snapshot = discover(repo)
    adapter_profile.set("files_discovered", len(snapshot.contexts))
    adapter_profile.set("directories_discovered", len(snapshot.directories))
    facts = Facts(repository=snapshot.repository)
    repository_id = facts.add_node(
        "repository",
        snapshot.repository,
        ".",
        snapshot.repository,
        identity=snapshot.repository,
    )

    directory_ids: dict[str, str] = {".": repository_id}
    for directory in snapshot.directories:
        relative = _posix_relative(snapshot.root, directory)
        if relative == ".":
            continue
        directory_ids[relative] = facts.add_node(
            "directory",
            PurePosixPath(relative).name,
            relative,
            relative,
            identity=relative,
        )
    for relative, identifier in sorted(directory_ids.items()):
        if relative == ".":
            continue
        parent = PurePosixPath(relative).parent.as_posix()
        facts.add_edge(directory_ids.get(parent, repository_id), identifier, "contains")

    for context in snapshot.contexts:
        relative = context.relative_path
        context.file_id = facts.add_node(
            "file",
            context.path.name,
            relative,
            relative,
            identity=relative,
            file_content_id=content_id(context.data),
        )
        parent = PurePosixPath(relative).parent.as_posix()
        facts.add_edge(directory_ids.get(parent, repository_id), context.file_id, "contains")
        context.module_id = facts.add_node(
            "module",
            _name_from_dotted(context.module_name),
            relative,
            context.module_name,
            identity=context.module_name,
        )
        facts.modules[context.module_name] = context.module_id
        facts.add_edge(context.file_id, context.module_id, "contains")
        if context.parse_error:
            facts.add_unresolved(
                context.module_id,
                "parses",
                relative,
                "unsupported-form",
                candidate_name=context.parse_error,
            )

    with adapter_profile.measure("declaration_extraction"):
        for context in snapshot.contexts:
            if context.tree is not None:
                DeclarationVisitor(facts, context).visit(context.tree)
    with adapter_profile.measure("semantic_resolution"):
        resolve_facts(facts, snapshot.contexts)
    adapter_profile.set("files", len(snapshot.contexts))
    adapter_profile.set("nodes", len(facts.nodes))
    adapter_profile.set("edges", len(facts.edges))
    adapter_profile.set("unresolved", len(facts.unresolved))
    with adapter_profile.measure("fact_materialization"):
        return emit_records(facts, __version__, changed_files, removed_files)


def write_facts(
    repo: Path,
    output: Path,
    changed_files: list[str] | None = None,
    removed_files: list[str] | None = None,
) -> None:
    try:
        records = build_facts(repo, changed_files, removed_files)
        with adapter_profile.measure("emission"):
            write_records(records, output)
        adapter_profile.set("records", len(records))
        adapter_profile.set("output_bytes", output.stat().st_size)
    finally:
        adapter_profile.write()
