package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/barbera01/HerbHub365/wateringtui/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// API types & calls
// ---------------------------------------------------------------------------

type channelStatus struct {
	Pin       int  `json:"pin"`
	On        bool `json:"on"`
	JobActive bool `json:"jobActive"`
}

type statusResponse struct {
	Chip     string                   `json:"chip"`
	Channels map[string]channelStatus `json:"channels"`
}

type (
	statusMsg   statusResponse
	errMsg      struct{ err error }
	actionOKMsg struct{ note string }
	tickMsg     time.Time
)

func fetchStatus(host string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(host + "/status")
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()
		var s statusResponse
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return errMsg{err}
		}
		return statusMsg(s)
	}
}

func postAction(host, path string, query url.Values, note string) tea.Cmd {
	return func() tea.Msg {
		u := host + path
		if query != nil {
			u += "?" + query.Encode()
		}
		resp, err := http.Post(u, "", nil)
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			var e struct {
				Error string `json:"error"`
			}
			json.NewDecoder(resp.Body).Decode(&e)
			return errMsg{fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Error)}
		}
		return actionOKMsg{note}
	}
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type inputMode int

const (
	modeNormal inputMode = iota
	modePulse            // typing pulse seconds for the selected channel
)

// section tracks which TUI screen is active.
type section int

const (
	secChannels section = iota
	secAdd
	secAdmin
)

type model struct {
	host         string
	pollInterval int
	config       *config.RootConfig

	section      section
	cursor       int
	mode         inputMode
	input        string
	addName      string
	addPin       string
	adminCursor  int
	adminActions []string

	names    []string // sorted channel names
	channels map[string]channelStatus
	note     string // last action / error line
	noteErr  bool
}

// NewModel creates the watering terminal UI model.
func NewModel(host string, cfg *config.RootConfig) tea.Model {
	channels := map[string]channelStatus{}
	if cfg != nil {
		for _, ch := range cfg.Channels.Entries {
			channels[ch.Name] = channelStatus{Pin: ch.Pin}
		}
	}
	return model{
		host:         host,
		pollInterval: cfg.Default.PollInterval,
		config:       cfg,
		channels:     channels,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchStatus(m.host), tea.Tick(time.Duration(m.pollInterval)*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case statusMsg:
		m.channels = msg.Channels
		names := make([]string, 0, len(msg.Channels))
		for n := range msg.Channels {
			names = append(names, n)
		}
		sort.Strings(names)
		m.names = names
		if m.cursor >= len(names) {
			m.cursor = 0
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(fetchStatus(m.host), tea.Tick(time.Duration(m.pollInterval)*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }))

	case actionOKMsg:
		m.note, m.noteErr = msg.note, false
		return m, fetchStatus(m.host)

	case errMsg:
		m.note, m.noteErr = msg.err.Error(), true
		return m, nil

	case tea.KeyMsg:
		if m.mode == modePulse {
			return m.updatePulseInput(msg)
		}
		switch m.section {
		case secAdd:
			return m.updateAdd(msg)
		case secAdmin:
			return m.updateAdmin(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.section == secChannels {
			if m.cursor > 0 {
				m.cursor--
			}
		}
	case "down", "j":
		if m.section == secChannels {
			if m.cursor < len(m.names)-1 {
				m.cursor++
			}
		}
	case "enter", " ":
		if m.section == secChannels {
			if ch, ok := m.selected(); ok {
				action := "on"
				if m.channels[ch].On {
					action = "off"
				}
				return m, postAction(m.host, "/relay/"+url.PathEscape(ch)+"/"+action, nil, ch+" -> "+action)
			}
		}
	case "p":
		if m.section == secChannels {
			if _, ok := m.selected(); ok {
				m.mode = modePulse
				m.input = ""
			}
		}
	case "a":
		// Switch to Add section
		m.section = secAdd
		m.addName = ""
		m.addPin = ""
		m.cursor = 0 // reset channel cursor
	case "A":
		// Switch to Admin section
		m.section = secAdmin
		m.adminCursor = 0
		m.adminActions = m.config.Admin.Actions
	case "r":
		return m, fetchStatus(m.host)
	case "back":
		// Go back to channels section
		m.section = secChannels
		m.cursor = 0
	}
	return m, nil
}

func (m model) updateAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.section = secChannels
		return m, nil
	case "back":
		m.section = secChannels
		return m, nil
	case "enter":
		// Validate name
		if m.addName == "" {
			m.note, m.noteErr = "channel name required", true
			return m, nil
		}
		// Validate pin
		pin, err := strconv.Atoi(m.addPin)
		if err != nil || pin <= 0 {
			m.note, m.noteErr = "invalid pin number", true
			return m, nil
		}
		// Check if channel already exists
		if _, exists := m.channels[m.addName]; exists {
			m.note, m.noteErr = "channel already exists: "+m.addName, true
			return m, nil
		}
		// Add channel locally
		m.channels[m.addName] = channelStatus{Pin: pin}
		m.note, m.noteErr = "added "+m.addName+" (pin "+fmt.Sprint(pin)+")", false
		m.addName = ""
		m.addPin = ""
		// Refresh to get live status
		return m, fetchStatus(m.host)
	case "tab":
		// Move to pin input
		if m.addName != "" && m.addPin == "" {
			return m, nil
		}
	case "up", "k":
		// Move back to name input
		if m.addPin != "" {
			m.addPin = ""
		}
	case "down", "j":
		// Move to pin input
		if m.addName != "" && m.addPin == "" {
			return m, nil
		}
	case "backspace":
		if len(m.addPin) > 0 {
			m.addPin = m.addPin[:len(m.addPin)-1]
		} else if len(m.addName) > 0 {
			m.addName = m.addName[:len(m.addName)-1]
		}
	default:
		// Digits for pin, letters for name
		s := msg.String()
		if len(s) == 1 && (s[0] >= '0' && s[0] <= '9') {
			m.addPin += s
		} else if len(s) == 1 && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z') {
			m.addName += s
		}
	}
	return m, nil
}

func (m model) updateAdmin(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "back":
		m.section = secChannels
		m.cursor = 0
		return m, nil
	case "enter":
		if m.adminCursor < len(m.adminActions) {
			action := m.adminActions[m.adminCursor]
			switch action {
			case "reset_all":
				m.note, m.noteErr = "resetting all channels...", false
				return m, postAction(m.host, "/all/reset", nil, "reset_all")
			case "ping":
				m.note, m.noteErr = "pinging...", false
				return m, postAction(m.host, "/admin/ping", nil, "ping")
			case "version":
				m.note, m.noteErr = "checking version...", false
				return m, postAction(m.host, "/admin/version", nil, "version")
			}
		}
	case "up", "k":
		if m.adminCursor > 0 {
			m.adminCursor--
		}
	case "down", "j":
		if m.adminCursor < len(m.adminActions)-1 {
			m.adminCursor++
		}
	}
	return m, nil
}

func (m model) updatePulseInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
	case "enter":
		ch, _ := m.selected()
		secs, err := strconv.ParseFloat(m.input, 64)
		if err != nil || secs <= 0 {
			m.note, m.noteErr = "invalid seconds", true
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeNormal
		return m, postAction(m.host, "/relay/"+url.PathEscape(ch)+"/pulse",
			url.Values{"seconds": {m.input}},
			fmt.Sprintf("pulse %s %.4gs", ch, secs))
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		// digits and a single decimal point only
		s := msg.String()
		if len(s) == 1 && (s[0] >= '0' && s[0] <= '9' || s == ".") {
			m.input += s
		}
	}
	return m, nil
}

func (m model) selected() (string, bool) {
	if m.cursor < len(m.names) {
		return m.names[m.cursor], true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	onStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	jobStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	noteStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m model) View() string {
	title := titleStyle.Render("🌿 HerbHub watering client") + noteStyle.Render("  "+m.host)
	switch m.section {
	case secChannels:
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("227")).Bold(true).Render(" [Channels]")
	case secAdd:
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render(" [Add Channel]")
	case secAdmin:
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(" [Admin]")
	}
	s := title + "\n\n"

	// Add section
	if m.section == secAdd {
		s += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("176")).Render("Channel name: ") + m.addName + "\n"
		s += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("176")).Render("Pin number:   ") + m.addPin + "\n"
		s += "\n"
		if m.note != "" {
			if m.noteErr {
				s += errStyle.Render("✗ "+m.note) + "\n"
			} else {
				s += noteStyle.Render("✓ "+m.note) + "\n"
			}
		}
		s += helpStyle.Render("enter: confirm • esc: cancel • tab: switch input") + "\n"
		return s
	}

	// Admin section
	if m.section == secAdmin {
		if m.note != "" {
			if m.noteErr {
				s += errStyle.Render("✗ "+m.note) + "\n"
			} else {
				s += noteStyle.Render("✓ "+m.note) + "\n"
			}
		}
		for i, action := range m.adminActions {
			cursor := "  "
			if i == m.adminCursor {
				cursor = cursorStyle.Render("> ")
			}
			s += fmt.Sprintf("%s%s\n", cursor, action)
		}
		s += "\n"
		s += helpStyle.Render("↑/↓: select • enter: execute • esc/back: return") + "\n"
		return s
	}

	// Channels section (default)
	if len(m.names) == 0 {
		s += noteStyle.Render("  loading status...") + "\n"
	}
	for i, name := range m.names {
		ch := m.channels[name]
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}
		state := offStyle.Render("○ off")
		if ch.On {
			state = onStyle.Render("● ON ")
		}
		job := ""
		if ch.JobActive {
			job = jobStyle.Render("  [job running]")
		}
		s += fmt.Sprintf("%s%-9s %s  pin %-2d%s\n", cursor, name, state, ch.Pin, job)
	}

	s += "\n"
	if m.mode == modePulse {
		ch, _ := m.selected()
		s += fmt.Sprintf("pulse %s for how many seconds? %s█\n", ch, m.input)
		s += helpStyle.Render("enter: run • esc: cancel") + "\n"
	} else {
		if m.note != "" {
			if m.noteErr {
				s += errStyle.Render("✗ "+m.note) + "\n"
			} else {
				s += noteStyle.Render("✓ "+m.note) + "\n"
			}
		}
		s += helpStyle.Render("↑/↓: select • enter: toggle • p: pulse • a: add • A: admin • r: refresh • q: quit") + "\n"
	}
	return s
}
