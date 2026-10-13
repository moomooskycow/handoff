package handoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteToFileRefusesExistingSymlink(t *testing.T) {
	for _, targetExists := range []bool{false, true} {
		t.Run(fmt.Sprintf("target_exists=%t", targetExists), func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target.txt")
			const original = "original target content"
			if targetExists {
				if err := os.WriteFile(target, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			output := filepath.Join(dir, "output.md")
			if err := os.Symlink(target, output); err != nil {
				t.Skipf("symlinks are unavailable: %v", err)
			}

			if err := WriteToFile("replacement", output, false); !errors.Is(err, ErrFileExists) {
				t.Errorf("WriteToFile() error = %v, want ErrFileExists", err)
			} else if !strings.Contains(err.Error(), output) {
				t.Errorf("overwrite error does not identify output path: %v", err)
			}
			if got, err := os.Readlink(output); err != nil || got != target {
				t.Errorf("output symlink changed: target = %q, error = %v", got, err)
			}
			if targetExists {
				if got, err := os.ReadFile(target); err != nil || string(got) != original {
					t.Errorf("target changed: content = %q, error = %v", got, err)
				}
			} else if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("missing symlink target was created: error = %v", err)
			}

			// Explicit overwrite retains the existing behavior of following symlinks.
			if err := WriteToFile("replacement", output, true); err != nil {
				t.Fatalf("WriteToFile() with overwrite=true failed: %v", err)
			}
			if got, err := os.ReadFile(target); err != nil || string(got) != "replacement" {
				t.Errorf("forced target content = %q, error = %v", got, err)
			}
			if got, err := os.Readlink(output); err != nil || got != target {
				t.Errorf("forced output symlink changed: target = %q, error = %v", got, err)
			}
		})
	}
}

func TestWriteToFileConcurrentNoOverwrite(t *testing.T) {
	const writers = 32
	output := filepath.Join(t.TempDir(), "nested", "output.md")
	start := make(chan struct{})
	type result struct {
		content string
		err     error
	}
	results := make(chan result, writers)
	for i := 0; i < writers; i++ {
		content := strings.Repeat(fmt.Sprintf("writer-%02d\n", i), 1024)
		go func() {
			<-start
			results <- result{content, WriteToFile(content, output, false)}
		}()
	}
	close(start)

	winners := 0
	var winner string
	for i := 0; i < writers; i++ {
		result := <-results
		if result.err == nil {
			winners++
			winner = result.content
		} else if !errors.Is(result.err, ErrFileExists) {
			t.Errorf("losing writer error = %v, want ErrFileExists", result.err)
		}
	}
	if winners != 1 {
		t.Fatalf("successful writers = %d, want exactly one", winners)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != winner {
		t.Errorf("output differs from the successful writer: error = %v", err)
	}
}

func TestWriteToFileOutputErrors(t *testing.T) {
	t.Run("existing directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteToFile("content", dir, false); !errors.Is(err, ErrFileExists) {
			t.Errorf("WriteToFile() error = %v, want ErrFileExists", err)
		}
		err := WriteToFile("content", dir, true)
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || !strings.Contains(err.Error(), dir) {
			t.Errorf("forced directory output error = %v, want wrapped error with path", err)
		}
	})
	t.Run("parent is a file", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "parent")
		if err := os.WriteFile(parent, []byte("preserve parent"), 0600); err != nil {
			t.Fatal(err)
		}
		err := WriteToFile("content", filepath.Join(parent, "output.md"), false)
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("WriteToFile() error = %v, want wrapped filesystem error", err)
		}
		if got, err := os.ReadFile(parent); err != nil || string(got) != "preserve parent" {
			t.Errorf("parent file changed: content = %q, error = %v", got, err)
		}
	})
}
