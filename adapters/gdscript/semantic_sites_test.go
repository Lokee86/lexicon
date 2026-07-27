package main

import (
	"reflect"
	"testing"
)

func TestSemanticSitesPrecomputeContextsCallsAndAssignments(t *testing.T) {
	function := declaration{
		kind:         "function",
		nodeID:       "run-id",
		ownerClassID: "owner-id",
		indent:       0,
		span: sourceSpan{
			"start_line": 1, "start_column": 1,
			"end_line": 1, "end_column": 17,
		},
	}
	callTokens := []token{
		{kind: tokenIdentifier, text: "target", line: 2, column: 5, endLine: 2, endColumn: 11},
		{kind: tokenSymbol, text: ".", line: 2, column: 11, endLine: 2, endColumn: 12},
		{kind: tokenIdentifier, text: "ping", line: 2, column: 12, endLine: 2, endColumn: 16},
		{kind: tokenSymbol, text: "(", line: 2, column: 16, endLine: 2, endColumn: 17},
		{kind: tokenIdentifier, text: "value", line: 2, column: 17, endLine: 2, endColumn: 22},
		{kind: tokenSymbol, text: ")", line: 2, column: 22, endLine: 2, endColumn: 23},
	}
	assignmentTokens := []token{
		{kind: tokenIdentifier, text: "value", line: 3, column: 5, endLine: 3, endColumn: 10},
		{kind: tokenSymbol, text: "=", line: 3, column: 11, endLine: 3, endColumn: 12},
		{kind: tokenIdentifier, text: "target", line: 3, column: 13, endLine: 3, endColumn: 19},
	}
	file := &parsedFile{
		path:          "fixture.gd",
		scriptOwnerID: "module-id",
		declarations:  []declaration{function},
		statements: []statement{
			{tokens: callTokens, indent: 4, start: callTokens[0], end: callTokens[len(callTokens)-1]},
			{tokens: assignmentTokens, indent: 4, start: assignmentTokens[0], end: assignmentTokens[len(assignmentTokens)-1]},
		},
	}

	calls := buildArgumentCallSites([]*parsedFile{file})[file]
	if len(calls) != 1 || calls[0].context.functionID != function.nodeID || calls[0].call.name != "ping" {
		t.Fatalf("argument sites = %#v", calls)
	}
	statements := buildInferenceStatementSites([]*parsedFile{file})[file]
	if len(statements) != 2 || statements[1].context.functionID != function.nodeID || statements[1].assignment != 1 || statements[1].declaration {
		t.Fatalf("inference sites = %#v", statements)
	}
}

func TestRuntimeReceiverOwnersUsesTransitiveChildIndex(t *testing.T) {
	facts := &factSet{}
	facts.indexParent("child", "base")
	facts.indexParent("grandchild", "child")
	model := &semanticModel{facts: facts}
	want := []string{"base", "child", "grandchild"}
	if got := model.runtimeReceiverOwners([]string{"base"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime receiver owners = %v, want %v", got, want)
	}

	fallbackFacts := &factSet{parentByOwnerID: map[string][]string{
		"child":      {"base"},
		"grandchild": {"child"},
	}}
	fallbackModel := &semanticModel{facts: fallbackFacts}
	if got := fallbackModel.runtimeReceiverOwners([]string{"base"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback runtime receiver owners = %v, want %v", got, want)
	}
}
