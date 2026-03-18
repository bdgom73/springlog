package renderer

import (
	"io"
	"springlog/internal/aggregator"
	"springlog/pkg/logentry"
)

// Renderer writes formatted output to a writer.
type Renderer interface {
	RenderEntry(w io.Writer, e *logentry.LogEntry) error
	RenderStats(w io.Writer, s *aggregator.Stats) error
}

// Format identifies the output format.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatText  Format = "text"
)

// Options holds cross-renderer configuration.
type Options struct {
	ColorEnabled  bool
	TimestampFmt  string
	MaxMessageLen int
	ShowProject   bool
}

func DefaultOptions() Options {
	return Options{
		ColorEnabled:  true,
		TimestampFmt:  "2006-01-02 15:04:05.000",
		MaxMessageLen: 0,
		ShowProject:   true,
	}
}

// New constructs a Renderer from a format string.
func New(f Format, opts Options) Renderer {
	switch f {
	case FormatJSON:
		return NewJSONRenderer()
	case FormatText:
		return NewTextRenderer(opts)
	default:
		return NewTableRenderer(opts)
	}
}
