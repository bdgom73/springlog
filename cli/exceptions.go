package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"springlog/internal/aggregator"
	"springlog/internal/detector"
	"springlog/internal/parser"
)

var exceptionsFlags struct {
	AllProjects bool
	TopN        int
	ShowStack   bool
}

var exceptionsCmd = &cobra.Command{
	Use:   "exceptions [flags] [path...]",
	Short: "Analyze exception types from stack traces",
	Long: `Deep-dive into exception patterns across log files.

Groups exceptions by class name, shows frequency, affected loggers,
and example stack traces. Helps identify the root cause of failures.

Examples:
  springlog exceptions ./logs/project-a/
  springlog exceptions ./logs/ --all-projects --top 20
  springlog exceptions ./logs/ -l ERROR --from -7d --show-stack`,
	RunE: runExceptions,
}

func init() {
	exceptionsCmd.Flags().BoolVar(&exceptionsFlags.AllProjects, "all-projects", false, "Treat each subdirectory as a separate project")
	exceptionsCmd.Flags().IntVar(&exceptionsFlags.TopN, "top", 10, "Number of exception types to show")
	exceptionsCmd.Flags().BoolVar(&exceptionsFlags.ShowStack, "show-stack", false, "Show example stack trace for each exception type")
}

func runExceptions(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	filters, err := buildFilters()
	if err != nil {
		return err
	}

	agg := aggregator.New(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var totalFiles int
	for _, arg := range args {
		files, err := collectLogFiles(arg, exceptionsFlags.AllProjects)
		if err != nil {
			return err
		}
		for _, lf := range files {
			totalFiles++
			f, err := os.Open(lf.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: cannot open %s: %v\n", lf.Path, err)
				continue
			}

			format, reader, err := detector.Detect(f)
			if err != nil {
				f.Close()
				continue
			}

			var p parser.Parser
			if format == detector.FormatJSON {
				p = parser.NewJSONParser()
			} else {
				p = parser.NewTextParser()
			}

			entCh, errCh := p.Parse(ctx, reader, lf.Path)
		loop:
			for {
				select {
				case e, ok := <-entCh:
					if !ok {
						break loop
					}
					e.Project = lf.Project
					if filters.Match(e) {
						agg.Ingest(e)
					}
				case err, ok := <-errCh:
					if !ok {
						continue
					}
					fmt.Fprintf(os.Stderr, "warn: %v\n", err)
				}
			}
			f.Close()
		}
	}

	fmt.Fprintf(os.Stderr, "Processed %d file(s)\n\n", totalFiles)

	stats := agg.Finalize(exceptionsFlags.TopN)
	renderExceptionReport(os.Stdout, stats, exceptionsFlags.ShowStack)
	return nil
}

func renderExceptionReport(w *os.File, s *aggregator.Stats, showStack bool) {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)

	if len(s.TopExceptions) == 0 {
		fmt.Fprintln(w, "No exceptions found in the selected log entries.")
		return
	}

	bold.Fprintln(w, "=== Exception Analysis ===")
	fmt.Fprintf(w, "Total entries analyzed : %d\n", s.Total)
	fmt.Fprintf(w, "Exception types found  : %d\n\n", len(s.TopExceptions))

	// Main exception table
	tbl := tablewriter.NewWriter(w)
	tbl.Header("#", "Exception Class", "Count", "First Seen", "Last Seen", "Thrown By (top loggers)")
	for i, ex := range s.TopExceptions {
		firstSeen, lastSeen := "", ""
		if !ex.FirstSeen.IsZero() {
			firstSeen = ex.FirstSeen.Format("01-02 15:04:05")
		}
		if !ex.LastSeen.IsZero() {
			lastSeen = ex.LastSeen.Format("01-02 15:04:05")
		}

		loggers := ex.Loggers
		if len(loggers) > 3 {
			loggers = loggers[:3]
		}
		short := make([]string, len(loggers))
		for j, l := range loggers {
			short[j] = shortLogger(l)
		}
		thrownBy := strings.Join(short, "\n")
		if len(ex.Loggers) > 3 {
			thrownBy += fmt.Sprintf("\n+%d more", len(ex.Loggers)-3)
		}

		cls := yellow.Sprint(ex.ClassName)
		if ex.Count >= 5 {
			cls = red.Sprint(ex.ClassName)
		}

		tbl.Append(
			fmt.Sprintf("%d", i+1),
			cls,
			fmt.Sprintf("%d", ex.Count),
			firstSeen,
			lastSeen,
			thrownBy,
		)
	}
	tbl.Render()

	// Spike summary
	if len(s.Spikes) > 0 {
		fmt.Fprintln(w)
		bold.Fprintln(w, "⚠ Error Spikes Detected:")
		for _, sp := range s.Spikes {
			red.Fprintf(w, "  %s — %d errors (%.1fx above average of %.1f/bucket)\n",
				sp.Start.Format("2006-01-02 15:04"),
				sp.ErrorCount,
				sp.Multiplier,
				sp.AvgErrors,
			)
		}
	}

	// Example stack traces per exception
	if showStack {
		fmt.Fprintln(w)
		bold.Fprintln(w, "=== Example Stack Traces ===")

		// Sort by count desc (already sorted)
		shown := s.TopExceptions
		if len(shown) > 5 {
			shown = shown[:5]
		}

		for _, ex := range shown {
			if len(ex.Examples) == 0 {
				continue
			}
			fmt.Fprintln(w)
			yellow.Fprintf(w, "── %s (%d occurrence(s)) ──\n", ex.FullName, ex.Count)

			e := ex.Examples[0]
			if !e.Timestamp.IsZero() {
				fmt.Fprintf(w, "  Time   : %s\n", e.Timestamp.Format("2006-01-02 15:04:05"))
			}
			if e.Logger != "" {
				fmt.Fprintf(w, "  Logger : %s\n", e.Logger)
			}
			if e.Project != "" {
				fmt.Fprintf(w, "  Project: %s\n", e.Project)
			}
			fmt.Fprintf(w, "  Message: %s\n", e.Message)

			if len(e.StackTrace) > 0 {
				fmt.Fprintln(w, "  Stack  :")
				lines := e.StackTrace
				if len(lines) > 10 {
					lines = lines[:10]
				}
				for _, line := range lines {
					color.New(color.FgHiBlack).Fprintf(w, "    %s\n", line)
				}
				if len(e.StackTrace) > 10 {
					fmt.Fprintf(w, "    ... (%d more lines)\n", len(e.StackTrace)-10)
				}
			}
		}
	}

	// Exception trend (which buckets had which exception types)
	if len(s.TimeHistogram) > 0 {
		fmt.Fprintln(w)
		bold.Fprintln(w, "--- Error Count by Hour ---")
		renderExceptionHistogram(w, s.TimeHistogram, s.Spikes)
	}

	// Top error-generating classes
	if len(s.TopLoggers) > 0 {
		fmt.Fprintln(w)
		bold.Fprintln(w, "--- Most Error-Prone Classes ---")
		ltbl := tablewriter.NewWriter(w)
		ltbl.Header("Rank", "Class", "FATAL", "ERROR", "WARN")

		sorted := make([]*aggregator.LoggerStat, len(s.TopLoggers))
		copy(sorted, s.TopLoggers)
		sort.Slice(sorted, func(i, j int) bool {
			ei := sorted[i].ByLevel[2] + sorted[i].ByLevel[1]
			ej := sorted[j].ByLevel[2] + sorted[j].ByLevel[1]
			return ei > ej
		})

		for i, l := range sorted {
			fatal := l.ByLevel[6] // LevelFatal=6
			errCnt := l.ByLevel[5]
			warn := l.ByLevel[4]

			fStr := "-"
			if fatal > 0 {
				fStr = red.Sprintf("%d", fatal)
			}
			eStr := "-"
			if errCnt > 0 {
				eStr = color.New(color.FgRed).Sprintf("%d", errCnt)
			}
			wStr := "-"
			if warn > 0 {
				wStr = color.New(color.FgYellow).Sprintf("%d", warn)
			}

			ltbl.Append(fmt.Sprintf("%d", i+1), shortLogger(l.Logger), fStr, eStr, wStr)
		}
		ltbl.Render()
	}
}

func renderExceptionHistogram(w *os.File, buckets []*aggregator.TimeBucket, spikes []*aggregator.Spike) {
	spikeSet := map[string]bool{}
	for _, sp := range spikes {
		spikeSet[sp.Start.Format("01-02 15:04")] = true
	}

	var maxErr int64
	for _, b := range buckets {
		e := b.ByLevel[5] + b.ByLevel[6]
		if e > maxErr {
			maxErr = e
		}
	}
	if maxErr == 0 {
		fmt.Fprintln(w, "  (no errors in selected time range)")
		return
	}

	const barWidth = 25
	for _, b := range buckets {
		errs := b.ByLevel[5] + b.ByLevel[6]
		label := b.Start.Format("01-02 15:04")
		barLen := 0
		if maxErr > 0 {
			barLen = int(float64(errs) / float64(maxErr) * barWidth)
		}
		bar := strings.Repeat("█", barLen)

		spike := ""
		if spikeSet[label] {
			spike = color.New(color.FgRed, color.Bold).Sprint(" ⚠ SPIKE")
		}

		errColor := color.New(color.FgGreen)
		if errs > 0 {
			errColor = color.New(color.FgRed)
		}

		fmt.Fprintf(w, "  %s │%-*s│ %s%s\n",
			label, barWidth, bar,
			errColor.Sprintf("%d err / %d total", errs, b.Count),
			spike,
		)
	}
}
