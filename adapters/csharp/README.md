# C# adapter

The C# adapter uses Roslyn syntax trees and semantic models to emit Lexicon facts-v1 JSONL.

## Coverage

- repository, file, namespace, type, interface, method, constructor, field/property/event, parameter, and local-variable nodes
- containment and definition edges
- `depends-on` edges from `using` directives
- `extends`, `implements`, and `overrides` edges
- definite and candidate call edges
- unresolved call records when Roslyn cannot bind a target
- field, property, parameter, and local-variable reads and writes
- deterministic full and owner-filtered incremental output

The adapter analyzes all repository-owned `.cs` files in one Roslyn compilation. It references the runtime trusted platform assemblies, so framework symbols resolve without loading a solution. NuGet/project references are not yet loaded through MSBuild; calls that depend on unavailable external assemblies remain unresolved rather than being guessed.

## Run

```text
dotnet run --project adapters/csharp/Lexicon.CSharp.csproj -- \
  --repo <repository> \
  --output <facts.jsonl>
```

Incremental scope uses repeated `--changed-file` and `--removed-file` arguments with normalized repository-relative paths.

## Test

```text
python adapters/csharp/tests/test_adapter.py
```

Set `LEXICON_DOTNET` when `dotnet` is not on `PATH`.
