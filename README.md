<p align="center">
  <img src="logo.png" alt="cloudlogin" width="320">
</p>

<p align="center">
  Ein modernes Tool zur Verwaltung von AWS- und Kubernetes-Verbindungen mit <strong>GUI, CLI und TUI</strong>.
</p>

---

## Features

### GUI (Grafische Oberfläche)
- **AWS SSO Login**: Authentifizierung zu AWS SSO Profilen
- **AWS SSO Config Update**: Aktualisierung von AWS Profilen aus SSO
- **AWS/Kubernetes Sync**: Automatische KUBECONFIG-Pflege passend zu AWS Profilen
- **Kubernetes Kontext Verwaltung**: Schneller Wechsel zwischen K8s Clustern
- **Verbindungstests**: AWS und Kubernetes Konnektivität prüfen
- **Quick Actions**: AWS Console, k9s, ENV-Export

### CLI
- `--update-aws-config`: AWS Profile aus SSO synchronisieren (non-interactive)
- `--sanitize-aws-config`: Beschädigte AWS Config bereinigen + Backup erstellen

### TUI
- `cloudlogin-awsconfig-tui`: Bubble Tea Terminal UI für interaktive SSO Config-Updates

## Requirements

- Go 1.21+ (mit CGO_ENABLED=1)
- `aws` CLI
- `kubectl`
- `k9s` (optional)
- `helm` (optional)

## Installation

### Binaries herunterladen
Fertig gebaute Binaries unter [Releases](https://github.com/afeldman/cloudlogin/releases).

### Homebrew (macOS)
```bash
brew install afeldman/tap/cloudlogin
```

### Bauen von Quellcode
```bash
task build
./cloudlogin
```

## Verwendung

### GUI Mode (Standard)
```bash
./cloudlogin
```

### CLI Mode
```bash
# AWS SSO Profile aktualisieren
./cloudlogin --update-aws-config

# AWS Config bereinigen (z.B. bei "Unable to parse config file")
./cloudlogin --sanitize-aws-config
```

### TUI Mode
```bash
./bin/cloudlogin-awsconfig-tui
```

## Entwicklung

```bash
task build        # Binary bauen
task run          # Bauen und starten
task test         # Tests ausführen
task deps         # Dependencies aktualisieren
task snapshot     # Lokaler Release-Build (macOS, kein Publish)
task clean        # Artefakte löschen
```

## Architektur

```
cloudlogin/
├── main.go                    # Entry Point (GUI + CLI Dispatch)
├── gui.go                     # Fyne GUI
├── assets.go                  # Eingebettete Assets (Logo)
├── internal/
│   ├── shell/                 # LoginResult, Terminal öffnen
│   ├── kube/                  # Kubernetes Contexts + KUBECONFIG-Writer
│   └── aws/                   # AWS Profile, EKS Sync
├── cmd/awsconfig-tui/         # TUI Binary (Bubble Tea)
└── pkg/awsconfig/             # SSO Config Logic (CLI + TUI)
```

Features werden in `internal/aws/` oder `pkg/awsconfig/` implementiert und dann in GUI, CLI und TUI integriert.

## Releases mit GoReleaser

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

GitHub Actions baut automatisch macOS und Linux Binaries via GoReleaser.

## Troubleshooting

**Build schlägt fehl (CGO)**
```bash
export CGO_ENABLED=1
task build
```

**`aws sso login` → "Unable to parse config file"**
```bash
./cloudlogin --sanitize-aws-config
```

**Kubernetes Contexts leer**
```bash
kubectl config get-contexts
echo $KUBECONFIG
```

## License

MIT
