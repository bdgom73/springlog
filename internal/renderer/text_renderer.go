package renderer

import (
	"fmt"
	"io"
	"strings"

	"springlog/internal/aggregator"
	"springlog/pkg/logentry"
)

// TextRenderer outputs log entries in the original Spring Boot text format.
type TextRenderer struct {
	opts Options
}

func NewTextRenderer(opts Options) *TextRenderer {
	return &TextRenderer{opts: opts}
}

func (r *TextRenderer) RenderEntry(w io.Writer, e *logentry.LogEntry) error {
	if e.Raw != "" {
		_, err := fmt.Fprintln(w, e.Raw)
		if err != nil {
			return err
		}
		for _, line := range e.StackTrace {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
		return nil
	}

	ts := ""
	if !e.Timestamp.IsZero() {
		ts = e.Timestamp.Format(r.opts.TimestampFmt)
	}
	_, err := fmt.Fprintf(w, "%s %s %d --- [%s] %s : %s\n",
		ts, e.Level, e.PID, e.Thread, e.Logger, e.Message)
	return err
}

func (r *TextRenderer) RenderStats(w io.Writer, s *aggregator.Stats) error {
	fmt.Fprintf(w, "Total: %d\n", s.Total)
	fmt.Fprintf(w, "Time range: %s -> %s\n",
		s.FirstSeen.Format(r.opts.TimestampFmt),
		s.LastSeen.Format(r.opts.TimestampFmt))

	fmt.Fprintln(w, "\nBy Level:")
	for _, lvl := range []logentry.Level{
		logentry.LevelFatal, logentry.LevelError, logentry.LevelWarn,
		logentry.LevelInfo, logentry.LevelDebug, logentry.LevelTrace,
	} {
		if cnt := s.ByLevel[lvl]; cnt > 0 {
			fmt.Fprintf(w, "  %-7s %d\n", lvl, cnt)
		}
	}

	if len(s.TopErrors) > 0 {
		fmt.Fprintln(w, "\nTop Errors:")
		for i, g := range s.TopErrors {
			msg := g.Fingerprint
			if idx := strings.Index(msg, "|"); idx >= 0 {
				msg = msg[idx+1:]
			}
			fmt.Fprintf(w, "  %d. [%d] %s\n", i+1, g.Count, msg)
		}
	}

	return nil
}
