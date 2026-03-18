package cli

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"springlog/internal/detector"
	"springlog/internal/filter"
	"springlog/internal/parser"
	"springlog/internal/renderer"
	"springlog/pkg/logentry"
)

var traceCmd = &cobra.Command{
	Use:   "trace [flags] [path...]",
	Short: "Reconstruct a full request trace by trace ID",
	Long: `Show all log entries belonging to a single request, ordered by time.

Requires --trace-id. Works with Spring Boot Micrometer Tracing or Sleuth.
Reconstructs the full lifecycle of a request across threads and services.

Examples:
  springlog trace ./logs/ --all-projects --trace-id abc123def456
  springlog trace ./logs/project-a/ --trace-id abc123 -o json`,
	RunE: runTrace,
}

var traceFlags struct {
	AllProjects bool
}

func init() {
	traceCmd.Flags().BoolVar(&traceFlags.AllProjects, "all-projects", false, "Scan all subdirectories as projects")
}

func runTrace(cmd *cobra.Command, args []string) error {
	if globalFlags.TraceID == "" {
		return fmt.Errorf("--trace-id is required for the trace command")
	}
	if len(args) == 0 {
		args = []string{"."}
	}

	traceFilter := filter.Chain{filter.NewTraceIDFilter(globalFlags.TraceID)}

	// Apply additional MDC filters if provided
	for _, kv := range globalFlags.MDC {
		parts := splitKV(kv)
		if parts != nil {
			traceFilter = append(traceFilter, filter.NewMDCFilter(parts[0], parts[1]))
		}
	}

	opts := buildRendererOptions()
	rend := renderer.New(renderer.Format(globalFlags.Output), opts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var matched []*logentry.LogEntry

	for _, arg := range args {
		files, err := collectLogFiles(arg, traceFlags.AllProjects)
		if err != nil {
			return err
		}
		for _, lf := range files {
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
					if traceFilter.Match(e) {
						matched = append(matched, e)
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

	if len(matched) == 0 {
		fmt.Fprintf(os.Stderr, "No entries found for trace ID: %s\n", globalFlags.TraceID)
		return nil
	}

	// Sort by timestamp
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Timestamp.Before(matched[j].Timestamp)
	})

	bold := color.New(color.Bold)
	bold.Fprintf(os.Stderr, "\n=== Trace: %s ===\n", globalFlags.TraceID)
	fmt.Fprintf(os.Stderr, "Entries found : %d\n", len(matched))
	if !matched[0].Timestamp.IsZero() && !matched[len(matched)-1].Timestamp.IsZero() {
		dur := matched[len(matched)-1].Timestamp.Sub(matched[0].Timestamp)
		fmt.Fprintf(os.Stderr, "Duration      : %s\n", dur)
	}

	// Gather unique threads
	threads := map[string]bool{}
	for _, e := range matched {
		if e.Thread != "" {
			threads[e.Thread] = true
		}
	}
	if len(threads) > 0 {
		fmt.Fprintf(os.Stderr, "Threads       : ")
		first := true
		for t := range threads {
			if !first {
				fmt.Fprintf(os.Stderr, ", ")
			}
			fmt.Fprintf(os.Stderr, "%s", t)
			first = false
		}
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr)

	for _, e := range matched {
		if err := rend.RenderEntry(os.Stdout, e); err != nil {
			return err
		}
	}

	// Summary
	errCount := 0
	for _, e := range matched {
		if e.Level >= logentry.LevelError {
			errCount++
		}
	}
	if errCount > 0 {
		color.New(color.FgRed).Fprintf(os.Stderr, "\n⚠ %d error(s) in this trace\n", errCount)
	} else {
		color.New(color.FgGreen).Fprintln(os.Stderr, "\n✓ No errors in this trace")
	}

	return nil
}

func splitKV(kv string) []string {
	for i, c := range kv {
		if c == '=' {
			return []string{kv[:i], kv[i+1:]}
		}
	}
	return nil
}
