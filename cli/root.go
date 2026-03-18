package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"springlog/internal/filter"
	"springlog/internal/renderer"
	"springlog/pkg/logentry"
)

// Version is set at build time via -ldflags "-X springlog/cli.Version=vX.Y.Z".
var Version = "dev"

// GlobalFlags holds all flags shared across subcommands.
type GlobalFlags struct {
	Output       string
	Level        string
	From         string
	To           string
	Search       string
	SearchFields []string
	Projects     []string
	NoColor      bool
	TraceID      string   // 2단계
	MDC          []string // 2단계: key=value pairs
}

var globalFlags GlobalFlags

var rootCmd = &cobra.Command{
	Use:   "springlog",
	Short: "Spring Boot log analyzer",
	Long: `springlog analyzes Spring Boot log files (log4j/slf4j).

Supports both text and JSON formats, auto-detected by content.
Handles multiple projects with 60+ days of rotated log files.

Examples:
  springlog analyze ./logs/project-a/
  springlog analyze ./logs/ --all-projects -l ERROR --from -24h
  springlog stats ./logs/project-a/ --top-errors 20
  springlog tail ./logs/project-a/app.log -l WARN`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&globalFlags.Output, "output", "o", "table", "Output format: table|json|text")
	rootCmd.PersistentFlags().StringVarP(&globalFlags.Level, "level", "l", "", "Minimum log level: TRACE|DEBUG|INFO|WARN|ERROR|FATAL")
	rootCmd.PersistentFlags().StringVar(&globalFlags.From, "from", "", "Start time (RFC3339 or relative: -1h, -30m, -7d, yesterday, today)")
	rootCmd.PersistentFlags().StringVar(&globalFlags.To, "to", "", "End time (same formats as --from)")
	rootCmd.PersistentFlags().StringVarP(&globalFlags.Search, "search", "s", "", "Keyword or regex to search in message")
	rootCmd.PersistentFlags().StringSliceVar(&globalFlags.SearchFields, "search-fields", []string{"message"}, "Fields to search: message,logger,thread,raw")
	rootCmd.PersistentFlags().StringSliceVarP(&globalFlags.Projects, "project", "p", nil, "Filter by project name(s)")
	rootCmd.PersistentFlags().BoolVar(&globalFlags.NoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().StringVar(&globalFlags.TraceID, "trace-id", "", "Filter by trace ID (Micrometer/Sleuth)")
	rootCmd.PersistentFlags().StringArrayVar(&globalFlags.MDC, "mdc", nil, "Filter by MDC field (e.g. --mdc userId=1234)")

	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(tailCmd)
	rootCmd.AddCommand(exceptionsCmd)
	rootCmd.AddCommand(traceCmd)
	rootCmd.AddCommand(dashboardCmd)
}

// buildFilters constructs a filter.Chain from global flags.
func buildFilters() (filter.Chain, error) {
	var chain filter.Chain

	if globalFlags.Level != "" {
		lvl := logentry.ParseLevel(globalFlags.Level)
		if lvl == logentry.LevelUnknown {
			return nil, fmt.Errorf("unknown log level: %s", globalFlags.Level)
		}
		chain = append(chain, filter.NewLevelFilter(lvl))
	}

	from, to, err := parseTimeRange(globalFlags.From, globalFlags.To)
	if err != nil {
		return nil, err
	}
	if !from.IsZero() || !to.IsZero() {
		chain = append(chain, filter.NewTimeFilter(from, to))
	}

	if globalFlags.Search != "" {
		kf, err := filter.NewKeywordFilter(globalFlags.Search, globalFlags.SearchFields)
		if err != nil {
			return nil, err
		}
		chain = append(chain, kf)
	}

	if len(globalFlags.Projects) > 0 {
		chain = append(chain, filter.NewProjectFilter(globalFlags.Projects))
	}

	// 2단계: Trace ID
	if globalFlags.TraceID != "" {
		chain = append(chain, filter.NewTraceIDFilter(globalFlags.TraceID))
	}

	// 2단계: MDC filters (key=value)
	for _, kv := range globalFlags.MDC {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			chain = append(chain, filter.NewMDCFilter(parts[0], parts[1]))
		}
	}

	return chain, nil
}

// buildRendererOptions returns renderer options from global flags.
func buildRendererOptions() renderer.Options {
	opts := renderer.DefaultOptions()
	opts.ColorEnabled = !globalFlags.NoColor
	return opts
}

// parseTimeRange parses --from and --to flags into time.Time values.
func parseTimeRange(from, to string) (time.Time, time.Time, error) {
	var start, end time.Time
	var err error

	if from != "" {
		start, err = parseTime(from)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --from value %q: %w", from, err)
		}
	}
	if to != "" {
		end, err = parseTime(to)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --to value %q: %w", to, err)
		}
	}

	return start, end, nil
}

func parseTime(s string) (time.Time, error) {
	now := time.Now()

	switch strings.ToLower(s) {
	case "today":
		y, m, d := now.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location()), nil
	case "yesterday":
		y, m, d := now.AddDate(0, 0, -1).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location()), nil
	}

	// Relative duration: -1h, -30m, -7d
	if strings.HasPrefix(s, "-") {
		s2 := s[1:]
		// Support days
		if strings.HasSuffix(s2, "d") {
			days := 0
			fmt.Sscanf(s2, "%dd", &days)
			return now.AddDate(0, 0, -days), nil
		}
		d, err := time.ParseDuration(s2)
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(-d), nil
	}

	// Absolute formats
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized time format")
}
