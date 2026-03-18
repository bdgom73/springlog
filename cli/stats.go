package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"springlog/internal/aggregator"
	"springlog/internal/detector"
	"springlog/internal/parser"
	"springlog/internal/renderer"
)

var statsFlags struct {
	AllProjects bool
	TopErrors   int
	BucketSize  time.Duration
}

var statsCmd = &cobra.Command{
	Use:   "stats [flags] [path...]",
	Short: "Show log statistics and top errors",
	Long: `Aggregate log files and display a summary report.

Includes level breakdown, top errors grouped by message fingerprint,
and a time distribution histogram.

Examples:
  springlog stats ./logs/project-a/
  springlog stats ./logs/ --all-projects --top-errors 20
  springlog stats ./logs/ -l ERROR --from -7d`,
	RunE: runStats,
}

func init() {
	statsCmd.Flags().BoolVar(&statsFlags.AllProjects, "all-projects", false, "Treat each subdirectory as a separate project")
	statsCmd.Flags().IntVar(&statsFlags.TopErrors, "top-errors", 10, "Number of top error groups to show")
	statsCmd.Flags().DurationVar(&statsFlags.BucketSize, "bucket-size", time.Hour, "Time histogram bucket size (e.g. 1h, 30m, 24h)")
}

func runStats(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	filters, err := buildFilters()
	if err != nil {
		return err
	}

	opts := buildRendererOptions()
	rend := renderer.New(renderer.Format(globalFlags.Output), opts)

	agg := aggregator.New(statsFlags.BucketSize)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var totalFiles int

	for _, arg := range args {
		files, err := collectLogFiles(arg, statsFlags.AllProjects)
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
			switch format {
			case detector.FormatJSON:
				p = parser.NewJSONParser()
			default:
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

	fmt.Fprintf(os.Stderr, "Processed %d file(s)\n", totalFiles)

	stats := agg.Finalize(statsFlags.TopErrors)
	return rend.RenderStats(os.Stdout, stats)
}
