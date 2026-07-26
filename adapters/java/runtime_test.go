package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

const runtimeFixture = "testdata/runtime"

func TestRuntimeCallsResolveDefinitePossibleAndExternalTargets(t *testing.T) {
	records := decodeRecords(t, analyzeFixture(t, runtimeFixture))
	nodes := nodeIndex(records)
	caller := "demo.runtime.RuntimeSlice.exercise(int,ExternalReceiver)"

	for _, target := range []string{
		"demo.runtime.RuntimeSlice.definite(int)",
		"demo.runtime.Helper.staticCall(int)",
		"demo.runtime.Helper.<init>(int)",
		"demo.runtime.RuntimeSlice.<init>(int)",
	} {
		assertRuntimeEdge(t, records, nodes, caller, target, "calls")
	}
	for _, target := range []string{
		"demo.runtime.RuntimeSlice.overloaded(int)",
		"demo.runtime.RuntimeSlice.overloaded(String)",
		"demo.runtime.Choice.<init>(int)",
		"demo.runtime.Choice.<init>(String)",
	} {
		assertRuntimeEdge(t, records, nodes, caller, target, "possible-calls")
	}

	assertRuntimeUnresolved(t, records, nodes, caller, "calls", "external.run()", "external-target")
	assertRuntimeUnresolved(t, records, nodes, caller, "calls", "missing(value)", "dynamic-target")
	assertRuntimeUnresolved(t, records, nodes, caller, "calls", "vendor.External.staticCall()", "external-target")
	assertRuntimeUnresolved(t, records, nodes, caller, "calls", "new vendor.External(value)", "external-target")
}

func TestTypedIdentifierReceiverCallsStayBoundedAndScoped(t *testing.T) {
	records := decodeRecords(t, analyzeFixture(t, runtimeFixture))
	nodes := nodeIndex(records)
	target := "demo.receivers.ReceiverTarget.unique(int)"

	assertRuntimeEdge(t, records, nodes, "demo.receivers.ReceiverCalls.parameter(ReceiverTarget)", target, "calls")
	assertRuntimeUnresolved(t, records, nodes, "demo.receivers.ReceiverCalls.parameter(ReceiverTarget)", "calls", "receiver.inherited()", "unsupported-form")
	assertNoRuntimeEdge(t, records, nodes, "demo.receivers.ReceiverCalls.parameter(ReceiverTarget)", "demo.receivers.ReceiverBase.inherited()", "calls")
	assertRuntimeEdge(t, records, nodes, "demo.receivers.ReceiverCalls.local()", target, "calls")
	for _, overload := range []string{
		"demo.receivers.ReceiverTarget.overloaded(int)",
		"demo.receivers.ReceiverTarget.overloaded(String)",
	} {
		assertRuntimeEdge(t, records, nodes, "demo.receivers.ReceiverCalls.overloaded(ReceiverTarget)", overload, "possible-calls")
	}

	assertRuntimeUnresolved(t, records, nodes, "demo.receivers.ReceiverCalls.external(vendor.External)", "calls", "receiver.run()", "external-target")
	assertRuntimeUnresolved(t, records, nodes, "demo.receivers.ReceiverCalls.ambiguous(SharedReceiver)", "calls", "receiver.run()", "ambiguous-target")

	scope := "demo.receivers.ReceiverCalls.scope()"
	assertRuntimeEdge(t, records, nodes, scope, target, "calls")
	assertRuntimeUnresolved(t, records, nodes, scope, "calls", "future.unique(1)", "dynamic-target")
	assertRuntimeUnresolved(t, records, nodes, scope, "calls", "inner.unique(1)", "dynamic-target")
	assertRuntimeUnresolved(t, records, nodes, scope, "calls", "ReceiverTarget.staticOnly()", "unsupported-form")
	assertRuntimeUnresolved(t, records, nodes, scope, "calls", "unique(1)", "dynamic-target")
	assertRuntimeUnresolved(t, records, nodes, scope, "calls", "unknown.unique(1)", "dynamic-target")
	assertNoRuntimeEdge(t, records, nodes, scope, "demo.receivers.ReceiverTarget.staticOnly()", "calls")
	assertAllEndpoints(t, records, nodes)
}

func TestRuntimeConstructorCallsAndOverrides(t *testing.T) {
	records := decodeRecords(t, analyzeFixture(t, runtimeFixture))
	nodes := nodeIndex(records)

	assertRuntimeEdge(t, records, nodes, "demo.runtime.RuntimeSlice.<init>()", "demo.runtime.RuntimeSlice.<init>(int)", "calls")
	assertRuntimeEdge(t, records, nodes, "demo.runtime.RuntimeSlice.<init>(int)", "demo.runtime.Base.<init>(int)", "calls")
	assertRuntimeEdge(t, records, nodes, "demo.runtime.RuntimeSlice.match(String)", "demo.runtime.Base.match(String)", "overrides")
	assertRuntimeEdge(t, records, nodes, "demo.runtime.RuntimeSlice.match(String)", "demo.runtime.Contract.match(String)", "overrides")
}

func TestRuntimeReadsWritesStayOnModeledFieldsAndParameters(t *testing.T) {
	records := decodeRecords(t, analyzeFixture(t, runtimeFixture))
	nodes := nodeIndex(records)
	caller := "demo.runtime.RuntimeSlice.exercise(int,ExternalReceiver)"
	parameter := caller + "#parameter:0:value"

	for _, edge := range []struct {
		relation string
		target   string
	}{
		{"reads", "demo.runtime.RuntimeSlice.field"},
		{"writes", "demo.runtime.RuntimeSlice.field"},
		{"reads", parameter},
		{"writes", parameter},
		{"reads", "demo.runtime.Helper.shared"},
		{"writes", "demo.runtime.Helper.shared"},
	} {
		assertRuntimeEdge(t, records, nodes, caller, edge.target, edge.relation)
	}

	shadow := "demo.runtime.RuntimeSlice.shadow(int)"
	assertNoRuntimeEdge(t, records, nodes, shadow, "demo.runtime.RuntimeSlice.field", "reads")
	assertNoRuntimeEdge(t, records, nodes, shadow, "demo.runtime.RuntimeSlice.field", "writes")
	assertRuntimeUnresolved(t, records, nodes, shadow, "writes", "field", "dynamic-target")
}

func TestRuntimeSemanticsDoNotClaimNestedCallableBodies(t *testing.T) {
	records := decodeRecords(t, analyzeFixture(t, runtimeFixture))
	nodes := nodeIndex(records)
	assertNoRuntimeEdge(
		t, records, nodes, "demo.runtime.RuntimeSlice.nestedBodies()",
		"demo.runtime.RuntimeSlice.definite(int)", "calls",
	)
}

func TestCallableBodyTokenRangesAreRetained(t *testing.T) {
	facts := newFactSet()
	state := &analysisState{
		declarations: make(map[string][]string), facts: facts,
		fields: make(map[string]map[string][]fieldDeclaration), namespaces: make(map[string]string),
		types: make(map[string][]typeDeclaration),
	}
	parseJavaSource(state, "file", "Sample.java", `class Sample {
    Sample() { this(1); }
    Sample(int value) { }
    void run(int value) { value++; }
}`)
	if len(state.callables) != 3 {
		t.Fatalf("retained callables = %d, want 3", len(state.callables))
	}
	for _, callable := range state.callables {
		if callable.bodyStart < 1 || callable.bodyEnd < callable.bodyStart || callable.tokens[callable.bodyStart-1].text != "{" || callable.tokens[callable.bodyEnd].text != "}" {
			t.Fatalf("invalid body range for %s: [%d,%d)", callable.name, callable.bodyStart, callable.bodyEnd)
		}
	}
}

func TestRuntimeSemanticsAreCanonicalDeterministicAndCheckoutIndependent(t *testing.T) {
	first := analyzeFixture(t, runtimeFixture)
	if second := analyzeFixture(t, runtimeFixture); !bytes.Equal(first, second) {
		t.Fatal("repeat runtime analysis was not byte-identical")
	}
	assertCanonicalJSONL(t, first)

	temporary := t.TempDir()
	left := filepath.Join(temporary, "left", "runtime")
	right := filepath.Join(temporary, "right", "runtime")
	copyTree(t, runtimeFixture, left)
	copyTree(t, runtimeFixture, right)
	if leftData, rightData := analyzeFixture(t, left), analyzeFixture(t, right); !bytes.Equal(leftData, rightData) {
		t.Fatal("absolute checkout path affected runtime output")
	}
}

func assertRuntimeEdge(t *testing.T, records []map[string]any, nodes map[string]map[string]any, sourceName, targetName, relation string) {
	t.Helper()
	source, target := nodes[sourceName], nodes[targetName]
	if source == nil || target == nil {
		t.Fatalf("missing runtime endpoint %s -> %s", sourceName, targetName)
	}
	for _, record := range records {
		if record["record"] == "edge" && record["relation"] == relation && record["source"] == source["id"] && record["target"] == target["id"] {
			return
		}
	}
	t.Fatalf("missing %s edge %s -> %s", relation, sourceName, targetName)
}

func assertNoRuntimeEdge(t *testing.T, records []map[string]any, nodes map[string]map[string]any, sourceName, targetName, relation string) {
	t.Helper()
	source, target := nodes[sourceName], nodes[targetName]
	for _, record := range records {
		if record["record"] == "edge" && record["relation"] == relation && record["source"] == source["id"] && record["target"] == target["id"] {
			t.Fatalf("unexpected %s edge %s -> %s", relation, sourceName, targetName)
		}
	}
}

func assertRuntimeUnresolved(t *testing.T, records []map[string]any, nodes map[string]map[string]any, sourceName, relation, expression, reason string) {
	t.Helper()
	source := nodes[sourceName]
	for _, record := range records {
		if record["record"] == "unresolved" && record["source"] == source["id"] && record["relation"] == relation && record["expression"] == expression && record["reason"] == reason {
			return
		}
	}
	t.Fatalf("missing unresolved %s %s %q (%s)", sourceName, relation, expression, reason)
}
