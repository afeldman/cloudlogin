# AWS SSO Configuration Management Package

Das `awsconfig` Package verwaltet AWS SSO (Single Sign-On) Profile und Config-Dateien. Es ist das Herzstück des cloudlogin Tools und wird von allen Modi (GUI, CLI, TUI) verwendet.

## API Overview

### `UpdateFromSSO(logFn func(string)) error`

Aktualisiert die AWS Config-Datei (`~/.aws/config`) mit Profilen aus AWS SSO.

**Flow:**
1. Suche SSO Access Token aus `~/.aws/sso/cache/`
2. Lade SSO Accounts via AWS SSO API
3. Lade Roles pro Account via AWS SSO API
4. Generiere Profile im Format `{account-name}-{role-name}`
5. Lese existierende `~/.aws/config`
6. Merge: Behalte nicht-verwaltete Sections, aktualisiere verwaltete Section
7. Schreibe zurück mit Original-Permissions

**Parameter:**
- `logFn func(string)`: Callback für Log-Messages (wird aufgerufen für jeden Status-Update)

**Rückgabe:**
- `error`: Nil bei Erfolg, ansonsten Error-Message

**Beispiel:**
```go
err := awsconfig.UpdateFromSSO(func(msg string) {
    fmt.Println(msg)  // Wird aufgerufen bei jedem Status-Update
})
if err != nil {
    // Fehlerbehandlung
}
```

### `SanitizeConfigFile(logFn func(string)) error`

Bereinigt die AWS Config-Datei von Kontrollezeichen, die den AWS CLI Parser blockieren.

**Was wird bereinigt:**
- Character mit Codes 0x00-0x1F AUSGENOMMEN:
  - 0x09 (Tab)
  - 0x0A (Line Feed)
  - 0x0D (Carriage Return)

**Behavior:**
- Erstellt Backup: `~/.aws/config.bak`
- Schreibt nur bei Änderungen (nutzt Original-Permissions)
- Retourniert Silent-Success wenn kein Cleaning nötig

**Beispiel:**
```go
err := awsconfig.SanitizeConfigFile(func(msg string) {
    fmt.Println(msg)
})
if err != nil {
    // Fehlerbehandlung
    fmt.Fprintf(os.Stderr, "Sanitization failed: %v\n", err)
}
```

## Internal Functions

### `findSSOToken() (*ssoCacheEntry, error)`

Sucht und validiert einen SSO Token aus `~/.aws/sso/cache/`.

**Prozess:**
1. Lese alle `.json` Dateien in `~/.aws/sso/cache/`
2. Parse als JSON (ssoCacheEntry)
3. Überprüfe `expiresAt` gegen aktuelle Zeit
4. Wähle letzten gültigen (nicht-abgelaufenen) Token

**Rückgabe:**
- Pointer zur ssoCacheEntry (enthält `accessToken`)
- Error wenn kein gültiger Token gefunden

### `listSSOAccounts(env []string) (accounts, error)`

Ruft alle SSO-Konten via AWS SSO API auf.

**AWS CLI Call:**
```bash
aws sso list-accounts --max-results 100
```

**Features:**
- Pagination für viele Accounts (100er Chunks)
- Environment-Gefiltert (keine AWS_PROFILE Interference)
- Sortiert nach Account-ID

### `listSSORoles(accountId string, env []string) (roles, error)`

Ruft alle Rollen für einen Account via AWS SSO API auf.

**AWS CLI Call:**
```bash
aws sso list-account-roles --account-id {id} --max-results 100
```

**Features:**
- Pagination möglich, aber meist <100 Roles
- Environment-Gefiltert
- Pro Account aufgerufen

### `runAWSJSON(args []string, env []string) (rawJSON, error)`

Wrapper um AWS CLI mit JSON parsing.

**Features:**
- Masked Error Messages: `--access-token xxx` → `--access-token ***`
- Custom Environment
- Structured Output parsing

Beispiel interner Aufruf:
```go
data, err := runAWSJSON(
    []string{"sso", "list-accounts", "--max-results", "100"},
    awsSSOEnv(),
)
```

## Environment Handling

### `awsSSOEnv() []string`

Retourniert gefilterte Environment für SSO Operations.

**Was wird gefiltert:**
- AWS_PROFILE entfernt (um alte Profiles zu vermeiden)
- AWS_DEFAULT_PROFILE entfernt
- Temporary AWS_CONFIG_FILE gesetzt (leere temporäre Datei)

**Grund:** Wenn der Nutzer eine kaputte `~/.aws/config` hat, würde `aws sso list-accounts` sofort fehlschlagen. Mit einer temp-datei umgehen wir dieses Problem.

### `filterEnv(env []string, keys ...string) []string`

Entfernt spezifische Environment-Variablen completely (nicht set to empty).

Beispiel:
```go
env := filterEnv(os.Environ(), "AWS_PROFILE", "AWS_DEFAULT_PROFILE")
```

## Helper Functions

### `sanitizeConfig(raw string) string`

Entfernt Control-Zeichen aus Config-String.

Pseudo-Code:
```go
for each char in raw {
    if char < 0x20 && char not in [0x09, 0x0A, 0x0D] {
        skip // remove
    }
}
```

### `maskSensitiveArgs(args []string) []string`

Maskiert `--access-token` Werte in Error-Messages.

Beispiel:
```
Before: ["sso", "list-accounts", "--access-token", "..."]
After:  ["sso", "list-accounts", "--access-token", "***"]
```

## Type Definitions

### `ssoCacheEntry`

AWS SSO Token Cache format:
```go
type ssoCacheEntry struct {
    AccessToken  string `json:"accessToken"`
    ExpiresAt    int64  `json:"expiresAt"`
    // ... weitere fields
}
```

### `awsSSOAccount`

Account aus AWS SSO API:
```go
type awsSSOAccount struct {
    AccountId   string `json:"accountId"`
    AccountName string `json:"accountName"`
}
```

### `awsSSORole`

Role aus AWS SSO API:
```go
type awsSSORole struct {
    RoleName    string `json:"roleName"`
    AccountId   string `json:"accountId"`
}
```

## Error Handling

Alle Funktionen retournieren aussagekräftige Fehlermeldungen:

```go
if err := awsconfig.UpdateFromSSO(logFn); err != nil {
    switch {
    case strings.Contains(err.Error(), "access token"):
        // Handle expired token
    case strings.Contains(err.Error(), "not found"):
        // Handle missing config
    default:
        // Generic error
    }
}
```

## Logging Conventions

Messages follgen Pattern: `{EMOJI} {Status Message}`

Beispiele:
- `🔍 Suche SSO Token...` (Info)
- `✅ SSO Config aktualisiert` (Success)
- `❌ Token abgelaufen` (Error)
- `ℹ️  Merge mit Profil: dev-admin` (Details)

Diese Messages werden via `logFn` gesendet und von GUI/CLI/TUI unterschiedlich behandelt.

## Development Notes

### Hinzufügen neuer Features

Wenn du ein neues AWS-Feature hinzufügst (z.B. neues API Call):

1. Schreibe Funktion mit `logFn func(string)` Parameter
2. Nutze `runAWSJSON()` für AWS CLI Calls
3. Berücksichtige Fehler und log via `logFn()`
4. Testen via:
   ```bash
   # CLI
   ./cloudlogin --update-aws-config
   
   # GUI
   ./cloudlogin  # -> AWS Tab -> Button
   
   # TUI
   ./bin/cloudlogin-awsconfig-tui
   ```

### AWS Config Format

Das Tool basiert auf INI-Format mit zwei Sections:

```ini
# Non-managed section (user-edited, wird nicht verändert)
[profile myprofile]
region = eu-central-1

# Managed section (von UpdateFromSSO generiert)
# ===== CLOUDLOGIN MANAGED - START =====
[profile lynqtech-dev-administratoraccess]
sso_start_url = https://...
sso_region = eu-central-1
sso_account_id = 123456789
sso_role_name = AdministratorAccess
region = eu-central-1

[profile lynqtech-prod-readonlyaccess]
...
# ===== CLOUDLOGIN MANAGED - END =====
```

## Example Integration

### In GUI (main.go)

```go
awsUpdateBtn := widget.NewButtonWithIcon(
    "AWS Config aktualisieren", 
    theme.CheckIcon(), 
    func() {
        go func() {
            if err := awsconfig.UpdateFromSSO(logFn); err != nil {
                logFn(fmt.Sprintf("❌ Fehler: %v", err))
                return
            }
            // Refresh profiles
            profiles, _ = parseAWSConfig()
        }()
    },
)
```

### In CLI (main.go)

```go
case "--update-aws-config":
    if err := awsconfig.UpdateFromSSO(func(msg string) { fmt.Println(msg) }); err != nil {
        fmt.Fprintf(os.Stderr, "AWS Config Update failed: %v\n", err)
        os.Exit(1)
    }
    return
```

### In TUI (cmd/awsconfig-tui/main.go)

```go
func startUpdate(logCh chan tea.Msg) {
    go func() {
        if err := awsconfig.UpdateFromSSO(func(msg string) {
            logCh <- logMsg(msg)
        }); err != nil {
            logCh <- doneMsg{err}
            return
        }
        logCh <- doneMsg{nil}
    }()
}
```

## Testing

Für lokale Tests:

```bash
# Test mit echten AWS Credentials
cd pkg/awsconfig
aws sso login  # Stelle sicher SSO Token vorhanden ist
go test -v ./...  # Falls Tests existieren
```

Manuelles Testing:

```bash
# Test CLI
./cloudlogin --update-aws-config

# Test GUI
./cloudlogin  # Click AWS Config button

# Test TUI  
./bin/cloudlogin-awsconfig-tui  # Press Enter to start
```
