package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	cloudaws "github.com/afeldman/cloudlogin/internal/aws"
	"github.com/afeldman/cloudlogin/internal/kube"
	"github.com/afeldman/cloudlogin/internal/shell"
	"github.com/afeldman/cloudlogin/pkg/awsconfig"
)

func runGUI() {
	icon := fyne.NewStaticResource("logo.png", appIcon)

	a := app.NewWithID("de.cloudlogin.tool")
	a.SetIcon(icon)
	w := a.NewWindow("☁️  Cloud Login Manager")
	w.SetIcon(icon)
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
	awsProfiles, err := cloudaws.ParseAWSConfig()
	if err != nil {
		awsProfiles = []cloudaws.AWSProfile{}
	}

	var selectedAWSProfile *cloudaws.AWSProfile
	awsInfoLabel := widget.NewLabel("Kein Profil ausgewählt")
	awsInfoLabel.Wrapping = fyne.TextWrapWord

	awsLoginBtn := widget.NewButtonWithIcon("SSO Login", theme.LoginIcon(), func() {
		if selectedAWSProfile == nil {
			dialog.ShowInformation("Hinweis", "Bitte zuerst ein AWS Profil auswählen", w)
			return
		}
		go func() {
			cloudaws.LoginAWS(*selectedAWSProfile, logFn)
		}()
	})
	awsLoginBtn.Importance = widget.HighImportance

	awsTestBtn := widget.NewButtonWithIcon("Verbindung testen", theme.ConfirmIcon(), func() {
		if selectedAWSProfile == nil {
			dialog.ShowInformation("Hinweis", "Bitte zuerst ein AWS Profil auswählen", w)
			return
		}
		go cloudaws.TestAWSConnection(*selectedAWSProfile, logFn)
	})

	var awsProfileList *widget.List

	awsRefreshBtn := widget.NewButtonWithIcon("Neu laden", theme.ViewRefreshIcon(), func() {
		logFn("🔄 AWS Config wird neu geladen...")
		profiles, err := cloudaws.ParseAWSConfig()
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
			profiles, err := cloudaws.ParseAWSConfig()
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
			profiles, err := cloudaws.ParseAWSConfig()
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

	// Vorab deklariert, damit die Sync-Button-Closures darauf zugreifen können
	// (werden unten beim Kubernetes-Tab initialisiert)
	var (
		kubeContexts  []kube.KubeContext
		kubePathLabel *widget.Label
		kubeCtxList   *widget.List
	)

	// AWS/Kube Sync Buttons
	awsSyncCheckBtn := widget.NewButtonWithIcon("Sync Status prüfen", theme.InfoIcon(), func() {
		go func() {
			synced, message, missingKube, missingAWS := cloudaws.CheckSyncStatus(logFn)
			if synced {
				dialog.ShowInformation("Sync Status", message, w)
			} else {
				// Show detailed dialog with sync options
				showSyncDialog(w, message, missingKube, missingAWS, logFn, kubeCtxList)
			}
		}()
	})

	awsSyncBtn := widget.NewButtonWithIcon("AWS/Kube synchronisieren", theme.ViewRefreshIcon(), func() {
		go func() {
			result := cloudaws.SyncAWSKube(logFn)
			if result.Success {
				dialog.ShowInformation("Synchronisierung", result.Message, w)
				// Refresh Kubernetes contexts after sync
				ctxs, path, err := kube.ParseKubeContexts()
				if err == nil {
					kubeContexts = ctxs
					fyne.Do(func() {
						kubePathLabel.SetText(fmt.Sprintf("KUBECONFIG: %s", path))
						kubeCtxList.Refresh()
					})
				}
			} else {
				dialog.ShowError(fmt.Errorf("Synchronisierung fehlgeschlagen: %s", result.Message), w)
			}
		}()
	})

	awsBtnBar := container.NewHBox(awsLoginBtn, awsTestBtn, awsRefreshBtn, awsUpdateBtn, awsSanitizeBtn, awsSyncCheckBtn, awsSyncBtn)

	awsTab := container.NewBorder(
		awsBtnBar,
		nil, nil, nil,
		container.NewHSplit(
			awsListCard,
			awsInfoCard,
		),
	)

	// -------- Kubernetes Tab --------
	var kubeconfigPath string
	kubeContexts, kubeconfigPath, _ = kube.ParseKubeContexts()
	currentCtx := kube.GetCurrentKubeContext()

	kubePathLabel = widget.NewLabel(fmt.Sprintf("KUBECONFIG: %s", kubeconfigPath))
	kubePathLabel.Wrapping = fyne.TextWrapWord

	var selectedKubeCtx *kube.KubeContext
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
			result := kube.SwitchKubeContext(*selectedKubeCtx, logFn)
			if result.Success {
				fyne.Do(func() {
					kubeCurrentLabel.SetText(fmt.Sprintf("Aktueller Context: %s", selectedKubeCtx.Name))
				})
			}
		}()
	})
	kubeSwitchBtn.Importance = widget.HighImportance

	kubeTestBtn := widget.NewButtonWithIcon("Verbindung testen", theme.ConfirmIcon(), func() {
		go kube.TestKubeConnection(logFn)
	})

	kubeRefreshBtn := widget.NewButtonWithIcon("Neu laden", theme.ViewRefreshIcon(), func() {
		logFn("🔄 Kubernetes Contexts werden neu geladen...")
		ctxs, path, err := kube.ParseKubeContexts()
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
			shell.OpenShellWithEnv(envLines, logFn)
		})

	exportAWSEnv := widget.NewButtonWithIcon(
		"AWS Env vars exportieren", theme.DocumentIcon(), func() {
			if selectedAWSProfile == nil {
				dialog.ShowInformation("Hinweis", "Bitte zuerst ein AWS Profil auswählen", w)
				return
			}
			cmd := exec.Command("aws", "configure", "export-credentials",
				"--format", "env")
			cmd.Env = cloudaws.AWSEnv(*selectedAWSProfile)
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

// showSyncDialog shows a detailed dialog with sync information and options
func showSyncDialog(w fyne.Window, message string, missingKube, missingAWS []string, logFn func(string), kubeList *widget.List) {
	content := widget.NewLabel(message)
	content.Wrapping = fyne.TextWrapWord

	syncBtn := widget.NewButton("Jetzt synchronisieren", func() {
		go func() {
			result := cloudaws.SyncAWSKube(logFn)
			fyne.Do(func() {
				if kubeList != nil {
					kubeList.Refresh()
				}
				if result.Success {
					dialog.ShowInformation("Synchronisierung", result.Message, w)
				} else {
					dialog.ShowError(fmt.Errorf("Synchronisierung fehlgeschlagen: %s", result.Message), w)
				}
			})
		}()
	})
	syncBtn.Importance = widget.HighImportance
	
	details := widget.NewLabel("")
	if len(missingKube) > 0 {
		details.SetText(fmt.Sprintf("Fehlende Kube Contexts:\n• %s\n\nKube Contexts ohne AWS Profile:\n• %s",
			strings.Join(missingKube, "\n• "),
			strings.Join(missingAWS, "\n• ")))
	}
	details.Wrapping = fyne.TextWrapWord
	
	dialog.ShowCustom("Sync Status - Nicht synchronisiert", "Schließen",
		container.NewVBox(
			content,
			widget.NewSeparator(),
			details,
			widget.NewSeparator(),
			syncBtn,
		), w)
}
