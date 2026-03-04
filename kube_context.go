package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
