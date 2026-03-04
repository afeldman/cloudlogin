package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AWSProfile represents a named AWS credential profile from ~/.aws/config.
//
// Fields:
//   - Name: Profile name (e.g., "default", "lynqtech-dev-administratoraccess")
//   - Region: AWS region (e.g., "eu-central-1", "us-east-1")
//   - SSOUrl: AWS SSO start URL for authentication
//
// Profiles with a non-empty SSOUrl will use SSO authentication via 'aws sso login'.
// Profiles without SSOUrl use standard AWS credential authentication.
type AWSProfile struct {
	Name   string
	Region string
	SSOUrl string
}

// parseAWSConfig reads and parses the AWS config file from ~/.aws/config.
//
// The function:
//  1. Opens ~/.aws/config in the user's home directory
//  2. Parses INI-style profile sections [profile name]
//  3. Extracts region and sso_start_url from each profile
//  4. Returns all profiles sorted by name
//
// Returns an error if:
//   - ~/.aws/config does not exist or cannot be read
//   - INI parsing fails
//
// Example:
//
//	profiles, err := parseAWSConfig()
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, profile := range profiles {
//		fmt.Printf("Profile: %s (Region: %s)\n", profile.Name, profile.Region)
//	}
func parseAWSConfig() ([]AWSProfile, error) {
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

	var profiles []AWSProfile
	var current *AWSProfile
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				profiles = append(profiles, *current)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			name = strings.TrimPrefix(name, "profile ")
			current = &AWSProfile{Name: name}
		} else if current != nil {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				switch k {
				case "region":
					current.Region = v
				case "sso_start_url":
					current.SSOUrl = v
				}
			}
		}
	}
	if current != nil {
		profiles = append(profiles, *current)
	}
	return profiles, scanner.Err()
}

// awsEnv constructs environment variables for AWS CLI commands.
//
// Sets:
//   - AWS_PROFILE: The name of the AWS profile to use
//   - AWS_REGION: The AWS region from the profile (if set)
//
// This is combined with os.Environ() to provide a clean environment
// for AWS CLI operations.
//
// Example:
//
//	cmd := exec.Command("aws", "sts", "get-caller-identity")
//	cmd.Env = awsEnv(profile)
//	cmd.Run()
func awsEnv(profile AWSProfile) []string {
	env := os.Environ()
	env = append(env, fmt.Sprintf("AWS_PROFILE=%s", profile.Name))
	if profile.Region != "" {
		env = append(env, fmt.Sprintf("AWS_REGION=%s", profile.Region))
	}
	return env
}

// loginAWS authenticates to AWS using SSO or implicit credentials.
//
// Behavior:
//   - If profile has SSOUrl: Runs 'aws sso login' for SSO authentication
//   - Otherwise: Runs 'aws sts get-caller-identity' to verify credentials
//
// Returns a LoginResult indicating success or failure.
// logFn receives status messages throughout the process.
//
// Example:
//
//	profile := AWSProfile{
//		Name:   "lynqtech-dev",
//		Region: "eu-central-1",
//		SSOUrl: "https://lynqtech.awsapps.com/start",
//	}
//	result := loginAWS(profile, func(msg string) {
//		fmt.Println(msg)
//	})
//	if result.Success {
//		fmt.Println("Login successful")
//	} else {
//		fmt.Printf("Login failed: %s\n", result.Message)
//	}
func loginAWS(profile AWSProfile, logFn func(string)) LoginResult {
	logFn(fmt.Sprintf("🔐 AWS SSO Login für Profil: %s", profile.Name))

	var cmd *exec.Cmd
	if profile.SSOUrl != "" {
		cmd = exec.Command("aws", "sso", "login")
	} else {
		// Normales Profil - einfach credentials prüfen
		cmd = exec.Command("aws", "sts", "get-caller-identity")
	}

	cmd.Env = awsEnv(profile)
	output, err := cmd.CombinedOutput()
	msg := string(output)

	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return LoginResult{false, msg}
	}
	logFn(fmt.Sprintf("✅ Erfolgreich eingeloggt: %s", profile.Name))
	return LoginResult{true, msg}
}

// testAWSConnection verifies AWS credentials and access for a given profile.
//
// Executes 'aws sts get-caller-identity' with the profile's environment.
// This checks if the AWS credentials are valid and active.
//
// Returns user and account information on success.
// Returns error message on authentication failure.
//
// logFn receives status messages and authentication details.
//
// Example:
//
//	profile := AWSProfile{
//		Name:   "dev",
//		Region: "eu-central-1",
//	}
//	testAWSConnection(profile, func(msg string) {
//		fmt.Println(msg)
//	})
//	// Output:
//	// 🔍 Teste AWS Verbindung für: dev
//	// ✅ Eingeloggt als: arn:aws:iam::123456789:user/dev-user
func testAWSConnection(profile AWSProfile, logFn func(string)) {
	logFn(fmt.Sprintf("🔍 Teste AWS Verbindung für: %s", profile.Name))
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "text")
	cmd.Env = awsEnv(profile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logFn(fmt.Sprintf("❌ Nicht eingeloggt: %s", string(output)))
		return
	}
	logFn(fmt.Sprintf("✅ Eingeloggt als: %s", strings.TrimSpace(string(output))))
}
