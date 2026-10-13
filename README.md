# Handoff

Handoff is both a command-line tool and a Go library for collecting and formatting code from multiple files. It's designed to make it easy to share code with AI assistants, documentation generators, or other tools that work with source code.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
  - [As a CLI Tool](#as-a-cli-tool)
  - [As a Library](#as-a-library)
- [Usage](#usage)
  - [Command Line](#command-line)
  - [Library Usage](#library-usage)
- [Output Format](#output-format)
- [Output Statistics](#output-statistics)
- [Git Integration](#git-integration)
- [Development and Contributing](#development-and-contributing)
- [License](#license)

## Features

- **Dual Interface**: Use as a command-line tool or import as a Go library
- **Flexible File Collection**: Process individual files or entire directories
- **Smart Filtering**: Include/exclude files by extension or name
- **Git-Aware**: Respects .gitignore rules to skip irrelevant files, with optional bypass for processing gitignored content
- **Format Customization**: Customize output with templates
- **Multiple Output Options**: Copy to clipboard or write to file
- **Content Statistics**: Get detailed stats about processed content
- **Binary Detection**: Automatically skips binary files
- **Safety Features**: File overwrite protection

## Installation

### Prerequisites
- Go 1.22.3 or higher

### As a CLI Tool

#### Building from Source
```bash
# Clone the repository
git clone https://github.com/phrazzld/handoff.git
cd handoff

# Build the binary
go build

# Optionally, install it to your GOPATH
go install
```

### As a Library

```bash
# Add to your Go project
go get github.com/phrazzld/handoff/lib

# Import in your code
import "github.com/phrazzld/handoff/lib"
```

## Usage

### Command Line

```bash
./handoff [options] [path1] [path2] ...
```

#### Options

- `-verbose`: Enable verbose output
- `-dry-run`: Preview what would be copied without actually copying
- `-output`: Write output to the specified file instead of clipboard (e.g., `HANDOFF.md`)
- `-force`: Allow overwriting existing files when using `-output` flag
- `-include`: Comma-separated list of file extensions to include (e.g., `.txt,.go`)
- `-exclude`: Comma-separated list of file extensions to exclude (e.g., `.exe,.bin`)
- `-exclude-names`: Comma-separated list of file names to exclude (e.g., `package-lock.json,yarn.lock`)
- `-ignore-gitignore`: Process files even if they are gitignored (bypasses .gitignore rules; default: false)
- `-format`: Custom format for output. Use `{path}` and `{content}` as placeholders

#### Examples

```bash
# Copy all files in the current directory
./handoff .

# Copy only .go files from the src directory
./handoff -include=.go src/

# Copy specific files
./handoff main.go utils.go config.go

# Use a custom format
./handoff -format="File: {path}\n```go\n{content}\n```\n\n" .

# Write output to a file instead of clipboard
./handoff -output=HANDOFF.md .

# Write output to a file, overwriting if it exists
./handoff -output=HANDOFF.md -force .

# Preview content that would be written to file
./handoff -output=HANDOFF.md -dry-run .

# Process files including those that are gitignored
./handoff -ignore-gitignore .

# Process specific gitignored documentation files
./handoff -ignore-gitignore docs/internal-notes.md README-private.md

# Combine with verbose output to see which gitignored files are being processed
./handoff -ignore-gitignore -verbose .
```

## Library Usage

Handoff's core functionality is available as a library for integration with your Go applications:

```go
package main

import (
	"fmt"
	"github.com/phrazzld/handoff/lib"
)

func main() {
	// Create a configuration with functional options
	config := lib.NewConfig(
		lib.WithVerbose(true),                               // Enable verbose logging
		lib.WithInclude(".go,.md,.txt"),                     // Only include specific file types
		lib.WithExclude(".exe,.bin,.jpg,.png"),              // Exclude binary and image files
		lib.WithExcludeNames("node_modules,package-lock.json"), // Skip specific files/directories
		lib.WithIgnoreGitignore(true),                       // Process files even if gitignored
		lib.WithFormat("## {path}\n```\n{content}\n```\n\n"), // Custom output format
	)
	
	// Process project files (the main API entry point)
	content, stats, err := lib.ProcessProject([]string{"./my-project"}, config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Use the content - write to a file with overwrite protection
	if err := lib.WriteToFile(content, "output.md", true); err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
	}
	
	// Display statistics returned from ProcessProject
	fmt.Printf("Processed %d/%d files\n", stats.FilesProcessed, stats.FilesTotal)
	fmt.Printf("Content stats: %d lines, %d chars, ~%d tokens\n", 
		stats.Lines, stats.Chars, stats.Tokens)
		
	// You can also calculate statistics for any content directly
	chars, lines, tokens := lib.CalculateStatistics("Some content to analyze")
	fmt.Printf("Analysis: %d chars, %d lines, ~%d tokens\n", chars, lines, tokens)
	
	// Wrap any content in context tags if needed
	wrappedContent := lib.WrapInContext("Content to be wrapped in context tags")
	fmt.Println(wrappedContent)
}
```

For more details on the library API, see the [lib/README.md](lib/README.md) documentation.

### Output Format

The copied content will be formatted as:

````
<context>
<path/to/file.txt>
```
content of file.txt
```
</path/to/file.txt>

<path/to/another/file.go>
```
content of file.go
```
</path/to/another/file.go>
</context>
````

You can customize this format using the `-format` flag with `{path}` and `{content}` placeholders.

### Output Statistics

After processing files, Handoff displays useful statistics about the copied content:

```
Handoff complete:
- Files: 3
- Lines: 45
- Characters: 1024
- Estimated tokens: 256
```

These statistics are particularly helpful for:
- Understanding how much content you're sharing
- Estimating LLM token usage when pasting into AI tools
- Monitoring the size of your clipboard or file content

### Output Mode Precedence

When multiple output options are specified, Handoff follows this precedence:
1. `-dry-run`: Highest priority - outputs to screen only, no clipboard/file modifications
2. `-output`: Medium priority - writes to the specified file
3. Clipboard: Default behavior - copies to clipboard when no other output option is specified

### File Overwrite Protection

When using the `-output` flag, Handoff includes built-in protection against accidental file overwrites:
- If the specified output file already exists, Handoff will refuse to overwrite it
- This protection includes existing symlinks and concurrent writes to the same output path
- To allow overwriting an existing file, use the `-force` flag
- This ensures you don't accidentally lose content in existing files

## Git Integration

Handoff provides flexible Git-aware file processing:

### Default Behavior
- In Git repositories, files ignored by Git (via `.gitignore`) will not be included
- In non-Git directories, hidden files (starting with `.`) will be skipped
- This ensures that binary files, build artifacts, and other irrelevant files are not copied

### Bypassing Gitignore Rules
Use the `-ignore-gitignore` flag when you need to process files that would normally be excluded:
- Useful for documentation files, configuration templates, or context files that are intentionally gitignored
- Common scenarios include processing `glance.md` files, internal notes, or draft documentation
- When combined with `-verbose`, you'll see explicit confirmation of which gitignored files are being processed

## Development and Contributing

### Project Structure

- **`/lib`**: The library package (importable as `github.com/phrazzld/handoff/lib`)
- **Root directory**: CLI tool implementation

### Versioning

This project follows Semantic Versioning. The current version is v0.1.0.

For more details on the versioning strategy, see [VERSIONING.md](VERSIONING.md).

### Testing

```bash
# Run all tests
go test ./...

# Run library tests with coverage
go test -coverprofile=coverage.out ./lib/...
go tool cover -func=coverage.out
```

The library package maintains a minimum test coverage threshold of 85%, enforced via GitHub Actions.

### Contributions

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
