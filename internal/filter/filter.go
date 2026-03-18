package filter

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"springlog/pkg/logentry"
)

// Filter is a predicate over a single LogEntry.
type Filter interface {
	Match(e *logentry.LogEntry) bool
	Name() string
}

// Chain applies multiple filters in sequence (AND semantics).
type Chain []Filter

func (c Chain) Match(e *logentry.LogEntry) bool {
	for _, f := range c {
		if !f.Match(e) {
			return false
		}
	}
	return true
}

// --- LevelFilter ---

// LevelFilter passes entries at or above Min level.
type LevelFilter struct {
	Min logentry.Level
}

func NewLevelFilter(min logentry.Level) *LevelFilter {
	return &LevelFilter{Min: min}
}

func (f *LevelFilter) Match(e *logentry.LogEntry) bool {
	return e.Level >= f.Min
}

func (f *LevelFilter) Name() string {
	return fmt.Sprintf("level>=%s", f.Min)
}

// --- TimeFilter ---

// TimeFilter passes entries within [Start, End]. Zero value means unbounded.
type TimeFilter struct {
	Start time.Time
	End   time.Time
}

func NewTimeFilter(start, end time.Time) *TimeFilter {
	return &TimeFilter{Start: start, End: end}
}

func (f *TimeFilter) Match(e *logentry.LogEntry) bool {
	if e.Timestamp.IsZero() {
		return true // pass entries with no timestamp
	}
	if !f.Start.IsZero() && e.Timestamp.Before(f.Start) {
		return false
	}
	if !f.End.IsZero() && e.Timestamp.After(f.End) {
		return false
	}
	return true
}

func (f *TimeFilter) Name() string {
	return fmt.Sprintf("time[%s,%s]", f.Start.Format(time.RFC3339), f.End.Format(time.RFC3339))
}

// --- KeywordFilter ---

// KeywordFilter passes entries whose fields match a keyword or regex.
type KeywordFilter struct {
	pattern *regexp.Regexp
	literal string
	fields  []string
}

// NewKeywordFilter auto-detects whether the query is a plain string or regex.
func NewKeywordFilter(query string, fields []string) (*KeywordFilter, error) {
	if len(fields) == 0 {
		fields = []string{"message"}
	}
	if isPlainString(query) {
		return &KeywordFilter{literal: strings.ToLower(query), fields: fields}, nil
	}
	re, err := regexp.Compile("(?i)" + query)
	if err != nil {
		return nil, fmt.Errorf("invalid regex %q: %w", query, err)
	}
	return &KeywordFilter{pattern: re, fields: fields}, nil
}

func (f *KeywordFilter) Match(e *logentry.LogEntry) bool {
	for _, field := range f.fields {
		if f.matchField(e, field) {
			return true
		}
	}
	return false
}

func (f *KeywordFilter) matchField(e *logentry.LogEntry, field string) bool {
	var value string
	switch strings.ToLower(field) {
	case "message", "msg":
		value = e.Message
	case "logger", "class":
		value = e.Logger
	case "thread":
		value = e.Thread
	case "raw":
		value = e.Raw
	default:
		if v, ok := e.Fields[field]; ok {
			value = fmt.Sprintf("%v", v)
		}
	}

	if value == "" {
		return false
	}

	if f.pattern != nil {
		return f.pattern.MatchString(value)
	}
	return strings.Contains(strings.ToLower(value), f.literal)
}

func (f *KeywordFilter) Name() string {
	if f.pattern != nil {
		return fmt.Sprintf("keyword~/%s/", f.pattern)
	}
	return fmt.Sprintf("keyword=%q", f.literal)
}

// --- ProjectFilter ---

// ProjectFilter passes entries matching a specific project name.
type ProjectFilter struct {
	Projects []string
}

func NewProjectFilter(projects []string) *ProjectFilter {
	lower := make([]string, len(projects))
	for i, p := range projects {
		lower[i] = strings.ToLower(p)
	}
	return &ProjectFilter{Projects: lower}
}

func (f *ProjectFilter) Match(e *logentry.LogEntry) bool {
	if len(f.Projects) == 0 {
		return true
	}
	proj := strings.ToLower(e.Project)
	for _, p := range f.Projects {
		if proj == p {
			return true
		}
	}
	return false
}

func (f *ProjectFilter) Name() string {
	return fmt.Sprintf("project=%s", strings.Join(f.Projects, ","))
}

// --- TraceIDFilter ---

// TraceIDFilter passes entries matching a specific traceId.
type TraceIDFilter struct {
	TraceID string
}

func NewTraceIDFilter(traceID string) *TraceIDFilter {
	return &TraceIDFilter{TraceID: traceID}
}

func (f *TraceIDFilter) Match(e *logentry.LogEntry) bool {
	return strings.EqualFold(e.TraceID, f.TraceID)
}

func (f *TraceIDFilter) Name() string {
	return fmt.Sprintf("traceId=%s", f.TraceID)
}

// --- MDCFilter ---

// MDCFilter passes entries where an MDC field matches a value.
type MDCFilter struct {
	Key   string
	Value string
}

func NewMDCFilter(key, value string) *MDCFilter {
	return &MDCFilter{Key: strings.ToLower(key), Value: value}
}

func (f *MDCFilter) Match(e *logentry.LogEntry) bool {
	return strings.EqualFold(e.MDCValue(f.Key), f.Value)
}

func (f *MDCFilter) Name() string {
	return fmt.Sprintf("mdc[%s]=%s", f.Key, f.Value)
}

// isPlainString returns true if the query contains no regex metacharacters.
func isPlainString(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}
