// Package main provides cloudlogin-awsconfig-tui - a Terminal UI for AWS SSO config updates.
//
// This is a standalone TUI application using Bubble Tea framework for interactive
// terminal sessions. It provides a simple interface for synchronizing AWS profiles
// from SSO with real-time log streaming.
//
// Features:
//   - Interactive terminal interface (Bubble Tea)
//   - Real-time log display during SSO profile synchronization
//   - Channel-based message passing for asynchronous operations
//   - Minimal dependencies (only Bubble Tea and standard library)
//
// Usage:
//
//	go run ./cmd/awsconfig-tui/main.go
//	# Or after building:
//	./bin/cloudlogin-awsconfig-tui
//
// Controls:
//   - ENTER: Start AWS SSO config update
//   - q: Quit application
//   - CTRL+C: Quit application
//
// The TUI uses the shared pkg/awsconfig package for all AWS operations,
// ensuring consistent behavior across all cloudlogin modes (GUI, CLI, TUI).
//
// Architecture:
//   - Model-Update-View (MVU) pattern from Bubble Tea
//   - logMsg and doneMsg types for channel-based async updates
//   - Goroutine-based long-running operations (AWS API calls)
//   - Non-blocking event loop for responsive UI
package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afeldman/cloudlogin/pkg/awsconfig"
)

// logMsg represents a single log line to display in the TUI.
//
// These messages are sent through a channel during SSO profile synchronization,
// allowing real-time updates without blocking the UI event loop.
type logMsg struct {
	line string
}

// doneMsg indicates that the AWS SSO config update operation has completed.
//
// Fields:
//   - err: nil if successful, otherwise contains error message
//
// This message is sent once at the end of the update process.
type doneMsg struct {
	err error
}

// model represents the complete state of the TUI application.
//
// Fields:
//   - status: Current status message ("Ready", "Running", "Complete", "Error")
//   - logs: Accumulated log lines to display in viewport
//   - running: true while SSO update is in progress
//   - done: true when update operation has completed
//   - logCh: Channel for receiving logMsg and doneMsg during async updates
//
// The model implements the Bubble Tea Model interface with Init, Update, and View methods.
type model struct {
	status  string
	logs    []string
	running bool
	done    bool
	logCh   chan tea.Msg
}

// Init initializes the model when the application starts.
//
// Returns nil as we don't need any initial commands. The actual update
// is triggered when the user presses ENTER.
func (m model) Init() tea.Cmd {
	return nil
}

// Update processes incoming messages and updates the model state.
//
// Message Types:
//   - tea.KeyMsg: Keyboard input (q=quit, enter=start, etc.)
//   - logMsg: New log line to display
//   - doneMsg: Update operation completed
//
// Returns the updated model and optional command to execute.
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

// View renders the current state of the TUI.
//
// Displays:
//   - Title and keyboard shortcuts
//   - Current status message
//   - All accumulated log lines from SSO update
//   - Environment variable hints
//
// The view is regenerated after each message, but since it's simple text,
// no special rendering framework is used (plain strings.Builder).
//
// Example output:
//
//	AWS Config Update (SSO)
//
//	[Enter] Update starten  |  [q] Beenden
//	Env: AWS_SSO_REGION, AWS_SSO_START_URL
//
//	Status: Update laeuft...
//
//	Logs:
//	- 🔄 AWS Config aktualisieren (SSO: https://...)
//	- ✅ Token gefunden
//	- 🔍 Lade Accounts...
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

// startUpdate launches an asynchronous AWS SSO config update operation.
//
// This function:
//  1. Spawns a goroutine to run awsconfig.UpdateFromSSO
//  2. Provides a callback function that sends logMsg to the channel
//  3. Sends doneMsg when the operation completes (success or error)
//  4. Closes the channel to signal end of stream
//
// Parameters:
//   - logCh: Channel to receive log messages and completion signal
//
// The separation of this function allows the UI to remain responsive while
// AWS API calls happen in the background. All output goes through the channel.
//
// Example:
//
//	logCh := make(chan tea.Msg, 200)
//	startUpdate(logCh)
//	// Logs and done messages will arrive through logCh asynchronously
func startUpdate(logCh chan tea.Msg) {
	go func() {
		err := awsconfig.UpdateFromSSO(func(msg string) {
			logCh <- logMsg{line: msg}
		})
		logCh <- doneMsg{err: err}
		close(logCh)
	}()
}

// listenLogCmd creates a Bubble Tea command that waits for the next message from logCh.
//
// This function is called after each message is processed to continue listening
// for updates from the background AWS SSO update goroutine.
//
// Returns:
//   - A tea.Cmd (function) that receives the next message from logCh
//   - When channel is closed: returns empty doneMsg{}
//   - Otherwise: returns the received  message (logMsg or doneMsg)
//
// This enables the event loop to block waiting for channel messages without
// blocking the UI. Messages arrive asynchronously as they're sent during SSO operations.
//
// Flow:
//  1. startUpdate() launches goroutine and sends logMsg to channel
//  2. listenLogCmd() waits for message
//  3. Message received -> model.Update() processes it
//  4. model.Update() calls listenLogCmd() again to wait for next message
//  5. When done -> doneMsg sent, channel closed, loop completes
func listenLogCmd(logCh chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-logCh
		if !ok {
			return doneMsg{}
		}
		return msg
	}
}

// main initializes and runs the TUI application.
//
// Behavior:
//  1. Creates a new Bubble Tea program with initial model
//  2. Sets initial status to "Bereit" (Ready)
//  3. Runs the program (blocking call)
//  4. Handles any errors from the TUI
//
// The program will continue running until the user presses 'q' or 'ctrl+c'.
//
// Exit codes:
//   - 0: User quit gracefully
//   - Non-zero: Error occurred during TUI execution
//
// Example:
//
//	$ go run ./cmd/awsconfig-tui/main.go
//	AWS Config Update (SSO)
//
//	[Enter] Update starten  |  [q] Beenden
func main() {
	p := tea.NewProgram(model{status: "Bereit"})
	if _, err := p.Run(); err != nil {
		fmt.Println("Fehler:", err)
	}
}
