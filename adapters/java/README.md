# Java adapter

The Java adapter is a self-contained Go executable that reads Java source as data and emits the Lexicon facts-v1 JSONL contract. It does not load classes, invoke build tools, evaluate manifests, run annotation processors, or execute analyzed project code.

## Build

Go 1.26 or newer is required. From this directory:

```text
go build -o lexicon-java .
```

No JDK, Java dependency download, or analyzed-project build is required.

## Usage

```text
lexicon-java --repo <repository-root> --output <facts.jsonl>
```

Use `--output -` (the default) for stdout. The adapter currently emits complete `mode: full` streams only. Output is UTF-8 JSONL with one header followed by canonically sorted nodes, edges, and unresolved records. JSON object keys are lexicographically ordered.

For source-tree execution:

```text
go -C adapters/java run . --repo /path/to/repository --output /tmp/java-facts.jsonl
```

## Modeled semantics

The foundation models:

- repository, directory, Java file, compilation-unit module, and Java package namespace nodes;
- source package membership and import declarations;
- repository-local exact type imports, package wildcards, static member imports, and static type wildcards when there is one safe target;
- class, interface, enum, record, and annotation declarations;
- nested member types;
- ordinary and compact constructors, methods, fields, record components, and callable parameters;
- file/module, package/module, declaration/member, and callable/parameter containment;
- declared `extends` and `implements` relationships from type headers;
- sealed-type `permits` references marked with `role: permitted-subtype`;
- annotation applications on modeled declarations;
- explicit unresolved relationship, `imports`, or `defines` evidence rather than guessed targets.

Classes, enums, and records use the common `type` node kind. Interfaces and annotation declarations use `interface`. Their exact Java surface is retained in `attributes.declaration_kind`. Record components use `field` with `declaration_kind: record-component`. Parameters preserve source order through their index and parent callable identity.

Import nodes are defined by their compilation-unit module. An import node emits `imports` to one repository-local declaration when that declaration is unique. Imports that require the JDK, third-party classpaths, generated code, or unavailable source remain `external-target`; duplicate safe candidates remain `ambiguous-target`. Package declarations are represented by package namespace nodes and package-to-module containment.

Declared type-header references resolve only to repository-local type declarations. The resolver considers an exact qualified name, the compilation unit's package, explicit non-static imports, non-static wildcard imports, and enclosing lexical type owners. It emits an edge only when the resulting target is unique; absent and ambiguous targets remain `external-target` and `ambiguous-target` unresolved records. `extends` clauses emit `extends`, `implements` clauses emit `implements`, and `permits` clauses emit `references` with `role: permitted-subtype` from the sealed declaration to the permitted declaration.

Annotations on modeled types, constructors, methods, fields, record components, and parameters use the same conservative lookup. A uniquely resolved repository-local annotation declaration is the target of an `annotates` edge from the annotated declaration. External annotations and ambiguous local annotation names remain unresolved `annotates` evidence, preserving the source expression and span.

## Canonical identities and paths

All IDs are lowercase SHA-256 values over the facts-v1 payload:

```text
lexicon:v1\0java\0<kind>\0<canonical-identity>
```

Canonical identities are:

- repository: repository directory name;
- directory, file, and compilation-unit module: normalized repository-relative path;
- package namespace: dotted package name, or `<default>`;
- type/interface: dotted package plus lexical containing types and simple name;
- method: containing type plus method name and normalized ordered parameter-type signature;
- constructor: containing type plus `<init>` and normalized ordered parameter-type signature;
- field and record component: containing type plus field name;
- parameter: callable identity plus source-order index and parameter name;
- import: repository-relative file, source line/column, and imported target.

Content IDs hash the unmodified file bytes. Absolute checkout paths never enter identities or emitted paths. Paths and owners are normalized repository-relative forward-slash paths; `.` is used only for synthetic repository/default-package placement.

## Discovery and permanent exclusions

Discovery walks deterministically without following directory symlinks and reads only `.java` files. Directory and source lists are sorted before analysis. Matching is case-insensitive for the permanent excluded directory names.

The adapter excludes Git/worktree metadata (`.git`, `.worktrees`, `.workingtrees`), Lexicon and Warlock state (`.ddocs`, `.lexicon`, `.arcana`, `.grimoire`, `.pitlord`, `.cantrip`, `.homunculus`, `.incubus`, `.ritual`, `.warlock`), dependency/cache trees (`.bundle`, `.cache`, `.gradle`, `.idea`, `.mvn`, `.next`, `node_modules`, `packages`, `vendor`), and build/output trees (`bin`, `build`, `coverage`, `dist`, `generated`, `generated-sources`, `obj`, `out`, `target`, `temp`, `tmp`).

## Current boundaries

This adapter uses a deterministic conservative Java lexer and declaration parser rather than javac attribution. It does not model implicit or transitive inheritance, overrides, calls, dataflow, module descriptors, local/anonymous classes, local variables, build dependency manifests, classpath resolution, generated sources, or incremental scope. It does not execute a compiler or infer targets from same-named declarations outside the supported package/import/lexical lookup. Annotation values, target legality, repeatable expansion, inherited annotations, and type-use placement are not semantically interpreted.

Malformed literals/comments, unclosed type/member bodies, malformed package/import syntax, and member forms that cannot be classified safely remain explicit `unsupported-form` unresolved records. Valid declarations already isolated before a later malformed region may still be emitted; the unresolved record preserves the unsupported boundary. Invalid UTF-8 or NUL-containing files retain file/module evidence and emit an unresolved `defines` record.

## Test

```text
go -C adapters/java test ./...
```

The permanent fixtures cover packages, local and external imports, static imports, every modeled type form, ordinary and compact constructors, methods, multi-field declarations, record components, parameters, nested types, malformed syntax, permanent exclusions, declared inheritance and implementation, sealed permits, local and external annotations, exact/package/explicit/wildcard/lexical resolution, ambiguous targets, valid endpoints, canonical identities and ordering, byte determinism, checkout-path independence, stdout, and file output. A generated stream can also be checked with:

```text
python tools/validate_jsonl.py /path/to/facts.jsonl
python tools/semantic_report.py /path/to/facts.jsonl
```
