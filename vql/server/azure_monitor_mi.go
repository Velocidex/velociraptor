//go:build sumo
// +build sumo

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

/*
Azure Monitor managed-identity / default-credential auth.

The azidentity-backed token sources for the managed_identity and
default_credential auth modes of azure_monitor_upload(). The service-principal
path (including AZURE_* environment variable credentials) uses
golang.org/x/oauth2 directly and lives in azure_monitor.go.

Note: azidentity manages its own HTTP stack for token acquisition (IMDS for
managed identity, the Entra endpoints for the default credential chain), so the
plugin's proxy / custom-CA / skip_verify settings do not apply to token
acquisition on these paths. Use the explicit service-principal path (or AZURE_*
env vars) if token acquisition must traverse a proxy or trust a custom CA.
*/
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"golang.org/x/oauth2"
	vfilter "www.velocidex.com/golang/vfilter"
)

// azureMonitorMITokenSource authenticates with a managed identity (optionally
// user-assigned via managed_identity_client_id).
func azureMonitorMITokenSource(
	ctx context.Context, scope vfilter.Scope,
	transport *http.Transport,
	arg *_AzureMonitorPluginArgs) (oauth2.TokenSource, error) {

	options := &azidentity.ManagedIdentityCredentialOptions{}
	if arg.ManagedIdentityClient != "" {
		options.ID = azidentity.ClientID(arg.ManagedIdentityClient)
	}

	cred, err := azidentity.NewManagedIdentityCredential(options)
	if err != nil {
		return nil, err
	}

	return newAzureADTokenSource(cred), nil
}

// azureMonitorDefaultTokenSource authenticates with the DefaultAzureCredential
// chain: AZURE_* env vars, workload identity, managed identity, Azure CLI -
// auto-detected by the SDK.
func azureMonitorDefaultTokenSource(
	ctx context.Context, scope vfilter.Scope,
	transport *http.Transport,
	arg *_AzureMonitorPluginArgs) (oauth2.TokenSource, error) {

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	return newAzureADTokenSource(cred), nil
}

// azureADCredTokenSource adapts an azcore.TokenCredential to an
// oauth2.TokenSource so it can be plugged into the same oauth2.Transport as the
// service-principal path.
type azureADCredTokenSource struct {
	cred azcore.TokenCredential
}

// newAzureADTokenSource wraps the credential in a caching token source.
// oauth2.Transport calls Token() on every single request and does no expiry
// caching of its own, so without ReuseTokenSource each upload would re-enter
// azidentity (for DefaultAzureCredential, the whole credential chain). This
// matches what clientcredentials.Config.TokenSource gives the
// service-principal path in azure_monitor.go for free.
func newAzureADTokenSource(cred azcore.TokenCredential) oauth2.TokenSource {
	return oauth2.ReuseTokenSource(nil, &azureADCredTokenSource{cred: cred})
}

func (self *azureADCredTokenSource) Token() (*oauth2.Token, error) {
	// Like the service-principal path in azure_monitor.go, this must not be
	// tied to the caller's context - token refresh still needs to succeed
	// during the shutdown flush, after that context is already cancelled.
	// A bounded timeout instead ensures a hung IMDS/Entra endpoint cannot
	// wedge the uploader forever.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tk, err := self.cred.GetToken(ctx,
		policy.TokenRequestOptions{Scopes: []string{azureMonitorScope}})
	if err != nil {
		return nil, err
	}

	return &oauth2.Token{
		AccessToken: tk.Token,
		Expiry:      tk.ExpiresOn,
		TokenType:   "Bearer",
	}, nil
}
