# Interactive configuration wizard.

The aim of the wizard is to make it easy to configure Velociraptor in
the most common deployment scenarios. Even though these scenarios will
not be a perfect fit for everyone, most users should be able to start
with these deploment modes and tweak the configuration to their
specific needs.

Although it is possible to mix and match configurations (for example
SSO with self signed certificates), the wizard does not offer these
combinations. We want to keep it as simple as possible and so minimize
choice to the most common choices.

There are currently 3 separate deployment types:

1. Self Signed:
  * Frontend will listen on port 8000 with self signed TLS certificate
  * GUI will listen on port 8889 with self signed TLS certificate
  * Basic authentication: Velociraptor will manage passwords for user accounts.

2. AutoCert:
  * Frontend and GUI are both listening with TLS on port 443
  * Certificates will be automatically minted with Let's Encrypt.
  * Basic authentication: Velociraptor will manage passwords for user accounts.

3. AutoCert with SSO
  * Frontend and GUI are both listening with TLS on port 443
  * Certificates will be automatically minted with Let's Encrypt.
  * SSO will be performed with OAuth using one of the most common providers:
     * Google
     * Azure
     * GitHub
     * Generic OIDC provider

## Breaking configuration questions by category

The Wizard goes through the configuration process grouping questions
by category

### Server Configuration

This configures the server itself, where to store the data, Frontend
host names etc.

Questions include:

* The type of deployment (Self Signed, AutoCert or AutoCert With SSO)
* Server Platform OS
* Data store paths
* Logging directories
* Internal PKI Expiration Time
* Add default plugin allow list

### Network configuration

This section configures the server's network parameters.

Questions include:
* Public DNS of the server frontend
* Optional features:
   * Built in Dynamic DNS updater if required.
   * Enable Websocket

If we are in Self Signed mode, the ports may be further configured:
* Frontend Listening port
* GUI Listening port

AutoCert modes must listen on ports 443 due to Let's Encrypt
limitations.

If the User selected the internal DNS Updater we also ask questions
about it:

For NOIP:
* NoIP DynDNS Username
* NoIP DynDNS Password

For Cloudflare
* Cloudflare Zone Name
* Cloudflare API Token

### Authentication configuration

This section configures the authentication providers

For Google or GitHub:
* OAuth Client ID
* OAuth Client Secret

For Azure an additional question:
* Tenant ID

For Generic OIDC:
* OIDC Issuer URL

Adding default administrators
