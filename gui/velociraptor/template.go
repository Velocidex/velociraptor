package velociraptor

// The template to expand into the index.html page
type HTMLtemplateArgs struct {
	Timestamp    int64
	Heading      string
	Help_url     string
	Report_url   string
	Version      string
	CsrfToken    string
	BasePath     string
	UserTheme    string
	Applications string

	// This is a JSON serialized instance of ErrState
	ErrState string
	OrgId    string
}

type AuthenticatorInfo struct {
	LoginURL       string
	ProviderAvatar string
	ProviderName   string
}

type ErrState struct {
	// Can be login error.
	Type           string
	Username       string
	Authenticators []AuthenticatorInfo
	BasePath       string
}
