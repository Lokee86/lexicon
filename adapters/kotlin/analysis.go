package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type analysis struct {
	facts          *factSet
	moduleByPath   map[string]string
	namespaceByQN  map[string]string
	repositoryID   string
	repositoryName string
}

func analyzeRepository(repository string) ([]byte, error) {
	_, repositoryName, sources, err := discoverSources(repository)
	if err != nil {
		return nil, err
	}
	facts := newFactSet()
	state := &analysis{
		facts: facts, moduleByPath: make(map[string]string), namespaceByQN: make(map[string]string), repositoryName: repositoryName,
	}
	state.repositoryID = facts.addNode(
		"repository", repositoryName, repositoryName, ".", repositoryName, "", nil,
		map[string]any{"analysis_mode": "structural", "source_file_count": len(sources)},
	)
	state.emitDirectories(sources)

	parsed := make([]*parsedKotlinFile, 0, len(sources))
	for _, source := range sources {
		file := parseKotlinFile(source.path, source.content)
		if err := validateParsedFile(file); err != nil {
			return nil, err
		}
		parsed = append(parsed, file)
		fileSpan := wholeFileSpan(source.path, source.content)
		fileID := facts.addFileNode(source.path, source.content, &fileSpan)
		parentID := state.parentDirectoryID(source.path)
		facts.addEdge(parentID, fileID, "contains", source.path, nil, nil)

		packageName := file.packageName
		if packageName == "" {
			packageName = "<default>"
		}
		moduleQN := sourceModuleQualifiedName(packageName, source.path)
		moduleAttributes := map[string]any{"package": packageName, "script": strings.HasSuffix(strings.ToLower(source.path), ".kts")}
		moduleID := facts.addNode("module", "source:"+source.path, strings.TrimSuffix(baseName(source.path), filepath.Ext(source.path)), source.path, moduleQN, source.path, &fileSpan, moduleAttributes)
		state.moduleByPath[source.path] = moduleID
		facts.addEdge(fileID, moduleID, "contains", source.path, nil, nil)

		namespaceID := state.ensureNamespace(packageName)
		packageSpan := file.packageSpan
		facts.addEdge(moduleID, namespaceID, "defines", source.path, packageSpan, map[string]any{"evidence": "package-directive"})
	}

	for _, file := range parsed {
		state.emitImports(file)
		ownerID := state.moduleByPath[file.path]
		packageName := file.packageName
		if packageName == "" {
			packageName = "<default>"
		}
		occurrences := make(map[string]int)
		for _, declaration := range file.declarations {
			state.emitDeclaration(file, declaration, ownerID, packageName, "source:"+file.path, "module", occurrences)
		}
		for _, diagnostic := range file.diagnostics {
			span := sourceSpan{
				EndColumn: diagnostic.token.endColumn, EndLine: diagnostic.token.endLine, Path: file.path,
				StartColumn: diagnostic.token.startColumn, StartLine: diagnostic.token.startLine,
			}
			facts.addUnresolved(
				ownerID, "defines", diagnosticExpression(file.content, diagnostic), "unsupported-form", file.path, &span,
				diagnosticAttributes(diagnostic),
			)
		}
	}
	return facts.render(repositoryName)
}

func (state *analysis) emitDirectories(sources []sourceFile) {
	directories := map[string]struct{}{}
	for _, source := range sources {
		directory := pathDirectory(source.path)
		for directory != "." && directory != "" {
			directories[directory] = struct{}{}
			directory = pathDirectory(directory)
		}
	}
	ordered := make([]string, 0, len(directories))
	for directory := range directories {
		ordered = append(ordered, directory)
	}
	sort.Slice(ordered, func(i, j int) bool {
		leftDepth := strings.Count(ordered[i], "/")
		rightDepth := strings.Count(ordered[j], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return ordered[i] < ordered[j]
	})
	for _, directory := range ordered {
		id := state.facts.addNode("directory", directory, baseName(directory), directory, directory, "", nil, nil)
		parent := pathDirectory(directory)
		parentID := state.repositoryID
		if parent != "." && parent != "" {
			parentID = stableID("directory", parent)
		}
		state.facts.addEdge(parentID, id, "contains", "", nil, nil)
	}
}

func (state *analysis) parentDirectoryID(path string) string {
	directory := pathDirectory(path)
	if directory == "." || directory == "" {
		return state.repositoryID
	}
	return stableID("directory", directory)
}

func (state *analysis) ensureNamespace(qualifiedName string) string {
	if existing := state.namespaceByQN[qualifiedName]; existing != "" {
		return existing
	}
	name := qualifiedName
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		name = name[index+1:]
	}
	id := state.facts.addNode(
		"namespace", "package:"+qualifiedName, name, ".", qualifiedName, "", nil,
		map[string]any{"language_construct": "package"},
	)
	state.namespaceByQN[qualifiedName] = id
	state.facts.addEdge(state.repositoryID, id, "contains", "", nil, nil)
	return id
}

func (state *analysis) emitImports(file *parsedKotlinFile) {
	moduleID := state.moduleByPath[file.path]
	for index, imported := range file.imports {
		localName := imported.alias
		if localName == "" {
			localName = imported.path
			if dot := strings.LastIndexByte(localName, '.'); dot >= 0 {
				localName = localName[dot+1:]
			}
		}
		canonical := fmt.Sprintf("%s::import::%s::%s::%d", file.path, imported.path, imported.alias, index)
		attributes := map[string]any{
			"alias": imported.alias, "imported": imported.path, "wildcard": imported.wildcard,
		}
		importID := state.facts.addNode("import", canonical, localName, file.path, imported.path, file.path, &imported.span, attributes)
		state.facts.addEdge(moduleID, importID, "defines", file.path, &imported.span, nil)
		targetAttributes := map[string]any{"external": true, "resolution": "syntactic-import-target"}
		targetID := state.facts.addNode("symbol", "external-import:"+imported.path, localName, "external", imported.path, "", nil, targetAttributes)
		edgeAttributes := map[string]any{"alias": imported.alias, "wildcard": imported.wildcard}
		state.facts.addEdge(importID, targetID, "imports", file.path, &imported.span, edgeAttributes)
	}
}

func (state *analysis) emitDeclaration(file *parsedKotlinFile, declaration *declaration, ownerID, ownerQN, ownerCanonical, ownerKind string, occurrences map[string]int) string {
	kind := declaration.kind
	if kind == "function" && (ownerKind == "type" || ownerKind == "interface") {
		kind = "method"
	}
	qualifiedBase := qualify(ownerQN, declaration.name)
	canonical := ""
	qualifiedName := qualifiedBase
	switch kind {
	case "function", "method":
		signature := parameterSignature(declaration.parameters)
		receiver := declaration.receiver
		canonical = fmt.Sprintf("%s::callable:%s::receiver:%s(%s)", ownerCanonical, declaration.name, receiver, signature)
		qualifiedName = fmt.Sprintf("%s(%s)", qualifiedBase, signature)
	case "constructor":
		signature := parameterSignature(declaration.parameters)
		canonical = fmt.Sprintf("%s::constructor(%s)", ownerCanonical, signature)
		qualifiedName = fmt.Sprintf("%s.<init>(%s)", ownerQN, signature)
		declaration.name = simpleQualifiedName(ownerQN)
	case "field":
		canonical = fmt.Sprintf("%s::property:%s::receiver:%s", ownerCanonical, declaration.name, declaration.receiver)
	default:
		canonical = fmt.Sprintf("%s::%s:%s", ownerCanonical, kind, declaration.name)
	}
	canonical = disambiguateCanonical(canonical, occurrences)
	attributes := declarationAttributes(declaration)
	id := state.facts.addNode(kind, canonical, declaration.name, file.path, qualifiedName, file.path, &declaration.span, attributes)
	state.facts.addEdge(ownerID, id, "contains", file.path, &declaration.span, nil)
	state.facts.addEdge(ownerID, id, "defines", file.path, &declaration.span, nil)

	if kind == "function" || kind == "method" || kind == "constructor" {
		for parameterIndex, parameter := range declaration.parameters {
			parameterCanonical := fmt.Sprintf("%s::parameter::%04d:%s", canonical, parameterIndex, parameter.name)
			parameterQN := fmt.Sprintf("%s::parameter:%s", qualifiedName, parameter.name)
			parameterAttributes := parameterAttributes(parameter, parameterIndex)
			parameterID := state.facts.addNode("parameter", parameterCanonical, parameter.name, file.path, parameterQN, file.path, &parameter.span, parameterAttributes)
			state.facts.addEdge(id, parameterID, "contains", file.path, &parameter.span, nil)
			state.facts.addEdge(id, parameterID, "defines", file.path, &parameter.span, nil)
		}
	}

	childOccurrences := make(map[string]int)
	for _, child := range declaration.children {
		state.emitDeclaration(file, child, id, qualifiedBase, canonical, declaration.kind, childOccurrences)
	}
	if declaration.kind == "type" {
		for _, parameter := range declaration.parameters {
			if !parameter.property {
				continue
			}
			property := constructorParameterProperty(parameter)
			propertyID := state.emitDeclaration(file, property, id, qualifiedBase, canonical, declaration.kind, childOccurrences)
			if record := state.facts.nodes[propertyID]; record != nil {
				attrs, _ := record["attributes"].(map[string]any)
				if attrs == nil {
					attrs = make(map[string]any)
					record["attributes"] = attrs
				}
				attrs["constructor_parameter"] = true
			}
		}
	}
	return id
}

func constructorParameterProperty(parameter parameterDecl) *declaration {
	return &declaration{
		form: "constructor_parameter_property", kind: "field", mutable: parameter.mutable,
		name: parameter.name, span: parameter.span, typeName: parameter.typeName,
	}
}

func declarationAttributes(declaration *declaration) map[string]any {
	attributes := make(map[string]any)
	if declaration.form != "" {
		attributes["declaration_kind"] = declaration.form
	}
	if len(declaration.annotations) != 0 {
		attributes["annotations"] = append([]string(nil), declaration.annotations...)
	}
	if len(declaration.modifiers) != 0 {
		attributes["modifiers"] = append([]string(nil), declaration.modifiers...)
	}
	if declaration.kind == "function" {
		attributes["suspend"] = containsString(declaration.modifiers, "suspend")
		if declaration.receiver != "" {
			attributes["extension_receiver"] = declaration.receiver
			attributes["extension_receiver_nullable"] = nullableType(declaration.receiver)
		}
		if declaration.returnType != "" {
			attributes["return_type"] = declaration.returnType
			attributes["return_nullable"] = nullableType(declaration.returnType)
		}
	}
	if declaration.kind == "constructor" {
		attributes["primary"] = declaration.primary
	}
	if declaration.kind == "field" {
		attributes["mutable"] = declaration.mutable
		if declaration.receiver != "" {
			attributes["extension_receiver"] = declaration.receiver
			attributes["extension_receiver_nullable"] = nullableType(declaration.receiver)
		}
		if declaration.typeName != "" {
			attributes["type"] = declaration.typeName
			attributes["nullable"] = nullableType(declaration.typeName)
		}
	}
	return attributes
}

func parameterAttributes(parameter parameterDecl, index int) map[string]any {
	attributes := map[string]any{
		"has_default": parameter.hasDefault,
		"index":       index,
		"nullable":    nullableType(parameter.typeName),
		"property":    parameter.property,
		"type":        parameter.typeName,
	}
	if parameter.property {
		attributes["mutable"] = parameter.mutable
	}
	if len(parameter.annotations) != 0 {
		attributes["annotations"] = append([]string(nil), parameter.annotations...)
	}
	if len(parameter.modifiers) != 0 {
		attributes["modifiers"] = append([]string(nil), parameter.modifiers...)
	}
	return attributes
}

func disambiguateCanonical(canonical string, occurrences map[string]int) string {
	occurrence := occurrences[canonical]
	occurrences[canonical] = occurrence + 1
	if occurrence == 0 {
		return canonical
	}
	return fmt.Sprintf("%s#%d", canonical, occurrence+1)
}

func parameterSignature(parameters []parameterDecl) string {
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = parameter.typeName
	}
	return strings.Join(parts, ",")
}

func qualify(owner, name string) string {
	if owner == "" || owner == "<default>" {
		return name
	}
	return owner + "." + name
}

func simpleQualifiedName(value string) string {
	if index := strings.LastIndexByte(value, '.'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func sourceModuleQualifiedName(packageName, path string) string {
	return fmt.Sprintf("%s::source:%s", packageName, path)
}

func pathDirectory(path string) string {
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		return path[:index]
	}
	return "."
}

func wholeFileSpan(path string, content []byte) sourceSpan {
	line, column := 1, 1
	for offset := 0; offset < len(content); {
		if content[offset] == '\r' {
			if offset+1 < len(content) && content[offset+1] == '\n' {
				offset += 2
			} else {
				offset++
			}
			line++
			column = 1
			continue
		}
		value, size := utf8.DecodeRune(content[offset:])
		offset += size
		if value == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return sourceSpan{EndColumn: column, EndLine: line, Path: path, StartColumn: 1, StartLine: 1}
}
