# Cross-adapter performance work

Performance pass dates: July 26-27, 2026.

This report records measured adapter optimizations and rejected experiments. Performance changes are accepted only when the facts-v1 stream remains deterministic and semantically unchanged. Dated timings are local measurements, not permanent guarantees.

## TypeScript, JavaScript, and Svelte

The dominant TypeScript cost was not parsing or JSONL rendering. Property-call dispatch resolution scanned every repository method declaration for each eligible call, and call resolution can repeat for as many as 16 fixed-point iterations. Large repositories therefore paid a repeated callsites-by-methods cost.

The adapter now maintains narrow indexes for:

- class method declarations by method name;
- module keys by module node ID;
- qualified symbol names by symbol node ID.

Call dispatch examines only the matching method-name bucket. Default-import, namespace-import, and qualified-name resolution use constant-time reverse lookups. JSONL emission also computes each fact sort key once rather than rebuilding tuple keys during every comparison.

On the pinned Lexicanter revision `eac754788b3cf18a930c085c1c49f8f353e18107`, the recorded warm pass changed from 229.11 seconds to 17.84 seconds: a 12.84x improvement and a 92.21% reduction. An immediate pre-index run under heavier machine contention took 428.03 seconds, so the historical pinned comparison is used as the more conservative headline.

The optimized validation emitted the same 41,520,048-byte facts-v1 stream with SHA-256 `6ce50aa47685e62eac6d1167a687d7a4ae2a045cc8a9d32e5b7cbb7e1483aad7`. Relation counts, unresolved counts, node counts, ordering, and repeated-run determinism were unchanged.

The smaller Space Rocks web and Workspace MCP cases completed in 6.45 and 6.78 seconds in the final concurrent corpus run. These repositories are not revision-pinned, so their timings and output counts are supporting observations rather than controlled before-and-after comparisons.

## GDScript

Phase profiling on a frozen 526-file snapshot of the Space Rocks client found that dataflow dominated runtime. The initial profiled pass spent 80.18 of 89.71 seconds in dataflow, while parsing, semantic modeling, call resolution, and rendering together accounted for less than 10 seconds.

Every identifier read called `gdscriptLocalTarget`, which scanned every repository declaration to find same-named locals in the current function. Every member read or write called `gdscriptMemberTargetID`, which performed another repository-wide declaration scan for each inferred receiver owner. The adapter now maintains narrow declaration indexes by function/name and owner/name. Lookup still applies the existing nearest-prior local, declaration-span, ambiguity, and unique-member rules; only the candidate search space changed.

The live Space Rocks checkout was being edited concurrently, so the controlled comparison used one frozen source snapshot. An alternating baseline/optimized/optimized/baseline run measured:

| Metric | Baseline | Optimized | Change |
| --- | ---: | ---: | ---: |
| Average runtime | 61.87 s | 7.43 s | 87.99% lower; 8.32x faster |
| Profiled dataflow phase | 80.18 s initial profile | 545 ms optimized profile | 99.32% lower |

All four alternating runs and the profiled optimized run emitted the same 44,219,265-byte facts-v1 stream with SHA-256 `ee51c7622f0a773701a11ce7cd26c518d2586d601af8c86a1f487a49d346c598`. The timer-free production build emitted the same stream again.

After the dataflow fix, semantic-model convergence became the largest measured phase: 5.11 seconds across nine fixed-point iterations, including 2.85 seconds of argument propagation. The follow-up pass retained evolving type, callable, and dispatch inference inside every iteration, but precomputed the static work around it:

- statement contexts, assignment positions, and declaration classification;
- parsed call sites and their enclosing contexts;
- direct child relationships for runtime subtype expansion.

Previously, argument propagation reparsed every statement's calls and rescanned file declarations to rebuild context on every iteration. Runtime receiver expansion also scanned the complete inheritance map for each receiver. The new semantic-site and child indexes change only the candidate-discovery path.

A controlled six-scan comparison used detached Space Rocks revision `8ecee944` in baseline/optimized/optimized/baseline/optimized/baseline order:

| Metric | Baseline | Optimized | Change |
| --- | ---: | ---: | ---: |
| Average runtime | 6.46 s | 3.46 s | 46.52% lower; 1.87x faster |

Every scan emitted the same 42,456,066-byte facts-v1 stream with SHA-256 `069753a087abf65651fb17905b890ec4f1e4ee10b3a3b9768cf44106b8064dac`. Focused tests also cover precomputed semantic metadata, transitive child expansion, and the lazy child-index fallback used by direct fixtures.

The final CPU profile measured semantic-model construction at about 1.60 seconds. Argument propagation was about 760 ms, declaration inference 420 ms, and statement inference 220 ms; no remaining semantic routine dominated the complete adapter run. Further GDScript optimization is therefore deferred until new profiling evidence justifies it.

## Generic fallback

The generic adapter previously rebuilt formatted composite keys inside Go sort comparators. It now computes one key per record and sorts keyed records.

A synthetic renderer benchmark containing 20,000 nodes, 20,000 edges, and 10,000 unresolved records measured:

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Render time | 347.48 ms | 242.22 ms | 30.29% lower; 1.43x faster |
| Allocated bytes | 97,188,397 | 88,625,461 | 8.81% lower |
| Allocations | 1,520,217 | 1,410,061 | 7.25% lower |

A complete generic-C scan of the Git corpus was also run before and after the change. The JSONL files were byte-identical.

## C#

Phase profiling showed that MSBuild loading remains the largest external cost, while declaration analysis repeatedly formatted the same Roslyn symbols into canonical qualified names. The adapter now caches each exact `ISymbol` display name with `SymbolEqualityComparer.Default` and reuses the already-computed name when constructing the symbol identity.

| Repository | Baseline | Optimized warm pass | Change |
| --- | ---: | ---: | ---: |
| Spectre.Console | 52.00 s | 47.96 s | 7.78% lower; 1.08x faster |
| Dapper | 60.57 s | 54.18 s | 10.54% lower; 1.12x faster |
| Polly | 169.45 s | 131.20 s | 22.58% lower; 1.29x faster |

The Spectre.Console result is the average of an alternating baseline/optimized/optimized/baseline run. Dapper and Polly report the second optimized validation pass against the immediately preceding baseline. Machine load varied, so the individual elapsed times are dated evidence rather than guarantees.

All three optimized outputs were byte-identical to their baselines and deterministic across repeated optimized runs:

- Spectre.Console: 25,634,972 bytes, SHA-256 `851cb444f14e6a75fdd9f901526cdd22f1c022abd18330871f4b5daf70a3126f`;
- Dapper: 16,523,009 bytes, SHA-256 `6a8a157ed67af158f05c8b766f6aff2dbc5ca2e6ee788d37d9ea62fc0b54d1fc`;
- Polly: 61,639,129 bytes, SHA-256 `dd2b2037d3ac3600aca777271dc71ee14df6f5e512a339c2d6c1cc1d253b26b5`.

Two broader experiments were rejected. Caching source ownership by syntax scope changed project-specific symbol identities in repositories where Roslyn compilations share syntax nodes. Reusing serialized fact keys preserved output but did not produce a repeatable end-to-end improvement. Neither experiment was retained.

## Rust rejected experiment

Rust already used `sort_by_key`, which computes an owned fact key repeatedly during sorting. Replacing it with `sort_by_cached_key` appeared analogous to the Go renderer changes, but the controlled corpus comparison rejected it.

| Repository | Existing warm pass | Cached-key warm pass | Result |
| --- | ---: | ---: | ---: |
| Arcana | 142.43 s | 177.11 s | 24.35% slower |
| Grimoire vector engine | 1.16 s | 1.50 s | 29.33% slower |

Both versions emitted identical deterministic facts-v1 streams. The cached-key change was discarded. Rust's next performance work must profile semantic extraction and resolution rather than assume rendering is dominant.

## Adapter survey

The exact Java/Kotlin defect is not present throughout the remaining fleet:

- GDScript already uses keyed-record sorting; its measured bottleneck was repeated repository-wide declaration lookup during dataflow;
- Python's `sorted(..., key=...)` computes each key once;
- Ruby's `sort_by` computes each key once;
- Go compares typed fields and spans directly rather than serializing records in comparators;
- C# benefits from exact Roslyn-symbol display-name caching, while broader syntax-scope caches are unsafe across project compilations.

The reusable rule is broader than a specific sorting implementation: measure repeated work inside hot loops, add the narrowest concrete index or cached representation, and require unchanged deterministic facts before integration.

## Verification

Accepted changes passed:

- root `go test -race ./... -count=1`;
- generic-adapter `go test -race ./... -count=1`;
- TypeScript `npm test`;
- full TypeScript corpus validation with no failures;
- repeated deterministic facts-v1 validation;
- byte-identical generic-C corpus comparison;
- GDScript race-enabled adapter tests, focused semantic-site/index regressions, and byte-identical frozen Space Rocks baseline/optimized comparisons;
- C# smoke, MSBuild graph, relation, incremental, and determinism checks;
- byte-identical baseline/optimized comparisons across Spectre.Console, Dapper, and Polly.

The Rust, C# source-ownership-cache, and C# renderer experiments passed their applicable semantic checks but were rejected because they were slower or changed canonical output.
