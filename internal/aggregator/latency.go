package aggregator

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"springlog/pkg/logentry"
)

// LatencyStats holds response time percentiles extracted from log messages.
type LatencyStats struct {
	Count        int64
	Min          float64
	Max          float64
	Mean         float64
	P50          float64
	P95          float64
	P99          float64
	SlowRequests []*SlowRequest
}

// SlowRequest is a single slow log entry exceeding the threshold.
type SlowRequest struct {
	DurationMs float64
	Message    string
	Logger     string
	Timestamp  string
	Project    string
}

// DBQueryStats tracks Hibernate/SQL query patterns.
type DBQueryStats struct {
	TotalQueries  int64
	UniqueQueries int64
	SlowQueries   []*SlowQuery
	TopQueries    []*QueryStat
}

type SlowQuery struct {
	SQL        string
	DurationMs float64
	Logger     string
}

type QueryStat struct {
	Template string
	Count    int64
	MaxMs    float64
}

// StartupStats contains Spring Boot startup analysis.
type StartupStats struct {
	TotalStartupMs float64
	Port           string
	Profile        string
	BeanCount      int
}

var (
	startupPattern  = regexp.MustCompile(`Started\s+\S+\s+in\s+([\d.]+)\s+seconds`)
	portPattern     = regexp.MustCompile(`Tomcat started on port\(?s?\)?[:\s]+(\d+)`)
	profilePattern  = regexp.MustCompile(`(?i)active profile[s]?[:\s]+([^\n]+)`)
	beanCountPattern = regexp.MustCompile(`(\d+)\s+beans`)
	sqlParamPattern  = regexp.MustCompile(`'[^']*'|\b\d+\b`)
)

const defaultSlowThresholdMs = 1000.0

// computeLatencyStats computes percentiles from duration values.
func computeLatencyStats(durations []float64, slowThresholdMs float64, entries []*logentry.LogEntry) *LatencyStats {
	if len(durations) == 0 {
		return nil
	}
	if slowThresholdMs <= 0 {
		slowThresholdMs = defaultSlowThresholdMs
	}

	sorted := make([]float64, len(durations))
	copy(sorted, durations)
	sort.Float64s(sorted)

	sum := 0.0
	for _, d := range sorted {
		sum += d
	}

	stats := &LatencyStats{
		Count: int64(len(sorted)),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Mean:  sum / float64(len(sorted)),
		P50:   percentile(sorted, 50),
		P95:   percentile(sorted, 95),
		P99:   percentile(sorted, 99),
	}

	for _, e := range entries {
		if e.DurationMs != nil && *e.DurationMs >= slowThresholdMs {
			ts := ""
			if !e.Timestamp.IsZero() {
				ts = e.Timestamp.Format("2006-01-02 15:04:05")
			}
			stats.SlowRequests = append(stats.SlowRequests, &SlowRequest{
				DurationMs: *e.DurationMs,
				Message:    e.Message,
				Logger:     e.Logger,
				Timestamp:  ts,
				Project:    e.Project,
			})
		}
	}
	sort.Slice(stats.SlowRequests, func(i, j int) bool {
		return stats.SlowRequests[i].DurationMs > stats.SlowRequests[j].DurationMs
	})
	if len(stats.SlowRequests) > 20 {
		stats.SlowRequests = stats.SlowRequests[:20]
	}

	return stats
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p / 100) * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// analyzeStartup extracts startup info from Spring Boot log entries.
func analyzeStartup(entries []*logentry.LogEntry) *StartupStats {
	stats := &StartupStats{}
	for _, e := range entries {
		msg := e.Message
		if m := startupPattern.FindStringSubmatch(msg); m != nil {
			if f, err := strconv.ParseFloat(m[1], 64); err == nil {
				stats.TotalStartupMs = f * 1000
			}
		}
		if m := portPattern.FindStringSubmatch(msg); m != nil {
			stats.Port = m[1]
		}
		if m := profilePattern.FindStringSubmatch(msg); m != nil {
			stats.Profile = strings.TrimSpace(m[1])
		}
		if m := beanCountPattern.FindStringSubmatch(msg); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > stats.BeanCount {
				stats.BeanCount = n
			}
		}
	}
	return stats
}

// normalizeSQL removes literal values from SQL for grouping.
func normalizeSQL(sql string) string {
	return strings.TrimSpace(sqlParamPattern.ReplaceAllString(sql, "?"))
}
