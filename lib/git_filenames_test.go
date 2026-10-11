package handoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRealGitClientPreservesFilenames(t *testing.T) {
	names := []string{" leading.txt", "plain.txt", "résumé.txt", "日本語.txt", "nested/file name.txt"}
	if runtime.GOOS != "windows" {
		// Windows does not support these filenames.
		names = append(names, "tab\tname.txt", "line\nbreak.txt", `quote"name.txt`, `back\slash.txt`, "z-trailing.txt ")
	}
	for _, quotePath := range []string{"true", "false"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("quotePath=%s/tracked=%t", quotePath, tracked), func(t *testing.T) {
				dir := newFilenameTestRepository(t)
				runGitForFilenames(t, dir, "config", "core.quotePath", quotePath)
				var want []string
				for _, name := range names {
					filename := filepath.Join(dir, filepath.FromSlash(name))
					if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filename, []byte("contents of "+name), 0644); err != nil {
						t.Fatal(err)
					}
					want = append(want, filename)
				}
				if tracked {
					runGitForFilenames(t, dir, "add", "--", ".")
				}

				got, err := NewRealGitClient().GetGitFiles(dir)
				if err != nil {
					t.Fatal(err)
				}
				slices.Sort(got)
				slices.Sort(want)
				if !slices.Equal(got, want) {
					t.Errorf("GetGitFiles() = %q, want exact paths %q", got, want)
				}

				content, stats, err := ProcessProject([]string{dir}, NewConfig())
				if err != nil {
					t.Fatal(err)
				}
				if stats.FilesTotal != len(names) || stats.FilesProcessed != len(names) {
					t.Errorf("processed %d of %d discovered files, want %d of %d", stats.FilesProcessed, stats.FilesTotal, len(names), len(names))
				}
				for _, name := range names {
					if !strings.Contains(content, "contents of "+name) {
						t.Errorf("ProcessProject() omitted content of %q", name)
					}
				}
			})
		}
	}
}

func TestRealGitClientEmptyRepository(t *testing.T) {
	dir := newFilenameTestRepository(t)
	files, err := NewRealGitClient().GetGitFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("GetGitFiles() = %q, want no files", files)
	}
}

func TestRealGitClientPreservesBoundaryWhitespace(t *testing.T) {
	names := []string{" leading.txt"}
	if runtime.GOOS != "windows" {
		names = append(names, "trailing.txt ")
	}
	for _, name := range names {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q/tracked=%t", name, tracked), func(t *testing.T) {
				dir := newFilenameTestRepository(t)
				filename := filepath.Join(dir, name)
				if err := os.WriteFile(filename, []byte("content"), 0644); err != nil {
					t.Fatal(err)
				}
				if tracked {
					runGitForFilenames(t, dir, "add", "--", name)
				}
				files, err := NewRealGitClient().GetGitFiles(dir)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(files, []string{filename}) {
					t.Errorf("GetGitFiles() = %q, want exact path %q", files, filename)
				}
			})
		}
	}
}

func TestRealGitClientIncludesTrackedAndUntrackedFiles(t *testing.T) {
	dir := newFilenameTestRepository(t)
	for _, name := range []string{"tracked.txt", "untracked.txt", "ignored.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runGitForFilenames(t, dir, "add", "--", "tracked.txt")
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte("ignored.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := NewRealGitClient().GetGitFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	want := []string{filepath.Join(dir, "tracked.txt"), filepath.Join(dir, "untracked.txt")}
	if !slices.Equal(files, want) {
		t.Errorf("GetGitFiles() = %q, want %q", files, want)
	}
}

func newFilenameTestRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for filename integration tests")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	runGitForFilenames(t, dir, "init", "--quiet")
	return dir
}

func runGitForFilenames(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
