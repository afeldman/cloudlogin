package aws

import (
	"fmt"

	"github.com/afeldman/cloudlogin/internal/kube"
)

// SyncResult is the outcome of a synchronization run.
type SyncResult struct {
	Success     bool
	Message     string
	MissingKube []string
	MissingAWS  []string
	Synced      []string
}

// contextAlias returns the kube context name for a profile+cluster combination.
// Uses profile.Name directly when the account has exactly one cluster,
// otherwise appends the cluster name to disambiguate.
func contextAlias(profileName, clusterName string, totalClusters int) string {
	if totalClusters == 1 {
		return profileName
	}
	return profileName + "-" + clusterName
}

// clustersByAccount fetches and caches EKS cluster lists keyed by accountID+region.
// Uses the first available profile per account for the API call.
func clustersByAccount(profiles []AWSProfile, logFn func(string)) map[string][]string {
	cache := make(map[string][]string)
	for _, p := range profiles {
		if p.AccountID == "" || p.SSOUrl == "" {
			continue
		}
		key := p.AccountID + "/" + p.Region
		if _, ok := cache[key]; ok {
			continue
		}
		clusters, err := listEKSClusters(p)
		if err != nil {
			logFn(fmt.Sprintf("  ⚠️  Account %s nicht erreichbar (nicht eingeloggt?): %v", p.AccountID, err))
			cache[key] = nil
			continue
		}
		cache[key] = clusters
	}
	return cache
}

// SyncAWSKube discovers EKS clusters for each SSO account and writes one kube
// context per AWS profile. Context names match the AWS profile name exactly.
// If an account has multiple clusters, the cluster name is appended as suffix.
func SyncAWSKube(logFn func(string)) SyncResult {
	logFn("🔄 Synchronisiere AWS Profile → KUBECONFIG Contexts...")

	profiles, err := ParseAWSConfig()
	if err != nil {
		return SyncResult{Success: false, Message: fmt.Sprintf("AWS Config Fehler: %v", err)}
	}

	if bak, err := kube.BackupKubeConfig(); err != nil {
		logFn(fmt.Sprintf("⚠️  KUBECONFIG Backup fehlgeschlagen: %v", err))
	} else if bak != "" {
		logFn(fmt.Sprintf("💾 KUBECONFIG Backup: %s", bak))
	}

	clusterCache := clustersByAccount(profiles, logFn)

	var synced, failed []string
	for _, profile := range profiles {
		if profile.AccountID == "" || profile.SSOUrl == "" {
			continue
		}
		key := profile.AccountID + "/" + profile.Region
		clusters := clusterCache[key]
		if len(clusters) == 0 {
			continue
		}

		for _, cluster := range clusters {
			alias := contextAlias(profile.Name, cluster, len(clusters))
			logFn(fmt.Sprintf("  📦 %s → Context %q", cluster, alias))
			if err := syncClusterToKubeconfig(profile, cluster, alias, logFn); err != nil {
				failed = append(failed, alias)
				logFn(fmt.Sprintf("  ❌ %s: %v", alias, err))
			} else {
				synced = append(synced, alias)
			}
		}
	}

	msg := fmt.Sprintf("Fertig: %d Contexts aktualisiert", len(synced))
	if len(failed) > 0 {
		msg += fmt.Sprintf(", %d fehlgeschlagen: %v", len(failed), failed)
	}
	return SyncResult{Success: len(failed) == 0, Message: msg, Synced: synced}
}

// CheckSyncStatus checks whether a kube context exists for each SSO profile.
func CheckSyncStatus(logFn func(string)) (bool, string, []string, []string) {
	profiles, err := ParseAWSConfig()
	if err != nil {
		return false, fmt.Sprintf("Fehler: %v", err), nil, nil
	}

	kubeContexts, _, err := kube.ParseKubeContexts()
	if err != nil {
		return false, fmt.Sprintf("Fehler: %v", err), nil, nil
	}

	kubeNames := make(map[string]bool)
	for _, ctx := range kubeContexts {
		kubeNames[ctx.Name] = true
	}

	clusterCache := clustersByAccount(profiles, logFn)

	var missingKube []string
	for _, profile := range profiles {
		if profile.AccountID == "" || profile.SSOUrl == "" {
			continue
		}
		key := profile.AccountID + "/" + profile.Region
		clusters := clusterCache[key]
		for _, cluster := range clusters {
			alias := contextAlias(profile.Name, cluster, len(clusters))
			if !kubeNames[alias] {
				missingKube = append(missingKube, alias)
			}
		}
	}

	if len(missingKube) == 0 {
		return true, "✅ Alle AWS Profile haben einen passenden Kube Context", nil, nil
	}
	msg := fmt.Sprintf("⚠️  %d Contexts fehlen: %v", len(missingKube), missingKube)
	return false, msg, missingKube, nil
}
