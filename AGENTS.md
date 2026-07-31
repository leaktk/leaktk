# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

LeakTK is a toolkit for leak detection, mitigation, and prevention. It wraps Betterleaks (a fork of Gitleaks) to scan various sources for secrets and sensitive data.

The tool operates in three modes:
- **scan**: Ad-hoc scanning with human-readable or structured output
- **listen**: Long-running server mode that reads JSONL requests from stdin and writes JSONL responses to stdout (logs go to stderr)
- **collect**: Collects facts about configured sources and streams them to stdout as CSV

## Development Philosophy

When working on this codebase, approach development as a senior engineer who values pragmatism and clarity:

- **Simplicity First**: Write simple, solid code that solves the problem at hand. Prefer straightforward solutions over clever ones.
- **Data-Oriented Design**: Follow data-oriented design principles. Let data structures and their transformations guide your architecture.
- **Test Your Work**: Write tests for your code. Tests document behavior and prevent regressions.
- **Avoid Premature Abstraction**: Don't add abstractions until you have concrete evidence they're needed. Three instances of similar code don't necessarily need a shared abstraction.
- **Well-Architected, Not Over-Architected**: Design clean interfaces and logical module boundaries, but don't build speculative flexibility for hypothetical future requirements.
- **Let the Linter Guide You**: Run `make lint` regularly to catch issues and guide your style. The linter enforces team standards.
- **Document User-Facing Changes**: Update documentation when making changes that affect users. Keep docs in sync with reality.
- **Iterate on Refactoring**: When you finish implementing a feature, review it for refactoring opportunities. Clean up duplication, awkward interfaces, and unclear naming. Then review again. Repeat until the code feels right and there's nothing obvious left to improve.
- **Comments With Purpose**: Add comments where the linter requires them and where they genuinely aid understanding. But strive to write code that's clear enough through good naming and simple logic that comments aren't always necessary.

## Development Commands

### Building
```bash
make build          # Build the leaktk binary (CGO_ENABLED=0)
make all            # Build binary + shell completions
```

### Testing
```bash
make test           # Full test suite (runs format, vet, lint, then tests with race detector)
make failfast       # Run tests and stop at first failure
go test ./pkg/scanner -run TestScanGit  # Run a specific test
```

### Linting & Formatting
```bash
make lint           # Run vet and golangci-lint
make format         # Run goimports and go fmt
make import         # Run goimports with local module prefix and go mod tidy
```

### Other
```bash
make clean          # Clean build artifacts (git clean -dfX)
```

## Architecture

### Entry Point & CLI
- `main.go` → `cmd/cmd.go`: Entry point delegates to cobra-based CLI
- CLI framework: Uses spf13/cobra for command parsing
- Commands defined in `cmd/cmd.go`: scan, listen, collect, login, logout, version

### Scanner Architecture (pkg/scanner)
The scanner uses a worker pool pattern with priority queues:
- **Request Queue**: Incoming scan requests (priority-based)
- **Response Queue**: Outgoing scan results (priority-based)
- **Workers**: Configurable number of goroutines processing requests concurrently

Key scanner components:
- `scanner.go`: Core Scanner type with worker pool
- `patterns.go`: Manages Betterleaks config files (fetch, cache, expiry)
- `betterleaks/`: Adapters for different scan types (git, files, JSON, URL, containers)

### Request/Response Protocol (pkg/proto)
The protocol supports multiple request kinds:
- `GitRepo`: Scan git repositories (local or remote)
- `Files`: Scan local filesystem paths
- `JSONData`: Scan JSON data for URLs to fetch and scan
- `Text`: Scan arbitrary text
- `URL`: Fetch and scan a URL
- `ContainerImage`: Scan container images

Each Request has:
- `ID`: Unique identifier for tracking
- `Kind`: Type of scan (enum)
- `Resource`: What to scan (URL, path, data, etc.)
- `Opts`: Options like branch, depth, priority, proxy, etc.

Responses include Results (array of findings) or Error.

### Collector Architecture (internal/collector)
The collector gathers facts about configured sources and streams them as CSV to stdout.

Usage: `leaktk collect <source-id>...`

**Facts** are structured as CSV rows with a header row using snake case field names. Facts are modeled after RDF triples: each has a subject (eid), predicate (kind), and object (value), plus a timestamp (ts). Entity IDs group related facts about the same entity (e.g. a user). Facts are streamed one per row so they can be produced incrementally.

Entity ID 0 is reserved for metadata. Before any real facts are emitted, the collector yields rows with `eid=0` that map each numeric `kind` value to its human-readable name (e.g. `0 → "ID"`, `1 → "Active"`). This lets subsequent rows use the compact numeric kind without repeating the string in every row. See `FactKindNames` in `facts.go` for the canonical mapping.

Fact fields:

- `eid`: Groups facts about the same entity (uint32; 0 = metadata/mapping rows, 1+ = real entities)
- `kind`: The type of fact as a numeric ID (maps to names like ID, Active, EmailAddress, Name, SourceID, URL, Username)
- `value`: The fact's value (for eid=0 rows, this is the kind's string name)
- `ts`: Unix timestamp of when the fact was collected

Key components:
- `collector.go`: Core Collector type, dispatches to source-specific collectors
- `facts.go`: Fact type, FactKind enum, and yield helpers
- `atlassian.go`: Atlassian Cloud Admin API integration (directory listing, user search with pagination)

**Sources** are configured in the TOML config under `[[sources]]`. Each source has a `kind`, `id`, and kind-specific fields (auth credentials, org IDs, base URLs). Source IDs are passed as arguments to the `collect` command. The sources concept is shared with other features (monitor, authenticated scans).

Supported source kinds:
- `AtlassianCloudAdmin`: Uses bearer token auth against the Atlassian Admin API to enumerate org directories and users
- `AtlassianCloudJira`: Uses basic auth (not yet implemented in collector)

Source config types live in `pkg/config/sources.go`, `pkg/config/auth.go`, and `pkg/config/source_kind.go`.

### Configuration (pkg/config)
Configuration is loaded from TOML files with this precedence:
1. `--config` flag path
2. `LEAKTK_CONFIG_PATH` env var
3. `~/.config/leaktk/config.toml` (XDG)
4. `/etc/leaktk/config.toml` (system)
5. Default config (hardcoded)

Key config sections:
- `scanner.patterns`: Pattern autofetch, expiry, server URL
- `scanner.scan_workers`: Number of concurrent workers
- `scanner.allow_local`: Whether to allow local filesystem scans
- `scanner.scan_timeout`: Per-scan timeout in seconds
- `[[sources]]`: Source definitions for the collector (kind, id, auth, and kind-specific fields)

### Git Operations
- Uses `git` CLI commands directly (not libgit2)
- Platform-specific command builders in `scanner/git_command_*.go`
- Clones are bare/mirror clones to `.cache/leaktk/scanner/clones/`
- Uses git worktrees to checkout `.gitleaks*` config files from repos
- Respects `.gitleaks.toml`, `.gitleaksignore`, `.gitleaksbaseline` in scanned repos

## Code Conventions

From CONTRIBUTING.md:
- Avoid extra libraries when the feature is small to implement from scratch
- Format code with `make format` before committing
- Use proper variable and function names (see style guide)
- Sort and group imports: built-in, external, internal (use `make import`)

## Important Notes

- **Pre-1.0 API**: The CLI input/output format may change between releases
- **Pattern Server**: Can fetch updated patterns from a remote server (default: GitHub patterns repo)
- **Listen Mode**: In listen mode, logger format switches to JSON automatically
- **Local Scans**: Can be disabled via config (`scanner.allow_local = false`) for security
