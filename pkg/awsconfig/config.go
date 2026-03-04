// Package awsconfig provides AWS SSO profile management and AWS config file operations.
//
// This package handles:
//   - AWS SSO profile discovery (accounts and roles via AWS SSO API)
//   - AWS config file parsing and updates
//   - Configuration file sanitization (removing invalid characters)
//   - Token caching and expiration handling
//
// Core Functions:
//   - UpdateFromSSO: Synchronize AWS profiles from SSO to ~/.aws/config
//   - SanitizeConfigFile: Remove invalid characters from AWS config
//
// Example usage:
//
//	logFn := func(msg string) { fmt.Println(msg) }
//
//	// Update AWS profiles from SSO
//	if err := UpdateFromSSO(logFn); err != nil {
//		log.Fatal(err)
//	}
//
//	// Sanitize config file  (remove invalid characters)
//	if err := SanitizeConfigFile(logFn); err != nil {
//		log.Fatal(err)
//	}
//
// The package uses callback-based logging (logFn) to support multiple UI modes:
// - GUI (Fyne): Updates log display in real-time
// - CLI: Prints to stdout/stderr
// - TUI (Bubble Tea): Sends log messages through channels
package awsconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SanitizeConfigFile removes non-printable characters from ~/.aws/config.
//
// This fixes "Unable to parse config file" errors caused by invisible bytes
// (control characters 0x00-0x1F except tab/newline/CR) embedded in the file.
//
// Behavior:
//  1. Reads ~/.aws/config
//  2. Removes all non-printable characters
//  3. Creates backup as ~/.aws/config.bak if changes needed
//  4. Writes cleaned version preserving original file permissions
//  5. Returns early if no cleaning needed
//
// logFn receives status messages during the process.
// Returns error if file cannot be read or written.
//
// Example:
//
//	err := SanitizeConfigFile(func(msg string) {
//		fmt.Println(msg)
//	})
//	// May output:
//	// "✅ AWS Config bereinigt (Backup: ~/.aws/config.bak)"
//	// or
//	// "✅ AWS Config ist bereits sauber"
func SanitizeConfigFile(logFn func(string)) error {
	configPath := filepath.Join(os.Getenv("HOME"), ".aws", "config")
	existing, err := os.ReadFile(configPath)
	if err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	clean := sanitizeConfig(string(existing))
	if clean == string(existing) {
		logFn("✅ AWS Config ist bereits sauber")
		return nil
	}

	backupPath := configPath + ".bak"
	if err := os.WriteFile(backupPath, existing, 0o600); err != nil {
		logFn(fmt.Sprintf("❌ Backup fehlgeschlagen: %v", err))
		return err
	}

	mode := os.FileMode(0o600)
	if info, err := os.Stat(configPath); err == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(configPath, []byte(clean), mode); err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	logFn(fmt.Sprintf("✅ AWS Config bereinigt (Backup: %s)", backupPath))
	return nil
}

// UpdateFromSSO synchronizes AWS SSO profiles to ~/.aws/config.
//
// This function:
//  1. Retrieves SSO access token from ~/.aws/sso/cache/
//  2. Lists all AWS accounts available via SSO
//  3. Lists all roles for each account
//  4. Generates profile names: {account-name}-{role-name}
//  5. Reads existing ~/.aws/config
//  6. Merges: Preserves non-managed profiles, updates managed section
//  7. Sanitizes invalid characters
//  8. Writes back with original permissions
//
// Environment variables:
//   - AWS_SSO_REGION: Override default SSO region (default: eu-central-1)
//   - AWS_SSO_START_URL: Override SSO start URL (default: lynqtech)
//
// logFn is called with progress messages and status updates.
// Returns error if SSO token missing, AWS API fails, or file I/O error.
//
// Example:
//
//	err := UpdateFromSSO(func(msg string) {
//		fmt.Println(msg)
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	// Output:
//	// 🔄 AWS Config aktualisieren (SSO: https://lynqtech.awsapps.com/start)
//	// ✅ AWS Config aktualisiert
func UpdateFromSSO(logFn func(string)) error {
	region := getEnvOrDefault("AWS_SSO_REGION", defaultSSORegion)
	startURL := getEnvOrDefault("AWS_SSO_START_URL", defaultSSOStartURL)

	logFn(fmt.Sprintf("🔄 AWS Config aktualisieren (SSO: %s)", startURL))
	accessToken, err := findSSOToken(startURL)
	if err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	accounts, err := listSSOAccounts(accessToken, region)
	if err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	entries, err := listSSOProfileEntries(accessToken, region, accounts)
	if err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	if len(entries) == 0 {
		err := errors.New("keine SSO Profile gefunden")
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	configPath := filepath.Join(os.Getenv("HOME"), ".aws", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}

	existing, _ := os.ReadFile(configPath)
	merged := mergeAWSConfig(sanitizeConfig(string(existing)), buildAWSConfig(entries, region, startURL))

	mode := os.FileMode(0o600)
	if info, err := os.Stat(configPath); err == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(configPath, []byte(merged), mode); err != nil {
		logFn(fmt.Sprintf("❌ %v", err))
		return err
	}
	logFn(fmt.Sprintf("✅ AWS Config aktualisiert"))
	return nil
}

func buildAWSConfig(entries []awsProfileEntry, region, startURL string) string {
	var b strings.Builder
	for _, entry := range entries {
		b.WriteString("\n")
		fmt.Fprintf(&b, "# %s / %s\n", entry.AccountName, entry.RoleName)
		fmt.Fprintf(&b, "[profile %s]\n", entry.Name)
		fmt.Fprintf(&b, "sso_start_url = %s\n", startURL)
		fmt.Fprintf(&b, "sso_region = %s\n", region)
		fmt.Fprintf(&b, "sso_account_id = %s\n", entry.AccountID)
		fmt.Fprintf(&b, "sso_role_name = %s\n", entry.RoleName)
		fmt.Fprintf(&b, "region = %s\n", region)
		b.WriteString("output = json\n")
	}
	return strings.TrimSpace(b.String())
}

func mergeAWSConfig(existing, generated string) string {
	block := fmt.Sprintf("%s\n%s\n%s\n", awsConfigStartTag, generated, awsConfigEndTag)
	if strings.Contains(existing, awsConfigStartTag) && strings.Contains(existing, awsConfigEndTag) {
		before := strings.SplitN(existing, awsConfigStartTag, 2)[0]
		after := strings.SplitN(existing, awsConfigEndTag, 2)[1]
		return strings.TrimRight(before, "\n") + "\n\n" + block + strings.TrimLeft(after, "\n")
	}
	if strings.TrimSpace(existing) == "" {
		return block
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + block
}
