package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/afeldman/cloudlogin/internal/kube"
)

type eksClusterList struct {
	Clusters []string `json:"clusters"`
}

type eksClusterDetail struct {
	Cluster struct {
		Endpoint             string `json:"endpoint"`
		CertificateAuthority struct {
			Data string `json:"data"`
		} `json:"certificateAuthority"`
	} `json:"cluster"`
}

func listEKSClusters(profile AWSProfile) ([]string, error) {
	args := []string{"eks", "list-clusters", "--output", "json"}
	if profile.Region != "" {
		args = append(args, "--region", profile.Region)
	}
	cmd := exec.Command("aws", args...)
	cmd.Env = AWSEnv(profile)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("aws eks list-clusters: %w", err)
	}
	var result eksClusterList
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("JSON Parse-Fehler: %w", err)
	}
	return result.Clusters, nil
}

func describeEKSCluster(profile AWSProfile, clusterName string) (*eksClusterDetail, error) {
	args := []string{"eks", "describe-cluster", "--name", clusterName, "--output", "json"}
	if profile.Region != "" {
		args = append(args, "--region", profile.Region)
	}
	cmd := exec.Command("aws", args...)
	cmd.Env = AWSEnv(profile)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("aws eks describe-cluster %s: %w", clusterName, err)
	}
	var detail eksClusterDetail
	if err := json.Unmarshal(out, &detail); err != nil {
		return nil, fmt.Errorf("JSON Parse-Fehler: %w", err)
	}
	return &detail, nil
}

// syncClusterToKubeconfig fetches EKS cluster details and writes them into KUBECONFIG.
// alias is the kube context name; clusterName is the actual EKS cluster name.
func syncClusterToKubeconfig(profile AWSProfile, clusterName, alias string, logFn func(string)) error {
	detail, err := describeEKSCluster(profile, clusterName)
	if err != nil {
		return err
	}

	endpoint := detail.Cluster.Endpoint
	caData := detail.Cluster.CertificateAuthority.Data
	if endpoint == "" || caData == "" {
		return fmt.Errorf("unvollständige Cluster-Daten für %s (endpoint=%q)", clusterName, endpoint)
	}

	region := profile.Region
	if region == "" {
		region = "eu-central-1"
	}

	path, err := kube.AddOrUpdateCluster(alias, clusterName, endpoint, caData, profile.Name, region)
	if err != nil {
		return err
	}
	logFn(fmt.Sprintf("  ✅ %s (Cluster: %s) → %s", alias, clusterName, path))
	return nil
}
