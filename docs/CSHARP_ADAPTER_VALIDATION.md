# C# adapter validation

Validation date: July 26, 2026.

This record captures the calibrated real-repository validation of C# adapter version 0.2.0 with restored MSBuild project loading. Counts and elapsed times are dated measurements, not permanent performance guarantees.

## Scope

The adapter emits facts-v1 repository, file, namespace, type, interface, method, constructor, field/property/event, parameter, and local-variable nodes. It emits containment, definition, dependency, inheritance, implementation, override, call, possible-call, read, and write edges, plus explicit unresolved call records.

The validation used the release-form self-contained `lexicon-csharp` executable. A compatible .NET 10.0.302 SDK and restored project assets were supplied for project evaluation. Every repository was analyzed twice. Each output passed `tools/validate_jsonl.py`, contained every required relation, and was byte-identical across both runs.

## Pinned corpus

| Split | Repository snapshot | Revision | Purpose |
| --- | --- | --- | --- |
| Calibration | Spectre.Console | `555a00ae317abc9b7a49100cda3251584b0a8861` | Broad modern C# declarations, fluent APIs, inheritance, interfaces, and overrides |
| Validation | Dapper | `72a54c475f75e18cb93cba0809d00a5e6e49efd9` | Extension-heavy and dynamic-adjacent code outside the calibration repository |
| Holdout | Polly | `9fd716c1e84288a6a8a1c7ad9426fb8a8e66942d` | Async, generic, and overload-heavy resilience pipelines |

The pinned cases are declared in `evaluation/corpus.json`. C# cases force `--project-loading msbuild` so a validation run cannot silently fall back to file-only analysis.

## Version 0.2.0 results

| Case | Mode | Projects | Fallback files | Nodes | Edges | Definite calls | Possible calls | Unresolved calls | Run times |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Dapper | `msbuild` | 11 | 0 | 10,294 | 33,888 | 7,246 | 16 | 180 | 59.83 s / 53.28 s |
| Polly | `msbuild+files` | 21 | 11 | 31,528 | 118,175 | 31,705 | 356 | 1,257 | 146.70 s / 141.05 s |
| Spectre.Console | `msbuild+files` | 9 | 1 | 15,476 | 47,272 | 10,078 | 185 | 225 | 97.97 s / 58.85 s |

All three cases reported `deterministic: true`, no missing required relations, and no unexpected nonzero relations. All selected target frameworks were `net10.0`.

## Comparison with version 0.1.0

| Case | v0.1 unresolved calls | v0.2 unresolved calls | Change | v0.1 definite calls | v0.2 definite calls |
| --- | ---: | ---: | ---: | ---: | ---: |
| Dapper | 1,837 | 180 | -90.2% | 5,511 | 7,246 |
| Polly | 25,569 | 1,257 | -95.1% | 5,645 | 31,705 |
| Spectre.Console | 1,116 | 225 | -79.8% | 9,137 | 10,078 |

The major gain is reference availability. Project and package references now participate in Roslyn binding instead of leaving calls unresolved or forcing the adapter to infer across one synthetic assembly.

## Correctness decisions

Project loading is optional in normal `auto` mode and mandatory in corpus validation. The adapter never performs restore or build. It consumes already restored project state and falls back conservatively when project evaluation is unavailable.

Partially loadable repositories use project compilations for loaded documents and one repository-wide fallback compilation only for missing source files. The repository node records the selected mode, project count, fallback-file count, target framework, and bounded normalized diagnostics.

Separate project assemblies may legally contain identical qualified type names. Source symbol identities therefore include the containing assembly. A permanent fixture verifies that an application referencing `Lib` resolves `Shared.Worker.Execute()` to `Lib/Worker.cs`, not to an unreferenced `Other` project declaring the same qualified symbol.

When several literal target frameworks are declared, the adapter selects the newest one. This corrected an initial Dapper calibration that selected legacy `net461`, produced only 2,901 definite calls, and left 3,876 calls unresolved. Selecting `net10.0` produced the accepted 7,246 definite calls and 180 unresolved calls.

Large overload candidate sets remain bounded: one to four candidates emit `possible-calls`; larger sets emit one explicit `ambiguous-target` record with a bounded sample.

## Performance follow-up

A subsequent phase profile found repeated canonical display formatting for identical Roslyn symbols during declaration analysis. Caching qualified names by exact `ISymbol` identity and reusing the name during symbol-ID construction preserved the complete facts-v1 stream while reducing measured runtime.

| Case | Baseline | Optimized warm pass | Change |
| --- | ---: | ---: | ---: |
| Dapper | 60.57 s | 54.18 s | 10.54% lower |
| Polly | 169.45 s | 131.20 s | 22.58% lower |
| Spectre.Console | 52.00 s average | 47.96 s average | 7.78% lower |

The Spectre.Console figures are alternating-run averages. Dapper and Polly use the second optimized pass. All three optimized outputs were byte-identical to their baselines, passed JSONL validation, and were deterministic across repeated optimized runs. This optimization changes repeated formatting cost only; node identities, relationships, unresolved evidence, ordering, and repository metadata remain unchanged.

## Remaining limits

Project evaluation depends on a compatible installed SDK and existing restore assets. Workspace diagnostics remain possible for custom build projects, unavailable generated outputs, repository-versioning targets, and project references whose design-time metadata is absent. Missing project-owned files remain represented through file fallback rather than disappearing.

The adapter does not invoke a normal build, application code, reflection, or runtime dependency injection. Source-generated inputs are represented only when restored design-time project state exposes them. Conditional configurations other than the selected target framework are not exhaustively enumerated.

## Reproduction

```text
python evaluation/bootstrap_corpus.py
python evaluation/prepare_csharp_projects.py
python adapters/csharp/tests/test_adapter.py
python evaluation/run_validation.py --adapter csharp --jobs 1
```

Set `LEXICON_DOTNET` to a compatible SDK executable when `dotnet` is not on `PATH`. Generated JSONL, audit samples, summaries, and the semantic report are written under `evaluation/validation/generated/` and are intentionally ignored by Git.
