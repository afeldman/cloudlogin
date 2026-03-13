package kube

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/afeldman/cloudlogin/internal/shell"
)

// KubeContext represents a Kubernetes context from the KUBECONFIG.
type KubeContext struct {
	Name    string
	Cluster string
	User    string
}

// ParseKubeContexts retrieves all available Kubernetes contexts from KUBECONFIG.
// Returns contexts, the kubeconfig path used, and any error.
func ParseKubeContexts() ([]KubeContext, string, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	out, err := exec.Command("kubectl", "config", "get-contexts",
		"-o", "name", "--kubeconfig", kubeconfig).Output()
	if err != nil {
		return nil, kubeconfig, fmt.Errorf("kubectl nicht gefunden oder Fehler: %w", err)
	}

	var contexts []KubeContext
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			contexts = append(contexts, KubeContext{Name: line})
		}
	}
	return contexts, kubeconfig, nil
}

// GetCurrentKubeContext returns the currently active Kubernetes context name.
func GetCurrentKubeContext() string {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SwitchKubeContext changes the active Kubernetes context.
func SwitchKubeContext(ctx KubeContext, logFn func(string)) shell.LoginResult {
	logFn(fmt.Sprintf("🔄 Wechsle zu Kubernetes Context: %s", ctx.Name))
	cmd := exec.Command("kubectl", "config", "use-context", ctx.Name)
	output, err := cmd.CombinedOutput()
	msg := string(output)
	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return shell.LoginResult{Success: false, Message: msg}
	}
	logFn(fmt.Sprintf("✅ Context gewechselt zu: %s", ctx.Name))
	return shell.LoginResult{Success: true, Message: msg}
}

// TestKubeConnection verifies connectivity to the active Kubernetes cluster.
func TestKubeConnection(logFn func(string)) {
	logFn("🔍 Teste Kubernetes Verbindung...")
	cmd := exec.Command("kubectl", "cluster-info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logFn(fmt.Sprintf("❌ Verbindung fehlgeschlagen: %s", string(output)))
		return
	}
	for _, l := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(l) != "" {
			logFn("  " + l)
		}
	}
}
