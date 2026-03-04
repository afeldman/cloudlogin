package aws

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/afeldman/cloudlogin/pkg/provider"
)

// AWSProvider implementiert das CloudProvider Interface für AWS
type AWSProvider struct {
}

// NewAWSProvider erstellt eine neue AWS Provider Instanz
func NewAWSProvider() *AWSProvider {
	return &AWSProvider{}
}

// Name gibt den Namen des Providers zurück
func (a *AWSProvider) Name() string {
	return "AWS"
}

// GetCredentials liest AWS Profile aus ~/.aws/config
func (a *AWSProvider) GetCredentials() ([]provider.Credential, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".aws", "config")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("~/.aws/config nicht gefunden: %w", err)
	}
	defer f.Close()

	var credentials []provider.Credential
	var current *provider.Credential

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				credentials = append(credentials, *current)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			name = strings.TrimPrefix(name, "profile ")
			current = &provider.Credential{
				ID:          name,
				DisplayName: name,
				Details:     make(map[string]string),
			}
		} else if current != nil {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				switch k {
				case "region":
					current.Region = v
				case "sso_start_url":
					current.Details["sso_url"] = v
				}
			}
		}
	}
	if current != nil {
		credentials = append(credentials, *current)
	}

	return credentials, scanner.Err()
}

// Login führt AWS SSO Login durch
func (a *AWSProvider) Login(profileName string, logFn func(string)) provider.LoginResult {
	logFn(fmt.Sprintf("🔐 AWS SSO Login für Profil: %s", profileName))

	cmd := exec.Command("aws", "sso", "login", "--profile", profileName)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	msg := string(output)

	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return provider.LoginResult{Success: false, Message: msg}
	}
	logFn(fmt.Sprintf("✅ Erfolgreich eingeloggt: %s", profileName))
	return provider.LoginResult{Success: true, Message: msg}
}

// TestConnection testet die AWS Verbindung
func (a *AWSProvider) TestConnection(logFn func(string)) {
	logFn("🔍 Teste AWS Verbindung...")

	// Versuche Credentials zu exportieren
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	output, err := cmd.CombinedOutput()

	if err != nil {
		logFn(fmt.Sprintf("❌ Nicht authentifiziert: %s", string(output)))
		return
	}

	logFn(fmt.Sprintf("✅ AWS Authentifizierung gültig:"))
	logFn("  " + strings.TrimSpace(string(output)))
}
