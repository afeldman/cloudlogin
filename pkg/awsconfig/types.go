package awsconfig

// ssoCacheEntry represents an entry from ~/.aws/sso/cache/*.json
type ssoCacheEntry struct {
	StartURL    string `json:"startUrl"`
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

// awsSSOAccount represents an AWS account from SSO API response
type awsSSOAccount struct {
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
}

type awsSSOAccountsResponse struct {
	AccountList []awsSSOAccount `json:"accountList"`
	NextToken   string          `json:"nextToken"`
}

// awsSSORole represents an AWS role from SSO API response
type awsSSORole struct {
	RoleName string `json:"roleName"`
}

type awsSSORolesResponse struct {
	RoleList  []awsSSORole `json:"roleList"`
	NextToken string       `json:"nextToken"`
}

// awsProfileEntry represents a single AWS profile to be written to config file
type awsProfileEntry struct {
	Name        string
	AccountID   string
	AccountName string
	RoleName    string
}

const (
	defaultSSORegion   = "eu-central-1"
	defaultSSOStartURL = "https://lynqtech.awsapps.com/start"
	awsConfigStartTag  = "# cloudlogin-managed-start"
	awsConfigEndTag    = "# cloudlogin-managed-end"
)
