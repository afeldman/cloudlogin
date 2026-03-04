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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ssoCacheEntry represents an entry from ~/.aws/sso/cache/*.json
type ssoCacheEntry struct {
	StartURL    string `json:"startUrl"`
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

// awsSSOAccount represents an AWS account from SSO API response
type awsSSOAccount struct {
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
}

type awsSSOAccountsResponse struct {
	AccountList []awsSSOAccount `json:"accountList"`
	NextToken   string          `json:"nextToken"`
}

// awsSSORole represents an AWS role from SSO API response
type awsSSORole struct {
	RoleName string `json:"roleName"`
}

type awsSSORolesResponse struct {
	RoleList  []awsSSORole `json:"roleList"`
	NextToken string       `json:"nextToken"`
}

// awsProfileEntry represents a single AWS profile to be written to config file
type awsProfileEntry struct {
	Name        string
	AccountID   string
	AccountName string
	RoleName    string
}

const (
	defaultSSORegion   = "eu-central-1"
	defaultSSOStartURL = "https://lynqtech.awsapps.com/start"
	awsConfigStartTag  = "# cloudlogin-managed-start"
	awsConfigEndTag    = "# cloudlogin-managed-end"
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

// getEnvOrDefault retrieves an environment variable or returns a default value.
//
// Returns the trimmed environment variable if non-empty, otherwise returns fallback.
// Useful for optional configuration that should have sensible defaults.
//
// Example:
//
//	region := getEnvOrDefault("AWS_SSO_REGION", "eu-central-1")
func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// findSSOToken retrieves the current SSO access token from ~/.aws/sso/cache/.
//
// Search process:
//  1. Scans all *.json files in ~/.aws/sso/cache/
//  2. Parses startUrl, accessToken, and expiresAt
//  3. Filters for tokens matching the given startURL
//  4. Selects token with latest expiresAt (prefers non-expired)
//
// Returns the access token string or error if:
//   - No cache files found
//   - No tokens for the given startURL
//   - All tokens are expired
//
// Example:
//
//	token, err := findSSOToken("https://lynqtech.awsapps.com/start")
//	if err != nil {
//		log.Fatal("SSO token not found:", err)
//	}
func findSSOToken(startURL string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(home, ".aws", "sso", "cache")
	paths, err := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	if err != nil {
		return "", err
	}

	var bestToken string
	var bestExpiry time.Time
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var entry ssoCacheEntry
		if json.Unmarshal(b, &entry) != nil {
			continue
		}
		if entry.StartURL != startURL || entry.AccessToken == "" {
			continue
		}
		if entry.ExpiresAt != "" {
			exp, err := time.Parse(time.RFC3339, entry.ExpiresAt)
			if err == nil {
				if time.Now().After(exp) {
					continue
				}
				if bestToken == "" || exp.After(bestExpiry) {
					bestToken = entry.AccessToken
					bestExpiry = exp
				}
				continue
			}
		}
		if bestToken == "" {
			bestToken = entry.AccessToken
		}
	}
	if bestToken == "" {
		return "", errors.New("kein gültiger SSO Token gefunden (aws sso login)")
	}
	return bestToken, nil
}

// listSSOAccounts retrieves all AWS accounts available via SSO API with pagination support.
//
// API Call: aws sso list-accounts
//
// Parameters:
//   - accessToken: SSO access token from cache
//   - region: AWS region for SSOregion (usually eu-central-1)
//
// Behavior:
//  1. Makes paginated requests to AWS SSO API
//  2. Handles nextToken for accounts > 100
//  3. Filters and sorts by account ID
//  4. Returns slice of all available accounts
//
// Returns error if AWS CLI fails or JSON parsing error.
//
// Example:
//
//	accounts, err := listSSOAccounts(token, "eu-central-1")
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, acct := range accounts {
//		fmt.Printf("Account: %s (%s)\n", acct.AccountName, acct.AccountID)
//	}
func listSSOAccounts(accessToken, region string) ([]awsSSOAccount, error) {
	var accounts []awsSSOAccount
	var nextToken string
	for {
		args := []string{"sso", "list-accounts", "--access-token", accessToken, "--region", region, "--output", "json"}
		if nextToken != "" {
			args = append(args, "--next-token", nextToken)
		}
		out, err := runAWSJSON(args...)
		if err != nil {
			return nil, err
		}
		var resp awsSSOAccountsResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, err
		}
		accounts = append(accounts, resp.AccountList...)
		if resp.NextToken == "" {
			break
		}
		nextToken = resp.NextToken
	}
	sort.Slice(accounts, func(i, j int) bool {
		return strings.ToLower(accounts[i].AccountName) < strings.ToLower(accounts[j].AccountName)
	})
	return accounts, nil
}

// listSSOProfileEntries generates AWS profile entries from accounts and roles.
//
// This function:
//  1. Iterates through all accounts
//  2. Lists roles for each account via AWS SSO API
//  3. Generates profile name: {account-name}-{role-name} (slugified)
//  4. Handles duplicate names by appending numeric suffix
//  5. Returns list of complete profile entries with metadata
//
// Profile names are slugified:
//   - Lowercase
//   - Spaces and special chars converted to hyphens
//   - Examples: "Lynqtech Dev" -> "lynqtech-dev"
//     "AdministratorAccess" -> "administratoraccess"
//
// Returns error if AWS API calls fail for any account/role.
//
// Example:
//
//	accounts := []awsSSOAccount{
//		{AccountID: "123456", AccountName: "Prod"},
//	}
//	entries, err := listSSOProfileEntries(token, "eu-central-1", accounts)
//	// entries[0].Name = "prod-administratoraccess"
func listSSOProfileEntries(accessToken, region string, accounts []awsSSOAccount) ([]awsProfileEntry, error) {
	var entries []awsProfileEntry
	seen := make(map[string]bool)
	for _, account := range accounts {
		roles, err := listSSORoles(accessToken, region, account.AccountID)
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			baseName := fmt.Sprintf("%s-%s", slugify(account.AccountName), slugify(role.RoleName))
			name := strings.Trim(baseName, "-")
			if name == "" {
				name = fmt.Sprintf("%s-%s", account.AccountID, slugify(role.RoleName))
			}
			if seen[name] {
				suffix := account.AccountID
				if len(suffix) > 4 {
					suffix = suffix[len(suffix)-4:]
				}
				name = fmt.Sprintf("%s-%s", name, suffix)
			}
			seen[name] = true
			entries = append(entries, awsProfileEntry{
				Name:        name,
				AccountID:   account.AccountID,
				AccountName: account.AccountName,
				RoleName:    role.RoleName,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// listSSORoles retrieves all AWS roles for a specific account via SSO API with pagination.
//
// API Call: aws sso list-account-roles --account-id {id}
//
// Parameters:
//   - accessToken: SSO access token from cache
//   - region: AWS region for SSO service
//   - accountID: AWS account ID to list roles for
//
// Returns:
//   - []awsSSORole: All available roles for the account
//   - error: AWS API error or JSON parsing error
//
// Pagination:
//   - Supports nextToken for accounts with >100 roles (rare)
//   - Automatically handles multiple API calls if needed
//
// Example:
//
//	roles, err := listSSORoles(token, "eu-central-1", "123456789")
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, role := range roles {
//		fmt.Printf("Role: %s\n", role.RoleName)
//	}
func listSSORoles(accessToken, region, accountID string) ([]awsSSORole, error) {
	var roles []awsSSORole
	var nextToken string
	for {
		args := []string{"sso", "list-account-roles", "--access-token", accessToken, "--account-id", accountID, "--region", region, "--output", "json"}
		if nextToken != "" {
			args = append(args, "--next-token", nextToken)
		}
		out, err := runAWSJSON(args...)
		if err != nil {
			return nil, err
		}
		var resp awsSSORolesResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, err
		}
		roles = append(roles, resp.RoleList...)
		if resp.NextToken == "" {
			break
		}
		nextToken = resp.NextToken
	}
	sort.Slice(roles, func(i, j int) bool {
		return strings.ToLower(roles[i].RoleName) < strings.ToLower(roles[j].RoleName)
	})
	return roles, nil
}

// runAWSJSON executes an AWS CLI command and returns parsed JSON output.
//
// This is a helper function that:
//  1. Executes 'aws' command with given arguments
//  2. Uses filtered environment to avoid profile/region interference
//  3. Masks sensitive arguments (--access-token) in error messages
//  4. Returns combined stdout/stderr as JSON bytes
//
// Parameters:
//   - args: Command and arguments (e.g., "sso", "list-accounts", "--output", "json")
//
// Returns:
//   - []byte: Raw JSON output from AWS CLI
//   - error: Command execution error with masked sensitive data
//
// Security:
//   - Access tokens replaced with *** in error messages
//   - Never leaks credentials to logs
//
// Example:
//
//	data, err := runAWSJSON("sso", "list-accounts", "--output", "json")
//	if err != nil {
//		// Error message will show: aws sso list-accounts --output json: ...
//		// NOT: aws sso list-accounts ... --access-token abc123
//		log.Fatal(err)
//	}
func runAWSJSON(args ...string) ([]byte, error) {
	cmd := exec.Command("aws", args...)
	cmd.Env = awsSSOEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "<keine Ausgabe>"
		}
		safeArgs := maskSensitiveArgs(args)
		return nil, fmt.Errorf("aws %s: %s", strings.Join(safeArgs, " "), msg)
	}
	return out, nil
}

// awsSSOEnv returns a filtered environment for AWS CLI commands.
//
// This environment:
//   - Removes AWS_PROFILE to prevent "profile not found" errors
//   - Removes AWS_DEFAULT_PROFILE for the same reason
//   - Uses a temporary empty AWS_CONFIG_FILE to bypass broken config files
//   - Preserves all other environment variables
//
// Rationale:
//   - If user's ~/.aws/config is corrupted, AWS CLI will fail to read any profiles
//   - Using a temp empty config prevents the CLI from trying to parse it
//   - The SSO endpoints don't need the config file; they get credentials from cache
//
// Returns []string representing filtered environment for os/exec Command.
//
// Example:
//
//	cmd := exec.Command("aws", "sso", "list-accounts")
//	cmd.Env = awsSSOEnv()  // Safe environment
//	cmd.Run()
func awsSSOEnv() []string {
	base := filterEnv(os.Environ(), "AWS_PROFILE", "AWS_DEFAULT_PROFILE")
	configFile, cleanup := tempAWSConfigFile()
	if cleanup != nil {
		defer cleanup()
	}
	if configFile != "" {
		base = append(base, fmt.Sprintf("AWS_CONFIG_FILE=%s", configFile))
	}
	return base
}

func filterEnv(env []string, keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[key+"="] = true
	}

	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		matched := false
		for prefix := range blocked {
			if strings.HasPrefix(kv, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}

func sanitizeConfig(raw string) string {
	clean := strings.TrimPrefix(raw, "\ufeff")
	var b strings.Builder
	b.Grow(len(clean))
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		if c == 0 {
			continue
		}
		if c == '\n' || c == '\r' || c == '\t' || c >= 32 {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func tempAWSConfigFile() (string, func()) {
	f, err := os.CreateTemp("", "cloudlogin-aws-config-*.ini")
	if err != nil {
		return "", nil
	}
	if _, err := f.WriteString("# cloudlogin temp config\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil
	}
	return f.Name(), func() {
		_ = os.Remove(f.Name())
	}
}

func maskSensitiveArgs(args []string) []string {
	masked := make([]string, len(args))
	copy(masked, args)
	for i := 0; i < len(masked); i++ {
		if masked[i] == "--access-token" && i+1 < len(masked) {
			masked[i+1] = "***"
		}
	}
	return masked
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

func slugify(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		switch r {
		case ' ', '-', '_', '.', '/':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
