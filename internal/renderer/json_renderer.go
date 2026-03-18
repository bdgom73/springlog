package renderer

import (
	"encoding/json"
	"io"
	"time"

	"springlog/internal/aggregator"
	"springlog/pkg/logentry"
)

// JSONRenderer outputs log entries as JSON lines (JSONL), compatible with jq.
type JSONRenderer struct{}

func NewJSONRenderer() *JSONRenderer {
	return &JSONRenderer{}
}

type jsonEntry struct {
	Timestamp  string         `json:"timestamp,omitempty"`
	Level      string         `json:"level"`
	PID        int            `json:"pid,omitempty"`
	Thread     string         `json:"thread,omitempty"`
	Logger     string         `json:"logger,omitempty"`
	Message    string         `json:"message"`
	StackTrace []string       `json:"stack_trace,omitempty"`
	Project    string         `json:"project,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
	SourceFile string         `json:"source_file,omitempty"`
	LineNumber int64          `json:"line_number,omitempty"`
}

func (r *JSONRenderer) RenderEntry(w io.Writer, e *logentry.LogEntry) error {
	j := jsonEntry{
		Level:      e.Level.String(),
		PID:        e.PID,
		Thread:     e.Thread,
		Logger:     e.Logger,
		Message:    e.Message,
		StackTrace: e.StackTrace,
		Project:    e.Project,
		Fields:     e.Fields,
		SourceFile: e.SourceFile,
		LineNumber: e.LineNumber,
	}
	if !e.Timestamp.IsZero() {
		j.Timestamp = e.Timestamp.Format(time.RFC3339Nano)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(j)
}

func (r *JSONRenderer) RenderStats(w io.Writer, s *aggregator.Stats) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
