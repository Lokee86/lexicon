# Java and Kotlin comparison with Codebase Memory

Comparison date: July 26, 2026.

This report compares Lexicon Java `0.3.0` and Kotlin `0.4.0` with Codebase Memory `0.9.0` on the same six pinned repositories used by the JVM corpus. The products optimize for different purposes: Lexicon emits a deterministic facts-v1 evidence stream with explicit certainty boundaries, while Codebase Memory builds a fast exploration graph containing semantic, heuristic, testing, complexity, and architecture-oriented relationships.

The comparison therefore does not treat total node or edge count as a quality score. It measures runtime, normalizes calls where possible, and audits concrete language cases where exact target identity matters.

## Corpus and method

Both tools scanned these pinned repositories:

- Java: jsoup, Gson, and HikariCP;
- Kotlin: detekt, kotlinx.coroutines, and Now in Android.

Codebase Memory ran in `full` mode with persistence disabled. Lexicon measurements use the second deterministic validation pass, which is the more favorable warm result. Codebase Memory stores one `CALLS` edge per source-target pair; Lexicon retains callsite records and can emit repeated pairs at distinct spans. Raw call totals are therefore reported alongside normalized Lexicon pair counts rather than treated as directly equivalent.

Codebase Memory `full` mode also adds `SIMILAR_TO` and `SEMANTICALLY_RELATED` edges. Those are useful for exploration but are not equivalent to Lexicon's language-semantic facts.

## Runtime

| Repository | Codebase Memory | Lexicon warm pass | Lexicon / CBM |
| --- | ---: | ---: | ---: |
| jsoup | 2.71 s | 20.20 s | 7.46x |
| Gson | 2.60 s | 18.36 s | 7.06x |
| HikariCP | 1.25 s | 6.88 s | 5.52x |
| detekt | 8.87 s | 31.16 s | 3.51x |
| kotlinx.coroutines | 6.84 s | 18.34 s | 2.68x |
| Now in Android | 2.45 s | 6.04 s | 2.46x |

Across all six repositories, Codebase Memory completed in 24.71 seconds and Lexicon's warm passes completed in 100.99 seconds. Codebase Memory was 4.09 times faster overall, 6.93 times faster on Java, and 3.06 times faster on Kotlin.

This is a decisive Codebase Memory advantage. Lexicon's current deterministic parsing, canonical fact construction, unresolved-evidence retention, and JSONL materialization cost substantially more CPU and output work.

## Structural model differences

Codebase Memory produced a broader navigation graph with complexity metrics, test relationships, configuration and HTTP relationships, usage edges, and heuristic similarity edges. These are valuable capabilities that Lexicon does not currently attempt to provide.

Lexicon retains language evidence that the compared Codebase Memory graphs did not represent as separate relationship classes:

- distinct `extends`, `implements`, and `overrides` relationships rather than a combined `INHERITS` edge;
- literal Maven and Gradle `depends-on` evidence;
- separate definite `calls`, `possible-calls`, and unresolved call records;
- unresolved expressions, source spans, and reasons such as `external-target`, `dynamic-target`, and `unsupported-form`;
- signature-qualified callable identities and parameter nodes.

No `DEPENDS_ON` edges appeared in any of the six Codebase Memory JVM graphs. The Java and Kotlin graphs also had no separate implementation or override edge types.

Codebase Memory does preserve useful uncertainty metadata on calls through `confidence` and `strategy`. However, every selected target is still represented as a `CALLS` edge, so consumers must apply their own confidence policy. Lexicon makes the certainty boundary structural and does not require a downstream threshold to distinguish proved, possible, and unresolved relationships.

## Java overload audit

Gson's `JsonWriter` declares seven `value(...)` overloads:

- `value(String)`;
- `value(boolean)`;
- `value(Boolean)`;
- `value(float)`;
- `value(double)`;
- `value(long)`;
- `value(Number)`.

Lexicon emitted seven independent method nodes with signature-qualified identities.

The Codebase Memory graph exposed one `JsonWriter.value` method node, carrying the final `(Number value)` signature. Its incoming calls included boolean, boxed-boolean, integral, float, and double test cases, all targeting that single node. Seven audited calls used confidence `0.95` and strategy `lsp_type_dispatch`; this included `testBooleans` and `testBoxedBooleans`, which should not bind to `value(Number)`.

Lexicon correctly bound obvious boolean literals to `value(boolean)` and integral literals to `value(long)`. It remains deliberately conservative for more complex boxed-Boolean expressions, leaving multiple overloads as `possible-calls` instead of claiming compiler-equivalent resolution.

This audit is more important than the aggregate Java call count. Codebase Memory's larger call graph does not imply overload-aware target coverage when overload declarations share one graph identity.

## Kotlin extension audit

The Kotlin calibration previously audited six repository-local extension or top-level bindings in detekt:

- two calls to `createKtFile(String, Boolean)`;
- `getDefaultConfiguration()`;
- `createSpec(Appendable, Appendable)`;
- `list(String, List<String>)`;
- `listOfMaps(String, List<Map<String, String?>>)`.

Lexicon emitted all six as definite callable-owned, signature-specific calls.

For the same names, Codebase Memory emitted one `createKtFile` call. Its source was the file pseudo-node rather than either enclosing test method, and it used `same_module` at confidence `0.90`. It emitted no calls to the other four audited extension targets.

Codebase Memory's aggregate Kotlin call counts are much larger, but most are heuristic. Calls below confidence `0.5` accounted for:

- 68.4% of detekt calls;
- 78.9% of kotlinx.coroutines calls;
- 48.2% of Now in Android calls.

The dominant Kotlin strategies were `suffix_match`, `unique_name`, and `same_module`. Compiler-oriented Kotlin strategies represented a small minority. This makes the graph useful for broad navigation and recall, but not interchangeable with Lexicon's conservative facts-v1 evidence.

## Normalized call breadth

| Repository | CBM call pairs | Lexicon definite pairs | Lexicon definite + possible pairs | Lexicon unresolved callsites |
| --- | ---: | ---: | ---: | ---: |
| jsoup | 12,056 | 5,799 | 9,851 | 16,302 |
| Gson | 7,444 | 4,219 | 8,090 | 13,779 |
| HikariCP | 2,518 | 1,293 | 1,319 | 4,118 |
| detekt | 15,325 | 2,492 | 2,733 | 24,917 |
| kotlinx.coroutines | 30,140 | 1,094 | 1,381 | 7,796 |
| Now in Android | 3,007 | 160 | 162 | 1,497 |

Java breadth is relatively close once Lexicon's possible targets are included, although the target identities and certainty models differ. Kotlin is intentionally much narrower. Lexicon refuses to convert the many inferred, chained, safe-call, fluent-result, external, and default-library cases into repository-local targets without bounded evidence.

## Conclusion

The comparison supports keeping both architectural approaches distinct.

Codebase Memory is substantially faster and produces a richer exploration graph. Its complexity properties, testing relationships, configuration detection, HTTP relationships, and heuristic recall are useful reference points for future Lexicon consumers or a separate non-authoritative navigation layer.

Lexicon's JVM adapters are stronger for conservative semantic evidence. Java preserves overload identity and exposes unresolved applicability instead of collapsing overload families. Kotlin resolves the tested explicitly typed repository-local extension slice more precisely and assigns edges to the actual enclosing callable. Lexicon also preserves implementation, override, dependency, possible-target, and unresolved evidence that was absent or combined in the compared Codebase Memory graphs.

The adapters should therefore be considered successful for Lexicon's intended contract, but not finished in an absolute sense. The comparison identifies two clear priorities:

1. reduce Java and Kotlin runtime without weakening canonical identities or certainty boundaries;
2. expand Kotlin receiver and extension evidence in bounded slices while keeping heuristic navigation separate from facts-v1.

A useful future design is an optional exploration overlay with confidence and strategy metadata, influenced by Codebase Memory's breadth, while retaining Lexicon's current facts-v1 stream as the authoritative semantic layer.
