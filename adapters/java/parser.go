package main

import (
	"sort"
	"strings"
	"unicode/utf8"
)

var javaModifiers = map[string]struct{}{
	"abstract": {}, "default": {}, "final": {}, "native": {}, "private": {}, "protected": {},
	"public": {}, "sealed": {}, "static": {}, "strictfp": {}, "synchronized": {}, "transient": {},
	"volatile": {},
}

type javaParser struct {
	facts                *factSet
	fileID               string
	moduleID             string
	packageName          string
	path                 string
	recordComponentTypes map[string][]string
	source               string
	state                *analysisState
	tokens               []token
}

func parseJavaSource(state *analysisState, fileID, path, source string) {
	tokens, lexErrors := lexJava(source)
	packageName, headerEnd, packageProblems := parsePackageHeader(tokens)
	moduleID := state.addModule(fileID, path, packageName)
	namespaceID := state.ensureNamespace(packageName)
	state.facts.addEdge(namespaceID, moduleID, "contains", path, nil, nil)
	parser := &javaParser{
		facts: state.facts, fileID: fileID, moduleID: moduleID, packageName: packageName,
		path: path, recordComponentTypes: make(map[string][]string), source: source, state: state, tokens: tokens,
	}
	for _, problem := range lexErrors {
		problemSpan := &span{EndColumn: problem.column + 1, EndLine: problem.line, Path: path, StartColumn: problem.column, StartLine: problem.line}
		state.facts.addUnresolved(moduleID, "defines", problem.String(), "unsupported-form", path, problemSpan, nil)
	}
	for _, problem := range packageProblems {
		state.facts.addUnresolved(moduleID, "defines", problem, "unsupported-form", path, nil, nil)
	}
	parser.parseImports()
	parser.parseTopLevel(headerEnd)
}

func parsePackageHeader(tokens []token) (string, int, []string) {
	packageName := ""
	headerEnd := 0
	var problems []string
	for index := 0; index < len(tokens); {
		if tokens[index].text == "package" {
			end := findToken(tokens, index+1, len(tokens), ";")
			if end < 0 {
				problems = append(problems, "package declaration has no terminating semicolon")
				return packageName, index + 1, problems
			}
			name, valid := qualifiedTokenName(tokens[index+1 : end])
			if !valid {
				problems = append(problems, "package declaration is not a qualified identifier")
			} else {
				packageName = name
			}
			headerEnd = end + 1
			index = end + 1
			continue
		}
		if tokens[index].text == "import" {
			end := findToken(tokens, index+1, len(tokens), ";")
			if end < 0 {
				problems = append(problems, "import declaration has no terminating semicolon")
				return packageName, index + 1, problems
			}
			headerEnd = end + 1
			index = end + 1
			continue
		}
		if _, _, ok := typeKeywordAt(tokens, skipPrefix(tokens, index, len(tokens))); ok {
			break
		}
		index++
	}
	return packageName, headerEnd, problems
}

func (parser *javaParser) parseImports() {
	for index := 0; index < len(parser.tokens); {
		if parser.tokens[index].text != "import" {
			index++
			continue
		}
		end := findToken(parser.tokens, index+1, len(parser.tokens), ";")
		if end < 0 {
			return
		}
		cursor := index + 1
		static := cursor < end && parser.tokens[cursor].text == "static"
		if static {
			cursor++
		}
		parts := parser.tokens[cursor:end]
		wildcard := len(parts) >= 2 && parts[len(parts)-2].text == "." && parts[len(parts)-1].text == "*"
		nameParts := parts
		if wildcard {
			nameParts = parts[:len(parts)-2]
		}
		target, valid := qualifiedTokenName(nameParts)
		statementSpan := parser.tokenSpan(index, end+1)
		expression := sourceExcerpt(parser.source, parser.tokens, index, end+1)
		if !valid {
			parser.facts.addUnresolved(parser.moduleID, "defines", expression, "unsupported-form", parser.path, statementSpan, nil)
			index = end + 1
			continue
		}
		identity := parser.path + "::import:" + decimal(parser.tokens[index].line) + ":" + decimal(parser.tokens[index].column) + ":" + target
		id := parser.facts.addNode("import", target, parser.path, identity, identity, parser.path, statementSpan, map[string]any{
			"expression": expression, "static": static, "target": target, "wildcard": wildcard,
		}, "")
		parser.facts.addEdge(parser.moduleID, id, "defines", parser.path, statementSpan, nil)
		parser.state.imports = append(parser.state.imports, importEvidence{
			expression: expression, id: id, owner: parser.path, span: statementSpan,
			static: static, target: target, wildcard: wildcard,
		})
		index = end + 1
	}
}

func (parser *javaParser) parseTopLevel(start int) {
	for index := start; index < len(parser.tokens); {
		if parser.tokens[index].text == ";" {
			index++
			continue
		}
		core := skipPrefix(parser.tokens, index, len(parser.tokens))
		if _, _, ok := typeKeywordAt(parser.tokens, core); ok {
			next := parser.parseType(index, core, len(parser.tokens), parser.moduleID, "", "defines")
			if next > index {
				index = next
				continue
			}
		}
		end := nextTopLevelTerminator(parser.tokens, index, len(parser.tokens))
		if end <= index {
			end = index + 1
		}
		parser.facts.addUnresolved(parser.moduleID, "defines", sourceExcerpt(parser.source, parser.tokens, index, end), "unsupported-form", parser.path, parser.tokenSpan(index, end), nil)
		index = end
	}
}

func (parser *javaParser) parseType(start, keywordIndex, limit int, ownerID, parentQualifiedName, relation string) int {
	keyword, declarationKind, ok := typeKeywordAt(parser.tokens, keywordIndex)
	if !ok {
		return start
	}
	nameIndex := keywordIndex + 1
	if keyword == "annotation" {
		nameIndex = keywordIndex + 2
	}
	if nameIndex >= limit || !identifierToken(parser.tokens[nameIndex].text) {
		parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, start, min(limit, nameIndex+1)), "unsupported-form", parser.path, parser.tokenSpan(start, min(limit, nameIndex+1)), nil)
		return min(limit, nameIndex+1)
	}
	bodyOpen := findTypeBody(parser.tokens, nameIndex+1, limit)
	if bodyOpen < 0 {
		end := nextTopLevelTerminator(parser.tokens, nameIndex+1, limit)
		if end <= nameIndex {
			end = min(limit, nameIndex+1)
		}
		parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, start, end), "unsupported-form", parser.path, parser.tokenSpan(start, end), nil)
		return end
	}
	bodyClose := matchingToken(parser.tokens, bodyOpen, limit, "{", "}")
	if bodyClose < 0 {
		parser.facts.addUnresolved(ownerID, "defines", "unclosed "+declarationKind+" "+parser.tokens[nameIndex].text, "unsupported-form", parser.path, parser.tokenSpan(keywordIndex, min(limit, bodyOpen+1)), nil)
		bodyClose = limit - 1
		if bodyClose < bodyOpen {
			return limit
		}
	}

	name := parser.tokens[nameIndex].text
	qualifiedName := name
	if parentQualifiedName != "" {
		qualifiedName = parentQualifiedName + "." + name
	} else if parser.packageName != "" {
		qualifiedName = parser.packageName + "." + name
	}
	kind := "type"
	if declarationKind == "interface" || declarationKind == "annotation" {
		kind = "interface"
	}
	modifiers := modifierList(parser.tokens, start, keywordIndex)
	attributes := map[string]any{"declaration_kind": declarationKind}
	if len(modifiers) != 0 {
		attributes["modifiers"] = modifiers
	}
	typeSpan := parser.tokenSpan(keywordIndex, bodyClose+1)
	id := parser.facts.addNode(kind, name, parser.path, qualifiedName, qualifiedName, parser.path, typeSpan, attributes, "")
	parser.state.registerDeclaration(qualifiedName, id)
	parser.facts.addEdge(ownerID, id, relation, parser.path, typeSpan, nil)
	if declarationKind == "record" {
		parser.recordComponentTypes[qualifiedName] = parser.parseRecordComponents(id, qualifiedName, nameIndex+1, bodyOpen)
	}
	memberStart := bodyOpen + 1
	if declarationKind == "enum" {
		memberStart = enumMemberStart(parser.tokens, memberStart, bodyClose)
	}
	parser.parseMembers(id, qualifiedName, name, memberStart, bodyClose)
	return bodyClose + 1
}

func (parser *javaParser) parseMembers(ownerID, ownerQualifiedName, ownerName string, start, limit int) {
	for index := start; index < limit; {
		if parser.tokens[index].text == ";" {
			index++
			continue
		}
		core := skipPrefix(parser.tokens, index, limit)
		if _, _, ok := typeKeywordAt(parser.tokens, core); ok {
			next := parser.parseType(index, core, limit, ownerID, ownerQualifiedName, "contains")
			if next > index {
				index = next
				continue
			}
		}
		delimiter, delimiterText := findMemberDelimiter(parser.tokens, index, limit)
		if delimiter < 0 {
			parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, index, limit), "unsupported-form", parser.path, parser.tokenSpan(index, limit), nil)
			return
		}
		if delimiterText == "{" {
			close := matchingToken(parser.tokens, delimiter, limit, "{", "}")
			if close < 0 {
				parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, index, delimiter+1), "unsupported-form", parser.path, parser.tokenSpan(index, delimiter+1), nil)
				return
			}
			if parser.parseCallable(ownerID, ownerQualifiedName, ownerName, index, delimiter, close+1) || initializerHeader(parser.tokens, core, delimiter) {
				index = close + 1
				continue
			}
			parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, index, close+1), "unsupported-form", parser.path, parser.tokenSpan(index, close+1), nil)
			index = close + 1
			continue
		}
		if parser.parseCallable(ownerID, ownerQualifiedName, ownerName, index, delimiter, delimiter+1) {
			index = delimiter + 1
			continue
		}
		if parser.parseFields(ownerID, ownerQualifiedName, index, delimiter) {
			index = delimiter + 1
			continue
		}
		parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, index, delimiter+1), "unsupported-form", parser.path, parser.tokenSpan(index, delimiter+1), nil)
		index = delimiter + 1
	}
}

func (parser *javaParser) parseCallable(ownerID, ownerQualifiedName, ownerName string, start, headerEnd, declarationEnd int) bool {
	core := skipPrefix(parser.tokens, start, headerEnd)
	if core >= headerEnd {
		return false
	}
	open, close := callableParameters(parser.tokens, core, headerEnd)
	compact := false
	nameIndex := -1
	if open >= 0 {
		nameIndex = open - 1
		if nameIndex < core || !identifierToken(parser.tokens[nameIndex].text) {
			return false
		}
	} else if headerEnd == core+1 && parser.tokens[core].text == ownerName {
		compact = true
		nameIndex = core
	} else {
		return false
	}
	name := parser.tokens[nameIndex].text
	constructor := name == ownerName
	if !constructor && nameIndex == core {
		return false
	}
	parameterTypes := []string{}
	parameters := []parameterDeclaration{}
	if compact {
		parameterTypes = append(parameterTypes, parser.recordComponentTypes[ownerQualifiedName]...)
	} else {
		parameters = parseParameters(parser.tokens, open+1, close)
		if parameters == nil && close > open+1 {
			return false
		}
		for _, parameter := range parameters {
			parameterTypes = append(parameterTypes, parameter.typeName)
		}
	}
	signature := strings.Join(parameterTypes, ",")
	kind := "method"
	qualifiedName := ownerQualifiedName + "." + name + "(" + signature + ")"
	declarationKind := "method"
	if constructor {
		kind = "constructor"
		qualifiedName = ownerQualifiedName + ".<init>(" + signature + ")"
		declarationKind = "constructor"
		if compact {
			declarationKind = "compact-constructor"
		}
	}
	attributes := map[string]any{"declaration_kind": declarationKind, "parameter_types": parameterTypes}
	if !constructor {
		returnType := normalizedTokens(parser.tokens[core:nameIndex])
		if returnType == "" {
			return false
		}
		attributes["return_type"] = returnType
	}
	if modifiers := modifierList(parser.tokens, start, core); len(modifiers) != 0 {
		attributes["modifiers"] = modifiers
	}
	callableSpan := parser.tokenSpan(core, declarationEnd)
	id := parser.facts.addNode(kind, name, parser.path, qualifiedName, qualifiedName, parser.path, callableSpan, attributes, "")
	parser.state.registerDeclaration(qualifiedName, id)
	parser.facts.addEdge(ownerID, id, "contains", parser.path, callableSpan, nil)
	for index, parameter := range parameters {
		parameterQualifiedName := qualifiedName + "#parameter:" + decimal(index) + ":" + parameter.name
		parameterSpan := parser.tokenSpan(parameter.start, parameter.end)
		parameterID := parser.facts.addNode("parameter", parameter.name, parser.path, parameterQualifiedName, parameterQualifiedName, parser.path, parameterSpan, map[string]any{
			"index": index, "type": parameter.typeName, "varargs": parameter.varargs,
		}, "")
		parser.state.registerDeclaration(parameterQualifiedName, parameterID)
		parser.facts.addEdge(id, parameterID, "contains", parser.path, parameterSpan, nil)
	}
	return true
}

func (parser *javaParser) parseFields(ownerID, ownerQualifiedName string, start, end int) bool {
	core := skipPrefix(parser.tokens, start, end)
	segments := splitTopLevel(parser.tokens, core, end, ",")
	if len(segments) == 0 {
		return false
	}
	firstName := variableName(parser.tokens, segments[0][0], segments[0][1])
	if firstName < 0 || firstName == core {
		return false
	}
	typeName := normalizedTokens(parser.tokens[core:firstName])
	if typeName == "" {
		return false
	}
	modifiers := modifierList(parser.tokens, start, core)
	for index, segment := range segments {
		nameIndex := variableName(parser.tokens, segment[0], segment[1])
		if nameIndex < 0 {
			return false
		}
		name := parser.tokens[nameIndex].text
		qualifiedName := ownerQualifiedName + "." + name
		attributes := map[string]any{"declaration_kind": "field", "type": typeName}
		if len(modifiers) != 0 {
			attributes["modifiers"] = modifiers
		}
		fieldSpan := parser.tokenSpan(segment[0], segment[1])
		id := parser.facts.addNode("field", name, parser.path, qualifiedName, qualifiedName, parser.path, fieldSpan, attributes, "")
		parser.state.registerDeclaration(qualifiedName, id)
		parser.facts.addEdge(ownerID, id, "contains", parser.path, fieldSpan, map[string]any{"declarator_index": index})
	}
	return true
}

func (parser *javaParser) parseRecordComponents(ownerID, ownerQualifiedName string, start, end int) []string {
	open := findToken(parser.tokens, start, end, "(")
	if open < 0 {
		return nil
	}
	close := matchingToken(parser.tokens, open, end, "(", ")")
	if close < 0 {
		parser.facts.addUnresolved(ownerID, "defines", "record component list", "unsupported-form", parser.path, parser.tokenSpan(open, min(end, open+1)), nil)
		return nil
	}
	components := parseParameters(parser.tokens, open+1, close)
	if components == nil && close > open+1 {
		parser.facts.addUnresolved(ownerID, "defines", sourceExcerpt(parser.source, parser.tokens, open, close+1), "unsupported-form", parser.path, parser.tokenSpan(open, close+1), nil)
		return nil
	}
	types := make([]string, 0, len(components))
	for index, component := range components {
		types = append(types, component.typeName)
		qualifiedName := ownerQualifiedName + "." + component.name
		componentSpan := parser.tokenSpan(component.start, component.end)
		id := parser.facts.addNode("field", component.name, parser.path, qualifiedName, qualifiedName, parser.path, componentSpan, map[string]any{
			"declaration_kind": "record-component", "index": index, "type": component.typeName,
		}, "")
		parser.state.registerDeclaration(qualifiedName, id)
		parser.facts.addEdge(ownerID, id, "contains", parser.path, componentSpan, nil)
	}
	return types
}

func (parser *javaParser) tokenSpan(start, end int) *span {
	if start < 0 || start >= len(parser.tokens) || end <= start {
		return nil
	}
	if end > len(parser.tokens) {
		end = len(parser.tokens)
	}
	first, last := parser.tokens[start], parser.tokens[end-1]
	return &span{EndColumn: last.endColumn, EndLine: last.endLine, Path: parser.path, StartColumn: first.column, StartLine: first.line}
}

type parameterDeclaration struct {
	end      int
	name     string
	start    int
	typeName string
	varargs  bool
}

func parseParameters(tokens []token, start, end int) []parameterDeclaration {
	if start >= end {
		return []parameterDeclaration{}
	}
	segments := splitTopLevel(tokens, start, end, ",")
	result := make([]parameterDeclaration, 0, len(segments))
	for _, segment := range segments {
		core := skipPrefix(tokens, segment[0], segment[1])
		nameIndex := variableName(tokens, core, segment[1])
		if nameIndex < 0 || nameIndex == core {
			return nil
		}
		typeTokens := make([]token, 0, nameIndex-core)
		for index := core; index < segment[1]; index++ {
			if index != nameIndex {
				typeTokens = append(typeTokens, tokens[index])
			}
		}
		typeName := normalizedTokens(typeTokens)
		if typeName == "" {
			return nil
		}
		result = append(result, parameterDeclaration{
			end: segment[1], name: tokens[nameIndex].text, start: segment[0], typeName: typeName,
			varargs: containsToken(tokens, core, nameIndex, "..."),
		})
	}
	return result
}

func callableParameters(tokens []token, start, end int) (int, int) {
	bestOpen, bestClose := -1, -1
	for index := start; index < end; index++ {
		if tokens[index].text != "(" || index == start || !identifierToken(tokens[index-1].text) {
			continue
		}
		close := matchingToken(tokens, index, end, "(", ")")
		if close >= 0 {
			bestOpen, bestClose = index, close
			index = close
		}
	}
	return bestOpen, bestClose
}

func findMemberDelimiter(tokens []token, start, end int) (int, string) {
	paren, bracket := 0, 0
	hasEquals := false
	for index := start; index < end; index++ {
		switch tokens[index].text {
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "=":
			if paren == 0 && bracket == 0 {
				hasEquals = true
			}
		case "{":
			if paren == 0 && bracket == 0 {
				if hasEquals {
					close := matchingToken(tokens, index, end, "{", "}")
					if close < 0 {
						return index, "{"
					}
					index = close
					continue
				}
				return index, "{"
			}
		case ";":
			if paren == 0 && bracket == 0 {
				return index, ";"
			}
		}
	}
	return -1, ""
}

func findTypeBody(tokens []token, start, end int) int {
	paren, bracket := 0, 0
	for index := start; index < end; index++ {
		switch tokens[index].text {
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "{":
			if paren == 0 && bracket == 0 {
				return index
			}
		case ";":
			if paren == 0 && bracket == 0 {
				return -1
			}
		}
	}
	return -1
}

func enumMemberStart(tokens []token, start, end int) int {
	paren, bracket, braces := 0, 0, 0
	for index := start; index < end; index++ {
		switch tokens[index].text {
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "{":
			braces++
		case "}":
			braces--
		case ";":
			if paren == 0 && bracket == 0 && braces == 0 {
				return index + 1
			}
		}
	}
	return end
}

func nextTopLevelTerminator(tokens []token, start, end int) int {
	paren, bracket, braces := 0, 0, 0
	for index := start; index < end; index++ {
		switch tokens[index].text {
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "{":
			braces++
		case "}":
			if braces == 0 {
				return index + 1
			}
			braces--
		case ";":
			if paren == 0 && bracket == 0 && braces == 0 {
				return index + 1
			}
		}
	}
	return end
}

func matchingToken(tokens []token, open, end int, left, right string) int {
	depth := 0
	for index := open; index < end; index++ {
		if tokens[index].text == left {
			depth++
		} else if tokens[index].text == right {
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func skipPrefix(tokens []token, start, end int) int {
	index := start
	for index < end {
		if tokens[index].text == "@" && index+1 < end && tokens[index+1].text != "interface" {
			index += 2
			for index+1 < end && tokens[index].text == "." && identifierToken(tokens[index+1].text) {
				index += 2
			}
			if index < end && tokens[index].text == "(" {
				close := matchingToken(tokens, index, end, "(", ")")
				if close < 0 {
					return index
				}
				index = close + 1
			}
			continue
		}
		if _, modifier := javaModifiers[tokens[index].text]; modifier {
			index++
			continue
		}
		if index+2 < end && tokens[index].text == "non" && tokens[index+1].text == "-" && tokens[index+2].text == "sealed" {
			index += 3
			continue
		}
		break
	}
	return index
}

func modifierList(tokens []token, start, end int) []string {
	values := make(map[string]struct{})
	for index := start; index < end; index++ {
		if _, modifier := javaModifiers[tokens[index].text]; modifier {
			values[tokens[index].text] = struct{}{}
		}
		if index+2 < end && tokens[index].text == "non" && tokens[index+1].text == "-" && tokens[index+2].text == "sealed" {
			values["non-sealed"] = struct{}{}
		}
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func typeKeywordAt(tokens []token, index int) (string, string, bool) {
	if index < 0 || index >= len(tokens) {
		return "", "", false
	}
	switch tokens[index].text {
	case "class":
		return "class", "class", true
	case "interface":
		return "interface", "interface", true
	case "enum":
		return "enum", "enum", true
	case "record":
		return "record", "record", true
	case "@":
		if index+1 < len(tokens) && tokens[index+1].text == "interface" {
			return "annotation", "annotation", true
		}
	}
	return "", "", false
}

func variableName(tokens []token, start, end int) int {
	limit := end
	paren, bracket, braces, angle := 0, 0, 0, 0
	for index := start; index < end; index++ {
		switch tokens[index].text {
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "{":
			braces++
		case "}":
			braces--
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		case "=":
			if paren == 0 && bracket == 0 && braces == 0 && angle == 0 {
				limit = index
				index = end
			}
		}
	}
	if limit > start && tokens[limit-1].text == "this" {
		return limit - 1
	}
	for index := limit - 1; index >= start; index-- {
		if identifierToken(tokens[index].text) {
			return index
		}
	}
	return -1
}

func splitTopLevel(tokens []token, start, end int, separator string) [][2]int {
	if start >= end {
		return nil
	}
	var result [][2]int
	segmentStart := start
	paren, bracket, braces, angle := 0, 0, 0, 0
	for index := start; index < end; index++ {
		switch tokens[index].text {
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "{":
			braces++
		case "}":
			braces--
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		default:
			if tokens[index].text == separator && paren == 0 && bracket == 0 && braces == 0 && angle == 0 {
				result = append(result, [2]int{segmentStart, index})
				segmentStart = index + 1
			}
		}
	}
	if segmentStart < end {
		result = append(result, [2]int{segmentStart, end})
	}
	return result
}

func qualifiedTokenName(tokens []token) (string, bool) {
	if len(tokens) == 0 || len(tokens)%2 == 0 {
		return "", false
	}
	var builder strings.Builder
	for index, item := range tokens {
		if index%2 == 0 {
			if !identifierToken(item.text) {
				return "", false
			}
			builder.WriteString(item.text)
		} else if item.text != "." {
			return "", false
		} else {
			builder.WriteByte('.')
		}
	}
	return builder.String(), true
}

func normalizedTokens(tokens []token) string {
	var builder strings.Builder
	for _, item := range tokens {
		builder.WriteString(item.text)
	}
	return builder.String()
}

func identifierToken(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	if !isIdentifierStart(r) {
		return false
	}
	for _, current := range text {
		if !isIdentifierPart(current) {
			return false
		}
	}
	return true
}

func initializerHeader(tokens []token, start, end int) bool {
	return start == end || (start+1 == end && tokens[start].text == "static")
}

func containsToken(tokens []token, start, end int, text string) bool {
	for index := start; index < end; index++ {
		if tokens[index].text == text {
			return true
		}
	}
	return false
}

func findToken(tokens []token, start, end int, text string) int {
	for index := start; index < end; index++ {
		if tokens[index].text == text {
			return index
		}
	}
	return -1
}

func decimal(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [24]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
