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
)

// Build information (set by ldflags during build)
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// ---------- Config Parser ----------

type AWSProfile struct {
	Name   string
	Region string
	SSOUrl string
}

type KubeContext struct {
	Name    string
	Cluster string
	User    string
}

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

func getCurrentKubeContext() string {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ---------- Login Actions ----------

type LoginResult struct {
	Success bool
	Message string
}

func awsEnv(profile AWSProfile) []string {
	env := os.Environ()
	env = append(env, fmt.Sprintf("AWS_PROFILE=%s", profile.Name))
	if profile.Region != "" {
		env = append(env, fmt.Sprintf("AWS_REGION=%s", profile.Region))
	}
	return env
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

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

// ---------- GUI ----------

func main() {
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

	awsRefreshBtn := widget.NewButtonWithIcon("Neu laden", theme.ViewRefreshIcon(), func() {
		logFn("🔄 AWS Config wird neu geladen...")
		profiles, err := parseAWSConfig()
		if err != nil {
			logFn(fmt.Sprintf("❌ Fehler: %v", err))
			return
		}
		logFn(fmt.Sprintf("✅ %d Profile geladen", len(profiles)))
	})

	var awsProfileList *widget.List
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

	awsBtnBar := container.NewHBox(awsLoginBtn, awsTestBtn, awsRefreshBtn)

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
