package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// LoginResult represents the outcome of an authentication or context-switching operation.
//
// Fields:
//   - Success: true if the operation completed without errors
//   - Message: Detailed message about the operation result
type LoginResult struct {
	Success bool
	Message string
}

// escapeAppleScript escapes special characters in a string for AppleScript compatibility.
//
// Escapes:
//   - Backslashes: \ -> \\
//   - Quotes: " -> \"
//
// This is necessary before embedding a string in an AppleScript tell block.
//
// Example:
//
//	cmd := "export AWS_PROFILE=dev; aws s3 ls"
//	escaped := escapeAppleScript(cmd)
//	// Result: export AWS_PROFILE=dev; aws s3 ls (with any quotes escaped)
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// openShellWithEnv opens a new terminal window with the given environment variables.
//
// Supported platforms:
//   - macOS: Uses AppleScript to open Terminal.app
//   - Linux: Tries x-terminal-emulator, gnome-terminal, konsole, xterm
//
// Environment variables should be in the format:
//   - "export KEY=VALUE"
//   - "KEY=VALUE"
//
// logFn receives status messages during execution.
//
// Example:
//
//	envLines := []string{
//		"export AWS_PROFILE=dev",
//		"export AWS_REGION=eu-central-1",
//	}
//	openShellWithEnv(envLines, func(msg string) {
//		fmt.Println(msg)
//	})
func openShellWithEnv(envLines []string, logFn func(string)) {
	if len(envLines) == 0 {
		logFn("❌ Keine ENV-Variablen gesetzt")
		return
	}

	cmdLine := strings.Join(envLines, "; ")
	var cmd *exec.Cmd
	var debugCmd string

	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf(
			`tell application "Terminal"
				activate
				do script "%s; exec zsh -l"
			end tell`,
			escapeAppleScript(cmdLine),
		)
		cmd = exec.Command("osascript", "-e", script)
		debugCmd = "osascript -e <script>"
	} else {
		shellCmd := fmt.Sprintf("%s; exec bash -l", cmdLine)
		if _, err := exec.LookPath("x-terminal-emulator"); err == nil {
			cmd = exec.Command("x-terminal-emulator", "-e", "bash", "-lc", shellCmd)
			debugCmd = "x-terminal-emulator -e bash -lc <cmd>"
		} else {
			for _, term := range []string{"gnome-terminal", "konsole", "xterm"} {
				if _, err := exec.LookPath(term); err == nil {
					switch term {
					case "gnome-terminal":
						cmd = exec.Command(term, "--", "bash", "-lc", shellCmd)
						debugCmd = "gnome-terminal -- bash -lc <cmd>"
					case "konsole":
						cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
						debugCmd = "konsole -e bash -lc <cmd>"
					case "xterm":
						cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
						debugCmd = "xterm -e bash -lc <cmd>"
					}
					break
				}
			}
		}
	}

	if cmd == nil {
		logFn("❌ Kein Terminal gefunden (x-terminal-emulator, gnome-terminal, konsole, xterm)")
		return
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "<keine Ausgabe>"
		}
		logFn(fmt.Sprintf("❌ Shell konnte nicht geöffnet werden (%s): %v", debugCmd, err))
		logFn(fmt.Sprintf("   Output: %s", msg))
		return
	}
	logFn("🖥️  Shell mit ENV geöffnet")
}
