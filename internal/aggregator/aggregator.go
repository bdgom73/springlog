package aggregator

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"springlog/pkg/logentry"
)

// Stats is the result of aggregating a stream of log entries.
type Stats struct {
	Total         int64
	ByLevel       map[logentry.Level]int64
	ByProject     map[string]int64
	TopErrors     []*ErrorGroup
	TopExceptions []*ExceptionStat
	TopLoggers    []*LoggerStat
	Spikes        []*Spike
	TimeHistogram []*TimeBucket
	FirstSeen     time.Time
	LastSeen      time.Time
	ParseErrors   int64

	// 3단계
	Latency *LatencyStats
	Startup *StartupStats
}

// ErrorGroup clusters entries by normalized message fingerprint.
type ErrorGroup struct {
	Fingerprint string
	Count       int64
	Level       logentry.Level
	Examples    []*logentry.LogEntry
	FirstSeen   time.Time
	LastSeen    time.Time
}

// ExceptionStat tracks occurrences of a specific exception class.
type ExceptionStat struct {
	ClassName  string // e.g. "NullPointerException"
	FullName   string // e.g. "java.lang.NullPointerException"
	Count      int64
	Loggers    []string // which loggers threw this
	FirstSeen  time.Time
	LastSeen   time.Time
	Examples   []*logentry.LogEntry
}

// LoggerStat tracks error/warn counts per logger class.
type LoggerStat struct {
	Logger  string
	ByLevel map[logentry.Level]int64
	Total   int64
}

// Spike represents a time bucket where error rate significantly exceeded the average.
type Spike struct {
	Start      time.Time
	End        time.Time
	ErrorCount int64
	TotalCount int64
	AvgErrors  float64 // baseline average errors per bucket
	Multiplier float64 // how many times above average
}

// TimeBucket represents entry count within a time window.
type TimeBucket struct {
	Start   time.Time
	End     time.Time
	Count   int64
	ByLevel map[logentry.Level]int64
}

// Aggregator consumes a stream of entries and produces Stats.
type Aggregator struct {
	mu             sync.Mutex
	stats          *Stats
	errorGroups    map[string]*ErrorGroup
	exceptionStats map[string]*ExceptionStat
	loggerStats    map[string]*LoggerStat
	bucketMap      map[int64]*TimeBucket // keyed by bucket start unix
	bucketSize     time.Duration
	maxExamples    int
	maxGroups      int

	// 3단계
	durations       []float64
	slowRequests    []*logentry.LogEntry // entries exceeding slow threshold
	startupEntries  []*logentry.LogEntry // only first N entries for startup analysis
	slowThresholdMs float64
}

func New(bucketSize time.Duration) *Aggregator {
	if bucketSize == 0 {
		bucketSize = time.Hour
	}
	return &Aggregator{
		stats: &Stats{
			ByLevel:   make(map[logentry.Level]int64),
			ByProject: make(map[string]int64),
		},
		errorGroups:     make(map[string]*ErrorGroup),
		exceptionStats:  make(map[string]*ExceptionStat),
		loggerStats:     make(map[string]*LoggerStat),
		bucketMap:       make(map[int64]*TimeBucket),
		bucketSize:      bucketSize,
		maxExamples:     3,
		maxGroups:       100,
		slowThresholdMs: defaultSlowThresholdMs,
	}
}

// Ingest adds a single entry. Thread-safe.
func (a *Aggregator) Ingest(e *logentry.LogEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.stats.Total++
	a.stats.ByLevel[e.Level]++

	if e.Project != "" {
		a.stats.ByProject[e.Project]++
	}

	if !e.Timestamp.IsZero() {
		if a.stats.FirstSeen.IsZero() || e.Timestamp.Before(a.stats.FirstSeen) {
			a.stats.FirstSeen = e.Timestamp
		}
		if e.Timestamp.After(a.stats.LastSeen) {
			a.stats.LastSeen = e.Timestamp
		}
		a.ingestBucket(e)
	}

	if e.Level >= logentry.LevelWarn {
		a.ingestLoggerStat(e)
	}

	if e.Level >= logentry.LevelError {
		a.ingestErrorGroup(e)
		a.ingestExceptions(e)
	}

	// 3단계: latency & startup
	if e.DurationMs != nil {
		a.durations = append(a.durations, *e.DurationMs)
		if *e.DurationMs >= a.slowThresholdMs && len(a.slowRequests) < 100 {
			a.slowRequests = append(a.slowRequests, e)
		}
	}
	// Only keep early entries for startup analysis (first 500)
	if len(a.startupEntries) < 500 {
		a.startupEntries = append(a.startupEntries, e)
	}
}

func (a *Aggregator) ingestBucket(e *logentry.LogEntry) {
	bucketStart := e.Timestamp.Truncate(a.bucketSize)
	key := bucketStart.Unix()
	if b, ok := a.bucketMap[key]; ok {
		b.Count++
		b.ByLevel[e.Level]++
		return
	}
	b := &TimeBucket{
		Start:   bucketStart,
		End:     bucketStart.Add(a.bucketSize),
		Count:   1,
		ByLevel: map[logentry.Level]int64{e.Level: 1},
	}
	a.bucketMap[key] = b
	a.stats.TimeHistogram = append(a.stats.TimeHistogram, b)
}

func (a *Aggregator) ingestErrorGroup(e *logentry.LogEntry) {
	fp := fingerprint(e)
	if g, ok := a.errorGroups[fp]; ok {
		g.Count++
		if e.Timestamp.After(g.LastSeen) {
			g.LastSeen = e.Timestamp
		}
		if len(g.Examples) < a.maxExamples {
			g.Examples = append(g.Examples, e)
		}
		return
	}
	if len(a.errorGroups) >= a.maxGroups {
		return
	}
	a.errorGroups[fp] = &ErrorGroup{
		Fingerprint: fp,
		Count:       1,
		Level:       e.Level,
		Examples:    []*logentry.LogEntry{e},
		FirstSeen:   e.Timestamp,
		LastSeen:    e.Timestamp,
	}
}

// ingestExceptions extracts exception class names from stack traces.
func (a *Aggregator) ingestExceptions(e *logentry.LogEntry) {
	classes := extractExceptionClasses(e)
	seen := map[string]bool{}

	for _, cls := range classes {
		if seen[cls.full] {
			continue
		}
		seen[cls.full] = true

		if stat, ok := a.exceptionStats[cls.full]; ok {
			stat.Count++
			if e.Timestamp.After(stat.LastSeen) {
				stat.LastSeen = e.Timestamp
			}
			if len(stat.Examples) < a.maxExamples {
				stat.Examples = append(stat.Examples, e)
			}
			// track unique loggers
			hasLogger := false
			for _, l := range stat.Loggers {
				if l == e.Logger {
					hasLogger = true
					break
				}
			}
			if !hasLogger && e.Logger != "" {
				stat.Loggers = append(stat.Loggers, e.Logger)
			}
		} else {
			loggers := []string{}
			if e.Logger != "" {
				loggers = []string{e.Logger}
			}
			a.exceptionStats[cls.full] = &ExceptionStat{
				ClassName: cls.simple,
				FullName:  cls.full,
				Count:     1,
				Loggers:   loggers,
				FirstSeen: e.Timestamp,
				LastSeen:  e.Timestamp,
				Examples:  []*logentry.LogEntry{e},
			}
		}
	}
}

// ingestLoggerStat tracks error/warn counts per logger.
func (a *Aggregator) ingestLoggerStat(e *logentry.LogEntry) {
	if e.Logger == "" {
		return
	}
	if stat, ok := a.loggerStats[e.Logger]; ok {
		stat.ByLevel[e.Level]++
		stat.Total++
	} else {
		a.loggerStats[e.Logger] = &LoggerStat{
			Logger:  e.Logger,
			ByLevel: map[logentry.Level]int64{e.Level: 1},
			Total:   1,
		}
	}
}

// Finalize returns completed Stats sorted and ready for rendering.
func (a *Aggregator) Finalize(topN int) *Stats {
	a.mu.Lock()
	defer a.mu.Unlock()

	if topN <= 0 {
		topN = 10
	}

	// Sort error groups
	groups := make([]*ErrorGroup, 0, len(a.errorGroups))
	for _, g := range a.errorGroups {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	if len(groups) > topN {
		groups = groups[:topN]
	}
	a.stats.TopErrors = groups

	// Sort exception stats
	excs := make([]*ExceptionStat, 0, len(a.exceptionStats))
	for _, e := range a.exceptionStats {
		excs = append(excs, e)
	}
	sort.Slice(excs, func(i, j int) bool { return excs[i].Count > excs[j].Count })
	if len(excs) > topN {
		excs = excs[:topN]
	}
	a.stats.TopExceptions = excs

	// Sort logger stats by error count
	loggers := make([]*LoggerStat, 0, len(a.loggerStats))
	for _, l := range a.loggerStats {
		loggers = append(loggers, l)
	}
	sort.Slice(loggers, func(i, j int) bool {
		ei := loggers[i].ByLevel[logentry.LevelError] + loggers[i].ByLevel[logentry.LevelFatal]
		ej := loggers[j].ByLevel[logentry.LevelError] + loggers[j].ByLevel[logentry.LevelFatal]
		if ei != ej {
			return ei > ej
		}
		return loggers[i].Total > loggers[j].Total
	})
	if len(loggers) > topN {
		loggers = loggers[:topN]
	}
	a.stats.TopLoggers = loggers

	// Sort time histogram
	sort.Slice(a.stats.TimeHistogram, func(i, j int) bool {
		return a.stats.TimeHistogram[i].Start.Before(a.stats.TimeHistogram[j].Start)
	})

	// Spike detection
	a.stats.Spikes = detectSpikes(a.stats.TimeHistogram)

	// 3단계: latency
	a.stats.Latency = computeLatencyStats(a.durations, a.slowThresholdMs, a.slowRequests)

	// 3단계: startup
	a.stats.Startup = analyzeStartup(a.startupEntries)

	return a.stats
}

// detectSpikes finds buckets where error count is > 3x the average (min 3 errors).
func detectSpikes(buckets []*TimeBucket) []*Spike {
	if len(buckets) < 3 {
		return nil
	}

	var totalErrors float64
	for _, b := range buckets {
		totalErrors += float64(b.ByLevel[logentry.LevelError] + b.ByLevel[logentry.LevelFatal])
	}
	avgErrors := totalErrors / float64(len(buckets))
	if avgErrors < 0.1 {
		avgErrors = 0.1
	}

	threshold := avgErrors * 3
	if threshold < 3 {
		threshold = 3
	}

	var spikes []*Spike
	for _, b := range buckets {
		errs := b.ByLevel[logentry.LevelError] + b.ByLevel[logentry.LevelFatal]
		if float64(errs) >= threshold {
			spikes = append(spikes, &Spike{
				Start:      b.Start,
				End:        b.End,
				ErrorCount: errs,
				TotalCount: b.Count,
				AvgErrors:  avgErrors,
				Multiplier: float64(errs) / avgErrors,
			})
		}
	}
	return spikes
}

// --- Exception extraction ---

type exceptionClass struct {
	simple string // NullPointerException
	full   string // java.lang.NullPointerException
}

// javaExceptionPattern matches lines like:
//   java.lang.NullPointerException: message
//   org.springframework.dao.DataAccessException: ...
//   Caused by: java.net.SocketTimeoutException: ...
var javaExceptionPattern = regexp.MustCompile(
	`(?:Caused by:\s+)?([a-zA-Z][a-zA-Z0-9_]*(?:\.[a-zA-Z][a-zA-Z0-9_$]*)*Exception[a-zA-Z0-9_$]*)(?::\s+.*)?$`,
)

func extractExceptionClasses(e *logentry.LogEntry) []exceptionClass {
	var result []exceptionClass
	seen := map[string]bool{}

	extract := func(line string) {
		m := javaExceptionPattern.FindStringSubmatch(line)
		if m == nil {
			return
		}
		full := m[1]
		if seen[full] {
			return
		}
		seen[full] = true

		parts := strings.Split(full, ".")
		simple := parts[len(parts)-1]
		result = append(result, exceptionClass{simple: simple, full: full})
	}

	// Check message itself (some logs embed exception class in the message)
	extract(e.Message)

	// Parse stack trace lines
	for _, line := range e.StackTrace {
		extract(line)
	}

	return result
}

// --- Fingerprinting ---

var (
	rNumbers  = regexp.MustCompile(`\b\d+\b`)
	rUUIDs    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	rHex      = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	rFilePath = regexp.MustCompile(`[A-Za-z]:[\\\/][^\s]+|\/[^\s]+`)
)

func fingerprint(e *logentry.LogEntry) string {
	msg := e.Message
	msg = rUUIDs.ReplaceAllString(msg, "<UUID>")
	msg = rHex.ReplaceAllString(msg, "<HEX>")
	msg = rFilePath.ReplaceAllString(msg, "<PATH>")
	msg = rNumbers.ReplaceAllString(msg, "<N>")
	msg = strings.TrimSpace(msg)

	if len(e.StackTrace) > 0 {
		msg += "|" + strings.TrimSpace(e.StackTrace[0])
	}
	return e.Logger + "|" + msg
}
