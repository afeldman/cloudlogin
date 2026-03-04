# AWS SSO Config TUI

Standalone Bubble Tea Terminal User Interface für interaktive AWS SSO Konfiguration.

## Usage

### Build

```bash
cd cmd/awsconfig-tui
go build -o ../../bin/cloudlogin-awsconfig-tui main.go
```

Oder über Makefile (falls vorhanden):

```bash
make build-tui
./bin/cloudlogin-awsconfig-tui
```

### Run

```bash
./bin/cloudlogin-awsconfig-tui
```

### Screenshots / Behavior

```
┌─────────────────────────────────────────────────────┐
│  AWS SSO Config Manager                             │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Status: Ready                                      │
│                                                     │
│  Logs:                                              │
│  🔍 Suche SSO Token...                              │
│  ✅ Token gefunden (expires at 2026-03-05)          │
│  🔍 Lade Accounts...                                │
│  ✅ 3 Accounts gefunden                              │
│  🔍 Lade Roles...                                   │
│  ✅ 8 Roles insgesamt                                │
│  🔍 Generiere Profile...                            │
│  ✅ AWS Config aktualisiert                         │
│                                                     │
│  Status: Complete!                                  │
│                                                     │
│  Press ENTER to start, 'q' to quit                 │
└─────────────────────────────────────────────────────┘
```

## Controls

| Key | Action |
|-----|--------|
| `ENTER` | Start AWS SSO Config Update |
| `q` / `CTRL+C` | Quit Application |

## How It Works

### Architecture

The TUI uses Bubble Tea's Model-Update-View (MVU) pattern with channels for async logging:

```go
type model struct {
    status  string           // Current status ("Ready", "Running", "Complete", "Error")
    logs    []string         // Displayed log lines
    running bool             // Is update in progress
    done    bool             // Has update finished
    logCh   chan tea.Msg     // Channel for log messages
}
```

### Log Streaming

Instead of collecting logs in an array, the TUI uses channels for real-time display:

1. User presses ENTER
2. `startUpdate()` launches goroutine with `awsconfig.UpdateFromSSO()`
3. Each log message sent via channel: `logCh <- logMsg("message")`
4. Bubble Tea event loop processes each message
5. UI updates in real-time (no blocking)

```go
func startUpdate(logCh chan tea.Msg) {
    go func() {
        logFn := func(msg string) {
            logCh <- logMsg(msg)  // Send to channel, TUI picks it up
        }
        if err := awsconfig.UpdateFromSSO(logFn); err != nil {
            logCh <- doneMsg{err}
        } else {
            logCh <- doneMsg{nil}
        }
    }()
}
```

### Message Types

```go
type logMsg string          // Log line to display
type doneMsg struct { Err error }  // Update finished (success/error)
```

The `Update()` method handles these messages:

```go
case logMsg(msg):
    m.logs = append(m.logs, string(msg))
    return m, listenLogCmd(m.logCh)  // Listen for next message
    
case doneMsg(result):
    m.done = true
    m.status = "Complete!"
    if result.Err != nil {
        m.status = "Error: " + result.Err.Error()
    }
```

## Integration with pkg/awsconfig

The TUI is just a presentation layer for `pkg/awsconfig.UpdateFromSSO()`:

```
TUI (main.go)
    ↓
    └── awsconfig.UpdateFromSSO(logFn)
        ├── findSSOToken()
        ├── listSSOAccounts()
        ├── listSSORoles()
        ├── buildConfigString()
        ├── mergeWithExisting()
        └── writeAndBackup()
```

All business logic is in `pkg/awsconfig`, the TUI only handles display and user interaction.

## Error Handling

Errors are logged via the channel and displayed in the UI:

```
Status: Error!

Logs:
❌ AWS Config Update failed: SSO token expired
```

Users can then:
1. Run `aws sso login` to refresh token
2. Restart the TUI

## Performance

The TUI is designed for:
- **Instant startup**: Only imports bubbletea stdlib
- **Live feedback**: Streaming logs without blocking
- **Clean exit**: Graceful shutdown on 'q'

The AWS operations (API calls) may take 5-30 seconds depending on:
- Number of accounts
- Number of roles
- Network latency
- AWS API response time

All this time, users see live logs streaming to the terminal.

## Customization

### Change TUI Colors

Edit `View()` method in main.go:

```go
func (m model) View() string {
    // Add lipgloss styling
    style := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
    return style.Render(output)
}
```

### Add New Modes

Future enhancement: Add menu for other operations (sanitize, test connection, etc.):

```go
type mode int
const (
    menuMode mode = iota
    updateMode
    sanitizeMode
    testMode
)

// User selects from menu, TUI starts appropriate operation
```

### Keyboard Shortcuts

Currently minimal, but could add:

```go
case "t":
    // Start testConnection
case "s":
    // Start sanitizeConfigFile
case "c":
    // Clear logs
```

## Troubleshooting

### TUI doesn't start

**Problem**: Module not found

**Solution**:
```bash
go get github.com/charmbracelet/bubbletea@v0.26.6
cd cmd/awsconfig-tui
go build -o ../../bin/cloudlogin-awsconfig-tui main.go
```

### Logs disappear too fast

**Problem**: Terminal scrolled up

**Solution**: 
- Manually run `clear` before TUI
- Or enhance TUI to use `lipgloss` for better layout

### Update finishes but says "Error"

**Problem**: AWS Config write failed

**Solution**:
```bash
# Check permissions
ls -la ~/.aws/config

# Check disk space
df -h ~/.aws

# Run with sanitization first
./cloudlogin --sanitize-aws-config
./bin/cloudlogin-awsconfig-tui
```

## Compatibility

- **Go**: 1.21+
- **Bubble Tea**: v0.26.6+
- **Platforms**: macOS, Linux, Windows (untested)
- **Terminals**: xterm, GNOME Terminal, iTerm2, etc.

The TUI requires a full terminal emulator (not web-based terminals may have limited support).

## Future Enhancements

Potential improvements:

1. **Menu System**: Select operation (update, sanitize, test) from menu
2. **Progress Bar**: Show progress for long operations
3. **History**: Keep history of recent updates
4. **Themes**: Light/Dark mode switching
5. **Logging File**: Option to save full logs to file
6. **Config Preview**: Show generated config before writing

## Code Structure

```go
main.go
├── Imports: bubbletea, awsconfig
├── Type definitions: model, logMsg, doneMsg
├── Init(): Returns nil
├── Update(msg): Handle messages (KeyMsg, logMsg, doneMsg)
├── View(): Render current state
├── updateCmd(): Dispatch command to start update
├── startUpdate(): Launch goroutine with awsconfig
└── listenLogCmd(): Poll logCh for messages
```

Each function is small and focused on its responsibility.

## Testing

Manual testing:

```bash
# Build
cd cmd/awsconfig-tui && go build -o ../../bin/cloudlogin-awsconfig-tui main.go

# Run
./bin/cloudlogin-awsconfig-tui

# Start update: press ENTER
# Watch logs stream in real-time
# Exit: press 'q'
```

Automated testing would require:
- Mocking `awsconfig.UpdateFromSSO()`
- Testing message handling
- Verifying view output

Not yet implemented (no tests in repository).
