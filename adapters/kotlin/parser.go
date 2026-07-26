package main

import (
	"fmt"
	"sort"
	"strings"
)

type importDecl struct {
	alias    string
	path     string
	span     sourceSpan
	wildcard bool
}

type parameterDecl struct {
	annotations []string
	hasDefault  bool
	modifiers   []string
	mutable     bool
	name        string
	property    bool
	span        sourceSpan
	typeName    string
}

type declaration struct {
	annotations []string
	children    []*declaration
	form        string
	kind        string
	modifiers   []string
	mutable     bool
	name        string
	parameters  []parameterDecl
	primary     bool
	receiver    string
	returnType  string
	span        sourceSpan
	typeName    string
}

type parsedKotlinFile struct {
	content      []byte
	declarations []*declaration
	diagnostics  []syntaxDiagnostic
	imports      []importDecl
	packageName  string
	packageSpan  *sourceSpan
	path         string
}

type parser struct {
	content     []byte
	diagnostics []syntaxDiagnostic
	index       int
	path        string
	tokens      []token
}

var declarationModifiers = map[string]struct{}{
	"abstract": {}, "actual": {}, "annotation": {}, "companion": {}, "const": {},
	"crossinline": {}, "data": {}, "enum": {}, "expect": {}, "external": {},
	"final": {}, "infix": {}, "inline": {}, "inner": {}, "internal": {},
	"lateinit": {}, "noinline": {}, "open": {}, "operator": {}, "out": {},
	"override": {}, "private": {}, "protected": {}, "public": {}, "reified": {},
	"sealed": {}, "suspend": {}, "tailrec": {}, "value": {}, "vararg": {},
}

var parameterModifiers = map[string]struct{}{
	"crossinline": {}, "noinline": {}, "vararg": {},
}

type declarationPrefix struct {
	annotations []string
	modifiers   []string
	start       int
}

func parseKotlinFile(path string, content []byte) *parsedKotlinFile {
	tokens, lexerDiagnostics := lex(content)
	state := &parser{content: content, diagnostics: append([]syntaxDiagnostic(nil), lexerDiagnostics...), path: path, tokens: tokens}
	result := &parsedKotlinFile{content: content, path: path}

	state.skipSeparators()
	for state.at("@") {
		state.parseAnnotation()
		state.skipSeparators()
	}
	if state.at("package") {
		start := state.index
		state.index++
		name, end, ok := state.parseQualifiedDirective(false)
		if ok {
			result.packageName = name
			span := state.span(start, end)
			result.packageSpan = &span
		} else {
			state.addDiagnostic(start, "malformed package directive")
		}
	}
	state.skipSeparators()
	for state.at("import") {
		start := state.index
		state.index++
		name, end, ok := state.parseQualifiedDirective(true)
		alias := ""
		if state.at("as") {
			state.index++
			if state.current().kind == tokenIdentifier {
				alias = identifierText(state.current())
				end = state.index
				state.index++
			} else {
				ok = false
			}
		}
		if ok {
			result.imports = append(result.imports, importDecl{
				alias: alias, path: name, span: state.span(start, end), wildcard: strings.HasSuffix(name, ".*"),
			})
		} else {
			state.addDiagnostic(start, "malformed import directive")
		}
		state.skipToLineEnd()
		state.skipSeparators()
	}

	result.declarations = state.parseScope(false)
	result.diagnostics = state.diagnostics
	return result
}

func (state *parser) parseScope(stopAtBrace bool) []*declaration {
	var declarations []*declaration
	for state.current().kind != tokenEOF {
		state.skipSeparators()
		if stopAtBrace && state.at("}") {
			state.index++
			return declarations
		}
		if state.current().kind == tokenEOF {
			break
		}
		if state.at(")") || state.at("]") || state.at("}") {
			state.addDiagnostic(state.index, "unmatched closing delimiter")
			state.index++
			continue
		}

		prefix := state.parsePrefix()
		var declaration *declaration
		switch state.current().text {
		case "class", "interface", "object":
			declaration = state.parseType(prefix)
		case "fun":
			declaration = state.parseFunction(prefix)
		case "val", "var":
			declaration = state.parseProperty(prefix)
		case "constructor":
			declaration = state.parseConstructor(prefix, false)
		default:
			if len(prefix.annotations) != 0 || len(prefix.modifiers) != 0 {
				state.addDiagnostic(prefix.start, "unsupported or malformed declaration")
			}
			state.skipStatement()
		}
		if declaration != nil {
			declarations = append(declarations, declaration)
		}
	}
	if stopAtBrace {
		state.addDiagnostic(maxInt(0, state.index-1), "unclosed declaration body")
	}
	return declarations
}

func (state *parser) parsePrefix() declarationPrefix {
	prefix := declarationPrefix{start: state.index}
	for {
		if state.at("@") {
			if annotation := state.parseAnnotation(); annotation != "" {
				prefix.annotations = append(prefix.annotations, annotation)
			}
			state.skipNewlines()
			continue
		}
		if _, ok := declarationModifiers[state.current().text]; ok {
			prefix.modifiers = append(prefix.modifiers, state.current().text)
			state.index++
			state.skipNewlines()
			continue
		}
		break
	}
	prefix.annotations = uniqueSorted(prefix.annotations)
	prefix.modifiers = uniqueSorted(prefix.modifiers)
	return prefix
}

func (state *parser) parseAnnotation() string {
	if !state.at("@") {
		return ""
	}
	state.index++
	var parts []string
	for state.current().kind == tokenIdentifier || state.at(".") || state.at(":") {
		parts = append(parts, state.current().text)
		state.index++
	}
	if state.at("[") {
		state.skipBalanced("[", "]")
	} else if state.at("(") {
		state.skipBalanced("(", ")")
	}
	return compactTokens(parts)
}

func (state *parser) parseType(prefix declarationPrefix) *declaration {
	keywordIndex := state.index
	keyword := state.current().text
	state.index++
	name := ""
	if keyword == "object" && containsString(prefix.modifiers, "companion") {
		name = "Companion"
		if state.current().kind == tokenIdentifier && !state.at(":") {
			name = identifierText(state.current())
			state.index++
		}
	} else if state.current().kind == tokenIdentifier {
		name = identifierText(state.current())
		state.index++
	} else {
		state.addDiagnostic(keywordIndex, "type declaration is missing a name")
		state.skipStatement()
		return nil
	}

	if state.at("<") {
		state.skipBalanced("<", ">")
	}
	for state.at("@") || isVisibility(state.current().text) {
		if state.at("@") {
			state.parseAnnotation()
		} else {
			state.index++
		}
		state.skipNewlines()
	}
	explicitConstructor := false
	if state.at("constructor") {
		explicitConstructor = true
		state.index++
		state.skipNewlines()
	}
	var parameters []parameterDecl
	if state.at("(") {
		var valid bool
		parameters, valid = state.parseParameterList()
		if !valid {
			state.skipStatement()
			return nil
		}
		explicitConstructor = true
	}

	form := keyword
	kind := "type"
	if keyword == "interface" {
		kind = "interface"
	}
	switch {
	case keyword == "object" && containsString(prefix.modifiers, "companion"):
		form = "companion_object"
	case keyword == "object" && containsString(prefix.modifiers, "data"):
		form = "data_object"
	case keyword == "object":
		form = "object"
	case containsString(prefix.modifiers, "enum"):
		form = "enum_class"
	case containsString(prefix.modifiers, "data"):
		form = "data_class"
	case containsString(prefix.modifiers, "value"):
		form = "value_class"
	case containsString(prefix.modifiers, "annotation"):
		form = "annotation_class"
	case containsString(prefix.modifiers, "sealed") && keyword == "interface":
		form = "sealed_interface"
	case containsString(prefix.modifiers, "sealed"):
		form = "sealed_class"
	}

	typeDeclaration := &declaration{
		annotations: prefix.annotations,
		form:        form,
		kind:        kind,
		modifiers:   prefix.modifiers,
		name:        name,
		parameters:  parameters,
		span:        state.span(prefix.start, maxInt(prefix.start, state.index-1)),
	}

	// Consume the supertype/header clause until the body or the declaration boundary.
	for state.current().kind != tokenEOF && !state.at("{") && !state.at(";") {
		if state.current().kind == tokenNewline {
			lookahead := state.nextNonNewline(state.index + 1)
			if lookahead >= len(state.tokens) || state.tokens[lookahead].text != "{" {
				break
			}
		}
		if state.at("(") {
			state.skipBalanced("(", ")")
			continue
		}
		if state.at("<") {
			state.skipBalanced("<", ">")
			continue
		}
		state.index++
	}

	if state.at("{") {
		state.index++
		typeDeclaration.children = state.parseScope(true)
		typeDeclaration.span = state.span(prefix.start, maxInt(prefix.start, state.index-1))
	}
	if kind == "type" && keyword == "class" {
		constructor := &declaration{
			form: "primary_constructor", kind: "constructor", name: name,
			parameters: parameters, primary: true, span: typeDeclaration.span,
		}
		if !explicitConstructor {
			constructor.modifiers = []string{"implicit"}
		}
		typeDeclaration.children = append([]*declaration{constructor}, typeDeclaration.children...)
	}
	return typeDeclaration
}

func (state *parser) parseFunction(prefix declarationPrefix) *declaration {
	start := prefix.start
	state.index++
	state.skipNewlines()
	if state.at("<") {
		state.skipBalanced("<", ">")
		state.skipNewlines()
	}
	nameStart := state.index
	open := state.findHeaderToken("(")
	if open < 0 {
		state.addDiagnostic(start, "function declaration has no parameter list")
		state.skipStatement()
		return nil
	}
	nameIndex := lastIdentifier(state.tokens, nameStart, open)
	if nameIndex < 0 {
		state.addDiagnostic(start, "function declaration is missing a name")
		state.index = open
		state.skipBalanced("(", ")")
		state.skipStatement()
		return nil
	}
	name := identifierText(state.tokens[nameIndex])
	receiver := ""
	if dot := lastToken(state.tokens, nameStart, nameIndex, "."); dot >= 0 {
		receiver = state.tokenText(nameStart, dot)
	}
	state.index = open
	parameters, valid := state.parseParameterList()
	if !valid {
		state.skipStatement()
		return nil
	}
	returnType := ""
	state.skipNewlines()
	if state.at(":") {
		state.index++
		typeStart := state.index
		end := state.findTypeEnd()
		returnType = state.tokenText(typeStart, end)
		state.index = end
	}
	for state.at("where") {
		state.skipStatementHeader()
	}
	if state.at("{") {
		state.skipBalanced("{", "}")
	} else if state.at("=") {
		state.skipExpression()
	}
	end := maxInt(start, state.index-1)
	return &declaration{
		annotations: prefix.annotations, form: "function", kind: "function",
		modifiers: prefix.modifiers, name: name, parameters: parameters,
		receiver: receiver, returnType: returnType, span: state.span(start, end),
	}
}

func (state *parser) parseProperty(prefix declarationPrefix) *declaration {
	start := prefix.start
	mutable := state.at("var")
	state.index++
	state.skipNewlines()
	if state.at("(") {
		state.addDiagnostic(start, "destructuring property declarations are not modeled")
		state.skipStatement()
		return nil
	}
	headerEnd := state.findPropertyHeaderEnd()
	colon := firstTopLevelToken(state.tokens, state.index, headerEnd, ":")
	equals := firstTopLevelToken(state.tokens, state.index, headerEnd, "=")
	by := firstTopLevelToken(state.tokens, state.index, headerEnd, "by")
	nameEnd := headerEnd
	for _, candidate := range []int{colon, equals, by} {
		if candidate >= 0 && candidate < nameEnd {
			nameEnd = candidate
		}
	}
	nameIndex := lastIdentifier(state.tokens, state.index, nameEnd)
	if nameIndex < 0 {
		state.addDiagnostic(start, "property declaration is missing a name")
		state.skipStatement()
		return nil
	}
	name := identifierText(state.tokens[nameIndex])
	receiver := ""
	if dot := lastToken(state.tokens, state.index, nameIndex, "."); dot >= 0 {
		receiver = state.tokenText(state.index, dot)
	}
	typeName := ""
	if colon >= 0 {
		typeEnd := headerEnd
		for _, candidate := range []int{equals, by} {
			if candidate > colon && candidate < typeEnd {
				typeEnd = candidate
			}
		}
		typeName = state.tokenText(colon+1, typeEnd)
	}
	state.index = headerEnd
	if state.at("=") || state.at("by") {
		state.skipExpression()
	}
	return &declaration{
		annotations: prefix.annotations, form: "property", kind: "field", modifiers: prefix.modifiers,
		mutable: mutable, name: name, receiver: receiver, span: state.span(start, maxInt(start, state.index-1)), typeName: typeName,
	}
}

func (state *parser) parseConstructor(prefix declarationPrefix, primary bool) *declaration {
	start := prefix.start
	state.index++
	state.skipNewlines()
	if !state.at("(") {
		state.addDiagnostic(start, "constructor declaration has no parameter list")
		state.skipStatement()
		return nil
	}
	parameters, valid := state.parseParameterList()
	if !valid {
		state.skipStatement()
		return nil
	}
	for state.current().kind != tokenEOF && state.current().kind != tokenNewline && !state.at("{") && !state.at(";") {
		if state.at("(") {
			state.skipBalanced("(", ")")
		} else {
			state.index++
		}
	}
	if state.at("{") {
		state.skipBalanced("{", "}")
	}
	return &declaration{
		annotations: prefix.annotations, form: "secondary_constructor", kind: "constructor",
		modifiers: prefix.modifiers, name: "constructor", parameters: parameters,
		primary: primary, span: state.span(start, maxInt(start, state.index-1)),
	}
}

func (state *parser) parseParameterList() ([]parameterDecl, bool) {
	open := state.index
	close := state.matchingDelimiter(open, "(", ")")
	if close < 0 {
		state.addDiagnostic(open, "unclosed parameter list")
		state.index = state.findRecoveryBoundary(open + 1)
		return nil, false
	}
	segments := splitTopLevel(state.tokens, open+1, close, ",")
	parameters := make([]parameterDecl, 0, len(segments))
	valid := true
	for _, segment := range segments {
		if parameter, ok := state.parseParameterSegment(segment[0], segment[1]); ok {
			parameters = append(parameters, parameter)
		} else if skipTokenKind(state.tokens, segment[0], segment[1], tokenNewline) < segment[1] {
			valid = false
		}
	}
	state.index = close + 1
	return parameters, valid
}

func (state *parser) parseParameterSegment(start, end int) (parameterDecl, bool) {
	start = skipTokenKind(state.tokens, start, end, tokenNewline)
	end = trimTokenKind(state.tokens, start, end, tokenNewline)
	if start >= end {
		return parameterDecl{}, false
	}
	originalStart := start
	var annotations, modifiers []string
	property := false
	mutable := false
	for start < end {
		if state.tokens[start].text == "@" {
			annotationEnd := annotationEnd(state.tokens, start, end)
			annotations = append(annotations, compactTokenRange(state.tokens, start+1, annotationEnd))
			start = annotationEnd
			continue
		}
		if _, ok := parameterModifiers[state.tokens[start].text]; ok {
			modifiers = append(modifiers, state.tokens[start].text)
			start++
			continue
		}
		if state.tokens[start].text == "val" || state.tokens[start].text == "var" {
			property = true
			mutable = state.tokens[start].text == "var"
			start++
			continue
		}
		break
	}
	colon := firstTopLevelToken(state.tokens, start, end, ":")
	if colon < 0 {
		state.addDiagnostic(originalStart, "parameter declaration has no type")
		return parameterDecl{}, false
	}
	nameIndex := lastIdentifier(state.tokens, start, colon)
	if nameIndex < 0 {
		state.addDiagnostic(originalStart, "parameter declaration is missing a name")
		return parameterDecl{}, false
	}
	equals := firstTopLevelToken(state.tokens, colon+1, end, "=")
	typeEnd := end
	if equals >= 0 {
		typeEnd = equals
	}
	return parameterDecl{
		annotations: uniqueSorted(annotations), hasDefault: equals >= 0, modifiers: uniqueSorted(modifiers),
		mutable: mutable, name: identifierText(state.tokens[nameIndex]), property: property,
		span: state.span(originalStart, end-1), typeName: state.tokenText(colon+1, typeEnd),
	}, true
}

func (state *parser) parseQualifiedDirective(allowWildcard bool) (string, int, bool) {
	var parts []string
	last := state.index
	expectName := true
	for state.current().kind != tokenEOF && state.current().kind != tokenNewline && !state.at(";") && !state.at("as") {
		current := state.current()
		if expectName {
			if current.kind == tokenIdentifier || (allowWildcard && current.text == "*") {
				parts = append(parts, identifierText(current))
				last = state.index
				state.index++
				expectName = false
				continue
			}
			return "", last, false
		}
		if current.text != "." {
			break
		}
		parts = append(parts, ".")
		last = state.index
		state.index++
		expectName = true
	}
	if len(parts) == 0 || expectName {
		return "", last, false
	}
	return strings.Join(parts, ""), last, true
}

func (state *parser) skipBalanced(open, close string) bool {
	if !state.at(open) {
		return false
	}
	matching := state.matchingDelimiter(state.index, open, close)
	if matching < 0 {
		state.addDiagnostic(state.index, "unclosed "+open+" delimiter")
		state.index = state.findRecoveryBoundary(state.index + 1)
		return false
	}
	state.index = matching + 1
	return true
}

func (state *parser) matchingDelimiter(openIndex int, open, close string) int {
	depth := 0
	for index := openIndex; index < len(state.tokens); index++ {
		switch state.tokens[index].text {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func (state *parser) findHeaderToken(wanted string) int {
	angle := 0
	for index := state.index; index < len(state.tokens); index++ {
		current := state.tokens[index]
		if current.kind == tokenEOF || current.text == "{" || current.text == "=" || current.text == ";" {
			return -1
		}
		if current.kind == tokenNewline && angle == 0 {
			return -1
		}
		if current.text == "<" {
			angle++
		} else if current.text == ">" && angle > 0 {
			angle--
		} else if current.text == wanted && angle == 0 {
			return index
		}
	}
	return -1
}

func (state *parser) findTypeEnd() int {
	depth := delimiterDepth{}
	for index := state.index; index < len(state.tokens); index++ {
		current := state.tokens[index]
		if depth.zero() && (current.kind == tokenEOF || current.kind == tokenNewline || current.text == "{" || current.text == "=" || current.text == ";" || current.text == "where") {
			return index
		}
		depth.update(current.text)
	}
	return len(state.tokens) - 1
}

func (state *parser) findPropertyHeaderEnd() int {
	depth := delimiterDepth{}
	for index := state.index; index < len(state.tokens); index++ {
		current := state.tokens[index]
		if depth.zero() && (current.kind == tokenEOF || current.kind == tokenNewline || current.text == ";" || current.text == "{") {
			return index
		}
		depth.update(current.text)
	}
	return len(state.tokens) - 1
}

func (state *parser) skipExpression() {
	depth := delimiterDepth{}
	for state.current().kind != tokenEOF {
		current := state.current()
		if depth.zero() && (current.kind == tokenNewline || current.text == ";" || current.text == "}") {
			return
		}
		depth.update(current.text)
		state.index++
	}
}

func (state *parser) skipStatement() {
	depth := delimiterDepth{}
	for state.current().kind != tokenEOF {
		current := state.current()
		if depth.zero() && (current.kind == tokenNewline || current.text == ";" || current.text == "}") {
			if current.kind == tokenNewline || current.text == ";" {
				state.index++
			}
			return
		}
		depth.update(current.text)
		state.index++
	}
}

func (state *parser) skipStatementHeader() { state.skipStatement() }

func (state *parser) skipToLineEnd() {
	for state.current().kind != tokenEOF && state.current().kind != tokenNewline && !state.at(";") {
		state.index++
	}
}

func (state *parser) findRecoveryBoundary(start int) int {
	for index := start; index < len(state.tokens); index++ {
		if state.tokens[index].kind == tokenNewline || state.tokens[index].text == ";" || state.tokens[index].text == "}" || state.tokens[index].kind == tokenEOF {
			return index
		}
	}
	return len(state.tokens) - 1
}

func (state *parser) skipSeparators() {
	for state.current().kind == tokenNewline || state.at(";") {
		state.index++
	}
}

func (state *parser) skipNewlines() {
	for state.current().kind == tokenNewline {
		state.index++
	}
}

func (state *parser) current() token {
	if state.index >= len(state.tokens) {
		return state.tokens[len(state.tokens)-1]
	}
	return state.tokens[state.index]
}

func (state *parser) at(text string) bool { return state.current().text == text }

func (state *parser) nextNonNewline(index int) int {
	for index < len(state.tokens) && state.tokens[index].kind == tokenNewline {
		index++
	}
	return index
}

func (state *parser) span(start, end int) sourceSpan {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start >= len(state.tokens) {
		start = len(state.tokens) - 1
	}
	if end >= len(state.tokens) {
		end = len(state.tokens) - 1
	}
	first, last := state.tokens[start], state.tokens[end]
	return sourceSpan{
		EndColumn: last.endColumn, EndLine: last.endLine, Path: state.path,
		StartColumn: first.startColumn, StartLine: first.startLine,
	}
}

func (state *parser) tokenText(start, end int) string {
	return compactTokenRange(state.tokens, start, end)
}

func (state *parser) addDiagnostic(index int, message string) {
	if len(state.tokens) == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(state.tokens) {
		index = len(state.tokens) - 1
	}
	state.diagnostics = append(state.diagnostics, syntaxDiagnostic{message: message, token: state.tokens[index]})
}

type delimiterDepth struct{ braces, brackets, parentheses, angles int }

func (depth *delimiterDepth) update(text string) {
	switch text {
	case "{":
		depth.braces++
	case "}":
		if depth.braces > 0 {
			depth.braces--
		}
	case "[":
		depth.brackets++
	case "]":
		if depth.brackets > 0 {
			depth.brackets--
		}
	case "(":
		depth.parentheses++
	case ")":
		if depth.parentheses > 0 {
			depth.parentheses--
		}
	case "<":
		depth.angles++
	case ">":
		if depth.angles > 0 {
			depth.angles--
		}
	}
}

func (depth delimiterDepth) zero() bool {
	return depth.braces == 0 && depth.brackets == 0 && depth.parentheses == 0 && depth.angles == 0
}

func splitTopLevel(tokens []token, start, end int, separator string) [][2]int {
	var result [][2]int
	segmentStart := start
	depth := delimiterDepth{}
	for index := start; index < end; index++ {
		if depth.zero() && tokens[index].text == separator {
			result = append(result, [2]int{segmentStart, index})
			segmentStart = index + 1
			continue
		}
		depth.update(tokens[index].text)
	}
	result = append(result, [2]int{segmentStart, end})
	return result
}

func firstTopLevelToken(tokens []token, start, end int, wanted string) int {
	depth := delimiterDepth{}
	for index := start; index < end; index++ {
		if depth.zero() && tokens[index].text == wanted {
			return index
		}
		depth.update(tokens[index].text)
	}
	return -1
}

func lastIdentifier(tokens []token, start, end int) int {
	for index := end - 1; index >= start; index-- {
		if tokens[index].kind == tokenIdentifier {
			return index
		}
	}
	return -1
}

func lastToken(tokens []token, start, end int, wanted string) int {
	for index := end - 1; index >= start; index-- {
		if tokens[index].text == wanted {
			return index
		}
	}
	return -1
}

func compactTokenRange(tokens []token, start, end int) string {
	parts := make([]string, 0, end-start)
	for index := start; index < end && index < len(tokens); index++ {
		if tokens[index].kind != tokenNewline && tokens[index].kind != tokenEOF {
			parts = append(parts, tokens[index].text)
		}
	}
	return compactTokens(parts)
}

func compactTokens(parts []string) string {
	var output strings.Builder
	previous := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if output.Len() != 0 && needsTokenSpace(previous, part) {
			output.WriteByte(' ')
		}
		output.WriteString(part)
		previous = part
	}
	return strings.TrimSpace(output.String())
}

func needsTokenSpace(previous, current string) bool {
	if previous == "" {
		return false
	}
	noSpaceBefore := ").,?]>:;"
	noSpaceAfter := "([<.@:"
	if strings.Contains(noSpaceBefore, current) || strings.Contains(noSpaceAfter, previous) {
		return false
	}
	if current == "<" || current == "(" || current == "[" || current == "." || current == "@" {
		return false
	}
	if previous == "." || previous == "?" || previous == "!" || previous == "~" {
		return false
	}
	return true
}

func annotationEnd(tokens []token, start, end int) int {
	index := start + 1
	for index < end && (tokens[index].kind == tokenIdentifier || tokens[index].text == "." || tokens[index].text == ":") {
		index++
	}
	if index < end && tokens[index].text == "(" {
		depth := 0
		for index < end {
			if tokens[index].text == "(" {
				depth++
			}
			if tokens[index].text == ")" {
				depth--
				if depth == 0 {
					return index + 1
				}
			}
			index++
		}
	}
	return index
}

func skipTokenKind(tokens []token, start, end int, kind tokenKind) int {
	for start < end && tokens[start].kind == kind {
		start++
	}
	return start
}

func trimTokenKind(tokens []token, start, end int, kind tokenKind) int {
	for end > start && tokens[end-1].kind == kind {
		end--
	}
	return end
}

func identifierText(value token) string {
	if strings.HasPrefix(value.text, "`") && strings.HasSuffix(value.text, "`") && len(value.text) >= 2 {
		return value.text[1 : len(value.text)-1]
	}
	return value.text
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isVisibility(value string) bool {
	return value == "public" || value == "private" || value == "protected" || value == "internal"
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func nullableType(value string) bool {
	return strings.HasSuffix(strings.TrimSpace(value), "?")
}

func diagnosticExpression(content []byte, diagnostic syntaxDiagnostic) string {
	start := diagnostic.token.startOffset
	if start < 0 || start > len(content) {
		start = 0
	}
	end := diagnostic.token.endOffset
	if end < start || end > len(content) {
		end = start
	}
	for start > 0 && content[start-1] != '\n' && content[start-1] != '\r' {
		start--
	}
	for end < len(content) && content[end] != '\n' && content[end] != '\r' {
		end++
	}
	expression := strings.TrimSpace(string(content[start:end]))
	if len(expression) > 160 {
		expression = expression[:157] + "..."
	}
	if expression == "" {
		expression = diagnostic.message
	}
	return expression
}

func diagnosticAttributes(diagnostic syntaxDiagnostic) map[string]any {
	return map[string]any{"diagnostic": diagnostic.message, "parser": "lexicon-kotlin-structural"}
}

func validateParsedFile(file *parsedKotlinFile) error {
	if file.path == "" {
		return fmt.Errorf("parsed Kotlin file has no path")
	}
	return nil
}
