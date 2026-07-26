# C# adapter

The C# adapter uses Roslyn syntax trees, semantic models, and optional MSBuild project evaluation to emit Lexicon facts-v1 JSONL.

## Coverage

- repository, file, namespace, type, interface, method, constructor, field/property/event, parameter, and local-variable nodes
- containment and definition edges
- `depends-on` edges from `using` directives
- `extends`, `implements`, and `overrides` edges
- definite and bounded candidate call edges
- explicit unresolved and ambiguous call records
- field, property, parameter, and local-variable reads and writes
- deterministic full and owner-filtered incremental output

## Project loading

`--project-loading auto` is the default. When the repository contains C# projects and a compatible .NET SDK is available, the adapter loads restored project state through `MSBuildWorkspace`. This supplies project references, package metadata references, target-framework configuration, generated compile inputs already present in project state, and per-project Roslyn compilations.

The adapter does not invoke `restore`, a normal build, or analyzed application code. Project assets must already exist. Design-time compilations may include generated inputs already exposed by restored project state. If project loading is unavailable or fails completely, `auto` falls back to a repository-wide file compilation. If only some source files are absent from loaded projects, those files use the conservative file-compilation fallback while project-owned files retain project semantics.

Use `--project-loading msbuild` to require a project graph and fail when none can be loaded. Use `--project-loading files` to force the original repository-wide compilation. The repository node records the selected mode, loaded-project count, fallback-file count, selected target framework, and bounded workspace diagnostics.

When projects target several frameworks, the adapter selects the newest literal target framework declared by the repository. Set `LEXICON_CSHARP_TARGET_FRAMEWORK` to force another target. `LEXICON_DOTNET` may point at the SDK executable when `dotnet` is not on `PATH`; `DOTNET_ROOT` is also honored.

Source symbol identities include their containing assembly. This prevents unrelated projects that declare the same qualified type name from collapsing into one node.

## Run

```text
dotnet run --project adapters/csharp/Lexicon.CSharp.csproj -- \
  --repo <repository> \
  --output <facts.jsonl> \
  --project-loading auto
```

Incremental scope uses repeated `--changed-file` and `--removed-file` arguments with normalized repository-relative paths. Project loading still uses complete repository context before owner-filtered emission.

## Test

```text
python adapters/csharp/tests/test_adapter.py
```

The permanent fixture verifies file mode, restored project loading, cross-project call resolution, duplicate qualified names in separate assemblies, repository-relative paths, valid edge endpoints, incremental headers, and byte determinism.
