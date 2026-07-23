package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorderWritesPhasesCountsAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles", "scan.json")
	recorder := New(path, "scan")
	finish := recorder.Measure("adapter.run", "go", "full")
	recorder.Add("files_changed", 2)
	finish()
	if err := recorder.Write(errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Operation != "scan" || report.Status != "error" || report.Error != "boom" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Counts["files_changed"] != 2 || len(report.Phases) != 1 {
		t.Fatalf("unexpected metrics: %+v", report)
	}
	if report.Phases[0].Language != "go" || report.Phases[0].Mode != "full" || report.Phases[0].DurationNS < 0 {
		t.Fatalf("unexpected phase: %+v", report.Phases[0])
	}
}

func TestRecorderMergesAdapterProfile(t *testing.T) {
	directory := t.TempDir()
	adapterPath := filepath.Join(directory, "adapter.json")
	if err := os.WriteFile(adapterPath, []byte(`{
  "version": 1,
  "phases": [{"name":"parsing","duration_ns":42}],
  "counts": {"files":3}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := New(filepath.Join(directory, "scan.json"), "scan")
	if err := recorder.MergeAdapter(adapterPath, "go", "full"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Write(nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "scan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Phases) != 1 || report.Phases[0].Name != "adapter.parsing" || report.Phases[0].Language != "go" || report.Phases[0].Mode != "full" {
		t.Fatalf("unexpected merged phase: %+v", report.Phases)
	}
	if report.Counts["adapter_files"] != 3 || report.Counts["adapter_go_files"] != 3 {
		t.Fatalf("unexpected merged counts: %+v", report.Counts)
	}
}

func TestNilRecorderIsNoop(t *testing.T) {
	var recorder *Recorder
	recorder.Measure("ignored", "", "")()
	recorder.Add("ignored", 1)
	recorder.Set("ignored", 1)
	if err := recorder.MergeAdapter("missing", "go", "full"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Write(nil); err != nil {
		t.Fatal(err)
	}
}
