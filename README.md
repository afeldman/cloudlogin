# Cloud Login Manager

Ein modernes Desktop-Tool zur Verwaltung von AWS- und Kubernetes-Verbindungen mit grafischer Benutzeroberfläche.

## Features

- **AWS SSO Login**: Einfache Authentifizierung zu AWS SSO Profilen
- **Kubernetes Kontext Verwaltung**: Schneller Wechsel zwischen K8s Clustern
- **Verbindungstests**: Überprüfe AWS und Kubernetes Konnektivität
- **Quick Actions**: Schnelleinstieg zu:
  - AWS Console im Browser
  - k9s Terminal UI für Kubernetes
  - AWS Environment-Variablen Export
  - Tool-Verfügbarkeitsprüfung

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

1. **AWS Tab**: AWS Profile aus `~/.aws/config` wählen, Verbindung testen oder SSO Login durchführen
2. **Kubernetes Tab**: Kubernetes Contexts aus KUBECONFIG wählen und zwischen ihnen wechseln
3. **Quick Actions Tab**: Schnelle Aktionen für häufige Aufgaben

## Architektur

Das Tool ist in folgende Funktionsbereiche strukturiert:

- **Config Parser**: Liest AWS Profiles und Kubernetes Contexts
- **Login Actions**: Führt AWS SSO Logins und Kubernetes Context Switches durch
- **GUI**: Fyne-basierte Oberfläche mit Tabs für jede Funktion

## Logging

Alle Aktionen werden in einer integrierten Log-Konsole protokolliert. Mit dem "Log leeren"-Button können Logs zurückgesetzt werden.

## Entwicklung

### Make-Commands
```bash
make build        # Binary bauen
make run          # Build und Run
make test         # Tests ausführen
make deps         # Dependencies herunterladen und tidyen
make clean        # Artifacts löschen
make help         # Hilfemeldung anzeigen
```

### Architektur

Das Projekt ist mit einer erweiterbaren **Plugin-Architektur** aufgebaut:

- **Cloud Provider Interface**: Einfach neue Provider (AWS, Azure, GCP, etc.) hinzufügen
- **Context Handler**: Für Kontext-basierte Services wie Kubernetes
- **Dynamische GUI**: Tabs werden automatisch aus registrierten Providern generiert

Lese [ARCHITECTURE.md](ARCHITECTURE.md) für Details wie du neue Provider hinzufügst.

### Vorhandene Provider

- ✅ **AWS**: SSO Login, Credential Management, Verbindungstests
- 📋 **Azure**: Template vorhanden, Implementierung in Arbeit
- ✅ **Kubernetes**: Context Switching, Verbindungstests

### Releases mit GoReleaser

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
