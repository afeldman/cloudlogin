package awsconfig

import (
	"os"
	"strings"
)

// getEnvOrDefault retrieves an environment variable or returns a default value.
//
// Returns the trimmed environment variable if non-empty, otherwise returns fallback.
// Useful for optional configuration that should have sensible defaults.
//
// Example:
//
//	region := getEnvOrDefault("AWS_SSO_REGION", "eu-central-1")
// createBackup copies src to src+".bak", overwriting any previous backup.
func createBackup(src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to back up
		}
		return err
	}
	return os.WriteFile(src+".bak", data, 0o600)
}

func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
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
