# Cloud Login Manager - Architektur

## Überblick

Das Cloud Login Manager Tool ist eine **multi-modale** Anwendung mit drei Einstiegspunkten:

1. **GUI** (`main.go`): Fyne-basierte Desktop-Oberfläche mit Tabs
2. **CLI** (`main.go` --flags): Command-Line Interface für Automatisierung
3. **TUI** (`cmd/awsconfig-tui/main.go`): Bubble Tea Terminal UI für interaktive Sessions

Alle Modi nutzen gemeinsame Business Logic aus dem `pkg/awsconfig` Package für:
- AWS SSO Config Management
- Environment Handling
- Config File Sanitization

## Code-Struktur

```
cloudlogin/
├── main.go                          # GUI Entry Point (Fyne) + CLI Dispatcher
├── cmd/
│   └── awsconfig-tui/
│       └── main.go                  # TUI Entry Point (Bubble Tea)
├── pkg/
│   ├── awsconfig/
│   │   ├── sso_config.go           # SSO Config Management Logic
│   │   │   ├── UpdateFromSSO()
│   │   │   ├── SanitizeConfigFile()
│   │   │   ├── findSSOToken()
│   │   │   ├── listSSOAccounts()
│   │   │   └── listSSORoles()
│   │   └── README.md               # Package Dokumentation
│   └── provider/                    # (Legacy) Plugin Architecture
│       ├── aws/
│       ├── azure/
│       └── kubernetes/
└── Makefile                         # Build Targets für alle Modi
```

## Workflow: Von Feature bis zu allen Modi

### 1. Feature als Package-Funktion implementieren (`pkg/awsconfig/`)

Alle neue Features sollten als Funktionen in `pkg/awsconfig` implementiert werden mit:
- Signatur: `func FeatureName(logFn func(string)) error`
- Logging via `logFn()` - kein direktes fmt.Println
- Rückgabe von `error` für Fehlerbehandlung

Beispiel:
```go
package awsconfig

func UpdateFromSSO(logFn func(string)) error {
    logFn("🔍 Suche SSO Token...")
    // ... implementation ...
    logFn("✅ SSO Config aktualisiert")
    return nil
}
```

### 2. GUI Integration (`main.go`)

Aufrufen in einem Goroutine mit GUI-spezifischem Logging:
```go
awsUpdateBtn := widget.NewButtonWithIcon("AWS Config aktualisieren", theme.CheckIcon(), func() {
    go func() {
        if err := awsconfig.UpdateFromSSO(logFn); err != nil {
            logFn(fmt.Sprintf("❌ Fehler: %v", err))
            return
        }
        // ggfs Profile neu laden
    }()
})
```

### 3. CLI Integration (`main.go` main() start)

Hinzufügen als Command-Line Flag:
```go
func main() {
    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "--update-aws-config":
            if err := awsconfig.UpdateFromSSO(func(msg string) { fmt.Println(msg) }); err != nil {
                fmt.Fprintf(os.Stderr, "Fehler: %v\n", err)
                os.Exit(1)
            }
            return
        }
    }
    // ... weiterer Code ...
}
```

### 4. TUI Integration (`cmd/awsconfig-tui/main.go`)

Aufrufen mit Channel-basiertem Logging für Live-Updates:
```go
func (m model) Init() tea.Cmd {
    return listenLogCmd(m.logCh)
}

func startUpdate(logCh chan tea.Msg) {
    go func() {
        logFn := func(msg string) {
            logCh <- logMsg(msg)
        }
        if err := awsconfig.UpdateFromSSO(logFn); err != nil {
            logCh <- doneMsg{err}
        } else {
            logCh <- doneMsg{nil}
        }
    }()
}
```

## AWS SSO Config Management (`pkg/awsconfig/`)

### UpdateFromSSO() Flow

```
findSSOToken()
    ↓
listSSOAccounts() [AWS API call]
    ↓
listSSORoles() [AWS API call pro Account]
    ↓
Build config string with profile definitions
    ↓
Read existing ~/.aws/config
    ↓
Merge: Keep non-managed sections, update managed section
    ↓
Write back with preserved permissions
```
       pm.RegisterProvider(azure.NewAzureProvider())
       pm.RegisterProvider(gcp.NewGCPProvider())  // ← Hinzufügen
       return pm
   }
   ```

Das war's! Der neue Tab wird automatisch generiert.

## Provider Manager

Der `ProviderManager` verwaltet alle registrierten Provider:

```go
pm := NewProviderManager()
pm.RegisterProvider(myProvider)           // Provider registrieren
p := pm.GetProvider("AWS")                // Provider abrufen
providers := pm.GetAllProviders()         // Alle Provider abrufen
```

## GUI Generation

Die GUI wird dynamisch aus den registrierten Providern generiert:

- Für jeden `CloudProvider` wird ein Tab erstellt (via `createProviderTab()`)
- Für jeden `ContextHandler` wird ein Tab erstellt (via `createContextTab()`)
- Zusätzlich gibt es einen "Quick Actions" Tab

## Nächste Schritte

- ✅ AWS Provider fertig
- 📋 Azure Provider (Template vorhanden, JSON Parsing nötig)
- 🔄 GCP Provider
- 🔄 Okta / SSO Provider
- 🔄 OpenTelemetry Integration für Logging/Tracing
