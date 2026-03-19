package parser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"springlog/pkg/logentry"
)

// Spring Boot log patterns:
// Classic:  2024-01-15 10:23:45.123 ERROR 12345 --- [main] c.example.MyClass : msg
// No ms:    2024-01-15 10:23:45 ERROR 12345 --- [main] c.example.MyClass : msg
// SB3+MDC:  2024-01-15T10:23:45.123+09:00  INFO 12345 --- [app] [t=abc s=def] c.e.MyClass : msg
// Sleuth:   2024-01-15 10:23:45.123 ERROR 12345 --- [main] [traceId=abc,spanId=def] c.e.MyClass : msg
var springBootPattern = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d{1,3})?(?:[+-]\d{2}:?\d{2}|Z)?)\s+` +
		`(\w+)\s+(\d+)\s+---\s+\[([^\]]*)\]\s+` +
		`(?:\[([^\]]*)\]\s+)?` + // optional MDC/trace bracket (SB3 style)
		`(\S+)\s+:\s+(.*)$`,
)

var stackContinuationPattern = regexp.MustCompile(
	`^(\s+at |\s+\.\.\. \d+ more|Caused by:|^\t)`,
)

// MDC patterns: [traceId=abc123 spanId=def456] or [key=val, key2=val2]
var mdcPattern = regexp.MustCompile(`(\w+)=([^\s,\]]+)`)

// Duration pattern: "completed in 342ms", "took 1.2s", "executed in 500 ms"
var durationPattern = regexp.MustCompile(
	`(?:in|took|elapsed|duration|latency)[:\s]+(\d+(?:\.\d+)?)\s*(ms|s|millis|seconds|milliseconds)`,
)

var timeFormats = []string{
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05,000",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05.000-07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
}

type TextParser struct{}

func NewTextParser() *TextParser { return &TextParser{} }

func (p *TextParser) Parse(ctx context.Context, r io.Reader, source string) (<-chan *logentry.LogEntry, <-chan error) {
	entries := make(chan *logentry.LogEntry, 256)
	errs := make(chan error, 16)

	go func() {
		defer close(entries)
		defer close(errs)

		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		var current *logentry.LogEntry
		var lineNum int64

		flush := func() {
			if current != nil {
				select {
				case entries <- current:
				case <-ctx.Done():
					return
				}
				current = nil
			}
		}

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				flush()
				return
			default:
			}

			lineNum++
			line := scanner.Text()

			if m := springBootPattern.FindStringSubmatch(line); m != nil {
				flush()

				ts := parseTime(m[1])
				pid, _ := strconv.Atoi(m[3])
				mdcRaw := m[5] // optional MDC bracket content
				logger := m[6]
				message := m[7]

				entry := &logentry.LogEntry{
					Timestamp:  ts,
					Level:      logentry.ParseLevel(m[2]),
					PID:        pid,
					Thread:     m[4],
					Logger:     logger,
					Message:    message,
					Raw:        line,
					LineNumber: lineNum,
					SourceFile: source,
				}

				// Parse MDC bracket → traceId, spanId, MDC map
				if mdcRaw != "" {
					entry.MDC = make(map[string]string)
					for _, kv := range mdcPattern.FindAllStringSubmatch(mdcRaw, -1) {
						k, v := kv[1], kv[2]
						entry.MDC[k] = v
						switch strings.ToLower(k) {
						case "traceid", "trace_id", "x-trace-id":
							entry.TraceID = v
						case "spanid", "span_id":
							entry.SpanID = v
						}
					}
				}

				// Extract duration from message
				entry.DurationMs = extractDuration(message)

				current = entry
			} else if current != nil && isStackLine(line) {
				current.StackTrace = append(current.StackTrace, line)
			} else if strings.TrimSpace(line) == "" {
				flush()
			} else if current != nil {
				current.Message += "\n" + line
				current.Raw += "\n" + line
			} else {
				select {
				case entries <- &logentry.LogEntry{
					Level:      logentry.LevelUnknown,
					Raw:        line,
					LineNumber: lineNum,
					SourceFile: source,
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		flush()

		if err := scanner.Err(); err != nil {
			select {
			case errs <- fmt.Errorf("%s: scan error: %w", source, err):
			default:
			}
		}
	}()

	return entries, errs
}

func isStackLine(line string) bool {
	return stackContinuationPattern.MatchString(line)
}

func parseTime(s string) time.Time {
	s = strings.ReplaceAll(s, ",", ".")
	for _, layout := range timeFormats {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func extractDuration(msg string) *float64 {
	m := durationPattern.FindStringSubmatch(strings.ToLower(msg))
	if m == nil {
		return nil
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	unit := m[2]
	switch unit {
	case "s", "seconds":
		val *= 1000
	}
	return &val
}
