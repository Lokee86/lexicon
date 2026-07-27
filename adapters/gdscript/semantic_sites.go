package main

type argumentCallSite struct {
	context analysisContext
	call    callReference
}

type inferenceStatementSite struct {
	context     analysisContext
	tokens      []token
	assignment  int
	declaration bool
}

func buildInferenceStatementSites(files []*parsedFile) map[*parsedFile][]inferenceStatementSite {
	sitesByFile := make(map[*parsedFile][]inferenceStatementSite, len(files))
	for _, file := range files {
		sites := make([]inferenceStatementSite, 0, len(file.statements))
		for _, stmt := range file.statements {
			if len(stmt.tokens) == 0 {
				continue
			}
			sites = append(sites, inferenceStatementSite{
				context:     contextForStatement(file, stmt),
				tokens:      stmt.tokens,
				assignment:  topLevelAssignment(stmt.tokens),
				declaration: parseDeclaration(stmt) != nil,
			})
		}
		sitesByFile[file] = sites
	}
	return sitesByFile
}

func buildArgumentCallSites(files []*parsedFile) map[*parsedFile][]argumentCallSite {
	sitesByFile := make(map[*parsedFile][]argumentCallSite, len(files))
	for _, file := range files {
		var sites []argumentCallSite
		for _, stmt := range file.statements {
			calls := findCalls(stmt, file.path)
			if len(calls) == 0 {
				continue
			}
			context := contextForStatement(file, stmt)
			for _, call := range calls {
				sites = append(sites, argumentCallSite{context: context, call: call})
			}
		}
		sitesByFile[file] = sites
	}
	return sitesByFile
}
