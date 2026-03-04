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

type ssoCacheEntry struct {
	StartURL    string `json:"startUrl"`
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

type awsSSOAccount struct {
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
}

type awsSSOAccountsResponse struct {
	AccountList []awsSSOAccount `json:"accountList"`
	NextToken   string          `json:"nextToken"`
}

type awsSSORole struct {
	RoleName string `json:"roleName"`
}

type awsSSORolesResponse struct {
	RoleList  []awsSSORole `json:"roleList"`
	NextToken string       `json:"nextToken"`
}

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

// SanitizeConfigFile removes non-printable characters from ~/.aws/config and writes a backup.
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

// UpdateFromSSO regenerates the managed section in ~/.aws/config using AWS SSO data.
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
	return nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

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
