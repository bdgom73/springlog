# springlog

> Spring Boot log analyzer CLI — filter, search, and aggregate log files across multiple projects.

[![CI](https://github.com/bdgom73/springlog/actions/workflows/ci.yml/badge.svg)](https://github.com/bdgom73/springlog/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bdgom73/springlog)](https://goreportcard.com/report/github.com/bdgom73/springlog)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bdgom73/springlog)](https://github.com/bdgom73/springlog/releases/latest)

**한국어 문서:** [docs/README-KO.md](docs/README-KO.md)

---

## Features

- **Auto-detects log format** — text (log4j/slf4j pattern) or JSON, by content not extension
- **Multi-project support** — analyze all projects at once with `--all-projects`
- **Filter** by log level, time range, keyword or regex
- **Statistics** — level breakdown, exception groups, error spike detection, latency percentiles
- **Exception analysis** — grouped by type with stack trace preview
- **Trace tracking** — follow a full request across services by trace ID
- **Real-time tail** — watch live logs with log rotation handling
- **Interactive dashboard** — TUI dashboard with live filtering
- **Multiple output formats** — colored table, JSON (jq-compatible), plain text
- **Zero installation** for users — single binary, no runtime required

---

## Installation

### Download binary (recommended)

Download the latest binary for your OS from [Releases](https://github.com/bdgom73/springlog/releases/latest):

| OS | File |
|----|------|
| Windows (64bit) | `springlog-vX.X.X-windows-amd64.exe` |
| macOS (Intel) | `springlog-vX.X.X-darwin-amd64` |
| macOS (Apple Silicon) | `springlog-vX.X.X-darwin-arm64` |
| Linux (64bit) | `springlog-vX.X.X-linux-amd64` |
| Linux (ARM) | `springlog-vX.X.X-linux-arm64` |

#### Windows

> ⚠️ This is a CLI tool. **Do not double-click the `.exe`** — the window will close immediately. Run it from PowerShell or CMD.

```powershell
# Rename the downloaded file for convenience
Rename-Item springlog-v0.1.0-windows-amd64.exe springlog.exe

# Run from the same directory
.\springlog.exe --help

# Or move to a folder in PATH (e.g. C:\tools) and run from anywhere
springlog --help
```

If Windows SmartScreen blocks the file: click **"More info"** → **"Run anyway"**.

#### macOS / Linux

```bash
# Make executable
chmod +x springlog-v0.1.0-darwin-arm64

# Move to PATH
sudo mv springlog-v0.1.0-darwin-arm64 /usr/local/bin/springlog

springlog --help
```

If macOS shows "cannot be opened because the developer cannot be verified":

```bash
xattr -d com.apple.quarantine springlog
```

### Build from source

```bash
git clone https://github.com/bdgom73/springlog.git
cd springlog
go build -o springlog ./cmd/springlog
```

---

## Log Directory Structure

```
logs/
├── project-a/
│   ├── app.log
│   ├── app.log.2024-01-14
│   └── app.log.2024-01-15
├── project-b/
│   └── app.log
└── project-c/
    └── app.json        # JSON format also supported
```

> File extension does not matter — format is detected from content.

---

## Usage

### analyze — Filter and display log entries

```bash
# Single project, ERROR and above
springlog analyze ./logs/project-a/ -l ERROR

# All projects at once
springlog analyze ./logs/ --all-projects -l ERROR

# Last 24 hours, keyword search
springlog analyze ./logs/project-a/ --from -24h -s "NullPointerException"

# Last 7 days, specific project, JSON output
springlog analyze ./logs/ --all-projects -p project-a --from -7d -o json | jq .

# Regex search
springlog analyze ./logs/ -s "timeout after \d+ms"

# Specific time window
springlog analyze ./logs/ --from "2024-01-15 09:00:00" --to "2024-01-15 10:00:00"
```

### stats — Summary report

```bash
# Statistics for a single project
springlog stats ./logs/project-a/

# All projects, top 20 error groups
springlog stats ./logs/ --all-projects --top-errors 20

# Errors in the last 7 days with 6-hour histogram buckets
springlog stats ./logs/ -l ERROR --from -7d --bucket-size 6h
```

### tail — Real-time monitoring

```bash
# Watch a log file live
springlog tail ./logs/project-a/app.log

# Watch with WARN and above filter
springlog tail ./logs/project-a/app.log -l WARN

# Watch for specific exceptions
springlog tail ./logs/project-a/app.log -s "Exception"
```

### exceptions — Exception analysis

```bash
# Exception type statistics
springlog exceptions ./logs/project-a/

# Show full stack traces
springlog exceptions ./logs/project-a/ --show-stack

# Only exceptions occurring 5+ times
springlog exceptions ./logs/ --all-projects --min-count 5
```

### trace — Trace ID request tracking

```bash
# Follow a full request across services (Micrometer / Sleuth)
springlog trace ./logs/ --trace-id 4bf92f3577b34da6a3ce929d0e0e4736
```

### dashboard — Interactive TUI dashboard

```bash
# Launch dashboard for a single project
springlog dashboard ./logs/project-a/

# All projects
springlog dashboard ./logs/ --all-projects
```

**Dashboard keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `/` | Type to search by keyword |
| `L` | Cycle log level filter |
| `T` | Cycle time range preset |
| `P` | Cycle project filter |
| `Esc` | Reset all filters |
| `Tab` / `←` `→` | Switch panels |
| `↑` `↓` | Scroll |
| `R` | Reload from disk |
| `Q` | Quit |

---

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `table` | Output format: `table`, `json`, `text` |
| `--level` | `-l` | — | Minimum level: `TRACE` `DEBUG` `INFO` `WARN` `ERROR` `FATAL` |
| `--from` | | — | Start time: `-1h`, `-7d`, `yesterday`, `today`, RFC3339 |
| `--to` | | — | End time (same formats) |
| `--search` | `-s` | — | Keyword or regex in message |
| `--search-fields` | | `message` | Fields to search: `message`, `logger`, `thread`, `raw` |
| `--project` | `-p` | — | Filter by project name(s) |
| `--trace-id` | | — | Filter by trace ID |
| `--mdc` | | — | Filter by MDC field (e.g. `--mdc userId=1234`) |
| `--no-color` | | `false` | Disable colored output |

---

## Supported Log Formats

### Spring Boot text (log4j/slf4j)

```
2024-01-15 10:23:45.123 ERROR 12345 --- [http-nio-8080-exec-1] c.example.MyClass : Something failed
java.lang.NullPointerException: null
    at c.example.MyClass.method(MyClass.java:42)
    ... 10 more
```

### JSON (Logback structured logging)

```json
{"@timestamp":"2024-01-15T10:23:45.123Z","level":"ERROR","logger":"c.example.MyClass","message":"Something failed"}
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

1. Open an issue first to discuss the change
2. Fork the repository
3. Create a feature branch: `git checkout -b feat/my-feature`
4. Commit using Conventional Commits: `git commit -m "feat: add my feature"`
5. Open a Pull Request — PR title must follow Conventional Commits format

---

## License

[MIT](LICENSE) © 2026 BDGOM73
