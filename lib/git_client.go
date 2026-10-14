package handoff

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitClient is an interface that abstracts git operations needed by the handoff package.
// This interface allows for dependency injection and easier testing without relying
// on the actual git executable or global state.
type GitClient interface {
	// IsAvailable returns true if git functionality is available
	IsAvailable() bool

	// IsGitIgnored checks if a file is ignored by git
	IsGitIgnored(file string) bool

	// GetGitFiles retrieves files from a directory using git ls-files
	GetGitFiles(dir string) ([]string, error)
}

// RealGitClient is the default implementation of GitClient that uses
// the actual git executable on the system.
type RealGitClient struct {
	// gitAvailable indicates whether the git command is available on the system.
	// This field is initialized during construction and cached for later use.
	gitAvailable bool
}

// NewRealGitClient creates a new RealGitClient instance and determines
// git availability by checking if the git executable is in the PATH.
func NewRealGitClient() *RealGitClient {
	_, err := exec.LookPath("git")
	return &RealGitClient{
		gitAvailable: err == nil,
	}
}

// IsAvailable returns whether git is available on the system.
func (c *RealGitClient) IsAvailable() bool {
	return c.gitAvailable
}

// IsGitIgnored checks if a file is ignored by git.
// If git is not available, it falls back to checking if the file is hidden
// (starts with a dot).
func (c *RealGitClient) IsGitIgnored(file string) bool {
	if !c.gitAvailable {
		return strings.HasPrefix(filepath.Base(file), ".")
	}

	dir := filepath.Dir(file)
	filename := filepath.Base(file)
	cmd := exec.Command("git", "-C", dir, "check-ignore", "-q", "--", filename)
	err := cmd.Run()

	if err == nil {
		// Exit code 0: file is ignored
		return true
	}

	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		// Exit code 1: file is not ignored
		return false
	}

	// Other errors (e.g., not a git repo): fall back to checking if hidden
	return strings.HasPrefix(filename, ".")
}

// GetGitFiles retrieves files from a directory using Git's ls-files command.
// If git is not available or the directory is not a git repository, it returns
// an appropriate error.
func (c *RealGitClient) GetGitFiles(dir string) ([]string, error) {
	if !c.gitAvailable {
		return nil, fmt.Errorf("git not available")
	}

	cmd := exec.Command("git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
			return nil, fmt.Errorf("not a git repository")
		}
		return nil, fmt.Errorf("error running git ls-files: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, filepath.Join(dir, line))
		}
	}
	return files, nil
}

// MockGitClient is a mock implementation of GitClient used for testing.
// It allows controlling git availability and behavior without requiring
// an actual git executable or repository.
type MockGitClient struct {
	available    bool
	ignoredFiles map[string]bool
	filesInDir   map[string][]string
}

// NewMockGitClient creates a new MockGitClient with the specified availability.
func NewMockGitClient(available bool) *MockGitClient {
	return &MockGitClient{
		available:    available,
		ignoredFiles: make(map[string]bool),
		filesInDir:   make(map[string][]string),
	}
}

// IsAvailable returns whether git is available as configured in the mock.
func (m *MockGitClient) IsAvailable() bool {
	return m.available
}

// IsGitIgnored checks if a file is ignored based on the mock configuration.
// If the file isn't explicitly configured, it falls back to checking if
// it's a hidden file (starts with a dot).
func (m *MockGitClient) IsGitIgnored(file string) bool {
	if ignored, ok := m.ignoredFiles[file]; ok {
		return ignored
	}
	// Fall back to checking if it's a hidden file
	return strings.HasPrefix(filepath.Base(file), ".")
}

// GetGitFiles returns the files configured for the specified directory.
// If the directory isn't configured, it returns an error indicating git
// is not available.
func (m *MockGitClient) GetGitFiles(dir string) ([]string, error) {
	if !m.available {
		return nil, fmt.Errorf("git not available")
	}

	if files, ok := m.filesInDir[dir]; ok {
		return files, nil
	}

	return nil, fmt.Errorf("not a git repository")
}

// SetIgnoredFiles configures which files should be considered as ignored by git.
func (m *MockGitClient) SetIgnoredFiles(files map[string]bool) {
	m.ignoredFiles = files
}

// SetFilesInDir configures which files should be returned for a specific directory.
func (m *MockGitClient) SetFilesInDir(dir string, files []string) {
	m.filesInDir[dir] = files
}
