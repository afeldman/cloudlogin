// Package main provides cloudlogin - a multi-mode AWS and Kubernetes credential manager.
//
// cloudlogin supports three interaction modes:
//
// 1. GUI Mode (Default)
//
//	Interactive Fyne-based graphical interface with tabs for AWS authentication,
//	Kubernetes context switching, and quick actions.
//
//	Example:
//	  go run main.go
//
// 2. CLI Mode
//
//	Command-line interface for automation and scripts.
//
//	Available flags:
//	  --update-aws-config      Synchronize AWS SSO profiles to ~/.aws/config
//	  --sanitize-aws-config    Clean invalid characters from ~/.aws/config
//
//	Example:
//	  cloudlogin --update-aws-config
//
// 3. TUI Mode
//
//	Terminal User Interface (Bubble Tea) for interactive terminal sessions.
//
//	Example:
//	  go run ./cmd/awsconfig-tui/main.go
//
// Features:
//   - AWS SSO profile discovery and caching
//   - Kubernetes context management
//   - AWS credential validation
//   - Configuration file sanitization
//   - Multi-platform support (macOS, Linux)
//
// The application manages configuration files:
//   - ~/.aws/config        AWS credential profiles
//   - ~/.kube/config       Kubernetes contexts (KUBECONFIG)
//   - ~/.aws/sso/cache/*   SSO access tokens
package main

import (
	"fmt"
	"os"

	"github.com/afeldman/cloudlogin/pkg/awsconfig"
)

// Build information (set by ldflags during build).
// These values are injected during compilation via -ldflags.
//
// Example build command:
//
//	go build -ldflags \
//	  "-X main.version=v1.0.0 \
//	   -X main.commit=abc123 \
//	   -X main.date=2024-01-15" \
//	  -o cloudlogin main.go
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// main is the entry point for cloudlogin.
//
// Behavior:
//  1. If command-line flags provided: Execute CLI mode and exit
//     - --update-aws-config: Sync SSO profiles
//     - --sanitize-aws-config: Clean AWS config file
//  2. Otherwise: Launch GUI mode with Fyne
//
// GUI Features:
//   - AWS Tab: Profile management and SSO login
//   - Kubernetes Tab: Context switching
//   - Quick Actions: Browser links, shell commands
//   - Integrated logging console
//
// Exit codes:
//   - 0: Success
//   - 1: CLI command failed
//
// Example CLI usage:
//
//	$ cloudlogin --update-aws-config
//	🔄 AWS Config aktualisieren...
//	✅ AWS Config aktualisiert
//
// Example GUI usage:
//
//	$ cloudlogin
//	# Opens interactive GUI window
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--update-aws-config":
			if err := awsconfig.UpdateFromSSO(func(msg string) { fmt.Println(msg) }); err != nil {
				fmt.Fprintf(os.Stderr, "AWS Config Update fehlgeschlagen: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ AWS Config aktualisiert")
			return
		case "--sanitize-aws-config":
			if err := awsconfig.SanitizeConfigFile(func(msg string) { fmt.Println(msg) }); err != nil {
				fmt.Fprintf(os.Stderr, "AWS Config Bereinigung fehlgeschlagen: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ AWS Config bereinigt")
			return
		}
	}

	// Launch GUI
	runGUI()
}
