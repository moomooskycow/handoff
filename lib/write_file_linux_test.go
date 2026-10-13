package handoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestWriteToFileReportsWriteFailure(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skipf("/dev/full is unavailable: %v", err)
	}
	output := filepath.Join(t.TempDir(), "output.md")
	if err := os.Symlink("/dev/full", output); err != nil {
		t.Fatal(err)
	}
	err := WriteToFile("synthetic content", output, true)
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("WriteToFile() error = %v, want wrapped ENOSPC", err)
	}
	if !strings.Contains(err.Error(), output) {
		t.Errorf("write error does not identify output path: %v", err)
	}
}
