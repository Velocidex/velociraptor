# Velociraptor SAML support

*NOTE: still WIP, tested with Simple SAML and Microsoft ADFS*

## Setting up Velociraptor with SAML login

There are four configuration values that need to be set in order to activate the Velociraptor SAML login feature:
- `saml_certificate`
  - SAML public certificate in PEM format
- `saml_private_key`
  - SAML private key in PEM format
- `saml_idp_metadata_url`
  - URL to IDP XML metadata
- `saml_root_url`
  - Velociraptor URL

These are expected to be set inside `server.config.yaml` under `GUI` key.

There is also an optional parameter `saml_user_attribute` to set which will be used as a user identificator. If it is not set, it will search for
the `name` attribute in the SAML response from the identity provider.

## Setting up testing environment

### Setting up Simple SAML

Easiest approach to test the SAML login feature is to use [test-saml-idp](https://hub.docker.com/r/kristophjunge/test-saml-idp/) docker image. 
Start the docker image by specifying the Velociraptor metadata URL as `SIMPLESAMLPHP_SP_ENTITY_ID` and Velociraptor ACS URL as `SIMPLESAMLPHP_SP_ASSERTION_CONSUMER_SERVICE`:

```
docker pull kristophjunge/test-saml-idp
docker run --name=testsamlidp_idp -p 8080:8080 -p 8443:8443 -e SIMPLESAMLPHP_SP_ENTITY_ID=https://localhost:8889/saml/metadata -e SIMPLESAMLPHP_SP_ASSERTION_CONSUMER_SERVICE=https://localhost:8889/saml/acs -d --rm kristophjunge/test-saml-idp
```

The docker image provides with two users which you can use to test the feature out:
- user1:user1pass
- user2:user2pass

Therefore, you need to have these users present in the Velociraptor users database.

### Configuring Velociraptor

To configure velociraptor for SAML logins you would need to generate your own SAML certificate and private key.
You can generate it with (courtesy of crewjam's SAML Go guide \[1\]):
```
openssl req -x509 -newkey rsa:2048 -keyout myservice.key -out myservice.cert -days 365 -nodes -subj "/CN=myservice.example.com"
```
This should be put into the Velociraptor server configuration in PEM format. The `myservice.cert` content is set as `saml_certificate` and `myservice.key` is set as `saml_private_key`.

If we assume that you've set up the Simple SAML on `localhost:8080`, you should be able to get the IDP metadata URL at `http://localhost:8080/simplesaml/saml2/idp/metadata.php?output=xhtml`. This URL should be specified as the `saml_idp_metadata_url` value. The `saml_root_url` is specified as the Velociraptor root URL which should be `https://localhost:8889` when testing locally.

At this point, you should be presented with Simple SAML login page when trying to visit the Velociraptor home page.

### Setting up with Microsoft ADFS

SAML login feature was tested with Microsoft ADFS and was working after claims were set up. Without these claims we've observed an `InvalidNameIDPolicy` status code.

```
I eventually got it working with ADFS. Here's my notes, which I recently re-implemented to get another working ADFS setup; I create three rules:

# LDAP transform

Attribute store = Active Directory
Outgoing claims:
E-Mail-Addresses -> E-Mail Address
SAM-Account-Name -> UPN
Display-Name -> Common-Name

# Transform an incoming claim

incoming type: email address
outgoing claim type: name id
outgoing name id format: transient identifier

# Add custom rule

c:[Type == "http://schemas.microsoft.com/ws/2008/06/identity/claims/windowsaccountname", Issuer == "AD AUTHORITY"]
=> issue(store = "Active Directory",
types = ("http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname",
"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname"),
query = ";mail,givenName,sn;{0}", param = c.Value);

This worked for me, but of course I don't know what exactly you're doing so mileage may vary.
```

The notes were copied from the following [link](https://github.com/crewjam/saml/issues/5#issuecomment-501328253). 
With this setup we've observed that the `saml_user_attribute` should be set to ` http://schemas.xmlsoap.org/ws/2005/05/identity/claims/upn`.

## Useful resources

1. SAML Go Library: https://github.com/crewjam/saml
