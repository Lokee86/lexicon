# Java and Kotlin adapter validation

Validation date: July 26, 2026.

This record describes the initial dedicated Java and Kotlin adapters accepted into Lexicon. It records fixture and contract evidence, not a pinned real-repository precision/recall benchmark.

## Implemented validation surface

Both adapters are independently executable Go programs that emit complete facts-v1 JSONL streams without invoking a JDK, compiler, Maven, Gradle, project plugin, annotation processor, or analyzed application code.

The permanent fixture suites cover:

- deterministic repository discovery, normalized paths, permanent exclusions, and checkout-independent canonical identities;
- package/import evidence and explicit external or ambiguous unresolved records;
- Java classes, interfaces, enums, records, annotations, constructors, methods, fields, record components, and parameters;
- Kotlin classes, interfaces, objects, companion objects, enums, data/sealed/value/annotation classes, constructors, top-level/member/extension/suspend functions, properties, parameters, receivers, modifiers, and nullability metadata;
- repository-local inheritance, implementation, sealed permits, Kotlin supertypes/delegation, import aliases/wildcards, nested lexical ownership, and annotation applications;
- conservative definite and possible repository-local calls, constructors, direct-supertype overrides, and simple modeled field/property/parameter reads and writes;
- literal direct Maven dependency declarations and supported literal Gradle dependency configurations, with unsupported computed forms retained as unresolved evidence;
- valid edge endpoints, facts-v1 validation, repeated-run byte identity, stdout/file CLI behavior, and race-enabled Go tests.

## Acceptance commands

The canonical application and adapter matrix is:

```text
python evaluation/run_tests.py
```

Focused JVM verification is:

```text
go -C adapters/java test -race ./...
go -C adapters/kotlin test -race ./...
```

Generated streams are validated with:

```text
python tools/validate_jsonl.py <facts.jsonl>
python tools/semantic_report.py <facts.jsonl>
```

## Accepted July 26 run

The merged `main` branch passed the full `evaluation/run_tests.py` matrix: the Lexicon application plus Go, GDScript, generic, Java, Kotlin, Python, Ruby, C#, TypeScript, and Rust adapters. Java and Kotlin also passed their focused race-enabled suites after runtime semantics and dependency manifests were consolidated.

The release packager completed successfully and produced `adapters/java/lexicon-java.exe` and `adapters/kotlin/lexicon-kotlin.exe`. Those packaged executables analyzed their complete permanent fixture trees, emitted 243,409-byte Java and 311,430-byte Kotlin streams, and both streams passed `tools/validate_jsonl.py`.

Every Java and Kotlin Go source file remained below the 300-line repository limit. The largest Java file was 278 lines; the largest Kotlin file was 294 lines.

## Evidence boundary

The adapters use conservative source parsers and repository-local resolution rather than javac or the Kotlin compiler. A definite edge requires a unique target under the explicitly modeled lookup rules. Multiple sound local candidates become `possible-calls`; missing, external, dynamically typed, build-generated, or unsupported targets remain unresolved.

Literal Maven and Gradle parsing is manifest evidence only. The adapters do not resolve parents, properties, profiles, BOMs, dependency management, version catalogs, project dependencies, platform wrappers, source sets, plugins, transitives, or installed artifacts.

## Explicit non-claims

This validation does not establish:

- compiler-equivalent name or overload resolution;
- classpath, module-path, Android plugin, Gradle model, or Maven effective-model parity;
- generated-source, annotation-processor, KAPT, KSP, compiler-plugin, reflection, or runtime dispatch recovery;
- complete local-variable, expression-type, delegated-property, coroutine, lambda, anonymous/local-class, or framework semantics;
- real-repository precision, recall, throughput, or memory measurements.

The pinned real-repository JVM corpus and its initial measurements are recorded in [JVM_CORPUS_CALIBRATION.md](JVM_CORPUS_CALIBRATION.md). Those measurements establish deterministic coverage baselines, not compiler-equivalent precision or recall.
