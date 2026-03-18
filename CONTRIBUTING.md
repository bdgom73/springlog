# Contributing to springlog

Thank you for your interest in contributing!

## Development Setup

```bash
git clone https://github.com/bdgom73/springlog.git
cd springlog
go mod download
go build ./...
go test ./...
```

## Project Structure

```
cmd/springlog/     # Binary entrypoint
cli/               # Cobra commands (analyze, stats, tail)
internal/
  detector/        # Format auto-detection
  parser/          # Text and JSON parsers
  filter/          # Filter chain (level, time, keyword)
  aggregator/      # Statistics aggregation
  tail/            # Real-time file watching
  renderer/        # Output renderers (table, json, text)
pkg/logentry/      # Shared LogEntry data model
testdata/          # Sample log files for testing
```

## Adding a New Log Format

1. Implement `parser.Parser` interface in `internal/parser/`
2. Add detection logic in `internal/detector/detector.go`
3. Register in `cli/analyze.go` and `cli/stats.go`
4. Add sample files to `testdata/`

## Testing

```bash
# Run all tests
go test ./...

# Run with race detector
go test ./... -race

# Run specific package
go test ./internal/parser/ -v
```

## Pull Request Guidelines

- Keep PRs focused — one feature or fix per PR
- Add tests for new functionality
- Update README if adding new flags or commands
- Ensure `go vet ./...` passes
