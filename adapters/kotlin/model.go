package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	adapterVersion = "0.3.0"
	language       = "kotlin"
)

type sourceSpan struct {
	EndColumn   int    `json:"end_column"`
	EndLine     int    `json:"end_line"`
	Path        string `json:"path"`
	StartColumn int    `json:"start_column"`
	StartLine   int    `json:"start_line"`
}

type factSet struct {
	nodes         map[string]map[string]any
	edges         map[string]map[string]any
	unresolved    map[string]map[string]any
	ownerByNodeID map[string]string
}

func newFactSet() *factSet {
	return &factSet{
		nodes:         make(map[string]map[string]any),
		edges:         make(map[string]map[string]any),
		unresolved:    make(map[string]map[string]any),
		ownerByNodeID: make(map[string]string),
	}
}

func stableID(kind, canonical string) string {
	return digest("lexicon:v1\x00" + language + "\x00" + kind + "\x00" + canonical)
}

func contentID(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func (facts *factSet) addNode(kind, canonical, name, path, qualifiedName, owner string, span *sourceSpan, attributes map[string]any) string {
	id := stableID(kind, canonical)
	record := map[string]any{
		"id":             id,
		"kind":           kind,
		"name":           name,
		"path":           path,
		"qualified_name": qualifiedName,
		"record":         "node",
	}
	if owner != "" {
		record["owner"] = owner
		facts.ownerByNodeID[id] = owner
	}
	if span != nil {
		record["span"] = span
	}
	if len(attributes) != 0 {
		record["attributes"] = attributes
	}
	if existing, ok := facts.nodes[id]; ok {
		// Prefer source-owned evidence over an external placeholder, then the
		// lexicographically smaller representation for deterministic merging.
		if directOwner(existing) != "" || owner == "" {
			return id
		}
	}
	facts.nodes[id] = record
	return id
}

func (facts *factSet) addFileNode(path string, content []byte, span *sourceSpan) string {
	id := facts.addNode("file", path, baseName(path), path, path, path, span, nil)
	facts.nodes[id]["content_id"] = contentID(content)
	return id
}

func (facts *factSet) addEdge(source, target, relation, owner string, span *sourceSpan, attributes map[string]any) {
	record := map[string]any{
		"record":   "edge",
		"relation": relation,
		"source":   source,
		"target":   target,
	}
	if owner != "" {
		record["owner"] = owner
	}
	if span != nil {
		record["span"] = span
	}
	if len(attributes) != 0 {
		record["attributes"] = attributes
	}
	facts.edges[edgeSortKey(record)] = record
}

func (facts *factSet) addUnresolved(source, relation, expression, reason, owner string, span *sourceSpan, attributes map[string]any) {
	record := map[string]any{
		"expression": expression,
		"reason":     reason,
		"record":     "unresolved",
		"relation":   relation,
		"source":     source,
	}
	if owner != "" {
		record["owner"] = owner
	}
	if span != nil {
		record["span"] = span
	}
	if len(attributes) != 0 {
		record["attributes"] = attributes
	}
	facts.unresolved[unresolvedSortKey(record)] = record
}

func (facts *factSet) render(repository string) ([]byte, error) {
	header := map[string]any{
		"adapter_version": adapterVersion,
		"language":        language,
		"mode":            "full",
		"record":          "lexicon",
		"repository":      repository,
		"schema_version":  1,
	}
	nodes := mapValues(facts.nodes)
	edges := mapValues(facts.edges)
	unresolved := mapValues(facts.unresolved)
	sort.Slice(nodes, func(i, j int) bool { return nodeSortKey(nodes[i]) < nodeSortKey(nodes[j]) })
	sort.Slice(edges, func(i, j int) bool { return edgeSortKey(edges[i]) < edgeSortKey(edges[j]) })
	sort.Slice(unresolved, func(i, j int) bool { return unresolvedSortKey(unresolved[i]) < unresolvedSortKey(unresolved[j]) })

	records := make([]map[string]any, 0, 1+len(nodes)+len(edges)+len(unresolved))
	records = append(records, header)
	records = append(records, nodes...)
	records = append(records, edges...)
	records = append(records, unresolved...)

	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return nil, fmt.Errorf("encode facts: %w", err)
		}
	}
	return []byte(output.String()), nil
}

func mapValues(input map[string]map[string]any) []map[string]any {
	values := make([]map[string]any, 0, len(input))
	for _, value := range input {
		values = append(values, value)
	}
	return values
}

func nodeSortKey(record map[string]any) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", record["id"], record["kind"], record["path"], record["qualified_name"], jsonKey(record))
}

func edgeSortKey(record map[string]any) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", record["source"], record["target"], record["relation"], spanSortKey(record), jsonKey(record))
}

func unresolvedSortKey(record map[string]any) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s", record["source"], record["relation"], record["expression"], record["reason"], spanSortKey(record), jsonKey(record))
}

func jsonKey(record map[string]any) string {
	data, _ := json.Marshal(record)
	return string(data)
}

func spanSortKey(record map[string]any) string {
	span, ok := record["span"].(*sourceSpan)
	if !ok || span == nil {
		return ""
	}
	return fmt.Sprintf("%s\x00%08d\x00%08d\x00%08d\x00%08d", span.Path, span.StartLine, span.StartColumn, span.EndLine, span.EndColumn)
}

func directOwner(record map[string]any) string {
	owner, _ := record["owner"].(string)
	return owner
}

func baseName(path string) string {
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		return path[index+1:]
	}
	return path
}
