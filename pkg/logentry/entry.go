package logentry

import (
	"strings"
	"time"
)

// Level represents a canonical log level normalized from any source format.
type Level int

const (
	LevelUnknown Level = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = [...]string{"UNKNOWN", "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"}

func (l Level) String() string {
	if int(l) < len(levelNames) {
		return levelNames[l]
	}
	return "UNKNOWN"
}

func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRACE":
		return LevelTrace
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelUnknown
	}
}

// LogEntry is the normalized representation of a single log event.
type LogEntry struct {
	Timestamp  time.Time
	Level      Level
	PID        int
	Thread     string
	Logger     string
	Message    string
	StackTrace []string
	Raw        string
	Fields     map[string]any
	LineNumber int64
	SourceFile string
	Project    string

	// 2단계: Trace & MDC
	TraceID string         // Micrometer/Sleuth traceId
	SpanID  string         // spanId
	MDC     map[string]string // Mapped Diagnostic Context (userId, requestId, etc.)

	// 3단계: Latency
	DurationMs *float64 // extracted duration in milliseconds (nil if not present)
}

func (e *LogEntry) IsError() bool      { return e.Level >= LevelError }
func (e *LogEntry) HasStackTrace() bool { return len(e.StackTrace) > 0 }

// MDCValue returns an MDC field value by key (case-insensitive).
func (e *LogEntry) MDCValue(key string) string {
	if e.MDC == nil {
		return ""
	}
	key = strings.ToLower(key)
	for k, v := range e.MDC {
		if strings.ToLower(k) == key {
			return v
		}
	}
	return ""
}
