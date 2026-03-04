package azure

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/afeldman/cloudlogin/pkg/provider"
)

// AzureProvider implementiert das CloudProvider Interface für Azure
type AzureProvider struct {
}

// azureSubscription repräsentiert eine Azure Subscription
type azureSubscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
	State     string `json:"state"`
	TenantID  string `json:"tenantId"`
}

type azureAccount struct {
	TenantID string `json:"tenantId"`
}

// NewAzureProvider erstellt eine neue Azure Provider Instanz
func NewAzureProvider() *AzureProvider {
	return &AzureProvider{}
}

// Name gibt den Namen des Providers zurück
func (az *AzureProvider) Name() string {
	return "Azure"
}

// GetCredentials liest Azure Subscriptions aus az cli
func (az *AzureProvider) GetCredentials() ([]provider.Credential, error) {
	cmd := exec.Command("az", "account", "list", "--output", "json")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("Azure CLI nicht verfügbar oder nicht eingeloggt: %w", err)
	}

	var subscriptions []azureSubscription
	if err := json.Unmarshal(output, &subscriptions); err != nil {
		return nil, fmt.Errorf("Fehler beim Parsen von Azure Subscriptions: %w", err)
	}

	var credentials []provider.Credential
	for _, sub := range subscriptions {
		details := make(map[string]string)
		details["state"] = sub.State
		details["tenant_id"] = sub.TenantID
		if sub.IsDefault {
			details["is_default"] = "true"
		}

		credentials = append(credentials, provider.Credential{
			ID:          sub.ID,
			DisplayName: sub.Name,
			Details:     details,
		})
	}

	return credentials, nil
}

// Login führt Azure Login durch
func (az *AzureProvider) Login(subscriptionID string, logFn func(string)) provider.LoginResult {
	logFn(fmt.Sprintf("🔐 Azure Login für Subscription: %s", subscriptionID))

	cmd := exec.Command("az", "login")
	cmd.Env = append(os.Environ(), fmt.Sprintf("AZURE_SUBSCRIPTION_ID=%s", subscriptionID))
	output, err := cmd.CombinedOutput()
	msg := string(output)

	if err != nil {
		logFn(fmt.Sprintf("❌ Fehler: %s", msg))
		return provider.LoginResult{Success: false, Message: msg}
	}

	// Wechsle zur Subscription
	cmdSub := exec.Command("az", "account", "set", "--subscription", subscriptionID)
	cmdSub.Env = append(os.Environ(), fmt.Sprintf("AZURE_SUBSCRIPTION_ID=%s", subscriptionID))
	if err := cmdSub.Run(); err != nil {
		logFn(fmt.Sprintf("❌ Subscription Fehler: %v", err))
		return provider.LoginResult{Success: false, Message: fmt.Sprintf("Subscription Fehler: %v", err)}
	}

	// Setze Env-Variablen für die laufende App
	_ = os.Setenv("AZURE_SUBSCRIPTION_ID", subscriptionID)
	accountCmd := exec.Command("az", "account", "show", "--subscription", subscriptionID, "--output", "json")
	if accountOut, accErr := accountCmd.Output(); accErr == nil {
		var account azureAccount
		if jsonErr := json.Unmarshal(accountOut, &account); jsonErr == nil && account.TenantID != "" {
			_ = os.Setenv("AZURE_TENANT_ID", account.TenantID)
		}
	}

	logFn(fmt.Sprintf("✅ Erfolgreich zu Subscription gewechselt: %s", subscriptionID))
	return provider.LoginResult{Success: true, Message: msg}
}

// TestConnection testet die Azure Verbindung
func (az *AzureProvider) TestConnection(logFn func(string)) {
	logFn("🔍 Teste Azure Verbindung...")

	cmd := exec.Command("az", "account", "show", "--output", "json")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	if err != nil {
		logFn(fmt.Sprintf("❌ Nicht authentifiziert: %s", string(output)))
		return
	}

	var account map[string]interface{}
	if err := json.Unmarshal(output, &account); err == nil {
		logFn(fmt.Sprintf("✅ Azure Authentifizierung gültig:"))
		if name, ok := account["name"]; ok {
			logFn(fmt.Sprintf("  Subscription: %v", name))
		}
		if id, ok := account["id"]; ok {
			logFn(fmt.Sprintf("  ID: %v", id))
		}
	} else {
		logFn(fmt.Sprintf("✅ Azure Authentifizierung gültig:"))
		logFn("  " + strings.TrimSpace(string(output)))
	}
}
