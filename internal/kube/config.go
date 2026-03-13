package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type kubeConfigFile struct {
	APIVersion     string             `yaml:"apiVersion"`
	Kind           string             `yaml:"kind"`
	Preferences    map[string]any     `yaml:"preferences"`
	CurrentContext string             `yaml:"current-context,omitempty"`
	Clusters       []kubeClusterEntry `yaml:"clusters"`
	Contexts       []kubeContextEntry `yaml:"contexts"`
	Users          []kubeUserEntry    `yaml:"users"`
}

type kubeClusterEntry struct {
	Name    string `yaml:"name"`
	Cluster struct {
		Server                   string `yaml:"server"`
		CertificateAuthorityData string `yaml:"certificate-authority-data"`
	} `yaml:"cluster"`
}

type kubeContextEntry struct {
	Name    string `yaml:"name"`
	Context struct {
		Cluster string `yaml:"cluster"`
		User    string `yaml:"user"`
	} `yaml:"context"`
}

type kubeUserEntry struct {
	Name string      `yaml:"name"`
	User kubeExecCfg `yaml:"user"`
}

type kubeExecCfg struct {
	Exec *execConfig `yaml:"exec,omitempty"`
}

type execConfig struct {
	APIVersion         string   `yaml:"apiVersion"`
	Command            string   `yaml:"command"`
	Args               []string `yaml:"args"`
	Env                any      `yaml:"env"`
	InteractiveMode    string   `yaml:"interactiveMode"`
	ProvideClusterInfo bool     `yaml:"provideClusterInfo"`
}

func kubeConfigPath() string {
	if p := os.Getenv("KUBECONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
}

func readKubeConfigFile() (*kubeConfigFile, string, error) {
	path := kubeConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &kubeConfigFile{
			APIVersion:  "v1",
			Kind:        "Config",
			Preferences: map[string]any{},
		}, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	var cfg kubeConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, path, fmt.Errorf("KUBECONFIG parse-Fehler: %w", err)
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = "v1"
	}
	if cfg.Kind == "" {
		cfg.Kind = "Config"
	}
	return &cfg, path, nil
}

func writeKubeConfigFile(cfg *kubeConfigFile, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("KUBECONFIG marshal-Fehler: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// upsertEntry adds or replaces cluster/context/user entries.
// alias is used as the context name; clusterName is the actual EKS name for token auth.
func upsertEntry(cfg *kubeConfigFile, alias, clusterName, endpoint, caData, awsProfile, region string) {
	cluster := kubeClusterEntry{Name: alias}
	cluster.Cluster.Server = endpoint
	cluster.Cluster.CertificateAuthorityData = caData
	replaceCluster(cfg, alias, cluster)

	ctx := kubeContextEntry{Name: alias}
	ctx.Context.Cluster = alias
	ctx.Context.User = alias
	replaceContext(cfg, alias, ctx)

	user := kubeUserEntry{
		Name: alias,
		User: kubeExecCfg{Exec: &execConfig{
			APIVersion:         "client.authentication.k8s.io/v1beta1",
			Command:            "aws",
			Args:               []string{"--region", region, "eks", "get-token", "--cluster-id", clusterName, "--profile", awsProfile},
			Env:                nil,
			InteractiveMode:    "IfAvailable",
			ProvideClusterInfo: false,
		}},
	}
	replaceUser(cfg, alias, user)
}

func replaceCluster(cfg *kubeConfigFile, name string, entry kubeClusterEntry) {
	for i, c := range cfg.Clusters {
		if c.Name == name {
			cfg.Clusters[i] = entry
			return
		}
	}
	cfg.Clusters = append(cfg.Clusters, entry)
}

func replaceContext(cfg *kubeConfigFile, name string, entry kubeContextEntry) {
	for i, c := range cfg.Contexts {
		if c.Name == name {
			cfg.Contexts[i] = entry
			return
		}
	}
	cfg.Contexts = append(cfg.Contexts, entry)
}

func replaceUser(cfg *kubeConfigFile, name string, entry kubeUserEntry) {
	for i, u := range cfg.Users {
		if u.Name == name {
			cfg.Users[i] = entry
			return
		}
	}
	cfg.Users = append(cfg.Users, entry)
}

// AddOrUpdateCluster writes an EKS cluster entry into the KUBECONFIG file.
// alias is the context/user name; clusterName is the actual EKS cluster name
// used for token retrieval (--cluster-id). Creates the file if needed.
func AddOrUpdateCluster(alias, clusterName, endpoint, caData, awsProfile, region string) (string, error) {
	cfg, path, err := readKubeConfigFile()
	if err != nil {
		return "", fmt.Errorf("KUBECONFIG lesen: %w", err)
	}
	upsertEntry(cfg, alias, clusterName, endpoint, caData, awsProfile, region)
	if err := writeKubeConfigFile(cfg, path); err != nil {
		return "", fmt.Errorf("KUBECONFIG schreiben: %w", err)
	}
	return path, nil
}

// BackupKubeConfig copies the current KUBECONFIG to <path>.bak.
// Returns the backup path. No-op (empty path, nil error) if file doesn't exist yet.
func BackupKubeConfig() (string, error) {
	path := kubeConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	bak := path + ".bak"
	return bak, os.WriteFile(bak, data, 0o600)
}
