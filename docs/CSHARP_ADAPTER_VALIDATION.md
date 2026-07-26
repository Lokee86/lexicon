# C# adapter validation

Validation date: July 25, 2026.

This record captures the first calibrated real-repository validation of the Roslyn-backed C# adapter. Counts and elapsed times are dated measurements, not permanent performance guarantees.

## Scope

The adapter emits facts-v1 repository, file, namespace, type, interface, method, constructor, field/property/event, parameter, and local-variable nodes. It emits containment, definition, dependency, inheritance, implementation, override, call, possible-call, read, and write edges, plus explicit unresolved call records.

The validation used the release-form self-contained `lexicon-csharp` executable produced by the evaluation harness. Every repository was analyzed twice. Each output passed `tools/validate_jsonl.py`, contained every required relation, and was byte-identical across both runs.

## Pinned corpus

| Split | Repository snapshot | Revision | Purpose |
| --- | --- | --- | --- |
| Calibration | Spectre.Console | `555a00ae317abc9b7a49100cda3251584b0a8861` | Broad modern C# declarations, fluent APIs, inheritance, interfaces, and overrides |
| Validation | Dapper | `72a54c475f75e18cb93cba0809d00a5e6e49efd9` | Extension-heavy and dynamic-adjacent code outside the calibration repository |
| Holdout | Polly | `9fd716c1e84288a6a8a1c7ad9426fb8a8e66942d` | Async, generic, and overload-heavy resilience pipelines |

The pinned cases are declared in `evaluation/corpus.json`. Local corpus directories carry matching `.lexicon-corpus.json` revision metadata.

## Results

| Case | Nodes | Edges | Definite calls | Possible calls | Reads | Writes | Extends | Implements | Unresolved calls | Run times |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Dapper | 9,795 | 31,610 | 5,511 | 54 | 14,569 | 1,414 | 185 | 66 | 1,837 | 34.40 s / 29.11 s |
| Polly | 30,510 | 83,611 | 5,645 | 3,678 | 35,414 | 4,084 | 368 | 181 | 25,569 | 82.02 s / 66.27 s |
| Spectre.Console | 15,339 | 46,155 | 9,137 | 190 | 19,163 | 1,737 | 238 | 131 | 1,116 | 50.01 s / 37.90 s |

All three cases reported `deterministic: true`, no missing required relations, and no unexpected nonzero relations.

## Calibration decision

The first Polly pass exposed 44,002 `possible-calls` edges from large ambiguous overload sets. Emitting every candidate materially inflated the graph without proving a useful runtime target.

The adapter now emits candidate edges only when Roslyn returns one to four defensible candidates. Larger sets become one `ambiguous-target` unresolved record carrying the candidate count, Roslyn candidate reason, and a bounded sample. On the same pinned Polly revision this reduced speculative call edges to 3,678 while preserving 1,998 large ambiguity records explicitly.

Merged namespaces and partial declarations also exposed multiple source declarations sharing one semantic identity. Node deduplication now selects a deterministic canonical source declaration instead of treating those spans as conflicting identities.

## Interpretation and limits

The adapter currently creates one repository-wide Roslyn compilation from discovered `.cs` files and trusted platform assemblies. Framework symbols resolve, but the adapter does not yet load MSBuild solution/project graphs, NuGet references, source-generator output, or conditional project configurations. Calls dependent on unavailable project or package assemblies remain unresolved rather than being guessed. This is the principal source of the holdout corpus's unresolved count and the next major precision seam.

Incremental requests analyze with complete repository context, then emit changed-file-owned records plus a complete shared set. The permanent smoke test verifies valid endpoints, required relations, byte determinism, and incremental header behavior.

## Reproduction

```text
python adapters/csharp/tests/test_adapter.py
python evaluation/run_validation.py --adapter csharp --jobs 1
```

Set `LEXICON_DOTNET` to a .NET 8 SDK executable when `dotnet` is not on `PATH`. Generated JSONL, audit samples, summaries, and the semantic report are written under `evaluation/validation/generated/` and are intentionally ignored by Git.
