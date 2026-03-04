package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afeldman/cloudlogin/pkg/awsconfig"
)

type logMsg struct {
	line string
}

type doneMsg struct {
	err error
}

type model struct {
	status  string
	logs    []string
	running bool
	done    bool
	logCh   chan tea.Msg
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.running {
				return m, nil
			}
			m.running = true
			m.status = "Update laeuft..."
			m.logs = nil
			m.logCh = make(chan tea.Msg, 200)
			startUpdate(m.logCh)
			return m, listenLogCmd(m.logCh)
		}
	case logMsg:
		m.logs = append(m.logs, msg.line)
		return m, listenLogCmd(m.logCh)
	case doneMsg:
		m.running = false
		m.done = true
		if msg.err != nil {
			m.status = fmt.Sprintf("Fehler: %v", msg.err)
		} else {
			m.status = "AWS Config aktualisiert"
		}
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString("AWS Config Update (SSO)\n\n")
	b.WriteString("[Enter] Update starten  |  [q] Beenden\n")
	b.WriteString("Env: AWS_SSO_REGION, AWS_SSO_START_URL\n\n")
	b.WriteString("Status: " + m.status + "\n\n")
	if len(m.logs) > 0 {
		b.WriteString("Logs:\n")
		for _, line := range m.logs {
			b.WriteString("- " + line + "\n")
		}
	}
	return b.String()
}

func startUpdate(logCh chan tea.Msg) {
	go func() {
		err := awsconfig.UpdateFromSSO(func(msg string) {
			logCh <- logMsg{line: msg}
		})
		logCh <- doneMsg{err: err}
		close(logCh)
	}()
}

func listenLogCmd(logCh chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-logCh
		if !ok {
			return doneMsg{}
		}
		return msg
	}
}

func main() {
	p := tea.NewProgram(model{status: "Bereit"})
	if _, err := p.Run(); err != nil {
		fmt.Println("Fehler:", err)
	}
}
