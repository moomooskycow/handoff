package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIGitIgnoreDashFilename(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	for name, content := range map[string]string{
		".gitignore": "-ignored.txt\n", "-ignored.txt": "excluded synthetic marker", "allowed.txt": "allowed synthetic marker",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	binary := buildBinary(t)
	for _, bypass := range []bool{false, true} {
		args := []string{"-dry-run"}
		if bypass {
			args = append(args, "-ignore-gitignore")
		}
		args = append(args, filepath.Join(dir, "allowed.txt"), filepath.Join(dir, "-ignored.txt"))
		stdout, stderr, err := runCliCommand(t, binary, args...)
		if err != nil {
			t.Fatalf("bypass=%t: CLI failed: %v\n%s", bypass, err, stderr)
		}
		if !strings.Contains(stdout, "allowed synthetic marker") {
			t.Errorf("bypass=%t: allowed content missing", bypass)
		}
		if strings.Contains(stdout, "excluded synthetic marker") != bypass {
			t.Errorf("bypass=%t: ignored content inclusion incorrect: %s", bypass, stdout)
		}
		wantCount := "Files: 1/2"
		if bypass {
			wantCount = "Files: 2/2"
		}
		if !strings.Contains(stderr, wantCount) {
			t.Errorf("bypass=%t: missing %q in diagnostics: %s", bypass, wantCount, stderr)
		}
	}
}
