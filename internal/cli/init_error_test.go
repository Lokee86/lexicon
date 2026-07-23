package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWithLanguagesReturnsInitializationError(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/profiletest\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(adapterRoot, "python"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"init",
		"--repo", repository,
		"--adapters", adapterRoot,
		"--languages", "go",
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("init unexpectedly succeeded: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "initialized Lexicon:") || strings.Contains(stdout.String(), "snapshot:") {
		t.Fatalf("init printed success output on failure: %q", stdout.String())
	}
}
