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
