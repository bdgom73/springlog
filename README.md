# springlog

> Spring Boot log analyzer CLI — filter, search, and aggregate log files across multiple projects.

[![CI](https://github.com/bdgom73/springlog/actions/workflows/ci.yml/badge.svg)](https://github.com/bdgom73/springlog/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bdgom73/springlog)](https://goreportcard.com/report/github.com/bdgom73/springlog)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bdgom73/springlog)](https://github.com/bdgom73/springlog/releases/latest)

---

## Features

- **Auto-detects log format** — text (log4j/slf4j pattern) or JSON, by content not extension
- **Multi-project support** — analyze all projects at once with `--all-projects`
- **Filter** by log level, time range, keyword or regex
- **Statistics** — level breakdown, top error groups, time distribution histogram
- **Real-time tail** — watch live logs with rotation handling
- **Multiple output formats** — colored table, JSON (jq-compatible), plain text
- **Zero installation** for users — single binary, no runtime required

---

## Installation

### Download binary (recommended)

Download the latest binary for your OS from [Releases](https://github.com/bdgom73/springlog/releases/latest):

| OS | File |
|----|------|
| Windows | `springlog-vX.X.X-windows-amd64.exe` |
| macOS (Intel) | `springlog-vX.X.X-darwin-amd64` |
| macOS (Apple Silicon) | `springlog-vX.X.X-darwin-arm64` |
| Linux | `springlog-vX.X.X-linux-amd64` |

### Build from source

```bash
git clone https://github.com/bdgom73/springlog.git
cd springlog
go build -o springlog ./cmd/springlog
```

---

## Log Directory Structure

springlog expects logs organized by project:

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
# Watch a log file
springlog tail ./logs/project-a/app.log

# Watch with WARN and above filter
springlog tail ./logs/project-a/app.log -l WARN

# Watch for specific exceptions
springlog tail ./logs/project-a/app.log -s "Exception"
```

---

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `table` | Output format: `table`, `json`, `text` |
| `--level` | `-l` | — | Minimum level: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL` |
| `--from` | | — | Start time: `-1h`, `-7d`, `yesterday`, `today`, RFC3339 |
| `--to` | | — | End time (same formats) |
| `--search` | `-s` | — | Keyword or regex in message |
| `--search-fields` | | `message` | Fields to search: `message`, `logger`, `thread`, `raw` |
| `--project` | `-p` | — | Filter by project name(s) |
| `--no-color` | | `false` | Disable colored output |

---

## Supported Log Formats

### Spring Boot text (log4j/slf4j)

```
2024-01-15 10:23:45.123 ERROR 12345 --- [main] c.example.MyClass : Something failed
java.lang.NullPointerException: message
    at c.example.MyClass.method(MyClass.java:42)
    ... 10 more
```

### JSON (Logback, structured logging)

```json
{"@timestamp":"2024-01-15T10:23:45.123Z","level":"ERROR","logger":"c.example.MyClass","message":"Something failed"}
```

JSON field mapping is configurable for custom schemas.

---

## Output Examples

### Table (default)

```
2024-01-15 10:05:44.771 ERROR [project-a] c.e.s.PaymentService : Payment processing failed
2024-01-15 10:05:44.990 ERROR [project-a] c.e.c.OrderController : Unhandled exception

6 entries matched.
```

### Stats

```
=== Log Analysis Summary ===
Total entries : 51
Time range    : 2024-01-15 07:55:00 → 2024-01-15 16:44:55

--- By Level ---
ERROR    12    23.5%
WARN      9    17.6%
INFO     28    54.9%

--- Top Errors ---
#1  [2x] Payment processing failed ...
#2  [1x] Cannot acquire database connection ...

--- Time Distribution ---
  01-15 10:00 │████████████████│ 5 (2 err)
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

1. Fork the repo
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Commit: `git commit -m "feat: add my feature"`
4. Push and open a Pull Request

---

## License

[MIT](LICENSE)
