// Package handoff provides functionality for collecting and formatting file contents
// from multiple files and directories for sharing with AI assistants or other applications.
//
// The package supports file filtering by extension or name, respects Git's ignore rules,
// detects and skips binary files, and provides customizable output formatting.
//
// Basic usage:
//
//	config := handoff.NewConfig()
//	config.Include = ".go,.md"  // Only include Go and Markdown files
//	config.ProcessConfig()      // Process string-based config into slice-based filters
//
//	content, err := handoff.ProcessProject([]string{"./src", "README.md"}, config)
//	if err != nil {
//	    // Handle error
//	}
//
//	// Use the formatted content or write it to a file
//	handoff.WriteToFile(content, "output.md")
package handoff

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// ErrNoFilesProcessed is returned when paths were provided, files were found,
// but no files were processed due to filtering
var ErrNoFilesProcessed = errors.New("no files were processed from the provided paths")

// ErrFileExists is returned when WriteToFile is called with overwrite=false and the file already exists
var ErrFileExists = errors.New("file already exists and overwrite is not allowed")

// Option is a function that configures a Config instance.
// It implements the functional options pattern for configuration.
type Option func(*Config)

// Config holds all configuration options for file processing and output formatting.
// Users should create a Config with NewConfig() and provide the desired options as arguments.
type Config struct {
	// Verbose enables detailed logging output
	Verbose bool

	// Format is a template string for formatting output, using {path} and {content} placeholders
	Format string

	// IgnoreGitignore bypasses gitignore filtering when true
	IgnoreGitignore bool

	// Internal representation of include/exclude patterns
	includeExts  []string
	excludeExts  []string
	excludeNames []string

	// Original string forms (retained for backward compatibility)
	include         string
	exclude         string
	excludeNamesStr string

	// GitClient is used for git-related operations
	GitClient GitClient
}

// NewConfig creates a new Config with default values and applies the given options.
// By default, Verbose is false and Format uses a sensible default format
// with file path headers and code fences.
func NewConfig(opts ...Option) *Config {
	c := &Config{
		Verbose:   false,
		Format:    "<{path}>\n```\n{content}\n```\n</{path}>\n\n",
		GitClient: NewRealGitClient(),
	}

	// Apply all options
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithVerbose sets the verbose output flag.
func WithVerbose(verbose bool) Option {
	return func(c *Config) {
		c.Verbose = verbose
	}
}

// WithFormat sets the output format template.
func WithFormat(format string) Option {
	return func(c *Config) {
		c.Format = format
	}
}

// WithInclude specifies file extensions to include.
// Extensions can be provided with or without dots (e.g., ".go,.md" or "go,md").
func WithInclude(include string) Option {
	return func(c *Config) {
		c.include = include
		c.includeExts = processExtensions(include)
	}
}

// WithExclude specifies file extensions to exclude.
// Extensions can be provided with or without dots (e.g., ".exe,.bin" or "exe,bin").
func WithExclude(exclude string) Option {
	return func(c *Config) {
		c.exclude = exclude
		c.excludeExts = processExtensions(exclude)
	}
}

// WithExcludeNames specifies file names to exclude.
func WithExcludeNames(excludeNames string) Option {
	return func(c *Config) {
		c.excludeNamesStr = excludeNames
		c.excludeNames = processNames(excludeNames)
	}
}

// WithGitClient sets a custom GitClient implementation.
// This is primarily useful for testing or when you want to provide
// a specialized git client implementation.
func WithGitClient(gitClient GitClient) Option {
	return func(c *Config) {
		c.GitClient = gitClient
	}
}

// WithIgnoreGitignore sets whether to ignore gitignore rules.
func WithIgnoreGitignore(ignoreGitignore bool) Option {
	return func(c *Config) {
		c.IgnoreGitignore = ignoreGitignore
	}
}

// Helper function to process comma-separated extensions
func processExtensions(exts string) []string {
	if exts == "" {
		return nil
	}

	result := []string{}
	for _, ext := range strings.Split(exts, ",") {
		ext = strings.TrimSpace(ext)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		result = append(result, ext)
	}
	return result
}

// Helper function to process comma-separated names
func processNames(names string) []string {
	if names == "" {
		return nil
	}

	result := []string{}
	for _, name := range strings.Split(names, ",") {
		result = append(result, strings.TrimSpace(name))
	}
	return result
}

// ProcessorFunc is a function type that processes a file's content and returns formatted output.
// It receives the file path and raw content and should return the processed content as a string.
// This type is used for custom file processing in internal functions like processFile and processPathWithProcessor.
type ProcessorFunc func(filePath string, content []byte) string

// Logger provides a simple logging interface with different log levels.
// All messages are sent to stderr with appropriate prefixes for their level.
type Logger struct {
	// verbose determines whether Verbose-level messages are displayed
	verbose bool
}

// NewLogger creates a new Logger instance with the specified verbosity setting.
// When verbose is false, calls to the Verbose method will be suppressed.
func NewLogger(verbose bool) *Logger {
	return &Logger{
		verbose: verbose,
	}
}

// Info logs an informational message to stderr.
// These messages are always displayed regardless of the verbose setting.
func (l *Logger) Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// Warn logs a warning message to stderr.
// Warning messages are prefixed with "warning: " and are always displayed.
func (l *Logger) Warn(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// Error logs an error message to stderr.
// Error messages are prefixed with "error: " and are always displayed.
func (l *Logger) Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
}

// Verbose logs a message to stderr only if verbose mode is enabled.
// These messages are useful for detailed progress information.
func (l *Logger) Verbose(format string, args ...interface{}) {
	if l.verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// Stats holds statistics about processed files and content.
// It's returned by file processing functions to provide information
// about the operation results without relying on logging.
type Stats struct {
	// FilesProcessed is the number of files successfully processed
	FilesProcessed int

	// FilesTotal is the total number of candidate files found before filtering
	FilesTotal int

	// Lines is the number of lines in the processed content
	Lines int

	// Chars is the number of characters in the processed content
	Chars int

	// Tokens is an estimated count of tokens in the processed content
	Tokens int
}

// Note: The global gitAvailable variable and its initialization have been replaced
// with a GitClient interface. This allows for better dependency injection and testing.
// See git_client.go for the implementation details.

// ProcessConfig is maintained for backward compatibility.
// It processes the string-based fields in the Config struct and populates
// the corresponding slice fields using the new helper functions.
//
// Note: This method is deprecated. New code should use the functional options pattern
// with NewConfig() and option functions like WithInclude(), WithExclude(), etc.
func (c *Config) ProcessConfig() {
	// Only re-process if the internal slices are nil or empty
	// This ensures we don't overwrite slices already set by option functions
	if c.includeExts == nil && c.include != "" {
		c.includeExts = processExtensions(c.include)
	}
	if c.excludeExts == nil && c.exclude != "" {
		c.excludeExts = processExtensions(c.exclude)
	}
	if c.excludeNames == nil && c.excludeNamesStr != "" {
		c.excludeNames = processNames(c.excludeNamesStr)
	}
}

// isGitIgnored checks if a file is gitignored or hidden (internal helper).
// It delegates the check to the GitClient implementation in the config.
func isGitIgnored(file string, config *Config) bool {
	return config.GitClient.IsGitIgnored(file)
}

// getGitFiles retrieves files from a directory using Git's ls-files command (internal helper)
// It delegates the operation to the GitClient implementation in the config.
func getGitFiles(dir string, config *Config) ([]string, error) {
	files, err := config.GitClient.GetGitFiles(dir)
	if err != nil {
		return nil, err
	}

	// Check if files still exist before returning them
	var existingFiles []string
	for _, filePath := range files {
		if _, err := os.Stat(filePath); err == nil {
			existingFiles = append(existingFiles, filePath)
		}
	}
	return existingFiles, nil
}

// getFilesWithFilepathWalk retrieves files from a directory by walking the filesystem (internal helper)
func getFilesWithFilepathWalk(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasPrefix(info.Name(), ".") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// getFilesFromDir retrieves all files to process from a directory.
// It tries to use Git first and falls back to filepath.Walk if Git is not available
// or the directory is not a Git repository. (internal helper)
func getFilesFromDir(dir string, config *Config) ([]string, error) {
	if config.GitClient.IsAvailable() {
		files, err := getGitFiles(dir, config)
		if err == nil {
			return files, nil
		}
		// If there's an error running git ls-files and it's not "not a git repository", return the error
		if !strings.Contains(err.Error(), "not a git repository") {
			return nil, err
		}
		// Otherwise fall back to filepath.Walk
	}

	// Fallback to walking the directory, excluding hidden files and dirs
	return getFilesWithFilepathWalk(dir)
}

// Constants for binary file detection
const (
	binarySampleSize            = 512 // Number of bytes to sample for binary detection
	binaryNonPrintableThreshold = 0.3 // Threshold ratio of non-printable chars to consider a file binary
)

// isBinaryFile uses heuristics to determine if content is likely binary.
// This is an internal helper that employs two main detection strategies:
//  1. Presence of null bytes (ASCII 0): Any null byte indicates binary content
//  2. High ratio of non-printable characters: If more than 30% of the first 512 bytes
//     are non-printable, non-whitespace characters, the content is considered binary
//
// Note that this heuristic approach:
//   - Only examines up to the first 512 bytes (configurable via binarySampleSize)
//   - May produce false positives for some text files with unusual encoding
//   - May produce false negatives for some binary files that appear text-like
//   - Only considers ASCII control characters and DEL (127) as non-printable
//   - Treats common whitespace characters (\n, \r, \t, space) as printable
func isBinaryFile(content []byte) bool {
	// Check for null bytes, which are common in binary files
	if len(content) > 0 && bytes.IndexByte(content, 0) != -1 {
		return true
	}

	// Check for a high percentage of non-printable, non-whitespace characters
	// which suggest binary content
	nonPrintable := 0
	sampleSize := minInt(len(content), binarySampleSize) // Sample the first bytes
	for i := 0; i < sampleSize; i++ {
		// Check for non-printable characters (ASCII 0-31 except whitespace, and DEL which is 127)
		if (content[i] < 32 && !isWhitespace(content[i])) || content[i] == 127 {
			nonPrintable++
		}
	}

	// If more than the threshold percentage of sampled bytes are non-printable, consider it binary
	return float64(nonPrintable) > float64(sampleSize)*binaryNonPrintableThreshold
}

// isWhitespace checks if a byte is a whitespace character (unexported, internal helper)
func isWhitespace(b byte) bool {
	return b == '\n' || b == '\r' || b == '\t' || b == ' '
}

// minInt returns the minimum of two integers (unexported, internal helper)
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// shouldProcess decides if a file should be processed based on all filters (internal helper)
func shouldProcess(file string, config *Config) bool {
	base := filepath.Base(file)
	ext := strings.ToLower(filepath.Ext(file))

	// Check exclude names filter
	if len(config.excludeNames) > 0 && slices.Contains(config.excludeNames, base) {
		return false
	}

	// Check include extensions filter
	if len(config.includeExts) > 0 {
		included := false
		for _, includeExt := range config.includeExts {
			if ext == includeExt {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}

	// Check exclude extensions filter
	if len(config.excludeExts) > 0 {
		for _, excludeExt := range config.excludeExts {
			if ext == excludeExt {
				return false
			}
		}
	}

	return true
}

// processFile processes a single file with the given processor function and configuration.
// It applies various filters (gitignore, extension/name filters, binary detection) and
// passes the valid file's content to the processor function to generate formatted output.
//
// If the file should be skipped (doesn't exist, is gitignored, doesn't match filters,
// or is binary), an empty string is returned and appropriate messages are logged.
//
// Parameters:
//   - filePath: The path to the file to process
//   - logger: Logger for status and error messages
//   - config: Configuration options controlling filtering
//   - processor: Function to process the file content
//
// Returns a formatted string for valid files or an empty string for skipped files.
func processFile(filePath string, logger *Logger, config *Config, processor ProcessorFunc) string {
	// First check if file exists
	if _, statErr := os.Stat(filePath); statErr != nil {
		if os.IsNotExist(statErr) {
			// Skip without warning if the file simply doesn't exist
			return ""
		}
		// Log warning for other errors
		logger.Warn("stat %s: %v", filePath, statErr)
		return ""
	}

	// Respect gitignore rules unless explicitly bypassed.
	// The IgnoreGitignore flag allows processing files that would normally be excluded
	// by .gitignore rules - useful for documentation files, context gathering, or
	// when users need to process specific files regardless of Git's ignore patterns.
	if isGitIgnored(filePath, config) {
		if config.IgnoreGitignore {
			logger.Verbose("processing gitignored file (bypass enabled): %s", filePath)
		} else {
			logger.Verbose("skipping gitignored file: %s", filePath)
			return ""
		}
	}

	// Check if file should be processed based on filters
	if !shouldProcess(filePath, config) {
		if len(config.excludeNames) > 0 && slices.Contains(config.excludeNames, filepath.Base(filePath)) {
			logger.Verbose("skipping file (in exclude-names list): %s", filePath)
		}
		return ""
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		logger.Warn("cannot read %s: %v", filePath, err)
		return ""
	}

	// Skip binary files
	if isBinaryFile(content) {
		logger.Verbose("skipping binary file: %s", filePath)
		return ""
	}

	// Process the content
	return processor(filePath, content)
}

// processPaths processes multiple file or directory paths according to the configuration.
// It creates a customized processor function that tracks progress and formats output
// using the config's Format template. The function first discovers all files to process
// upfront, then processes them, avoiding redundant directory scans.
//
// Parameters:
//   - paths: List of file or directory paths to process
//   - config: Configuration for file filtering and processing
//   - logger: Logger for status and error messages
//
// Returns:
//   - A string containing the combined formatted content
//   - Stats struct with information about processed files and content
//   - An error if the processing fails, including ErrNoFilesProcessed if paths were provided,
//     files were found (stats.FilesTotal > 0), but no files were processed due to filtering
func processPaths(paths []string, config *Config, logger *Logger) (string, Stats, error) {
	contentBuilder := &strings.Builder{}
	processedFiles := 0

	// Discover all files upfront to avoid redundant directory scans
	var allFiles []string

	// First, discover all files from all paths
	for _, path := range paths {
		logger.Verbose("Processing path: %s", path)

		info, err := os.Stat(path)
		if err != nil {
			logger.Warn("%v", err)
			continue
		}

		if info.IsDir() {
			files, err := getFilesFromDir(path, config)
			if err != nil {
				logger.Warn("Error getting files from directory %s: %v", path, err)
				continue
			}
			allFiles = append(allFiles, files...)
		} else {
			// It's a single file
			allFiles = append(allFiles, path)
		}
	}

	// Store total file count for stats and progress tracking
	totalFiles := len(allFiles)
	logger.Verbose("Found %d total files across all paths", totalFiles)

	// Process all discovered files
	for _, file := range allFiles {
		// Create a processor function that tracks progress
		processor := func(filepath string, fileContent []byte) string {
			processedFiles++
			logger.Verbose("Processing file (%d/%d): %s", processedFiles, totalFiles, filepath)

			// Format the output using the custom format
			output := config.Format
			output = strings.ReplaceAll(output, "{path}", filepath)
			output = strings.ReplaceAll(output, "{content}", string(fileContent))
			return output
		}

		// Process the file directly without rediscovering it
		output := processFile(file, logger, config, processor)
		if output != "" {
			contentBuilder.WriteString(output)
		}
	}

	content := contentBuilder.String()

	// Calculate statistics for the content
	chars, lines, tokens := CalculateStatistics(content)

	// Create and populate Stats struct
	stats := Stats{
		FilesProcessed: processedFiles,
		FilesTotal:     totalFiles,
		Lines:          lines,
		Chars:          chars,
		Tokens:         tokens,
	}

	// Check if paths were provided but no files ended up being processed
	// Only return an error if paths exist but no files were processed due to filtering
	if len(paths) > 0 && stats.FilesProcessed == 0 && stats.FilesTotal > 0 {
		return content, stats, ErrNoFilesProcessed
	}

	return content, stats, nil
}

// WrapInContext wraps the content in top-level context tags.
// This provides consistent formatting for the final output, making it easier
// to identify the boundaries of the collected content.
//
// Parameter:
//   - content: The raw content to wrap
//
// Returns:
//   - The content wrapped with <context> tags
func WrapInContext(content string) string {
	return "<context>\n" + content + "</context>"
}

// estimateTokenCount provides a simple approximation of token count in text.
// This is an internal helper function that uses a basic whitespace-based approach:
//   - Counts transitions from non-whitespace sequences to whitespace
//   - Treats any continuous sequence of non-whitespace characters as one token
//   - Adds a final count if text ends with non-whitespace characters
//
// Note that this method:
//   - Is significantly less sophisticated than actual LLM tokenizers
//   - Doesn't account for subword tokenization used by most modern LLMs
//   - May undercount tokens for punctuation that would be separate tokens in LLMs
//   - May overcount for common words that LLMs represent as single tokens
//   - Is intended for rough estimation purposes only, with accuracy varying
//     by content type and language
func estimateTokenCount(text string) int {
	count := 0
	inToken := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if inToken {
				count++
				inToken = false
			}
		} else {
			inToken = true
		}
	}
	if inToken {
		count++ // Count the last token if text ends with non-whitespace
	}
	return count
}

// CalculateStatistics calculates useful statistics about the content.
// This function analyzes the provided content and returns counts of characters,
// lines, and tokens (words or code-like tokens) it contains.
//
// Parameter:
//   - content: The content to analyze
//
// Returns:
//   - charCount: Total number of characters in the content
//   - lineCount: Total number of lines in the content (based on newlines)
//   - tokenCount: Estimated number of tokens/words in the content
func CalculateStatistics(content string) (charCount, lineCount, tokenCount int) {
	charCount = len(content)
	lineCount = strings.Count(content, "\n") + 1
	tokenCount = estimateTokenCount(content)
	return charCount, lineCount, tokenCount
}

// ProcessProject collects and formats content from files in the specified paths.
// This is the main entry point for the library and the primary function that external
// applications should use. It handles all aspects of file collection, filtering, and
// formatting according to the provided configuration.
//
// If config is nil, default configuration is used. For backward compatibility, the function
// calls ProcessConfig() on the configuration, but this is unnecessary when using the functional
// options pattern.
//
// Parameters:
//   - paths: List of file or directory paths to process
//   - config: Configuration for file filtering and processing (can be nil for defaults)
//
// Returns:
//   - The formatted content wrapped in context tags
//   - Stats struct with information about processed files and content
//   - An error if no paths are provided or if processing fails
func ProcessProject(paths []string, config *Config) (string, Stats, error) {
	if config == nil {
		config = NewConfig()
	}

	// For backward compatibility with existing code
	// This call is unnecessary when using the functional options pattern
	config.ProcessConfig()

	logger := NewLogger(config.Verbose)

	if len(paths) == 0 {
		return "", Stats{}, fmt.Errorf("no paths provided")
	}

	// Process paths
	content, stats, err := processPaths(paths, config, logger)
	if err != nil {
		return "", Stats{}, err
	}

	// Wrap content in context tag
	formattedContent := WrapInContext(content)

	return formattedContent, stats, nil
}

// WriteToFile writes the content to a file at the specified path.
// This is a convenience function for saving the output of ProcessProject
// directly to a file. The file is created with 0644 permissions.
// Parent directories are automatically created if they don't exist.
//
// By default, it will not overwrite existing files unless overwrite is set to true.
// If the output path exists (including a symlink) and overwrite is false, it returns
// ErrFileExists. Creation is exclusive so concurrent writers cannot overwrite it.
//
// Parameters:
//   - content: The content to write to the file
//   - filePath: The path where the file should be created
//   - overwrite: If true, existing files will be overwritten; if false, returns an error when the file exists
//
// Returns:
//   - An error if the file cannot be written (e.g., due to directory creation failure,
//     permissions issues, file already exists with overwrite=false, or other I/O errors)
func WriteToFile(content, filePath string, overwrite bool) error {
	// Create parent directories if they don't exist
	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %q: %w", filePath, err)
	}

	// Enforce overwrite protection when opening, without a check-then-write race.
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(filePath, flags, 0644)
	if err != nil {
		if !overwrite && errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrFileExists, filePath)
		}
		return fmt.Errorf("failed to open file %q: %w", filePath, err)
	}

	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("failed to write to file %q: %w", filePath, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close file %q: %w", filePath, closeErr)
	}
	return nil
}
