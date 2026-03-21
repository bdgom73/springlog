package parser

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"springlog/pkg/logentry"
)

type FieldMap struct {
	Timestamp []string
	Level     []string
	Logger    []string
	Message   []string
	Thread    []string
	PID       []string
	TraceID   []string
	SpanID    []string
}

func DefaultFieldMap() FieldMap {
	return FieldMap{
		Timestamp: []string{"@timestamp", "timestamp", "time", "datetime"},
		Level:     []string{"level", "severity", "log.level", "loglevel"},
		Logger:    []string{"logger", "logger_name", "loggerName", "class"},
		Message:   []string{"message", "msg"},
		Thread:    []string{"thread", "thread_name", "threadName"},
		PID:       []string{"pid", "process.pid"},
		TraceID:   []string{"traceId", "trace_id", "traceid", "X-B3-TraceId", "x-trace-id"},
		SpanID:    []string{"spanId", "span_id", "spanid", "X-B3-SpanId"},
	}
}

type JSONParser struct {
	FieldMap FieldMap
}

func NewJSONParser() *JSONParser {
	return &JSONParser{FieldMap: DefaultFieldMap()}
}

func (p *JSONParser) Parse(ctx context.Context, r io.Reader, source string) (<-chan *logentry.LogEntry, <-chan error) {
	entries := make(chan *logentry.LogEntry, 256)
	errs := make(chan error, 16)

	go func() {
		defer close(entries)
		defer close(errs)

		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		var lineNum int64

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			lineNum++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				select {
				case errs <- fmt.Errorf("%s:%d: json parse error: %w", source, lineNum, err):
				default:
				}
				continue
			}

			entry := p.mapEntry(raw, line, source, lineNum)
			select {
			case entries <- entry:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case errs <- fmt.Errorf("%s: scan error: %w", source, err):
			default:
			}
		}
	}()

	return entries, errs
}

func (p *JSONParser) mapEntry(raw map[string]any, line, source string, lineNum int64) *logentry.LogEntry {
	entry := &logentry.LogEntry{
		Raw:        line,
		LineNumber: lineNum,
		SourceFile: source,
		Fields:     make(map[string]any),
		MDC:        make(map[string]string),
	}

	knownKeys := map[string]bool{}

	if ts := p.getString(raw, p.FieldMap.Timestamp); ts != "" {
		entry.Timestamp = parseJSONTime(ts)
		markKnown(knownKeys, p.FieldMap.Timestamp)
	} else if t, ok := parseInstantField(raw); ok {
		// Log4j2 JSON Layout: {"instant":{"epochSecond":1234567890,"nanoOfSecond":123456789}}
		entry.Timestamp = t
		knownKeys["instant"] = true
	}
	if lvl := p.getString(raw, p.FieldMap.Level); lvl != "" {
		entry.Level = logentry.ParseLevel(lvl)
		markKnown(knownKeys, p.FieldMap.Level)
	}
	if logger := p.getString(raw, p.FieldMap.Logger); logger != "" {
		entry.Logger = logger
		markKnown(knownKeys, p.FieldMap.Logger)
	}
	if msg := p.getString(raw, p.FieldMap.Message); msg != "" {
		entry.Message = msg
		markKnown(knownKeys, p.FieldMap.Message)
	}
	if thread := p.getString(raw, p.FieldMap.Thread); thread != "" {
		entry.Thread = thread
		markKnown(knownKeys, p.FieldMap.Thread)
	} else {
		// Some JSON layouts use numeric threadId — try as number fallback
		for _, key := range p.FieldMap.Thread {
			if v, ok := raw[key]; ok {
				entry.Thread = fmt.Sprintf("%v", v)
				knownKeys[key] = true
				break
			}
		}
	}

	// Trace & Span
	if traceID := p.getString(raw, p.FieldMap.TraceID); traceID != "" {
		entry.TraceID = traceID
		markKnown(knownKeys, p.FieldMap.TraceID)
	}
	if spanID := p.getString(raw, p.FieldMap.SpanID); spanID != "" {
		entry.SpanID = spanID
		markKnown(knownKeys, p.FieldMap.SpanID)
	}

	// MDC fields
	if mdc, ok := raw["mdc"]; ok {
		if mdcMap, ok := mdc.(map[string]any); ok {
			for k, v := range mdcMap {
				entry.MDC[k] = fmt.Sprintf("%v", v)
			}
			knownKeys["mdc"] = true
		}
	}

	// Stack trace
	for _, key := range []string{"stack_trace", "stackTrace", "exception", "error.stack_trace"} {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				entry.StackTrace = strings.Split(s, "\n")
				knownKeys[key] = true
				break
			}
		}
	}

	// Duration extraction from message
	entry.DurationMs = extractDuration(entry.Message)

	// Check for numeric duration fields
	for _, key := range []string{"duration", "durationMs", "duration_ms", "elapsed", "latency", "responseTime"} {
		if v, ok := raw[key]; ok {
			if ms, ok := toFloat64(v); ok {
				entry.DurationMs = &ms
				knownKeys[key] = true
				break
			}
		}
	}

	// Remaining fields
	for k, v := range raw {
		if !knownKeys[k] {
			entry.Fields[k] = v
			// Any string field could be MDC
			if s, ok := v.(string); ok {
				entry.MDC[k] = s
			}
		}
	}

	return entry
}

func (p *JSONParser) getString(raw map[string]any, keys []string) string {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func markKnown(m map[string]bool, keys []string) {
	for _, k := range keys {
		m[k] = true
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

var jsonTimeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05.000",
	"2006-01-02 15:04:05.000",
}

func parseJSONTime(s string) time.Time {
	for _, layout := range jsonTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseInstantField handles Log4j2 JSON Layout instant objects:
// {"instant":{"epochSecond":1234567890,"nanoOfSecond":123456789}}
func parseInstantField(raw map[string]any) (time.Time, bool) {
	inst, ok := raw["instant"]
	if !ok {
		return time.Time{}, false
	}
	instMap, ok := inst.(map[string]any)
	if !ok {
		return time.Time{}, false
	}
	epochSec, ok := toFloat64(instMap["epochSecond"])
	if !ok {
		return time.Time{}, false
	}
	var nanos int64
	if n, ok := toFloat64(instMap["nanoOfSecond"]); ok {
		nanos = int64(n)
	}
	return time.Unix(int64(epochSec), nanos), true
}
