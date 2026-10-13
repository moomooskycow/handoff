package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIOutputSymlinkRequiresForce(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	const content = "synthetic source content"
	if err := os.WriteFile(source, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.md")
	output := filepath.Join(dir, "output.md")
	if err := os.Symlink(target, output); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	_, stderr, err := runCliCommand(t, binary, "-ignore-gitignore", "-output="+output, source)
	if err == nil {
		t.Error("CLI succeeded without -force for an existing output symlink")
	}
	if !strings.Contains(stderr, output) || !strings.Contains(stderr, "overwrite") {
		t.Errorf("missing useful output-path/overwrite error: %s", stderr)
	}
	if strings.Contains(stderr, "successfully written") || strings.Contains(stderr, "Handoff complete") {
		t.Errorf("CLI reported success after refusing output: %s", stderr)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("output target was created without -force: %v", err)
	}

	_, stderr, err = runCliCommand(t, binary, "-ignore-gitignore", "-force", "-output="+output, source)
	if err != nil {
		t.Fatalf("CLI failed with -force: %v\n%s", err, stderr)
	}
	if got, err := os.ReadFile(target); err != nil || !strings.Contains(string(got), content) {
		t.Errorf("forced output missing source: content = %q, error = %v", got, err)
	}
	if got, err := os.Readlink(output); err != nil || got != target {
		t.Errorf("-force changed output symlink: target = %q, error = %v", got, err)
	}
}
