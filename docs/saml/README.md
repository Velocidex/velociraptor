# Velociraptor SAML support

Velociraptor SAML support was built with [crewjam's SAML Go library](https://github.com/crewjam/saml).
*NOTE: still WIP, tested only with Simple SAML and Microsoft ADFS*

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

There are also optional parameters:
- `saml_user_attribute` to set which will be used as a user identificator. If it is not set, it will search for the `name` attribute in the SAML response from the identity provider.
- `saml_user_roles` enables automatic creation of users and assigns them the configured roles when authenticated via SAML. If it is not set, then no users will be created automatically.

## Setting up testing environment

### Setting up Simple SAML

Easiest approach to test the SAML login feature is to use [test-saml-idp](https://hub.docker.com/r/kristophjunge/test-saml-idp/) docker image. 
Start the docker image by specifying the Velociraptor metadata URL as `SIMPLESAMLPHP_SP_ENTITY_ID` and Velociraptor ACS URL as `SIMPLESAMLPHP_SP_ASSERTION_CONSUMER_SERVICE`:

```
docker pull kristophjunge/test-saml-idp
docker run --name=testsamlidp_idp -p 8080:8080 -p 8443:8443 -e SIMPLESAMLPHP_SP_ENTITY_ID=https://localhost:8889/saml/metadata -e SIMPLESAMLPHP_SP_ASSERTION_CONSUMER_SERVICE=https://localhost:8889/saml/acs -d --rm kristophjunge/test-saml-idp
```

The docker image provides with two users which you can use to test the feature out:
- user1@example.com:user1pass
- user2@example.com:user2pass

Therefore, you need to have these users present in the Velociraptor users database.

### Configuring Velociraptor

To configure Velociraptor for SAML logins you would need to generate your own SAML certificate and private key.

This is tricky because Velociraptor does not trust unknown certificates, so you would need to sign your certificate with Velociraptor's CA.
You can find the Velociraptor CA inside `server.config.yaml` under `CA.private_key` - copy it into a separate file. After you have the
CA in a separate file (let's assume that the name is `VelociraptorCA.key`) you need to execute several commands 
to obtain SAML certificate and SAML private key (adapted from [fntlnz's gist](https://gist.github.com/fntlnz/cf14feb5a46b2eda428e000157447309) \[2\]):

```
openssl req -x509 -new -nodes -key VelociraptorCA.key -sha256 -days 1024 -out VelociraptorCA.crt
openssl genrsa -out example.com.key 2048
openssl req -new -key example.com.key -out example.com.csr
openssl x509 -req -in example.com.csr -CA VelociraptorCA.crt -CAkey VelociraptorCA.key -CAcreateserial -out example.com.crt -days 500 -sha256
```

The `example.com.crt` content is set as `saml_certificate` and `example.com.key` is set as `saml_private_key`.

If we assume that you've set up the Simple SAML on `localhost:8080`, you should be able to get the IDP metadata at `http://localhost:8080/simplesaml/saml2/idp/metadata.php`.
This URL should be specified as the `saml_idp_metadata_url` value. The `saml_root_url` is specified as the Velociraptor 
root URL which should be `https://localhost:8889` when testing locally.

To link user emails in Velociraptor database with SimpleSAML users, set `saml_user_attribute` to `email`.

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
2. Self signed certificate with custom CA: https://gist.github.com/fntlnz/cf14feb5a46b2eda428e000157447309