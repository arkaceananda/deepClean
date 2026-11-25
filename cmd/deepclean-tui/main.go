package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"deepClean/internal/config"
)

type field int

const (
	fieldPath field = iota
	fieldDays
	fieldMinSize
	fieldRisk
	fieldCount
)

type model struct {
	inputs []textinput.Model
	focus  int
	risks  []string
	risk   int
	status string
}

func initialModel() model {
	cfg, _ := config.Load()
	m := model{inputs: make([]textinput.Model, 3), risks: []string{"all", "safe", "caution", "risky"}}

	m.inputs[fieldPath] = textinput.New()
	m.inputs[fieldPath].Placeholder = "/"
	m.inputs[fieldPath].CharLimit = 256
	m.inputs[fieldPath].Width = 40
	if cfg.Path != "" {
		m.inputs[fieldPath].SetValue(cfg.Path)
	} else {
		m.inputs[fieldPath].SetValue("/")
	}

	m.inputs[fieldDays] = textinput.New()
	m.inputs[fieldDays].Placeholder = "90"
	m.inputs[fieldDays].CharLimit = 4
	m.inputs[fieldDays].Width = 10
	if cfg.Days > 0 {
		m.inputs[fieldDays].SetValue(strconv.Itoa(cfg.Days))
	} else {
		m.inputs[fieldDays].SetValue("90")
	}

	m.inputs[fieldMinSize] = textinput.New()
	m.inputs[fieldMinSize].Placeholder = "1"
	m.inputs[fieldMinSize].CharLimit = 6
	m.inputs[fieldMinSize].Width = 10
	if cfg.MinSizeMB > 0 {
		m.inputs[fieldMinSize].SetValue(fmt.Sprintf("%g", cfg.MinSizeMB))
	} else {
		m.inputs[fieldMinSize].SetValue("1")
	}

	risk := cfg.Risk
	if risk == "" {
		risk = "all"
	}
	m.risk = 0
	for i, r := range m.risks {
		if r == risk {
			m.risk = i
		}
	}
	m.inputs[fieldPath].Focus()
	return m
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "down":
			m.focus = (m.focus + 1) % int(fieldCount)
			m.updateFocus()
			return m, nil
		case "shift+tab", "up":
			m.focus = (m.focus - 1 + int(fieldCount)) % int(fieldCount)
			m.updateFocus()
			return m, nil
		case "left", "right", " ":
			if m.focus == int(fieldRisk) {
				if msg.String() == "left" {
					m.risk = (m.risk - 1 + len(m.risks)) % len(m.risks)
				} else {
					m.risk = (m.risk + 1) % len(m.risks)
				}
				return m, nil
			}
		case "enter":
			// save
			days, _ := strconv.Atoi(strings.TrimSpace(m.inputs[fieldDays].Value()))
			if days <= 0 {
				days = 90
			}
			minSize, _ := strconv.ParseFloat(strings.TrimSpace(m.inputs[fieldMinSize].Value()), 64)
			if minSize <= 0 {
				minSize = 1
			}
			cfg, _ := config.Load()
			cfg.Path = strings.TrimSpace(m.inputs[fieldPath].Value())
			cfg.Days = days
			cfg.MinSizeMB = minSize
			cfg.Risk = m.risks[m.risk]
			p, _ := config.Path()
			_ = os.MkdirAll(filepath.Dir(p), 0700)
			b, _ := json.MarshalIndent(cfg, "", "  ")
			_ = os.WriteFile(p, b, 0600)
			m.status = "Saved. Press q to quit."
			return m, nil
		}
	}
	// update focused text input
	if m.focus < 3 {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateFocus() {
	for i := range m.inputs {
		if i == m.focus {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("deepclean config")
	labels := []string{"Path", "Days", "Min size (MB)"}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	for i := range 3 {
		cursor := "  "
		if m.focus == i {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%-12s %s\n", cursor, labels[i], m.inputs[i].View()))
	}
	// risk
	cursor := "  "
	if m.focus == int(fieldRisk) {
		cursor = "> "
	}
	b.WriteString(fmt.Sprintf("%s%-12s %s  (left/right to change)\n", cursor, "Risk", m.risks[m.risk]))
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString("Tab/↑↓ navigate • Enter save • q quit\n")
	return b.String()
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
