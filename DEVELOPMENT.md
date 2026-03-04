# Development Guide

Leitfaden für Entwickler, die am cloudlogin Projekt arbeiten.

## Environment Setup

### Voraussetzungen

```bash
# System requirements
- macOS 10.15+ oder Linux
- Go 1.21+
- AWS CLI v2
- kubectl
- git

# Homebrew (macOS)
brew install go awscli2 kubectl
```

### Repository klonen

```bash
git clone https://github.com/afeldman/cloudlogin.git
cd cloudlogin
```

### Dependencies installieren

```bash
# Alle Go Dependencies herunterladen
go mod download
go mod tidy

# Falls nicht kompatibel, Versions-Konflikt prüfen
go mod verify
```

### Build testen

```bash
# GUI build
make build
./cloudlogin

# TUI build (falls ARCHITECTURE.md vorsieht)
cd cmd/awsconfig-tui && go build -o ../../bin/cloudlogin-awsconfig-tui main.go
```

## Development Workflow

### Feature Development Cycle

1. **Feature in `pkg/awsconfig/` implementieren**
   - Alle Business Logic gehört hierher
   - Signature: `func FeatureName(logFn func(string)) error`
   - Nutze logFn für alle Status Messages

2. **Tests schreiben** (optional aber empfohlen)
   ```bash
   cd pkg/awsconfig
   go test -v -run TestFeatureName
   ```

3. **Integration in alle Modi**
   - GUI: Button/Action in `main.go`
   - CLI: Flag in `main() switch` Statement
   - TUI: Command in `cmd/awsconfig-tui/main.go`

4. **Format & Lint überprüfen**
   ```bash
   gofmt -w main.go pkg/awsconfig/sso_config.go
   go vet ./...
   ```

5. **Lokal testen vor Commit**
   ```bash
   # Test in allen Modi
   ./cloudlogin --new-feature  # CLI
   ./cloudlogin               # GUI, dann Action klicken
   ./bin/cloudlogin-awsconfig-tui  # TUI
   ```

### Struktur-Konventionen

#### Package Functions

Alle public Functions sollten dieses Pattern folgen:

```go
// DetailedDescription explains what the function does
// and under which conditions it's useful.
func MyFeature(logFn func(string)) error {
    logFn("🔍 Starting process...")
    
    // Core logic here
    if condition {
        logFn(fmt.Sprintf("ℹ️  Detail: %s", detail))
    }
    
    if err != nil {
        logFn(fmt.Sprintf("❌ Error: %v", err))
        return err
    }
    
    logFn("✅ Process completed")
    return nil
}
```

#### Logging Emoji Conventions

- 🔍 Info/Searching
- 🔐 Security/Authentication
- 🔄 Processing/Switching
- ✅ Success
- ❌ Error
- ℹ️  Information/Details
- 🖥️  System/Terminal
- 📄 Files
- ⚠️  Warning

#### Error Messages

```go
// GOOD: Clear, actionable messages
logFn("❌ AWS Config Parser fehlgeschlagen: Invalid INI syntax at line 15")

// BAD: Generic messages  
logFn("❌ Error occurred")
```

## Common Tasks

### Einen neuen AWS API Call hinzufügen

1. Nutze `runAWSJSON()` wrapper:

```go
func MyNewAPICall(logFn func(string)) error {
    logFn("🔍 Calling AWS API...")
    
    data, err := runAWSJSON(
        []string{"service", "operation", "--param", "value"},
        awsSSOEnv(),  // Use filtered environment
    )
    if err != nil {
        logFn(fmt.Sprintf("❌ API call failed: %v", err))
        return err
    }
    
    var result MyType
    if err := json.Unmarshal(data, &result); err != nil {
        logFn(fmt.Sprintf("❌ Parse failed: %v", err))
        return err
    }
    
    logFn("✅ Daten erhalten")
    return nil
}
```

2. Test mit echtem AWS Account:

```bash
# Stelle sicher SSO Token aktiv
aws sso login

# Test mit CLI
./cloudlogin --my-feature --param value

# Debug: Environment überprüfen
env | grep AWS
```

### Config File Handling

Config Files sollten immer mit Backup erstellt werden:

```go
// Read existing
existing, err := os.ReadFile(configPath)
if err != nil {
    return err
}

// Create backup
backupPath := configPath + ".bak"
if err := os.WriteFile(backupPath, existing, 0o600); err != nil {
    return err
}

// Preserve original permissions
info, _ := os.Stat(configPath)
mode := info.Mode()

// Write new version
if err := os.WriteFile(configPath, newContent, mode); err != nil {
    return err
}

logFn(fmt.Sprintf("✅ Backup created: %s", backupPath))
```

### Error Handling bei AWS CLI

Immer Sensitive Data (Access Tokens) masken:

```go
func maskSensitiveArgs(args []string) []string {
    result := make([]string, len(args))
    for i, arg := range args {
        if arg == "--access-token" && i+1 < len(args) {
            result[i] = arg
            result[i+1] = "***"
            i++
        } else {
            result[i] = arg
        }
    }
    return result
}

// In error logging
if err != nil {
    maskedArgs := maskSensitiveArgs(args)
    logFn(fmt.Sprintf("❌ AWS call failed: %v (command: %v)", err, maskedArgs))
}
```

### Environment Variables richtig handhaben

```go
// WRONG: Environment Pollution
env := os.Environ()
env = append(env, fmt.Sprintf("AWS_PROFILE=%s", profile))

// RIGHT: Filter existing vars first
env := filterEnv(os.Environ(), "AWS_PROFILE", "AWS_DEFAULT_PROFILE")
env = append(env, fmt.Sprintf("AWS_PROFILE=%s", profile))

// Use in exec.Command
cmd.Env = env
```

## Debugging

### Logs überprüfen

```bash
# GUI Logs in der Konsole sehen
./cloudlogin 2>&1 | grep -E "(ERROR|WARN|❌|❌)"

# CLI Logs direkt
./cloudlogin --my-feature

# TUI mit Output-Dateispeicherung
./bin/cloudlogin-awsconfig-tui 2>debug.log
```

### AWS Environment testen

```bash
# Check token validity
ls -la ~/.aws/sso/cache/
cat ~/.aws/sso/cache/*.json | jq '.expiresAt, .accessToken' 

# Test AWS API
aws sso list-accounts

# Check config parsing
head -20 ~/.aws/config
```

### Go Code debuggen

```bash
# Mit verbocsen Output
GODEBUG=madvdontneed=1 go run main.go

# Mit Race Detector
go run -race main.go

# Profiling für Performance
go test -cpuprofile=cpu.prof -bench .
go tool pprof cpu.prof
```

## Code Quality

### Format überprüfen

```bash
# Format alle Dateien
gofmt -w .

# Oder mit -s flag für simplifications
gofmt -s -w .

# Überprüfe ohne zu ändern
gofmt -l .
```

### Lint überprüfen

```bash
# Go vet (eingebaut)
go vet ./...

# Golangci-lint (optional, besser)
golangci-lint run ./...
```

### Tests schreiben

```bash
# Test-Datei erstellen
touch pkg/awsconfig/my_feature_test.go

# Test-Template
cat > pkg/awsconfig/my_feature_test.go << 'EOF'
package awsconfig

import "testing"

func TestMyFeature(t *testing.T) {
    logs := make([]string, 0)
    logFn := func(msg string) {
        logs = append(logs, msg)
    }
    
    err := MyFeature(logFn)
    
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    
    if len(logs) == 0 {
        t.Fatal("Expected logs")
    }
}
EOF

# Tests ausführen
go test -v ./pkg/awsconfig/
```

## Common Issues & Solutions

### Issue: "CGO_ENABLED=0" makes build fail

**Solution:**
```bash
export CGO_ENABLED=1
make clean build
```

### Issue: AWS CLI commands fail with "could not be found"

**Solution:**
```bash
# aws might not be in PATH, find it
which aws
/usr/local/bin/aws sso login

# Test within code
exec.Command("aws", "sso", "login").Run()
```

### Issue: `~/.aws/config` parse errors

**Solution:**
```bash
# Clean with utility
./cloudlogin --sanitize-aws-config

# Or manually
perl -ne 'print "$.:$_" if /[^\x09\x0A\x0D\x20-\x7E]/' ~/.aws/config
```

### Issue: Kubernetes contexts not showing

**Solution:**
```bash
# Check KUBECONFIG
echo $KUBECONFIG  # Should not be empty
kubectl config get-contexts

# Or set it
export KUBECONFIG=~/.kube/config
./cloudlogin
```

### Issue: TUI doesn't build

**Solution:**
```bash
# Check Bubble Tea version
grep -i bubbletea go.mod

# Should be v0.26.6 or compatible
go get github.com/charmbracelet/bubbletea@v0.26.6

# Rebuild
cd cmd/awsconfig-tui && go build -o ../../bin/cloudlogin-awsconfig-tui main.go
```

## Release Process

### Create a new version

```bash
# 1. Update version in code (if needed)
# Edit main.go if version const exists

# 2. Create git tag
git tag -a v0.1.0 -m "Release v0.1.0"

# 3. Push tag (triggers GitHub Actions)
git push origin v0.1.0

# 4. GitHub Actions builds and releases automatically
# Check status at: https://github.com/afeldman/cloudlogin/actions
```

### Manual local build

```bash
# Test release build without publishing
make test-release

# Check output
ls -lh dist/
```

## Contributing

1. Fork repository
2. Create feature branch: `git checkout -b feature/my-feature`
3. Make changes following conventions above
4. Format code: `gofmt -w .`
5. Run tests: `go test ./...`
6. Commit: `git commit -am "Add my feature"`
7. Push: `git push origin feature/my-feature`
8. Create Pull Request with description

## Resources

- [Fyne Documentation](https://developer.fyne.io/)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [AWS SDK for Go](https://docs.aws.amazon.com/sdk-for-go/)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
