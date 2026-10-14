package handoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitIgnoreTreatsDashNamesAsPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n-ignored.txt\n--help\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := NewRealGitClient()
	for _, tc := range []struct {
		name    string
		ignored bool
	}{
		{"ignored.txt", true}, {"-ignored.txt", true}, {"--help", true},
		{"allowed.txt", false}, {"-allowed.txt", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			const marker = "synthetic fixture content"
			if err := os.WriteFile(path, []byte(marker), 0600); err != nil {
				t.Fatal(err)
			}
			if got := client.IsGitIgnored(path); got != tc.ignored {
				t.Errorf("IsGitIgnored(%q) = %t, want %t", tc.name, got, tc.ignored)
			}
			// Include an allowed companion so filtering can be verified without conflating
			// it with the no-files-processed error contract.
			companion := filepath.Join(dir, "companion.txt")
			if err := os.WriteFile(companion, []byte("allowed companion"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, bypass := range []bool{false, true} {
				content, stats, err := ProcessProject([]string{companion, path}, NewConfig(WithIgnoreGitignore(bypass)))
				if err != nil {
					t.Fatal(err)
				}
				wantIncluded := !tc.ignored || bypass
				if strings.Contains(content, marker) != wantIncluded {
					t.Errorf("bypass=%t: content inclusion = %t, want %t", bypass, strings.Contains(content, marker), wantIncluded)
				}
				wantCount := 1
				if wantIncluded {
					wantCount++
				}
				if stats.FilesProcessed != wantCount || stats.FilesTotal != 2 {
					t.Errorf("bypass=%t: stats = %+v, want %d/2 files", bypass, stats, wantCount)
				}
			}
		})
	}
}
