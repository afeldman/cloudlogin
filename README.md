# Cloud Login Manager

Ein modernes Tool zur Verwaltung von AWS- und Kubernetes-Verbindungen mit **GUI, CLI und TUI**-Modi.

## Features

### GUI (Grafische Oberfläche)
- **AWS SSO Login**: Einfache Authentifizierung zu AWS SSO Profilen
- **AWS SSO Config Update**: Live-Aktualisierung von AWS Profilen aus SSO
- **AWS Config Sanitization**: Automatische Bereinigung von beschädigten AWS Config-Dateien
- **Kubernetes Kontext Verwaltung**: Schneller Wechsel zwischen K8s Clustern
- **Verbindungstests**: Überprüfe AWS und Kubernetes Konnektivität
- **Quick Actions**: Schnelleinstieg zu:
  - AWS Console im Browser
  - k9s Terminal UI für Kubernetes
  - AWS Environment-Variablen Export
  - Tool-Verfügbarkeitsprüfung

### CLI (Kommandozeile)
- `--update-aws-config`: Aktualisiert AWS Profile basierend auf SSO (non-interactive)
- `--sanitize-aws-config`: Bereinigt beschädigte AWS Config-Dateien und erstellt Backup

### TUI (Terminal User Interface)
- `cloudlogin-awsconfig-tui`: Eigenständiges Bubble Tea Programm für interaktive SSO Config-Updates
- Live Log-Streaming während der Verarbeitung
- Einfache Bedienung: Enter zum Starten, q zum Beenden

## Requirements

- Go 1.21+
- Installiert auf dem System:
  - `aws` CLI
  - `kubectl`
  - `k9s` (optional, für k9s Aktion)
  - `helm` (optional)

## Installation

### Binaries herunterladen
Fertig gebaute Binaries für dein Betriebssystem findest du unter [Releases](https://github.com/afeldman/cloudlogin/releases).

### Homebrew (macOS)
```bash
brew install afeldman/tap/cloudlogin
```

### Bauen von Quellcode
```bash
make build
./cloudlogin
```

Oder direkt mit Go:
```bash
go build -o cloudlogin main.go
./cloudlogin
```

## Verwendung

### GUI Mode (Standard)
```bash
./cloudlogin
```

Startet die grafische Oberfläche mit Tabs für:
1. **AWS Tab**: AWS Profile aus `~/.aws/config` wählen, Verbindung testen oder SSO Login durchführen
2. **Kubernetes Tab**: Kubernetes Contexts aus KUBECONFIG wählen und zwischen ihnen wechseln
3. **Quick Actions Tab**: Schnelle Aktionen für häufige Aufgaben

### CLI Mode
```bash
# AWS SSO Profiles aktualisieren (non-interactive)
./cloudlogin --update-aws-config

# AWS Config-Datei bereinigen (z.B. bei Parsing-Fehlern)
./cloudlogin --sanitize-aws-config
```

**Hinweis zur Sanitization**: Falls `aws sso login` mit "Unable to parse config file" fehlschlägt, kann die AWS Config-Datei beschädigte Zeichen enthalten. Das `--sanitize-aws-config` Flag bereinigt diese automatisch und erstellt eine Backup-Datei.

### TUI Mode
```bash
# AWS SSO Config-Update mit interaktiver Terminal UI
./bin/cloudlogin-awsconfig-tui
```

Startet eine Bubble Tea-basierte Terminal-Benutzeroberfläche mit:
- Live-Logging während des SSO Updates
- Interaktive Bedienung (Enter zum Starten, q zum Beenden)

## Architektur

Das Tool ist in folgende Funktionsbereiche strukturiert:

- **`main.go`**: GUI Entry Point (Fyne) + CLI Dispatcher
- **`cmd/awsconfig-tui/main.go`**: TUI Entry Point (Bubble Tea)
- **`pkg/awsconfig/`**: Reusable SSO Config Management Logic
  - `UpdateFromSSO()`: Aktualisiert AWS Profile basierend auf SSO
  - `SanitizeConfigFile()`: Bereinigt beschädigte AWS Config-Dateien
- **`pkg/provider/`**: (Legacy) Plugin-basierte Architektur für Cloud Provider

### Neue Paketstruktur

Das `pkg/awsconfig` Package enthält die SSO-Verwaltungslogik, die von allen Modi (GUI, CLI, TUI) verwendet wird:

```
pkg/awsconfig/
├── sso_config.go        # UpdateFromSSO, SanitizeConfigFile, AWS SSO Integration
└── README.md            # Ausführliche Dokumentation des Packages
```

Das Package mit:
- AWS SSO Token-Management
- Account und Role Enumeration via AWS API
- Config-Datei Parsing und Merging
- Umweltvariation-Handling für kaputte Config-Dateien
- Logging-Callbacks für Integration in alle Modi

## Logging & Monitoring

Alle Aktionen werden in einer integrierten Log-Konsole protokolliert:
- In der **GUI**: Integrierte Log-Konsole am unteren Rand mit "Log leeren"-Button
- In der **CLI**: Ausgabe auf stdout/stderr
- In der **TUI**: Live-Log-Display während der Verarbeitung

Logs zeigen:
- ✅ Erfolgreiche Operationen (grün)
- ❌ Fehler (rot) - Sensitive-Tokens werden automatisch maskiert
- 🔐/🔄/🔍 Vorgänge (andere Icons zur schnellen Orientierung)

## Dependencies

- **Go 1.21+**
- **CGO**: Erforderlich für einige externe Abhängigkeiten (CGO_ENABLED=1)
- **AWS CLI**: `aws` muss installiert sein für SSO funktionalität
- **kubectl**: Erforderlich für Kubernetes Features
- **Optionale Tools**:
  - `k9s`: Für k9s Terminal UI Aktion
  - `helm`: Für erweiterte Kubernetes Operationen

## Entwicklung

### Build & Run

```bash
# GUI bauen und starten
make build
./cloudlogin

# Oder direkt mit make
make run

# Tests ausführen
make test

# Dependencies aktualisieren
make deps

# Cleanup
make clean

# Hilfe anzeigen
make help
```

### Dependencies verstehen

Das Projekt nutzt zwei verschiedene UI-Frameworks:

1. **Fyne v2** (GUI)
   - Cross-platform Desktop GUI
   - Verwendet in `main.go`
   - Benötigt CGO_ENABLED=1

2. **Bubble Tea v0.26.6** (TUI)
   - Terminal User Interface Framework
   - Verwendet in `cmd/awsconfig-tui/main.go`
   - Portable, nur Stdlib

### Neue Features hinzufügen

#### In `pkg/awsconfig` Funktionen hinzufügen
1. Implementiere neue Funktion mit `logFn func(string)` Parameter
2. Funktion wird von GUI, CLI und TUI verwendet
3. Callbacks ermöglichen Logging in allen Modi

Beispiel:
```go
func MyNewFunction(logFn func(string)) error {
    logFn("ℹ️  Verarbeite...")
    // ...
    logFn("✅ Fertig")
    return nil
}
```

#### Von GUI aufrufen
```go
go func() {
    if err := awsconfig.MyNewFunction(logFn); err != nil {
        logFn(fmt.Sprintf("❌ Fehler: %v", err))
        return
    }
}()
```

#### Von CLI aufrufen
```go
case "--my-feature":
    if err := awsconfig.MyNewFunction(func(msg string) { fmt.Println(msg) }); err != nil {
        fmt.Fprintf(os.Stderr, "Fehler: %v\n", err)
        os.Exit(1)
    }
    return
```

### Troubleshooting

**Problem**: Build schlägt mit CGO-Fehler fehl
```bash
# Lösung: CGO aktivieren
export CGO_ENABLED=1
make build
```

**Problem**: `aws sso login` schlägt mit "Unable to parse config file" fehl
```bash
# Lösung 1: AWS Config bereinigen
./cloudlogin --sanitize-aws-config

# Lösung 2: Via GUI
./cloudlogin  # -> AWS Tab -> "AWS Config bereinigen"

# Lösung 3: Via TUI
./bin/cloudlogin-awsconfig-tui
```

**Problem**: Kubernetes Contexts werden nicht angezeigt
```bash
# Überprüfe KUBECONFIG
echo $KUBECONFIG
kubectl config get-contexts
```



## Documentation

Umfassende Dokumentation für verschiedene Zwecke:

| Dokument | Zielgruppe | Inhalt |
|----------|-----------|---------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Entwickler | Multi-Mode Architektur, Feature-Integration, Workflow |
| [DEVELOPMENT.md](DEVELOPMENT.md) | Entwickler | Environment Setup, Development Workflow, Debugging |
| [pkg/awsconfig/README.md](pkg/awsconfig/README.md) | Entwickler | AWS SSO Package API, Funktionen, Error Handling |
| [cmd/awsconfig-tui/README.md](cmd/awsconfig-tui/README.md) | Entwickler | TUI Architektur, Bubble Tea Integration, Troubleshooting |

## Releases mit GoReleaser

Das Projekt nutzt **GoReleaser mit separaten Konfigurationen** für verschiedene Plattformen:

- **`.goreleaser.yaml`**: macOS Binaries (CGO_ENABLED=1, darwin amd64 + arm64)
  - Läuft auf `macos-latest` in GitHub Actions
  
- **`.goreleaser-linux.yaml`**: Linux Binaries (CGO_ENABLED=1, linux amd64 + arm64)
  - Läuft auf `ubuntu-latest` mit Linux GUI Dependencies
  
Dies nachdem diese Lösung bereits erfolgreich in `git-signing-manager` verwendet wird.

#### Ein Release erstellen:

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

GitHub Actions erstellt automatisch:
- Multi-Plattform Binaries (macOS, Linux)
- Checksums (SHA256)
- GitHub Release mit allen Assets

#### Lokal testen:

```bash
make test-release  # Lokaler Snapshot-Build ohne zu veröffentlichen
```

## Bekannte Probleme

- k9s Launch für Linux: Das Tool versucht verschiedene Terminal-Emulatoren zu nutzen (GNOME Terminal, XTerm, KDE Konsole)

## License

MIT
