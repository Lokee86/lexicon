package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const runtimeFixturePath = "testdata/runtime"

func TestRuntimeFixtureEmitsConservativeCalls(t *testing.T) {
	first, err := analyzeRepository(filepath.FromSlash(runtimeFixturePath))
	if err != nil {
		t.Fatal(err)
	}
	second, err := analyzeRepository(filepath.FromSlash(runtimeFixturePath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("runtime analysis changed byte output")
	}
	records := decodeFacts(t, first)
	nodes := factRecords(records, "node")
	edges := factRecords(records, "edge")
	unresolved := factRecords(records, "unresolved")
	assertEdgeEndpoints(t, nodes, edges)

	run := requireRuntimeNode(t, nodes, "method", "runtime.slice.Child.run(Int,ExternalWorker)")
	for _, target := range []map[string]any{
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Child.helper(Int)"),
		requireRuntimeNode(t, nodes, "function", "runtime.slice.top(Int)"),
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Helpers.work(Int)"),
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Factory.Companion.create(Int)"),
		requireRuntimeNode(t, nodes, "constructor", "runtime.slice.Child.<init>(Int)"),
	} {
		requireRuntimeEdge(t, edges, run, target, "calls")
	}
	for _, target := range []map[string]any{
		requireRuntimeNode(t, nodes, "function", "runtime.slice.choose(Int)"),
		requireRuntimeNode(t, nodes, "function", "runtime.slice.choose(String)"),
	} {
		requireRuntimeEdge(t, edges, run, target, "possible-calls")
		assertNoRuntimeEdge(t, edges, run, target, "calls")
	}

	secondaryInt := requireRuntimeNode(t, nodes, "constructor", "runtime.slice.Secondary.<init>(Int)")
	secondaryEmpty := requireRuntimeConstructor(t, nodes, "runtime.slice.Secondary.<init>()", false)
	requireRuntimeEdge(t, edges, secondaryInt,
		requireRuntimeNode(t, nodes, "constructor", "runtime.slice.Parent.<init>(Int)"), "calls")
	requireRuntimeEdge(t, edges, secondaryEmpty, secondaryInt, "calls")
	requireRuntimeEdge(t, edges, secondaryInt,
		requireRuntimeNode(t, nodes, "parameter", "runtime.slice.Secondary.<init>(Int)::parameter:value"), "reads")
	requireRuntimeUnresolved(t, unresolved, run, "worker.run()", "dynamic-target")
	requireRuntimeUnresolved(t, unresolved, run, "println(input)", "external-target")
	requireRuntimeUnresolved(t, unresolved, run, "unsupported()", "unsupported-form")
}

func TestRuntimeFixtureEmitsOverridesAndProvenDataflow(t *testing.T) {
	data, err := analyzeRepository(filepath.FromSlash(runtimeFixturePath))
	if err != nil {
		t.Fatal(err)
	}
	records := decodeFacts(t, data)
	nodes := factRecords(records, "node")
	edges := factRecords(records, "edge")

	childCompute := requireRuntimeNode(t, nodes, "method", "runtime.slice.Child.compute(Int)")
	requireRuntimeEdge(t, edges, childCompute,
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Base.compute(Int)"), "overrides")
	requireRuntimeEdge(t, edges, childCompute,
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Contract.compute(Int)"), "overrides")
	requireRuntimeEdge(t, edges,
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Child.render(Int)"),
		requireRuntimeNode(t, nodes, "method", "runtime.slice.Base.render(Int)"), "overrides")

	run := requireRuntimeNode(t, nodes, "method", "runtime.slice.Child.run(Int,ExternalWorker)")
	count := requireRuntimeNode(t, nodes, "field", "runtime.slice.Child.count")
	state := requireRuntimeNode(t, nodes, "field", "runtime.slice.Helpers.state")
	input := requireRuntimeNode(t, nodes, "parameter", "runtime.slice.Child.run(Int,ExternalWorker)::parameter:input")
	worker := requireRuntimeNode(t, nodes, "parameter", "runtime.slice.Child.run(Int,ExternalWorker)::parameter:worker")
	for _, target := range []map[string]any{count, state, input, worker} {
		requireRuntimeEdge(t, edges, run, target, "reads")
	}
	for _, target := range []map[string]any{count, state} {
		requireRuntimeEdge(t, edges, run, target, "writes")
	}

	delegated := requireRuntimeNode(t, nodes, "field", "runtime.slice.Child.delegated")
	for _, sourceQN := range []string{
		"runtime.slice.Child.run(Int,ExternalWorker)",
		"runtime.slice.Child.shadows(Int)",
		"runtime.slice.Child.destructured(Pair<Int, Int>)",
		"runtime.slice.Child.delegatedLocal()",
		"runtime.slice.Child.nestedLocal()",
		"runtime.slice.Child.nestedLambda()",
	} {
		source := requireRuntimeNode(t, nodes, "method", sourceQN)
		assertNoRuntimeDataflowTarget(t, edges, source, delegated)
		if sourceQN != "runtime.slice.Child.run(Int,ExternalWorker)" {
			assertNoRuntimeDataflowTarget(t, edges, source, count)
		}
	}
}

func TestParserRetainsCallableBodyAndDelegationRanges(t *testing.T) {
	path := filepath.FromSlash(runtimeFixturePath + "/src/main/kotlin/runtime/slice/Runtime.kt")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed := parseKotlinFile(filepath.ToSlash(path), content)
	declarations := flattenRuntimeDeclarations(parsed.declarations)
	var bodies, delegations int
	for _, declaration := range declarations {
		if declaration.kind != "function" && declaration.kind != "constructor" {
			continue
		}
		if declaration.body.end > declaration.body.start {
			bodies++
		}
		if declaration.delegation.end > declaration.delegation.start {
			delegations++
		}
	}
	if bodies == 0 || delegations != 2 {
		t.Fatalf("retained callable ranges: bodies=%d delegations=%d", bodies, delegations)
	}
}

func requireRuntimeNode(t *testing.T, nodes []map[string]any, kind, qualifiedName string) map[string]any {
	t.Helper()
	var found map[string]any
	for _, node := range nodes {
		if node["kind"] == kind && node["qualified_name"] == qualifiedName {
			if found != nil {
				t.Fatalf("multiple %s nodes named %s", kind, qualifiedName)
			}
			found = node
		}
	}
	if found == nil {
		t.Fatalf("missing %s node %s", kind, qualifiedName)
	}
	return found
}

func requireRuntimeConstructor(t *testing.T, nodes []map[string]any, qualifiedName string, primary bool) map[string]any {
	t.Helper()
	for _, node := range nodes {
		if node["kind"] == "constructor" && node["qualified_name"] == qualifiedName && attributes(node)["primary"] == primary {
			return node
		}
	}
	t.Fatalf("missing constructor %s primary=%v", qualifiedName, primary)
	return nil
}

func requireRuntimeEdge(t *testing.T, edges []map[string]any, source, target map[string]any, relation string) {
	t.Helper()
	for _, edge := range edges {
		if edge["source"] == source["id"] && edge["target"] == target["id"] && edge["relation"] == relation {
			return
		}
	}
	t.Fatalf("missing %s from %s to %s", relation, source["qualified_name"], target["qualified_name"])
}

func assertNoRuntimeEdge(t *testing.T, edges []map[string]any, source, target map[string]any, relation string) {
	t.Helper()
	for _, edge := range edges {
		if edge["source"] == source["id"] && edge["target"] == target["id"] && edge["relation"] == relation {
			t.Fatalf("unexpected %s from %s to %s", relation, source["qualified_name"], target["qualified_name"])
		}
	}
}

func assertNoRuntimeDataflowTarget(t *testing.T, edges []map[string]any, source, target map[string]any) {
	t.Helper()
	assertNoRuntimeEdge(t, edges, source, target, "reads")
	assertNoRuntimeEdge(t, edges, source, target, "writes")
}

func requireRuntimeUnresolved(t *testing.T, records []map[string]any, source map[string]any, expression, reason string) {
	t.Helper()
	for _, record := range records {
		if record["source"] == source["id"] && record["relation"] == "calls" && record["expression"] == expression && record["reason"] == reason {
			return
		}
	}
	t.Fatalf("missing unresolved call %q (%s) from %s", expression, reason, source["qualified_name"])
}

func flattenRuntimeDeclarations(input []*declaration) []*declaration {
	var result []*declaration
	for _, declaration := range input {
		result = append(result, declaration)
		result = append(result, flattenRuntimeDeclarations(declaration.children)...)
	}
	return result
}
