package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"io"

	"github.com/spf13/cobra"
	"springlog/internal/detector"
	"springlog/internal/parser"
	"springlog/internal/renderer"
)

var analyzeFlags struct {
	AllProjects bool
	Head        int
	FormatHint  string
}

var analyzeCmd = &cobra.Command{
	Use:     "analyze [flags] [path...]",
	Aliases: []string{"a"},
	Short:   "Filter and display log entries",
	Long: `Analyze one or more log files or directories.

When a directory is given, all log files inside are processed.
Use --all-projects to scan all subdirectories as separate projects.

Examples:
  springlog analyze ./logs/project-a/
  springlog analyze ./logs/ --all-projects -l ERROR
  springlog analyze app.log --search "NullPointer" --from -1h
  springlog analyze ./logs/ -l ERROR -o json | jq .`,
	RunE: runAnalyze,
}

func init() {
	analyzeCmd.Flags().BoolVar(&analyzeFlags.AllProjects, "all-projects", false, "Treat each subdirectory as a separate project")
	analyzeCmd.Flags().IntVar(&analyzeFlags.Head, "head", 0, "Stop after N matching entries (0 = unlimited)")
	analyzeCmd.Flags().StringVar(&analyzeFlags.FormatHint, "format-hint", "", "Override format detection: text|json")
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	// stdin 파이프 지원: 인자가 없고 stdin이 파이프면 stdin 읽기
	if len(args) == 0 {
		if isStdinPiped() {
			return runAnalyzeReader(os.Stdin, "stdin")
		}
		args = []string{"."}
	}

	filters, err := buildFilters()
	if err != nil {
		return err
	}

	opts := buildRendererOptions()
	rend := renderer.New(renderer.Format(globalFlags.Output), opts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int
	for _, arg := range args {
		files, err := collectLogFiles(arg, analyzeFlags.AllProjects)
		if err != nil {
			return err
		}

		for _, lf := range files {
			f, err := os.Open(lf.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: cannot open %s: %v\n", lf.Path, err)
				continue
			}

			format := detectorFormatHint(analyzeFlags.FormatHint)
			var reader io.Reader = f
			if format == detector.FormatUnknown {
				format, reader, err = detector.Detect(f)
				if err != nil {
					f.Close()
					continue
				}
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
					if !filters.Match(e) {
						continue
					}
					if err := rend.RenderEntry(os.Stdout, e); err != nil {
						f.Close()
						return err
					}
					count++
					if analyzeFlags.Head > 0 && count >= analyzeFlags.Head {
						f.Close()
						return nil
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

	if count == 0 {
		fmt.Fprintln(os.Stderr, "No matching log entries found.")
	} else {
		fmt.Fprintf(os.Stderr, "\n%d entries matched.\n", count)
	}

	return nil
}

// runAnalyzeReader handles reading from an io.Reader (e.g. stdin).
func runAnalyzeReader(r io.Reader, source string) error {
	filters, err := buildFilters()
	if err != nil {
		return err
	}
	opts := buildRendererOptions()
	rend := renderer.New(renderer.Format(globalFlags.Output), opts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	format, reader, err := detector.Detect(r)
	if err != nil {
		return err
	}

	var p parser.Parser
	if format == detector.FormatJSON {
		p = parser.NewJSONParser()
	} else {
		p = parser.NewTextParser()
	}

	entCh, errCh := p.Parse(ctx, reader, source)
	var count int
loop:
	for {
		select {
		case e, ok := <-entCh:
			if !ok {
				break loop
			}
			if !filters.Match(e) {
				continue
			}
			if err := rend.RenderEntry(os.Stdout, e); err != nil {
				return err
			}
			count++
			if analyzeFlags.Head > 0 && count >= analyzeFlags.Head {
				return nil
			}
		case err, ok := <-errCh:
			if !ok {
				continue
			}
			fmt.Fprintf(os.Stderr, "warn: %v\n", err)
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d entries matched.\n", count)
	return nil
}

// isStdinPiped reports whether stdin is connected to a pipe (not a terminal).
func isStdinPiped() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}

// logFile represents a log file with its resolved project name.
type logFile struct {
	Path    string
	Project string
}

// collectLogFiles collects all log files under a path.
// If allProjects is true, each immediate subdirectory becomes a project.
func collectLogFiles(root string, allProjects bool) ([]logFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []logFile{{Path: root, Project: filepath.Base(filepath.Dir(root))}}, nil
	}

	var files []logFile

	if allProjects {
		// Each subdirectory is a project
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			projectName := e.Name()
			projectPath := filepath.Join(root, projectName)
			lfs, err := walkLogFiles(projectPath, projectName)
			if err != nil {
				return nil, err
			}
			files = append(files, lfs...)
		}
	} else {
		project := filepath.Base(root)
		lfs, err := walkLogFiles(root, project)
		if err != nil {
			return nil, err
		}
		files = append(files, lfs...)
	}

	// Sort by path for deterministic order
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

func walkLogFiles(root, project string) ([]logFile, error) {
	var files []logFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		if isLogFile(path) {
			files = append(files, logFile{Path: path, Project: project})
		}
		return nil
	})
	return files, err
}

func isLogFile(path string) bool {
	ext := filepath.Ext(path)
	base := filepath.Base(path)
	// Accept .log, .json, or files with no extension containing "log" in name
	switch ext {
	case ".log", ".json":
		return true
	case "":
		return containsLogKeyword(base)
	}
	// Spring Boot rotated logs: app.log.1, app.log.2024-01-15
	if len(ext) > 1 {
		prev := filepath.Ext(base[:len(base)-len(ext)])
		if prev == ".log" {
			return true
		}
	}
	return false
}

func containsLogKeyword(name string) bool {
	for _, kw := range []string{"log", "error", "warn", "debug", "trace"} {
		if len(name) >= len(kw) {
			for i := 0; i <= len(name)-len(kw); i++ {
				if name[i:i+len(kw)] == kw {
					return true
				}
			}
		}
	}
	return false
}

func detectorFormatHint(hint string) detector.Format {
	switch hint {
	case "json":
		return detector.FormatJSON
	case "text":
		return detector.FormatText
	default:
		return detector.FormatUnknown
	}
}
