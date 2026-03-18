package renderer

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"springlog/internal/aggregator"
	"springlog/pkg/logentry"
)

var levelColors = map[logentry.Level]*color.Color{
	logentry.LevelTrace: color.New(color.FgHiBlack),
	logentry.LevelDebug: color.New(color.FgCyan),
	logentry.LevelInfo:  color.New(color.FgGreen),
	logentry.LevelWarn:  color.New(color.FgYellow),
	logentry.LevelError: color.New(color.FgRed),
	logentry.LevelFatal: color.New(color.FgRed, color.Bold),
}

// TableRenderer renders log entries as a colored terminal table.
type TableRenderer struct {
	opts Options
}

func NewTableRenderer(opts Options) *TableRenderer {
	if !opts.ColorEnabled {
		color.NoColor = true
	}
	return &TableRenderer{opts: opts}
}

func (r *TableRenderer) RenderEntry(w io.Writer, e *logentry.LogEntry) error {
	ts := ""
	if !e.Timestamp.IsZero() {
		ts = e.Timestamp.Format(r.opts.TimestampFmt)
	}

	level := colorizeLevel(e.Level)
	msg := e.Message
	if r.opts.MaxMessageLen > 0 && len(msg) > r.opts.MaxMessageLen {
		msg = msg[:r.opts.MaxMessageLen] + "..."
	}

	project := ""
	if r.opts.ShowProject && e.Project != "" {
		project = fmt.Sprintf("[%s] ", e.Project)
	}

	_, err := fmt.Fprintf(w, "%s %-5s %s%s : %s\n", ts, level, project, shortLogger(e.Logger), msg)
	if err != nil {
		return err
	}
	for _, line := range e.StackTrace {
		_, err = fmt.Fprintf(w, "  %s\n", color.New(color.FgHiBlack).Sprint(line))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *TableRenderer) RenderStats(w io.Writer, s *aggregator.Stats) error {
	bold := color.New(color.Bold)

	bold.Fprintln(w, "\n=== Log Analysis Summary ===")
	fmt.Fprintf(w, "Total entries : %d\n", s.Total)
	if !s.FirstSeen.IsZero() {
		fmt.Fprintf(w, "Time range    : %s → %s\n",
			s.FirstSeen.Format(r.opts.TimestampFmt),
			s.LastSeen.Format(r.opts.TimestampFmt))
	}

	// --- Level breakdown ---
	bold.Fprintln(w, "\n--- By Level ---")
	tbl := tablewriter.NewWriter(w)
	tbl.Header("Level", "Count", "Ratio")
	for _, lvl := range []logentry.Level{
		logentry.LevelFatal, logentry.LevelError, logentry.LevelWarn,
		logentry.LevelInfo, logentry.LevelDebug, logentry.LevelTrace,
	} {
		cnt := s.ByLevel[lvl]
		if cnt == 0 {
			continue
		}
		ratio := float64(cnt) / float64(s.Total) * 100
		tbl.Append(colorizeLevel(lvl), fmt.Sprintf("%d", cnt), fmt.Sprintf("%.1f%%", ratio))
	}
	tbl.Render()

	// --- By Project ---
	if len(s.ByProject) > 1 {
		bold.Fprintln(w, "\n--- By Project ---")
		ptbl := tablewriter.NewWriter(w)
		ptbl.Header("Project", "Count")
		for proj, cnt := range s.ByProject {
			ptbl.Append(proj, fmt.Sprintf("%d", cnt))
		}
		ptbl.Render()
	}

	// --- [1단계] Exception Type Analysis ---
	if len(s.TopExceptions) > 0 {
		bold.Fprintln(w, "\n--- Exception Analysis ---")
		etbl := tablewriter.NewWriter(w)
		etbl.Header("#", "Exception", "Count", "First Seen", "Last Seen", "Thrown By")
		for i, ex := range s.TopExceptions {
			firstSeen, lastSeen := "", ""
			if !ex.FirstSeen.IsZero() {
				firstSeen = ex.FirstSeen.Format("01-02 15:04:05")
			}
			if !ex.LastSeen.IsZero() {
				lastSeen = ex.LastSeen.Format("01-02 15:04:05")
			}

			// Show up to 2 loggers
			loggers := ex.Loggers
			if len(loggers) > 2 {
				loggers = loggers[:2]
			}
			thrownBy := strings.Join(shortLoggers(loggers), ", ")
			if len(ex.Loggers) > 2 {
				thrownBy += fmt.Sprintf(" +%d", len(ex.Loggers)-2)
			}

			etbl.Append(
				fmt.Sprintf("%d", i+1),
				color.New(color.FgYellow).Sprint(ex.ClassName),
				fmt.Sprintf("%d", ex.Count),
				firstSeen,
				lastSeen,
				thrownBy,
			)
		}
		etbl.Render()
	}

	// --- [1단계] Top Error Classes ---
	if len(s.TopLoggers) > 0 {
		bold.Fprintln(w, "\n--- Top Error Classes ---")
		ltbl := tablewriter.NewWriter(w)
		ltbl.Header("Logger", "FATAL", "ERROR", "WARN", "Total")
		for _, l := range s.TopLoggers {
			fatal := l.ByLevel[logentry.LevelFatal]
			errCnt := l.ByLevel[logentry.LevelError]
			warn := l.ByLevel[logentry.LevelWarn]

			fatalStr := "-"
			if fatal > 0 {
				fatalStr = color.New(color.FgRed, color.Bold).Sprintf("%d", fatal)
			}
			errStr := "-"
			if errCnt > 0 {
				errStr = color.New(color.FgRed).Sprintf("%d", errCnt)
			}
			warnStr := "-"
			if warn > 0 {
				warnStr = color.New(color.FgYellow).Sprintf("%d", warn)
			}

			ltbl.Append(shortLogger(l.Logger), fatalStr, errStr, warnStr, fmt.Sprintf("%d", l.Total))
		}
		ltbl.Render()
	}

	// --- Top Errors (message fingerprint) ---
	if len(s.TopErrors) > 0 {
		bold.Fprintln(w, "\n--- Top Errors ---")
		errtbl := tablewriter.NewWriter(w)
		errtbl.Header("#", "Count", "Level", "First Seen", "Message")
		for i, g := range s.TopErrors {
			msg := g.Fingerprint
			if idx := strings.Index(msg, "|"); idx >= 0 {
				msg = msg[idx+1:]
			}
			if len(msg) > 80 {
				msg = msg[:80] + "..."
			}
			firstSeen := ""
			if !g.FirstSeen.IsZero() {
				firstSeen = g.FirstSeen.Format("01-02 15:04")
			}
			errtbl.Append(
				fmt.Sprintf("%d", i+1),
				fmt.Sprintf("%d", g.Count),
				colorizeLevel(g.Level),
				firstSeen,
				msg,
			)
		}
		errtbl.Render()
	}

	// --- [1단계] Spike Detection ---
	if len(s.Spikes) > 0 {
		bold.Fprintln(w, "\n--- ⚠ Error Spike Detection ---")
		spikeTbl := tablewriter.NewWriter(w)
		spikeTbl.Header("Time", "Errors", "Total", "Avg/bucket", "Multiplier", "Severity")
		for _, sp := range s.Spikes {
			severity := "⚠ HIGH"
			if sp.Multiplier >= 5 {
				severity = color.New(color.FgRed, color.Bold).Sprint("🔥 CRITICAL")
			} else if sp.Multiplier >= 3 {
				severity = color.New(color.FgRed).Sprint("⚠ HIGH")
			}
			spikeTbl.Append(
				sp.Start.Format("01-02 15:04"),
				color.New(color.FgRed).Sprintf("%d", sp.ErrorCount),
				fmt.Sprintf("%d", sp.TotalCount),
				fmt.Sprintf("%.1f", sp.AvgErrors),
				fmt.Sprintf("%.1fx", sp.Multiplier),
				severity,
			)
		}
		spikeTbl.Render()
	}

	// --- [3단계] Startup Analysis ---
	if s.Startup != nil && s.Startup.TotalStartupMs > 0 {
		bold.Fprintln(w, "\n--- Spring Boot Startup ---")
		fmt.Fprintf(w, "  Startup time : %.0f ms (%.2f s)\n", s.Startup.TotalStartupMs, s.Startup.TotalStartupMs/1000)
		if s.Startup.Port != "" {
			fmt.Fprintf(w, "  Port         : %s\n", s.Startup.Port)
		}
		if s.Startup.Profile != "" {
			fmt.Fprintf(w, "  Profile      : %s\n", s.Startup.Profile)
		}
		if s.Startup.BeanCount > 0 {
			fmt.Fprintf(w, "  Beans loaded : %d\n", s.Startup.BeanCount)
		}
	}

	// --- [3단계] Latency Percentiles ---
	if s.Latency != nil && s.Latency.Count > 0 {
		bold.Fprintln(w, "\n--- Response Time Percentiles ---")
		ltbl := tablewriter.NewWriter(w)
		ltbl.Header("Metric", "Value")
		ltbl.Append("Count", fmt.Sprintf("%d requests", s.Latency.Count))
		ltbl.Append("Min", fmt.Sprintf("%.0f ms", s.Latency.Min))
		ltbl.Append("Mean", fmt.Sprintf("%.0f ms", s.Latency.Mean))
		ltbl.Append("p50 (median)", fmt.Sprintf("%.0f ms", s.Latency.P50))
		ltbl.Append("p95", formatLatency(s.Latency.P95))
		ltbl.Append("p99", formatLatency(s.Latency.P99))
		ltbl.Append("Max", formatLatency(s.Latency.Max))
		ltbl.Render()

		if len(s.Latency.SlowRequests) > 0 {
			fmt.Fprintf(w, "\n  Slow requests (>= 1000ms):\n")
			stbl := tablewriter.NewWriter(w)
			stbl.Header("Duration", "Time", "Logger", "Message")
			for _, sr := range s.Latency.SlowRequests {
				msg := sr.Message
				if len(msg) > 60 {
					msg = msg[:60] + "..."
				}
				stbl.Append(
					color.New(color.FgRed).Sprintf("%.0fms", sr.DurationMs),
					sr.Timestamp,
					shortLogger(sr.Logger),
					msg,
				)
			}
			stbl.Render()
		}
	}

	// --- Time Distribution ---
	if len(s.TimeHistogram) > 0 {
		bold.Fprintln(w, "\n--- Time Distribution ---")
		renderHistogram(w, s.TimeHistogram, s.Spikes)
	}

	return nil
}

func formatLatency(ms float64) string {
	c := color.New(color.FgGreen)
	if ms >= 3000 {
		c = color.New(color.FgRed, color.Bold)
	} else if ms >= 1000 {
		c = color.New(color.FgRed)
	} else if ms >= 500 {
		c = color.New(color.FgYellow)
	}
	return c.Sprintf("%.0f ms", ms)
}

func renderHistogram(w io.Writer, buckets []*aggregator.TimeBucket, spikes []*aggregator.Spike) {
	spikeSet := map[string]bool{}
	for _, sp := range spikes {
		spikeSet[sp.Start.Format("01-02 15:04")] = true
	}

	var maxCount int64
	for _, b := range buckets {
		if b.Count > maxCount {
			maxCount = b.Count
		}
	}
	if maxCount == 0 {
		return
	}

	const barWidth = 30
	for _, b := range buckets {
		label := b.Start.Format("01-02 15:04")
		barLen := int(float64(b.Count) / float64(maxCount) * barWidth)
		bar := strings.Repeat("█", barLen)

		errors := b.ByLevel[logentry.LevelError] + b.ByLevel[logentry.LevelFatal]
		suffix := ""
		if errors > 0 {
			suffix = color.RedString(" (%d err)", errors)
		}

		spike := ""
		if spikeSet[label] {
			spike = color.New(color.FgRed, color.Bold).Sprint(" ⚠ SPIKE")
		}

		fmt.Fprintf(w, "  %s │%-*s│ %d%s%s\n", label, barWidth, bar, b.Count, suffix, spike)
	}
}

func colorizeLevel(l logentry.Level) string {
	c, ok := levelColors[l]
	if !ok {
		return l.String()
	}
	return c.Sprint(l.String())
}

func shortLogger(logger string) string {
	parts := strings.Split(logger, ".")
	if len(parts) <= 1 {
		return logger
	}
	short := make([]string, len(parts))
	for i, p := range parts[:len(parts)-1] {
		if len(p) > 0 {
			short[i] = string(p[0])
		}
	}
	short[len(parts)-1] = parts[len(parts)-1]
	return strings.Join(short, ".")
}

func shortLoggers(loggers []string) []string {
	result := make([]string, len(loggers))
	for i, l := range loggers {
		result[i] = shortLogger(l)
	}
	return result
}
