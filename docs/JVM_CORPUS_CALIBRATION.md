# Java and Kotlin corpus calibration

Calibration date: July 26, 2026.

This record establishes the first pinned real-repository corpus for the Java and Kotlin adapters. It captures deterministic coverage baselines and identifies the next calibration work. Relation counts are evidence of what the adapters emitted, not precision or recall measurements.

## Accepted corpus

| Adapter | Split | Repository | Revision | Purpose |
| --- | --- | --- | --- | --- |
| Java | Calibration | jsoup | `d24b16d952530f3aefe87e91c344b23fe4b8a7fc` | Dense parser, tree-model, nested-type, overload, and Maven source patterns |
| Java | Validation | Gson | `aebc51a56ca0793c13b841c29f73433b82446695` | Generics, adapters, reflection-oriented APIs, annotations, and multi-module Maven evidence |
| Java | Holdout | HikariCP | `a4d93f4f85517f90e632b795486d7102e933d7ff` | JDBC interfaces, concurrency-heavy implementation code, inheritance, overrides, and Maven evidence |
| Kotlin | Calibration | detekt | `f9e1d5cc239ab740ce499b1edb36b872012648e2` | Large multi-module analyzer, Gradle Kotlin DSL, extensions, fluent tests, and compiler-adjacent code |
| Kotlin | Validation | kotlinx.coroutines | `04ada74fae2e8914ae92ece34e06e80bb15385e9` | Multiplatform sources, coroutines, generics, interfaces, overrides, and Gradle dependency forms |
| Kotlin | Holdout | Now in Android | `7d45eae4f8720a0c77f507712ba2437ff974b6ed` | Android architecture, Compose-style code, dependency injection, KSP, version catalogs, and convention plugins |

`evaluation/bootstrap_corpus.py` restores these revisions beneath the workspace-level `corpus/` directory. `evaluation/corpus.json` owns their split and relation gates.

## Initial deterministic baseline

Every accepted case completed two scans, passed facts-v1 validation, emitted byte-identical output, and satisfied its required nonzero relation gates.

### Java

| Case | Seconds | Output | Calls | Possible calls | Unresolved calls | Reads | Writes | Dependencies | Extends / Implements / Overrides |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| jsoup | 54.92 / 41.87 | 32,408,772 B | 3,940 | 4,251 | 20,391 | 4,535 | 685 | 6 | 100 / 15 / 316 |
| Gson | 43.08 / 54.32 | 31,561,836 B | 3,040 | 1,852 | 17,124 | 3,121 | 705 | 23 | 96 / 100 / 192 |
| HikariCP | 10.31 / 6.59 | 10,677,741 B | 687 | 24 | 4,964 | 1,564 | 351 | 3 | 13 / 24 / 144 |

### Kotlin

| Case | Seconds | Output | Calls | Possible calls | Unresolved calls | Reads | Writes | Dependencies | Extends / Implements / Overrides |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| detekt | 66.23 / 35.00 | 67,568,131 B | 2,622 | 246 | 24,925 | 7,020 | 125 | 4 | 428 / 253 / 218 |
| kotlinx.coroutines | 23.34 / 20.05 | 38,535,322 B | 1,205 | 298 | 7,806 | 4,322 | 203 | 18 | 246 / 174 / 319 |
| Now in Android | 8.35 / 7.79 | 15,759,381 B | 212 | 6 | 1,498 | 1,039 | 32 | 0 definite / 211 unresolved | 2 / 72 / 95 |

The Now in Android dependency result is expected under the current literal manifest boundary: its build relies heavily on version catalogs, project dependencies, convention plugins, and generated/KSP surfaces, which remain unresolved rather than guessed.

## First calibration results

The first Java and Kotlin calibration slices were implemented and rerun across all six accepted repositories. Every changed stream remained deterministic and passed facts-v1 validation.

### Java typed receiver resolution

Java adapter `0.2.0` now resolves calls through explicitly typed callable parameters and simple declaration-before-use local identifiers. Receiver types use the existing package/import/lexical rules. Only directly declared, non-static repository-local methods are considered; inherited, external, ambiguous, chained, inferred, and otherwise unsupported receivers remain unresolved.

| Case | Definite calls | Possible calls | Unresolved calls |
| --- | ---: | ---: | ---: |
| jsoup | +3,621 | +975 | -4,089 |
| Gson | +2,448 | +5,423 | -3,345 |
| HikariCP | +844 | +4 | -846 |

Audit samples included `Document.head()`, `Element.text()`, `Element.html()`, `JsonWriter.beginArray()`, and `JsonWriter.endArray()`. The large Gson `possible-calls` increase is caused by same-arity overload families such as `JsonWriter.value(...)`. Those edges are explicitly uncertain rather than definite, but the fan-out is now the primary Java semantic-noise target. The next Java calibration slice should use conservative argument-type evidence to narrow obvious literal and explicitly typed identifier arguments without attempting compiler-equivalent overload resolution.

### Kotlin extension and receiver resolution

Kotlin adapter `0.4.0` now resolves repository-local extension calls through explicitly typed parameter, local, and property identifiers. Visibility follows lexical member extensions, aliases, explicit imports, same-package declarations, and wildcard imports. Ordinary members win over extensions; inferred, external, safe-call, chained, and fluent-result receivers remain unresolved.

| Case | Definite calls | Possible calls | Unresolved calls |
| --- | ---: | ---: | ---: |
| detekt | +8 | 0 | -8 |
| kotlinx.coroutines | +10 | 0 | -10 |
| Now in Android | +1 | 0 | -1 |

All 19 additions were audited as concrete repository-local bindings. Examples include `createKtFile(...)`, `getDefaultConfiguration()`, `createSpec(...)`, `list(...)`, and `listOfMaps(...)`. No holdout-only relaxation or name-only fallback was added.

## Remaining priorities

### Java scalability

Apache Maven and OpenJDK JMH were also cloned as stress repositories. Apache Maven remained CPU-bound for more than 15 minutes at roughly 428 MB without producing its first output stream. JMH also failed to complete within the bounded exploratory run. Both are excluded from the normal holdout gate until the Java adapter's large-repository runtime path is profiled and bounded.

This is a separate problem from semantic coverage. The likely investigation target is repeated global candidate scanning during runtime resolution, but profiling is required before changing the implementation.

## Commands

Restore the pinned corpus:

```text
python evaluation/bootstrap_corpus.py
```

Run the accepted JVM corpus:

```text
python evaluation/run_validation.py --adapter java --adapter kotlin --jobs 3
```

Run one case while calibrating:

```text
python evaluation/run_validation.py --case java-jsoup --jobs 1
python evaluation/run_validation.py --case kotlin-detekt --jobs 1
```

Generated JSONL, summaries, relation samples, and the combined semantic report are written under `evaluation/validation/generated/` and remain ignored by Git.

## Evidence boundary

This baseline proves deterministic execution and nonzero observable relation coverage on the pinned revisions. It does not prove compiler-equivalent binding, complete graph coverage, or edge-level precision. Any semantic change must be reviewed using both relation-count deltas and the bounded audit samples; a green gate alone is not sufficient evidence that the change improved the graph.
