package detector

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
)

// Format identifies the detected log format.
type Format int

const (
	FormatUnknown Format = iota
	FormatText           // Spring Boot log4j/slf4j pattern
	FormatJSON           // JSON lines
)

func (f Format) String() string {
	switch f {
	case FormatText:
		return "text"
	case FormatJSON:
		return "json"
	default:
		return "unknown"
	}
}

var springBootPattern = regexp.MustCompile(
	`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`,
)

const sampleSize = 8192

// Detect reads up to 8KB from r and returns the dominant log format.
// It replays the consumed bytes back so callers can re-read from the start.
func Detect(r io.Reader) (Format, io.Reader, error) {
	buf := make([]byte, sampleSize)
	n, err := io.ReadAtLeast(r, buf, 1)
	if err != nil && err != io.ErrUnexpectedEOF {
		return FormatUnknown, r, err
	}
	buf = buf[:n]

	// Stitch the peeked bytes back so the caller can re-read everything.
	combined := io.MultiReader(bytes.NewReader(buf), r)

	format := detectFormat(buf)
	return format, combined, nil
}

func detectFormat(sample []byte) Format {
	scanner := bufio.NewScanner(bytes.NewReader(sample))

	var jsonScore, textScore int

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		if line[0] == '{' && line[len(line)-1] == '}' {
			jsonScore++
		} else if springBootPattern.Match(line) {
			textScore++
		}
	}

	total := jsonScore + textScore
	if total == 0 {
		return FormatUnknown
	}

	if jsonScore > textScore {
		return FormatJSON
	}
	return FormatText
}
