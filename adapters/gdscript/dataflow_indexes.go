package main

import "fmt"

func (f *factSet) addDataflowEdge(record map[string]any) {
	key := fmt.Sprintf("%s\x00%s\x00%s", record["source"], record["target"], record["relation"])
	if _, exists := f.dataflowKeys[key]; exists {
		return
	}
	f.dataflowKeys[key] = struct{}{}
	f.addEdge(record)
}

func (f *factSet) indexDataflowDeclaration(decl *declaration) {
	if f.dataflowLocalByFunctionName == nil {
		f.dataflowLocalByFunctionName = make(map[string]map[string][]string)
	}
	if f.dataflowMemberByOwnerName == nil {
		f.dataflowMemberByOwnerName = make(map[string]map[string][]string)
	}
	if decl.ownerFunction != "" {
		if f.dataflowLocalByFunctionName[decl.ownerFunction] == nil {
			f.dataflowLocalByFunctionName[decl.ownerFunction] = make(map[string][]string)
		}
		f.dataflowLocalByFunctionName[decl.ownerFunction][decl.name] = append(
			f.dataflowLocalByFunctionName[decl.ownerFunction][decl.name], decl.nodeID)
		return
	}
	if f.dataflowMemberByOwnerName[decl.ownerID] == nil {
		f.dataflowMemberByOwnerName[decl.ownerID] = make(map[string][]string)
	}
	f.dataflowMemberByOwnerName[decl.ownerID][decl.name] = append(
		f.dataflowMemberByOwnerName[decl.ownerID][decl.name], decl.nodeID)
}

func (f *factSet) ensureDataflowDeclarationIndexes() {
	if f.dataflowLocalByFunctionName != nil && f.dataflowMemberByOwnerName != nil {
		return
	}
	f.dataflowLocalByFunctionName = make(map[string]map[string][]string)
	f.dataflowMemberByOwnerName = make(map[string]map[string][]string)
	for _, decl := range f.declarationByID {
		if decl == nil || (decl.kind != "variable" && decl.kind != "constant") {
			continue
		}
		f.indexDataflowDeclaration(decl)
	}
}
