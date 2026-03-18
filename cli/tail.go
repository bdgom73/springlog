package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"springlog/internal/renderer"
	"springlog/internal/tail"
)

var tailCmd = &cobra.Command{
	Use:   "tail [flags] file",
	Short: "Watch a log file in real time",
	Long: `Tail a log file and stream new entries as they appear.
Handles log rotation automatically.

Examples:
  springlog tail ./logs/project-a/app.log
  springlog tail ./logs/project-a/app.log -l ERROR
  springlog tail ./logs/project-a/app.log --search "Exception"`,
	Args: cobra.ExactArgs(1),
	RunE: runTail,
}

func runTail(cmd *cobra.Command, args []string) error {
	path := args[0]

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}

	filters, err := buildFilters()
	if err != nil {
		return err
	}

	opts := buildRendererOptions()
	rend := renderer.New(renderer.Format(globalFlags.Output), opts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nStopping tail...")
		cancel()
	}()

	fmt.Fprintf(os.Stderr, "Tailing %s (Ctrl+C to stop)\n\n", path)

	tailer := tail.New(filters)
	entCh, errCh := tailer.Tail(ctx, path)

	for {
		select {
		case e, ok := <-entCh:
			if !ok {
				return nil
			}
			if err := rend.RenderEntry(os.Stdout, e); err != nil {
				return err
			}

		case err, ok := <-errCh:
			if !ok {
				continue
			}
			fmt.Fprintf(os.Stderr, "warn: %v\n", err)

		case <-ctx.Done():
			return nil
		}
	}
}
