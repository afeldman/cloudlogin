package provider

// CloudProvider definiert die Schnittstelle für Cloud-Provider
type CloudProvider interface {
	// Name gibt den Namen des Providers zurück (z.B. "AWS", "Azure")
	Name() string

	// Login führt den Login für diesen Provider durch
	Login(credential string, logFn func(string)) LoginResult

	// TestConnection testet die Verbindung zu diesem Provider
	TestConnection(logFn func(string))

	// GetCredentials gibt alle verfügbaren Credentials/Profiles zurück
	GetCredentials() ([]Credential, error)
}

// Credential repräsentiert eine Authentifizierungsmethode (z.B. AWS Profile, Azure Subscription)
type Credential struct {
	ID          string            // Eindeutige ID (z.B. Profilename, Subscription ID)
	DisplayName string            // Anzeigename
	Region      string            // Region/Location (optional)
	Details     map[string]string // Zusätzliche Details
}

// LoginResult ist das Ergebnis eines Login-Versuchs
type LoginResult struct {
	Success bool
	Message string
}

// ContextHandler definiert die Schnittstelle für Kontext-Management (z.B. Kubernetes)
type ContextHandler interface {
	// Name gibt den Namen des Handlers zurück (z.B. "Kubernetes")
	Name() string

	// GetCurrentContext gibt den aktuellen Kontext zurück
	GetCurrentContext() string

	// GetContexts gibt alle verfügbaren Kontexte zurück
	GetContexts() ([]Context, error)

	// SwitchContext wechselt zum angegebenen Kontext
	SwitchContext(contextName string, logFn func(string)) LoginResult

	// TestConnection testet die Verbindung
	TestConnection(logFn func(string))
}

// Context repräsentiert einen Kontext (z.B. Kubernetes Cluster)
type Context struct {
	Name    string
	Cluster string
	Server  string
	Details map[string]string // Zusätzliche Details
}
