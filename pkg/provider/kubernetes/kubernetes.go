package kubernetes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/afeldman/cloudlogin/pkg/provider"
)

// KubernetesHandler implementiert das ContextHandler Interface für Kubernetes
type KubernetesHandler struct {
	kubeconfigPath string
}

// NewKubernetesHandler erstellt einen neuen Kubernetes Handler
func NewKubernetesHandler() *KubernetesHandler {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	return &KubernetesHandler{
		kubeconfigPath: kubeconfig,
	}
}

// Name gibt den Namen des Handlers zurück
func (k *KubernetesHandler) Name() string {
	return "Kubernetes"
}

// GetCurrentContext gibt den aktuellen Kontext zurück
func (k *KubernetesHandler) GetCurrentContext() string {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetContexts liest alle Kubernetes Contexts
func (k *KubernetesHandler) GetContexts() ([]provider.Context, error) {
	out, err := exec.Command("kubectl", "config", "get-contexts", "-o", "name", "--kubeconfig", k.kubeconfigPath).Output()
	if err != nil {
		return nil, fmt.Errorf("kubectl nicht gefunden oder Fehler: %w", err)
	}

	var contexts []provider.Context
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			contexts = append(contexts, provider.Context{
				Name:    line,
				Details: make(map[string]string),
			})
		}
	}
	return contexts, nil
}

// SwitchContext wechselt zum angegebenen Kontext
func (k *KubernetesHandler) SwitchContext(contextName string, logFn func(string)) provider.LoginResult {
	logFn(fmt.Sprintf("🔄 Wechsle zu Kubernetes Context: %s", contextName))

	cmd := exec.Command("kubectl", "config", "use-context", contextName)
	output, err := cmd.CombinedOutput()
	msg := string(output)

	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return provider.LoginResult{Success: false, Message: msg}
	}

	logFn(fmt.Sprintf("✅ Context gewechselt zu: %s", contextName))
	return provider.LoginResult{Success: true, Message: msg}
}

// TestConnection testet die Kubernetes Verbindung
func (k *KubernetesHandler) TestConnection(logFn func(string)) {
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

// GetKubeconfigPath gibt den Pfad zur KUBECONFIG Datei zurück
func (k *KubernetesHandler) GetKubeconfigPath() string {
	return k.kubeconfigPath
}
