package aws

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/afeldman/cloudlogin/internal/shell"
)

// AWSProfile represents a named AWS credential profile from ~/.aws/config.
type AWSProfile struct {
	Name      string
	Region    string
	SSOUrl    string
	AccountID string
}

// ParseAWSConfig reads and parses all profiles from ~/.aws/config.
func ParseAWSConfig() ([]AWSProfile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(home, ".aws", "config"))
	if err != nil {
		return nil, fmt.Errorf("~/.aws/config nicht gefunden: %w", err)
	}
	defer f.Close()

	var profiles []AWSProfile
	var current *AWSProfile
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				profiles = append(profiles, *current)
			}
			name := strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"), "profile ")
			current = &AWSProfile{Name: name}
		} else if current != nil {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				switch k {
				case "region":
					current.Region = v
				case "sso_start_url":
					current.SSOUrl = v
				case "sso_account_id":
					current.AccountID = v
				}
			}
		}
	}
	if current != nil {
		profiles = append(profiles, *current)
	}
	return profiles, scanner.Err()
}

// AWSEnv builds the environment for AWS CLI commands using the given profile.
func AWSEnv(profile AWSProfile) []string {
	env := os.Environ()
	env = append(env, fmt.Sprintf("AWS_PROFILE=%s", profile.Name))
	if profile.Region != "" {
		env = append(env, fmt.Sprintf("AWS_REGION=%s", profile.Region))
	}
	return env
}

// LoginAWS authenticates to AWS using SSO or implicit credentials.
func LoginAWS(profile AWSProfile, logFn func(string)) shell.LoginResult {
	logFn(fmt.Sprintf("🔐 AWS SSO Login für Profil: %s", profile.Name))
	var cmd *exec.Cmd
	if profile.SSOUrl != "" {
		cmd = exec.Command("aws", "sso", "login")
	} else {
		cmd = exec.Command("aws", "sts", "get-caller-identity")
	}
	cmd.Env = AWSEnv(profile)
	output, err := cmd.CombinedOutput()
	msg := string(output)
	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return shell.LoginResult{Success: false, Message: msg}
	}
	logFn(fmt.Sprintf("✅ Erfolgreich eingeloggt: %s", profile.Name))
	return shell.LoginResult{Success: true, Message: msg}
}

// TestAWSConnection verifies AWS credentials for the given profile.
func TestAWSConnection(profile AWSProfile, logFn func(string)) {
	logFn(fmt.Sprintf("🔍 Teste AWS Verbindung für: %s", profile.Name))
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "text")
	cmd.Env = AWSEnv(profile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logFn(fmt.Sprintf("❌ Nicht eingeloggt: %s", string(output)))
		return
	}
	logFn(fmt.Sprintf("✅ Eingeloggt als: %s", strings.TrimSpace(string(output))))
}
