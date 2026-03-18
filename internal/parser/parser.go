package parser

import (
	"context"
	"io"
	"springlog/pkg/logentry"
)

// Parser transforms a raw log stream into a channel of LogEntry.
type Parser interface {
	Parse(ctx context.Context, r io.Reader, source string) (<-chan *logentry.LogEntry, <-chan error)
}
