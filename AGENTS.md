# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

LeakTK is a toolkit for leak detection, mitigation, and prevention. It wraps Betterleaks (a fork of Gitleaks) to scan various sources for secrets and sensitive data.

The tool operates in two modes:
- **scan**: Ad-hoc scanning with human-readable or structured output
- **listen**: Long-running server mode that reads JSONL requests from stdin and writes JSONL responses to stdout (logs go to stderr)

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
- Commands defined in `cmd/cmd.go`: scan, listen, analyze, login, logout, version
  - **scan**: Ad-hoc scanning of resources
  - **listen**: Long-running server mode processing JSONL requests
  - **analyze**: Re-analyze scan/listen output through Rego policies (post-scan analysis)
  - **login/logout**: Manage pattern server authentication
  - **version**: Display version information

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

Each Result has:
- Core fields: `ID`, `Kind`, `Secret`, `Match`, `Context`, `Entropy`, `Date`, `Rule`, `Contact`, `Location`
- `Notes`: `map[string]string` - Arbitrary key-value metadata about the result
- `Analysis`: `map[string]any` - Analysis data populated by Rego policies (ML predictions, validation status, etc.)
- `Valid`: `*bool` - Validation status set by Rego policies:
  - `nil`: Not yet validated (unknown)
  - `true`: Confirmed valid secret
  - `false`: Confirmed false positive

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

### Analyst Architecture (pkg/analyst)
The analyst provides post-scan analysis through OPA Rego policies. It can:
- **Filter results**: Remove findings based on policy rules
- **Enrich results**: Add analysis metadata, validation status
- **Validate results**: Mark findings as valid/invalid via API calls

Key components:
- `analyst.go`: Core Analyst type that orchestrates Rego policy evaluation

**Integration with Scanner:**
- Scanner optionally runs analyst after each scan (via `scanner.enable_analysis` config)
- Analyst.Analyze() takes a Response and returns enriched Response
- Analysis happens in-process before results are queued

**Standalone Analysis:**
The `leaktk analyze` command re-analyzes scan output:
```bash
leaktk scan <resource> | leaktk analyze        # Analyze scan output
leaktk listen < requests.jsonl | leaktk analyze  # Analyze listen output
```

This allows updating analysis post-scan when policies or models change.

### Patterns Architecture (pkg/patterns)
Patterns manages fetching, caching, and updating configurations from a pattern server:
- **Gitleaks patterns**: Betterleaks/Gitleaks TOML config for secret detection rules
- **LeakTK patterns**: Combined JSON with ML models + OPA Rego policies

Pattern management features:
- **Autofetch**: Automatically fetch updated patterns from server
- **Caching**: Local file cache with configurable expiry/refresh times
- **Hash-based updates**: Only recompile/reload when server content changes
- **Versioning**: Patterns versioned independently (e.g., gitleaks/8.27.0, leaktk/1)

Pattern storage: `~/.cache/leaktk/scanner/patterns/{provider}/{version}`

**LeakTK Pattern Format (version 1):**
Single JSON file containing:
```json
{
  "models": [
    {
      "kind": "LogisticRegression",
      "coefficients": {...},
      "keywords": [...],
      "stopwords": [...]
    }
  ],
  "opa_policy": "package leaktk.analyst\n..."
}
```

The Rego policy is compiled at load time and made available to the analyst. Policies query with:
`data.leaktk.analyst.analyzed_response`

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
