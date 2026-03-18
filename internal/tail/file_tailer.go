package tail

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"springlog/internal/detector"
	"springlog/internal/filter"
	"springlog/internal/parser"
	"springlog/pkg/logentry"
)

// FileTailer watches a log file and streams new entries in real time.
type FileTailer struct {
	Filters filter.Chain
}

func New(filters filter.Chain) *FileTailer {
	return &FileTailer{Filters: filters}
}

// Tail streams new log entries from the given file path.
// It handles log rotation (file rename/remove → reopen).
func (t *FileTailer) Tail(ctx context.Context, path string) (<-chan *logentry.LogEntry, <-chan error) {
	entries := make(chan *logentry.LogEntry, 256)
	errs := make(chan error, 16)

	go func() {
		defer close(entries)
		defer close(errs)

		f, offset, err := openAtEnd(path)
		if err != nil {
			errs <- err
			return
		}
		defer f.Close()

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			errs <- err
			return
		}
		defer watcher.Close()

		if err := watcher.Add(path); err != nil {
			errs <- err
			return
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		readNew := func() {
			f.Seek(offset, io.SeekStart)
			format, reader, _ := detector.Detect(f)
			var p parser.Parser
			switch format {
			case detector.FormatJSON:
				p = parser.NewJSONParser()
			default:
				p = parser.NewTextParser()
			}

			subCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			entCh, _ := p.Parse(subCtx, reader, path)
			for e := range entCh {
				if t.Filters.Match(e) {
					select {
					case entries <- e:
					case <-ctx.Done():
						return
					}
				}
			}

			pos, _ := f.Seek(0, io.SeekCurrent)
			offset = pos
		}

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					readNew()
				}
				if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					// Log rotation — wait briefly and reopen
					time.Sleep(200 * time.Millisecond)
					f.Close()
					newF, newOffset, err := openAtStart(path)
					if err != nil {
						errs <- err
						return
					}
					f = newF
					offset = newOffset
					watcher.Add(path)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				errs <- err

			case <-ticker.C:
				// Polling fallback for network filesystems
				readNew()
			}
		}
	}()

	return entries, errs
}

func openAtEnd(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, offset, nil
}

func openAtStart(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	return f, 0, nil
}
