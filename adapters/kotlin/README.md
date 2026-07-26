# Kotlin adapter

The Kotlin adapter is a self-contained Go executable that reads Kotlin source text and emits the Lexicon facts-v1 JSONL contract. It uses a deterministic, non-executing structural lexer and parser: it does not run application code, Gradle, Kotlin scripts, compiler plugins, annotation processors, or repository build logic.

## Build

Go 1.22 or newer is required.

```text
cd adapters/kotlin
go build -o lexicon-kotlin-adapter .
```

No Kotlin, Java, Gradle, or external Go dependency is required.

## Usage

Write facts to a file:

```text
adapters/kotlin/lexicon-kotlin-adapter \
  --repo <repository-root> \
  --output <facts.jsonl>
```

Use `--output -` for stdout:

```text
cd adapters/kotlin
go run . --repo <repository-root> --output -
```

The adapter emits a complete `mode: "full"` repository view. Incremental owner-filtered emission is not currently implemented.

## Modeled semantics

The foundation emits:

- repository, source-ancestor directory, `.kt`/`.kts` file, source-module, and package namespace nodes;
- package declaration evidence from each source module to its namespace;
- import declaration nodes, including aliases and wildcard metadata, plus `imports` edges to deterministic syntactic target nodes;
- class, interface, object, companion object, enum class, data class, sealed class/interface, value class, and annotation-class declarations;
- explicit primary constructors (including the implicit empty constructor) and secondary constructors;
- top-level functions and member methods;
- top-level, member, extension, and primary-constructor properties, represented by the common facts-v1 `field` kind;
- callable and constructor parameters;
- `contains` and `defines` ownership edges for declarations and parameters;
- annotations and sorted Kotlin modifiers when present;
- `suspend`, extension receiver, declared type, return type, default-parameter, mutability, and nullability metadata where syntactically available;
- explicit `unresolved` records with relation `defines`, reason `unsupported-form`, and parser diagnostics for malformed delimiters, malformed declarations, unsupported destructuring properties, unterminated literals/comments, and other declaration syntax the structural parser cannot interpret safely.

Enum entries, property accessors, local declarations, and initializer/body expressions are not emitted as declarations. Their text is skipped without execution.

## Canonical identities

Every node ID is the facts-v1 digest of:

```text
lexicon:v1\0kotlin\0<kind>\0<canonical identity>
```

Canonical identities never contain an absolute checkout path:

- repository: repository directory name;
- directory and file: normalized repository-relative path;
- source module: `source:<repository-relative path>`;
- package namespace: `package:<qualified package>`;
- import: source path, imported name, alias, and source-order occurrence;
- external syntactic import target: `external-import:<imported name>`;
- declaration: source-module identity followed by the declaration ownership chain and declaration kind/name;
- function/method: declaration owner, name, extension receiver, and declared parameter-type signature;
- constructor: containing declaration identity and declared parameter-type signature;
- property: declaration owner, name, and extension receiver;
- parameter: callable/constructor identity, zero-based position, and declared name.

Qualified names use the declared Kotlin package and nested declaration names. Callable qualified names include declared parameter types. Repository-relative source ownership and spans remain attached to every source-derived record.

Duplicate malformed declarations that have the same canonical source identity receive a deterministic source-order suffix. Valid Kotlin overloads remain distinct through their declared receiver and parameter types.

## Discovery and permanent exclusions

Discovery recursively scans regular `.kt` and `.kts` files. Paths are normalized to repository-relative forward-slash form and sorted before parsing. Symlinks are not followed.

The following directory names are permanently excluded case-insensitively:

```text
.git .worktrees .workingtrees
.lexicon .arcana .grimoire .pitlord .cantrip .homunculus
.incubus .ritual .warlock .ddocs
.idea .vscode .vs
.gradle .kotlin
node_modules packages vendor .bundle
build dist out target artifacts generated bin obj coverage tmp temp
__pycache__ .pytest_cache .godot .import
```

These exclusions cover repository metadata, linked-worktree metadata, Warlock state, dependency/vendor trees, IDE state, language caches, and generated/build output. A source file under an excluded directory is never analyzed.

## Determinism

Files are discovered in canonical path order. Node identities use SHA-256, attributes use deterministic scalar values or sorted arrays, duplicate records are keyed canonically, and emission follows facts-v1 ordering: header, nodes, edges, then unresolved records. JSON object keys are lexicographically ordered by Go's JSON encoder. Identical source bytes and repository directory identity produce byte-identical output.

## Tests

```text
cd adapters/kotlin
go test ./...
```

The permanent foundation fixture covers package/import evidence, aliases, wildcard imports, classes, interfaces, objects, companion objects, enum/data/sealed/value classes, primary/secondary/implicit constructors, top-level/member/extension/suspend functions, properties, parameters, nullability, exclusions, malformed syntax, valid edge endpoints, repository-relative paths, CLI file/stdout behavior, canonical IDs, and byte determinism.

## Current boundaries

This foundation is syntax-structural rather than compiler-semantic. It deliberately does not:

- evaluate Gradle settings, builds, source sets, dependencies, compiler plugins, or Kotlin scripts;
- invoke the Kotlin compiler or resolve symbols/types across source files or libraries;
- infer inheritance/implementation targets, calls, reads, writes, overrides, or dependency manifests;
- model enum entries, type parameters, local declarations, delegated-property semantics, accessors, contracts, annotations as separate nodes, or generated declarations;
- recover an unsafe declaration by guessing its name or shape.

Imports retain source evidence but target deterministic external syntactic symbols; they are not claims of successful compiler resolution. Unsupported or malformed syntax is preserved as unresolved evidence instead of being silently converted into a declaration. These boundaries keep the adapter safe and deterministic while leaving compiler-backed resolution and additional relationship/dataflow surfaces for later slices.
