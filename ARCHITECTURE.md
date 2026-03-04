# Cloud Login Manager - Architektur

## Überblick

Das Cloud Login Manager ist now mit einer erweiterbaren Plugin-Architektur aufgebaut, die es einfach macht, neue Cloud-Provider (AWS, Azure, GCP, etc.) hinzuzufügen.

## Struktur

```
pkg/provider/
├── provider.go           # Interface Definitionen
├── aws/
│   └── aws.go           # AWS Provider Implementierung
├── azure/
│   └── azure.go         # Azure Provider Implementierung (Template)
└── kubernetes/
    └── kubernetes.go    # Kubernetes Context Handler
```

## Interfaces

### CloudProvider Interface

Jeder Cloud-Provider muss das `CloudProvider` Interface implementieren:

```go
type CloudProvider interface {
    Name() string                                          // Name des Providers (z.B. "AWS", "Azure")
    GetCredentials() ([]Credential, error)                // Hole alle verfügbaren Credentials
    Login(credential string, logFn func(string)) LoginResult // Führe Login durch
    TestConnection(logFn func(string))                     // Teste Verbindung
}
```

### ContextHandler Interface

Für Kontext-basierte Services (z.B. Kubernetes):

```go
type ContextHandler interface {
    Name() string                                         // Name des Handlers (z.B. "Kubernetes")
    GetCurrentContext() string                            // Aktueller Kontext
    GetContexts() ([]Context, error)                      // Alle verfügbaren Kontexte
    SwitchContext(contextName string, logFn func(string)) LoginResult
    TestConnection(logFn func(string))
}
```

## Neue Provider hinzufügen

### Beispiel: GCP Provider

1. **Ordner erstellen:**
   ```bash
   mkdir -p pkg/provider/gcp
   ```

2. **Provider implementieren:**
   ```go
   // pkg/provider/gcp/gcp.go
   package gcp

   import (
       "github.com/afeldman/cloudlogin/pkg/provider"
   )

   type GCPProvider struct {}

   func NewGCPProvider() *GCPProvider {
       return &GCPProvider{}
   }

   func (g *GCPProvider) Name() string {
       return "GCP"
   }

   func (g *GCPProvider) GetCredentials() ([]provider.Credential, error) {
       // Implementiere GCP Projekte Auslesen
       return nil, nil
   }

   func (g *GCPProvider) Login(projectID string, logFn func(string)) provider.LoginResult {
       // Implementiere GCP Auth
       return provider.LoginResult{}
   }

   func (g *GCPProvider) TestConnection(logFn func(string)) {
       // Teste GCP Verbindung
   }
   ```

3. **In main.go registrieren:**
   ```go
   import "github.com/afeldman/cloudlogin/pkg/provider/gcp"

   func NewProviderManager() *ProviderManager {
       pm := &ProviderManager{
           providers: make(map[string]provider.CloudProvider),
       }
       pm.RegisterProvider(aws.NewAWSProvider())
       pm.RegisterProvider(azure.NewAzureProvider())
       pm.RegisterProvider(gcp.NewGCPProvider())  // ← Hinzufügen
       return pm
   }
   ```

Das war's! Der neue Tab wird automatisch generiert.

## Provider Manager

Der `ProviderManager` verwaltet alle registrierten Provider:

```go
pm := NewProviderManager()
pm.RegisterProvider(myProvider)           // Provider registrieren
p := pm.GetProvider("AWS")                // Provider abrufen
providers := pm.GetAllProviders()         // Alle Provider abrufen
```

## GUI Generation

Die GUI wird dynamisch aus den registrierten Providern generiert:

- Für jeden `CloudProvider` wird ein Tab erstellt (via `createProviderTab()`)
- Für jeden `ContextHandler` wird ein Tab erstellt (via `createContextTab()`)
- Zusätzlich gibt es einen "Quick Actions" Tab

## Nächste Schritte

- ✅ AWS Provider fertig
- 📋 Azure Provider (Template vorhanden, JSON Parsing nötig)
- 🔄 GCP Provider
- 🔄 Okta / SSO Provider
- 🔄 OpenTelemetry Integration für Logging/Tracing
