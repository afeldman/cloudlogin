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
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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

// KubeContext represents a Kubernetes context from the KUBECONFIG.
//
// Fields:
//   - Name: Context name (e.g., "kubernetes-admin@cluster.local")
//   - Cluster: Cluster name
//   - User: User/authentication identity
//
// Contexts are used to switch between multiple Kubernetes clusters
// without manually updating KUBECONFIG.
type KubeContext struct {
	Name    string
	Cluster string
	User    string
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

// parseKubeContexts retrieves all available Kubernetes contexts from KUBECONFIG.
//
// The function:
//  1. Reads KUBECONFIG environment variable (defaults to ~/.kube/config)
//  2. Executes 'kubectl config get-contexts' to list contexts
//  3. Parses the output to extract context names
//
// Returns:
//   - []KubeContext: List of available Kubernetes contexts
//   - string: Path to the KUBECONFIG file used
//   - error: kubectl command error or file parsing error
//
// Example:
//
//	contexts, kubeconfig, err := parseKubeContexts()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Using KUBECONFIG: %s\n", kubeconfig)
//	fmt.Printf("Available contexts: %v\n", contexts)
func parseKubeContexts() ([]KubeContext, string, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	// kubectl verwenden um contexts zu lesen
	out, err := exec.Command("kubectl", "config", "get-contexts",
		"-o", "name", "--kubeconfig", kubeconfig).Output()
	if err != nil {
		return nil, kubeconfig, fmt.Errorf("kubectl nicht gefunden oder Fehler: %w", err)
	}

	var contexts []KubeContext
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			contexts = append(contexts, KubeContext{Name: line})
		}
	}
	return contexts, kubeconfig, nil
}

// getCurrentKubeContext returns the currently active Kubernetes context.
//
// Executes 'kubectl config current-context' and returns the context name.
// Returns an empty string if the command fails or no context is set.
//
// Example:
//
//	current := getCurrentKubeContext()
//	if current != "" {
//		fmt.Printf("Current context: %s\n", current)
//	}
func getCurrentKubeContext() string {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// LoginResult represents the outcome of an authentication or context-switching operation.
//
// Fields:
//   - Success: true if the operation completed without errors
//   - Message: Detailed message about the operation result
type LoginResult struct {
	Success bool
	Message string
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

// escapeAppleScript escapes special characters in a string for AppleScript compatibility.
//
// Escapes:
//   - Backslashes: \ -> \\
//   - Quotes: " -> \"
//
// This is necessary before embedding a string in an AppleScript tell block.
//
// Example:
//
//	cmd := "export AWS_PROFILE=dev; aws s3 ls"
//	escaped := escapeAppleScript(cmd)
//	// Result: export AWS_PROFILE=dev; aws s3 ls (with any quotes escaped)
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// openShellWithEnv opens a new terminal window with the given environment variables.
//
// Supported platforms:
//   - macOS: Uses AppleScript to open Terminal.app
//   - Linux: Tries x-terminal-emulator, gnome-terminal, konsole, xterm
//
// Environment variables should be in the format:
//   - "export KEY=VALUE"
//   - "KEY=VALUE"
//
// logFn receives status messages during execution.
//
// Example:
//
//	envLines := []string{
//		"export AWS_PROFILE=dev",
//		"export AWS_REGION=eu-central-1",
//	}
//	openShellWithEnv(envLines, func(msg string) {
//		fmt.Println(msg)
//	})
func openShellWithEnv(envLines []string, logFn func(string)) {
	if len(envLines) == 0 {
		logFn("❌ Keine ENV-Variablen gesetzt")
		return
	}

	cmdLine := strings.Join(envLines, "; ")
	var cmd *exec.Cmd
	var debugCmd string

	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf(
			`tell application "Terminal"
				activate
				do script "%s; exec zsh -l"
			end tell`,
			escapeAppleScript(cmdLine),
		)
		cmd = exec.Command("osascript", "-e", script)
		debugCmd = "osascript -e <script>"
	} else {
		shellCmd := fmt.Sprintf("%s; exec bash -l", cmdLine)
		if _, err := exec.LookPath("x-terminal-emulator"); err == nil {
			cmd = exec.Command("x-terminal-emulator", "-e", "bash", "-lc", shellCmd)
			debugCmd = "x-terminal-emulator -e bash -lc <cmd>"
		} else {
			for _, term := range []string{"gnome-terminal", "konsole", "xterm"} {
				if _, err := exec.LookPath(term); err == nil {
					switch term {
					case "gnome-terminal":
						cmd = exec.Command(term, "--", "bash", "-lc", shellCmd)
						debugCmd = "gnome-terminal -- bash -lc <cmd>"
					case "konsole":
						cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
						debugCmd = "konsole -e bash -lc <cmd>"
					case "xterm":
						cmd = exec.Command(term, "-e", "bash", "-lc", shellCmd)
						debugCmd = "xterm -e bash -lc <cmd>"
					}
					break
				}
			}
		}
	}

	if cmd == nil {
		logFn("❌ Kein Terminal gefunden (x-terminal-emulator, gnome-terminal, konsole, xterm)")
		return
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "<keine Ausgabe>"
		}
		logFn(fmt.Sprintf("❌ Shell konnte nicht geöffnet werden (%s): %v", debugCmd, err))
		logFn(fmt.Sprintf("   Output: %s", msg))
		return
	}
	logFn("🖥️  Shell mit ENV geöffnet")
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

// switchKubeContext changes the active Kubernetes context.
//
// Executes 'kubectl config use-context' to switch the current context.
// This does not require authentication; it only updates the KUBECONFIG.
//
// Returns a LoginResult indicating success or failure.
// logFn receives status messages throughout the process.
//
// Example:
//
//	ctx := KubeContext{Name: "prod-cluster"}
//	result := switchKubeContext(ctx, func(msg string) {
//		fmt.Println(msg)
//	})
func switchKubeContext(ctx KubeContext, logFn func(string)) LoginResult {
	logFn(fmt.Sprintf("🔄 Wechsle zu Kubernetes Context: %s", ctx.Name))
	cmd := exec.Command("kubectl", "config", "use-context", ctx.Name)
	output, err := cmd.CombinedOutput()
	msg := string(output)
	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return LoginResult{false, msg}
	}
	logFn(fmt.Sprintf("✅ Context gewechselt zu: %s", ctx.Name))
	return LoginResult{true, msg}
}

// testKubeConnection verifies connectivity to the active Kubernetes cluster.
//
// Executes 'kubectl cluster-info' and displays the output.
// This helps verify that kubectl can reach the cluster API server.
//
// logFn receives status messages and cluster information.
//
// Example:
//
//	testKubeConnection(func(msg string) {
//		fmt.Println(msg)
//	})
//	// Output:
//	// 🔍 Teste Kubernetes Verbindung...
//	// Kubernetes control plane is running at https://...
//	// CoreDNS is running at https://...
func testKubeConnection(logFn func(string)) {
	logFn("🔍 Teste Kubernetes Verbindung...")
	cmd := exec.Command("kubectl", "cluster-info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logFn(fmt.Sprintf("❌ Verbindung fehlgeschlagen: %s", string(output)))
		return
	}
	lines := strings.Split(string(output), "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			logFn("  " + l)
		}
	}
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

	a := app.NewWithID("de.cloudlogin.tool")
	w := a.NewWindow("☁️  Cloud Login Manager")
	w.Resize(fyne.NewSize(900, 650))

	// Log-Bereich mit RichText für besseres Styling
	logRichText := widget.NewRichTextFromMarkdown("")
	logRichText.Wrapping = fyne.TextWrapWord
	logLines := make([]string, 0, 200)

	// Container mit ScrollContainer für Log
	logScroll := container.NewScroll(logRichText)
	logScroll.SetMinSize(fyne.NewSize(0, 200))

	logFn := func(msg string) {
		fyne.Do(func() {
			// Escape markdown special chars und füge als plain text hinzu
			escapedMsg := strings.ReplaceAll(msg, "*", "\\*")
			escapedMsg = strings.ReplaceAll(escapedMsg, "#", "\\#")
			escapedMsg = strings.ReplaceAll(escapedMsg, "\\", "\\\\")
			escapedMsg += "  \n" // Zeilenumbruch in Markdown
			logLines = append(logLines, escapedMsg)
			logRichText.ParseMarkdown(strings.Join(logLines, "  \n"))
			logScroll.ScrollToBottom()
		})
	}

	clearLog := widget.NewButtonWithIcon("Log leeren", theme.DeleteIcon(), func() {
		fyne.Do(func() {
			logLines = logLines[:0]
			logRichText.ParseMarkdown("")
		})
	})

	// -------- AWS Tab --------
	awsProfiles, err := parseAWSConfig()
	if err != nil {
		awsProfiles = []AWSProfile{}
	}

	var selectedAWSProfile *AWSProfile
	awsInfoLabel := widget.NewLabel("Kein Profil ausgewählt")
	awsInfoLabel.Wrapping = fyne.TextWrapWord

	awsLoginBtn := widget.NewButtonWithIcon("SSO Login", theme.LoginIcon(), func() {
		if selectedAWSProfile == nil {
			dialog.ShowInformation("Hinweis", "Bitte zuerst ein AWS Profil auswählen", w)
			return
		}
		go func() {
			loginAWS(*selectedAWSProfile, logFn)
		}()
	})
	awsLoginBtn.Importance = widget.HighImportance

	awsTestBtn := widget.NewButtonWithIcon("Verbindung testen", theme.ConfirmIcon(), func() {
		if selectedAWSProfile == nil {
			dialog.ShowInformation("Hinweis", "Bitte zuerst ein AWS Profil auswählen", w)
			return
		}
		go testAWSConnection(*selectedAWSProfile, logFn)
	})

	var awsProfileList *widget.List

	awsRefreshBtn := widget.NewButtonWithIcon("Neu laden", theme.ViewRefreshIcon(), func() {
		logFn("🔄 AWS Config wird neu geladen...")
		profiles, err := parseAWSConfig()
		if err != nil {
			logFn(fmt.Sprintf("❌ Fehler: %v", err))
			return
		}
		awsProfiles = profiles
		fyne.Do(func() {
			awsProfileList.Refresh()
		})
		logFn(fmt.Sprintf("✅ %d Profile geladen", len(profiles)))
	})

	awsUpdateBtn := widget.NewButtonWithIcon("AWS Config aktualisieren", theme.ViewRefreshIcon(), func() {
		go func() {
			if err := awsconfig.UpdateFromSSO(logFn); err != nil {
				logFn(fmt.Sprintf("❌ AWS Config Update fehlgeschlagen: %v", err))
				return
			}
			profiles, err := parseAWSConfig()
			if err != nil {
				logFn(fmt.Sprintf("❌ Fehler beim Laden: %v", err))
				return
			}
			awsProfiles = profiles
			fyne.Do(func() {
				awsProfileList.Refresh()
			})
			logFn(fmt.Sprintf("✅ %d Profile geladen", len(profiles)))
		}()
	})

	awsSanitizeBtn := widget.NewButtonWithIcon("AWS Config bereinigen", theme.ContentClearIcon(), func() {
		go func() {
			if err := awsconfig.SanitizeConfigFile(logFn); err != nil {
				logFn(fmt.Sprintf("❌ AWS Config Bereinigung fehlgeschlagen: %v", err))
				return
			}
			profiles, err := parseAWSConfig()
			if err != nil {
				logFn(fmt.Sprintf("❌ Fehler beim Laden: %v", err))
				return
			}
			awsProfiles = profiles
			fyne.Do(func() {
				awsProfileList.Refresh()
			})
			logFn(fmt.Sprintf("✅ %d Profile geladen", len(profiles)))
		}()
	})
	awsProfileList = widget.NewList(
		func() int { return len(awsProfiles) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.AccountIcon()),
				widget.NewLabel("Profil"),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			label := box.Objects[1].(*widget.Label)
			label.SetText(awsProfiles[i].Name)
		},
	)
	awsProfileList.OnSelected = func(i widget.ListItemID) {
		p := awsProfiles[i]
		selectedAWSProfile = &p
		_ = os.Setenv("AWS_PROFILE", p.Name)
		if p.Region != "" {
			_ = os.Setenv("AWS_REGION", p.Region)
		} else {
			_ = os.Unsetenv("AWS_REGION")
		}
		info := fmt.Sprintf("Profil: %s\nRegion: %s", p.Name, p.Region)
		if p.SSOUrl != "" {
			info += fmt.Sprintf("\nSSO URL: %s", p.SSOUrl)
		} else {
			info += "\nTyp: Standard / IAM"
		}
		info += fmt.Sprintf("\nAWS_PROFILE: %s", p.Name)
		if p.Region != "" {
			info += fmt.Sprintf("\nAWS_REGION: %s", p.Region)
		}
		awsInfoLabel.SetText(info)
		logFn(fmt.Sprintf("📌 AWS Profil ausgewählt: %s", p.Name))
	}

	awsListCard := widget.NewCard("AWS Profile", "Aus ~/.aws/config",
		container.NewBorder(nil, nil, nil, nil, awsProfileList))

	awsInfoCard := widget.NewCard("Profil Details", "", awsInfoLabel)

	awsBtnBar := container.NewHBox(awsLoginBtn, awsTestBtn, awsRefreshBtn, awsUpdateBtn, awsSanitizeBtn)

	awsTab := container.NewBorder(
		awsBtnBar,
		nil, nil, nil,
		container.NewHSplit(
			awsListCard,
			awsInfoCard,
		),
	)

	// -------- Kubernetes Tab --------
	kubeContexts, kubeconfigPath, _ := parseKubeContexts()
	currentCtx := getCurrentKubeContext()

	kubePathLabel := widget.NewLabel(fmt.Sprintf("KUBECONFIG: %s", kubeconfigPath))
	kubePathLabel.Wrapping = fyne.TextWrapWord

	var selectedKubeCtx *KubeContext
	kubeInfoLabel := widget.NewLabel("Kein Context ausgewählt")
	kubeCurrentLabel := widget.NewLabelWithStyle(
		fmt.Sprintf("Aktueller Context: %s", currentCtx),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	kubeSwitchBtn := widget.NewButtonWithIcon("Context wechseln", theme.NavigateNextIcon(), func() {
		if selectedKubeCtx == nil {
			dialog.ShowInformation("Hinweis", "Bitte zuerst einen Context auswählen", w)
			return
		}
		go func() {
			result := switchKubeContext(*selectedKubeCtx, logFn)
			if result.Success {
				fyne.Do(func() {
					kubeCurrentLabel.SetText(fmt.Sprintf("Aktueller Context: %s", selectedKubeCtx.Name))
				})
			}
		}()
	})
	kubeSwitchBtn.Importance = widget.HighImportance

	kubeTestBtn := widget.NewButtonWithIcon("Verbindung testen", theme.ConfirmIcon(), func() {
		go testKubeConnection(logFn)
	})

	kubeRefreshBtn := widget.NewButtonWithIcon("Neu laden", theme.ViewRefreshIcon(), func() {
		logFn("🔄 Kubernetes Contexts werden neu geladen...")
		ctxs, path, err := parseKubeContexts()
		if err != nil {
			logFn(fmt.Sprintf("❌ Fehler: %v", err))
			return
		}
		kubeContexts = ctxs
		fyne.Do(func() {
			kubePathLabel.SetText(fmt.Sprintf("KUBECONFIG: %s", path))
		})
		logFn(fmt.Sprintf("✅ %d Contexts geladen", len(ctxs)))
	})

	// Namespace anzeigen
	kubeNsBtn := widget.NewButtonWithIcon("Namespaces", theme.ListIcon(), func() {
		go func() {
			logFn("📋 Namespaces:")
			out, err := exec.Command("kubectl", "get", "namespaces",
				"--no-headers", "-o", "custom-columns=NAME:.metadata.name").Output()
			if err != nil {
				logFn(fmt.Sprintf("❌ Fehler: %v", err))
				return
			}
			for _, ns := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				logFn("  • " + ns)
			}
		}()
	})

	var kubeCtxList *widget.List
	kubeCtxList = widget.NewList(
		func() int { return len(kubeContexts) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.ComputerIcon()),
				widget.NewLabel("Context"),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			label := box.Objects[1].(*widget.Label)
			name := kubeContexts[i].Name
			if name == currentCtx {
				label.SetText("✅ " + name)
			} else {
				label.SetText(name)
			}
		},
	)
	kubeCtxList.OnSelected = func(i widget.ListItemID) {
		ctx := kubeContexts[i]
		selectedKubeCtx = &ctx
		kubeInfoLabel.SetText(fmt.Sprintf("Context: %s", ctx.Name))
		logFn(fmt.Sprintf("📌 Kubernetes Context ausgewählt: %s", ctx.Name))
	}

	kubeListCard := widget.NewCard("Kubernetes Contexts", "Aus KUBECONFIG",
		container.NewBorder(nil, nil, nil, nil, kubeCtxList))

	kubeDetailCard := widget.NewCard("Context Details", "",
		container.NewVBox(kubeCurrentLabel, kubeInfoLabel, kubePathLabel))

	kubeBtnBar := container.NewHBox(
		kubeSwitchBtn, kubeTestBtn, kubeNsBtn, kubeRefreshBtn)

	kubeTab := container.NewBorder(
		kubeBtnBar,
		nil, nil, nil,
		container.NewHSplit(
			kubeListCard,
			kubeDetailCard,
		),
	)

	// -------- Quick Actions Tab --------
	quickTitle := widget.NewLabelWithStyle(
		"Schnell-Aktionen", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	openAWSConsole := widget.NewButtonWithIcon(
		"AWS Console im Browser öffnen", theme.ComputerIcon(), func() {
			url := "https://console.aws.amazon.com"
			var cmd *exec.Cmd
			if runtime.GOOS == "darwin" {
				cmd = exec.Command("open", url)
			} else {
				cmd = exec.Command("xdg-open", url)
			}
			cmd.Start()
			logFn("🌐 AWS Console wird geöffnet...")
		})

	openK9s := widget.NewButtonWithIcon("k9s starten (Terminal)", theme.ComputerIcon(), func() {
		logFn("🚀 Starte k9s...")
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("osascript", "-e",
				`tell application "Terminal" to do script "k9s"`)
		} else {
			// Linux - versuche verschiedene Terminals
			for _, term := range []string{"gnome-terminal", "xterm", "konsole"} {
				if _, err := exec.LookPath(term); err == nil {
					cmd = exec.Command(term, "-e", "k9s")
					break
				}
			}
		}
		if cmd != nil {
			if err := cmd.Start(); err != nil {
				logFn(fmt.Sprintf("❌ Fehler: %v", err))
			}
		} else {
			logFn("❌ Kein Terminal gefunden")
		}
	})

	openEnvShell := widget.NewButtonWithIcon(
		"Shell mit Cloud-ENV öffnen", theme.ComputerIcon(), func() {
			var envLines []string
			if selectedAWSProfile != nil {
				envLines = append(envLines, fmt.Sprintf("export AWS_PROFILE=%s", selectedAWSProfile.Name))
				if selectedAWSProfile.Region != "" {
					envLines = append(envLines, fmt.Sprintf("export AWS_REGION=%s", selectedAWSProfile.Region))
				}
			}
			if v := os.Getenv("AZURE_SUBSCRIPTION_ID"); v != "" {
				envLines = append(envLines, fmt.Sprintf("export AZURE_SUBSCRIPTION_ID=%s", v))
			}
			if v := os.Getenv("AZURE_TENANT_ID"); v != "" {
				envLines = append(envLines, fmt.Sprintf("export AZURE_TENANT_ID=%s", v))
			}
			openShellWithEnv(envLines, logFn)
		})

	exportAWSEnv := widget.NewButtonWithIcon(
		"AWS Env vars exportieren", theme.DocumentIcon(), func() {
			if selectedAWSProfile == nil {
				dialog.ShowInformation("Hinweis", "Bitte zuerst ein AWS Profil auswählen", w)
				return
			}
			cmd := exec.Command("aws", "configure", "export-credentials",
				"--format", "env")
			cmd.Env = awsEnv(*selectedAWSProfile)
			out, err := cmd.Output()
			if err != nil {
				logFn(fmt.Sprintf("❌ Fehler: %v", err))
				return
			}
			// In Zwischenablage kopieren
			w.Clipboard().SetContent(string(out))
			logFn("📋 AWS Credentials in Zwischenablage kopiert!")
			logFn(string(out))
		})

	checkTools := widget.NewButtonWithIcon(
		"Tools prüfen (aws, kubectl, k9s)", theme.ConfirmIcon(), func() {
			for _, tool := range []string{"aws", "kubectl", "k9s", "helm"} {
				path, err := exec.LookPath(tool)
				if err != nil {
					logFn(fmt.Sprintf("❌ %s: nicht gefunden", tool))
				} else {
					logFn(fmt.Sprintf("✅ %s: %s", tool, path))
				}
			}
		})

	quickTab := container.NewVBox(
		quickTitle,
		widget.NewSeparator(),
		openAWSConsole,
		openK9s,
		openEnvShell,
		exportAWSEnv,
		checkTools,
	)

	// -------- Tabs zusammenbauen --------
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("AWS", theme.StorageIcon(), awsTab),
		container.NewTabItemWithIcon("Kubernetes", theme.ComputerIcon(), kubeTab),
		container.NewTabItemWithIcon("Quick Actions", theme.MediaFastForwardIcon(), quickTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	logCard := widget.NewCard("", "",
		container.NewBorder(nil, clearLog, nil, nil, logScroll))

	// Haupt-Layout: Tabs oben, Log unten
	split := container.NewVSplit(tabs, logCard)
	split.Offset = 0.65

	w.SetContent(split)

	// Startup Info
	logFn(fmt.Sprintf("🚀 Cloud Login Manager v%s (Commit: %s, Built: %s)", version, commit, date))
	logFn(fmt.Sprintf("📁 AWS Profile gefunden: %d", len(awsProfiles)))
	logFn(fmt.Sprintf("☸️  Kubernetes Contexts gefunden: %d", len(kubeContexts)))
	if currentCtx != "" {
		logFn(fmt.Sprintf("📌 Aktueller K8s Context: %s", currentCtx))
	}

	w.ShowAndRun()
}
