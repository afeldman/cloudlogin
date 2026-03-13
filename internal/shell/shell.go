package shell

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// LoginResult is the outcome of an authentication or context-switching operation.
type LoginResult struct {
	Success bool
	Message string
}

// OpenShellWithEnv opens a new terminal window with the given environment variables.
// Supports macOS (Terminal.app via AppleScript) and common Linux terminal emulators.
func OpenShellWithEnv(envLines []string, logFn func(string)) {
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
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
			if _, err := exec.LookPath(term); err != nil {
				continue
			}
			switch term {
			case "x-terminal-emulator":
				cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
			case "gnome-terminal":
				cmd = exec.Command(term, "--", "bash", "-lc", shellCmd)
			default:
				cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
			}
			debugCmd = term + " -e bash -lc <cmd>"
			break
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

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}
