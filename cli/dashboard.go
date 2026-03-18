package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"springlog/internal/aggregator"
	"springlog/internal/detector"
	"springlog/internal/parser"
	"springlog/pkg/logentry"
)

// ── Styles ───────────────────────────────────────────────────────────────────

var (
	styleTitle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleBorderBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	styleHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	styleActiveTab  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Underline(true)
	styleInactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleError      = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleWarn       = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleInfo       = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleFatal      = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Underline(true)
	styleOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSpike      = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleFilterBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1)
	styleFilterKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	styleFilterVal  = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236")).Padding(0, 1)
	styleFilterValActive = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("39")).Padding(0, 1)
	styleKey        = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(lipgloss.Color("236")).Padding(0, 1)
	styleSelected   = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("255"))
)

// ── Filter state ─────────────────────────────────────────────────────────────

type filterMode int

const (
	modeNormal filterMode = iota
	modeSearching
)

var levelCycle = []logentry.Level{
	logentry.LevelUnknown, // means ALL
	logentry.LevelDebug,
	logentry.LevelInfo,
	logentry.LevelWarn,
	logentry.LevelError,
	logentry.LevelFatal,
}

var levelLabels = map[logentry.Level]string{
	logentry.LevelUnknown: "ALL",
	logentry.LevelDebug:   "DEBUG",
	logentry.LevelInfo:    "INFO",
	logentry.LevelWarn:    "WARN",
	logentry.LevelError:   "ERROR",
	logentry.LevelFatal:   "FATAL",
}

type timePreset struct {
	label string
	from  time.Time
}

var timePresets = func() []timePreset {
	now := time.Now()
	y, m, d := now.Date()
	return []timePreset{
		{"ALL", time.Time{}},
		{"-1h", now.Add(-1 * time.Hour)},
		{"-6h", now.Add(-6 * time.Hour)},
		{"-24h", now.Add(-24 * time.Hour)},
		{"-7d", now.AddDate(0, 0, -7)},
		{"today", time.Date(y, m, d, 0, 0, 0, 0, now.Location())},
	}
}()

// ── Model ─────────────────────────────────────────────────────────────────────

type dashboardModel struct {
	// layout
	width  int
	height int
	tab    int // 0=summary 1=exceptions 2=latency 3=errors

	// data
	allEntries  []*logentry.LogEntry
	allProjects []string
	stats       *aggregator.Stats
	recentErrs  []*logentry.LogEntry
	errScroll   int

	// state
	loading  bool
	loadedAt time.Time
	path     string
	allProj  bool

	// filter state
	mode        filterMode
	searchInput textinput.Model
	levelIdx    int
	timeIdx     int
	projectIdx  int // 0 = ALL
}

// ── Messages ──────────────────────────────────────────────────────────────────

type entriesLoadedMsg struct {
	entries  []*logentry.LogEntry
	projects []string
	err      error
}

type statsComputedMsg struct {
	stats      *aggregator.Stats
	recentErrs []*logentry.LogEntry
}

type recomputeMsg struct{}

// ── Command ───────────────────────────────────────────────────────────────────

var dashboardFlags struct {
	AllProjects bool
	RefreshSec  int
}

var dashboardCmd = &cobra.Command{
	Use:   "dashboard [flags] [path...]",
	Short: "Interactive TUI dashboard with live filters",
	Long: `Full interactive terminal dashboard — filter, search, and analyze in real time.

All filters are applied interactively without restarting:

  /          Activate search (keyword or regex)
  Esc        Clear search / exit filter mode
  L          Cycle log level  (ALL → DEBUG → INFO → WARN → ERROR → FATAL)
  T          Cycle time range (ALL → -1h → -6h → -24h → -7d → today)
  P          Cycle project    (ALL → project-a → project-b → ...)
  Tab / ← →  Switch panels   (Summary / Exceptions / Latency / Errors)
  ↑ ↓        Scroll error list
  R          Reload from disk
  Q / Ctrl+C Quit

Examples:
  springlog dashboard ./logs/ --all-projects
  springlog dashboard ./logs/project-a/`,
	RunE: runDashboard,
}

func init() {
	dashboardCmd.Flags().BoolVar(&dashboardFlags.AllProjects, "all-projects", false, "Treat each subdirectory as a separate project")
	dashboardCmd.Flags().IntVar(&dashboardFlags.RefreshSec, "refresh", 0, "Auto-refresh interval in seconds")
}

func runDashboard(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	ti := textinput.New()
	ti.Placeholder = "keyword or /regex/"
	ti.CharLimit = 100

	m := dashboardModel{
		loading:     true,
		path:        strings.Join(args, ","),
		allProj:     dashboardFlags.AllProjects,
		searchInput: ti,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m dashboardModel) Init() tea.Cmd {
	return loadAllEntries(m.path, m.allProj)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case entriesLoadedMsg:
		if msg.err != nil {
			m.loading = false
			return m, nil
		}
		m.allEntries = msg.entries
		m.allProjects = msg.projects
		m.loading = false
		m.loadedAt = time.Now()
		return m, m.computeStats()

	case statsComputedMsg:
		m.stats = msg.stats
		m.recentErrs = msg.recentErrs

	case recomputeMsg:
		return m, m.computeStats()

	case tea.KeyMsg:
		// Search mode: route most keys to textinput
		if m.mode == modeSearching {
			switch msg.String() {
			case "esc":
				m.mode = modeNormal
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				return m, tea.Batch(m.computeStats(), textinput.Blink)
			case "enter":
				m.mode = modeNormal
				m.searchInput.Blur()
				return m, tea.Batch(m.computeStats(), textinput.Blink)
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, tea.Batch(cmd, func() tea.Msg { return recomputeMsg{} })
			}
		}

		// Normal mode
		switch strings.ToLower(msg.String()) {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "/":
			m.mode = modeSearching
			m.searchInput.Focus()
			return m, textinput.Blink

		case "esc":
			m.searchInput.SetValue("")
			m.levelIdx = 0
			m.timeIdx = 0
			m.projectIdx = 0
			m.errScroll = 0
			return m, m.computeStats()

		case "l":
			m.levelIdx = (m.levelIdx + 1) % len(levelCycle)
			m.errScroll = 0
			return m, m.computeStats()

		case "t":
			m.timeIdx = (m.timeIdx + 1) % len(timePresets)
			m.errScroll = 0
			return m, m.computeStats()

		case "p":
			total := len(m.allProjects) + 1 // +1 for ALL
			m.projectIdx = (m.projectIdx + 1) % total
			m.errScroll = 0
			return m, m.computeStats()

		case "tab", "right":
			m.tab = (m.tab + 1) % 4
			m.errScroll = 0
		case "left":
			m.tab = (m.tab + 3) % 4
			m.errScroll = 0
		case "1":
			m.tab = 0
		case "2":
			m.tab = 1
		case "3":
			m.tab = 2
		case "4":
			m.tab = 3

		case "up", "k":
			if m.errScroll > 0 {
				m.errScroll--
			}
		case "down", "j":
			m.errScroll++

		case "r":
			m.loading = true
			return m, loadAllEntries(m.path, m.allProj)
		}
	}

	return m, nil
}

// computeStats filters allEntries based on current filter state and recomputes.
func (m dashboardModel) computeStats() tea.Cmd {
	entries := m.allEntries
	levelMin := levelCycle[m.levelIdx]
	timeFrom := timePresets[m.timeIdx].from
	search := strings.ToLower(m.searchInput.Value())
	project := ""
	if m.projectIdx > 0 && m.projectIdx-1 < len(m.allProjects) {
		project = m.allProjects[m.projectIdx-1]
	}

	return func() tea.Msg {
		agg := aggregator.New(time.Hour)
		var recentErrs []*logentry.LogEntry

		for _, e := range entries {
			// Level filter
			if levelMin != logentry.LevelUnknown && e.Level < levelMin {
				continue
			}
			// Time filter
			if !timeFrom.IsZero() && !e.Timestamp.IsZero() && e.Timestamp.Before(timeFrom) {
				continue
			}
			// Project filter
			if project != "" && !strings.EqualFold(e.Project, project) {
				continue
			}
			// Search filter
			if search != "" {
				hay := strings.ToLower(e.Message + " " + e.Logger + " " + e.Raw)
				if !strings.Contains(hay, search) {
					continue
				}
			}

			agg.Ingest(e)
			if e.Level >= logentry.LevelError {
				recentErrs = append(recentErrs, e)
			}
		}

		// Newest errors first
		sort.Slice(recentErrs, func(i, j int) bool {
			return recentErrs[i].Timestamp.After(recentErrs[j].Timestamp)
		})
		if len(recentErrs) > 500 {
			recentErrs = recentErrs[:500]
		}

		return statsComputedMsg{
			stats:      agg.Finalize(10),
			recentErrs: recentErrs,
		}
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m dashboardModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sections []string

	sections = append(sections, m.viewHeader())
	sections = append(sections, m.viewFilterBar())
	sections = append(sections, m.viewTabBar())

	if m.loading {
		sections = append(sections, styleInfo.Render("\n  ⟳  Loading log data, please wait...\n"))
	} else if m.stats == nil {
		sections = append(sections, styleWarn.Render("\n  No data.\n"))
	} else {
		reservedLines := 8 // header + filterbar + tabbar + helpbar
		contentH := m.height - reservedLines
		if contentH < 5 {
			contentH = 5
		}
		switch m.tab {
		case 0:
			sections = append(sections, m.viewSummary(contentH))
		case 1:
			sections = append(sections, m.viewExceptions(contentH))
		case 2:
			sections = append(sections, m.viewLatency(contentH))
		case 3:
			sections = append(sections, m.viewErrors(contentH))
		}
	}

	sections = append(sections, m.viewHelp())
	return strings.Join(sections, "\n")
}

// ── Header ────────────────────────────────────────────────────────────────────

func (m dashboardModel) viewHeader() string {
	title := styleTitle.Render("⚡ springlog dashboard")
	path := styleDim.Render("  " + m.path)
	age := ""
	if !m.loadedAt.IsZero() {
		age = styleDim.Render(fmt.Sprintf("  refreshed %s ago", time.Since(m.loadedAt).Round(time.Second)))
	}
	total := ""
	if m.stats != nil {
		total = styleDim.Render(fmt.Sprintf("  %d entries", m.stats.Total))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, title, path, total, age)
}

// ── Filter bar ────────────────────────────────────────────────────────────────

func (m dashboardModel) viewFilterBar() string {
	// Search
	searchLabel := styleFilterKey.Render("/ Search:")
	var searchVal string
	if m.mode == modeSearching {
		searchVal = styleFilterValActive.Render(m.searchInput.View())
	} else {
		v := m.searchInput.Value()
		if v == "" {
			v = "─"
		}
		searchVal = styleFilterVal.Render(v)
	}

	// Level
	lvl := levelCycle[m.levelIdx]
	lvlLabel := styleFilterKey.Render("L Level:")
	lvlVal := styleFilterVal.Render(levelLabel(lvl))

	// Time
	tp := timePresets[m.timeIdx]
	timeLabel := styleFilterKey.Render("T Time:")
	timeVal := styleFilterVal.Render(tp.label)

	// Project
	proj := "ALL"
	if m.projectIdx > 0 && m.projectIdx-1 < len(m.allProjects) {
		proj = m.allProjects[m.projectIdx-1]
	}
	projLabel := styleFilterKey.Render("P Project:")
	projVal := styleFilterVal.Render(proj)

	line := fmt.Sprintf("  %s %s   %s %s   %s %s   %s %s",
		searchLabel, searchVal,
		lvlLabel, lvlVal,
		timeLabel, timeVal,
		projLabel, projVal,
	)

	return styleFilterBox.Width(m.width - 4).Render(line)
}

// ── Tab bar ───────────────────────────────────────────────────────────────────

func (m dashboardModel) viewTabBar() string {
	tabs := []string{"[1] Summary", "[2] Exceptions", "[3] Latency", "[4] Errors"}
	var parts []string
	for i, t := range tabs {
		if i == m.tab {
			parts = append(parts, styleActiveTab.Render(" "+t+" "))
		} else {
			parts = append(parts, styleInactiveTab.Render(" "+t+" "))
		}
	}
	return strings.Join(parts, styleDim.Render(" │ "))
}

// ── Help bar ──────────────────────────────────────────────────────────────────

func (m dashboardModel) viewHelp() string {
	keys := []string{"/ search", "L level", "T time", "P project", "Esc reset", "Tab panel", "↑↓ scroll", "R reload", "Q quit"}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = styleKey.Render(k)
	}
	return strings.Join(parts, " ")
}

// ── Panel: Summary ────────────────────────────────────────────────────────────

func (m dashboardModel) viewSummary(height int) string {
	s := m.stats
	var sb strings.Builder

	// Level breakdown
	sb.WriteString(styleHeader.Render("  📊 Level Breakdown") + "\n")
	total := s.Total
	if total == 0 {
		sb.WriteString(styleDim.Render("  (no entries match current filters)\n"))
	}
	for _, lvl := range []logentry.Level{logentry.LevelFatal, logentry.LevelError, logentry.LevelWarn, logentry.LevelInfo, logentry.LevelDebug, logentry.LevelTrace} {
		cnt := s.ByLevel[lvl]
		if cnt == 0 {
			continue
		}
		pct := float64(cnt) / float64(total) * 100
		bar := miniBar(cnt, total, 20)
		label := levelStyled(lvl).Render(fmt.Sprintf("%-7s", lvl))
		sb.WriteString(fmt.Sprintf("  %s %s %5d  %5.1f%%\n", label, bar, cnt, pct))
	}

	// Time range
	if !s.FirstSeen.IsZero() {
		sb.WriteString("\n" + styleHeader.Render("  🕐 Time Range") + "\n")
		sb.WriteString(fmt.Sprintf("  From : %s\n", s.FirstSeen.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("  To   : %s\n", s.LastSeen.Format("2006-01-02 15:04:05")))
		dur := s.LastSeen.Sub(s.FirstSeen).Round(time.Second)
		sb.WriteString(fmt.Sprintf("  Span : %s\n", dur))
	}

	// By project
	if len(s.ByProject) > 1 {
		sb.WriteString("\n" + styleHeader.Render("  📁 By Project") + "\n")
		type ps struct {
			name string
			cnt  int64
		}
		var projs []ps
		for k, v := range s.ByProject {
			projs = append(projs, ps{k, v})
		}
		sort.Slice(projs, func(i, j int) bool { return projs[i].cnt > projs[j].cnt })
		for _, p := range projs {
			bar := miniBar(p.cnt, total, 20)
			sb.WriteString(fmt.Sprintf("  %-20s %s %d\n", p.name, bar, p.cnt))
		}
	}

	// Spikes
	if len(s.Spikes) > 0 {
		sb.WriteString("\n" + styleSpike.Render("  ⚠ Error Spikes Detected") + "\n")
		for _, sp := range s.Spikes {
			sb.WriteString(styleSpike.Render(fmt.Sprintf("  %s — %d errors (%.1fx avg)\n",
				sp.Start.Format("01-02 15:04"), sp.ErrorCount, sp.Multiplier)))
		}
	}

	// Histogram (last 12 buckets)
	if len(s.TimeHistogram) > 0 {
		sb.WriteString("\n" + styleHeader.Render("  📈 Error Rate by Hour") + "\n")
		spikeSet := map[string]bool{}
		for _, sp := range s.Spikes {
			spikeSet[sp.Start.Format("01-02 15:04")] = true
		}
		var maxErr int64
		for _, b := range s.TimeHistogram {
			e := b.ByLevel[logentry.LevelError] + b.ByLevel[logentry.LevelFatal]
			if e > maxErr {
				maxErr = e
			}
		}
		buckets := s.TimeHistogram
		if len(buckets) > 12 {
			buckets = buckets[len(buckets)-12:]
		}
		for _, b := range buckets {
			errs := b.ByLevel[logentry.LevelError] + b.ByLevel[logentry.LevelFatal]
			label := b.Start.Format("01-02 15:04")
			barLen := 0
			if maxErr > 0 {
				barLen = int(math.Round(float64(errs) / float64(maxErr) * 20))
			}
			bar := strings.Repeat("█", barLen) + strings.Repeat("░", 20-barLen)
			spikeMark := ""
			if spikeSet[label] {
				spikeMark = styleSpike.Render(" ⚠")
			}
			errStr := "-"
			if errs > 0 {
				errStr = styleError.Render(fmt.Sprintf("%d", errs))
			}
			sb.WriteString(fmt.Sprintf("  %s │%s│ %s err%s\n", label, bar, errStr, spikeMark))
		}
	}

	return styleBorderBox.Width(m.width - 4).Render(sb.String())
}

// ── Panel: Exceptions ─────────────────────────────────────────────────────────

func (m dashboardModel) viewExceptions(height int) string {
	s := m.stats
	var sb strings.Builder

	sb.WriteString(styleHeader.Render("  🔥 Exception Types") + "\n\n")
	if len(s.TopExceptions) == 0 {
		sb.WriteString(styleOK.Render("  ✓ No exceptions match current filters\n"))
	} else {
		var maxCount int64
		for _, ex := range s.TopExceptions {
			if ex.Count > maxCount {
				maxCount = ex.Count
			}
		}
		for i, ex := range s.TopExceptions {
			bar := miniBar(ex.Count, maxCount, 15)
			cls := styleWarn.Render(ex.ClassName)
			if ex.Count >= 3 {
				cls = styleError.Render(ex.ClassName)
			}
			logger := ""
			if len(ex.Loggers) > 0 {
				logger = styleDim.Render("  ← " + shortLogger(ex.Loggers[0]))
			}
			lastSeen := ""
			if !ex.LastSeen.IsZero() {
				lastSeen = styleDim.Render("  " + ex.LastSeen.Format("01-02 15:04:05"))
			}
			sb.WriteString(fmt.Sprintf("  %2d. %s %s %dx%s%s\n", i+1, cls, bar, ex.Count, lastSeen, logger))
		}
	}

	sb.WriteString("\n" + styleHeader.Render("  🏆 Most Error-Prone Classes") + "\n\n")
	if len(s.TopLoggers) == 0 {
		sb.WriteString(styleOK.Render("  ✓ No error-generating classes\n"))
	} else {
		for i, l := range s.TopLoggers {
			fatal := l.ByLevel[logentry.LevelFatal]
			errs := l.ByLevel[logentry.LevelError]
			warn := l.ByLevel[logentry.LevelWarn]
			var counts []string
			if fatal > 0 {
				counts = append(counts, styleFatal.Render(fmt.Sprintf("FATAL:%d", fatal)))
			}
			if errs > 0 {
				counts = append(counts, styleError.Render(fmt.Sprintf("ERR:%d", errs)))
			}
			if warn > 0 {
				counts = append(counts, styleWarn.Render(fmt.Sprintf("WARN:%d", warn)))
			}
			sb.WriteString(fmt.Sprintf("  %2d. %-38s %s\n", i+1, shortLogger(l.Logger), strings.Join(counts, "  ")))
		}
	}

	if len(s.Spikes) > 0 {
		sb.WriteString("\n" + styleSpike.Render("  ⚠ Spike Alerts") + "\n\n")
		for _, sp := range s.Spikes {
			sb.WriteString(styleSpike.Render(fmt.Sprintf("  %s  %d errors  %.1fx above avg (%.1f/bucket)\n",
				sp.Start.Format("2006-01-02 15:04"), sp.ErrorCount, sp.Multiplier, sp.AvgErrors)))
		}
	}

	return styleBorderBox.Width(m.width - 4).Render(sb.String())
}

// ── Panel: Latency ────────────────────────────────────────────────────────────

func (m dashboardModel) viewLatency(height int) string {
	s := m.stats
	var sb strings.Builder

	if s.Startup != nil && s.Startup.TotalStartupMs > 0 {
		sb.WriteString(styleHeader.Render("  🚀 Spring Boot Startup") + "\n\n")
		sb.WriteString(fmt.Sprintf("  Startup time : %s\n", formatMs(s.Startup.TotalStartupMs)))
		if s.Startup.Port != "" {
			sb.WriteString(fmt.Sprintf("  Port         : %s\n", s.Startup.Port))
		}
		if s.Startup.Profile != "" {
			sb.WriteString(fmt.Sprintf("  Profile      : %s\n", s.Startup.Profile))
		}
		if s.Startup.BeanCount > 0 {
			sb.WriteString(fmt.Sprintf("  Beans loaded : %d\n", s.Startup.BeanCount))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(styleHeader.Render("  ⏱ Response Time Percentiles") + "\n\n")
	if s.Latency == nil || s.Latency.Count == 0 {
		sb.WriteString(styleDim.Render("  No timing data in filtered entries.\n"))
		sb.WriteString(styleDim.Render("  Tip: log messages containing durations like '342ms' or '1.2s' are parsed automatically.\n"))
	} else {
		l := s.Latency
		rows := [][2]string{
			{"Count", fmt.Sprintf("%d requests", l.Count)},
			{"Min", formatMs(l.Min)},
			{"Mean", formatMs(l.Mean)},
			{"p50 (median)", formatMs(l.P50)},
			{"p95", formatMsColored(l.P95)},
			{"p99", formatMsColored(l.P99)},
			{"Max", formatMsColored(l.Max)},
		}
		for _, row := range rows {
			sb.WriteString(fmt.Sprintf("  %-16s %s\n", row[0], row[1]))
		}

		if len(l.SlowRequests) > 0 {
			sb.WriteString("\n" + styleHeader.Render("  🐌 Slow Requests (≥1000ms)") + "\n\n")
			limit := 10
			if limit > len(l.SlowRequests) {
				limit = len(l.SlowRequests)
			}
			for _, sr := range l.SlowRequests[:limit] {
				msg := sr.Message
				maxLen := m.width - 55
				if maxLen > 0 && len(msg) > maxLen {
					msg = msg[:maxLen] + "…"
				}
				ts := sr.Timestamp
				if len(ts) > 19 {
					ts = ts[11:19] // HH:MM:SS
				}
				sb.WriteString(fmt.Sprintf("  %s  %s  %s\n",
					styleError.Render(fmt.Sprintf("%7.0fms", sr.DurationMs)),
					styleDim.Render(ts),
					msg,
				))
			}
		}
	}

	return styleBorderBox.Width(m.width - 4).Render(sb.String())
}

// ── Panel: Errors ─────────────────────────────────────────────────────────────

func (m dashboardModel) viewErrors(height int) string {
	var sb strings.Builder

	total := len(m.recentErrs)
	sb.WriteString(styleHeader.Render(fmt.Sprintf("  ❌ Errors / Fatal (%d)", total)) + "\n\n")

	if total == 0 {
		sb.WriteString(styleOK.Render("  ✓ No errors match current filters\n"))
		return styleBorderBox.Width(m.width - 4).Render(sb.String())
	}

	entryH := 2 // entry line + optional stack line
	maxVisible := (height - 6) / entryH
	if maxVisible < 2 {
		maxVisible = 2
	}

	start := m.errScroll
	if start >= total {
		start = total - 1
	}
	end := start + maxVisible
	if end > total {
		end = total
	}

	for _, e := range m.recentErrs[start:end] {
		ts := ""
		if !e.Timestamp.IsZero() {
			ts = e.Timestamp.Format("01-02 15:04:05")
		}
		lvlStr := levelStyled(e.Level).Render(fmt.Sprintf("%-5s", e.Level))
		project := ""
		if e.Project != "" {
			project = styleDim.Render(fmt.Sprintf("[%s] ", e.Project))
		}
		trace := ""
		if e.TraceID != "" {
			trace = styleDim.Render(" t:" + e.TraceID[:8] + "…")
		}
		msg := e.Message
		maxLen := m.width - 55
		if maxLen > 0 && len(msg) > maxLen {
			msg = msg[:maxLen] + "…"
		}
		sb.WriteString(fmt.Sprintf("  %s %s %s%s%s : %s\n",
			styleDim.Render(ts), lvlStr, project,
			styleDim.Render(shortLogger(e.Logger)), trace, msg))

		// First stack trace line
		if len(e.StackTrace) > 0 {
			line := strings.TrimSpace(e.StackTrace[0])
			if len(line) > m.width-8 {
				line = line[:m.width-8]
			}
			sb.WriteString("    " + styleDim.Render(line) + "\n")
		}
	}

	if total > maxVisible {
		pct := int(float64(start+maxVisible) / float64(total) * 100)
		sb.WriteString(styleDim.Render(fmt.Sprintf("\n  %d–%d of %d  (%d%%)  ↑↓ to scroll\n",
			start+1, end, total, pct)))
	}

	return styleBorderBox.Width(m.width - 4).Render(sb.String())
}

// ── Data loading ──────────────────────────────────────────────────────────────

func loadAllEntries(path string, allProjects bool) tea.Cmd {
	return func() tea.Msg {
		paths := strings.Split(path, ",")
		ctx := context.Background()

		var allEntries []*logentry.LogEntry
		projectSet := map[string]bool{}

		for _, p := range paths {
			files, err := collectLogFiles(strings.TrimSpace(p), allProjects)
			if err != nil {
				return entriesLoadedMsg{err: err}
			}
			for _, lf := range files {
				f, err := os.Open(lf.Path)
				if err != nil {
					continue
				}
				format, reader, err := detector.Detect(f)
				if err != nil {
					f.Close()
					continue
				}
				var par parser.Parser
				if format == detector.FormatJSON {
					par = parser.NewJSONParser()
				} else {
					par = parser.NewTextParser()
				}
				entCh, _ := par.Parse(ctx, reader, lf.Path)
				for e := range entCh {
					e.Project = lf.Project
					allEntries = append(allEntries, e)
					if lf.Project != "" {
						projectSet[lf.Project] = true
					}
				}
				f.Close()
			}
		}

		// Collect sorted project list
		var projects []string
		for p := range projectSet {
			projects = append(projects, p)
		}
		sort.Strings(projects)

		return entriesLoadedMsg{entries: allEntries, projects: projects}
	}
}

// ── Util ──────────────────────────────────────────────────────────────────────

func miniBar(val, max int64, width int) string {
	if max == 0 {
		return strings.Repeat("░", width)
	}
	filled := int(math.Round(float64(val) / float64(max) * float64(width)))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func levelStyled(l logentry.Level) lipgloss.Style {
	switch l {
	case logentry.LevelFatal:
		return styleFatal
	case logentry.LevelError:
		return styleError
	case logentry.LevelWarn:
		return styleWarn
	case logentry.LevelInfo:
		return styleInfo
	default:
		return styleDim
	}
}

func levelLabel(l logentry.Level) string {
	if s, ok := levelLabels[l]; ok {
		return s
	}
	return "ALL"
}

func formatMs(ms float64) string {
	if ms >= 1000 {
		return fmt.Sprintf("%.2fs", ms/1000)
	}
	return fmt.Sprintf("%.0fms", ms)
}

func formatMsColored(ms float64) string {
	s := formatMs(ms)
	switch {
	case ms >= 3000:
		return styleFatal.Render(s)
	case ms >= 1000:
		return styleError.Render(s)
	case ms >= 500:
		return styleWarn.Render(s)
	default:
		return styleOK.Render(s)
	}
}
