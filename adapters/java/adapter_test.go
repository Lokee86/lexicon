package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const foundationFixture = "testdata/foundation"

func TestFoundationDeclarationsImportsAndContainment(t *testing.T) {
	data := analyzeFixture(t, foundationFixture)
	records := decodeRecords(t, data)
	if header := records[0]; header["record"] != "lexicon" || header["language"] != "java" || header["mode"] != "full" || header["schema_version"] != float64(1) {
		t.Fatalf("unexpected header: %#v", header)
	}

	nodes := nodeIndex(records)
	assertDeclaration(t, nodes, "com.acme.Service", "type", "class")
	assertDeclaration(t, nodes, "com.acme.Worker", "interface", "interface")
	assertDeclaration(t, nodes, "com.acme.Mode", "type", "enum")
	assertDeclaration(t, nodes, "com.acme.Result", "type", "record")
	assertDeclaration(t, nodes, "com.acme.Marker", "interface", "annotation")
	assertDeclaration(t, nodes, "com.acme.Service.<init>(Helper)", "constructor", "constructor")
	assertDeclaration(t, nodes, "com.acme.Result.<init>(String,int)", "constructor", "compact-constructor")
	assertDeclaration(t, nodes, "com.acme.Service.execute(String,int...)", "method", "method")
	assertDeclaration(t, nodes, "com.acme.Service.helper", "field", "field")
	assertDeclaration(t, nodes, "com.acme.Service.count", "field", "field")
	assertDeclaration(t, nodes, "com.acme.Service.limit", "field", "field")
	assertDeclaration(t, nodes, "com.acme.Result.value", "field", "record-component")
	assertDeclaration(t, nodes, "com.acme.Result.count", "field", "record-component")
	assertDeclaration(t, nodes, "com.acme.Service.Nested", "type", "class")

	parameterNames := map[string]bool{}
	for _, node := range nodes {
		if node["kind"] == "parameter" {
			parameterNames[node["name"].(string)] = true
		}
	}
	for _, name := range []string{"helper", "input", "retries", "value"} {
		if !parameterNames[name] {
			t.Errorf("missing parameter %q", name)
		}
	}

	assertAllEndpoints(t, records, nodes)
	assertContains(t, records, nodes, "com.acme.Service", "com.acme.Service.execute(String,int...)")
	assertContains(t, records, nodes, "com.acme.Service.execute(String,int...)", "com.acme.Service.execute(String,int...)#parameter:0:input")
	assertContains(t, records, nodes, "com.acme.Service", "com.acme.Service.Nested")
	assertImport(t, records, nodes, "com.acme.support.Helper")
	assertImport(t, records, nodes, "com.acme.support.Helper.DEFAULT_NAME")
	assertImportExpression(t, records, nodes, "import com.acme.support.*;", "com.acme.support")
	assertImportExpression(t, records, nodes, "import static com.acme.support.Helper.*;", "com.acme.support.Helper")
	assertUnresolved(t, records, "imports", "import java.util.List;", "external-target")
	assertUnresolvedRelation(t, records, "defines", "unsupported-form", "src/main/java/com/acme/broken/Broken.java")

	for _, node := range nodes {
		path := node["path"].(string)
		if strings.Contains(path, "\\") {
			t.Errorf("path is not normalized: %q", path)
		}
		if strings.Contains(path, "vendor") || node["name"] == "IgnoredVendorSource" {
			t.Errorf("permanently excluded source was emitted: %#v", node)
		}
	}
}

func TestFoundationIsCanonicalDeterministicAndCheckoutIndependent(t *testing.T) {
	first := analyzeFixture(t, foundationFixture)
	second := analyzeFixture(t, foundationFixture)
	if !bytes.Equal(first, second) {
		t.Fatal("repeat analysis was not byte-identical")
	}
	assertCanonicalJSONL(t, first)

	temporary := t.TempDir()
	left := filepath.Join(temporary, "left", "foundation")
	right := filepath.Join(temporary, "right", "foundation")
	copyTree(t, foundationFixture, left)
	copyTree(t, foundationFixture, right)
	leftData := analyzeFixture(t, left)
	rightData := analyzeFixture(t, right)
	if !bytes.Equal(leftData, rightData) {
		t.Fatal("absolute checkout path affected adapter output")
	}

	records := decodeRecords(t, first)
	service := nodeIndex(records)["com.acme.Service"]
	payload := []byte("lexicon:v1\x00java\x00type\x00com.acme.Service")
	digest := sha256.Sum256(payload)
	expected := "sha256:" + hex.EncodeToString(digest[:])
	if service["id"] != expected {
		t.Fatalf("Service id = %v, want %s", service["id"], expected)
	}
}

func TestCLIWritesStdoutAndFile(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"--repo", foundationFixture, "--output", "-"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if records := decodeRecords(t, stdout.Bytes()); len(records) < 2 {
		t.Fatalf("stdout emitted %d records", len(records))
	}

	output := filepath.Join(t.TempDir(), "facts.jsonl")
	if err := run([]string{"--repo", foundationFixture, "--output", output}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), written) {
		t.Fatal("stdout and file output differ")
	}
}

func analyzeFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := analyzeRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeRecords(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var records []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func nodeIndex(records []map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for _, record := range records {
		if record["record"] == "node" {
			result[record["qualified_name"].(string)] = record
		}
	}
	return result
}

func assertDeclaration(t *testing.T, nodes map[string]map[string]any, qualifiedName, kind, declarationKind string) {
	t.Helper()
	node := nodes[qualifiedName]
	if node == nil {
		t.Fatalf("missing declaration %s", qualifiedName)
	}
	attributes, _ := node["attributes"].(map[string]any)
	if node["kind"] != kind || attributes["declaration_kind"] != declarationKind {
		t.Fatalf("declaration %s = %#v", qualifiedName, node)
	}
}

func assertAllEndpoints(t *testing.T, records []map[string]any, nodes map[string]map[string]any) {
	t.Helper()
	ids := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		ids[node["id"].(string)] = true
	}
	for _, record := range records {
		if record["record"] == "edge" && (!ids[record["source"].(string)] || !ids[record["target"].(string)]) {
			t.Fatalf("edge has missing endpoint: %#v", record)
		}
		if record["record"] == "unresolved" && !ids[record["source"].(string)] {
			t.Fatalf("unresolved record has missing source: %#v", record)
		}
	}
}

func assertContains(t *testing.T, records []map[string]any, nodes map[string]map[string]any, sourceName, targetName string) {
	t.Helper()
	source, target := nodes[sourceName], nodes[targetName]
	if source == nil || target == nil {
		t.Fatalf("missing containment endpoint %s -> %s", sourceName, targetName)
	}
	for _, record := range records {
		if record["record"] == "edge" && record["relation"] == "contains" && record["source"] == source["id"] && record["target"] == target["id"] {
			return
		}
	}
	t.Fatalf("missing contains edge %s -> %s", sourceName, targetName)
}

func assertImport(t *testing.T, records []map[string]any, nodes map[string]map[string]any, targetName string) {
	t.Helper()
	target := nodes[targetName]
	if target == nil {
		t.Fatalf("missing import target %s", targetName)
	}
	for _, record := range records {
		if record["record"] == "edge" && record["relation"] == "imports" && record["target"] == target["id"] {
			return
		}
	}
	t.Fatalf("missing resolved import to %s", targetName)
}

func assertImportExpression(t *testing.T, records []map[string]any, nodes map[string]map[string]any, expression, targetName string) {
	t.Helper()
	target := nodes[targetName]
	if target == nil {
		t.Fatalf("missing import target %s", targetName)
	}
	importIDs := make(map[string]bool)
	for _, node := range nodes {
		attributes, _ := node["attributes"].(map[string]any)
		if node["kind"] == "import" && attributes["expression"] == expression {
			importIDs[node["id"].(string)] = true
		}
	}
	for _, record := range records {
		if record["record"] == "edge" && record["relation"] == "imports" && importIDs[record["source"].(string)] && record["target"] == target["id"] {
			return
		}
	}
	t.Fatalf("missing resolved import %q to %s", expression, targetName)
}

func assertUnresolved(t *testing.T, records []map[string]any, relation, expression, reason string) {
	t.Helper()
	for _, record := range records {
		if record["record"] == "unresolved" && record["relation"] == relation && record["expression"] == expression && record["reason"] == reason {
			return
		}
	}
	t.Fatalf("missing unresolved %s %q (%s)", relation, expression, reason)
}

func assertUnresolvedRelation(t *testing.T, records []map[string]any, relation, reason, owner string) {
	t.Helper()
	for _, record := range records {
		if record["record"] == "unresolved" && record["relation"] == relation && record["reason"] == reason && record["owner"] == owner {
			return
		}
	}
	t.Fatalf("missing unresolved %s (%s) owned by %s", relation, reason, owner)
}

func assertCanonicalJSONL(t *testing.T, data []byte) {
	t.Helper()
	idPattern := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for index, line := range lines {
		var ordered map[string]any
		if err := json.Unmarshal([]byte(line), &ordered); err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(ordered))
		for key := range ordered {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var compact bytes.Buffer
		encoder := json.NewEncoder(&compact)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(ordered); err != nil {
			t.Fatal(err)
		}
		if compact.String() != line+"\n" {
			t.Fatalf("line %d is not canonically encoded", index+1)
		}
		if id, ok := ordered["id"].(string); ok && !idPattern.MatchString(id) {
			t.Fatalf("invalid id on line %d: %s", index+1, id)
		}
	}

	records := decodeRecords(t, data)[1:]
	ordered := append([]map[string]any(nil), records...)
	sort.Slice(ordered, func(left, right int) bool {
		return testRecordKey(ordered[left]) < testRecordKey(ordered[right])
	})
	if !reflect.DeepEqual(records, ordered) {
		t.Fatal("records are not in canonical facts-v1 order")
	}
}

func testRecordKey(record map[string]any) string {
	switch record["record"] {
	case "node":
		return "0\x00" + record["id"].(string) + "\x00" + record["kind"].(string) + "\x00" + record["path"].(string) + "\x00" + record["qualified_name"].(string)
	case "edge":
		return "1\x00" + record["source"].(string) + "\x00" + record["target"].(string) + "\x00" + record["relation"].(string) + "\x00" + testSpanKey(record)
	default:
		return "2\x00" + record["source"].(string) + "\x00" + record["relation"].(string) + "\x00" + record["expression"].(string) + "\x00" + record["reason"].(string) + "\x00" + testSpanKey(record)
	}
}

func testSpanKey(record map[string]any) string {
	value, _ := record["span"].(map[string]any)
	if value == nil {
		return ""
	}
	encoded, _ := json.Marshal([]any{value["path"], value["start_line"], value["start_column"], value["end_line"], value["end_column"]})
	return string(encoded)
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
}
